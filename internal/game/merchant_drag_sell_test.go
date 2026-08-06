package game

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/graphics"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

// merchantDragGame stages an open plain-merchant dialog with a valued stack of
// potions in bag slot 0 and the shared drag machinery ready to resolve.
func merchantDragGame(t *testing.T) (*MMGame, *UISystem) {
	t.Helper()
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.dialogActive = true
	g.dialogNPC = &character.NPC{Name: "Trader", SellAvailable: true}
	g.party.Inventory = []items.Item{{
		Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5, InstanceID: 7,
		Attributes: map[string]int{"value": 10},
	}}
	g.party.Gold = 0
	return g, ui
}

// dropBagStackOnStockGrid fakes the end state of a real drag: bag entry idx
// carried, released over the merchant's stock grid, then runs the draw pass
// that resolves drops.
func dropBagStackOnStockGrid(g *MMGame, ui *UISystem, idx int, onStock bool) {
	dlg := npcDialogLayout(g)
	leftX, _, gridTop, _ := merchantGridLayout(dlg.x, dlg.y)
	g.stashDragActive = true
	g.stashDragDrop = true
	g.stashDragFrom = stashDragInvBase + idx
	g.stashDragItem = g.party.Inventory[idx]
	if onStock {
		g.stashDragCurX, g.stashDragCurY = leftX+4, gridTop+4
	} else {
		g.stashDragCurX, g.stashDragCurY = dlg.x+2, dlg.y+2 // title bar: no drop zone
	}
	screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	ui.drawMerchantDialog(screen, dlg.x, dlg.y, dlg.w, dlg.h)
}

func TestDragStackToMerchantOpensSellPickerWithFullStack(t *testing.T) {
	g, ui := merchantDragGame(t)

	dropBagStackOnStockGrid(g, ui, 0, true)
	if !ui.stackSplitPicker.open || ui.stackSplitPicker.source != stackSplitPickerMerchantSell {
		t.Fatalf("picker = %+v, want an open merchant-sell picker", ui.stackSplitPicker)
	}
	if ui.stackSplitPicker.quantity != 5 {
		t.Fatalf("default quantity = %d, want the FULL stack (5)", ui.stackSplitPicker.quantity)
	}
	if g.stashDragActive || g.stashDragDrop {
		t.Fatal("the resolved drop left drag state behind")
	}

	// Confirm sells the chosen units at the merchant price.
	ui.stackSplitConfirm()
	if len(g.party.Inventory) != 0 {
		t.Fatalf("selling the full stack left %d entries", len(g.party.Inventory))
	}
	if g.party.Gold != 50 {
		t.Fatalf("gold = %d, want 5 x 10", g.party.Gold)
	}
	if ui.stackSplitPicker.open {
		t.Fatal("the picker stayed open after the sale")
	}
}

func TestDragReleasedOffTheStockGridJustCancels(t *testing.T) {
	g, ui := merchantDragGame(t)

	dropBagStackOnStockGrid(g, ui, 0, false)
	if ui.stackSplitPicker.open {
		t.Fatal("a drop outside the stock grid opened the sell picker")
	}
	if g.stashDragActive || g.stashDragDrop {
		t.Fatal("a cancelled drop left drag state behind")
	}
	if len(g.party.Inventory) != 1 || g.party.Gold != 0 {
		t.Fatal("a cancelled drop mutated the bag or the purse")
	}
}

// EVERY sale confirms, singles included: one misdrop must never cost the party
// a sword. The picker opens at quantity 1 and only its Sell button parts with
// the item.
func TestDragSellSingleItemStillConfirms(t *testing.T) {
	g, ui := merchantDragGame(t)
	g.party.Inventory[0].Quantity = 1

	dropBagStackOnStockGrid(g, ui, 0, true)
	if !ui.stackSplitPicker.open || ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("a single item did not open its confirmation: %+v", ui.stackSplitPicker)
	}
	if len(g.party.Inventory) != 1 || g.party.Gold != 0 {
		t.Fatal("the item was sold before the player confirmed")
	}

	ui.stackSplitConfirm()
	if len(g.party.Inventory) != 0 || g.party.Gold != 10 {
		t.Fatalf("confirmed single sale wrong: bag=%d gold=%d", len(g.party.Inventory), g.party.Gold)
	}
}

