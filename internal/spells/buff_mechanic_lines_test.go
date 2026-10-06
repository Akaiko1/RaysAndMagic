package spells

import (
	"slices"
	"testing"

	uitext "ugataima/assets/text"
)

func TestEffectViewsKeepBuffMagnitudesInSpellCards(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  SpellDefinition
		want []string
	}{
		{"per_stat", SpellDefinition{StatBonuses: map[string]int{"might": 9, "speed": -3}}, []string{"+9 Might (whole party)", "-3 Speed (whole party)"}},
		{"school_resistance", SpellDefinition{ResistBuffSchool: "fire", ResistBuffSchoolPct: 50}, []string{uitext.Text("spell.party_resists_for_the_duration", "Fire", 50)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, lines := range map[string][]string{"core": tc.def.CoreEffectLines(), "card": tc.def.CardEffectLines()} {
				if !slices.Equal(lines, tc.want) {
					t.Errorf("%s effects = %q, want authored magnitudes %q", name, lines, tc.want)
				}
			}
			if lines := tc.def.BuffMechanicLines(); len(lines) != 0 {
				t.Errorf("buff mechanics repeat live magnitudes: %q", lines)
			}
		})
	}
}
