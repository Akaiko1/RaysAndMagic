package config

import "testing"

// A non-stacking card rule loads only complete, in range and on a card.
func TestCardRulesValidation(t *testing.T) {
	previous := GlobalItems
	t.Cleanup(func() { GlobalItems = previous })
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(d *ItemDefinitionConfig)
		ok   bool
	}{
		{"authored wildfire", func(*ItemDefinitionConfig) {}, true},
		{"wildfire without radius", func(d *ItemDefinitionConfig) { d.CardWildfireRadiusTiles = 0 }, false},
		{"wildfire of an unknown school", func(d *ItemDefinitionConfig) { d.CardWildfireSchool = "plasma" }, false},
		{"wildfire of another school", func(d *ItemDefinitionConfig) { d.CardWildfireSchool = "water" }, true},
		{"wildfire burn of part of a round", func(d *ItemDefinitionConfig) { d.CardWildfireBurnSeconds = 4 }, false},
		{"rule field without its strength", func(d *ItemDefinitionConfig) { d.CardOverflowCapPct = 50 }, false},
		{"rule on a non-card", func(d *ItemDefinitionConfig) { d.Type = "trinket" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := *GlobalItems.Items["solstice_aureth_card"]
			tc.edit(&d)
			if err := d.validateCardRules(); (err == nil) != tc.ok {
				t.Fatalf("valid=%v, want %v: %v", err == nil, tc.ok, err)
			}
		})
	}
}
