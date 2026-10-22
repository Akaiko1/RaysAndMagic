package game

import (
	"image"
	"testing"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/spells"

	"ugataima/internal/items"
)

// Quick slots: drink/equip dispatch, inventory<->slot drag resolution, and the
// save round-trip. Casting is exercised by the spellbook tests (shared path).
func TestQuickSlots_UseDropAndPersist(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	// Drinking and equipping never spend an action, so an empty turn budget must not block them.
	ch.ActionsRemaining = 0

	// Consumable: double-click drinks one and empties the slot.
	pot := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[0] = &pot
	ch.HitPoints = 1
	game.useQuickSlot(0, 0)
	if ch.HitPoints <= 1 {
		t.Fatalf("potion in quick slot did not heal (HP=%d)", ch.HitPoints)
	}
	if ch.QuickSlots[0] != nil {
		t.Fatalf("consumed potion should leave the slot empty")
	}

	// Weapon: equip from the slot. Stash the starting weapon, clear the hand,
	// then re-equip it via the quick slot.
	cur, had := ch.Equipment[items.SlotMainHand]
	if !had {
		t.Fatal("fixture: the first hero has no starting main-hand weapon")
	}
	w := cur
	ch.QuickSlots[1] = &w
	delete(ch.Equipment, items.SlotMainHand)
	game.useQuickSlot(0, 1)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatalf("weapon was not equipped from the quick slot")
	}
	if ch.QuickSlots[1] != nil {
		t.Fatalf("equipping into an empty hand should empty the slot")
	}
	if ch.ActionsRemaining != 0 {
		t.Fatalf("drinking or equipping spent the turn budget: %d", ch.ActionsRemaining)
	}

	// Drag an inventory item into a slot: it leaves the shared bag.
	game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	idx := len(game.party.Inventory) - 1
	bagLen := len(game.party.Inventory)
	game.dragSrc = dragFromInventory
	game.dragInvIndex = idx
	game.resolveQuickSlotDrop(0, 3)
	if ch.QuickSlots[3] == nil {
		t.Fatalf("inventory item was not moved into the quick slot")
	}
	if len(game.party.Inventory) != bagLen-1 {
		t.Fatalf("item should have left the bag (len %d -> %d)", bagLen, len(game.party.Inventory))
	}

	// Drag it back out onto the inventory: slot empties, bag regrows.
	ui := &UISystem{game: game}
	game.dragSrc = dragFromQuickSlot
	game.dragQuickChar = 0
	game.dragQuickSlot = 3
	game.menuOpen = true
	game.dragDropAt = 1
	game.drag.curX, game.drag.curY = 5, 5
	ui.quickInvDropZone(0, 0, 100, 100)
	if ch.QuickSlots[3] != nil {
		t.Fatalf("dragging out should empty the slot")
	}
	if len(game.party.Inventory) != bagLen {
		t.Fatalf("item should be back in the bag")
	}

	// Potions are passive: a quick-slot potion works even on RT cooldown and
	// never spends an action (free, by design - unlike weapons/spells/traps).
	game.turnBasedMode = false
	gated := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[4] = &gated
	ch.HitPoints = 1
	ch.RTCooldown = 10
	game.useQuickSlot(0, 4)
	if ch.HitPoints <= 1 || ch.QuickSlots[4] != nil {
		t.Fatalf("potion quick slot must work even on cooldown (HP=%d, slot nil=%v)", ch.HitPoints, ch.QuickSlots[4] == nil)
	}

	// Quest item (map): opens the overlay even on cooldown, and is NOT consumed.
	mapItem := items.Item{Name: "Old Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}}
	ch.QuickSlots[4] = &mapItem
	ch.RTCooldown = 10 // would block a combat action; a map is not one
	game.mapOverlayOpen = false
	game.useQuickSlot(0, 4)
	if !game.mapOverlayOpen {
		t.Fatalf("map quick slot should open the map overlay")
	}
	if ch.QuickSlots[4] == nil {
		t.Fatalf("a map must not be consumed on use")
	}
	game.mapOverlayOpen = false
	ch.QuickSlots[4] = nil
	ch.RTCooldown = 0

	// Save round-trip preserves a populated slot.
	keep := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[2] = &keep
	rc := restoreCharacterSave(buildCharacterSave(ch))
	if rc.QuickSlots[2] == nil || rc.QuickSlots[2].Name != keep.Name {
		t.Fatalf("quick slot not preserved across save/load")
	}

	// Row II and its switch round-trip; a row II slot in a save that lost the
	// switch turns it back on rather than hiding the item.
	ch.QuickSecondRow = true
	two := items.CreateItemFromYAML("health_potion")
	ch.QuickSlots[character.QuickSlotsPerRow+2] = &two
	save := buildCharacterSave(ch)
	rc = restoreCharacterSave(save)
	if !rc.QuickSecondRow || rc.QuickSlots[character.QuickSlotsPerRow+2] == nil || rc.QuickSlots[2] == nil {
		t.Fatalf("row II not preserved across save/load (on=%v)", rc.QuickSecondRow)
	}
	save.QuickSecondRow = false
	if rc = restoreCharacterSave(save); !rc.QuickSecondRow {
		t.Fatal("a row II slot loaded with the second row off")
	}
}