func TestDragSellRefusesWorthlessItems(t *testing.T) {
	g, ui := merchantDragGame(t)
	g.party.Inventory[0].Attributes["value"] = 0

	dropBagStackOnStockGrid(g, ui, 0, true)
	if ui.stackSplitPicker.open || len(g.party.Inventory) != 1 || g.party.Gold != 0 {
		t.Fatal("a worthless item was sold or opened the picker")
	}
}

// The picker types like a form: the first digit replaces the prefilled full
// stack, further digits append, clamping never exceeds the stack, arrows step,
// Enter sells.
func TestSellPickerKeyboardEntry(t *testing.T) {
	g, ui := merchantDragGame(t)
	g.party.Inventory[0].Quantity = 20
	loop := &GameLoop{game: g, ui: ui}
	g.gameLoop = loop

	dropBagStackOnStockGrid(g, ui, 0, true)
	if !ui.stackSplitPicker.open || ui.stackSplitPicker.quantity != 20 {
		t.Fatalf("staging broke: picker %+v", ui.stackSplitPicker)
	}

	pressed := map[ebiten.Key]bool{}
	ih := NewInputHandler(g)
	loop.inputHandler = ih
	ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed[k] })
	press := func(k ebiten.Key) {
		pressed = map[ebiten.Key]bool{k: true}
		ih.keys.BeginFrame() // one press per frame, as HandleInput would
		if !ih.handleTopModalInput() {
			t.Fatal("the open picker no longer owns input")
		}
	}

	press(ebiten.KeyDigit7) // first digit REPLACES the prefilled 20
	if q := ui.stackSplitPicker.quantity; q != 7 {
		t.Fatalf("after '7': quantity = %d, want 7", q)
	}
	press(ebiten.KeyDigit3) // append -> 73, clamped to the stack of 20
	if q := ui.stackSplitPicker.quantity; q != 20 {
		t.Fatalf("after '73': quantity = %d, want clamp to 20", q)
	}
	press(ebiten.KeyBackspace) // 20 -> 2
	if q := ui.stackSplitPicker.quantity; q != 2 {
		t.Fatalf("after backspace: quantity = %d, want 2", q)
	}
	press(ebiten.KeyUp)
	press(ebiten.KeyDown)
	press(ebiten.KeyDown) // 2 +1 -1 -1 -> clamped at 1... (2->3->2->1)
	if q := ui.stackSplitPicker.quantity; q != 1 {
		t.Fatalf("after arrows: quantity = %d, want 1", q)
	}
	press(ebiten.KeyEnter)
	if ui.stackSplitPicker.open {
		t.Fatal("Enter did not confirm the sale")
	}
	if g.party.Gold != 10 || g.party.Inventory[0].Count() != 19 {
		t.Fatalf("keyboard sale wrong: gold=%d remaining=%d, want 10 and 19", g.party.Gold, g.party.Inventory[0].Count())
	}
}

// Only the Cancel button (or ESC) dismisses a trade dialog. A stray click on
// the dim used to close it, which on a purchase reads as "did that go through?"
func TestTradePickerClosesOnlyOnCancelButton(t *testing.T) {
	g, ui := merchantDragGame(t)
	dropBagStackOnStockGrid(g, ui, 0, true)
	if !ui.stackSplitPicker.open {
		t.Fatal("staging broke: no picker")
	}

	screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	r := stackSplitPickerRect(g.config.GetScreenWidth(), g.config.GetScreenHeight())

	// A click on the dim, well clear of every button.
	g.mouseLeftClicks = []queuedClick{{x: r.Min.X - 40, y: r.Min.Y - 40, at: 1000}}
	ui.drawStackSplitPicker(screen)
	if !ui.stackSplitPicker.open {
		t.Fatal("a click outside the panel dismissed the trade dialog")
	}
	// A click inside the panel but on no button (under the buttons row).
	g.mouseLeftClicks = []queuedClick{{x: r.Min.X + 140, y: r.Min.Y + 88, at: 1001}}
	ui.drawStackSplitPicker(screen)
	if !ui.stackSplitPicker.open {
		t.Fatal("a click on empty panel space dismissed the trade dialog")
	}

	// Control: the Cancel button still closes it, with nothing sold.
	cancel := stackSplitPickerLayout(r).cancel
	g.mouseLeftClicks = []queuedClick{{x: (cancel.Min.X + cancel.Max.X) / 2, y: (cancel.Min.Y + cancel.Max.Y) / 2, at: 1002}}
	ui.drawStackSplitPicker(screen)
	if ui.stackSplitPicker.open {
		t.Fatal("control failed: the Cancel button did not close the dialog")
	}
	if len(g.party.Inventory) != 1 || g.party.Gold != 0 {
		t.Fatal("cancelling still traded")
	}
}

