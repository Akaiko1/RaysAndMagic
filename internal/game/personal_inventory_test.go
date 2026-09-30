package game

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/world"
)

func personalBagCell(r layoutRect, slot int) (int, int) {
	x, y, w, h := scaleInventorySourceRect(r.x, r.y, r.w, r.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[slot])
	return x + w/2, y + h/2
}

func TestPersonalInventoryDisplayedTransfers(t *testing.T) {
	for _, res := range [][2]int{{800, 600}, {800, 680}, {1024, 768}, {1920, 1080}} {
		for _, direction := range []string{"shared to personal", "personal to shared", "onto existing stack", "outside cancel"} {
			t.Run(fmt.Sprintf("%v/%s", res, direction), func(t *testing.T) {
				h := newDisplayedModalHarness(t, res[0], res[1])
				g := h.g
				g.menuOpen, g.currentTab = true, TabInventory
				hero := g.party.Members[0]
				item := g.party.Inventory[0]
				hero.Inventory = nil
				layout := computeInventoryContentLayout(computeTabbedMenuLayout(res[0], gameplayViewportBottom(g)).content)
				src, dst := layout.grid, layout.personalGrid
				var source, target *character.MMCharacter
				target = hero
				if direction == "personal to shared" {
					g.party.Inventory = nil
					hero.Inventory = []items.Item{item}
					src, dst = dst, src
					source, target = hero, nil
				}
				if direction == "onto existing stack" {
					extra := item
					extra.Quantity = 2
					extra.InstanceID = 99
					hero.Inventory = []items.Item{extra}
				}
				sx, sy := personalBagCell(src, 0)
				dx, dy := personalBagCell(dst, 0)
				if direction == "outside cancel" {
					dx, dy = 1, 1
				}
				fp := installFakePointer(t)
				fp.moveTo(sx, sy)
				fp.press()
				h.pointerStep()
				if g.dragSrc != dragFromInventory || g.dragInvOwner != source {
					t.Fatal("displayed source captured the wrong physical bag")
				}
				fp.hold()
				fp.moveTo(dx, dy)
				h.pointerStep()
				fp.release()
				h.pointerStep()
				if g.dragSrc != dragNone {
					t.Fatal("released drag survived")
				}
				if direction == "outside cancel" {
					if inventoryBagUnits(g.party.Bag()) != 5 || len(hero.Inventory) != 0 {
						t.Fatal("cancel changed ownership")
					}
					return
				}
				want := 5
				if direction == "onto existing stack" {
					want = 7
				}
				if inventoryBagUnits(g.party.Bag(source)) != 0 || inventoryBagUnits(g.party.Bag(target)) != want || len(g.party.Bag(target).Items()) != 1 {
					t.Fatal("transfer lost, duplicated, or failed to merge items")
				}
				if direction != "onto existing stack" && g.party.Bag(target).Items()[0].InstanceID != item.InstanceID {
					t.Fatal("transfer changed physical identity")
				}
			})
		}
	}
}

func TestPersonalInventoryIndependentWheelAndFiltering(t *testing.T) {
	h := newDisplayedModalHarness(t, 800, 680)
	g := h.g
	g.menuOpen, g.currentTab = true, TabInventory
	hero := g.party.Members[0]
	for _, owner := range []*character.MMCharacter{nil, hero} {
		for i := 0; i < 34; i++ {
			g.party.Bag(owner).Add(items.Item{Name: fmt.Sprintf("Sword %d", i), Type: items.ItemWeapon})
		}
	}
	l := computeInventoryContentLayout(computeTabbedMenuLayout(800, gameplayViewportBottom(g)).content)
	fp := installFakePointer(t)
	old := pointerWheel
	t.Cleanup(func() { pointerWheel = old })
	pointerWheel = func() (float64, float64) { return 0, -1 }
	for _, r := range []layoutRect{l.personalGrid, l.personalGrid, l.grid} {
		fp.moveTo(r.x+10, r.y+10)
		h.pointerStep()
	}
	if h.ui.personalInventoryPage != 2 || h.ui.inventoryPage != 1 {
		t.Fatalf("pages coupled: personal=%d shared=%d", h.ui.personalInventoryPage, h.ui.inventoryPage)
	}
	h.ui.setInventoryTab(inventoryTabOwning(items.Item{Type: items.ItemWeapon}))
	if h.ui.personalInventoryPage != 2 || h.ui.inventoryPage != 0 {
		t.Fatal("shared filter changed the personal page or retained its own stale page")
	}
	if len(g.inventoryViewIndices(h.ui.personalInventoryTab, hero)) != 34 || len(g.inventoryViewIndices(h.ui.inventoryTab)) != 34 {
		t.Fatal("filter used wrong physical indices")
	}
}

