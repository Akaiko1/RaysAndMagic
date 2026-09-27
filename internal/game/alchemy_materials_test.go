package game

import (
	"encoding/json"
	"fmt"
	"reflect"
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
					if g.party.MaxAlchemyBatches(r, g.alchemyChoices(r)) != 1 {
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

func TestAlchemyMaterialProvenanceAndQuantities(t *testing.T) {
	rareClassGame(t, character.ClassAlchemist, false)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, recipe := range config.GlobalAlchemy.Recipes {
		for _, source := range recipe.Ingredients[1].Alternatives {
			for _, key := range source.Items {
				t.Run(recipe.Key+"/"+key, func(t *testing.T) {
					def, _ := config.GetItemDefinition(key)
					want := 1 // Finite, sparse and valuable drops pay once per batch.
					switch key {
					case "carp_scale", "koi_scale", "rainbow_salmon_scale":
						want = 4
					case "wolf_pelt", "spider_silk", "lemur_fur", "rabbit_pelt", "dawnleaf", "mooncap", "bitterroot", "embercap":
						want = 2
					}
					if source.Count != want {
						t.Errorf("batch costs %d %s, want %d for its availability", source.Count, key, want)
					}
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
					if g.party.MaxAlchemyBatches(&recipe, choices) != 1 || !reflect.DeepEqual(before, g.party.Inventory) {
						t.Fatal("batch preview disagrees with independent source or spends inventory")
					}
					if _, _, err := g.party.Brew(c, &recipe, choices, 2); err == nil || !reflect.DeepEqual(before, g.party.Inventory) {
						t.Fatal("short batch did not fail atomically")
					}
					g.selectedRare, g.alchemyBatches, g.brewAnimation = recipeIndex, 1, nil
					g.alchemy.Choices = map[string][]int{recipe.Key: choices}
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

func TestAlchemyHealthManaMixedScaleCostAtEveryMastery(t *testing.T) {
	g, c := rareClassGame(t, character.ClassAlchemist, false)
	for _, base := range []struct{ recipe, herb string }{
		{"health_potion", "dawnleaf"},
		{"mana_potion", "mooncap"},
	} {
		for tier, yield := range []int{2, 3, 4, 6} {
			t.Run(fmt.Sprintf("%s/%d", base.recipe, tier), func(t *testing.T) {
				c.Skills[character.SkillAlchemy] = &character.Skill{Mastery: character.SkillMastery(tier)}
				g.party.Inventory = nil
				for _, key := range []string{base.herb, "carp_scale", "koi_scale", "rainbow_salmon_scale"} {
					it, _ := items.TryCreateItemFromYAML(key)
					g.party.AddItem(it)
				}
				recipe := config.AlchemyRecipeByKey(base.recipe)
				before := append([]items.Item(nil), g.party.Inventory...)
				if _, _, err := g.party.Brew(c, recipe, []int{0, 0}, 1); err == nil || !reflect.DeepEqual(before, g.party.Inventory) {
					t.Fatal("three scales must not pay for a batch")
				}
				it, _ := items.TryCreateItemFromYAML("carp_scale")
				g.party.AddItem(it)
				count, _, err := g.party.Brew(c, recipe, []int{0, 0}, 1)
				if err != nil || count != yield || len(g.party.Inventory) != 1 || g.party.Inventory[0].Count() != yield {
					t.Fatalf("four mixed scales + one %s: count=%d inventory=%v error=%v", base.herb, count, g.party.Inventory, err)
				}
			})
		}
	}
}
