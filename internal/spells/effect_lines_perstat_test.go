package spells

import "testing"

// Per-stat buffs are character-independent, so both production views (the live
// tooltip and the editor card) must state every authored stat.
func TestEffectLines_PerStatBonuses(t *testing.T) {
	d := SpellDefinition{StatBonuses: map[string]int{"might": 15, "speed": 5}}
	views := map[string]func() []string{
		"CoreEffectLines": d.CoreEffectLines,
		"CardEffectLines": d.CardEffectLines,
	}
	for name, view := range views {
		lines := view()
		want := map[string]bool{"+15 Might (whole party)": false, "+5 Speed (whole party)": false}
		for _, l := range lines {
			if _, ok := want[l]; ok {
				want[l] = true
			}
		}
		for line, seen := range want {
			if !seen {
				t.Errorf("%s missing %q (got %v)", name, line, lines)
			}
		}
	}
}
