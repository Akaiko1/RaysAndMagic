package config

import "testing"

func TestValidateSpellAuthoring_Category(t *testing.T) {
	for _, tc := range []struct {
		name      string
		category  string
		utility   bool
		duration  int
		wantError bool
	}{
		{name: "default"},
		{name: "buff", category: "buff", utility: true, duration: 1},
		{name: "case-insensitive", category: "BuFf", utility: true, duration: 1},
		{name: "unknown", category: "enchantment", utility: true, duration: 1, wantError: true},
		{name: "instant utility cannot be a buff", category: "buff", utility: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := &SpellDefinitionConfig{Category: tc.category, IsUtility: tc.utility, Duration: tc.duration}
			cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{
				"test": def,
			}}
			err := validateSpellAuthoring(cfg)
			if (err != nil) != tc.wantError {
				t.Fatalf("validate category %q: err=%v, wantError=%v", tc.category, err, tc.wantError)
			}
		})
	}
}
