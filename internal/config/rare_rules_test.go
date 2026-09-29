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
		{"missing_status", "status_damage", "", nil},
		{"missing_burn", "status_damage", "burn_percent", nil},
		{"missing_boss_burn", "status_damage", "boss_burn_percent", nil},
		{"zero_burn", "status_damage", "burn_percent", 0},
		{"invalid_boss_burn", "status_damage", "boss_burn_percent", 101},
		{"missing_status_effects", "status_effects", "", nil},
		{"missing_slow_skip", "status_effects", "slow_skip_every_turns", nil},
		{"slow_skip_every_turn", "status_effects", "slow_skip_every_turns", 1},
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
			if tc.section == "status_damage" || tc.section == "status_effects" {
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
	previousItems, previousLoot := GlobalItems, GlobalLoots
	t.Cleanup(func() { GlobalItems = previousItems; GlobalLoots = previousLoot })
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
					rarity = "min_rarity: rare\n        max_rarity: rare"
				}
				data := fmt.Sprintf("crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: catalog\n        item_type: %s\n        weight: 100\n        %s\n", d.Type, rarity)
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