func TestInventoryDropZonesRespectModalBlocker(t *testing.T) {
	tests := []struct {
		name string
		drop func(*UISystem)
	}{
		{name: "quick slot to bag", drop: func(ui *UISystem) { ui.quickInvDropZone(0, 0, 100, 100) }},
		{name: "bag to equipment", drop: func(ui *UISystem) { ui.equipSlotDropZone(items.SlotMainHand, 0, 0, 100, 100) }},
		{name: "bag cell swap", drop: func(ui *UISystem) { ui.inventoryCellDropZone(1, 0, 0, 100, 100) }},
		{name: "bag tail move", drop: func(ui *UISystem) { ui.inventoryEmptyDropZone(0, 0, 100, 100) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game, _, _ := tbBehaviorGame(t, 20, 20)
			ui := NewUISystem(game)
			game.menuOpen = true
			game.statPopupOpen = true
			game.party.Inventory = []items.Item{
				{Name: "First", Type: items.ItemConsumable},
				{Name: "Second", Type: items.ItemConsumable},
			}
			quick := items.Item{Name: "Quick", Type: items.ItemConsumable}
			game.party.Members[0].QuickSlots[0] = &quick
			game.dragSrc = dragFromInventory
			if tt.name == "quick slot to bag" {
				game.dragSrc = dragFromQuickSlot
				game.dragQuickChar = 0
				game.dragQuickSlot = 0
			}
			game.dragInvIndex = 0
			game.dragDropAt = 1
			game.drag.curX, game.drag.curY = 5, 5

			tt.drop(ui)

			if game.dragSrc == dragNone {
				t.Fatal("modal-blocked drop was resolved and cleared")
			}
			if len(game.party.Inventory) != 2 || game.party.Inventory[0].Name != "First" || game.party.Inventory[1].Name != "Second" {
				t.Fatalf("modal-blocked drop mutated inventory: %+v", game.party.Inventory)
			}
			if game.party.Members[0].QuickSlots[0] == nil {
				t.Fatal("modal-blocked drop removed the quick-slot item")
			}
		})
	}
}

func TestQuickSlotDrop_TakesOnlyDraggedStackUnits(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	game.party.Inventory = []items.Item{{
		Name: "Health Potion", Type: items.ItemConsumable, Quantity: 3, InstanceID: 42,
	}}

	// Shift-drag is represented by a positive partial quantity. It must leave
	// the two remaining potions in the bag and place only one in the quick slot.
	game.dragSrc = dragFromInventory
	game.dragInvIndex = 0
	game.dragSplitQuantity = 1
	game.dragItem = game.party.Inventory[0]
	game.dragItem.Quantity = 1
	game.resolveQuickSlotDrop(0, 0)

	if ch.QuickSlots[0] == nil || ch.QuickSlots[0].Count() != 1 || ch.QuickSlots[0].InstanceID != 42 {
		t.Fatalf("quick slot fragment = %+v, want one original-lineage potion", ch.QuickSlots[0])
	}
	if len(game.party.Inventory) != 1 || game.party.Inventory[0].Count() != 2 || game.party.Inventory[0].InstanceID == 42 {
		t.Fatalf("bag remainder = %+v, want two rekeyed potions", game.party.Inventory)
	}

	// A second partial drop onto the same quick slot merges the quantity there,
	// while leaving one potion in the bag.
	game.dragSrc = dragFromInventory
	game.dragInvIndex = 0
	game.dragSplitQuantity = 1
	game.dragItem = game.party.Inventory[0]
	game.dragItem.Quantity = 1
	game.resolveQuickSlotDrop(0, 0)
	if ch.QuickSlots[0].Count() != 2 || game.party.Inventory[0].Count() != 1 {
		t.Fatalf("second partial drop quick=%+v bag=%+v, want 2 and 1", ch.QuickSlots[0], game.party.Inventory)
	}
}

