package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// A ricochet leg must not inherit the parent bolt's nearly-spent lifetime: a
// first hit at max range used to leave ~0 frames for the leap.
func TestRicochetContinuationGetsFreshLifetime(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ts := float64(g.config.GetTileSize())

	first := mkTestMonster("First", 50)
	first.X, first.Y = 10*ts, 10*ts
	second := mkTestMonster("Second", 50)
	second.X, second.Y = 12*ts, 10*ts
	g.world.Monsters = []*monsterPkg.Monster3D{first, second}

	ar := Arrow{
		ID:           "rico_test",
		Active:       true,
		X:            first.X,
		Y:            first.Y,
		VelX:         8,
		VelY:         0,
		Damage:       10,
		LifeTime:     2, // nearly spent at first impact
		BowKey:       "nest_arbalest",
		DamageType:   monsterPkg.DamagePhysical.String(),
		RicochetLeft: 1,
		Owner:        ProjectileOwnerPlayer,
	}
	g.arrows = append(g.arrows, ar)
	cs.applyProjectileDamage(&g.arrows[len(g.arrows)-1], "arrow", first, ar.ID)

	var cont *Arrow
	for i := range g.arrows {
		if a := &g.arrows[i]; a.Active && a.RicochetLeft == 0 && a.SkipMonster == first {
			cont = a
		}
	}
	if cont == nil {
		t.Fatal("no ricochet continuation spawned")
	}
	if cont.LifeTime <= 2 {
		t.Fatalf("continuation inherited spent lifetime %d, want a fresh one", cont.LifeTime)
	}
	if cont.VelX <= 0 || cont.VelY != 0 {
		t.Fatalf("continuation not aimed at the second target: vel (%.1f, %.1f)", cont.VelX, cont.VelY)
	}
}

// Ricochet follows the party auto-target policy: the nearest bound former
// enemy is valid; summons, charmed monsters and the caravan are skipped.
func TestRicochetAutoTargetFollowsAutoTargetPolicy(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ts := float64(cs.game.config.GetTileSize())
	def := &config.WeaponDefinitionConfig{RicochetRangeTiles: 4}
	victim := mkTestMonster("Victim", 50)
	excluded := []*monsterPkg.Monster3D{mkTestMonster("Charmed", 50), mkTestMonster("Caravan", 50)}
	excluded[0].Pacified = true
	excluded[1].Disposition = monsterPkg.DispositionCaravan
	for _, s := range partySummonKinds(cs.game) {
		m := mkTestMonster(s.kind, 50)
		markPurePartySummon(m, s.owner)
		excluded = append(excluded, m)
	}
	for i, m := range excluded {
		m.X = float64(i+1) * ts / 4 // nearer than any valid target
	}
	enemy := mkTestMonster("Enemy", 50)
	enemy.X = 3 * ts
	bound := mkTestMonster("Bound", 50)
	bound.Bound = true
	bound.X = 2 * ts

	cs.game.world.Monsters = append([]*monsterPkg.Monster3D{nil, victim, enemy}, excluded...)
	if got := cs.nearestRicochetTarget(victim, def); got != enemy {
		t.Fatalf("ricochet target = %v, want the enemy past excluded actors", got)
	}
	cs.game.world.Monsters = append(cs.game.world.Monsters, bound)
	if got := cs.nearestRicochetTarget(victim, def); got != bound {
		t.Fatalf("ricochet target = %v, want the nearer bound former enemy", got)
	}
	cs.game.world.Monsters = append([]*monsterPkg.Monster3D{victim}, excluded...)
	if got := cs.nearestRicochetTarget(victim, def); got != nil {
		t.Fatalf("ricochet selected excluded actor %v", got.Name)
	}
}

func TestRicochetRequiresLandedProjectileHit(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*monsterPkg.Monster3D)
	}{
		{
			name: "perfect dodge",
			configure: func(m *monsterPkg.Monster3D) {
				m.PerfectDodge = 100
			},
		},
		{
			name: "sealed boss",
			configure: func(m *monsterPkg.Monster3D) {
				m.BossDormant = true
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			ts := float64(g.config.GetTileSize())
			first := mkTestMonster("First", 50)
			first.X, first.Y = 10*ts, 10*ts
			tc.configure(first)
			second := mkTestMonster("Second", 50)
			second.X, second.Y = 12*ts, 10*ts
			g.world.Monsters = []*monsterPkg.Monster3D{first, second}

			ar := Arrow{
				ID: "rico_rejected", Active: true, X: first.X, Y: first.Y,
				VelX: 8, Damage: 10, LifeTime: 30, BowKey: "nest_arbalest",
				DamageType:   monsterPkg.DamagePhysical.String(),
				RicochetLeft: 1, Owner: ProjectileOwnerPlayer,
			}
			g.arrows = append(g.arrows, ar)
			cs.applyProjectileDamage(&g.arrows[0], "arrow", first, ar.ID)

			for i := range g.arrows {
				if cont := &g.arrows[i]; cont.Active && cont.SkipMonster == first {
					t.Fatalf("rejected primary hit spawned ricochet %+v", *cont)
				}
			}
		})
	}
}

