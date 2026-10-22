package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestInventoryReviewPickedFragmentDisplayedDrop(t *testing.T) {
	for _, personal := range []bool{false, true} {
		for _, destination := range []string{"empty", "occupied", "frame", "same bag", "outside", "modal", "quick slot"} {
			t.Run(fmt.Sprintf("personal=%v/%s", personal, destination), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, ui := h.g, h.ui
				g.menuOpen, g.currentTab = true, TabInventory
				hero := g.party.Members[0]
				var source, target *character.MMCharacter
				target = hero
				if personal {
					source, target = hero, nil
					hero.Inventory, g.party.Inventory = g.party.Inventory, nil
				}
				bag, dst := g.party.Bag(source), g.party.Bag(target)
				if destination == "occupied" {
					it := bag.Items()[0]
					it.Quantity = 4
					dst.Add(it)
				}
				l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content, 1)
				srcRect, dstRect := l.grid, l.personalGrid
				if personal {
					srcRect, dstRect = dstRect, srcRect
				}
				sx, sy := personalBagCell(srcRect, 0)
				h.clicks(true, sx, sy, 1)
				cx, cy := min(ui.inventoryContextX, 1024-166-4), min(ui.inventoryContextY, 768-72-4)
				h.clicks(false, cx+20, cy+36, 1)
				if !ui.stackSplitPicker.open || ui.stackSplitPicker.owner != source {
					t.Fatal("displayed Split action lost the physical source")
				}
				ui.stackSplitSetQuantity(2)
				take := stackSplitPickerLayout(stackSplitPickerRect(1024, 768)).take
				h.clicks(false, take.Min.X+4, take.Min.Y+4, 1)
				if !g.dragPickedUp || !ui.inventoryInputBlocked() {
					t.Fatal("picker did not claim the next click")
				}
				dx, dy := personalBagCell(dstRect, 0)
				switch destination {
				case "frame":
					dx, dy = dstRect.x+3, dstRect.y+3
				case "same bag":
					dx, dy = personalBagCell(srcRect, 1)
				case "outside":
					dx, dy = 1, 1
				case "modal":
					g.mapOverlayOpen = true
				case "quick slot":
					_, slots := quickSlotRects(l.quickSlots.x, l.quickSlots.y, l.quickSlots.w)
					dx, dy = slots[0].Min.X+5, slots[0].Min.Y+5
				}
				fp := installFakePointer(t)
				fp.moveTo(dx, dy)
				fp.press()
				h.pointerStep()
				wantSource, wantTarget := 3, 2
				switch destination {
				case "occupied":
					wantTarget = 6
				case "same bag", "outside", "modal":
					wantSource, wantTarget = 5, 0
				case "quick slot":
					wantTarget = 0
					if hero.QuickSlots[0] == nil || hero.QuickSlots[0].Count() != 2 {
						t.Fatal("picked fragment did not reach the displayed quick slot")
					}
				}
				if inventoryBagUnits(bag) != wantSource || inventoryBagUnits(dst) != wantTarget || destination != "modal" && g.dragPickedUp {
					t.Fatalf("drop lost units or ownership: source=%d target=%d picked=%v", inventoryBagUnits(bag), inventoryBagUnits(dst), g.dragPickedUp)
				}
				if destination == "same bag" { // free stacks: the fragment keeps a stack of its own
					var stacks []int
					for _, it := range bag.Items() {
						if it.Name == bag.Items()[0].Name {
							stacks = append(stacks, it.Count())
						}
					}
					if len(stacks) != 2 || stacks[0] != 3 || stacks[1] != 2 {
						t.Fatalf("a fragment dropped in its own bag: stacks %v, want [3 2]", stacks)
					}
				}
			})
		}
	}
}

