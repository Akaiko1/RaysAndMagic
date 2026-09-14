package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestOrcishFuryDamageAtEveryMastery(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	attacker := cs.game.party.Members[0]
	weapon := attacker.Equipment[items.SlotMainHand]
	delete(attacker.Skills, character.SkillOrcishFury)
	base, _, _ := cs.CalculateWeaponDamage(weapon, attacker)

	tests := []struct {
		mastery character.SkillMastery
		want    int
	}{
		{character.MasteryNovice, 3},
		{character.MasteryExpert, 5},
		{character.MasteryMaster, 7},
		{character.MasteryGrandMaster, 10},
	}
	for _, tt := range tests {
		attacker.Skills[character.SkillOrcishFury] = &character.Skill{Mastery: tt.mastery}
		got, _, _ := cs.CalculateWeaponDamage(weapon, attacker)
		if got-base != tt.want {
			t.Errorf("%s Orcish Fury bonus = %d, want %d", tt.mastery, got-base, tt.want)
		}
	}
}

func TestOnlyOrcishFuryEntersMasteryUpgradePools(t *testing.T) {
	member := &character.MMCharacter{Skills: map[character.SkillType]*character.Skill{
		character.SkillCelestialProvidence: {Mastery: character.MasteryNovice},
		character.SkillOrcishFury:          {Mastery: character.MasteryNovice},
		character.SkillHalflingGuile:       {Mastery: character.MasteryNovice},
		character.SkillDarkElfBinding:      {Mastery: character.MasteryNovice},
	}}
	options := padLevelUpOptions(member, nil)
	if len(options) != 1 || options[0].skillType != character.SkillOrcishFury || !options[0].hasMastery {
		t.Fatalf("level-up racial options = %+v, want only trainable Orcish Fury", options)
	}
	trainers := trainerOptions(member)
	if len(trainers) != 1 || trainers[0].SkillType != character.SkillOrcishFury {
		t.Fatalf("trainer racial options = %+v, want only trainable Orcish Fury", trainers)
	}
}

func TestHalflingRandomTargetWeightIsHalf(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	normal := cs.game.party.Members[0]
	halfling := cs.game.party.Members[1]
	cs.game.party.Members = []*character.MMCharacter{normal, halfling}
	normal.Race = "human"
	halfling.Race = "halfling"
	normal.HitPoints, halfling.HitPoints = 100, 100

	const trials = 60000
	halflingHits := 0
	for i := 0; i < trials; i++ {
		if cs.randomLivingMember() == halfling {
			halflingHits++
		}
	}
	fraction := float64(halflingHits) / trials
	if math.Abs(fraction-1.0/3.0) > 0.02 {
		t.Fatalf("halfling target fraction = %.4f, want about 1/3 from weights 1:2", fraction)
	}
}

func TestHalflingRangedWeightAppliesAfterTankBiasInBothClocks(t *testing.T) {
	tests := []struct {
		name          string
		turnBased     bool
		halflingIndex int
	}{
		{"real-time halfling tank", false, 0},
		{"turn-based halfling tank", true, 0},
		{"real-time halfling off-tank", false, 1},
		{"turn-based halfling off-tank", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			cs.game.turnBasedMode = tt.turnBased
			members := cs.game.party.Members
			if len(members) < 3 {
				t.Skip("need >=3 members")
			}
			for i, member := range members {
				member.Race = "human"
				member.HitPoints = member.MaxHitPoints
				if i == tt.halflingIndex {
					member.Race = "halfling"
				}
			}
			const trials = 60000
			counts := make([]int, len(members))
			for i := 0; i < trials; i++ {
				target := cs.rangedTarget()
				for idx, member := range members {
					if target == member {
						counts[idx]++
						break
					}
				}
			}
			if tt.halflingIndex == 0 {
				// Raw tickets: tank=70*(n-1)/2, off-tanks=30 each.
				want := float64(70*(len(members)-1)) / float64(70*(len(members)-1)+60*(len(members)-1))
				got := float64(counts[0]) / trials
				if math.Abs(got-want) > 0.02 {
					t.Fatalf("halfling tank fraction = %.3f, want %.3f; counts=%v", got, want, counts)
				}
				return
			}
			ratio := float64(counts[tt.halflingIndex]) / float64(counts[2])
			if math.Abs(ratio-0.5) > 0.06 {
				t.Fatalf("halfling/non-halfling off-tank ratio = %.3f, want 0.5; counts=%v", ratio, counts)
			}
		})
	}
}