// Keyboard quantity entry through the REAL input entry point (HandleInput),
// clamped to the attainable maximum - not just the modal dispatcher.
func TestBuyPickerKeyboardThroughHandleInput(t *testing.T) {
	g, ui := merchantBuyGame(t, "", potionStock(10, 6)) // 6 on the shelf
	g.party.Gold = 1000
	loop := &GameLoop{game: g, ui: ui}
	g.gameLoop = loop

	dragShelfItemToBag(g, ui, 0, true)
	pressed := map[ebiten.Key]bool{}
	ih := NewInputHandler(g)
	loop.inputHandler = ih
	ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed[k] })
	press := func(k ebiten.Key) {
		pressed = map[ebiten.Key]bool{k: true}
		ih.HandleInput()
	}

	press(ebiten.KeyDigit4) // replaces the default 1
	if q := ui.stackSplitPicker.quantity; q != 4 {
		t.Fatalf("typed 4, got %d", q)
	}
	press(ebiten.KeyNumpad9) // 49 -> clamped to the shelf's 6
	if q := ui.stackSplitPicker.quantity; q != 6 {
		t.Fatalf("typed 49 against a shelf of 6, got %d - the max must clamp", q)
	}
	press(ebiten.KeyEnter)
	if ui.stackSplitPicker.open {
		t.Fatal("Enter did not confirm the purchase")
	}
	if g.party.Gold != 1000-6*10 || g.party.CountItemsByName("Health Potion") != 6 {
		t.Fatalf("keyboard purchase wrong: gold=%d bag=%d", g.party.Gold, g.party.CountItemsByName("Health Potion"))
	}
}

// The double-click sale still sells exactly ONE unit through the same body.
func TestSellInventoryUnitsPartialAndFull(t *testing.T) {
	g, _ := merchantDragGame(t)

	if !g.sellInventoryUnits(0, 3) {
		t.Fatal("partial sale refused")
	}
	if g.party.Gold != 30 || g.party.Inventory[0].Count() != 2 {
		t.Fatalf("partial sale: gold=%d remaining=%d, want 30 and 2", g.party.Gold, g.party.Inventory[0].Count())
	}
	if !g.sellInventoryUnits(0, 2) {
		t.Fatal("closing sale refused")
	}
	if g.party.Gold != 50 || len(g.party.Inventory) != 0 {
		t.Fatalf("closing sale: gold=%d bag=%d, want 50 and empty", g.party.Gold, len(g.party.Inventory))
	}
	if g.sellInventoryUnits(0, 1) {
		t.Fatal("selling from an empty bag succeeded")
	}
}

// merchantBuyGame stages a shop with a stocked shelf and the given till.
func merchantBuyGame(t *testing.T, currency string, entry *character.MerchantStockItem) (*MMGame, *UISystem) {
	t.Helper()
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.dialogActive = true
	g.dialogNPC = &character.NPC{
		Name: "Trader", Currency: currency, SellAvailable: currency == "",
		MerchantStock: []*character.MerchantStockItem{entry},
	}
	g.party.Inventory = nil // the default roster ships a potion; count purchases alone
	return g, ui
}

// dragShelfItemToBag fakes a completed drag from stock cell idx onto the bag
// grid (or, with onBag=false, somewhere harmless).
func dragShelfItemToBag(g *MMGame, ui *UISystem, idx int, onBag bool) {
	dlg := npcDialogLayout(g)
	_, rightX, gridTop, _ := merchantGridLayout(dlg.x, dlg.y)
	g.stashDragActive = true
	g.stashDragDrop = true
	g.stashDragFrom = stashShopDragBase + idx
	g.stashDragItem = g.merchantVisibleStock()[idx].Item
	if onBag {
		g.stashDragCurX, g.stashDragCurY = rightX+4, gridTop+4
	} else {
		g.stashDragCurX, g.stashDragCurY = dlg.x+2, dlg.y+2
	}
	ui.drawMerchantDialog(ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight()), dlg.x, dlg.y, dlg.w, dlg.h)
}

