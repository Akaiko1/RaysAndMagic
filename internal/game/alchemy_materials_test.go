package game

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

func TestAlchemySavedSourcesFollowCurrentRecipe(t *testing.T) {
	for _, tc := range []struct {
		name        string
		saved, want []int
	}{
		{"legacy_absent", nil, []int{0, 0}},
		{"valid", []int{0, 1}, []int{0, 1}},
		{"removed_source", []int{0, 99}, []int{0, 3}},
		{"negative", []int{-1, -1}, []int{0, 0}},
		{"changed_groups", []int{0}, []int{0, 0}},
	} {
		for _, entry := range []string{"preview", "brew"} {
			t.Run(tc.name+"/"+entry, func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassAlchemist, false)
				r := config.AlchemyRecipeByKey("health_potion")
				save := GameSave{Alchemy: AlchemyState{Choices: map[string][]int{r.Key: tc.saved}}}
				raw, err := json.Marshal(save)
				if err != nil {
					t.Fatal(err)
				}
				var decoded GameSave
				if err := json.Unmarshal(raw, &decoded); err != nil {
					t.Fatal(err)
				}
				g.restoreSavedTurnState(&decoded)
				g.party.Inventory = nil
				for i, group := range r.Ingredients {
					a := group.Alternatives[tc.want[i]]
					it, _ := items.TryCreateItemFromYAML(a.Items[0])
					it.Quantity = a.Count
					g.party.AddItem(it)
				}
				if entry == "preview" {
					if (&UISystem{game: g}).alchemyPlan(r, g.alchemySelection(r), 1).max != 1 {
						t.Fatal("saved source breaks the displayed batch limit")
					}
				} else {
					g.selectedRare, g.alchemyBatches = 0, 1
					if !g.brewSelectedRecipe() {
						t.Fatalf("saved source blocks crafting: %s", g.rareBookMessage)
					}
				}
				if !reflect.DeepEqual(g.alchemy.Choices[r.Key], tc.want) || !reflect.DeepEqual(decoded.Alchemy.Choices[r.Key], tc.saved) {
					t.Fatal("selection normalization disagrees with preview or mutates the save")
				}
			})
		}
	}
}

// Batch costs stay recipe-authored (alchemy_recipes.yaml); this checks only
// that ordinary recipes draw on mundane materials with a regular source.
func TestAlchemyMaterialProvenance(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, recipe := range config.GlobalAlchemy.Recipes {
		for _, source := range recipe.Ingredients[1].Alternatives {
			for _, key := range source.Items {
				t.Run(recipe.Key+"/"+key, func(t *testing.T) {
					def, _ := config.GetItemDefinition(key)
					if recipe.Family != "common" {
						return // Specialty recipes may use gems and boss trophies.
					}
					if def.Value > 200 || def.Rarity == "rare" || def.Rarity == "legendary" {
						t.Errorf("ordinary batch consumes valuable material %s", key)
					}
					if def.HarvestSprite != "" && def.Rarity == "common" {
						return
					}
					for mobKey, mob := range monster.MonsterConfig.Monsters {
						if mob.Boss {
							continue
						}
						for _, drop := range config.GetLootTable(mobKey, false) {
							if drop.Type == "item" && drop.Key == key && drop.Chance >= .1 {
								return
							}
						}
					}
					t.Errorf("ordinary material %s has no regular non-boss source", key)
				})
			}
		}
	}
}

