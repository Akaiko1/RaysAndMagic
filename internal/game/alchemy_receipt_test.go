package game

import (
	"fmt"
	"maps"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Compare cosmetic inputs with actual inventory loss, independently of the plan.
func assertAlchemyReceipt(t *testing.T, before, after, receipt []items.Item) {
	t.Helper()
	want := map[string]int{}
	for _, it := range before {
		if it.Type == items.ItemTrinket {
			want[it.Name] += it.Count()
		}
	}
	for _, it := range after {
		if it.Type == items.ItemTrinket {
			want[it.Name] -= it.Count()
		}
	}
	for name, count := range want {
		if count == 0 {
			delete(want, name)
		}
	}
	got := map[string]int{}
	for _, it := range receipt {
		if _, duplicate := got[it.Name]; duplicate || it.Type != items.ItemTrinket || it.Quantity < 1 {
			t.Fatalf("invalid or duplicated displayed ingredient: %+v", it)
		}
		got[it.Name] = it.Quantity
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("displayed ingredients %v differ from consumed materials %v", got, want)
	}
	var order []string
	seen := map[string]bool{}
	for _, it := range before {
		if want[it.Name] > 0 && !seen[it.Name] {
			order = append(order, it.Name)
			seen[it.Name] = true
		}
	}
	for i, it := range receipt {
		if it.Name != order[i] {
			t.Fatal("ingredient icons changed inventory order")
		}
	}
}

func TestAlchemyReceiptMixedInputsAndSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, recipe    string
		choice, batches int
		keys            []string
		counts          []int
	}{
		{"split_herbs_surplus", "mana_potion", 2, 2,
			[]string{"mooncap", "bitterroot", "bitterroot", "embercap", "dawnleaf"}, []int{3, 1, 2, 3, 10}},
		{"mixed_fish", "health_potion", 0, 1,
			[]string{"dawnleaf", "carp_scale", "koi_scale", "rainbow_salmon_scale"}, []int{1, 1, 1, 2}},
		{"five_materials", "antivenom", 0, 4,
			[]string{"bitterroot", "plague_rat_tail", "echo_membrane", "serpent_skin", "bound_cucumbers"}, []int{4, 1, 1, 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Inventory = nil
			for i, key := range tc.keys {
				it, _ := items.TryCreateItemFromYAML(key)
				it.Quantity = tc.counts[i]
				// Keep the split stacks separate to cover legacy inventory layouts.
				g.party.Inventory = append(g.party.Inventory, it)
			}
			for i, r := range config.GlobalAlchemy.Recipes {
				if r.Key == tc.recipe {
					g.selectedRare = i
				}
			}
			g.alchemyBatches = tc.batches
			g.alchemy.Choices = map[string][]int{tc.recipe: {0, tc.choice}}
			before := append([]items.Item(nil), g.party.Inventory...)
			if !g.brewSelectedRecipe() {
				t.Fatal(g.rareBookMessage)
			}
			fx := g.brewAnimation
			assertAlchemyReceipt(t, before, g.party.Inventory, fx.Ingredients)
			snapshot := append([]items.Item(nil), fx.Ingredients...)
			for i := range snapshot {
				snapshot[i].Attributes = maps.Clone(snapshot[i].Attributes)
			}
			// The receipt must survive changes to stock, source choice and selection.
			for i := range g.party.Inventory {
				g.party.Inventory[i].Quantity++
				g.party.Inventory[i].Attributes["value"] = 999
			}
			g.alchemy.Choices[tc.recipe][1] = 0
			g.selectedRare = (g.selectedRare + 1) % len(config.GlobalAlchemy.Recipes)
			if !reflect.DeepEqual(fx.Ingredients, snapshot) {
				t.Fatal("committed ingredient snapshots changed with inventory or selection")
			}
		})
	}
}

func TestAlchemyReceiptOverlappingGroupsAndFailures(t *testing.T) {
	for _, state := range []string{"overlap", "missing", "incapacitated", "outside_party", "invalid_output", "ordinary_output"} {
		t.Run(state, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassAlchemist, false)
			r := *config.AlchemyRecipeByKey("mana_potion")
			r.Ingredients = []config.AlchemyIngredient{
				{Label: "First", Alternatives: []config.AlchemyAlternative{{Items: []string{"mooncap"}, Count: 1}}},
				{Label: "Second", Alternatives: []config.AlchemyAlternative{{Items: []string{"mooncap", "bitterroot"}, Count: 2}}},
			}
			g.party.Inventory = nil
			for _, key := range []string{"mooncap", "mooncap", "bitterroot"} {
				it, _ := items.TryCreateItemFromYAML(key)
				g.party.Inventory = append(g.party.Inventory, it)
			}
			switch state {
			case "missing":
				g.party.Inventory = g.party.Inventory[:2]
			case "incapacitated":
				c.Conditions = []character.Condition{character.ConditionUnconscious}
			case "outside_party":
				g.party.Members[0] = g.party.Members[1]
			case "invalid_output":
				r.Output = "missing_alchemy_output"
			case "ordinary_output":
				r.Output = "health_potion"
			}
			before := append([]items.Item(nil), g.party.Inventory...)
			n, receipt, err := g.party.Brew(c, &r, nil, 1)
			if state != "overlap" {
				if err == nil || n != 0 || receipt != nil || !reflect.DeepEqual(before, g.party.Inventory) {
					t.Fatal("failed brew emitted a receipt or changed inventory")
				}
				return
			}
			if err != nil || n <= 0 {
				t.Fatal(err)
			}
			assertAlchemyReceipt(t, before, g.party.Inventory, receipt)
		})
	}
}

func TestAlchemyReceiptIconLayout(t *testing.T) {
	for count := 1; count <= 16; count++ {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			for i := 0; i < count; i++ {
				r := alchemyBrewIngredientRect(i, count)
				if r.x < 0 || r.y < 0 || r.right() > 320 || r.bottom() > 210 || r.x < 226 && r.right() > 94 {
					t.Fatal("ingredient icon leaves canvas or overlaps the vessel")
				}
				for j := 0; j < i; j++ {
					other := alchemyBrewIngredientRect(j, count)
					if r.x < other.right() && r.right() > other.x && r.y < other.bottom() && r.bottom() > other.y {
						t.Fatal("mixed ingredient icons overlap")
					}
				}
			}
		})
	}
}