func TestWeaponKillResolverRunsClutchburstForEveryWeaponKillPath(t *testing.T) {
	emberEgg := func(t *testing.T) *config.WeaponDefinitionConfig {
		t.Helper()
		def, ok := config.GetWeaponDefinition("ember_egg_mace")
		if !ok || def == nil || def.DeathBurstDamage <= 0 {
			t.Fatal("Ember Egg config missing or authors no death burst")
		}
		return def
	}
	t.Run("mastery true damage through dodge", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		g := cs.game
		ts := float64(g.config.GetTileSize())
		weaponDef := emberEgg(t)
		attacker := g.party.Members[g.selectedChar]
		attacker.Skills[character.SkillMace] = &character.Skill{Mastery: character.MasteryExpert}

		primary := mkTestMonster("Dodger", MasteryWeaponTrueDamagePerTier)
		primary.X, primary.Y = 10*ts, 10*ts
		primary.PerfectDodge = 100
		bystander := mkTestMonster("Bystander", 100)
		bystander.X, bystander.Y = 11*ts, 10*ts
		g.world.Monsters = []*monsterPkg.Monster3D{primary, bystander}

		cs.ApplyDamageToMonster(primary, 100, weaponDef.Name, false)

		if primary.IsAlive() {
			t.Fatal("mastery true damage did not kill the dodging target")
		}
		if got, want := bystander.HitPoints, 100-weaponDef.DeathBurstDamage; got != want {
			t.Fatalf("Clutchburst left bystander at %d HP, want %d", got, want)
		}
	})

	t.Run("secondary weapon splash kill", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		g := cs.game
		ts := float64(g.config.GetTileSize())
		weaponDef := emberEgg(t)
		center := mkTestMonster("Center", 1000)
		center.X, center.Y = 10*ts, 10*ts
		secondary := mkTestMonster("Secondary", 10)
		secondary.X, secondary.Y = 11*ts, 10*ts
		witness := mkTestMonster("Witness", 100)
		witness.X, witness.Y = 12.75*ts, 10*ts
		g.world.Monsters = []*monsterPkg.Monster3D{center, secondary, witness}
		attack := cs.newPartyMonsterAttack(
			20, 0, monsterPkg.DamageFire.String(), 0,
			weaponDef, weaponDef.Name, false, false, true,
		)

		cs.applyAoeSplash(center, attack, weaponDef.AoeRadiusTiles)

		if secondary.IsAlive() {
			t.Fatal("weapon splash did not kill secondary target")
		}
		if got, want := witness.HitPoints, 100-weaponDef.DeathBurstDamage; got != want {
			t.Fatalf("secondary Clutchburst left witness at %d HP, want %d", got, want)
		}
	})
}

