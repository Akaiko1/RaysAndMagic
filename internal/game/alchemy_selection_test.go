package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func TestAlchemyCheckedMaterialsTransaction(t *testing.T) {
	type stock struct {
		key          string
		count, value int
	}
	for _, tc := range []struct {
		name             string
		stock            []stock
		checked          []string
		used             map[string]int
		maximum, batches int
	}{
		{"unchecked_cheapest", []stock{{"ocelot_pelt", 2, 1}, {"wolf_pelt", 4, 30}}, []string{"wolf_pelt"}, map[string]int{"wolf_pelt": 2}, 2, 1},
		{"mixed_rates", []stock{{"wolf_pelt", 1, 10}, {"carp_scale", 1, 2}, {"koi_scale", 1, 3}}, []string{"wolf_pelt", "carp_scale", "koi_scale"}, map[string]int{"wolf_pelt": 1, "carp_scale": 1, "koi_scale": 1}, 1, 1},
		{"scales_3_to_1", []stock{{"carp_scale", 3, 3}, {"koi_scale", 1, 5}}, []string{"carp_scale", "koi_scale"}, map[string]int{"carp_scale": 3, "koi_scale": 1}, 1, 1},
		{"scales_2_to_2", []stock{{"carp_scale", 2, 3}, {"koi_scale", 2, 5}}, []string{"carp_scale", "koi_scale"}, map[string]int{"carp_scale": 2, "koi_scale": 2}, 1, 1},
		{"scales_1_to_3", []stock{{"carp_scale", 1, 3}, {"koi_scale", 3, 5}}, []string{"carp_scale", "koi_scale"}, map[string]int{"carp_scale": 1, "koi_scale": 3}, 1, 1},
		{"cheapest_total_not_unit", []stock{{"wolf_pelt", 2, 5}, {"carp_scale", 4, 3}}, []string{"wolf_pelt", "carp_scale"}, map[string]int{"wolf_pelt": 2}, 2, 1},
		{"expensive_early_stack", []stock{{"ocelot_pelt", 1, 100}, {"carp_scale", 4, 3}}, []string{"ocelot_pelt", "carp_scale"}, map[string]int{"carp_scale": 4}, 2, 1},
		{"split_stacks_many_batches", []stock{{"wolf_pelt", 1, 2}, {"wolf_pelt", 2, 2}, {"carp_scale", 1, 1}, {"koi_scale", 1, 1}}, []string{"wolf_pelt", "carp_scale", "koi_scale"}, map[string]int{"wolf_pelt": 3, "carp_scale": 1, "koi_scale": 1}, 2, 2},
		{"empty_selection", []stock{{"wolf_pelt", 10, 2}}, nil, nil, 0, 1},
		{"selected_but_absent", []stock{{"wolf_pelt", 10, 2}}, []string{"carp_scale"}, nil, 0, 1},
		{"fractional_shortage", []stock{{"wolf_pelt", 1, 2}, {"carp_scale", 1, 1}}, []string{"wolf_pelt", "carp_scale"}, nil, 0, 1},
		{"batch_shortage", []stock{{"wolf_pelt", 2, 2}}, []string{"wolf_pelt"}, nil, 1, 2},
	} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", tc.name, tb), func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, tb)
				r := config.AlchemyRecipeByKey("health_potion")
				g.party.Inventory = nil
				herb, _ := items.TryCreateItemFromYAML("dawnleaf")
				herb.Quantity = 16
				g.party.Inventory = append(g.party.Inventory, herb)
				for _, s := range tc.stock {
					it, _ := items.TryCreateItemFromYAML(s.key)
					it.Quantity = s.count
					it.Attributes["value"] = s.value
					g.party.Inventory = append(g.party.Inventory, it)
				}
				selected := character.AlchemySelection{{"dawnleaf": true}, {}}
				for _, key := range tc.checked {
					selected[1][key] = true
				}
				g.alchemy.Selections = map[string]character.AlchemySelection{r.Key: selected}
				g.selectedRare, g.alchemyBatches = 0, tc.batches
				before := append([]items.Item(nil), g.party.Inventory...)
				ui := &UISystem{game: g}
				preview := ui.alchemyPlan(r, g.alchemySelection(r), tc.batches)
				if preview.max != tc.maximum {
					t.Fatalf("preview max=%d want %d", preview.max, tc.maximum)
				}
				if !reflect.DeepEqual(before, g.party.Inventory) {
					t.Fatal("preview spent materials")
				}
				if got := g.brewSelectedRecipe(); got != (tc.used != nil) {
					t.Fatalf("brew=%v: %s", got, g.rareBookMessage)
				}
				if tc.used == nil {
					if preview.err == nil || !reflect.DeepEqual(before, g.party.Inventory) || g.brewAnimation != nil {
						t.Fatal("invalid selection was not atomic")
					}
					return
				}
				if preview.err != nil {
					t.Fatal(preview.err)
				}
				expected := map[string]int{herb.Name: tc.batches}
				for key, n := range tc.used {
					d, _ := config.GetItemDefinition(key)
					expected[d.Name] = n
				}
				actual := map[string]int{}
				for _, it := range g.brewAnimation.Ingredients {
					actual[it.Name] += it.Count()
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("receipt=%v want %v", actual, expected)
				}
				fromPlan := map[string]int{}
				for i, n := range preview.plan {
					fromPlan[before[i].Name] += n
				}
				if !reflect.DeepEqual(fromPlan, actual) {
					t.Fatal("preview and commit disagree")
				}
				assertAlchemyReceipt(t, before, g.party.Inventory, g.brewAnimation.Ingredients)
				after := ui.alchemyPlan(r, g.alchemySelection(r), tc.batches)
				if after.max != tc.maximum-tc.batches {
					t.Fatal("inventory change left stale preview")
				}
			})
		}
	}
}