func newRacialTarget(name, monsterType string) *monster.Monster3D {
	return &monster.Monster3D{
		Name: name, MonsterType: monsterType,
		HitPoints: 1000, MaxHitPoints: 1000,
	}
}

func forceRacialProc(cs *CombatSystem, t *testing.T) {
	t.Helper()
	cs.racialProcRoll = func(chance int) bool {
		if chance != darkElfBindingChancePct {
			t.Fatalf("racial proc chance = %d, want %d", chance, darkElfBindingChancePct)
		}
		return true
	}
}

func TestDarkElfBindingEligibilityThroughMeleeEntry(t *testing.T) {
	tests := []struct {
		name         string
		monsterType  string
		boss         bool
		invulnerable bool
		wantBound    bool
	}{
		{"ordinary living mob", "beast", false, false, true},
		{"undead immunity", "undead", false, false, false},
		{"formless immunity", "formless", false, false, false},
		{"boss immunity", "beast", true, false, false},
		{"encounter invulnerability", "beast", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			forceRacialProc(cs, t)
			attacker := cs.game.party.Members[0]
			attacker.Race = "dark_elf"
			target := newRacialTarget(tt.name, tt.monsterType)
			target.Boss = tt.boss
			target.BossWarded = tt.invulnerable
			before := target.HitPoints
			weapon := attacker.Equipment[items.SlotMainHand]
			cs.ApplyDamageToMonster(target, 20, weapon.Name, false)
			if target.Bound != tt.wantBound {
				t.Fatalf("bound = %v, want %v", target.Bound, tt.wantBound)
			}
			if tt.wantBound && target.HitPoints != before {
				t.Fatalf("binding proc dealt damage: HP %d -> %d", before, target.HitPoints)
			}
			if !tt.wantBound && !tt.invulnerable && target.HitPoints >= before {
				t.Fatalf("immune target did not receive the original hit")
			}
		})
	}
}

func TestDarkElfBindingDirectHitEntryPoints(t *testing.T) {
	tests := []struct {
		name string
		hit  func(*CombatSystem, *character.MMCharacter, *monster.Monster3D)
	}{
		{
			"spell projectile",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				p := &MagicProjectile{ID: "racial-spell", Active: true, LifeTime: 10, Damage: 25, SpellType: "firebolt", Attacker: caster}
				cs.applyProjectileDamage(p, "magic_projectile", target, p.ID)
			},
		},
		{
			"weapon projectile",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				p := &Arrow{ID: "racial-arrow", Active: true, LifeTime: 10, Damage: 25, DamageType: "physical", Attacker: caster, Owner: ProjectileOwnerPlayer}
				cs.applyProjectileDamage(p, "arrow", target, p.ID)
			},
		},
		{
			"splash",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				center := newRacialTarget("center", "beast")
				center.X, center.Y = 64, 64
				target.X, target.Y = 64, 64
				cs.game.world.Monsters = []*monster.Monster3D{center, target}
				attack := cs.newPartyMonsterAttack(25, 0, "physical", 0, nil, "test", false, false, true)
				attack.Attacker = caster
				cs.applyAoeSplash(center, attack, 2)
			},
		},
		{
			"area stun cast",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				target.X, target.Y = cs.game.camera.X, cs.game.camera.Y
				cs.game.world.Monsters = []*monster.Monster3D{target}
				cs.tryCastAoeStunBy("racial-stun", spells.SpellDefinition{
					Name: "Racial Stun", StunRadiusTiles: 2, StunDurationSeconds: 2,
				}, caster)
			},
		},
		{
			"nova cast",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				cs.game.world.Monsters = []*monster.Monster3D{target}
				def, err := spells.GetSpellDefinitionByID("inferno")
				if err != nil {
					t.Fatalf("load Inferno: %v", err)
				}
				cs.tryCastInferno(def, caster)
			},
		},
		{
			"mortar detonation",
			func(cs *CombatSystem, caster *character.MMCharacter, target *monster.Monster3D) {
				target.X, target.Y = 64, 64
				cs.game.world.Monsters = []*monster.Monster3D{target}
				cs.detonateMortar(pendingMortar{SpellID: "stone_blossom", X: 64, Y: 64, Damage: 25, Caster: caster, RadiusTiles: 2, School: "earth"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			forceRacialProc(cs, t)
			caster := cs.game.party.Members[0]
			caster.Race = "dark_elf"
			target := newRacialTarget(tt.name, "beast")
			before := target.HitPoints
			tt.hit(cs, caster, target)
			if !target.Bound || target.HitPoints != before {
				t.Fatalf("entry did not replace hit with binding: bound=%v HP=%d want %d", target.Bound, target.HitPoints, before)
			}
		})
	}
}

