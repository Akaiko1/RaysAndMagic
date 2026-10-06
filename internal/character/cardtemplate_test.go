package character

import (
	"slices"
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

	filtered := FilteredItemEffectLines(def)
	if want := def.StatBonusLines(); len(want) == 0 || !slices.Equal(filtered, want) {
		t.Fatalf("filtered effect lines = %q, want only the stat rows %q", filtered, want)
	}
	for _, line := range filtered {
		if strings.HasPrefix(line, "Armor class") || strings.HasPrefix(line, "AC +Endurance") {
			t.Fatalf("structured armor row leaked into filtered effects: %q", line)
		}
	}
}