func TestAlchemyKeyedSelectionPersistence(t *testing.T) {
	for _, mode := range []string{"legacy", "multiple", "empty", "removed_key", "wrong_group_count"} {
		t.Run(mode, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			r := config.AlchemyRecipeByKey("health_potion")
			state := AlchemyState{Choices: map[string][]int{r.Key: {0, 1}}}
			want := character.AlchemySourceSelection(r, []int{0, 1})
			if mode != "legacy" {
				want = character.AlchemySelection{{"dawnleaf": true}, {}}
				if mode == "multiple" {
					want[1] = map[string]bool{"wolf_pelt": true, "carp_scale": true, "koi_scale": false}
				}
				state.Selections = map[string]character.AlchemySelection{r.Key: want.Clone()}
				state.Selections[r.Key][0]["dawnleaf"] = false
				if mode == "removed_key" {
					state.Selections[r.Key][1]["no_longer_in_recipe"] = true
				}
				if mode == "wrong_group_count" {
					state.Selections[r.Key] = nil
					want = character.AlchemySourceSelection(r, []int{0, 1})
				}
			}
			raw, err := json.Marshal(GameSave{Alchemy: state})
			if err != nil {
				t.Fatal(err)
			}
			var saved GameSave
			if err = json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(saved)
			g.restoreSavedTurnState(&saved)
			if got := g.alchemySelection(r); !reflect.DeepEqual(got, want) {
				t.Fatalf("selection=%v want %v", got, want)
			}
			g.alchemySelection(config.AlchemyRecipeByKey("mana_potion"))
			if !reflect.DeepEqual(g.alchemySelection(r), want) {
				t.Fatal("recipe switch lost selection")
			}
			cloned := g.alchemy.Clone()
			g.alchemy.Selections[r.Key][1]["wolf_pelt"] = !g.alchemy.Selections[r.Key][1]["wolf_pelt"]
			if !reflect.DeepEqual(cloned.Selections[r.Key], want) {
				t.Fatal("save clone aliases selection")
			}
			after, _ := json.Marshal(saved)
			if string(before) != string(after) {
				t.Fatal("restoring mutated saved choices")
			}
			g.restoreSavedTurnState(&GameSave{Alchemy: cloned})
			if !reflect.DeepEqual(g.alchemySelection(r), want) {
				t.Fatal("keyed selection did not survive reload")
			}
		})
	}
}

func TestAlchemyMaterialCatalogValidation(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	original := config.GlobalAlchemy
	t.Cleanup(func() { config.GlobalAlchemy = original })
	for _, tc := range []struct {
		name   string
		change func(*config.AlchemyConfig)
	}{
		{"valid", func(c *config.AlchemyConfig) {}},
		{"missing_category", func(c *config.AlchemyConfig) { c.Categories = c.Categories[1:] }},
		{"duplicate_category", func(c *config.AlchemyConfig) { c.Categories = append(c.Categories, c.Categories[0]) }},
		{"ambiguous_material", func(c *config.AlchemyConfig) {
			c.Categories[1].Items = append(c.Categories[1].Items, c.Categories[0].Items[0])
		}},
		{"unknown_material", func(c *config.AlchemyConfig) { c.Categories[0].Items = append(c.Categories[0].Items, "unknown") }},
		{"conflicting_cost", func(c *config.AlchemyConfig) {
			g := &c.Recipes[0].Ingredients[1]
			g.Alternatives = append(g.Alternatives, config.AlchemyAlternative{Items: g.Alternatives[0].Items, Count: 1})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := yaml.Marshal(original)
			var c config.AlchemyConfig
			if err := yaml.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			tc.change(&c)
			raw, _ = yaml.Marshal(c)
			path := filepath.Join(t.TempDir(), "alchemy.yaml")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := config.LoadAlchemyConfig(path)
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("catalog validation: %v", err)
			}
		})
	}
}

