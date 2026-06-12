package game

// Verifies the school-based damage scaling stat: Body/Mind/Spirit (self magic)
// scale with Personality; all other schools scale with Intellect. Guards both
// the formula and the shared spellScalesWithPersonality helper the tooltip uses.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// Party-buff magnitudes (Heroism/Stone Skin/Day of the Gods) are FLAT by
// balance decision — mastery scales ONLY the duration. Combat applies the raw
// YAML values; EffectLines quotes the same numbers (combat=tooltip SSoT).
func TestPartyBuffMagnitudeIsFlat_DurationScales(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 5, 5)
	cs := game.combat
	cleric := character.CreateCharacter("Cle", character.ClassCleric, game.config)
	spirit := cleric.MagicSchools[character.MagicSchoolSpirit]
	if spirit == nil {
		t.Fatal("cleric should start with the spirit school")
	}
	def, err := spells.GetSpellDefinitionByID("heroism")
	if err != nil {
		t.Fatalf("heroism def: %v", err)
	}

	castAt := func(m character.SkillMastery) TimedCombatBuff {
		spirit.Mastery = m
		if !cs.tryCastPartyBuff("heroism", def, cleric) {
			t.Fatalf("heroism must cast as a party buff")
		}
		b, ok := game.combatBuffByID("heroism")
		if !ok {
			t.Fatalf("buff not registered")
		}
		return b
	}

	tests := []struct {
		name        string
		mastery     character.SkillMastery
		durationPct int
	}{
		{name: "novice", mastery: character.MasteryNovice, durationPct: 100},
		{name: "expert", mastery: character.MasteryExpert, durationPct: 120},
		{name: "master", mastery: character.MasteryMaster, durationPct: 140},
		{name: "grandmaster", mastery: character.MasteryGrandMaster, durationPct: 160},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buff := castAt(tt.mastery)
			if buff.OutBonus != def.OutgoingDamageBonus {
				t.Errorf("magnitude must be flat YAML value %d, got %d",
					def.OutgoingDamageBonus, buff.OutBonus)
			}

			wantFrames := def.Duration * tt.durationPct / 100 * game.config.GetTPS()
			if buff.Frames != wantFrames {
				t.Errorf("duration at %s mastery = %d frames, want %d",
					tt.name, buff.Frames, wantFrames)
			}
		})
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