func TestPersonalInventoryDoubleClickBindsBag(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.menuOpen, g.currentTab = true, TabInventory
	ch := g.party.Members[0]
	ch.HitPoints = 1
	it := items.CreateItemFromYAML("health_potion")
	it.Quantity = 3
	g.party.Inventory = []items.Item{it}
	ch.Inventory = []items.Item{it}
	l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
	sx, sy := personalBagCell(l.grid, 0)
	px, py := personalBagCell(l.personalGrid, 0)
	h.clicks(false, sx, sy, 1)
	h.clicks(false, px, py, 1)
	if ch.Inventory[0].Count() != 3 || g.party.Inventory[0].Count() != 3 {
		t.Fatal("clicks on different bags formed a double click")
	}
	h.clicks(false, px, py, 1)
	if ch.Inventory[0].Count() != 2 || g.party.Inventory[0].Count() != 3 || ch.HitPoints <= 1 {
		t.Fatal("personal double-click used the wrong source")
	}
}

func TestPersonalInventorySplitAndEquipmentSources(t *testing.T) {
	for _, personal := range []bool{false, true} {
		t.Run(fmt.Sprint(personal), func(t *testing.T) {
			g, ui := merchantDragGame(t)
			g.dialogActive = false
			g.menuOpen = true
			ch := g.party.Members[0]
			var owner *character.MMCharacter
			if personal {
				owner = ch
				ch.Inventory = g.party.Inventory
				g.party.Inventory = nil
			}
			bag := g.party.Bag(owner)
			ui.openStackSplitPicker(stackSplitPickerInventory, 0, bag.Items()[0], owner)
			ui.stackSplitPicker.quantity = 2
			ui.stackSplitConfirm()
			if g.dragInvOwner != owner || g.dragSplitQuantity != 2 {
				t.Fatal("split lost source")
			}
			var dst *character.MMCharacter
			if owner == nil {
				dst = ch
			}
			ui.transferInventoryDrag(dst)
			if inventoryBagUnits(bag) != 3 || inventoryBagUnits(g.party.Bag(dst)) != 2 {
				t.Fatal("split crossed bags incorrectly")
			}
			sword := items.CreateWeaponFromYAML("iron_sword")
			old := ch.Equipment[items.SlotMainHand]
			bag.Add(sword)
			if !g.equipPartyItemFromInventory(len(bag.Items())-1, 0, owner) {
				t.Fatal("cannot equip from bag")
			}
			found := false
			for _, it := range bag.Items() {
				if it.Name == old.Name {
					found = true
				}
			}
			if old.Name != "" && !found {
				t.Fatal("displaced equipment returned to the wrong bag")
			}
			g.unequipPartyItemToInventory(items.SlotMainHand, 0, owner)
			found = false
			for _, it := range bag.Items() {
				if it.Name == sword.Name {
					found = true
				}
			}
			if !found {
				t.Fatal("unequip lost destination")
			}
		})
	}
}

