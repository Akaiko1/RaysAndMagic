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