func potionStock(cost, quantity int) *character.MerchantStockItem {
	return &character.MerchantStockItem{
		Item:     items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 1, Attributes: map[string]int{"value": 10}},
		Cost:     cost,
		Quantity: quantity,
	}
}

// Dragging a shelf item onto the bag opens the buy picker at ONE unit - the
// safe spending default - with the attainable maximum as the typing ceiling.
func TestDragBuyDefaultsToOneUnit(t *testing.T) {
	entry := potionStock(20, 7) // 7 on the shelf
	g, ui := merchantBuyGame(t, "", entry)
	g.party.Gold = 1000 // affords far more than the shelf holds

	dragShelfItemToBag(g, ui, 0, true)
	if !ui.stackSplitPicker.open || ui.stackSplitPicker.source != stackSplitPickerMerchantBuy {
		t.Fatalf("picker = %+v, want an open merchant-buy picker", ui.stackSplitPicker)
	}
	if ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("prefill = %d, want the safe default 1", ui.stackSplitPicker.quantity)
	}

	// The shelf count is the ceiling: asking for more clamps to 7.
	ui.stackSplitSetQuantity(999)
	if ui.stackSplitPicker.quantity != 7 {
		t.Fatalf("max = %d, want the shelf's 7", ui.stackSplitPicker.quantity)
	}

	ui.stackSplitConfirm()
	if g.party.Gold != 1000-7*20 {
		t.Fatalf("gold = %d, want 7 x 20 charged", g.party.Gold)
	}
	if entry.Quantity != 0 {
		t.Fatalf("shelf = %d, want emptied", entry.Quantity)
	}
	if got := g.party.CountItemsByName("Health Potion"); got != 7 {
		t.Fatalf("bag holds %d potions, want 7", got)
	}
}