func TestDarkElfBindingPersistentDamageZones(t *testing.T) {
	tests := []struct {
		spellID     spells.SpellID
		monsterType string
		wantBound   bool
	}{
		{"hot_steam", "beast", true},
		{"hot_steam", "undead", false},
		{"hot_steam", "formless", false},
		{"firewall", "beast", true},
		{"firewall", "undead", false},
		{"firewall", "formless", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.spellID)+"/"+tt.monsterType, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.world = newTestWorldSized(g.config, 20, 20)
			tile := float64(g.config.GetTileSize())
			g.camera.X, g.camera.Y = 5.5*tile, 5.5*tile
			caster := g.party.Members[0]
			caster.Race = "dark_elf"
			forceRacialProc(cs, t)
			def, err := spells.GetSpellDefinitionByID(tt.spellID)
			if err != nil {
				t.Fatalf("zone definition: %v", err)
			}
			if !cs.tryCastPersistentDamageZone(tt.spellID, def, caster).handled() || len(g.persistentDamageZones) == 0 {
				t.Fatal("persistent zone was not created")
			}
			fieldID := g.persistentDamageZones[0].FieldID
			for i := range g.persistentDamageZones {
				zone := &g.persistentDamageZones[i]
				if zone.FieldID == fieldID && zone.CasterName != caster.Name {
					t.Fatalf("zone cell %d caster = %q, want %q", i, zone.CasterName, caster.Name)
				}
			}
			target := newRacialTarget(tt.monsterType, tt.monsterType)
			target.ID = "zone-target"
			target.X, target.Y = g.persistentDamageZones[0].X, g.persistentDamageZones[0].Y
			g.world.Monsters = []*monster.Monster3D{target}
			before := target.HitPoints
			cs.applyZoneEntrySpell(string(tt.spellID))
			if target.Bound != tt.wantBound {
				t.Fatalf("bound = %v, want %v", target.Bound, tt.wantBound)
			}
			if tt.wantBound && target.HitPoints != before {
				t.Fatalf("binding zone dealt damage: HP %d -> %d", before, target.HitPoints)
			}
			if !tt.wantBound && target.HitPoints >= before {
				t.Fatalf("immune target did not receive the original zone hit")
			}
		})
	}
}