func TestPersonalInventoryMerchantBoundary(t *testing.T) {
	for _, mode := range []string{"coins", "barter", "mixed", "shortage", "reserve"} {
		t.Run(mode, func(t *testing.T) {
			entry := potionStock(3, 1)
			currency := "item:clock_hand"
			if mode == "coins" || mode == "mixed" {
				currency = ""
			}
			if mode == "mixed" {
				entry.CurrencyItem = "clock_hand"
			}
			g, _ := merchantBuyGame(t, currency, entry)
			material := items.CreateItemFromYAML("clock_hand")
			material.Quantity = 1
			g.party.AddItem(material)
			hero := g.party.Members[0]
			material.Quantity = 3
			hero.Inventory = []items.Item{material}
			if mode == "reserve" {
				g.party.Reserve = append(g.party.Reserve, hero)
				g.party.Members = g.party.Members[1:]
			}
			if mode == "shortage" {
				hero.Inventory[0].Quantity = 1
			}
			if mode == "coins" {
				if len(g.merchantBagItems()) != 1 {
					t.Fatal("coin shop exposes personal stock")
				}
				if g.sellInventoryUnits(1, 1) || hero.Inventory[0].Count() != 3 {
					t.Fatal("coin shop can sell personal stock")
				}
				return
			}
			before := g.party.CountItemsByName(material.Name)
			success := mode == "barter" || mode == "mixed"
			if got := g.buyMerchantUnits(entry, 1); got != success {
				t.Fatalf("purchase=%v expected=%v", got, success)
			}
			if success {
				if g.party.CountItemsByName(material.Name) != before-3 || hero.Inventory[0].Count() != 1 || len(g.party.Inventory) != 1 || g.party.Inventory[0].Name != entry.Item.Name {
					t.Fatal("barter did not consume shared then active personal stock and deliver to shared")
				}
			} else if g.party.CountItemsByName(material.Name) != before || entry.Quantity != 1 {
				t.Fatal("failed barter consumed stock")
			}
		})
	}
}

func TestPersonalInventoryAlchemyAcrossBags(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			g, ch := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Inventory = nil
			ch.Inventory = nil
			other := character.CreateCharacter("Carrier", character.ClassKnight, g.config)
			g.party.Members = append(g.party.Members, other)
			for _, s := range []struct {
				owner *character.MMCharacter
				key   string
				n     int
			}{{nil, "carp_scale", 3}, {ch, "dawnleaf", 1}, {other, "koi_scale", 1}} {
				it := items.CreateItemFromYAML(s.key)
				it.Quantity = s.n
				g.party.Bag(s.owner).Add(it)
			}
			if short {
				other.Inventory = nil
			}
			r := config.AlchemyRecipeByKey("health_potion")
			g.alchemy.Selections = map[string]character.AlchemySelection{r.Key: {{"dawnleaf": true}, {"carp_scale": true, "koi_scale": true}}}
			g.selectedRare, g.alchemyBatches = 0, 1
			before := append([]items.Item(nil), g.party.CarriedItems()...)
			preview := (&UISystem{game: g}).alchemyPlan(r, g.alchemySelection(r), 1)
			if got := g.brewSelectedRecipe(); got == short {
				t.Fatalf("brew=%v short=%v: %s", got, short, g.rareBookMessage)
			}
			if short {
				if !reflect.DeepEqual(before, g.party.CarriedItems()) {
					t.Fatal("failed brew spent cross-bag ingredients")
				}
				return
			}
			if preview.err != nil || len(ch.Inventory) != 0 || len(other.Inventory) != 0 || len(g.party.Inventory) != 1 || g.party.Inventory[0].Name == before[0].Name {
				t.Fatal("cross-bag recipe or shared output failed")
			}
			receipt := 0
			for _, it := range g.brewAnimation.Ingredients {
				receipt += it.Count()
			}
			if receipt != 5 {
				t.Fatal("receipt omitted personal ingredients")
			}
		})
	}
}

func TestPersonalInventoryAutomaticPotionIsolation(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"personal", "shared", "other", "priority"} {
			t.Run(fmt.Sprintf("%v/%s", tb, source), func(t *testing.T) {
				g, _, ch, _ := sniperFixture(t, tb)
				ch.MaxHitPoints, ch.HitPoints = 100, 1
				g.party.Inventory = nil
				ch.Inventory = nil
				other := character.CreateCharacter("Other", character.ClassKnight, g.config)
				g.party.Members = append(g.party.Members, other)
				it := items.CreateItemFromYAML("health_potion")
				it.Quantity = 3
				switch source {
				case "personal":
					ch.Inventory = []items.Item{it}
				case "shared":
					g.party.Inventory = []items.Item{it}
				case "other":
					other.Inventory = []items.Item{it}
				case "priority":
					ch.Inventory = []items.Item{it}
					g.party.Inventory = []items.Item{it}
				}
				g.updateAutomaticConsumables()
				if source == "other" {
					if ch.HitPoints != 1 || other.Inventory[0].Count() != 3 {
						t.Fatal("auto-drink stole from another hero")
					}
					return
				}
				if ch.HitPoints <= 1 {
					t.Fatal("auto-drink ignored available stock")
				}
				if source == "priority" && (ch.Inventory[0].Count() != 2 || g.party.Inventory[0].Count() != 3) {
					t.Fatal("shared bag drank before own bag")
				}
			})
		}
	}
}