func TestInventoryReviewDragonStatueCarriedStock(t *testing.T) {
	loadTestConfig(t) // Statue spawning needs the monster catalog even in isolation.
	for _, quick := range []bool{false, true} {
		for _, source := range []string{"shared", "personal", "reserve", "captive", "missing", "restored", "moved", "removed", "locked"} {
			t.Run(fmt.Sprintf("%s/quick=%v", source, quick), func(t *testing.T) {
				g, qm := bootQuestGiverTest(t)
				g.world = newTestWorldSized(g.config, 12, 12)
				g.collisionSystem.UpdateTileChecker(g.world)
				g.camera.X, g.camera.Y = 192, 192
				hero := g.party.Members[0]
				g.party.Inventory, hero.Inventory = nil, nil
				it := items.CreateItemFromYAML("black_dragon_statuette")
				if source == "shared" || source == "moved" {
					g.party.AddItem(it)
					g.party.AddItem(it)
				} else if source != "missing" {
					hero.Inventory = []items.Item{it, it}
				}
				if quick && len(hero.Inventory) > 0 {
					hero.Inventory = nil
					carryInQuickSlot(t, g, 0, 0, it)
					carryInQuickSlot(t, g, 0, 4, it)
				}
				if source == "reserve" || source == "captive" {
					if source == "reserve" {
						g.party.Reserve = append(g.party.Reserve, hero)
					} else {
						g.party.Captive = append(g.party.Captive, hero)
					}
					g.party.Members = g.party.Members[1:]
				}
				if source == "restored" {
					g.party.Members[0] = restoreCharacterSave(buildCharacterSave(hero))
				}
				if source != "locked" {
					if err := qm.ActivateQuest("dragon_slayer"); err != nil {
						t.Fatal(err)
					}
				}
				npc, err := character.CreateNPCFromConfig("dragon_statue_black", g.camera.X, g.camera.Y)
				if err != nil {
					t.Fatal(err)
				}
				ih := &InputHandler{game: g}
				ih.buildStatueChoices(npc)
				offered := false
				for _, choice := range npc.DialogueData.Choices {
					offered = offered || choice.Action == "summon_dragon"
				}
				eligible := source != "reserve" && source != "captive" && source != "missing" && source != "locked"
				if offered != eligible {
					t.Fatalf("offer=%v, want %v", offered, eligible)
				}
				if source == "moved" {
					g.party.Bag().MoveTo(g.party.Bag(hero), 0, 1)
					g.party.Bag().MoveTo(g.party.Bag(hero), 0, 1)
				}
				if source == "removed" {
					hero.QuickSlots = [character.QuickSlotCount]*items.Item{}
					hero.Inventory = nil
					eligible = false
				}
				stock, monsters := g.party.CountItemsByName(it.Name), len(g.world.Monsters)
				ih.summonDragonFromStatue(npc, 0)
				if npc.Visited != eligible || len(g.world.Monsters) != monsters+boolInt(eligible) || g.party.CountItemsByName(it.Name) != stock-boolInt(eligible) {
					t.Fatal("rite did not pay exactly one carried unit for one dragon")
				}
				ih.summonDragonFromStatue(npc, 0)
				if len(g.world.Monsters) != monsters+boolInt(eligible) || g.party.CountItemsByName(it.Name) != stock-boolInt(eligible) {
					t.Fatal("spent statue accepted the rite twice")
				}
			})
		}
	}

}

func TestInventoryReviewAccessLabels(t *testing.T) {
	loadTestConfig(t)
	text := uitext.Text("item.auto_drink_details", 1.0)
	if !strings.Contains(text, "own quick slots, then own bag, then shared bag") {
		t.Fatalf("autodrink source order missing: %s", text)
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, currency := range []string{"item:clock_hand", character.CurrencyArenaPoints} {
			npc := &character.NPC{Currency: currency}
			label := merchantBagHeaderLabel(npc, merchantBuysForGold(npc))
			if !strings.Contains(label, "drop to buy") || !strings.Contains(label, "no selling") || uiTextWidth(label) > merchantGridW {
				t.Fatalf("non-selling shop lost its usable header: %q (%dpx of %d)", label, uiTextWidth(label), merchantGridW)
			}
		}
	})
}