func TestWeaponDeathBurstPreservesRacialAttacker(t *testing.T) {
	tests := []struct {
		name      string
		race      string
		wantBound bool
	}{
		{name: "dark elf source binds", race: "dark_elf", wantBound: true},
		{name: "human source deals damage", race: "human", wantBound: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			forceRacialProc(cs, t)
			attacker := cs.game.party.Members[0]
			attacker.Race = tt.race
			corpse := newRacialTarget("burst corpse", "beast")
			corpse.ID = "burst-corpse"
			corpse.HitPoints = 0
			corpse.X, corpse.Y = 64, 64
			target := newRacialTarget("burst target", "beast")
			target.ID = "burst-target"
			target.X, target.Y = 64, 64
			cs.game.world.Monsters = []*monster.Monster3D{corpse, target}
			before := target.HitPoints

			cs.finishWeaponKill(corpse, &config.WeaponDefinitionConfig{
				Name: "Test Ember Egg", DeathBurstDamage: 25, DeathBurstRadiusTiles: 2,
			}, attacker)

			if target.Bound != tt.wantBound {
				t.Fatalf("death-burst target bound = %v, want %v", target.Bound, tt.wantBound)
			}
			if tt.wantBound && target.HitPoints != before {
				t.Fatalf("binding death burst dealt damage: HP %d -> %d", before, target.HitPoints)
			}
			if !tt.wantBound && target.HitPoints >= before {
				t.Fatalf("non-binding death burst dealt no damage: HP %d -> %d", before, target.HitPoints)
			}
		})
	}
}

func TestPersistentDamageZoneCasterIdentitySaveAndLegacyFallback(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	caster := g.party.Members[0]
	caster.Race = "dark_elf"
	forceRacialProc(cs, t)
	tile := float64(g.config.GetTileSize())

	tests := []struct {
		name       string
		casterName string
		wantBound  bool
	}{
		{"saved caster identity", caster.Name, true},
		{"legacy save without caster", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := newRacialTarget(tt.name, "beast")
			target.ID = "saved-zone-target"
			target.X, target.Y = tile, tile
			g.world.Monsters = []*monster.Monster3D{target}
			g.persistentDamageZones = restorePersistentDamageZones([]PersistentDamageZoneSave{{
				SpellID: "hot_steam", CasterName: tt.casterName, X: tile, Y: tile,
				Radius: tile, FramesLeft: 60, TickDamage: 10, IntervalFrames: 60,
			}}, "")
			before := target.HitPoints
			cs.applyZoneEntrySpell("hot_steam")
			if target.Bound != tt.wantBound {
				t.Fatalf("bound = %v, want %v", target.Bound, tt.wantBound)
			}
			if tt.wantBound && target.HitPoints != before {
				t.Fatalf("restored source proc dealt damage: HP %d -> %d", before, target.HitPoints)
			}
			if !tt.wantBound && target.HitPoints >= before {
				t.Fatal("legacy source-free zone did not retain its original damage behavior")
			}
		})
	}
}

func TestCelestialProvidenceRunsAtBothPhaseBoundaries(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	celestial := character.CreateRosterCharacter(g.config.Characters.TavernRecruits[3], g.config)
	if celestial == nil || celestial.Race != "celestial" {
		t.Fatal("celestial roster fixture missing")
	}
	g.party.Members = []*character.MMCharacter{celestial}

	assertOneMasterBuff := func() string {
		t.Helper()
		id := g.celestialBuffSpellID
		if id == "" {
			t.Fatal("Providence did not choose a buff")
		}
		total := len(g.statBuffs) + len(g.combatBuffs)
		if total != 1 {
			t.Fatalf("active Providence buff count = %d, want 1", total)
		}
		if len(g.statBuffs) == 1 && g.statBuffs[0].SourceID != celestialProvidenceSourceID {
			t.Fatalf("Providence stat source = %q", g.statBuffs[0].SourceID)
		}
		if len(g.combatBuffs) == 1 && g.combatBuffs[0].SourceID != celestialProvidenceSourceID {
			t.Fatalf("Providence combat source = %q", g.combatBuffs[0].SourceID)
		}
		switch id {
		case "day_of_the_gods":
			if g.combatBuffs[0].ResistPct != 23 {
				t.Fatalf("Master Day of the Gods = %d, want 23", g.combatBuffs[0].ResistPct)
			}
		case "hour_of_power":
			if g.combatBuffs[0].OutBonus != 11 || g.combatBuffs[0].InReduce != 3 {
				t.Fatalf("Master Hour of Power = %+v", g.combatBuffs[0])
			}
		case "bless":
			if g.statBuffs[0].Bonuses.Might != 8 {
				t.Fatalf("Master Bless = %+v, want +8", g.statBuffs[0].Bonuses)
			}
		case "stone_skin":
			if g.combatBuffs[0].InReduce != 8 {
				t.Fatalf("Master Stone Skin = %d, want 8", g.combatBuffs[0].InReduce)
			}
		case "heroism":
			if g.combatBuffs[0].OutBonus != 7 {
				t.Fatalf("Master Heroism = %d, want 7", g.combatBuffs[0].OutBonus)
			}
		default:
			t.Fatalf("unexpected Providence buff %q", id)
		}
		return id
	}

	g.applyDayNightPhase(true)
	first := assertOneMasterBuff()
	g.applyDayNightPhase(false)
	second := assertOneMasterBuff()
	if first != second {
		if _, ok := g.statBuffByID(first); ok {
			t.Fatalf("previous stat buff %q survived phase replacement", first)
		}
		if _, ok := g.combatBuffByID(first); ok {
			t.Fatalf("previous combat buff %q survived phase replacement", first)
		}
	}
}