// Equipping gear via a quick slot is a free swap - it must not spend a turn-based
// action or set a real-time cooldown (only spells/traps are combat actions).
func TestQuickSlot_EquipIsFree(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 20, 20)
	ch := game.party.Members[0]
	cur, had := ch.Equipment[items.SlotMainHand]
	if !had {
		t.Fatal("fixture: the first hero has no starting main-hand weapon")
	}

	// Turn-based: equipping from a slot must NOT consume an action.
	game.turnBasedMode = true
	ch.ActionsRemaining = 1
	w := cur
	delete(ch.Equipment, items.SlotMainHand)
	ch.QuickSlots[0] = &w
	game.useQuickSlot(0, 0)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatal("weapon was not equipped from the quick slot")
	}
	if ch.ActionsRemaining != 1 {
		t.Errorf("equipping spent a TB action (ActionsRemaining=%d, want 1)", ch.ActionsRemaining)
	}

	// Real-time: equipping must NOT set a cooldown, and works even while on one.
	game.turnBasedMode = false
	w2 := ch.Equipment[items.SlotMainHand]
	delete(ch.Equipment, items.SlotMainHand)
	ch.QuickSlots[1] = &w2
	ch.RTCooldown = 30 // already busy from a swing
	game.useQuickSlot(0, 1)
	if _, ok := ch.Equipment[items.SlotMainHand]; !ok {
		t.Fatal("weapon was not equipped while on cooldown (equip should be free)")
	}
	if ch.RTCooldown != 30 {
		t.Errorf("equipping changed the RT cooldown (RTCooldown=%d, want 30 unchanged)", ch.RTCooldown)
	}
}

// A double-click on the in-game bar uses the slot - the two clicks land in
// separate frames with the bar redrawn between them, whether or not the
// slot's card is showing. One click alone does nothing, and a click on
// another cell does not complete the pair. With the second row on, the spell
// sits in row II right over the potion.
func TestInGameQuickSlotDoubleClickUsesTheSlot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rowII   bool
		slot    int
		tooltip bool
		second  int    // slot of the second click
		want    string // "heal", "cast" or "" for nothing used
	}{
		{"potion", false, 0, false, 0, "heal"},
		{"potion with its card up", false, 0, true, 0, "heal"},
		{"spell", false, 1, false, 1, "cast"},
		{"spell with its card up", false, 1, true, 1, "cast"},
		{"clicks on two cells", false, 0, false, 1, ""},
		{"row II spell", true, 5, false, 5, "cast"},
		{"potion under row II", true, 0, false, 0, "heal"},
		{"same column, two rows", true, 5, false, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newDwellHarness(t)
			g := d.h.g
			g.party.Members[g.selectedChar] = character.CreateCharacter("Lysander", character.ClassSorcerer, g.config)
			ch := g.party.Members[g.selectedChar]
			potion := items.CreateItemFromYAML("health_potion")
			potion.Quantity = 3
			spell, err := spells.CreateSpellItem("firebolt")
			if err != nil {
				t.Fatal(err)
			}
			spellSlot := 1
			if tc.rowII {
				spellSlot = character.QuickSlotsPerRow
			}
			ch.QuickSlots[0], ch.QuickSlots[spellSlot], ch.QuickSecondRow = &potion, &spell, tc.rowII
			ch.HitPoints = ch.MaxHitPoints / 2
			layout, visible := inGameActionBarLayout(g)
			if !visible || !layout.hasQuick || (layout.quick2.w > 0) != tc.rowII {
				t.Fatalf("fixture: in-game bar %+v", layout)
			}
			cell := func(slot int) image.Point {
				bar := layout.quick
				if slot >= character.QuickSlotsPerRow {
					bar = layout.quick2
				}
				_, cells := quickSlotRects(bar.x, bar.y, bar.w)
				return centerOf(cells[slot%character.QuickSlotsPerRow])
			}
			first, second := cell(tc.slot), cell(tc.second)
			d.at = first
			d.frame()
			if tc.tooltip {
				d.clock = d.clock.Add(quickSlotCardDelay)
				if d.frame() == "" {
					t.Fatal("fixture: the card did not open")
				}
			}
			hp, sp := ch.HitPoints, ch.SpellPoints
			click := func(p image.Point) {
				d.at = p
				beginUIFrame(d.h.screen, g.uiPixelScale())
				d.h.clicksAt(false, p.X, p.Y, 1, d.clock.UnixMilli())
				d.clock = d.clock.Add(100 * time.Millisecond)
				d.frame()
			}
			click(first)
			if ch.HitPoints != hp || ch.SpellPoints != sp || potion.Quantity != 3 {
				t.Fatal("a single click used the slot")
			}
			click(second)
			used := ""
			switch {
			case ch.HitPoints > hp:
				used = "heal"
			case ch.SpellPoints < sp:
				used = "cast"
			}
			if used != tc.want {
				t.Fatalf("double-click used %q, want %q (HP %d->%d, SP %d->%d)", used, tc.want, hp, ch.HitPoints, sp, ch.SpellPoints)
			}
		})
	}
}

