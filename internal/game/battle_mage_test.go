package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// The Battle Mage kit contract: skills, four schools with their authored
// spells, and the three-piece starting equipment routed by slot.
func TestBattleMageClassKit(t *testing.T) {
	cfg := loadTestConfig(t)
	ch := character.CreateCharacter("Isolde", character.ClassBattleMage, cfg)

	for _, skill := range []character.SkillType{
		character.SkillSword, character.SkillMace, character.SkillAxe,
		character.SkillPlate, character.SkillBodybuilding,
		character.SkillSpellAbsorption, character.SkillStrongMagic,
	} {
		if !ch.HasSkill(skill) {
			t.Errorf("battle mage kit is missing skill %s", skill)
		}
	}
	wantSpells := map[character.MagicSchoolID]spells.SpellID{
		"light": "resurrect",
		"earth": "rock_blast",
		"fire":  "firewall",
		"air":   "sparks",
	}
	for school, spellID := range wantSpells {
		ms, ok := ch.MagicSchools[school]
		if !ok {
			t.Errorf("school %s is not open", school)
			continue
		}
		found := false
		for _, known := range ms.KnownSpells {
			if known == spellID {
				found = true
			}
		}
		if !found {
			t.Errorf("school %s does not know %s: %v", school, spellID, ms.KnownSpells)
		}
	}
	if got := ch.Equipment[items.SlotMainHand]; got.Name != "Silver Sword" {
		t.Errorf("main hand = %q, want Silver Sword", got.Name)
	}
	if got := ch.Equipment[items.SlotArmor]; got.Name != "Iron Armor" {
		t.Errorf("armor slot = %q, want Iron Armor", got.Name)
	}
	if got := ch.Equipment[items.SlotRing1]; got.Name != "Magic Ring" {
		t.Errorf("ring slot = %q, want Magic Ring", got.Name)
	}
	// The class is registered end to end: key round-trip, roster, blurb.
	if key := character.ClassBattleMage.Key(); key != "battle_mage" {
		t.Fatalf("class key = %q", key)
	}
	if cls, ok := character.ClassFromKey("battle_mage"); !ok || cls != character.ClassBattleMage {
		t.Fatal("ClassFromKey does not resolve battle_mage")
	}
	if character.ClassBattleMage.String() != "Battle Mage" || character.ClassBattleMage.Blurb() == "" {
		t.Fatal("battle mage String/Blurb incomplete")
	}
	found := false
	for _, cls := range character.PlayableClasses {
		if cls == character.ClassBattleMage {
			found = true
		}
	}
	if !found {
		t.Fatal("battle mage missing from PlayableClasses")
	}
	recruit := false
	for _, entry := range cfg.Characters.TavernRecruits {
		if entry.Name == "Isolde" && entry.Class == "battle_mage" {
			recruit = true
		}
	}
	if !recruit {
		t.Fatal("Isolde (battle_mage) missing from tavern_recruits")
	}
}

func absorptionTestMember(cs *CombatSystem, tier character.SkillMastery) *character.MMCharacter {
	member := cs.game.party.Members[0]
	member.Skills[character.SkillSpellAbsorption] = &character.Skill{Mastery: tier}
	member.Luck = 0 // no Perfect Dodge noise in the non-absorb rows
	member.MaxHitPoints, member.MaxSpellPoints = 500, 500
	member.HitPoints, member.SpellPoints = 100, 10
	return member
}

