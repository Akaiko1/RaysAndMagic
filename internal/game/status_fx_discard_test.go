package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Quest items stay undiscardable, except items that opt out via
// `discardable: true` in items.yaml (a refusable quest choice).
func TestQuestItemDiscardability(t *testing.T) {
	loadTestConfig(t)
	for _, tc := range []struct {
		name string
		def  config.ItemDefinitionConfig
		want bool
	}{
		{"quest_opted_out", config.ItemDefinitionConfig{Name: "Fixture Refusable Relic", Type: "quest", Discardable: true}, true},
		{"plain_quest", config.ItemDefinitionConfig{Name: "Fixture Quest Relic", Type: "quest"}, false},
		{"non_quest", config.ItemDefinitionConfig{Name: "Fixture Trinket", Type: "trinket"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := "discard_fixture_" + tc.name
			def := tc.def
			config.GlobalItems.Items[key] = &def
			t.Cleanup(func() { delete(config.GlobalItems.Items, key) })
			if got := itemDiscardable(items.CreateItemFromYAML(key)); got != tc.want {
				t.Fatalf("itemDiscardable = %v, want %v", got, tc.want)
			}
		})
	}
}
