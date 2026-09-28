package boot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlchemyTradeMaterialsFailAtBoot(t *testing.T) {
	if os.Getenv("RAM_TEST_ALCHEMY_MATERIALS") == "1" {
		LoadGameData()
		return
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(root, "assets", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	recipes, npcs := read("alchemy_recipes.yaml"), read("npcs.yaml")
	cases := []struct{ name, recipeItem, currency, merchant string }{
		{"valid", "", "", ""},
		{"shop_currency", "carp_scale", "clock_hand", "clockmaker"},
		{"black_scale", "ruby", "black_dragon_scale", "scalewright"},
		{"red_scale", "ruby", "red_dragon_scale", "scalewright"},
		{"green_scale", "emerald", "green_dragon_scale", "scalewright"},
		{"gold_scale", "emerald", "gold_dragon_scale", "scalewright"},
		{"new_shop_currency", "", "rabbit_pelt", "clockmaker"},
		{"new_stock_currency", "", "rabbit_pelt", "scalewright"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawRecipes, rawNPCs := recipes, npcs
			if tc.recipeItem != "" {
				rawRecipes = strings.Replace(recipes, "- "+tc.recipeItem+"\n", "- "+tc.currency+"\n", 1)
				if rawRecipes == recipes {
					t.Fatal("recipe fixture missing")
				}
				// Keep the category valid so boot reaches the merchant-currency
				// exclusion instead of rejecting an uncategorized ingredient first.
				rawRecipes = strings.Replace(rawRecipes, "\nrecipes:", "\n- key: currency_test\n  label: Currency test\n  items: ["+tc.currency+"]\nrecipes:", 1)
			} else if tc.name == "new_shop_currency" {
				rawNPCs = strings.Replace(npcs, "item:clock_hand", "item:rabbit_pelt", 1)
			} else if tc.name == "new_stock_currency" {
				rawNPCs = strings.Replace(npcs, "currency_item: black_dragon_scale", "currency_item: rabbit_pelt", 1)
				if rawNPCs == npcs {
					rawNPCs = strings.Replace(npcs, "currency_item: \"black_dragon_scale\"", "currency_item: \"rabbit_pelt\"", 1)
				}
			}
			if strings.HasPrefix(tc.name, "new_") && rawNPCs == npcs {
				t.Fatal("merchant fixture missing")
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
			for name, content := range map[string]string{"alchemy_recipes.yaml": rawRecipes, "npcs.yaml": rawNPCs} {
				if err := os.WriteFile(filepath.Join(assetDir, name), []byte(content), 0600); err != nil {
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
			if tc.name == "valid" {
				if err != nil {
					t.Fatalf("valid catalogs failed boot: %v\n%s", err, out)
				}
			} else if err == nil || !strings.Contains(string(out), "Alchemy materials:") || !strings.Contains(string(out), tc.currency) || !strings.Contains(string(out), tc.merchant) {
				t.Fatalf("merchant currency was not rejected at boot: %v\n%s", err, out)
			}
		})
	}
}