// The Spell Absorption rule, one row per damage channel: only spell-labeled
// channels can be absorbed, and an absorbed hit deals nothing while restoring
// HP and SP equal to the packet's own damage. Non-spell rows are deterministic;
// spell rows sample the real roll (miss probability at GM over 400 tries is
// 0.4^400, i.e. never).
func TestSpellAbsorptionChannelTable(t *testing.T) {
	if got := []int{
		character.SpellAbsorbChancePct(0), character.SpellAbsorbChancePct(1),
		character.SpellAbsorbChancePct(2), character.SpellAbsorbChancePct(3),
	}; got[0] != 15 || got[1] != 30 || got[2] != 45 || got[3] != 60 {
		t.Fatalf("absorption chance ladder = %v, want 15/30/45/60", got)
	}

	// Channel labeling at the constructors (the "spell label revision" rows).
	mob := mkTestMonster("Goblin", 1000)
	if hitFromMonster(mob, 10, "fire", false, 0, true, false).Spell {
		t.Fatal("melee channel must not be a spell")
	}
	if !hitFromMonster(mob, 10, "fire", false, 0, false, true).Spell {
		t.Fatal("dragon-breath channel must be a spell")
	}

	tests := []struct {
		name       string
		spell      bool
		viaParts   bool // drive damagePartyMemberParts instead of monsterHitCharacter
		absorbable bool
	}{
		{name: "melee hit is never absorbed", spell: false, absorbable: false},
		{name: "weapon dart is never absorbed", spell: false, absorbable: false},
		{name: "spell projectile can be absorbed", spell: true, absorbable: true},
		{name: "fireburst channel can be absorbed", spell: true, viaParts: true, absorbable: true},
		{name: "crate trap channel is never absorbed", spell: false, viaParts: true, absorbable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			member := absorptionTestMember(cs, character.MasteryGrandMaster)
			mob := mkTestMonster("Goblin", 1000)
			parts := damagecalc.Parts{Normal: 10, True: 5}
			absorbed := false
			for try := 0; try < 400; try++ {
				member.HitPoints, member.SpellPoints = 100, 10
				if tt.viaParts {
					cs.damagePartyMemberParts(0, member, parts, "fire", tt.spell)
				} else {
					hit := monsterCharacterHit{Parts: parts, DamageType: "fire", Spell: tt.spell}
					cs.monsterHitCharacter(mob, member, "Test Mob", hit)
				}
				if member.HitPoints > 100 || member.SpellPoints > 10 {
					absorbed = true
					if member.HitPoints != 100+parts.Total() {
						t.Fatalf("absorb restored %d HP, want +%d", member.HitPoints-100, parts.Total())
					}
					if member.SpellPoints != 10+parts.Total() {
						t.Fatalf("absorb restored %d SP, want +%d", member.SpellPoints-10, parts.Total())
					}
					break
				}
			}
			if absorbed != tt.absorbable {
				t.Fatalf("absorbed = %v, want %v", absorbed, tt.absorbable)
			}
		})
	}

	t.Run("no skill: a spell hit always lands", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		member := cs.game.party.Members[0]
		member.Luck = 0
		member.HitPoints, member.SpellPoints = 100, 10
		delete(member.Skills, character.SkillSpellAbsorption)
		mob := mkTestMonster("Goblin", 1000)
		for try := 0; try < 50; try++ {
			hit := monsterCharacterHit{Parts: damagecalc.Parts{Normal: 10}, DamageType: "fire", Spell: true}
			cs.monsterHitCharacter(mob, member, "Test Mob", hit)
			if member.HitPoints > 100 || member.SpellPoints > 10 {
				t.Fatal("a member without the skill absorbed a spell")
			}
			member.HitPoints = 100
		}
	})

	// HP and SP clamp independently, and the combat line must report the actual
	// deltas rather than the incoming packet's uncapped total.
	for _, tt := range []struct {
		name                   string
		maxHP, maxSP           int
		wantHPGain, wantSPGain int
		wantMessage            string
	}{
		{name: "neither resource capped", maxHP: 200, maxSP: 200, wantHPGain: 50, wantSPGain: 50, wantMessage: "+50 HP, +50 SP"},
		{name: "HP capped only", maxHP: 105, maxSP: 200, wantHPGain: 5, wantSPGain: 50, wantMessage: "+5 HP, +50 SP"},
		{name: "SP capped only", maxHP: 200, maxSP: 12, wantHPGain: 50, wantSPGain: 2, wantMessage: "+50 HP, +2 SP"},
		{name: "both resources capped", maxHP: 105, maxSP: 12, wantHPGain: 5, wantSPGain: 2, wantMessage: "+5 HP, +2 SP"},
	} {
		t.Run("reported restoration/"+tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			member := absorptionTestMember(cs, character.MasteryGrandMaster)
			member.MaxHitPoints, member.MaxSpellPoints = tt.maxHP, tt.maxSP
			parts := damagecalc.Parts{Normal: 50}
			for try := 0; try < 400; try++ {
				member.HitPoints, member.SpellPoints = 100, 10
				if !cs.tryAbsorbSpellHit(member, parts, true, "Test") {
					continue
				}
				if got := member.HitPoints - 100; got != tt.wantHPGain {
					t.Fatalf("HP gain = %d, want %d", got, tt.wantHPGain)
				}
				if got := member.SpellPoints - 10; got != tt.wantSPGain {
					t.Fatalf("SP gain = %d, want %d", got, tt.wantSPGain)
				}
				last := cs.game.combatLogHistory[len(cs.game.combatLogHistory)-1].Text
				if !strings.Contains(last, tt.wantMessage) {
					t.Fatalf("absorption message %q does not report %q", last, tt.wantMessage)
				}
				return
			}
			t.Fatal("no absorb in 400 tries at 60% - the roll is broken")
		})
	}

	// Inferno is a spell, but its party splash belongs to the party caster. It
	// must never enter the hostile-spell absorption gate.
	t.Run("friendly Inferno self-splash is never absorbed", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		cs.game.world = newTestWorldSized(cs.game.config, 20, 20)
		ts := float64(cs.game.config.GetTileSize())
		cs.game.camera.X, cs.game.camera.Y = 5.5*ts, 5.5*ts
		member := absorptionTestMember(cs, character.MasteryGrandMaster)
		member.MaxHitPoints = 10000
		def, err := spells.GetSpellDefinitionByID("inferno")
		if err != nil {
			t.Fatalf("inferno definition: %v", err)
		}
		for try := 0; try < 100; try++ {
			member.HitPoints = member.MaxHitPoints
			if !cs.tryCastInferno(def, member) {
				t.Fatal("inferno was not handled")
			}
			if member.HitPoints >= member.MaxHitPoints {
				t.Fatalf("friendly Inferno was absorbed on try %d", try+1)
			}
		}
	})
}