// The second row as the player uses it in a book: the check box shows row II,
// a spell dragged onto a row II cell binds that slot, and unchecking sends row
// II's items to the hero's bag while its spells just leave the bar. Row I is
// untouched throughout.
func TestQuickSecondRowCheckBoxAndDrop(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.menuOpen, g.currentTab, g.selectedChar = true, TabSpellbook, 0
	caster := g.party.Members[0]
	caster.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
	caster.LearnSpell("firebolt")
	g.selectedSchool, g.selectedSpell = 0, -1
	keep := items.CreateItemFromYAML("health_potion")
	caster.QuickSlots[0] = &keep
	layout := func() bookLayout {
		return computeBookLayout(computeTabbedMenuLayout(1024, gameplayViewportBottom(g)).content, caster.QuickRows())
	}
	// The check box ends the caption line over the bars.
	toggle := func() {
		l := layout()
		bars := l.quick
		if l.quick2.w > 0 {
			bars = bars.union(l.quick2)
		}
		h.clicks(false, bars.right()-3, bars.y-quickSlotTabLabelSpace+quickSlotTabLabelH/2, 1)
	}
	potions := func() int {
		n := 0
		for _, it := range g.party.Bag(caster).Items() {
			if it.Name == keep.Name {
				n += it.Count()
			}
		}
		return n
	}

	toggle()
	if !caster.QuickSecondRow {
		t.Fatal("the check box did not turn the second row on")
	}
	l := layout()
	x, y := l.cardPos(0)
	_, cells := quickSlotRects(l.quick2.x, l.quick2.y, l.quick2.w)
	fp := installFakePointer(t)
	fp.moveTo(x+l.iconSize/2, y+l.iconSize/2)
	fp.press()
	h.pointerStep()
	fp.hold()
	fp.moveTo(cells[2].Min.X+3, cells[2].Min.Y+3)
	h.pointerStep()
	fp.release()
	h.pointerStep()
	fp.idle()
	rowII := character.QuickSlotsPerRow
	if got := caster.QuickSlots[rowII+2]; got == nil || got.SpellEffect != "firebolt" || caster.QuickSlots[2] != nil {
		t.Fatalf("drop on row II cell 2 bound %+v (row I cell 2 %+v)", got, caster.QuickSlots[2])
	}

	potion := items.CreateItemFromYAML("health_potion")
	caster.QuickSlots[rowII] = &potion
	bag := potions()
	toggle()
	if caster.QuickSecondRow {
		t.Fatal("the check box did not turn the second row off")
	}
	for i := rowII; i < character.QuickSlotCount; i++ {
		if caster.QuickSlots[i] != nil {
			t.Fatalf("slot %d kept %+v with the second row off", i, caster.QuickSlots[i])
		}
	}
	if potions() != bag+1 || caster.QuickSlots[0] != &keep {
		t.Fatalf("bag potions %d -> %d, row I %+v; want the row II potion back and row I kept", bag, potions(), caster.QuickSlots[0])
	}
	for _, it := range g.party.Bag(caster).Items() {
		if it.VirtualAction() {
			t.Fatalf("spell shortcut %q went to the bag", it.Name)
		}
	}
	toggle()
	if !caster.QuickSecondRow || caster.QuickSlots[rowII] != nil {
		t.Fatal("turning the row back on did not show an empty row II")
	}
}