// Weaken scales the WHOLE outgoing packet (normal and the attacker's true
// damage) exactly once on every monster damage channel. A fired projectile
// keeps the packet it launched with after Weaken expires. Party-hit channels
// read the HP an isolated member lost; the trap field carries no true damage.
func TestWeakenScalesWholeOutgoingPacketOnce(t *testing.T) {
	type channel struct {
		name            string
		pct             int
		normal, trueDmg int
		carriesTrue     bool
		partyHP         bool // only the total is observable
		snapshot        bool // the packet is fixed when fired
		// fire delivers one attack and returns a reader of its packet.
		fire func(t *testing.T, cs *CombatSystem, m *monsterPkg.Monster3D, normal int) func() damagecalc.Parts
	}
	partyHit := func(apply func(cs *CombatSystem, m *monsterPkg.Monster3D)) func(*testing.T, *CombatSystem, *monsterPkg.Monster3D, int) func() damagecalc.Parts {
		return func(_ *testing.T, cs *CombatSystem, m *monsterPkg.Monster3D, _ int) func() damagecalc.Parts {
			member := cs.game.party.Members[0]
			member.HitPoints = member.MaxHitPoints
			apply(cs, m)
			dealt := member.MaxHitPoints - member.HitPoints
			return func() damagecalc.Parts { return damagecalc.Parts{Normal: dealt} }
		}
	}
	for _, tc := range []channel{
		{name: "melee_packet", pct: 25, normal: 100, trueDmg: 40, carriesTrue: true,
			fire: func(t *testing.T, cs *CombatSystem, m *monsterPkg.Monster3D, normal int) func() damagecalc.Parts {
				m.DamageMin, m.DamageMax = normal, normal
				roll := cs.monsterAttackDamage(m)
				if roll != normal {
					t.Fatalf("monsterAttackDamage applied weaken (%d); hitFromMonster owns it", roll)
				}
				hit := hitFromMonster(m, roll, monsterPkg.DamagePhysical.String(), false, 0, true, false)
				return func() damagecalc.Parts { return hit.Parts }
			}},
		{name: "packet_transform", pct: 25, normal: 80, trueDmg: 20, carriesTrue: true,
			fire: func(_ *testing.T, _ *CombatSystem, m *monsterPkg.Monster3D, normal int) func() damagecalc.Parts {
				parts := m.OutgoingDamage(damagecalc.Parts{Normal: normal, True: m.TrueDamage})
				return func() damagecalc.Parts { return parts }
			}},
		{name: "projectile", pct: 25, normal: 80, trueDmg: 20, carriesTrue: true, snapshot: true,
			fire: func(t *testing.T, cs *CombatSystem, m *monsterPkg.Monster3D, normal int) func() damagecalc.Parts {
				m.DamageMin, m.DamageMax = normal, normal
				cs.spawnMonsterSpellProjectile(m, "firebolt", cs.game.camera.X, cs.game.camera.Y, ProjectileOwnerMonster)
				if len(cs.game.magicProjectiles) != 1 {
					t.Fatalf("spawned projectiles = %d, want 1", len(cs.game.magicProjectiles))
				}
				return func() damagecalc.Parts {
					p := cs.game.magicProjectiles[0]
					return damagecalc.Parts{Normal: p.Damage, True: p.TrueDamage}
				}
			}},
		{name: "inferno", pct: 50, normal: 40, trueDmg: 20, carriesTrue: true, partyHP: true,
			fire: partyHit(func(cs *CombatSystem, m *monsterPkg.Monster3D) {
				m.InfernoDamage = 40
				cs.applyMonsterInferno(m)
			})},
		{name: "fireburst", pct: 50, normal: 40, trueDmg: 20, carriesTrue: true, partyHP: true,
			fire: partyHit(func(cs *CombatSystem, m *monsterPkg.Monster3D) {
				m.FireburstDamageMin, m.FireburstDamageMax = 40, 40
				cs.applyMonsterFireburst(m)
			})},
		{name: "trap_field", pct: 50, normal: 40, trueDmg: 20, partyHP: true,
			fire: partyHit(func(cs *CombatSystem, m *monsterPkg.Monster3D) {
				m.TrapVolleyDamage = 40
				cs.game.detonateBossFireTrap(m)
			})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.party.Members = g.party.Members[:1]
			isolateTrueDamageMember(g.party.Members[0], 0)
			g.party.Members[0].MaxHitPoints = 1000
			m := mkTestMonster("Weakened", 100)
			m.TrueDamage = tc.trueDmg
			m.ApplyWeaken(tc.pct, 600, 5)

			read := tc.fire(t, cs, m, tc.normal)
			want := damagecalc.Parts{Normal: tc.normal * (100 - tc.pct) / 100}
			if tc.carriesTrue {
				want.True = tc.trueDmg * (100 - tc.pct) / 100
			}
			got := read()
			if (tc.partyHP && got.Total() != want.Total()) || (!tc.partyHP && got != want) {
				t.Fatalf("weakened packet = %+v, want %+v (once, normal and true)", got, want)
			}
			if tc.snapshot {
				m.WeakenPct, m.WeakenFramesRemaining, m.WeakenTurnsRemaining = 0, 0, 0
				if after := read(); after != got {
					t.Fatalf("in-flight packet changed after Weaken expired: %+v -> %+v", got, after)
				}
			}
		})
	}
}

func TestMonsterPoisonImmunityLivesAtStatusEntryPoint(t *testing.T) {
	undead := mkTestMonster("Undead", 100)
	undead.MonsterType = "undead"
	if undead.ApplyPoison(120) || undead.PoisonedFramesRemaining != 0 {
		t.Fatal("undead accepted poison through Monster3D.ApplyPoison")
	}

	living := mkTestMonster("Living", 100)
	if !living.ApplyPoison(120) || living.PoisonedFramesRemaining != 120 {
		t.Fatal("living monster rejected poison")
	}
}

func TestWeaponSpellEchoRepeatsOffensiveCastForFree(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := cs.game.party.Members[0]
	weapon, err := items.TryCreateWeaponFromYAML("verdant_eye_scepter")
	if err != nil {
		t.Fatalf("create Verdant Eye: %v", err)
	}
	caster.Equipment[items.SlotMainHand] = weapon

	weaponDef, ok := config.GetWeaponDefinition("verdant_eye_scepter")
	if !ok {
		t.Fatal("Verdant Eye definition missing")
	}
	oldEchoPct := weaponDef.SpellEchoPct
	weaponDef.SpellEchoPct = 100
	t.Cleanup(func() {
		weaponDef.SpellEchoPct = oldEchoPct
	})

	spellDef, err := spells.GetSpellDefinitionByID("firebolt")
	if err != nil {
		t.Fatalf("load firebolt: %v", err)
	}
	caster.SpellPoints = spellDef.SpellPointsCost + 10
	spBefore := caster.SpellPoints
	if !cs.castResolvedSpell("firebolt", spellDef, caster, spellDef.SpellPointsCost, false, false) {
		t.Fatal("firebolt cast failed")
	}
	if got := len(cs.game.magicProjectiles); got != 2 {
		t.Fatalf("echo spawned %d projectiles, want 2", got)
	}
	if spent := spBefore - caster.SpellPoints; spent != spellDef.SpellPointsCost {
		t.Fatalf("echo spent %d SP, want one cast cost %d", spent, spellDef.SpellPointsCost)
	}
}