// The Strong Magic exchange: the ladder, the one damage-builder boost (combat
// AND tooltips read spellDamageParts), the HP burn at the one SP-payment site,
// and the never-self-kill clamp.
func TestStrongMagicExchangeTable(t *testing.T) {
	if got := []int{
		character.StrongMagicPct(0), character.StrongMagicPct(1),
		character.StrongMagicPct(2), character.StrongMagicPct(3),
	}; got[0] != 25 || got[1] != 50 || got[2] != 75 || got[3] != 100 {
		t.Fatalf("strong magic ladder = %v, want 25/50/75/100", got)
	}

	cs := newTestCombatSystemWithConfig(t)
	caster := cs.game.party.Members[0]
	rockBlast, err := spells.GetSpellDefinitionByID("rock_blast")
	if err != nil {
		t.Fatalf("rock_blast definition: %v", err)
	}
	heal, err := spells.GetSpellDefinitionByID("heal")
	if err != nil {
		t.Fatalf("heal definition: %v", err)
	}

	baseline := cs.spellDamageParts("rock_blast", caster, 40).Total()
	caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
	boosted := cs.spellDamageParts("rock_blast", caster, 40).Total()
	if boosted != baseline*2 {
		t.Fatalf("GM Strong Magic damage = %d, want double the baseline %d", boosted, baseline)
	}
	if pct := strongMagicPct(caster, heal); pct != 0 {
		t.Fatalf("a heal must not be boosted, got %d%%", pct)
	}

	tests := []struct {
		name     string
		tier     character.SkillMastery
		cost     int
		hpBefore int
		wantHP   int
	}{
		{name: "novice burns a quarter of the cost", tier: character.MasteryNovice, cost: 20, hpBefore: 100, wantHP: 95},
		{name: "grandmaster burns the full cost", tier: character.MasteryGrandMaster, cost: 20, hpBefore: 100, wantHP: 80},
		{name: "burn never takes the last hit point", tier: character.MasteryGrandMaster, cost: 20, hpBefore: 1, wantHP: 1},
		{name: "burn clamps to leave one hit point", tier: character.MasteryGrandMaster, cost: 20, hpBefore: 15, wantHP: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: tt.tier}
			caster.HitPoints = tt.hpBefore
			cs.applyStrongMagicBurn(caster, rockBlast, tt.cost)
			if caster.HitPoints != tt.wantHP {
				t.Fatalf("HP after burn = %d, want %d", caster.HitPoints, tt.wantHP)
			}
		})
	}

	t.Run("the real cast pays SP and burns HP together", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		caster := cs.game.party.Members[0]
		caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
		caster.HitPoints, caster.MaxHitPoints = 200, 200
		caster.SpellPoints, caster.MaxSpellPoints = 100, 100
		cost := cs.effectiveSpellCost(caster, rockBlast.SpellPointsCost)
		if !cs.castResolvedSpell("rock_blast", rockBlast, caster, cost, false, false) {
			t.Fatal("rock_blast cast failed")
		}
		if caster.SpellPoints != 100-cost {
			t.Fatalf("SP after cast = %d, want %d", caster.SpellPoints, 100-cost)
		}
		if want := 200 - cost; caster.HitPoints != want {
			t.Fatalf("HP after GM cast = %d, want %d (burn == full cost)", caster.HitPoints, want)
		}
	})
}