// The placement rule in the inventory screen, through real gestures: onto a
// matching stack a drop merges, the free tail keeps a stack of its own, a
// whole stack trades cells with another item (across bags too), part of a
// stack (one unit with Shift) never displaces anything, and a quick-slot item
// merges onto the stack it is dropped on or else lands at the free tail. Each
// row starts from shared [P5 S P2], personal [D] and quick slot P2: P potions,
// S a sword, D a dagger.
func TestInventoryDropsFollowTheCell(t *testing.T) {
	type cell struct {
		bag string // "shared", "personal" or "quick"
		idx int
	}
	unchangedShared, unchangedPersonal := []string{"P5", "S", "P2"}, []string{"D"}
	for _, tc := range []struct {
		name                     string
		from, to                 cell
		shift                    bool
		wantShared, wantPersonal []string
		slotEmptied              bool
	}{
		{"shift-drag to the free tail", cell{"shared", 0}, cell{"shared", 3}, true, []string{"P4", "S", "P2", "P1"}, unchangedPersonal, false},
		{"shift-drag onto the matching stack", cell{"shared", 0}, cell{"shared", 2}, true, []string{"P4", "S", "P3"}, unchangedPersonal, false},
		{"shift-drag onto another item moves nothing", cell{"shared", 0}, cell{"shared", 1}, true, unchangedShared, unchangedPersonal, false},
		{"whole stack swaps with another item", cell{"shared", 0}, cell{"shared", 1}, false, []string{"S", "P5", "P2"}, unchangedPersonal, false},
		{"whole stack swaps with another bag's item", cell{"shared", 0}, cell{"personal", 0}, false, []string{"D", "S", "P2"}, []string{"P5"}, false},
		{"shift-drag onto another bag's item moves nothing", cell{"shared", 0}, cell{"personal", 0}, true, unchangedShared, unchangedPersonal, false},
		{"shift-drag to another bag's free tail", cell{"shared", 0}, cell{"personal", 1}, true, []string{"P4", "S", "P2"}, []string{"D", "P1"}, false},
		{"quick-slot item merges onto the stack it lands on", cell{"quick", 0}, cell{"shared", 2}, false, []string{"P5", "S", "P4"}, unchangedPersonal, true},
		{"quick-slot item to the free tail", cell{"quick", 0}, cell{"shared", 3}, false, []string{"P5", "S", "P2", "P2"}, unchangedPersonal, true},
		{"quick-slot item onto another item lands at the free tail", cell{"quick", 0}, cell{"shared", 1}, false, []string{"P5", "S", "P2", "P2"}, unchangedPersonal, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab, g.selectedChar = true, TabInventory, 0
			item := func(name string, typ items.ItemType, n int) items.Item {
				it := items.Item{Name: name, Type: typ, Quantity: n}
				items.EnsureInstanceID(&it)
				return it
			}
			hero := g.party.Members[0]
			g.party.Inventory = []items.Item{item("Health Potion", items.ItemConsumable, 5), item("Iron Sword", items.ItemWeapon, 0), item("Health Potion", items.ItemConsumable, 2)}
			hero.Inventory = []items.Item{item("Iron Dagger", items.ItemWeapon, 0)}
			slot := item("Health Potion", items.ItemConsumable, 2)
			hero.QuickSlots = [character.QuickSlotCount]*items.Item{&slot}
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content, 1)
			_, quick := quickSlotRects(l.quickSlots.x, l.quickSlots.y, l.quickSlots.w)
			at := func(c cell) (int, int) {
				switch c.bag {
				case "personal":
					return personalBagCell(l.personalGrid, c.idx)
				case "quick":
					r := quick[c.idx]
					return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
				}
				return personalBagCell(l.grid, c.idx)
			}
			x0, y0 := at(tc.from)
			x1, y1 := at(tc.to)
			fp := installFakePointer(t)
			fp.shift = tc.shift
			fp.moveTo(x0, y0)
			fp.press()
			h.pointerStep()
			fp.hold()
			fp.moveTo((x0+x1)/2, (y0+y1)/2)
			h.pointerStep()
			fp.moveTo(x1, y1)
			h.pointerStep()
			fp.release()
			h.pointerStep()
			fp.idle()
			h.pointerStep()
			labels := func(bag []items.Item) []string {
				var out []string
				for _, it := range bag {
					if it.Stackable() {
						out = append(out, fmt.Sprintf("P%d", it.Count()))
					} else {
						out = append(out, strings.Fields(it.Name)[1][:1])
					}
				}
				return out
			}
			shared, personal := labels(g.party.Inventory), labels(hero.Inventory)
			if fmt.Sprint(shared) != fmt.Sprint(tc.wantShared) || fmt.Sprint(personal) != fmt.Sprint(tc.wantPersonal) || (hero.QuickSlots[0] == nil) != tc.slotEmptied {
				t.Fatalf("shared %v personal %v slot emptied=%v; want %v, %v and %v", shared, personal, hero.QuickSlots[0] == nil, tc.wantShared, tc.wantPersonal, tc.slotEmptied)
			}
			refused := slices.Contains(g.GetCombatMessages(), uitext.Text("stack.part_needs_room"))
			if refused != strings.HasSuffix(tc.name, "moves nothing") {
				t.Fatalf("refusal message shown=%v", refused)
			}
		})
	}
}