func TestPersonalInventorySaveAndStashAllRosters(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.party.Members = g.party.Members[:1]
	g.party.Reserve = []*character.MMCharacter{character.CreateCharacter("Reserve", character.ClassKnight, g.config)}
	g.party.Captive = []*character.MMCharacter{character.CreateCharacter("Captive", character.ClassKnight, g.config)}
	for i, ch := range []*character.MMCharacter{g.party.Members[0], g.party.Reserve[0], g.party.Captive[0]} {
		it := items.CreateItemFromYAML("health_potion")
		it.Quantity = 5
		it.InstanceID = uint64(100 + i)
		ch.Inventory = []items.Item{it}
	}
	save := g.buildSave(world.GlobalWorldManager)
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GameSave
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	h.ui.inventoryContextOpen, h.ui.inventoryContextOwner = true, g.party.Members[0]
	h.ui.stackSplitPicker = stackSplitPickerState{open: true, owner: g.party.Members[0]}
	g.healPickerOpen, g.pickerInventoryOwner = true, g.party.Members[0]
	g.restoreSavedParty(&decoded)
	if h.ui.inventoryContextOpen || h.ui.stackSplitPicker.open || g.healPickerOpen || g.pickerInventoryOwner != nil {
		t.Fatal("load retained an old bag interaction")
	}
	g.stash = &stash.Stash{}
	for i, ch := range []*character.MMCharacter{g.party.Members[0], g.party.Reserve[0], g.party.Captive[0]} {
		if len(ch.Inventory) != 1 || ch.Inventory[0].Count() != 5 || ch.Inventory[0].InstanceID != uint64(100+i) {
			t.Fatal("personal bag lost in save round trip")
		}
		g.stash.Slots[i] = ch.Inventory[0]
		g.stash.Slots[i].Quantity = 2
	}
	g.reconcilePartyAgainstStash()
	for _, ch := range []*character.MMCharacter{g.party.Members[0], g.party.Reserve[0], g.party.Captive[0]} {
		if len(ch.Inventory) != 1 || ch.Inventory[0].Count() != 3 {
			t.Fatal("stash duplication in personal roster")
		}
	}
	old := g.party.Members[0]
	if !g.party.SwapActiveReserve(0, 0) || g.party.Reserve[0] != old || len(old.Inventory) != 1 {
		t.Fatal("personal bag did not travel with roster swap")
	}
	legacy := decoded.Party.Members[0]
	legacy.Inventory = nil
	if len(restoreCharacterSave(legacy).Inventory) != 0 {
		t.Fatal("legacy save invented personal stock")
	}
}

func TestPersonalInventoryGeometryAllResolutions(t *testing.T) {
	mw, mh := MinimumWindowSize()
	for _, size := range [][2]int{{mw, mh}, {800, 600}, {1024, 768}, {1280, 720}, {1280, 800}, {1366, 768}, {1440, 900}, {1600, 900}, {1680, 1050}, {1920, 1080}, {1920, 1200}, {2560, 1440}, {3440, 1440}, {3840, 2160}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			region, boxes := inventoryLayoutBoxes(size[0], size[1])
			l := computeInventoryContentLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottomWithPartyHUD(size[1])).content)
			for i, grid := range []layoutRect{l.paper, l.personalGrid, l.grid} {
				if i == 0 {
					continue
				}
				for _, source := range inventoryGridSlots {
					x, y, w, h := scaleInventorySourceRect(grid.x, grid.y, grid.w, grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, source)
					minSlot := 36
					if size[1] < mh {
						minSlot = 30
					}
					if w < minSlot || h < minSlot || !namedLayoutBox("grid", grid).contains(uiBox{"cell", x, y, w, h}) {
						t.Fatalf("invisible or unreadable bag cell: %dx%d", w, h)
					}
				}
			}
			boxes = append(boxes, namedLayoutBox("quick label", layoutRect{l.quickSlots.x, l.quickSlots.y - quickSlotTabLabelSpace, l.quickSlots.w, quickSlotTabLabelH}))
			assertNoCollisions(t, "personal inventory", region, boxes)
		})
	}
}