// Strong Magic must reach the packet that actually FLIES, for every cast form:
// a plain projectile (rock_blast), an AoE projectile (fireball), and a zone
// (firewall - every tick). Each row casts through the real castResolvedSpell
// twice with one caster - baseline without the skill, then GM - and demands
// exact doubling. Luck=0 pins the crit roll out of both samples.
func TestStrongMagicBoostsEveryCastForm(t *testing.T) {
	newCaster := func(cs *CombatSystem) *character.MMCharacter {
		caster := cs.game.party.Members[0]
		caster.Luck = 0
		caster.HitPoints, caster.MaxHitPoints = 500, 500
		caster.SpellPoints, caster.MaxSpellPoints = 200, 200
		delete(caster.Skills, character.SkillStrongMagic)
		return caster
	}
	cast := func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, spellID spells.SpellID) {
		t.Helper()
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err != nil {
			t.Fatalf("%s definition: %v", spellID, err)
		}
		cost := cs.effectiveSpellCost(caster, def.SpellPointsCost)
		if !cs.castResolvedSpell(spellID, def, caster, cost, false, false) {
			t.Fatalf("%s cast failed", spellID)
		}
	}

	for _, tt := range []struct {
		name    string
		spellID spells.SpellID
	}{
		{name: "plain projectile rock_blast", spellID: "rock_blast"},
		{name: "aoe projectile fireball", spellID: "fireball"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			caster := newCaster(cs)

			cast(t, cs, caster, tt.spellID)
			if len(cs.game.magicProjectiles) == 0 {
				t.Fatalf("%s spawned no projectile", tt.spellID)
			}
			base := cs.game.magicProjectiles[len(cs.game.magicProjectiles)-1]
			if base.Damage <= 0 {
				t.Fatalf("baseline projectile damage = %d", base.Damage)
			}

			caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
			cast(t, cs, caster, tt.spellID)
			boosted := cs.game.magicProjectiles[len(cs.game.magicProjectiles)-1]
			if boosted.Damage != base.Damage*2 || boosted.TrueDamage != base.TrueDamage*2 {
				t.Fatalf("GM %s in flight = %d/%d true, want exactly double the baseline %d/%d",
					tt.spellID, boosted.Damage, boosted.TrueDamage, base.Damage, base.TrueDamage)
			}
		})
	}

	t.Run("zone firewall ticks", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		g := cs.game
		g.world = newTestWorldSized(g.config, 20, 20)
		ts := float64(g.config.GetTileSize())
		g.camera.X, g.camera.Y = 5.5*ts, 5.5*ts
		caster := newCaster(cs)

		g.persistentDamageZones = g.persistentDamageZones[:0]
		cast(t, cs, caster, "firewall")
		if len(g.persistentDamageZones) == 0 {
			t.Fatal("firewall laid no zone cells")
		}
		base := g.persistentDamageZones[0].TickDamage
		if base <= 0 {
			t.Fatalf("baseline tick damage = %d", base)
		}

		caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
		g.persistentDamageZones = g.persistentDamageZones[:0]
		cast(t, cs, caster, "firewall")
		if len(g.persistentDamageZones) == 0 {
			t.Fatal("boosted firewall laid no zone cells")
		}
		if got := g.persistentDamageZones[0].TickDamage; got != base*2 {
			t.Fatalf("GM firewall tick = %d, want exactly double the baseline %d", got, base)
		}
	})
}