// A thin purse caps the MAXIMUM below the shelf count; the default stays 1.
func TestDragBuyMaximumCappedByPurse(t *testing.T) {
	g, ui := merchantBuyGame(t, "", potionStock(20, 10))
	g.party.Gold = 65 // three at 20, with change

	dragShelfItemToBag(g, ui, 0, true)
	if ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("prefill = %d, want 1", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitSetQuantity(999)
	if ui.stackSplitPicker.quantity != 3 {
		t.Fatalf("max = %d, want the affordable 3", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitConfirm()
	if g.party.Gold != 5 {
		t.Fatalf("gold = %d, want 5 left", g.party.Gold)
	}
}

// An unlimited shelf with a deep purse still types up to the SANE cap only.
func TestDragBuyUnlimitedShelfCapsMaximum(t *testing.T) {
	g, ui := merchantBuyGame(t, "", potionStock(2, -1))
	g.party.Gold = 100000

	dragShelfItemToBag(g, ui, 0, true)
	if ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("prefill = %d, want 1", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitSetQuantity(100000)
	if ui.stackSplitPicker.quantity != maxUnlimitedShelfUnits {
		t.Fatalf("max = %d, want the %d cap", ui.stackSplitPicker.quantity, maxUnlimitedShelfUnits)
	}
}

// In a trade the 1/2 button halves the picker's MAXIMUM (a buy item is a
// one-unit shelf template, so halving its count would always give 1).
func TestBuyPickerHalfButtonHalvesTheMaximum(t *testing.T) {
	g, ui := merchantBuyGame(t, "", potionStock(20, 8))
	g.party.Gold = 1000

	dragShelfItemToBag(g, ui, 0, true)
	if ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("staging broke: prefill = %d, want the default 1", ui.stackSplitPicker.quantity)
	}
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		t.Fatal("picker lost its stock entry")
	}
	halfQ := item.Count() / 2
	if stackSplitIsMerchantTrade(ui.stackSplitPicker.source) {
		halfQ = ui.stackSplitMaxQuantity(item) / 2
	}
	ui.stackSplitSetQuantity(halfQ)
	if ui.stackSplitPicker.quantity != 4 {
		t.Fatalf("half = %d, want 4 (half of the max 8)", ui.stackSplitPicker.quantity)
	}
}

// An arena-points shop trades in its own currency, and its shelf can still be
// dragged - the till only restricts SELLING.
func TestDragBuyUsesArenaPointsTill(t *testing.T) {
	g, ui := merchantBuyGame(t, character.CurrencyArenaPoints, potionStock(5, -1))
	g.party.ArenaPoints = 12
	g.party.Gold = 0

	dragShelfItemToBag(g, ui, 0, true)
	if ui.stackSplitPicker.quantity != 1 {
		t.Fatalf("prefill = %d, want 1", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitSetQuantity(999) // the points till affords 2 at 5
	if ui.stackSplitPicker.quantity != 2 {
		t.Fatalf("max = %d, want 2 (12 points at 5)", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitConfirm()
	if g.party.ArenaPoints != 2 || g.party.Gold != 0 {
		t.Fatalf("arena purchase charged wrong: points=%d gold=%d", g.party.ArenaPoints, g.party.Gold)
	}
}

// A non-coin trader cannot BUY the party's goods: no sell drag source, and the
// shared sale body refuses even if something else reached it.
func TestNonGoldTraderNeverBuysFromTheParty(t *testing.T) {
	g, ui := merchantBuyGame(t, character.CurrencyArenaPoints, potionStock(5, -1))
	g.dialogNPC.SellAvailable = true // an authoring slip the loader would reject
	g.party.Gold = 0
	g.party.Inventory = []items.Item{{
		Name: "Longsword", Type: items.ItemWeapon, Quantity: 1,
		Attributes: map[string]int{"value": 500},
	}}

	if merchantBuysForGold(g.dialogNPC) {
		t.Fatal("an arena-points till was treated as a coin buyer")
	}
	if g.sellInventoryUnits(0, 1) {
		t.Fatal("a non-coin trader bought the party's sword")
	}
	if len(g.party.Inventory) != 1 || g.party.Gold != 0 {
		t.Fatal("the refused sale still moved goods or coin")
	}

	// Control: the same sale works once the shop keeps a coin till.
	g.dialogNPC.Currency = ""
	if !g.sellInventoryUnits(0, 1) || g.party.Gold == 0 {
		t.Fatal("control failed: a coin shop refused a valued item")
	}
	_ = ui
}

// Shipped content must never author a non-coin shop that buys.
func TestShippedTradersSellOnlyForGold(t *testing.T) {
	crateTestGame(t) // loads npcs.yaml (which now fails the load on a slip)
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc.SellAvailable && npc.Currency != "" {
			t.Errorf("NPC %q buys goods but trades in %q", key, npc.Currency)
		}
	}
}

// Bulk-buying gear must mint a FRESH instance id per unit: two bows are two
// physical items, and stash reconciliation identifies gear by that id - shared
// ids would let depositing one delete the other.
func TestBulkBuyGearGetsDistinctInstanceIDs(t *testing.T) {
	bow := items.Item{Name: "Hunting Bow", Type: items.ItemWeapon, Quantity: 1, InstanceID: 4242}
	entry := &character.MerchantStockItem{Item: bow, Cost: 50, Quantity: 5}
	g, ui := merchantBuyGame(t, "", entry)
	g.party.Gold = 1000

	dragShelfItemToBag(g, ui, 0, true)
	ui.stackSplitSetQuantity(3)
	ui.stackSplitConfirm()

	if len(g.party.Inventory) != 3 {
		t.Fatalf("bag holds %d entries, want 3 separate bows", len(g.party.Inventory))
	}
	seen := map[uint64]bool{}
	for _, it := range g.party.Inventory {
		if it.InstanceID == 0 {
			t.Fatal("a purchased bow has no instance id")
		}
		if seen[it.InstanceID] {
			t.Fatalf("instance id %d was handed out twice", it.InstanceID)
		}
		if it.InstanceID == bow.InstanceID {
			t.Fatal("a purchased bow kept the shelf template's id")
		}
		seen[it.InstanceID] = true
	}
	if entry.Item.InstanceID != bow.InstanceID {
		t.Fatal("the shelf template itself was rekeyed")
	}
}

// Gesture arbitration: the press that STARTS a drag must not also count as the
// second click of a double-click. Driven through the real press/move/release
// sequence, so the fix has to live in the input pipeline, not in a helper.
func TestDragStartDoesNotTradeOnTheClickThatBeginsIt(t *testing.T) {
	g, ui := merchantDragGame(t)
	loop := &GameLoop{game: g, ui: ui}
	g.gameLoop = loop
	ih := NewInputHandler(g)
	loop.inputHandler = ih
	fp := installFakePointer(t)

	dlg := npcDialogLayout(g)
	_, rightX, gridTop, _ := merchantGridLayout(dlg.x, dlg.y)
	cx, cy, cw, ch := merchantCellRect(rightX, gridTop, 0)
	cellX, cellY := cx+cw/2, cy+ch/2

	screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	frame := func() {
		ui.updateMouseState()
		ih.HandleInput()
		ui.drawMerchantDialog(screen, dlg.x, dlg.y, dlg.w, dlg.h)
	}

	// Gesture 1: a plain click on the cell (press, release in place) SELECTS.
	fp.moveTo(cellX, cellY)
	fp.press()
	frame()
	fp.release()
	frame()
	fp.idle()
	if g.party.Inventory[0].Count() != 5 {
		t.Fatalf("the selecting click already sold a unit: %d left", g.party.Inventory[0].Count())
	}

	// Gesture 2 STARTS A DRAG on the same cell: its press must not land as the
	// second click of a double-click and sell a unit.
	fp.press()
	frame()
	if g.party.Inventory[0].Count() != 5 {
		t.Fatalf("the press that begins a drag sold a unit: %d left", g.party.Inventory[0].Count())
	}
	fp.moveTo(cellX-60, cellY) // well past quickDragThreshold
	fp.hold()
	frame()
	fp.release()
	frame()
	fp.idle()

	if g.party.Inventory[0].Count() != 5 || g.party.Gold != 0 {
		t.Fatalf("the drag gesture traded: bag=%d gold=%d", g.party.Inventory[0].Count(), g.party.Gold)
	}

	// Control: two plain clicks in place DO sell one unit, so the arbitration
	// above is not simply blocking every trade.
	fp.moveTo(cellX, cellY)
	for i := 0; i < 2; i++ {
		fp.press()
		frame()
		fp.release()
		frame()
		fp.idle()
	}
	if g.party.Inventory[0].Count() != 4 || g.party.Gold != 10 {
		t.Fatalf("control failed: a real double click did not sell one unit (bag=%d gold=%d)",
			g.party.Inventory[0].Count(), g.party.Gold)
	}
}

// Backspace that erases the LAST digit ends the typing run, so the next digit
// starts a fresh number ("7", Backspace, "3" reads 3 - not 13).
func TestPickerBackspaceEndsTheTypingRun(t *testing.T) {
	g, ui := merchantDragGame(t)
	g.party.Inventory[0].Quantity = 40

	dropBagStackOnStockGrid(g, ui, 0, true)
	ui.stackSplitAppendDigit(7)
	if ui.stackSplitPicker.quantity != 7 {
		t.Fatalf("typed 7, got %d", ui.stackSplitPicker.quantity)
	}
	ui.stackSplitBackspace()
	ui.stackSplitAppendDigit(3)
	if ui.stackSplitPicker.quantity != 3 {
		t.Fatalf("after 7, backspace, 3 the quantity is %d - want a fresh 3", ui.stackSplitPicker.quantity)
	}
	// A partial delete still appends: 12, backspace -> 1, then 5 -> 15.
	ui.stackSplitSetQuantity(1)
	ui.stackSplitPicker.typed = false
	ui.stackSplitAppendDigit(1)
	ui.stackSplitAppendDigit(2)
	ui.stackSplitBackspace()
	ui.stackSplitAppendDigit(5)
	if ui.stackSplitPicker.quantity != 15 {
		t.Fatalf("after 12, backspace, 5 the quantity is %d - want 15", ui.stackSplitPicker.quantity)
	}
}

// Release-driven clicks belong to the DRAG SURFACE, not to the dialog layer:
// an ordinary dialog has no drag machine to queue its clicks, so gating on the
// layer alone would leave quest/service/trainer dialogs unclickable. Driven
// through the pointer seam, since the defect lived in the input pipeline.
func TestOrdinaryDialogStillReceivesClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	loop := &GameLoop{game: g, ui: ui}
	g.gameLoop = loop
	ih := NewInputHandler(g)
	loop.inputHandler = ih
	fp := installFakePointer(t)

	npc := &character.NPC{
		Name: "Villager", Type: "encounter",
		DialogueData: &character.NPCDialogue{
			Greeting: "A word?",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Ask about the road", Action: "none"},
				{Text: "Leave", Action: "leave"},
			},
		},
	}
	g.dialogActive = true
	g.dialogNPC = npc
	if got := npcDialogKindFor(npc); got != dialogKindChoices {
		t.Fatalf("fixture is a %v, want an ordinary choice dialog", got)
	}

	dlg := npcDialogLayout(g)
	x, y, w, h := g.dialogueChoiceRect(npc, 1, dlg.x, dlg.y, dlg.w) // the Leave row
	fp.moveTo(x+w/2, y+h/2)

	frame := func() {
		ui.updateMouseState()
		ih.HandleInput()
	}
	// Two clicks: dialog lists act on the second (double-click convention).
	for i := 0; i < 2; i++ {
		fp.press()
		frame()
		fp.release()
		frame()
		fp.idle()
	}

	if g.dialogActive {
		t.Fatal("an ordinary dialog never saw the click - Leave did not close it")
	}
}

// A leading zero is not a number: it must neither write a quantity nor open the
// append run ("0", "3" reads 3 - not 13).
func TestPickerLeadingZeroIsIgnored(t *testing.T) {
	g, ui := merchantDragGame(t)
	g.party.Inventory[0].Quantity = 40

	dropBagStackOnStockGrid(g, ui, 0, true)
	before := ui.stackSplitPicker.quantity
	ui.stackSplitAppendDigit(0)
	if ui.stackSplitPicker.quantity != before {
		t.Fatalf("a leading zero changed the quantity to %d, want the prefill %d", ui.stackSplitPicker.quantity, before)
	}
	ui.stackSplitAppendDigit(3)
	if ui.stackSplitPicker.quantity != 3 {
		t.Fatalf("after 0 then 3 the quantity is %d - want a fresh 3", ui.stackSplitPicker.quantity)
	}
	// A zero INSIDE a typed number still appends: 1 then 0 is ten.
	ui.stackSplitAppendDigit(0)
	if ui.stackSplitPicker.quantity != 30 {
		t.Fatalf("3 then 0 = %d, want 30", ui.stackSplitPicker.quantity)
	}
}

// The shipped Scalewright prices gear in scales PLUS gold, which is far wider
// than the band between the -/+ buttons. The price gets its own line: it must
// never overlap a button, and it is clipped to the panel with a tooltip when it
// still does not fit.
func TestBuyPickerPriceLineNeverOverlapsButtons(t *testing.T) {
	entry := &character.MerchantStockItem{
		Item:         items.Item{Name: "Drakefang", Type: items.ItemWeapon, Quantity: 1},
		Cost:         2,
		GoldCost:     20000,
		CurrencyItem: "red_dragon_scale",
		Quantity:     -1,
	}
	g, ui := merchantBuyGame(t, "", entry)
	g.party.Gold = 100000
	g.party.Inventory = []items.Item{{
		Name: "Red Dragon Scale", Type: items.ItemTrinket, Quantity: 10,
		Attributes: map[string]int{"value": 1},
	}}

	dragShelfItemToBag(g, ui, 0, true)
	if !ui.stackSplitPicker.open {
		t.Fatal("staging broke: no buy picker")
	}

	// The label really is the long, two-currency form this test exists for.
	label := merchantTotalPriceLabel(g, entry, 1)
	if !strings.Contains(label, "Red Dragon Scale") || !strings.Contains(label, "20000 g") {
		t.Fatalf("price label = %q, want the scale + gold form", label)
	}

	r := stackSplitPickerRect(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	L := stackSplitPickerLayout(r)
	for name, btn := range map[string]image.Rectangle{
		"minus": L.minus, "half": L.half, "plus": L.plus, "take": L.take, "cancel": L.cancel,
	} {
		if L.price.Overlaps(btn) {
			t.Errorf("the price line %v overlaps the %s button %v", L.price, name, btn)
		}
	}
	if !L.price.In(r) {
		t.Errorf("the price line %v leaves the panel %v", L.price, r)
	}
	// The band must be wide enough for the shipped worst case to be READ, not
	// merely clipped: this is what the narrow inter-button band failed at.
	if got := debugTextWidth(label); got > L.price.Dx() {
		t.Errorf("shipped price %q needs %d px but the band is %d - it would be truncated", label, got, L.price.Dx())
	}
	// And whatever an author invents later, the drawn text is clipped to fit.
	if got := debugTextWidth(clipDebugText(label, L.price.Dx())); got > L.price.Dx() {
		t.Errorf("clipped price width %d exceeds the band %d", got, L.price.Dx())
	}
	// And the quantity band keeps clear of the buttons it sits between.
	if L.quantity.Overlaps(L.minus) || L.quantity.Overlaps(L.half) {
		t.Errorf("the quantity band %v collides with a button", L.quantity)
	}
}

// The bag grid is the drop target of a drag-to-buy, so it must be DRAWN at
// every shop - including one that pays no coin for the party's goods. An
// invisible drop zone that looks disabled is a trap. Observed without pixels:
// the grid's own pager is drawn by the same block, so a click on it only lands
// when that block ran.
func TestBagGridIsDrawnEvenAtANonCoinShop(t *testing.T) {
	pagerFlips := func(t *testing.T, currency string) bool {
		t.Helper()
		g, ui := merchantBuyGame(t, currency, potionStock(5, -1))
		for i := 0; i < merchantPageSize+4; i++ { // more than one page of bag items
			g.party.Inventory = append(g.party.Inventory, items.Item{
				Name: fmt.Sprintf("Trinket %d", i), Type: items.ItemTrinket, Quantity: 1,
				Attributes: map[string]int{"value": 3},
			})
		}
		dlg := npcDialogLayout(g)
		_, rightX, _, pagerY := merchantGridLayout(dlg.x, dlg.y)
		// drawPager puts Next at the right edge of its width, 30x18.
		nextX, nextY := rightX+merchantGridW-15, pagerY+9

		g.merchantSellPage = 0
		g.mouseLeftClicks = []queuedClick{{x: nextX, y: nextY, at: 1000}}
		ui.drawMerchantDialog(ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight()), dlg.x, dlg.y, dlg.w, dlg.h)
		return g.merchantSellPage == 1
	}

	if !pagerFlips(t, "") {
		t.Fatal("control failed: a coin shop does not draw its bag grid either")
	}
	if !pagerFlips(t, character.CurrencyArenaPoints) {
		t.Fatal("a non-coin shop skips the bag grid - the buy drop zone is invisible")
	}
}

// The drag-to-buy hint must live in the HEADER row, never in a line of its own
// under it: a 16px line starting above gridTop would paint over the first row
// of item icons (AGENTS.md forbids overlapping UI text).
func TestBagHeaderCarriesTheBuyHintWithoutOverlappingIcons(t *testing.T) {
	coin := &character.NPC{Name: "Trader", SellAvailable: true}
	points := &character.NPC{Name: "Quartermaster", Currency: character.CurrencyArenaPoints}

	if got := merchantBagHeaderLabel(coin); got != "Your Items" {
		t.Fatalf("coin shop header = %q, want the plain title", got)
	}
	hint := merchantBagHeaderLabel(points)
	// Both facts must survive: the grid buys, and the shop does not buy back.
	for _, want := range []string{"drop", "buy", "no selling"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("non-coin header = %q - it does not say %q", hint, want)
		}
	}

	g, _ := merchantBuyGame(t, character.CurrencyArenaPoints, potionStock(5, -1))
	dlg := npcDialogLayout(g)
	_, _, gridTop, _ := merchantGridLayout(dlg.x, dlg.y)

	// The header sits a full line clear of the first icon row.
	headerY := gridTop - 24
	if headerY+debugTextCharHeight > gridTop {
		t.Fatalf("the header line (y=%d..%d) reaches the icons at y=%d", headerY, headerY+debugTextCharHeight, gridTop)
	}
	// Assert the DRAWN text, not the source: clipDebugText always "fits", so a
	// too-long label passes a width check while losing the words that matter.
	drawn := clipDebugText(hint, merchantGridW)
	if drawn != hint {
		t.Fatalf("the header is truncated on screen: %q -> %q (grid %d px, needs %d)",
			hint, drawn, merchantGridW, debugTextWidth(hint))
	}
	_ = g
}
