package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// helper: a character with a fixed effective Speed (no stat bonus, no gear).
func charWithSpeed(speed int) *character.MMCharacter {
	c := &character.MMCharacter{Speed: speed, Equipment: map[items.EquipSlot]items.Item{}}
	return c
}

// TestSpellCooldownFrames_MatchesAuthoredSeconds checks that at the reference
// Speed every spell's RT cooldown equals its authored cooldown_seconds (×TPS),
// proving the agreed table (firebolt 0.8s … buffs 5s) is what actually fires —
// i.e. the safety clamp no longer crushes the long spells.
func TestSpellCooldownFrames_MatchesAuthoredSeconds(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	tps := cs.game.config.GetTPS()
	caster := charWithSpeed(SpellCooldownSpeedRefSpeed)

	cases := []struct {
		id      string
		seconds float64
	}{
		{"firebolt", 0.8},
		{"fireball", 1.5},
		{"deadly_swarm", 1.5},
		{"charm", 2.0},
		{"starburst", 3.0},
		{"inferno", 3.0},
		{"hot_steam", 3.0},
		{"stun", 3.0},
		{"darkness", 4.0},
		{"bless", 5.0},
		{"hour_of_power", 5.0},
		{"resurrect", 5.0},
	}
	for _, tc := range cases {
		want := int(tc.seconds * float64(tps)) // factor == 1.0 at reference speed
		got := cs.SpellCooldownFrames(caster, spells.SpellID(tc.id))
		if got != want {
			t.Errorf("%s: cooldown = %d frames (%.2fs), want %d (%.2fs)",
				tc.id, got, float64(got)/float64(tps), want, tc.seconds)
		}
	}
}

// TestSpellCooldownFrames_SpeedScales confirms Speed still influences spell
// cooldown: a faster caster casts the same spell sooner.
func TestSpellCooldownFrames_SpeedScales(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	slow := cs.SpellCooldownFrames(charWithSpeed(5), "fireball")
	ref := cs.SpellCooldownFrames(charWithSpeed(SpellCooldownSpeedRefSpeed), "fireball")
	fast := cs.SpellCooldownFrames(charWithSpeed(50), "fireball")
	if !(slow > ref && ref > fast) {
		t.Errorf("expected slow(%d) > ref(%d) > fast(%d)", slow, ref, fast)
	}
}

// TestWeaponCooldown_TypeMultipliers checks the weapon-type ordering (dagger
// fast … axe slow) and that the Bow of Hellfire legendary override makes it
// SLOWER than an ordinary bow despite both being bows.
func TestWeaponCooldown_TypeMultipliers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	speed := 25

	frames := func(weaponKey string) int {
		c := charWithSpeed(speed)
		c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(weaponKey)
		return cs.WeaponCooldownFrames(c)
	}

	dagger := frames("magic_dagger")
	sword := frames("iron_sword")
	bow := frames("hunting_bow")
	axe := frames("bronze_labrys") // category-baseline axe (steel_axe/gorehorn carry per-weapon overrides)
	hellfire := frames("bow_of_hellfire")

	if !(dagger < sword && sword < bow && bow < axe) {
		t.Errorf("expected dagger(%d) < sword(%d) < bow(%d) < axe(%d)", dagger, sword, bow, axe)
	}
	if hellfire <= bow {
		t.Errorf("Bow of Hellfire (%d) should be SLOWER than a plain bow (%d)", hellfire, bow)
	}

	// "throwing" is not a real weapon type — it resolves to the dagger SKILL, so
	// the throwing knife must share the dagger cooldown.
	if throwingKnife := frames("throwing_knife"); throwingKnife != dagger {
		t.Errorf("throwing_knife cooldown (%d) should equal dagger (%d) — throwing maps to the dagger skill", throwingKnife, dagger)
	}
	// "blaster" maps to no weapon skill, so the alien blaster gets the neutral
	// 1.0 multiplier (same as the sword baseline).
	if blaster := frames("alien_blaster"); blaster != sword {
		t.Errorf("alien_blaster cooldown (%d) should be neutral 1.0 == sword (%d) — blaster is not a real weapon type", blaster, sword)
	}
}

// TestArchmageStaff_ReducesSpellCooldown verifies the legendary staff's
// spell_cooldown_multiplier (−20%) lowers the caster's spell cooldown.
func TestArchmageStaff_ReducesSpellCooldown(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	bare := charWithSpeed(25)
	staffed := charWithSpeed(25)
	staffed.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("archmage_staff")

	base := cs.SpellCooldownFrames(bare, "fireball")
	withStaff := cs.SpellCooldownFrames(staffed, "fireball")
	if withStaff >= base {
		t.Errorf("Archmage Staff should reduce spell cooldown: with=%d, base=%d", withStaff, base)
	}
}

// TestSpellClassification_OffensiveVsSupport guards the smart-attack autocast
// rule: offensive spells (incl. AoE-stun and zones that are flagged utility)
// count as combat; heals/buffs/pure-utility do not.
func TestSpellClassification_OffensiveVsSupport(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	offensive := []string{"firebolt", "fireball", "stun", "darkness", "hot_steam", "inferno", "charm", "disintegrate", "psychic_shock"}
	support := []string{"heal", "heal_other", "mass_heal", "bless", "heroism", "stone_skin", "resurrect", "torch_light", "wizard_eye", "awaken"}
	for _, id := range offensive {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !def.IsOffensive() {
			t.Errorf("%s should be offensive (autocast on Space)", id)
		}
	}
	for _, id := range support {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if def.IsOffensive() {
			t.Errorf("%s should NOT be offensive (Space must skip it)", id)
		}
	}
}