// A flat outgoing party buff is the final additive stage for every spell form:
// Strong Magic owns and multiplies only the spell packet, never the flat buff.
// Each row drives the real damage-delivery entry point for that form.
func TestStrongMagicOutgoingBuffOrderTable(t *testing.T) {
	const (
		baseDamage = 40
		flatBonus  = 7
	)
	type formResult struct {
		got, want int
	}
	tests := []struct {
		name  string
		apply func(*testing.T, *CombatSystem, *character.MMCharacter, *monsterPkg.Monster3D) formResult
	}{
		{
			name: "projectile",
			apply: func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, mob *monsterPkg.Monster3D) formResult {
				parts := cs.spellDamageParts("rock_blast", caster, baseDamage)
				projectile := &MagicProjectile{
					ID: "strong-magic-projectile", Active: true, LifeTime: 60,
					Damage: parts.Normal, TrueDamage: parts.True, SpellType: "rock_blast", Attacker: caster,
				}
				before := mob.HitPoints
				cs.applyProjectileDamage(projectile, "magic_projectile", mob, projectile.ID)
				return formResult{got: before - mob.HitPoints, want: parts.Total() + flatBonus}
			},
		},
		{
			name: "persistent zone",
			apply: func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, mob *monsterPkg.Monster3D) formResult {
				parts := cs.spellDamageParts("firewall", caster, baseDamage)
				zone := &PersistentDamageZone{
					SpellID: "firewall", FieldID: 1, X: mob.X, Y: mob.Y,
					Radius: float64(cs.game.config.GetTileSize()), FramesLeft: 60,
					TickDamage: parts.Normal, TrueTickDamage: parts.True,
				}
				before := mob.HitPoints
				cs.damageZoneMonsters("firewall", []*PersistentDamageZone{zone}, []*PersistentDamageZone{zone})
				return formResult{got: before - mob.HitPoints, want: parts.Total() + flatBonus}
			},
		},
		{
			name: "mortar",
			apply: func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, mob *monsterPkg.Monster3D) formResult {
				parts := cs.spellDamageParts("stone_blossom", caster, baseDamage)
				before := mob.HitPoints
				cs.detonateMortar(pendingMortar{
					X: mob.X, Y: mob.Y, SpellID: "stone_blossom",
					Damage: parts.Normal, TrueDamage: parts.True, Caster: caster,
					RadiusTiles: 2, School: "earth",
				})
				return formResult{got: before - mob.HitPoints, want: parts.Total() + flatBonus}
			},
		},
		{
			name: "party nova against monsters",
			apply: func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, mob *monsterPkg.Monster3D) formResult {
				def, err := spells.GetSpellDefinitionByID("inferno")
				if err != nil {
					t.Fatalf("inferno definition: %v", err)
				}
				parts := cs.spellDamageParts(def.ID, caster, cs.CalculateInfernoDamage(def, caster))
				before := mob.HitPoints
				if !cs.tryCastInferno(def, caster) {
					t.Fatal("inferno was not handled")
				}
				return formResult{got: before - mob.HitPoints, want: parts.Total() + flatBonus}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			cs.game.world = newTestWorldSized(cs.game.config, 20, 20)
			ts := float64(cs.game.config.GetTileSize())
			cs.game.camera.X, cs.game.camera.Y = 5.5*ts, 5.5*ts
			caster := cs.game.party.Members[0]
			caster.Luck = 0
			caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
			cs.game.combatBuffs = []TimedCombatBuff{{
				SpellID: "test-outgoing-buff", Frames: 600,
				OutBonus: flatBonus, OutDamageType: "all",
			}}
			mob := mkTestMonster("Target", 10000)
			mob.X, mob.Y = cs.game.camera.X, cs.game.camera.Y
			mob.ArmorClass = 0
			mob.Resistances = nil
			cs.game.world.Monsters = []*monsterPkg.Monster3D{mob}

			result := tt.apply(t, cs, caster, mob)
			if result.got != result.want {
				t.Fatalf("damage = %d, want boosted spell packet plus one flat bonus = %d", result.got, result.want)
			}
		})
	}
}