// On every frame an interface size can produce, the paperdoll uses the room
// the frame leaves it: it reaches the bottom of the panel, or the three
// columns already span the width. The bag column stays inside the panel and
// never outgrows the doll. A 1080-unit frame keeps its known layout.
func TestInventoryArtUsesTheFrame(t *testing.T) {
	var logical [][2]int
	for _, size := range [][2]int{{1024, 768}, {1280, 720}, {1366, 768}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3440, 1440}} {
		w, h := logicalScreenSize(size[0], size[1], 1)
		logical = append(logical, [2]int{w, h})
	}
	for _, size := range withInterfaceFrames(t, logical) {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			content := computeTabbedMenuLayout(size[0], gameplayViewportBottomWithPartyHUD(size[1])).content
			l := computeInventoryContentLayout(content)
			fillsHeight := l.paper.bottom() >= content.bottom()-1
			spansWidth := l.grid.right()-l.paper.x >= content.w-32-3
			if !fillsHeight && !spansWidth {
				t.Fatalf("paperdoll %v stops %d short of the panel bottom with %d units of width unused",
					l.paper, content.bottom()-l.paper.bottom(), content.w-32-(l.grid.right()-l.paper.x))
			}
			if l.quickSlots.bottom() > content.bottom() || l.grid.h > l.paper.h {
				t.Fatalf("bag column %v / quick slots %v against doll %v in %v", l.grid, l.quickSlots, l.paper, content)
			}
			if size == [2]int{1920, 1080} && (l.paper.w != 496 || l.paper.h != 745 || l.grid.w != 496 || l.textScale != readingTextScale) {
				t.Fatalf("Normal at 1080 units changed: doll %v grid %v text x%d", l.paper, l.grid, l.textScale)
			}
		})
	}
}

func TestPersonalInventoryQuickSlotRoundTrip(t *testing.T) {
	for _, personal := range []bool{false, true} {
		t.Run(fmt.Sprint(personal), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			ch := g.party.Members[0]
			g.menuOpen, g.currentTab = true, TabInventory
			var owner *character.MMCharacter
			if personal {
				owner = ch
				ch.Inventory = g.party.Inventory
				g.party.Inventory = nil
			}
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			grid := l.grid
			if personal {
				grid = l.personalGrid
			}
			sx, sy := personalBagCell(grid, 0)
			_, slots := quickSlotRects(l.quickSlots.x, l.quickSlots.y, l.quickSlots.w)
			dx, dy := slots[0].Min.X+3, slots[0].Min.Y+3
			fp := installFakePointer(t)
			gesture := func(sx, sy, dx, dy int) {
				fp.moveTo(sx, sy)
				fp.press()
				h.pointerStep()
				fp.hold()
				fp.moveTo(dx, dy)
				h.pointerStep()
				fp.release()
				h.pointerStep()
			}
			gesture(sx, sy, dx, dy)
			if ch.QuickSlots[0] == nil || ch.QuickSlots[0].Count() != 5 || len(g.party.Bag(owner).Items()) != 0 {
				t.Fatal("bag to quick slot lost source")
			}
			fp.idle()
			h.pointerStep()
			gesture(dx, dy, sx, sy)
			if ch.QuickSlots[0] != nil || inventoryBagUnits(g.party.Bag(owner)) != 5 {
				t.Fatal("quick slot returned to wrong bag")
			}
		})
	}
}

func TestPersonalInventoryStaleDragCannotRetarget(t *testing.T) {
	for _, change := range []string{"hero", "source", "resize", "party"} {
		t.Run(change, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			ch := g.party.Members[0]
			g.menuOpen, g.currentTab = true, TabInventory
			ch.Inventory = g.party.Inventory
			g.party.Inventory = nil
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			sx, sy := personalBagCell(l.personalGrid, 0)
			dx, dy := personalBagCell(l.grid, 0)
			fp := installFakePointer(t)
			fp.moveTo(sx, sy)
			fp.press()
			h.pointerStep()
			switch change {
			case "hero":
				g.selectedChar = 1
			case "source":
				ch.Inventory[0].Quantity = 4
			case "resize":
				g.config.Display.ScreenWidth = 1280
			case "party":
				replacement := *g.party
				g.party = &replacement
			}
			fp.hold()
			fp.moveTo(dx, dy)
			h.pointerStep()
			fp.release()
			h.pointerStep()
			if len(g.party.Inventory) != 0 || len(ch.Inventory) != 1 {
				t.Fatal("stale pointer gesture transferred an undisplayed source")
			}
		})
	}
}

