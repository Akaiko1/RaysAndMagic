package boot

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// yamlValue returns the value node under key in a mapping (or document) node.
func yamlValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if node.Kind == yaml.DocumentNode {
		node = node.Content[0]
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	t.Fatalf("yaml fixture has no %q key", key)
	return nil
}

// Item-backed merchant currencies (shop "item:" currencies and per-stock
// currency_item overrides) may not be alchemy ingredients. Cases come from the
// shipped catalogs: every reserved currency injected into a recipe, and an
// ingredient newly adopted as a shop and as a stock currency.
func TestAlchemyTradeMaterialsFailAtBoot(t *testing.T) {
	if os.Getenv("RAM_TEST_ALCHEMY_MATERIALS") == "1" {
		LoadGameData()
		return
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(root, "assets", name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	recipesRaw, npcsRaw := read("alchemy_recipes.yaml"), read("npcs.yaml")

	var catalog struct {
		NPCs map[string]struct {
			Currency  string `yaml:"currency"`
			Inventory []struct {
				CurrencyItem string `yaml:"currency_item"`
			} `yaml:"inventory"`
		} `yaml:"npcs"`
	}
	if err := yaml.Unmarshal(npcsRaw, &catalog); err != nil {
		t.Fatal(err)
	}
	reservers := map[string][]string{}
	shopNPC, stockNPC := "", ""
	for _, key := range slices.Sorted(maps.Keys(catalog.NPCs)) {
		npc := catalog.NPCs[key]
		if item, ok := strings.CutPrefix(npc.Currency, "item:"); ok {
			reservers[item] = append(reservers[item], key)
			if shopNPC == "" {
				shopNPC = key
			}
		}
		for _, entry := range npc.Inventory {
			if entry.CurrencyItem != "" {
				reservers[entry.CurrencyItem] = append(reservers[entry.CurrencyItem], key)
				if stockNPC == "" {
					stockNPC = key
				}
			}
		}
	}
	if shopNPC == "" || stockNPC == "" {
		t.Fatalf("fixture needs a shop item currency and a stock currency override (shop=%q stock=%q)", shopNPC, stockNPC)
	}

	parse := func(raw []byte) *yaml.Node {
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		return &doc
	}
	// firstItem is the first ingredient item of the first recipe.
	firstItem := func(recipes *yaml.Node) *yaml.Node {
		recipe := yamlValue(t, recipes, "recipes").Content[0]
		group := yamlValue(t, recipe, "ingredients").Content[0]
		source := yamlValue(t, group, "alternatives").Content[0]
		return yamlValue(t, source, "items").Content[0]
	}
	ingredient := firstItem(parse(recipesRaw)).Value
	if len(reservers[ingredient]) > 0 {
		t.Fatalf("fixture ingredient %q is already a merchant currency", ingredient)
	}

	type bootCase struct {
		name      string
		mutate    func(recipes, npcs *yaml.Node)
		reserved  string
		merchants []string
	}
	cases := []bootCase{{name: "valid"}}
	for _, currency := range slices.Sorted(maps.Keys(reservers)) {
		cases = append(cases, bootCase{
			name: "ingredient_" + currency,
			mutate: func(recipes, _ *yaml.Node) {
				firstItem(recipes).Value = currency
				// Keep the ingredient categorized so boot reaches the merchant
				// currency exclusion instead of rejecting an uncategorized item.
				categories := yamlValue(t, recipes, "categories")
				var category yaml.Node
				if err := yaml.Unmarshal([]byte("{key: currency_test, label: Currency test, items: ["+currency+"]}"), &category); err != nil {
					t.Fatal(err)
				}
				categories.Content = append(categories.Content, category.Content[0])
			},
			reserved: currency, merchants: reservers[currency],
		})
	}
	cases = append(cases,
		bootCase{
			name: "new_shop_currency",
			mutate: func(_, npcs *yaml.Node) {
				yamlValue(t, yamlValue(t, yamlValue(t, npcs, "npcs"), shopNPC), "currency").Value = "item:" + ingredient
			},
			reserved: ingredient, merchants: []string{shopNPC},
		},
		bootCase{
			name: "new_stock_currency",
			mutate: func(_, npcs *yaml.Node) {
				for _, entry := range yamlValue(t, yamlValue(t, yamlValue(t, npcs, "npcs"), stockNPC), "inventory").Content {
					for i := 0; i+1 < len(entry.Content); i += 2 {
						if entry.Content[i].Value == "currency_item" {
							entry.Content[i+1].Value = ingredient
							return
						}
					}
				}
				t.Fatalf("%s lost its stock currency", stockNPC)
			},
			reserved: ingredient, merchants: []string{stockNPC},
		},
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawRecipes, rawNPCs := recipesRaw, npcsRaw
			if tc.mutate != nil {
				recipes, npcs := parse(recipesRaw), parse(npcsRaw)
				tc.mutate(recipes, npcs)
				var err error
				if rawRecipes, err = yaml.Marshal(recipes); err != nil {
					t.Fatal(err)
				}
				if rawNPCs, err = yaml.Marshal(npcs); err != nil {
					t.Fatal(err)
				}
			}
			dir := t.TempDir()
			assetDir := filepath.Join(dir, "assets")
			if err := os.Mkdir(assetDir, 0700); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(root, "assets"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				name := entry.Name()
				if name == "alchemy_recipes.yaml" || name == "npcs.yaml" {
					continue
				}
				if err := os.Symlink(filepath.Join(root, "assets", name), filepath.Join(assetDir, name)); err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range map[string][]byte{"alchemy_recipes.yaml": rawRecipes, "npcs.yaml": rawNPCs} {
				if err := os.WriteFile(filepath.Join(assetDir, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(filepath.Join(root, "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestAlchemyTradeMaterialsFailAtBoot$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RAM_TEST_ALCHEMY_MATERIALS=1")
			out, err := cmd.CombinedOutput()
			if tc.mutate == nil {
				if err != nil {
					t.Fatalf("valid catalogs failed boot: %v\n%s", err, out)
				}
				return
			}
			named := slices.ContainsFunc(tc.merchants, func(m string) bool { return strings.Contains(string(out), m) })
			if err == nil || !strings.Contains(string(out), "Alchemy materials:") || !strings.Contains(string(out), tc.reserved) || !named {
				t.Fatalf("merchant currency %q (%v) was not rejected at boot: %v\n%s", tc.reserved, tc.merchants, err, out)
			}
		})
	}
}
