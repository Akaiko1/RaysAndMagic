package game

// Verifies the school-based damage scaling stat: Body/Mind/Spirit (self magic)
// scale with Personality; all other schools scale with Intellect. Guards both
// the formula and the shared spellScalesWithPersonality helper the tooltip uses.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// Party-buff magnitudes (Heroism/Stone Skin/Day of the Gods) scale with the
// spell-mastery bonus — the SAME helper combat applies in tryCastPartyBuff and
// the tooltip prints in getSpellMechanicsFromDefinition (combat=tooltip SSoT).
func TestPartyBuffMagnitudeScalesWithMastery(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	cleric := character.CreateCharacter("Cle", character.ClassCleric, game.config)
	spirit := cleric.MagicSchools[character.MagicSchoolSpirit]
	if spirit == nil {
		t.Fatal("cleric should start with the spirit school")
	}

	// Heroism is spirit-school; Novice (tier 0) → base, no bonus.
	spirit.Mastery = character.MasteryNovice
	if got := cs.spellBuffMagnitude(10, "heroism", cleric); got != 10 {
		t.Errorf("novice: want base 10, got %d", got)
	}
	// Master (tier 2) → base + 2×MasterySpellEffectPerLevel.
	spirit.Mastery = character.MasteryMaster
	if want, got := 10+2*MasterySpellEffectPerLevel, cs.spellBuffMagnitude(10, "heroism", cleric); got != want {
		t.Errorf("master: want %d, got %d", want, got)
	}
	// A zero base (spell lacks that effect) stays zero regardless of mastery.
	if got := cs.spellBuffMagnitude(0, "heroism", cleric); got != 0 {
		t.Errorf("zero base must stay zero, got %d", got)
	}
}

func TestSpellDamageScalingStatBySchool(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	caster := game.party.Members[0]

	// Helper: total damage for a spell with a given Intellect/Personality.
	dmg := func(spellID string, intel, pers int) int {
		caster.Intellect = intel
		caster.Personality = pers
		_, _, total := game.combat.CalculateSpellDamage(spells.SpellID(spellID), caster)
		return total
	}

	cases := []struct {
		spell     string
		personIty bool // expected to scale with Personality
	}{
		{"mind_blast", true},  // mind
		{"spirit_lash", true}, // spirit
		{"harm", true},        // body
		{"firebolt", false},   // fire
		{"rock_blast", false}, // earth
	}
	for _, c := range cases {
		hiPers := dmg(c.spell, 4, 60) // low Int, high Personality
		hiInt := dmg(c.spell, 60, 4)  // high Int, low Personality
		if c.personIty {
			if hiPers <= hiInt {
				t.Errorf("%s should scale with Personality: hiPers=%d should exceed hiInt=%d", c.spell, hiPers, hiInt)
			}
		} else {
			if hiInt <= hiPers {
				t.Errorf("%s should scale with Intellect: hiInt=%d should exceed hiPers=%d", c.spell, hiInt, hiPers)
			}
		}
	}

	// The tooltip label must match the formula's scaling stat.
	if got := spellDamageStatLabel("mind", false); got != "Personality" {
		t.Errorf("mind label = %q, want Personality", got)
	}
	if got := spellDamageStatLabel("fire", false); got != "Intellect" {
		t.Errorf("fire label = %q, want Intellect", got)
	}
	// A non-self school flagged scales_with_personality (e.g. ray_of_light) adds
	// a Personality term on top of Intellect — the label must name both.
	if got := spellDamageStatLabel("light", true); got != "Intellect + Personality" {
		t.Errorf("light+personality label = %q, want 'Intellect + Personality'", got)
	}
}