func TestCelestialProvidenceOwnershipAcrossRecastAndRemoval(t *testing.T) {
	tests := []struct {
		name    string
		spellID string
		install func(*MMGame, string)
		lookup  func(*MMGame, string) (string, int, bool)
	}{
		{
			name: "stat buff", spellID: "bless",
			install: func(g *MMGame, source string) {
				g.addStatBuff(TimedStatBuff{SpellID: "bless", SourceID: source, Frames: 60, Bonuses: character.UniformStatBonuses(8)})
			},
			lookup: func(g *MMGame, id string) (string, int, bool) {
				b, ok := g.statBuffByID(id)
				return b.SourceID, b.Frames, ok
			},
		},
		{
			name: "combat buff", spellID: "heroism",
			install: func(g *MMGame, source string) {
				g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", SourceID: source, Frames: 60, OutBonus: 7})
			},
			lookup: func(g *MMGame, id string) (string, int, bool) {
				b, ok := g.combatBuffByID(id)
				return b.SourceID, b.Frames, ok
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" phase cleanup", func(t *testing.T) {
			g := newTestCombatSystemWithConfig(t).game
			g.party.Members = nil
			tt.install(g, celestialProvidenceSourceID)
			g.celestialBuffSpellID = tt.spellID
			g.refreshCelestialProvidence()
			if _, _, ok := tt.lookup(g, tt.spellID); ok {
				t.Fatal("phase cleanup left the Providence-owned entry active")
			}
		})
		t.Run(tt.name+" player recast", func(t *testing.T) {
			g := newTestCombatSystemWithConfig(t).game
			g.party.Members = nil
			tt.install(g, celestialProvidenceSourceID)
			g.celestialBuffSpellID = tt.spellID
			tt.install(g, "")
			if tt.spellID == "bless" {
				g.statBuffs[0].Frames = 999
			} else {
				g.combatBuffs[0].Frames = 999
			}
			g.refreshCelestialProvidence()
			source, frames, ok := tt.lookup(g, tt.spellID)
			if !ok || source != "" || frames != 999 {
				t.Fatalf("player recast after phase cleanup = source %q frames %d ok=%v", source, frames, ok)
			}
		})
	}
}

func TestCelestialProvidencePreservesActiveBuffOwnership(t *testing.T) {
	tests := []struct {
		name             string
		manualSpellID    spells.SpellID
		fallbackSpellID  spells.SpellID
		installManual    func(*MMGame)
		manualStillOwned func(*MMGame) bool
		fallbackOwned    func(*MMGame) bool
	}{
		{
			name: "manual stat falls back to combat", manualSpellID: "bless", fallbackSpellID: "heroism",
			installManual: func(g *MMGame) {
				g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 999, Bonuses: character.UniformStatBonuses(10)})
			},
			manualStillOwned: func(g *MMGame) bool {
				b, ok := g.statBuffByID("bless")
				return ok && b.SourceID == "" && b.Frames == 999 && b.Bonuses.Might == 10
			},
			fallbackOwned: func(g *MMGame) bool {
				b, ok := g.combatBuffByID("heroism")
				return ok && b.SourceID == celestialProvidenceSourceID
			},
		},
		{
			name: "manual combat falls back to stat", manualSpellID: "heroism", fallbackSpellID: "bless",
			installManual: func(g *MMGame) {
				g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 999, OutBonus: 10})
			},
			manualStillOwned: func(g *MMGame) bool {
				b, ok := g.combatBuffByID("heroism")
				return ok && b.SourceID == "" && b.Frames == 999 && b.OutBonus == 10
			},
			fallbackOwned: func(g *MMGame) bool {
				b, ok := g.statBuffByID("bless")
				return ok && b.SourceID == celestialProvidenceSourceID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldPool := celestialProvidenceBuffPool
			celestialProvidenceBuffPool = []spells.SpellID{tt.manualSpellID, tt.fallbackSpellID}
			t.Cleanup(func() { celestialProvidenceBuffPool = oldPool })

			g := newTestCombatSystemWithConfig(t).game
			celestial := character.CreateRosterCharacter(g.config.Characters.TavernRecruits[3], g.config)
			g.party.Members = []*character.MMCharacter{celestial}
			tt.installManual(g)

			g.refreshCelestialProvidence()
			if !tt.manualStillOwned(g) {
				t.Fatal("Providence changed the active player-owned buff")
			}
			if g.celestialBuffSpellID != string(tt.fallbackSpellID) || !tt.fallbackOwned(g) {
				t.Fatalf("Providence fallback = %q, want %q with racial ownership", g.celestialBuffSpellID, tt.fallbackSpellID)
			}

			g.party.Members = nil
			g.refreshCelestialProvidence()
			if !tt.manualStillOwned(g) {
				t.Fatal("next-phase cleanup removed the player-owned buff")
			}
			if g.celestialBuffSpellID != "" {
				t.Fatalf("racial marker survived cleanup: %q", g.celestialBuffSpellID)
			}
		})
	}

	t.Run("all choices already player-owned", func(t *testing.T) {
		oldPool := celestialProvidenceBuffPool
		celestialProvidenceBuffPool = []spells.SpellID{"bless", "heroism"}
		t.Cleanup(func() { celestialProvidenceBuffPool = oldPool })

		g := newTestCombatSystemWithConfig(t).game
		celestial := character.CreateRosterCharacter(g.config.Characters.TavernRecruits[3], g.config)
		g.party.Members = []*character.MMCharacter{celestial}
		g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 999, Bonuses: character.UniformStatBonuses(10)})
		g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 999, OutBonus: 10})

		g.refreshCelestialProvidence()
		bless, blessOK := g.statBuffByID("bless")
		heroism, heroismOK := g.combatBuffByID("heroism")
		if !blessOK || bless.SourceID != "" || bless.Frames != 999 ||
			!heroismOK || heroism.SourceID != "" || heroism.Frames != 999 {
			t.Fatalf("all-blocked Providence changed manual buffs: bless=%+v heroism=%+v", bless, heroism)
		}
		if g.celestialBuffSpellID != "" {
			t.Fatalf("all-blocked Providence claimed %q", g.celestialBuffSpellID)
		}
	})
}

