package character

import (
	"strings"
	"testing"

	"ugataima/internal/config"
)

func TestFilteredItemEffectLinesRemovesStructuredArmorRows(t *testing.T) {
	def := &config.ItemDefinitionConfig{
		ArmorClassBase:          7,
		EnduranceScalingDivisor: 5,
		BonusMight:              3,
	}

	all := def.EffectLines()
	filtered := FilteredItemEffectLines(def)
	if got, want := len(filtered), len(all)-2; got != want {
		t.Fatalf("filtered effect lines = %d, want %d (all=%q filtered=%q)", got, want, all, filtered)
	}
	for _, line := range filtered {
		if strings.HasPrefix(line, "Armor class") || strings.HasPrefix(line, "AC +Endurance") {
			t.Fatalf("structured armor row leaked into filtered effects: %q", line)
		}
	}
}
