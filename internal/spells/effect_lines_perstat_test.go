package spells

import "testing"

func TestEffectLines_PerStatBonuses(t *testing.T) {
	d := SpellDefinition{StatBonuses: map[string]int{"might": 15, "speed": 5}}
	lines := d.EffectLines()
	want := map[string]bool{"+15 Might (whole party)": false, "+5 Speed (whole party)": false}
	for _, l := range lines {
		if _, ok := want[l]; ok {
			want[l] = true
		}
	}
	for line, seen := range want {
		if !seen {
			t.Errorf("EffectLines missing %q (got %v)", line, lines)
		}
	}
}