func TestAlchemyDisplayedMaterialScrollAndSelection(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, g.config)
			g.menuOpen, g.currentTab, g.selectedChar = true, TabSpellbook, 0
			r := &config.GlobalAlchemy.Recipes[0]
			l := computeRareBookLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content, true)
			a := makeAlchemyMaterialLayout(l, r)
			fp := installFakePointer(t)
			oldWheel := pointerWheel
			t.Cleanup(func() { pointerWheel = oldWheel })
			wheel := 0.0
			pointerWheel = func() (float64, float64) { return 0, wheel }
			h.clicks(false, a.toolbar.right()-20, a.toolbar.y+10, 1) // None.
			for _, cell := range a.cells {
				target := min(cell.rect.y-a.viewport.y, max(0, a.height-a.viewport.h))
				fp.moveTo(a.viewport.x+10, a.viewport.y+10)
				wheel = float64(h.ui.alchemyScroll[r.Key]-target) / 40
				h.pointerStep()
				wheel = 0
				if h.ui.alchemyScroll[r.Key] != target {
					t.Fatal("wheel did not reach material")
				}
				h.clicks(false, cell.rect.x+15, cell.rect.y-target+25, 1)
				if !g.alchemySelection(r)[cell.group][cell.key] {
					t.Fatalf("%s cannot be checked after scrolling", cell.key)
				}
			}
			before := g.alchemySelection(r).Clone()
			offset := h.ui.alchemyScroll[r.Key]
			fp.moveTo(a.viewport.x+10, a.viewport.y-2)
			wheel = 10
			h.pointerStep()
			wheel = 0
			if h.ui.alchemyScroll[r.Key] != offset {
				t.Fatal("wheel outside ingredient list scrolled it")
			}
			g.statPopupOpen, g.statPopupCharIdx = true, 0
			fp.moveTo(a.viewport.x+10, a.viewport.y+10)
			wheel = 1
			h.pointerStep()
			wheel = 0
			if h.ui.alchemyScroll[r.Key] != offset {
				t.Fatal("wheel crossed the modal into the ingredient list")
			}
			g.statPopupOpen = false
			// The wheel moves the next row under the cursor. A queued old-frame click
			// must not check either the old tile or its unseen replacement.
			fp.moveTo(a.viewport.x+15, a.viewport.y+25)
			wheel = 1
			h.clicks(false, a.viewport.x+15, a.viewport.y+25, 1)
			wheel = 0
			if !reflect.DeepEqual(before, g.alchemySelection(r)) {
				t.Fatal("scroll also toggled a stale tile")
			}
			savedOffset := h.ui.alchemyScroll[r.Key]
			g.selectedRare = 1
			h.pointerStep()
			if h.ui.alchemyScroll[config.GlobalAlchemy.Recipes[1].Key] != 0 {
				t.Fatal("recipes share scroll position")
			}
			g.selectedRare = 0
			h.pointerStep()
			if h.ui.alchemyScroll[r.Key] != savedOffset || !reflect.DeepEqual(before, g.alchemySelection(r)) {
				t.Fatal("recipe switch lost its selections or scroll")
			}
			g.menuOpen = false
			h.pointerStep()
			g.menuOpen = true
			h.pointerStep()
			if !reflect.DeepEqual(before, g.alchemySelection(r)) {
				t.Fatal("closing the book lost checked materials")
			}
			// Select just one on-screen scale and brew through the displayed button.
			h.ui.alchemyScroll[r.Key] = 0
			h.clicks(false, a.toolbar.right()-20, a.toolbar.y+10, 1)
			first := a.cells[0]
			h.clicks(false, first.rect.x+15, first.rect.y+25, 1)
			g.party.Inventory = nil
			for _, key := range []string{"dawnleaf", first.key, "ocelot_pelt"} {
				it, _ := items.TryCreateItemFromYAML(key)
				it.Quantity = 4
				g.party.AddItem(it)
			}
			inventory := append([]items.Item(nil), g.party.Inventory...)
			h.clicks(false, a.brew.x+20, a.brew.y+10, 1)
			if g.brewAnimation == nil {
				t.Fatalf("displayed selection failed to brew: %s", g.rareBookMessage)
			}
			assertAlchemyReceipt(t, inventory, g.party.Inventory, g.brewAnimation.Ingredients)
			d, _ := config.GetItemDefinition("ocelot_pelt")
			if g.party.CountItemsByName(d.Name) != 4 {
				t.Fatal("displayed selection consumed unchecked material")
			}
		})
	}
}

func TestAlchemyCategoriesAreSharedAcrossRecipes(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	l := computeRareBookLayout(layoutRect{0, 0, 1000, 700}, true)
	for _, recipe := range config.GlobalAlchemy.Recipes {
		a := makeAlchemyMaterialLayout(l, &recipe)
		count := map[string]int{}
		for _, cell := range a.cells {
			count[cell.key]++
		}
		previous := -1
		for _, heading := range a.headings {
			at := -1
			for i, category := range config.GlobalAlchemy.Categories {
				if category.Label == heading.label {
					at = i
				}
			}
			if at <= previous {
				t.Fatalf("%s has inconsistent category ordering", recipe.Key)
			}
			previous = at
		}
		for _, group := range recipe.Ingredients {
			if len(group.Materials()) == 1 {
				continue
			}
			for key, cost := range group.Materials() {
				if count[key] != 1 {
					t.Fatalf("%s/%s not shown exactly once", recipe.Key, key)
				}
				for _, cell := range a.cells {
					if cell.key == key && cell.count != cost {
						t.Fatal("displayed batch cost differs from recipe")
					}
				}
			}
		}
	}
}