func TestAlchemyEveryMaterialCraftsIndependently(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	for recipeIndex, recipe := range config.GlobalAlchemy.Recipes {
		for choice, source := range recipe.Ingredients[1].Alternatives {
			for _, key := range source.Items {
				t.Run(fmt.Sprintf("%s/source%d/%s", recipe.Key, choice, key), func(t *testing.T) {
					g.party.Inventory = nil
					add := func(key string, n int) string {
						it, err := items.TryCreateItemFromYAML(key)
						if err != nil {
							t.Fatal(err)
						}
						it.Quantity = n
						g.party.AddItem(it)
						return it.Name
					}
					want := map[string]int{}
					for _, reserved := range []string{"clock_hand", "black_dragon_scale", "red_dragon_scale", "green_dragon_scale", "gold_dragon_scale"} {
						want[add(reserved, 3)] = 3
					}
					base := recipe.Ingredients[0].Alternatives[0]
					add(base.Items[0], base.Count)
					add(key, source.Count)
					before := append([]items.Item(nil), g.party.Inventory...)
					choices := []int{0, choice}
					// The book's preview and commit read the selection a legacy source choice migrates to.
					g.alchemy.Choices = map[string][]int{recipe.Key: choices}
					g.alchemy.Selections = nil
					selected := g.alchemySelection(&recipe)
					plan := (&UISystem{game: g}).alchemyPlan(&recipe, selected, 2)
					if plan.max != 1 || plan.err == nil || !reflect.DeepEqual(before, g.party.Inventory) {
						t.Fatal("batch preview disagrees with independent source or spends inventory")
					}
					if _, _, err := g.party.BrewSelected(c, &recipe, selected, 2); err == nil || !reflect.DeepEqual(before, g.party.Inventory) {
						t.Fatal("short batch did not fail atomically")
					}
					g.selectedRare, g.alchemyBatches, g.brewAnimation = recipeIndex, 1, nil
					g.alchemy.Choices = map[string][]int{recipe.Key: choices}
					g.alchemy.Selections = nil // Simulate loading the legacy source choice.
					if !g.brewSelectedRecipe() {
						t.Fatal(g.rareBookMessage)
					}
					count := g.brewAnimation.Count
					assertAlchemyReceipt(t, before, g.party.Inventory, g.brewAnimation.Ingredients)
					output, _ := config.GetItemDefinition(recipe.Output)
					want[output.Name] = character.AlchemyYield(c.SkillTier(character.SkillAlchemy), recipe.Family)
					got := map[string]int{}
					for _, item := range g.party.Inventory {
						got[item.Name] += item.Count()
					}
					if count != want[output.Name] || !reflect.DeepEqual(got, want) {
						t.Fatalf("selected material or currency consumption: got %v, want %v", got, want)
					}
				})
			}
		}
	}
}

// A mixed stock pays its authored count across different items; one short of
// it fails without spending anything. Through the book's brew at every tier.
func TestAlchemyHealthManaMixedScaleCostAtEveryMastery(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	add := func(key string, n int) {
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			t.Fatal(err)
		}
		it.Quantity = n
		g.party.AddItem(it)
	}
	for _, key := range []string{"health_potion", "mana_potion"} {
		recipeIndex := slices.IndexFunc(config.GlobalAlchemy.Recipes, func(r config.AlchemyRecipe) bool { return r.Key == key })
		if recipeIndex < 0 {
			t.Fatalf("%s recipe missing", key)
		}
		recipe := &config.GlobalAlchemy.Recipes[recipeIndex]
		base, stock := recipe.Ingredients[0].Alternatives[0], recipe.Ingredients[1].Alternatives[0]
		if len(stock.Items) < 2 || stock.Count < 2 {
			t.Fatalf("fixture: %s first stock must mix several items", key)
		}
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("%s/%d", key, tier), func(t *testing.T) {
				c.Skills[character.SkillAlchemy] = &character.Skill{Mastery: character.SkillMastery(tier)}
				g.party.Inventory = nil
				add(base.Items[0], base.Count)
				for i := 0; i < stock.Count-1; i++ {
					add(stock.Items[i%len(stock.Items)], 1)
				}
				// No saved choice: every group uses its first source.
				g.selectedRare, g.alchemyBatches, g.brewAnimation = recipeIndex, 1, nil
				g.alchemy.Choices, g.alchemy.Selections = nil, nil
				before := append([]items.Item(nil), g.party.Inventory...)
				if g.brewSelectedRecipe() || !reflect.DeepEqual(before, g.party.Inventory) {
					t.Fatalf("%d mixed items must not pay for a batch of %d", stock.Count-1, stock.Count)
				}
				add(stock.Items[(stock.Count-1)%len(stock.Items)], 1)
				if !g.brewSelectedRecipe() {
					t.Fatal(g.rareBookMessage)
				}
				yield := character.AlchemyYield(tier, recipe.Family)
				if g.brewAnimation.Count != yield || len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != yield {
					t.Fatalf("%d mixed stock + %s: count=%d inventory=%v, want %d", stock.Count, base.Items[0], g.brewAnimation.Count, g.party.Inventory, yield)
				}
			})
		}
	}
}
