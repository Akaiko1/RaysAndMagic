package config

import "testing"

func TestRangedWeaponsDoNotScaleWithMight(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}

	for key, def := range cfg.Weapons {
		if def.Range <= 3 {
			continue
		}
		if def.BonusStat == "" {
			t.Errorf("ranged weapon %q has no bonus_stat and falls back to Might", key)
		}
		if def.BonusStat == "Might" {
			t.Errorf("ranged weapon %q scales with Might", key)
		}
	}
}

func TestWeaponBalanceOverrides(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}

	tests := []struct {
		key   string
		check func(*WeaponDefinitionConfig) bool
		want  string
	}{
		{"inazuma_matchlock", func(def *WeaponDefinitionConfig) bool { return def.Damage == 28 }, "damage 28"},
		{"book_of_darkness", func(def *WeaponDefinitionConfig) bool { return def.ExecuteBelowPct == 15 }, "execute threshold 15%"},
		{"bronze_labrys", func(def *WeaponDefinitionConfig) bool { return def.Rarity == "common" }, "common rarity"},
		{"suppressor_gun", func(def *WeaponDefinitionConfig) bool { return def.BonusStat == "Accuracy" }, "Accuracy scaling"},
		{"longlance_rifle", func(def *WeaponDefinitionConfig) bool { return def.BonusStat == "Accuracy" }, "Accuracy scaling"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			def, ok := cfg.Weapons[tt.key]
			if !ok {
				t.Fatalf("weapon %q is missing", tt.key)
			}
			if !tt.check(def) {
				t.Errorf("weapon %q does not have %s", tt.key, tt.want)
			}
		})
	}
}
