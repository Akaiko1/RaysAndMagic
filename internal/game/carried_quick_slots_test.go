package game

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// Use the same transfer as the displayed quick-slot drop: the physical item
// must leave its bag, while the chosen active hero remains its sole owner.
func carryInQuickSlot(t *testing.T, g *MMGame, actor, slot int, it items.Item) {
	t.Helper()
	hero := g.party.Members[actor]
	hero.Inventory = append(hero.Inventory, it)
	g.dragSrc, g.dragInvOwner, g.dragInvIndex = dragFromInventory, hero, len(hero.Inventory)-1
	g.resolveQuickSlotDrop(actor, slot)
	if hero.QuickSlots[slot] == nil || hero.QuickSlots[slot].Name != it.Name {
		t.Fatal("item did not reach its physical quick slot")
	}
}

func TestQuickCarriedDoorKeys(t *testing.T) {
	for _, key := range []string{"ordinary_key", "skeleton_key"} {
		for _, state := range []string{"active", "other", "restored", "reserve", "captive", "removed"} {
			t.Run(key+"/"+state, func(t *testing.T) {
				npc := woodenDoorNPC()
				g := makeDoorGame(t, npc)
				idx := 0
				if state == "other" {
					idx = 1
				}
				hero := g.party.Members[idx]
				carryInQuickSlot(t, g, idx, 4, items.CreateItemFromYAML(key))
				if state == "restored" {
					hero = restoreCharacterSave(buildCharacterSave(hero))
					g.party.Members[idx] = hero
				}
				eligible := state != "reserve" && state != "captive" && state != "removed"
				if state == "reserve" || state == "captive" {
					g.party.Members = g.party.Members[1:]
					if state == "reserve" {
						g.party.Reserve = append(g.party.Reserve, hero)
					} else {
						g.party.Captive = append(g.party.Captive, hero)
					}
				}
				if state == "removed" {
					if len(g.availableDoorUnlocks(npc)) == 0 {
						t.Fatal("held quick key was not offered")
					}
					hero.QuickSlots[4] = nil
				}
				g.dialogNPC = npc
				g.openLockedDoor(npc, 0)
				if npc.Visited != eligible {
					t.Fatalf("opened=%v want=%v", npc.Visited, eligible)
				}
				if eligible && (hero.QuickSlots[4] == nil) != (key == "ordinary_key") {
					t.Fatal("door must consume ordinary key and retain master key")
				}
			})
		}
	}
}

func TestQuickCarriedBarter(t *testing.T) {
	for _, mode := range []string{"shop", "stock override", "mixed", "shortage", "reserve", "captive", "restored", "coins"} {
		t.Run(mode, func(t *testing.T) {
			entry := potionStock(3, 1)
			currency := "item:clock_hand"
			if mode == "stock override" || mode == "coins" {
				currency = ""
			}
			if mode == "stock override" {
				entry.CurrencyItem = "clock_hand"
			}
			g, _ := merchantBuyGame(t, currency, entry)
			g.party.Inventory = nil
			hero := g.party.Members[0]
			hero.Inventory = nil
			it := items.CreateItemFromYAML("clock_hand")
			it.Quantity = 4
			if mode == "shortage" {
				it.Quantity = 2
			}
			if mode == "mixed" {
				it.Quantity = 1
				g.party.Inventory = []items.Item{it}
				hero.Inventory = []items.Item{it}
				it.Quantity = 2
			}
			carryInQuickSlot(t, g, 0, 4, it)
			if mode == "restored" {
				hero = restoreCharacterSave(buildCharacterSave(hero))
				g.party.Members[0] = hero
			}
			if mode == "reserve" || mode == "captive" {
				g.party.Members = g.party.Members[1:]
				if mode == "reserve" {
					g.party.Reserve = append(g.party.Reserve, hero)
				} else {
					g.party.Captive = append(g.party.Captive, hero)
				}
			}
			if mode == "coins" {
				if len(g.merchantBagItems()) != 0 {
					t.Fatal("coin shop exposed quick-slot stock")
				}
				return
			}
			eligible := mode != "shortage" && mode != "reserve" && mode != "captive"
			if (g.merchantMaxUnits(entry) > 0) != eligible {
				t.Fatal("barter quote ignores quick-slot balance")
			}
			if g.buyMerchantUnits(entry, 1) != eligible {
				t.Fatal("barter payment disagrees with displayed budget")
			}
			if eligible {
				if hero.QuickSlots[4] == nil || hero.QuickSlots[4].Count() != 1 || len(hero.Inventory) != 0 || len(g.party.Inventory) != 1 {
					t.Fatal("barter did not consume bags then quick stock and deliver to shared")
				}
			} else if hero.QuickSlots[4].Count() != it.Count() {
				t.Fatal("failed barter consumed quick stock")
			}
		})
	}
}

