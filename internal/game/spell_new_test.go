package game

// Tests for the spells added to bring every magic school to >=4: the code-backed
// ones (Raise Dead, Inferno, buff stacking). Pure-YAML damage spells
// are exercised by the generic projectile path already.

import (
	"testing"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Raise Dead restores a fallen (non-eradicated) ally to its authored
// revive_hp_pct of max HP and clears Dead/Unconscious.
func TestRaiseDead_RevivesToAuthoredPct(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "raise_dead", 100, 30)
	def, err := spells.GetSpellDefinitionByID("raise_dead")
	if err != nil {
		t.Fatalf("raise_dead def: %v", err)
	}
	if def.ReviveHpPct <= 0 || def.ReviveHpPct >= 100 {
		t.Fatalf("raise_dead must author a partial revive_hp_pct, got %d", def.ReviveHpPct)
	}

	fallen := game.party.Members[1]
	fallen.MaxHitPoints = 40
	fallen.HitPoints = 0
	fallen.AddCondition(character.ConditionDead)

	if !game.combat.CastEquippedSpell() {
		t.Fatalf("raise_dead cast failed")
	}
	if fallen.HasCondition(character.ConditionDead) || fallen.HasCondition(character.ConditionUnconscious) {
		t.Errorf("raise_dead should clear Dead/Unconscious")
	}
	if want := max(1, fallen.MaxHitPoints*def.ReviveHpPct/100); fallen.HitPoints != want {
		t.Errorf("raise_dead should restore %d%% of max (%d), got %d", def.ReviveHpPct, want, fallen.HitPoints)
	}
}

// Raise Dead must NOT revive an eradicated ally (that's Resurrect's domain).
func TestRaiseDead_SkipsEradicated(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "raise_dead", 100, 30)

	gone := game.party.Members[1]
	gone.HitPoints = 0
	gone.AddCondition(character.ConditionEradicated)

	game.combat.CastEquippedSpell()
	if !gone.HasCondition(character.ConditionEradicated) || gone.HitPoints != 0 {
		t.Errorf("raise_dead must not touch an eradicated ally, got hp=%d eradicated=%v", gone.HitPoints, gone.HasCondition(character.ConditionEradicated))
	}
}

// Inferno damages the monsters AND burns every living party member, not only
// the caster.
func TestInferno_DamagesMobsAndEveryPartyMember(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "inferno", 100, 30)
	for _, member := range game.party.Members {
		member.MaxHitPoints, member.HitPoints = 1000, 1000
	}

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 100, 100
	m.PerfectDodge = 0
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if !game.combat.CastEquippedSpell() {
		t.Fatalf("inferno cast failed")
	}
	if m.HitPoints >= 100 {
		t.Errorf("inferno should damage the nearby monster, got %d", m.HitPoints)
	}
	for i, member := range game.party.Members {
		if member.HitPoints >= 1000 {
			t.Errorf("inferno spared party member %d", i)
		}
	}
}

func TestInferno_UsesFireResistanceButNeverGMPierce(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "inferno", 100, 30)
	caster := game.party.Members[0]
	caster.MagicSchools[character.MagicSchoolFire].Mastery = character.MasteryGrandMaster
	caster.Equipment[items.SlotRing1] = items.Item{
		Type:       items.ItemAccessory,
		Attributes: map[string]int{"resist_fire": 50},
	}
	caster.MaxHitPoints, caster.HitPoints = 1000, 1000

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 1000, 1000
	m.ArmorClass = 0 // isolate fire resistance: % armor would also blunt elemental
	m.Resistances[monster.DamageFire] = 60
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	def, err := spells.GetSpellDefinitionByID("inferno")
	if err != nil {
		t.Fatalf("inferno def: %v", err)
	}
	payload := game.combat.CalculatePartyNovaDamage(def, caster)

	if !game.combat.CastEquippedSpell() {
		t.Fatal("inferno cast failed")
	}
	// Plain resistance math with zero pierce: GM never pierces Inferno.
	if got, want := 1000-m.HitPoints, damagecalc.ApplyResistance(payload, 60, 0); got != want {
		t.Errorf("GM Inferno damage through 60%% fire resist = %d, want %d (%d normal fire damage, no school GM pierce)", got, want, payload)
	}
	if got, want := 1000-caster.HitPoints, damagecalc.ApplyResistance(payload, 50, 0); got != want {
		t.Errorf("Inferno self-damage through 50%% fire resist = %d, want %d", got, want)
	}
}