func TestWeaponDamagePreviewIncludesAuthoredTrueDamageWithoutBearer(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	weapon, err := items.TryCreateWeaponFromYAML("broodspike")
	if err != nil {
		t.Fatalf("create Broodspike: %v", err)
	}
	def, ok := config.GetWeaponDefinition("broodspike")
	if !ok {
		t.Fatal("Broodspike definition missing")
	}

	preview := cs.calculateWeaponDamagePreview(weapon, nil)
	if preview.AuthoredTrue != def.TrueDamage || preview.True != def.TrueDamage {
		t.Fatalf("authored true preview = %d/%d, want %d",
			preview.AuthoredTrue, preview.True, def.TrueDamage)
	}
	if preview.Total != preview.Normal+def.TrueDamage {
		t.Fatalf("shop total = %d, want normal %d + true %d",
			preview.Total, preview.Normal, def.TrueDamage)
	}

	wantLine := fmt.Sprintf("Weapon: +%d True", def.TrueDamage)
	tooltip := GetItemTooltip(weapon, nil, cs, true)
	if !strings.Contains(tooltip, wantLine) {
		t.Fatalf("shop tooltip omits authored true damage:\n%s", tooltip)
	}
	editor := GetItemTooltip(items.CreateWeaponFromYAML(items.GetWeaponKeyByName(def.Name)), nil, nil, true)
	if !strings.Contains(editor, wantLine) {
		t.Fatalf("editor card omits authored true damage:\n%s", editor)
	}
}

// Starting exterminate quests must anchor to the live world: an already-empty
// map completes immediately (the boss can spawn), a populated one snapshots
// its census as the journal target.
func TestReconcileExterminationQuests(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	cfg := &quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"purge_test": {
			Name:            "Purge Test",
			Type:            quests.QuestTypeKill,
			TargetMonster:   "target",
			TargetCount:     3,
			Exterminate:     true,
			IsStartingQuest: true,
		},
	}}
	g.questManager = quests.NewQuestManager(cfg)
	g.questManager.InitializeStartingQuests()

	// Two living targets: census becomes the dynamic goal, quest stays open.
	a, b := mkTestMonster("Target", 10), mkTestMonster("Target", 10)
	g.world.Monsters = []*monsterPkg.Monster3D{a, b}
	g.reconcileKillQuests()
	q := g.questManager.GetQuest("purge_test")
	if q.Completed || q.Target() != 2 {
		t.Fatalf("populated reconcile: completed=%v target=%d, want open with target 2", q.Completed, q.Target())
	}

	// Targets all dead (an old save's state): reconcile completes on the spot.
	a.HitPoints, b.HitPoints = 0, 0
	g.reconcileKillQuests()
	if !g.questManager.GetQuest("purge_test").Completed {
		t.Fatal("empty-map reconcile did not complete the quest")
	}
}

// The shared formatters must surface the new mechanics (tooltip SSoT contract).
func TestNewMechanicsAppearInSharedFormatters(t *testing.T) {
	item := &config.ItemDefinitionConfig{
		ProjectileReflectPct: 20,
		StatusDurationPct:    -50,
		ScaleStackAC:         1,
		ScaleStackMax:        8,
	}
	joined := strings.Join(item.CoreEffectLines(), "\n")
	for _, want := range []string{"Mirror scales: 20%", "Hostile statuses on the wearer last 50%", "Growing scales: +1 AC per hit taken (max +8)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("item CoreEffectLines missing %q in:\n%s", want, joined)
		}
	}
	potion := &config.ItemDefinitionConfig{
		ResistBuffSchool: "fire", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60,
	}
	if joined := strings.Join(potion.CoreEffectLines(), "\n"); !strings.Contains(joined, "Fire resistance +50% for 60s") {
		t.Fatalf("draught CoreEffectLines missing ward line in:\n%s", joined)
	}
	stone := &config.ItemDefinitionConfig{BuffArmorClass: 15, BuffDurationSeconds: 60}
	if joined := strings.Join(stone.CoreEffectLines(), "\n"); !strings.Contains(joined, "stoneskin: armor class +15 for 60s") {
		t.Fatalf("stoneskin CoreEffectLines missing line in:\n%s", joined)
	}

	mon := monsterPkg.MonsterDefinition{
		TrapVolleyCount:           20,
		TrapVolleyRadiusTiles:     15,
		TrapVolleyIntervalSeconds: 10,
		TrapVolleyIntervalTurns:   3,
		TrapVolleyDamage:          40,
	}
	var lines []string
	for _, l := range mon.CombatEffectLines() {
		lines = append(lines, l.Text)
	}
	joined = strings.Join(lines, "\n")
	if !strings.Contains(joined, "Melee: Physical") {
		t.Fatalf("monster lines missing melee school in:\n%s", joined)
	}
	if !strings.Contains(joined, "Trap field: sows 20 fire traps") {
		t.Fatalf("monster lines missing trap field in:\n%s", joined)
	}
}

