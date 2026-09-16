package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func usageFixture(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../assets/items.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}
func loadUsageFixture(t *testing.T, root map[string]any) (*ItemSystemConfig, error) {
	t.Helper()
	data, err := yaml.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "items.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return LoadItemConfig(path)
}
func preserveUsageCatalog(t *testing.T) {
	t.Helper()
	old, defs, keys := GlobalItems, itemDefByName, itemKeyByName
	t.Cleanup(func() { GlobalItems, itemDefByName, itemKeyByName = old, defs, keys })
}

func TestItemUsageDefaultsFromYAML(t *testing.T) {
	preserveUsageCatalog(t)
	root := usageFixture(t)
	root["tooltip_usage_defaults"] = map[string]any{"card": []string{"Card one", "Card two"}, "trinket": "Trade", "activate_inventory": "Activate", "consumed_on_use": "Consume", "consumed_after_promotion": "Promote", "cannot_sell": "No sale", "cannot_drop": "No drop"}
	cfg, err := loadUsageFixture(t, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key  string
		want []string
	}{
		{"medusa_card", []string{"Card one", "Card two"}},
		{"granite", []string{"Trade"}},
		{"health_potion", []string{"Activate", "Consume"}},
		{"leather_armor", nil}, {"belt_of_strength", nil},
	} {
		t.Run(tc.key, func(t *testing.T) {
			got := cfg.Items[tc.key].TooltipUsageLines()
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	for _, mode := range []string{"ordinary", "map", "promotion", "both"} {
		for _, value := range []int{0, 10} {
			for _, discardable := range []bool{false, true} {
				t.Run(fmt.Sprintf("quest/%s/%d/%v", mode, value, discardable), func(t *testing.T) {
					d := *cfg.Items["world_map"]
					d.OpensMap = mode == "map" || mode == "both"
					d.PromotesLich = mode == "promotion" || mode == "both"
					d.Value = value
					d.Discardable = discardable
					var want []string
					if mode != "ordinary" {
						want = append(want, "Activate")
					}
					if mode == "promotion" || mode == "both" {
						want = append(want, "Promote")
					}
					if value == 0 {
						want = append(want, "No sale")
					}
					if !discardable {
						want = append(want, "No drop")
					}
					if got := d.TooltipUsageLines(); !slices.Equal(got, want) {
						t.Fatalf("got %v want %v", got, want)
					}
					d.TooltipUsage = []string{"Override"}
					got := d.TooltipUsageLines()
					got[0] = "Changed"
					if d.TooltipUsage[0] != "Override" {
						t.Fatal("override slice aliased")
					}
				})
			}
		}
	}
	card := cfg.Items["medusa_card"]
	got := card.TooltipUsageLines()
	got[0] = "Changed"
	if card.TooltipUsageLines()[0] != "Card one" {
		t.Fatal("shared defaults aliased")
	}
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	if card.TooltipUsageLines()[0] != "Card one" {
		t.Fatal("reload changed another catalog's definitions")
	}
}

func TestItemUsageDefaultsRejectInvalidLoad(t *testing.T) {
	preserveUsageCatalog(t)
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"card", "trinket", "activate_inventory", "consumed_on_use", "consumed_after_promotion", "cannot_sell", "cannot_drop"} {
		for _, mode := range []string{"missing", "blank", "non ASCII"} {
			t.Run(key+"/"+mode, func(t *testing.T) {
				root := usageFixture(t)
				defaults := root["tooltip_usage_defaults"].(map[string]any)
				switch mode {
				case "missing":
					delete(defaults, key)
				case "blank":
					defaults[key] = " "
				case "non ASCII":
					defaults[key] = "No\u2014"
				}
				if key == "card" && mode != "missing" {
					defaults[key] = []string{defaults[key].(string)}
				}
				before := GlobalItems
				_, err := loadUsageFixture(t, root)
				if err == nil || !strings.Contains(err.Error(), key) || GlobalItems != before {
					t.Fatalf("invalid defaults published or wrong diagnostic: %v", err)
				}
			})
		}
	}
}
