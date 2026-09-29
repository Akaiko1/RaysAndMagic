package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPotionPreferencesPersistence(t *testing.T) {
	previous := GlobalItems
	t.Cleanup(func() { GlobalItems = previous })
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, data   string
		hp, sp       int
		health, mana bool
		bad          bool
	}{
		{"missing", "", 35, 35, true, true, false},
		{"legacy empty", "{}", 35, 35, true, true, false},
		{"partial", `{"health":{"threshold_percent":60}}`, 60, 35, true, true, false},
		{"off and clamped", `{"health":{"threshold_percent":-10},"mana":{"threshold_percent":120}}`, 0, 100, true, true, false},
		{"empty selection", `{"health":{"allowed_types":{}}}`, 35, 35, false, true, false},
		{"selected only", `{"health":{"allowed_types":{"antivenom":true,"mana_potion":true,"unknown":true}},"mana":{"allowed_types":{"mana_potion":false}}}`, 35, 35, false, false, false},
		{"malformed", `{"health":`, 0, 0, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			c.Characters.AutoDrink.ThresholdPct = 35
			path := filepath.Join(t.TempDir(), "settings.json")
			if tc.data != "" {
				if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			original := c.EnsurePotionPreferences()
			err := c.LoadPotionPreferences(path)
			if tc.bad {
				if err == nil || c.PlayerPotions != original {
					t.Fatal("bad file replaced the current policy")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p := c.PlayerPotions
			if p.Health.ThresholdPct != tc.hp || p.Mana.ThresholdPct != tc.sp || p.Health.Allowed["health_potion"] != tc.health || p.Mana.Allowed["mana_potion"] != tc.mana {
				t.Fatalf("wrong policy: %+v", p)
			}
			if p.Health.Allowed["unknown"] || p.Health.Allowed["mana_potion"] {
				t.Fatal("unknown/ineligible selection authorized")
			}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			c.PlayerPotions = nil
			if err := c.LoadPotionPreferences(path); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p, c.PlayerPotions) {
				t.Fatal("restart changed preferences")
			}
		})
	}
}