// Every catalog entry that spawns a monster by key resolves at boot: boss
// summons, summon spells and summon cards. A typo would otherwise panic in the
// monster constructor on the first summon, mid-fight.
func TestShippedMonsterCatalogReferencesResolve(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// The shipped catalog is a GLOBAL; under -shuffle no earlier test is
	// guaranteed to have loaded it.
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	shipped := monsterPkg.MonsterConfig
	if err := monsterPkg.ValidateCatalogReferences(shipped, cs.game.config); err != nil {
		t.Fatalf("shipped monster catalog reference: %v", err)
	}
	for _, tc := range []struct {
		name, want string
		breakRef   func(t *testing.T) *monsterPkg.MonsterYAMLConfig
	}{
		{"boss summon", "summon_monsters", func(t *testing.T) *monsterPkg.MonsterYAMLConfig {
			bad := &monsterPkg.MonsterYAMLConfig{Monsters: map[string]monsterPkg.MonsterDefinition{}}
			for k, v := range shipped.Monsters {
				bad.Monsters[k] = v
			}
			bad.Monsters["summoner"] = monsterPkg.MonsterDefinition{SummonMonsters: []string{"missing"}}
			return bad
		}},
		{"summon spell", "summon_monster", func(t *testing.T) *monsterPkg.MonsterYAMLConfig {
			spell := config.GlobalSpells.Spells["summon_ice_elemental"]
			prev := spell.SummonMonster
			spell.SummonMonster = "missing"
			t.Cleanup(func() { spell.SummonMonster = prev })
			return shipped
		}},
		{"summon card", "card_summon_monster", func(t *testing.T) *monsterPkg.MonsterYAMLConfig {
			card := config.GlobalItems.Items["orc_warlord_card"]
			prev := card.CardSummonMonster
			card.CardSummonMonster = "missing"
			t.Cleanup(func() { card.CardSummonMonster = prev })
			return shipped
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := monsterPkg.ValidateCatalogReferences(tc.breakRef(t), cs.game.config)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("unknown %s key passed catalog validation: %v", tc.want, err)
			}
		})
	}
}

// suppressorFloor is the Suppressor's authored personal TB action floor.
func suppressorFloor(t *testing.T) int {
	t.Helper()
	def, ok := config.GetWeaponDefinition("suppressor_gun")
	if !ok || def == nil || def.TBActionsPerRound <= 1 {
		t.Fatal("Suppressor config missing or authors no TB action floor")
	}
	return def.TBActionsPerRound
}

// The Suppressor's drum autofire: its wielder starts a TB round at the weapon's
// authored action floor (personal, like dual-wield's 2 - Speed bonuses still
// stack on top).
func TestSuppressorGrantsAuthoredTBActions(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	holder := g.party.Members[0]
	w, err := items.TryCreateWeaponFromYAML("suppressor_gun")
	if err != nil {
		t.Fatalf("create Suppressor: %v", err)
	}
	holder.Equipment[items.SlotMainHand] = w
	floor := suppressorFloor(t)
	g.startPartyTurn()
	if holder.ActionsRemaining < floor {
		t.Fatalf("Suppressor wielder has %d TB actions, want >= %d", holder.ActionsRemaining, floor)
	}
	for _, other := range g.party.Members[1:] {
		if other.ActionsRemaining >= floor {
			t.Fatalf("non-wielder %s got %d actions - the floor is personal", other.Name, other.ActionsRemaining)
		}
	}
}

// The TB turn that consumes a debuff's FINAL tick must still suffer it (the
// Root pattern): N authored turns mean N weakened turns, not N-1.
func TestSlowAndWeakenHoldThroughFinalTBTurn(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	m := mkTestMonster("Silted", 100)
	m.ApplySlow(30, 0, 1)   // exactly one TB turn left
	m.ApplyWeaken(25, 0, 1) // exactly one TB turn left

	m.TickSlowTurn() // the tick that consumes the last turn...
	m.TickWeakenTurn()
	if got := m.ActiveSlowPct(); got != 30 {
		t.Fatalf("final-turn slow pct = %d, want 30 (latched)", got)
	}
	if got := m.OutgoingDamage(damagecalc.Parts{Normal: 100}).Normal; got != 75 {
		t.Fatalf("final-turn weakened damage = %d, want 75 (latched)", got)
	}

	m.TickSlowTurn() // next turn: expired for real
	m.TickWeakenTurn()
	if got := m.ActiveSlowPct(); got != 0 {
		t.Fatalf("expired slow pct = %d, want 0", got)
	}
	if got := m.OutgoingDamage(damagecalc.Parts{Normal: 100}).Normal; got != 100 {
		t.Fatalf("expired weakened damage = %d, want 100", got)
	}
}

