package game

// Tests for the spells added to bring every magic school to ≥4: the code-backed
// ones (Raise Dead, Mass Heal, Inferno, buff stacking). Pure-YAML damage spells
// are exercised by the generic projectile path already.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// Raise Dead restores a fallen (non-eradicated) ally to 25% of max HP and clears
// Dead/Unconscious.
func TestRaiseDead_RevivesTo25Pct(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "raise_dead", 100, 30)

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
	if fallen.HitPoints != 10 { // 25% of 40
		t.Errorf("raise_dead should restore 25%% of max (10), got %d", fallen.HitPoints)
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

// Mass Heal restores every party member, not just the caster.
func TestMassHeal_HealsWholeParty(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "mass_heal", 100, 30)
	for _, m := range game.party.Members {
		m.MaxHitPoints = 100
		m.HitPoints = 50
	}
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("mass_heal cast failed")
	}
	for i, m := range game.party.Members {
		if m.HitPoints <= 50 {
			t.Errorf("member %d should have been healed, got %d", i, m.HitPoints)
		}
	}
}

// Inferno damages every monster in range AND the whole party.
func TestInferno_DamagesMobsAndParty(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "inferno", 100, 30)

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 100, 100
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	partyHP0 := partyHPSum(game)
	if !game.combat.CastEquippedSpell() {
		t.Fatalf("inferno cast failed")
	}
	if m.HitPoints >= 100 {
		t.Errorf("inferno should damage the nearby monster, got %d", m.HitPoints)
	}
	if partyHPSum(game) >= partyHP0 {
		t.Errorf("inferno should damage the party too (was %d, now %d)", partyHP0, partyHPSum(game))
	}
}

func TestInferno_UsesFireResistanceButNeverGMPierce(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	equipSpellAndPrepareCaster(t, game.combat, "inferno", 100, 30)
	caster := game.party.Members[0]
	caster.MagicSchools[character.MagicSchoolFire] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	caster.Equipment[items.SlotRing1] = items.Item{
		Type:       items.ItemAccessory,
		Attributes: map[string]int{"resist_fire": 50},
	}
	caster.MaxHitPoints, caster.HitPoints = 1000, 1000

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 1000, 1000
	m.Resistances[monster.DamageFire] = 60
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if !game.combat.CastEquippedSpell() {
		t.Fatal("inferno cast failed")
	}
	if got, want := 1000-m.HitPoints, 18; got != want {
		t.Errorf("GM Inferno damage through 60%% fire resist = %d, want %d (no GM pierce)", got, want)
	}
	if got, want := 1000-caster.HitPoints, 22; got != want {
		t.Errorf("Inferno self-damage through 50%% fire resist = %d, want %d", got, want)
	}
}

// Stone Skin, Heroism and Hour of Power must STACK — the refactored buff list
// sums their bonuses instead of clobbering a single slot.
func TestPartyBuffs_Stack(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	for _, id := range []string{"hour_of_power", "stone_skin", "heroism"} {
		equipSpellAndPrepareCaster(t, game.combat, id, 100, 30)
		if !game.combat.CastEquippedSpell() {
			t.Fatalf("%s cast failed", id)
		}
	}
	// Hour of Power +15 out / -5 in, Stone Skin -6 in, Heroism +10 out.
	if out := game.combatBuffOutBonus(); out != 25 {
		t.Errorf("outgoing bonus should stack to 25, got %d", out)
	}
	if in := game.combatBuffInReduce(); in != 11 {
		t.Errorf("incoming reduction should stack to 11, got %d", in)
	}
	if n := len(game.combatBuffs); n != 3 {
		t.Errorf("expected 3 distinct active buffs, got %d", n)
	}
}

// Hot Steam must damage monsters in TURN-BASED mode too (once per monster turn
// via tickSteamZonesTB), not only in real time.
func TestHotSteam_DamagesInTurnBased(t *testing.T) {
	game, gl, _ := tbBehaviorGame(t, 7, 7)
	equipSpellAndPrepareCaster(t, game.combat, "hot_steam", 100, 30)

	mon := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	mon.MaxHitPoints, mon.HitPoints = 200, 200
	game.world.Monsters = []*monster.Monster3D{mon}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if !game.combat.CastEquippedSpell() { // creates the zone at the party
		t.Fatalf("hot_steam cast failed")
	}
	if len(game.steamZones) == 0 {
		t.Fatalf("no steam zone created")
	}
	// Run a monster turn — the zone should sear the monster inside it.
	game.turnBasedMode = true
	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased()

	if mon.HitPoints >= 200 {
		t.Errorf("hot_steam should damage the monster in TB (hp still %d)", mon.HitPoints)
	}
}

func TestHotSteam_GMPiercesResistance(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 7, 7)
	equipSpellAndPrepareCaster(t, game.combat, "hot_steam", 100, 30)
	caster := game.party.Members[0]
	caster.MagicSchools[character.MagicSchoolWater] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}

	m := monster.NewMonster3DFromConfig(game.camera.X+32, game.camera.Y, "goblin", game.config)
	m.MaxHitPoints, m.HitPoints = 1000, 1000
	m.Resistances[monster.DamageWater] = 60
	game.world.Monsters = []*monster.Monster3D{m}

	if !game.combat.CastEquippedSpell() {
		t.Fatal("hot_steam cast failed")
	}
	z := &game.steamZones[0]
	if z.ResistPierce != MagicGMResistPiercePct {
		t.Fatalf("Hot Steam stored resist pierce %d, want %d", z.ResistPierce, MagicGMResistPiercePct)
	}
	game.combat.damageSteamZoneOnce(z)
	want := z.TickDamage * 70 / 100 // 60% resistance becomes 30% after 50% pierce.
	if got := 1000 - m.HitPoints; got != want {
		t.Errorf("GM Hot Steam damage through 60%% water resist = %d, want %d", got, want)
	}
}

func TestHotSteam_OverlappingZonesReplaceButSeparateZonesRemain(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	equipSpellAndPrepareCaster(t, game.combat, "hot_steam", 1000, 30)

	if !game.combat.CastEquippedSpell() {
		t.Fatal("first hot_steam cast failed")
	}
	radius := game.steamZones[0].Radius

	game.camera.X += radius
	if !game.combat.CastEquippedSpell() {
		t.Fatal("overlapping hot_steam cast failed")
	}
	if got := len(game.steamZones); got != 1 {
		t.Fatalf("overlapping Hot Steam zones = %d, want 1 replacement", got)
	}

	game.camera.X += radius * 3
	if !game.combat.CastEquippedSpell() {
		t.Fatal("separate hot_steam cast failed")
	}
	if got := len(game.steamZones); got != 2 {
		t.Fatalf("separate Hot Steam zones = %d, want 2", got)
	}
}