func TestPersonalInventoryRevivalPickerRetainsOwner(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			owner := g.party.Members[0]
			it := items.CreateItemFromYAML("revival_potion")
			it.Quantity = 3
			owner.Inventory = []items.Item{it}
			g.party.Inventory = []items.Item{it}
			for _, ch := range g.party.Members[1:3] {
				ch.HitPoints = 0
				ch.Conditions = []character.Condition{character.ConditionUnconscious}
			}
			if g.UseConsumableFromInventory(0, 0, owner) || !g.revivalPickerOpen || g.pickerInventoryOwner != owner {
				t.Fatal("personal potion did not own revival picker")
			}
			g.selectedChar = 2
			if cancel {
				g.cancelRevivalPicker()
			} else {
				if !g.applyReviveTo(g.revivalPickerItemIdx, 1) {
					t.Fatal("revival failed")
				}
				g.resolvePickerQuickSource(g.revivalPickerItemIdx, true)
				g.revivalPickerOpen = false
			}
			want := 2
			if cancel {
				want = 3
			}
			if owner.Inventory[0].Count() != want || g.party.Inventory[0].Count() != 3 || g.pickerInventoryOwner != nil {
				t.Fatal("picker retargeted or failed to release its personal source")
			}
		})
	}
}

func TestPersonalInventoryContextActions(t *testing.T) {
	for _, action := range []string{"discard", "split", "move"} {
		t.Run(action, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			ch := g.party.Members[0]
			g.menuOpen, g.currentTab = true, TabInventory
			ch.Inventory = g.party.Inventory
			g.party.Inventory = nil
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			x, y := personalBagCell(l.personalGrid, 0)
			h.clicks(true, x, y, 1)
			if !h.ui.inventoryContextOpen || h.ui.inventoryContextOwner != ch {
				t.Fatal("context menu bound wrong bag")
			}
			row := 0
			switch action {
			case "split":
				row = 1
			case "move":
				row = 2
			}
			h.clicks(false, x+10, y+row*24+12, 1)
			switch action {
			case "discard":
				if inventoryBagUnits(g.party.Bag(ch)) != 4 || len(g.party.Inventory) != 0 {
					t.Fatal("discard used another bag")
				}
			case "split":
				if !h.ui.stackSplitPicker.open || h.ui.stackSplitPicker.owner != ch || inventoryBagUnits(g.party.Bag(ch)) != 5 {
					t.Fatal("split picker lost owner or spent early")
				}
			case "move":
				if len(ch.Inventory) != 0 || inventoryBagUnits(g.party.Bag()) != 5 {
					t.Fatal("move did not transfer to shared")
				}
			}
		})
	}
}

func TestPersonalInventoryCarriedSpecialItems(t *testing.T) {
	for _, kind := range []string{"flask", "card", "key"} {
		t.Run(kind, func(t *testing.T) {
			g, ch := rareClassGame(t, character.ClassAlchemist, false)
			g.party.Inventory = nil
			ch.Inventory = nil
			switch kind {
			case "flask":
				it := items.CreateItemFromYAML("harm_flask")
				it.Quantity = 2
				ch.Inventory = []items.Item{it}
				if g.flaskStock(g.party.Members[0], "harm_flask") != 2 || !g.throwFlask(0, "harm_flask", false) || ch.Inventory[0].Count() != 1 {
					t.Fatal("personal flask unavailable or wrong stock consumed")
				}
			case "card":
				it := items.CreateItemFromYAML("wolf_card")
				ch.Inventory = []items.Item{it}
				indices := g.inventoryCardIndices()
				if len(indices) != 1 || !g.placeCardFromInventory(indices[0]) || len(ch.Inventory) != 0 || g.cardCollectionItem(0).InstanceID != it.InstanceID {
					t.Fatal("personal card collection lost source identity")
				}
				if !g.removeCardToInventory(0) || len(g.party.Inventory) != 1 {
					t.Fatal("card did not return to shared bag")
				}
			case "key":
				ch.Inventory = []items.Item{{Name: "Test key", Attributes: map[string]int{"master_key": 1}}}
				if _, ok := g.partyMasterKeyName(); !ok {
					t.Fatal("personal key unavailable")
				}
			}
		})
	}
}