func TestHostileStatusDurationFloorAppliesAfterCombiningSources(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	set := config.GetItemSet("padded")
	itemDef, ok := config.GetItemDefinition("deathgod_aegis")
	if set == nil || !ok {
		t.Fatal("status-duration test definitions missing")
	}
	oldSetPct, oldItemPct := set.StunDurationPct, itemDef.StatusDurationPct
	// -120 + 20 = -100 floors to the minimum; clamping the set first would
	// give -90 + 20 = -70.
	set.StunDurationPct = -120
	itemDef.StatusDurationPct = 20
	t.Cleanup(func() {
		set.StunDurationPct = oldSetPct
		itemDef.StatusDurationPct = oldItemPct
	})

	member.Equipment[items.SlotHelmet] = items.CreateItemFromYAML("padded_cap")
	member.Equipment[items.SlotArmor] = items.CreateItemFromYAML("padded_vest")
	member.Equipment[items.SlotOffHand] = items.CreateItemFromYAML("padded_greaves")
	member.Equipment[items.SlotGauntlets] = items.CreateItemFromYAML("padded_gloves")
	member.Equipment[items.SlotRing1] = items.CreateItemFromYAML("deathgod_aegis")

	if got := member.SetStunDurationPct(); got != -120 {
		t.Fatalf("set contribution was clamped before combination: %d", got)
	}
	cs.applyScaledCharStun(member, 100, 100)
	want := 100 + config.MinHostileStatusDurationPct
	if member.StunFramesRemaining != want || member.StunTurnsRemaining != want {
		t.Fatalf("combined -100%% duration = %d frames/%d turns, want the %d%% floor %d/%d",
			member.StunFramesRemaining, member.StunTurnsRemaining, config.MinHostileStatusDurationPct, want, want)
	}
}

// A queued (deferred) quest spawn must land BEFORE any save snapshot: the
// done-key is written at queue time, so a save without the monster would lose
// the boss forever on load.
func TestBuildSaveFlushesPendingQuestSpawns(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	m := monsterPkg.NewMonster3DFromConfig(64, 64, "alien_enforcer", g.config)
	g.pendingQuestSpawns = append(g.pendingQuestSpawns, pendingQuestSpawn{world: g.world, monster: m})

	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps = map[string]*world.World3D{"test": g.world}
	wm.CurrentMapKey = "test"
	setTestWorldManager(t, wm)
	save := g.buildSave(wm)
	if len(g.pendingQuestSpawns) != 0 {
		t.Fatal("buildSave left the pending quest spawn queued")
	}
	found := false
	for _, wm := range g.world.Monsters {
		if wm == m {
			found = true
		}
	}
	if !found {
		t.Fatal("pending spawn not registered into the world before the snapshot")
	}
	_ = save
}

func TestQuestCompletionSpawnUsesStableAuthoredID(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"spawn_test": {
			Name:            "Spawn Test",
			Type:            quests.QuestTypeKill,
			TargetMonster:   "rat",
			TargetCount:     1,
			IsStartingQuest: true,
			OnCompleteSpawns: []quests.QuestSpawn{{
				ID: "stable_rat", Map: "test", Monster: "rat",
			}},
		},
	}})
	g.questManager.InitializeStartingQuests()
	g.questManager.MarkCompleted("spawn_test")

	g.spawnQuestCompletionMonsters(false)

	if !g.questSpawnsDone["spawn_test#stable_rat"] {
		t.Fatalf("spawn completion keys = %v, want authored stable ID", g.questSpawnsDone)
	}
	if len(g.pendingQuestSpawns) != 1 {
		t.Fatalf("queued completion spawns = %d, want 1", len(g.pendingQuestSpawns))
	}
}

func TestSuppressorActionFloorCannotTransferAcrossGearSwap(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	g.currentTurn = 0
	holder := g.party.Members[0]
	suppressor, err := items.TryCreateWeaponFromYAML("suppressor_gun")
	if err != nil {
		t.Fatalf("create Suppressor: %v", err)
	}
	longlance, err := items.TryCreateWeaponFromYAML("longlance_rifle")
	if err != nil {
		t.Fatalf("create Longlance: %v", err)
	}
	holder.Equipment[items.SlotMainHand] = suppressor
	for _, member := range g.party.Members {
		member.Speed = -1000 // isolate the weapon floor from Speed bonus actions
	}
	floor := suppressorFloor(t)
	g.startPartyTurn()
	if holder.ActionsRemaining != floor || holder.TBRoundActionFloor != floor {
		t.Fatalf("initial Suppressor state = %d actions/floor %d, want %d/%d",
			holder.ActionsRemaining, holder.TBRoundActionFloor, floor, floor)
	}

	g.party.Inventory = append(g.party.Inventory, longlance)
	if !g.equipPartyItemFromInventoryToSlot(0, 0, items.SlotMainHand) {
		t.Fatal("could not replace Suppressor with Longlance")
	}
	if holder.ActionsRemaining != 1 || holder.TBRoundActionFloor != 1 {
		t.Fatalf("post-swap state = %d actions/floor %d, want 1/1",
			holder.ActionsRemaining, holder.TBRoundActionFloor)
	}

	suppressorIndex := -1
	for i := range g.party.Inventory {
		if g.party.Inventory[i].Name == suppressor.Name {
			suppressorIndex = i
			break
		}
	}
	if suppressorIndex < 0 {
		t.Fatal("displaced Suppressor did not return to inventory")
	}
	if !g.equipPartyItemFromInventoryToSlot(suppressorIndex, 0, items.SlotMainHand) {
		t.Fatal("could not re-equip Suppressor")
	}
	if holder.ActionsRemaining != 1 || holder.TBRoundActionFloor != 1 {
		t.Fatalf("re-equip refilled round to %d actions/floor %d, want unchanged 1/1",
			holder.ActionsRemaining, holder.TBRoundActionFloor)
	}

	restored := restoreCharacterSave(buildCharacterSave(holder))
	if restored.ActionsRemaining != 1 || restored.TBRoundActionFloor != 1 {
		t.Fatalf("save/load restored %d actions/floor %d, want 1/1",
			restored.ActionsRemaining, restored.TBRoundActionFloor)
	}
}