func TestInferno_OutgoingBuffAppliesOnlyToMonsters(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "inferno", 100, 30)
	def, err := spells.GetSpellDefinitionByID("inferno")
	if err != nil {
		t.Fatalf("inferno def: %v", err)
	}
	game.addCombatBuff(TimedCombatBuff{SpellID: "hour_of_power", Frames: 600, OutBonus: 5})

	for _, member := range game.party.Members {
		member.MaxHitPoints, member.HitPoints = 1000, 1000
	}
	caster := game.party.Members[0]
	casterHP0 := caster.HitPoints

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 1000, 1000
	m.ArmorClass = 0 // isolate the outgoing buff: % armor would scale it down too
	m.Resistances[monster.DamageFire] = 0
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if !game.combat.CastEquippedSpell() {
		t.Fatal("inferno cast failed")
	}

	baseDamage := def.SpellPointsCost * spells.SpellDamagePerSP
	if got, want := 1000-m.HitPoints, baseDamage+5; got != want {
		t.Errorf("Inferno monster damage with Hour of Power = %d, want %d", got, want)
	}
	if got, want := casterHP0-caster.HitPoints, baseDamage; got != want {
		t.Errorf("Inferno self-damage should not use outgoing bonus = %d, want %d", got, want)
	}
}

// Stone Skin, Heroism and Hour of Power must STACK - the refactored buff list
// sums their bonuses instead of clobbering a single slot.
func TestPartyBuffs_Stack(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	caster := game.party.Members[0]
	wantOut, wantIn := 0, 0
	for _, id := range []string{"hour_of_power", "stone_skin", "heroism"} {
		equipSpellAndPrepareCaster(t, game.combat, id, 100, 30)
		if !game.combat.CastEquippedSpell() {
			t.Fatalf("%s cast failed", id)
		}
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s def: %v", id, err)
		}
		if tier := character.SpellMasteryTier(caster, def); tier != 0 {
			t.Fatalf("%s cast at tier %d; the authored base applies at Novice only", id, tier)
		}
		// Untyped outgoing bonuses apply to every damage type.
		if def.OutgoingDamageType == "" || def.OutgoingDamageType == "physical" {
			wantOut += def.OutgoingDamageBonus
		}
		wantIn += def.IncomingDamageReduction
	}
	if out := game.combatBuffOutBonusForDamageType("physical"); out != wantOut {
		t.Errorf("outgoing bonus should stack to %d, got %d", wantOut, out)
	}
	if in := game.combatBuffInReduce(); in != wantIn {
		t.Errorf("incoming reduction should stack to %d, got %d", wantIn, in)
	}
	if n := len(game.combatBuffs); n != 3 {
		t.Errorf("expected 3 distinct active buffs, got %d", n)
	}
}

// Zones merge by TILE, not by overlap (user spec): a re-cast on the same tile is
// one field, a cast from another tile is its own field even when the two overlap.
// Overlap changes nothing else - no lifetime refresh, and never double damage
// (TestZone_IntersectionOfTwoCastsBillsOnce).
func TestHotSteam_ZonesMergeByTileNotByOverlap(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	equipSpellAndPrepareCaster(t, game.combat, "hot_steam", 1000, 30)

	if !game.combat.CastEquippedSpell() {
		t.Fatal("first hot_steam cast failed")
	}
	radius := game.persistentDamageZones[0].Radius

	if !game.combat.CastEquippedSpell() {
		t.Fatal("same-tile hot_steam cast failed")
	}
	if got := len(game.persistentDamageZones); got != 1 {
		t.Fatalf("same-tile re-cast = %d zones, want 1", got)
	}

	game.camera.X += radius
	if !game.combat.CastEquippedSpell() {
		t.Fatal("overlapping hot_steam cast failed")
	}
	if got := len(game.persistentDamageZones); got != 2 {
		t.Fatalf("overlapping cast from another tile = %d zones, want 2", got)
	}

	game.camera.X += radius * 3
	if !game.combat.CastEquippedSpell() {
		t.Fatal("separate hot_steam cast failed")
	}
	if got := len(game.persistentDamageZones); got != 3 {
		t.Fatalf("separate cast = %d zones, want 3", got)
	}
}