func TestCelestialProvidenceSourcePersistsAndMigrates(t *testing.T) {
	stat := restoreStatBuffs(buildStatBuffSaves([]TimedStatBuff{{
		SpellID: "bless", SourceID: celestialProvidenceSourceID, Frames: 42,
		Bonuses: character.UniformStatBonuses(8),
	}}))
	combat := restoreCombatBuffs(buildCombatBuffSaves([]TimedCombatBuff{{
		SpellID: "heroism", SourceID: celestialProvidenceSourceID, Frames: 42, OutBonus: 7,
	}}))
	if len(stat) != 1 || stat[0].SourceID != celestialProvidenceSourceID ||
		len(combat) != 1 || combat[0].SourceID != celestialProvidenceSourceID {
		t.Fatalf("source round trip failed: stat=%+v combat=%+v", stat, combat)
	}

	tests := []struct {
		name   string
		stat   []TimedStatBuff
		combat []TimedCombatBuff
	}{
		{"legacy stat", []TimedStatBuff{{SpellID: "bless", Frames: 42}}, nil},
		{"legacy combat", nil, []TimedCombatBuff{{SpellID: "heroism", Frames: 42}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestCombatSystemWithConfig(t).game
			g.statBuffs, g.combatBuffs = tt.stat, tt.combat
			if len(tt.stat) != 0 {
				g.celestialBuffSpellID = tt.stat[0].SpellID
			} else {
				g.celestialBuffSpellID = tt.combat[0].SpellID
			}
			g.restoreCelestialProvidenceOwnership(0)
			if len(g.statBuffs) != 0 && g.statBuffs[0].SourceID != celestialProvidenceSourceID {
				t.Fatalf("legacy stat source = %q", g.statBuffs[0].SourceID)
			}
			if len(g.combatBuffs) != 0 && g.combatBuffs[0].SourceID != celestialProvidenceSourceID {
				t.Fatalf("legacy combat source = %q", g.combatBuffs[0].SourceID)
			}
		})
	}

	t.Run("source-aware player recast", func(t *testing.T) {
		g := newTestCombatSystemWithConfig(t).game
		g.statBuffs = []TimedStatBuff{{SpellID: "bless", Frames: 42}}
		g.celestialBuffSpellID = "bless"
		g.restoreCelestialProvidenceOwnership(timedBuffSourceSaveVersion)
		if g.statBuffs[0].SourceID != "" {
			t.Fatalf("player recast was remigrated to source %q", g.statBuffs[0].SourceID)
		}
	})
}