// A map switch between the queue and the flush must not teleport the deferred
// boss to the destination map: each entry lands on ITS OWN pinned world.
func TestPendingQuestSpawnLandsOnItsOwnWorld(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	home := g.world
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	m := monsterPkg.NewMonster3DFromConfig(64, 64, "dragon_brood_mother", g.config)
	g.pendingQuestSpawns = append(g.pendingQuestSpawns, pendingQuestSpawn{world: home, monster: m})

	g.world = newTestWorldSized(g.config, 4, 4) // the party left before the flush
	g.flushPendingQuestSpawns()

	for _, wm := range g.world.Monsters {
		if wm == m {
			t.Fatal("deferred spawn landed on the DESTINATION map")
		}
	}
	found := false
	for _, wm := range home.Monsters {
		if wm == m {
			found = true
		}
	}
	if !found {
		t.Fatal("deferred spawn missing from its authored home world")
	}
	if len(g.pendingQuestSpawns) != 0 {
		t.Fatal("queue not drained")
	}
}

func TestApplySaveDropsDeferredSpawnFromPreviousTimeline(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"test": g.world}
	wm.CurrentMapKey = "test"
	setTestWorldManager(t, wm)

	save := g.buildSave(wm)
	queued := mkTestMonster("Old Timeline Boss", 100)
	queued.ID = "old_timeline_boss"
	g.pendingQuestSpawns = append(g.pendingQuestSpawns, pendingQuestSpawn{world: g.world, monster: queued})

	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("applySave: %v", err)
	}
	g.flushPendingQuestSpawns()
	for _, m := range g.world.Monsters {
		if m != nil && m.ID == queued.ID {
			t.Fatal("deferred spawn from the replaced timeline survived load")
		}
	}
}

// reloadIntoFreshGame saves `saved` as mapKey and loads it into a new game on
// loadWorld, so nothing survives in memory between the two. prep runs on the
// new game before the load.
func reloadIntoFreshGame(t *testing.T, saved *MMGame, mapKey string, loadWorld *world.World3D, prep func(*MMGame)) *MMGame {
	t.Helper()
	wmSave := world.NewWorldManager(saved.config)
	wmSave.LoadedMaps = map[string]*world.World3D{mapKey: saved.world}
	wmSave.CurrentMapKey = mapKey
	save := saved.buildSave(wmSave)

	wmLoad := world.NewWorldManager(saved.config)
	wmLoad.LoadedMaps = map[string]*world.World3D{mapKey: loadWorld}
	wmLoad.CurrentMapKey = mapKey
	setTestWorldManager(t, wmLoad)
	loaded := newTestGame(saved.config, loadWorld)
	if prep != nil {
		prep(loaded)
	}
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("applySave: %v", err)
	}
	return loaded
}

func TestSaveLoadPreservesFinalTurnSlowAndWeakenLatches(t *testing.T) {
	cfg := loadTestConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	saveWorld := newTestWorld(cfg)
	m := monsterPkg.NewMonster3DFromConfig(96, 96, "goblin", cfg)
	m.ID = "latched_debuff_monster"
	m.ApplySlow(30, 0, 1)
	m.ApplyWeaken(25, 0, 1)
	m.TickSlowTurn()
	m.TickWeakenTurn()
	saveWorld.Monsters = []*monsterPkg.Monster3D{m}

	saveGame := newTestGame(cfg, saveWorld)
	saveGame.turnBasedMode = true
	saveGame.turnBasedMonsterPassesLeft = 1
	saveGame.turnBasedMonsterPassDelay = 3
	loaded := reloadIntoFreshGame(t, saveGame, "forest", newTestWorld(cfg), nil)

	restored := loaded.monsterByID(m.ID)
	if restored == nil {
		t.Fatal("saved monster was not restored")
	}
	if got := restored.ActiveSlowPct(); got != 30 {
		t.Fatalf("restored final-turn slow = %d, want 30", got)
	}
	if got := restored.OutgoingDamage(damagecalc.Parts{Normal: 100}).Normal; got != 75 {
		t.Fatalf("restored final-turn weaken damage = %d, want 75", got)
	}
}