// The Strong Magic CONTRACT, cell by cell: HP burns only on a CONFIRMED cast
// (a refunded or refused cast costs no blood), the direct nova path deals the
// boosted packet like every other form, and every tooltip quotes the boosted
// numbers combat actually fires.
func TestStrongMagicContractTable(t *testing.T) {
	gmCaster := func(cs *CombatSystem) *character.MMCharacter {
		caster := cs.game.party.Members[0]
		caster.Luck = 0
		caster.HitPoints, caster.MaxHitPoints = 500, 500
		caster.SpellPoints, caster.MaxSpellPoints = 200, 200
		caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: character.MasteryGrandMaster}
		return caster
	}
	castByID := func(t *testing.T, cs *CombatSystem, caster *character.MMCharacter, spellID spells.SpellID) bool {
		t.Helper()
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err != nil {
			t.Fatalf("%s definition: %v", spellID, err)
		}
		return cs.castResolvedSpell(spellID, def, caster, cs.effectiveSpellCost(caster, def.SpellPointsCost), false, false)
	}

	t.Run("confirmed firewall burns HP", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		cs.game.world = newTestWorldSized(cs.game.config, 20, 20)
		ts := float64(cs.game.config.GetTileSize())
		cs.game.camera.X, cs.game.camera.Y = 5.5*ts, 5.5*ts
		caster := gmCaster(cs)
		if !castByID(t, cs, caster, "firewall") {
			t.Fatal("firewall cast failed")
		}
		if len(cs.game.persistentDamageZones) == 0 {
			t.Fatal("firewall laid no cells")
		}
		if caster.HitPoints == 500 {
			t.Fatal("a confirmed offensive cast must burn HP")
		}
	})

	t.Run("fully blocked firewall refunds SP and never burns HP", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		cs.game.world = newTestWorldSized(cs.game.config, 20, 20)
		ts := float64(cs.game.config.GetTileSize())
		// Hard against the west border wall, facing INTO it: every wall cell
		// lands in solid tiles and the zone path refunds the whole cast.
		cs.game.camera.X, cs.game.camera.Y = 1.5*ts, 1.5*ts
		cs.game.camera.Angle = math.Pi // facing -X, the map border
		caster := gmCaster(cs)
		spBefore, hpBefore := caster.SpellPoints, caster.HitPoints
		castByID(t, cs, caster, "firewall")
		if len(cs.game.persistentDamageZones) != 0 {
			t.Fatal("setup: the wall was expected to block every cell")
		}
		if caster.SpellPoints != spBefore {
			t.Fatalf("blocked cast did not refund SP: %d -> %d", spBefore, caster.SpellPoints)
		}
		if caster.HitPoints != hpBefore {
			t.Fatalf("blocked cast burned HP: %d -> %d", hpBefore, caster.HitPoints)
		}
	})

	t.Run("refused cast (not enough SP) never burns HP", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		caster := gmCaster(cs)
		caster.SpellPoints = 0
		hpBefore := caster.HitPoints
		if castByID(t, cs, caster, "rock_blast") {
			t.Fatal("cast with no SP must be refused")
		}
		if caster.HitPoints != hpBefore {
			t.Fatalf("refused cast burned HP: %d -> %d", hpBefore, caster.HitPoints)
		}
	})

	t.Run("nova deals the boosted packet to monsters and the party splash", func(t *testing.T) {
		loadTestConfig(t) // monsters.yaml for the goblin fixture
		measure := func(gm bool) (monsterLoss, memberLoss int) {
			cs := newTestCombatSystemWithConfig(t)
			cs.game.world = newTestWorldSized(cs.game.config, 20, 20)
			ts := float64(cs.game.config.GetTileSize())
			cs.game.camera.X, cs.game.camera.Y = 5.5*ts, 5.5*ts
			caster := gmCaster(cs)
			if !gm {
				delete(caster.Skills, character.SkillStrongMagic)
			}
			mob := monsterPkg.NewMonster3DFromConfig(6.5*ts, 5.5*ts, "goblin", cs.game.config)
			mob.Resistances = nil // exact doubling: no resist rounding between samples
			mob.ArmorClass = 0    // ...and no elemental armor-cap rounding either
			mob.MaxHitPoints, mob.HitPoints = 10000, 10000
			cs.game.world.Monsters = []*monsterPkg.Monster3D{mob}
			member := cs.game.party.Members[1]
			member.HitPoints, member.MaxHitPoints = 400, 400
			if !castByID(t, cs, caster, "inferno") {
				t.Fatal("inferno cast failed")
			}
			return 10000 - mob.HitPoints, 400 - member.HitPoints
		}
		baseMob, baseMember := measure(false)
		gmMob, gmMember := measure(true)
		if baseMob <= 0 || baseMember <= 0 {
			t.Fatalf("baseline nova dealt nothing: mob %d, member %d", baseMob, baseMember)
		}
		if gmMob != baseMob*2 {
			t.Fatalf("GM nova on monsters = %d, want double the baseline %d", gmMob, baseMob)
		}
		if gmMember != baseMember*2 {
			t.Fatalf("GM nova party splash = %d, want double the baseline %d", gmMember, baseMember)
		}
	})

	t.Run("tooltips quote the boosted packet", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		caster := gmCaster(cs)
		cs.game.combatBuffs = []TimedCombatBuff{{
			SpellID: "test-outgoing-buff", Frames: 600,
			OutBonus: 7, OutDamageType: "all",
		}}
		for _, tc := range []struct {
			spellID  spells.SpellID
			needle   string
			expected func(def spells.SpellDefinition) string
		}{
			{spellID: "rock_blast", needle: "Total Damage: ", expected: func(def spells.SpellDefinition) string {
				_, _, total := cs.CalculateSpellDamage(def.ID, caster)
				parts, _ := cs.spellPartsWithOutgoingBuff(cs.spellDamageParts(def.ID, caster, total), def.School)
				return fmt.Sprintf("Total Damage: %d", parts.Total())
			}},
			{spellID: "firewall", needle: "Total per tick: ", expected: func(def spells.SpellDefinition) string {
				tick := cs.CalculatePersistentDamageZoneTickDamage(def, caster)
				parts, _ := cs.spellPartsWithOutgoingBuff(cs.spellDamageParts(def.ID, caster, tick), def.School)
				return fmt.Sprintf("Total per tick: %d", parts.Total())
			}},
			{spellID: "inferno", needle: "Damage: ", expected: func(def spells.SpellDefinition) string {
				parts, _ := cs.spellPartsWithOutgoingBuff(
					cs.spellDamageParts(def.ID, caster, cs.CalculateInfernoDamage(def, caster)), def.School,
				)
				return fmt.Sprintf("Damage: %d", parts.Total())
			}},
		} {
			def, err := spells.GetSpellDefinitionByID(tc.spellID)
			if err != nil {
				t.Fatalf("%s definition: %v", tc.spellID, err)
			}
			card := buildSpellTooltipUnified(def, caster, cs, true)
			if want := tc.expected(def); !strings.Contains(card, want) {
				t.Errorf("%s card does not quote the boosted packet %q:\n%s", tc.spellID, want, card)
			}
			if !strings.Contains(card, "Strong Magic - Grandmaster: +100% damage") {
				t.Errorf("%s card is missing the active Strong Magic line:\n%s", tc.spellID, card)
			}
		}
	})

	t.Run("comparison tooltip compares boosted totals", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		caster := gmCaster(cs)
		cs.game.combatBuffs = []TimedCombatBuff{{
			SpellID: "test-outgoing-buff", Frames: 600,
			OutBonus: 7, OutDamageType: "all",
		}}
		lines := buildSpellComparisonLinesByID("rock_blast", "firebolt", caster, cs)
		_, _, rockTotal := cs.CalculateSpellDamage("rock_blast", caster)
		_, _, boltTotal := cs.CalculateSpellDamage("firebolt", caster)
		rockParts, _ := cs.spellPartsWithOutgoingBuff(cs.spellDamageParts("rock_blast", caster, rockTotal), "earth")
		boltParts, _ := cs.spellPartsWithOutgoingBuff(cs.spellDamageParts("firebolt", caster, boltTotal), "fire")
		rock := rockParts.Total()
		bolt := boltParts.Total()
		want := fmt.Sprintf("Total Damage: %d vs %d (%+d)", rock, bolt, rock-bolt)
		found := false
		for _, ln := range lines {
			if ln == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("comparison lines %q do not contain %q", lines, want)
		}
	})
}