func TestCelestialProvidenceCannotBeDispelledAsAnOrdinaryCast(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.addStatBuff(TimedStatBuff{
		SpellID: "bless", SourceID: celestialProvidenceSourceID, Frames: 60,
		Bonuses: character.UniformStatBonuses(8),
	})
	g.addCombatBuff(TimedCombatBuff{
		SpellID: "heroism", SourceID: celestialProvidenceSourceID, Frames: 60, OutBonus: 7,
	})
	dispeller := &monster.Monster3D{Name: "Dispeller", DispelChance: 1}
	cs.tryApplyMonsterDispel(dispeller, g.party.Members[0])
	if len(g.statBuffs) != 1 || len(g.combatBuffs) != 1 {
		t.Fatalf("phase-bound buffs entered the monster dispel pool: stat=%d combat=%d", len(g.statBuffs), len(g.combatBuffs))
	}
	if g.removeStatBuff("bless") || g.removeCombatBuff("heroism") {
		t.Fatal("spell-id/manual dispel removed a phase-bound Providence buff")
	}

	// A player recast of the same spell replaces ownership and becomes normally
	// dispellable again.
	g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", Frames: 90, OutBonus: 3})
	if !g.removeCombatBuff("heroism") {
		t.Fatal("player-owned recast was incorrectly protected from dispel")
	}
}

func TestCharacterSavePersistsRaceAndRacialSkill(t *testing.T) {
	member := &character.MMCharacter{
		Name: "Nyra", Race: "dark_elf", Class: character.ClassThief,
		Skills: map[character.SkillType]*character.Skill{
			character.SkillDarkElfBinding: {Mastery: character.MasteryNovice},
		},
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{},
		Equipment:    map[items.EquipSlot]items.Item{},
	}
	restored := restoreCharacterSave(buildCharacterSave(member))
	if restored.Race != member.Race || restored.Skills[character.SkillDarkElfBinding] == nil {
		t.Fatalf("race save round trip = race %q skills %+v", restored.Race, restored.Skills)
	}
}