func TestSaveLoadPreservesPartyAndMonsterDoTTickPhases(t *testing.T) {
	cfg := loadTestConfig(t)
	tps := cfg.GetTPS()
	const phase = 17

	saveWorld := newTestWorld(cfg)
	mob := monsterPkg.NewMonster3DFromConfig(96, 96, "goblin", cfg)
	mob.ID = "dot_phase_monster"
	mob.ApplyPoison(3 * tps)
	mob.ApplyBurn(3 * tps)
	mob.TickPoisonTurn(phase)
	mob.TickBurnTurn(phase)
	saveWorld.Monsters = []*monsterPkg.Monster3D{mob}

	saveGame := newTestGame(cfg, saveWorld)
	member := saveGame.party.Members[0]
	member.ApplyPoison(3 * tps)
	member.ApplyBurn(3 * tps)
	member.TickPoisonTurn(phase, tps)
	member.TickBurnTurn(phase, tps)
	wantPartyPoison, wantPartyBurn := member.DoTTickTimers()
	wantMonsterPoison, wantMonsterBurn := mob.DoTTickTimers()
	loaded := reloadIntoFreshGame(t, saveGame, "forest", newTestWorld(cfg), nil)

	gotPartyPoison, gotPartyBurn := loaded.party.Members[0].DoTTickTimers()
	if gotPartyPoison != wantPartyPoison || gotPartyBurn != wantPartyBurn {
		t.Fatalf("party DoT phases = %d/%d, want %d/%d",
			gotPartyPoison, gotPartyBurn, wantPartyPoison, wantPartyBurn)
	}
	restored := loaded.monsterByID(mob.ID)
	if restored == nil {
		t.Fatal("saved DoT monster was not restored")
	}
	gotMonsterPoison, gotMonsterBurn := restored.DoTTickTimers()
	if gotMonsterPoison != wantMonsterPoison || gotMonsterBurn != wantMonsterBurn {
		t.Fatalf("monster DoT phases = %d/%d, want %d/%d",
			gotMonsterPoison, gotMonsterBurn, wantMonsterPoison, wantMonsterBurn)
	}
}

func TestBossTrapCooldownCarriesAcrossCombatModes(t *testing.T) {
	setTestWorldManager(t, nil)
	g, _, ts := tbBehaviorGame(t, 50, 50)
	g.turnBasedMode = false
	boss := monsterPkg.NewMonster3DFromConfig(25.5*ts, 25.5*ts, "dragon_brood_mother", g.config)
	boss.ID = "brood_mother"
	boss.IsEngagingPlayer = true
	g.world.Monsters = []*monsterPkg.Monster3D{boss}

	g.combat.tryBossTrapVolley(boss, false)
	if len(g.bossFireTraps) == 0 {
		t.Fatal("first trap volley did not sow a field")
	}
	if boss.TrapVolleyCDFrames <= 0 || boss.TrapVolleyTurnCD != boss.TrapVolleyIntervalTurns ||
		boss.TrapVolleyCDRate <= 0 {
		t.Fatalf("initial cooldown = %d frames/%d turns at rate %d",
			boss.TrapVolleyCDFrames, boss.TrapVolleyTurnCD, boss.TrapVolleyCDRate)
	}

	for i := 0; i < boss.TrapVolleyCDRate; i++ {
		g.combat.tryBossTrapVolley(boss, false)
	}
	if boss.TrapVolleyTurnCD != boss.TrapVolleyIntervalTurns-1 {
		t.Fatalf("RT elapsed time left %d TB turns, want %d",
			boss.TrapVolleyTurnCD, boss.TrapVolleyIntervalTurns-1)
	}

	g.combat.tryBossTrapVolley(boss, true)
	if boss.TrapVolleyTurnCD != 1 {
		t.Fatalf("first TB pass after switch left %d turns, want 1", boss.TrapVolleyTurnCD)
	}
	g.combat.tryBossTrapVolley(boss, true)
	if boss.TrapVolleyTurnCD != boss.TrapVolleyIntervalTurns {
		t.Fatalf("due TB pass did not re-arm cooldown: got %d, want %d",
			boss.TrapVolleyTurnCD, boss.TrapVolleyIntervalTurns)
	}
}

func TestSaveLoadRestoresBossTrapStateWithoutQuestManager(t *testing.T) {
	cfg := loadTestConfig(t)
	saveWorld := newTestWorldSized(cfg, 40, 40)
	ts := float64(cfg.GetTileSize())
	boss := monsterPkg.NewMonster3DFromConfig(20*ts, 20*ts, "dragon_brood_mother", cfg)
	boss.ID = "saved_brood_mother"
	boss.ArmTrapVolleyCooldown(cfg.GetTPS())
	saveWorld.Monsters = []*monsterPkg.Monster3D{boss}

	saveGame := newTestGame(cfg, saveWorld)
	saveGame.combat = NewCombatSystem(saveGame)
	saveGame.questManager = nil
	saveGame.bossFireTraps = []bossFireTrap{{TX: 19, TY: 20}}
	saveGame.bossFireTrapsOwner = boss.ID
	loaded := reloadIntoFreshGame(t, saveGame, "dragon_cliffs", newTestWorldSized(cfg, 40, 40), func(g *MMGame) {
		g.combat = NewCombatSystem(g)
		g.questManager = nil
	})

	if len(loaded.bossFireTraps) != 1 || loaded.bossFireTrapsOwner != boss.ID {
		t.Fatalf("restored trap field = %+v owner %q", loaded.bossFireTraps, loaded.bossFireTrapsOwner)
	}
	restored := loaded.monsterByID(boss.ID)
	if restored == nil {
		t.Fatal("saved Brood Mother was not restored")
	}
	if restored.TrapVolleyCDRate != boss.TrapVolleyCDRate {
		t.Fatalf("restored trap cooldown rate = %d, want %d",
			restored.TrapVolleyCDRate, boss.TrapVolleyCDRate)
	}
}