func TestQuickCarriedCraftingAndCollection(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			g, hero := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Inventory, hero.Inventory = nil, nil
			leaf, carp, koi := items.CreateItemFromYAML("dawnleaf"), items.CreateItemFromYAML("carp_scale"), items.CreateItemFromYAML("koi_scale")
			carp.Quantity = 3
			carryInQuickSlot(t, g, 0, 0, leaf)
			carryInQuickSlot(t, g, 0, 2, carp)
			if !short {
				carryInQuickSlot(t, g, 0, 4, koi)
			}
			r := alchemyRecipeByKey("health_potion")
			g.alchemy.Selections = map[string]character.AlchemySelection{r.Key: {{"dawnleaf": true}, {"carp_scale": true, "koi_scale": true}}}
			g.selectedRare, g.alchemyBatches = 0, 1
			before := g.party.CarriedItems()
			preview := (&UISystem{game: g}).alchemyPlan(r, g.alchemySelection(r), 1)
			if g.brewSelectedRecipe() == short {
				t.Fatal("quick-slot brew availability is wrong")
			}
			if short {
				if !reflect.DeepEqual(before, g.party.CarriedItems()) {
					t.Fatal("failed brew consumed a quick-slot reagent")
				}
			} else {
				if preview.err != nil || len(g.party.Inventory) != 1 || hero.QuickSlots[0] != nil || hero.QuickSlots[2] != nil || hero.QuickSlots[4] != nil {
					t.Fatal("recipe plan/commit disagreed about physical quick stock")
				}
				units := 0
				for _, it := range g.brewAnimation.Ingredients {
					units += it.Count()
				}
				if units != 5 {
					t.Fatal("brew receipt omitted quick-slot ingredients")
				}
			}
			card := items.CreateItemFromYAML("wolf_card")
			carryInQuickSlot(t, g, 1, 3, card)
			indices := g.inventoryCardIndices()
			if len(indices) != 1 || !g.placeCardFromInventory(indices[0]) || g.party.Members[1].QuickSlots[3] != nil || g.cardCollectionItem(0).InstanceID != card.InstanceID {
				t.Fatal("card collection lost or duplicated quick-slot card")
			}
		})
	}
}

func TestQuickCarriedSaveRestore(t *testing.T) {
	h := newDisplayedModalHarness(t, 800, 680)
	g := h.g
	for i, key := range []string{"world_map", "ordinary_key", "black_dragon_statuette", "clock_hand"} {
		carryInQuickSlot(t, g, 0, i, items.CreateItemFromYAML(key))
	}
	for i := range 2 {
		hero := character.CreateCharacter("Absent", character.ClassKnight, g.config)
		it := items.CreateItemFromYAML("clock_hand")
		it.Quantity = 11 + i
		hero.QuickSlots[4] = &it
		if i == 0 {
			g.party.Reserve = append(g.party.Reserve, hero)
		} else {
			g.party.Captive = append(g.party.Captive, hero)
		}
	}
	wm := world.GlobalWorldManager
	save := g.buildSave(wm)
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"world_map", "ordinary_key", "black_dragon_statuette", "clock_hand"} {
		it := items.CreateItemFromYAML(key)
		if g.party.CountItemsByName(it.Name) != 1 {
			t.Fatalf("restored %s is not available exactly once", key)
		}
	}
	g.useQuickSlot(0, 0)
	if !g.mapOverlayOpen || g.party.Members[0].QuickSlots[0] == nil {
		t.Fatal("carried rule broke reusable quick-slot map")
	}
}