func TestPersonalInventoryDisplayedFiltersStayIndependent(t *testing.T) {
	for _, personal := range []bool{false, true} {
		for _, size := range [][2]int{{800, 680}, {1920, 1080}} {
			t.Run(fmt.Sprintf("personal=%v/%v", personal, size), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				g.menuOpen, g.currentTab = true, TabInventory
				hero := g.party.Members[0]
				g.party.Inventory = nil
				for _, owner := range []*character.MMCharacter{nil, hero} {
					for i := 0; i < 34; i++ {
						g.party.Bag(owner).Add(items.Item{Name: fmt.Sprintf("Blade %d", i), Type: items.ItemWeapon, InstanceID: uint64(i + 10)})
					}
					g.party.Bag(owner).Add(items.Item{Name: "Fur", Type: items.ItemTrinket})
				}
				h.pointerStep() // Establish the selected hero before setting page state.
				h.ui.inventoryPage, h.ui.personalInventoryPage = 1, 2
				l := computeInventoryContentLayout(computeTabbedMenuLayout(size[0], gameplayViewportBottom(g)).content)
				filters := l.categories
				var owner *character.MMCharacter
				if personal {
					owner = hero
					filters = l.personalCategories
				}
				target := inventoryTabOwning(items.Item{Type: items.ItemWeapon})
				rect := inventoryTabRects(filters.x, filters.y, filters.w)[target]
				h.clicks(false, rect.x+rect.w/2, rect.y+rect.h/2, 1)
				selected, page := h.ui.inventoryFilterState(owner)
				otherSelected, otherPage := h.ui.inventoryFilterState(hero)
				wantOtherPage := 2
				if personal {
					otherSelected, otherPage = h.ui.inventoryFilterState()
					wantOtherPage = 1
				}
				if *selected != target || *page != 0 || *otherSelected != inventoryTabAll || *otherPage != wantOtherPage {
					t.Fatalf("coupled filters: shared %d/%d personal %d/%d", h.ui.inventoryTab, h.ui.inventoryPage, h.ui.personalInventoryTab, h.ui.personalInventoryPage)
				}
				*page = 1
				h.clicks(false, rect.x+rect.w/2, rect.y+rect.h/2, 1)
				if *page != 1 {
					*page = 1
					t.Fatal("reselecting a filter reset its page")
				}
			})
		}
	}
}

func TestPersonalInventoryDragAcrossDifferentFilters(t *testing.T) {
	for _, personalSource := range []bool{false, true} {
		t.Run(fmt.Sprint(personalSource), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			hero := g.party.Members[0]
			g.menuOpen, g.currentTab = true, TabInventory
			g.party.Inventory = nil
			hero.Inventory = nil
			for _, owner := range []*character.MMCharacter{nil, hero} {
				for i := 0; i < 19; i++ {
					g.party.Bag(owner).Add(items.Item{Name: fmt.Sprintf("Blade %d", i), Type: items.ItemWeapon, InstanceID: uint64(i + 50)})
				}
				g.party.Bag(owner).Add(items.Item{Name: "Fur", Type: items.ItemTrinket, Quantity: 3})
			}
			h.pointerStep()
			var source, destination *character.MMCharacter
			source, destination = nil, hero
			if personalSource {
				source, destination = hero, nil
			}
			h.ui.setInventoryTab(inventoryTabOwning(items.Item{Type: items.ItemWeapon}), source)
			h.ui.setInventoryTab(inventoryTabCatchAll, destination)
			_, page := h.ui.inventoryFilterState(source)
			*page = 1
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			src, dst := l.grid, l.personalGrid
			if personalSource {
				src, dst = dst, src
			}
			sx, sy := personalBagCell(src, 1)
			dx, dy := personalBagCell(dst, 0)
			expected := g.party.Bag(source).Items()[17]
			fp := installFakePointer(t)
			fp.moveTo(sx, sy)
			fp.press()
			h.pointerStep()
			fp.hold()
			fp.moveTo(dx, dy)
			h.pointerStep()
			fp.release()
			h.pointerStep()
			srcItems, dstItems := g.party.Bag(source).Items(), g.party.Bag(destination).Items()
			if len(srcItems) != 19 || len(dstItems) != 21 || dstItems[20].Name != expected.Name || dstItems[20].InstanceID != expected.InstanceID {
				t.Fatal("filtered drag moved the wrong absolute entry")
			}
			dstFilter, _ := h.ui.inventoryFilterState(destination)
			if *dstFilter != inventoryTabCatchAll {
				t.Fatal("drop changed target filter")
			}
		})
	}
}

