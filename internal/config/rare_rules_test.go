package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRareRulesRequiredBalanceConfig(t *testing.T) {
	original, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	var baseline map[string]any
	if err = yaml.Unmarshal(original, &baseline); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, section, key string
		value              any
	}{
		{"missing_status_effects", "status_effects", "", nil},
		{"missing_adventure_scaling", "adventure_scaling", "", nil},
		{"missing_adventure_boss_curve", "adventure_scaling", "boss", nil},
		{"negative_visit_bonus", "adventure_scaling", "visit_level_bonus", -1},
		{"overflowing_curve", "adventure_scaling", "regular", map[string]any{
			"hp":                       map[string]any{"base": 1000, "growth": []any{map[string]any{"to_level": 25, "slope": 0.1}}, "flat_per_level": 1},
			"hit":                      map[string]any{"base": 2, "growth": []any{map[string]any{"to_level": 45, "slope": 0.05}}, "flat_per_level": 1},
			"experience_level_percent": 24}},
	}
	for key := range baseline["characters"].(map[string]any)["classes"].(map[string]any) {
		for _, field := range []string{"", "primary", "speed", "endurance"} {
			cases = append(cases, struct {
				name, section, key string
				value              any
			}{key + "/" + field, key, field, nil})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := yaml.Unmarshal(original, &data); err != nil {
				t.Fatal(err)
			}
			expected := tc.section
			if tc.section == "status_effects" || tc.section == "adventure_scaling" {
				if tc.key == "" {
					delete(data, tc.section)
				} else if tc.value == nil {
					delete(data[tc.section].(map[string]any), tc.key)
				} else {
					data[tc.section].(map[string]any)[tc.key] = tc.value
				}
			} else {
				expected = "auto_stats"
				class := data["characters"].(map[string]any)["classes"].(map[string]any)[tc.section].(map[string]any)
				if tc.key == "" {
					delete(class, "auto_stats")
				} else {
					delete(class["auto_stats"].(map[string]any), tc.key)
				}
			}
			encoded, err := yaml.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err = os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			before := GlobalConfig
			if _, err = LoadConfig(path); err == nil || !strings.Contains(err.Error(), expected) {
				t.Fatalf("missing balance field did not fail at load: %v", err)
			}
			if GlobalConfig != before {
				t.Fatal("invalid config was published")
			}
		})
	}
}

func TestRareRulesCrateValidationMatchesRuntime(t *testing.T) {
	previousItems, previousLoot, previousWeapons := GlobalItems, GlobalLoots, GlobalWeapons
	t.Cleanup(func() { GlobalItems, GlobalLoots, GlobalWeapons = previousItems, previousLoot, previousWeapons })
	GlobalWeapons = nil // the one item under test is the whole catalog
	for _, kind := range []string{"crafted", "harvest", "quest", "ordinary"} {
		for _, filter := range []string{"rarity", "range"} {
			t.Run(kind+"/"+filter, func(t *testing.T) {
				d := &ItemDefinitionConfig{Type: "trinket", Rarity: "rare"}
				switch kind {
				case "crafted":
					d.Type = "consumable"
					d.CraftedOnly = true
				case "harvest":
					d.HarvestSprite = "plant"
				case "quest":
					d.Type = "quest"
				}
				GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"only": d}}
				rarity := "rarity: rare"
				if filter == "range" {
					rarity = "rarity: uncommon-rare"
				}
				itemType := d.Type
				if kind == "quest" {
					itemType = "any" // quest items stay out of even the widest catalog
				}
				data := fmt.Sprintf("crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: catalog\n        item_type: %s\n        weight: 100\n        %s\n", itemType, rarity)
				path := filepath.Join(t.TempDir(), "loot.yaml")
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				_, err := LoadLootTables(path)
				if (err == nil) != (kind == "ordinary") {
					t.Fatalf("crate eligibility drift at load: %v", err)
				}
				if err != nil && !strings.Contains(err.Error(), "no catalog items match") {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRareRulesZoneAuthoringValidation(t *testing.T) {
	original, err := os.ReadFile("../../assets/spells.yaml")
	if err != nil {
		t.Fatal(err)
	}
	previous := GlobalSpells
	t.Cleanup(func() { GlobalSpells = previous })
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"zone_edge_tiles", -1}, {"zone_edge_tiles", 0}, {"zone_edge_damage_percent", 0}, {"zone_edge_damage_percent", 101}, {"zone_burn_seconds", -1}, {"zone_radius_tiles", 0}, {"zone_width_tiles", 0},
	} {
		t.Run(fmt.Sprint(tc.key, tc.value), func(t *testing.T) {
			var data map[string]any
			if err := yaml.Unmarshal(original, &data); err != nil {
				t.Fatal(err)
			}
			data["spells"].(map[string]any)["firewall"].(map[string]any)[tc.key] = tc.value
			encoded, err := yaml.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "spells.yaml")
			if err = os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = LoadSpellConfig(path); err == nil {
				t.Fatal("invalid zone authoring accepted")
			}
		})
	}
}