func TestPersonalInventoryFilterChangeRejectsStaleCellClicks(t *testing.T) {
	for _, personal := range []bool{false, true} {
		t.Run(fmt.Sprint(personal), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			hero := g.party.Members[0]
			g.menuOpen, g.currentTab = true, TabInventory
			g.party.Inventory = nil
			hero.Inventory = nil
			var owner *character.MMCharacter
			if personal {
				owner = hero
			}
			bag := g.party.Bag(owner)
			bag.Add(items.CreateWeaponFromYAML("iron_sword"))
			bag.Add(items.CreateItemFromYAML("health_potion"))
			h.pointerStep()
			l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
			grid := l.grid
			if personal {
				grid = l.personalGrid
			}
			x, y := personalBagCell(grid, 0)
			h.ui.Draw(h.screen)
			selected, _ := h.ui.inventoryFilterState(owner)
			*selected = inventoryTabOwning(items.Item{Type: items.ItemConsumable})
			g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: 100}, {x: x, y: y, at: 120}}
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if len(bag.Items()) != 2 {
				t.Fatal("stale cell activated after its bag filter changed")
			}
		})
	}
}

func TestPersonalInventoryMapOpensWithEitherHUDLayout(t *testing.T) {
	for _, hud := range []bool{false, true} {
		for _, personal := range []bool{false, true} {
			t.Run(fmt.Sprintf("hud=%v/personal=%v", hud, personal), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				g.showPartyStats = hud
				g.menuOpen, g.currentTab = true, TabInventory
				g.party.Inventory = nil
				hero := g.party.Members[0]
				hero.Inventory = nil
				var owner *character.MMCharacter
				if personal {
					owner = hero
				}
				g.party.Bag(owner).Add(items.Item{Name: "World Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}})
				l := computeInventoryContentLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content)
				grid := l.grid
				if personal {
					grid = l.personalGrid
				}
				x, y := personalBagCell(grid, 0)
				h.clicks(false, x, y, 2)
				if !g.mapOverlayOpen {
					t.Fatal("displayed bag item did not open its map overlay")
				}
			})
		}
	}
}

func TestPersonalInventoryLongLabelsKeepFullHoverText(t *testing.T) {
	for _, label := range []string{"hero", "gold", "food"} {
		t.Run(label, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 680)
			g := h.g
			g.showPartyStats, g.menuOpen, g.currentTab = true, true, TabInventory
			g.party.Members[0].Name = "An Exceptionally Long Traveller Name That Cannot Fit The Heading"
			g.party.Gold, g.party.Food = int(^uint(0)>>1), int(^uint(0)>>1)
			l := computeInventoryContentLayout(computeTabbedMenuLayout(800, gameplayViewportBottom(g)).content)
			r, full := l.headings[0], g.party.Members[0].Name
			if label == "gold" {
				r, full = l.resources[0], fmt.Sprintf("Gold %d", g.party.Gold)
			} else if label == "food" {
				r, full = l.resources[1], fmt.Sprintf("Food %d", g.party.Food)
			}
			fp := installFakePointer(t)
			fp.moveTo(r.x+r.w/2, r.y+r.h/2)
			h.ui.Draw(h.screen)
			var want []string
			if (uiTextWidth(full)+4)*l.textScale > r.w-8 {
				want = []string{full}
			}
			if !reflect.DeepEqual(h.ui.tooltipLines, want) {
				t.Fatalf("%s full-text hover = %v, expected %v", label, h.ui.tooltipLines, want)
			}
		})
	}
}
