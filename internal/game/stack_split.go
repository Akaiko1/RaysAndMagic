package game

import (
	"fmt"
	"image"
	"image/color"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
)

type stackSplitPickerSource uint8

const (
	stackSplitPickerInventory stackSplitPickerSource = iota + 1
	stackSplitPickerStash
	// stackSplitPickerMerchantSell reuses the picker as the drag-to-sell
	// quantity dialog: same bag-indexed source, Sell instead of Pick up.
	stackSplitPickerMerchantSell
	// stackSplitPickerMerchantBuy is its mirror on the shop side: from indexes
	// the VISIBLE stock, and the quantity defaults to the most the party can
	// take right now (stock and purse permitting).
	stackSplitPickerMerchantBuy
)

// stackSplitIsMerchantTrade reports the two picker modes that trade with a
// merchant rather than splitting a carried stack.
func stackSplitIsMerchantTrade(source stackSplitPickerSource) bool {
	return source == stackSplitPickerMerchantSell || source == stackSplitPickerMerchantBuy
}

// shiftModifierHeld is the shared Shift state for UI modifiers: stack splitting
// in inventory surfaces and party-focus toggling in the unobstructed game HUD.
func shiftModifierHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// stackSplitPickerState holds only UI-transient selection state. The actual
// item mutation remains in Party.TakeStackUnits / Item.SplitOff at drop time.
type stackSplitPickerState struct {
	open     bool
	source   stackSplitPickerSource
	from     int // inventory index or encoded stash source
	quantity int
	// typed flips on the first digit key so typing REPLACES the prefilled
	// quantity, then appends - the "write how many" convention.
	typed    bool
	name     string
	itemType items.ItemType
	id       uint64
}

const (
	stackSplitPickerW = 280
	stackSplitPickerH = 144
)

// stackSplitPickerWidgets is the picker's inner geometry - the ONE source for
// both the draw pass and its layout tests, so a moved button cannot drift from
// its hitbox or slide under a text band.
type stackSplitPickerWidgets struct {
	minus, half, plus image.Rectangle
	take, cancel      image.Rectangle
	quantity, price   image.Rectangle
}

func stackSplitPickerLayout(r image.Rectangle) stackSplitPickerWidgets {
	return stackSplitPickerWidgets{
		minus:  image.Rect(r.Min.X+24, r.Min.Y+48, r.Min.X+56, r.Min.Y+72),
		half:   image.Rect(r.Min.X+178, r.Min.Y+48, r.Min.X+216, r.Min.Y+72),
		plus:   image.Rect(r.Min.X+224, r.Min.Y+48, r.Min.X+256, r.Min.Y+72),
		take:   image.Rect(r.Min.X+24, r.Min.Y+102, r.Min.X+136, r.Min.Y+128),
		cancel: image.Rect(r.Min.X+144, r.Min.Y+102, r.Min.X+256, r.Min.Y+128),
		// The band between - and 1/2 carries the quantity only; the price gets
		// its own full-width line below the buttons.
		quantity: image.Rect(r.Min.X+64, r.Min.Y+53, r.Min.X+64+104, r.Min.Y+53+18),
		price:    image.Rect(r.Min.X+12, r.Min.Y+80, r.Max.X-12, r.Min.Y+80+16),
	}
}

func stackSplitPickerRect(screenW, screenH int) image.Rectangle {
	x := (screenW - stackSplitPickerW) / 2
	y := (screenH - stackSplitPickerH) / 2
	return image.Rect(x, y, x+stackSplitPickerW, y+stackSplitPickerH)
}

func (ui *UISystem) openStackSplitPicker(source stackSplitPickerSource, from int, item items.Item) {
	// A SPLIT needs a stack to split; a SALE opens for any single item too -
	// the picker doubles as its confirmation dialog.
	if !stackSplitSourceAccepts(source, item) {
		return
	}
	quantity := item.Count() / 2
	if quantity < 1 {
		quantity = 1
	}
	ui.stackSplitPicker = stackSplitPickerState{
		open: true, source: source, from: from, quantity: quantity,
		name: item.Name, itemType: item.Type, id: item.InstanceID,
	}
	ui.inventoryContextOpen = false
}

func (ui *UISystem) closeStackSplitPicker() {
	ui.stackSplitPicker = stackSplitPickerState{}
}

// stackSplitSourceAccepts is the ONE eligibility rule shared by the picker's
// open guard and its per-frame source validation: split sources need a real
// stack, the merchant sale accepts any item (quantity 1 = a confirm dialog).
func stackSplitSourceAccepts(source stackSplitPickerSource, item items.Item) bool {
	if stackSplitIsMerchantTrade(source) {
		return item.Count() >= 1
	}
	return item.Stackable() && item.Count() >= 2
}

// stackSplitStockEntry resolves the merchant stock entry a buy picker points
// at, revalidated every frame (stock shrinks, tabs flip).
func (ui *UISystem) stackSplitStockEntry() *character.MerchantStockItem {
	s := ui.stackSplitPicker
	if s.source != stackSplitPickerMerchantBuy {
		return nil
	}
	stock := ui.game.merchantVisibleStock()
	if s.from < 0 || s.from >= len(stock) {
		return nil
	}
	entry := stock[s.from]
	if entry == nil || entry.Item.Name != s.name || entry.Item.Type != s.itemType {
		return nil
	}
	return entry
}

// stackSplitMaxQuantity is the picker's upper clamp: a SPLIT never takes the
// whole entry (a full take is the ordinary drag), a SALE may sell it all.
func (ui *UISystem) stackSplitMaxQuantity(item items.Item) int {
	switch ui.stackSplitPicker.source {
	case stackSplitPickerMerchantSell:
		return item.Count()
	case stackSplitPickerMerchantBuy:
		return max(1, ui.game.merchantMaxUnits(ui.stackSplitStockEntry()))
	default:
		return item.Count() - 1
	}
}

// stackSplitSetQuantity is the ONE mutation point for the picker's quantity:
// buttons, keyboard arrows, and typed digits all clamp through it.
func (ui *UISystem) stackSplitSetQuantity(q int) {
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		return
	}
	if maxQ := ui.stackSplitMaxQuantity(item); q > maxQ {
		q = maxQ
	}
	if q < 1 {
		q = 1
	}
	ui.stackSplitPicker.quantity = q
}

func (ui *UISystem) stackSplitAdjust(delta int) {
	ui.stackSplitSetQuantity(ui.stackSplitPicker.quantity + delta)
}

// stackSplitAppendDigit types into the quantity: the first digit replaces the
// prefilled value, later digits append. A LEADING zero is not a number - it
// would clamp to 1 and then turn the next digit into a teen ("0", "3" must read
// 3, not 13) - so it neither writes nor opens the append run.
func (ui *UISystem) stackSplitAppendDigit(d int) {
	if ui.stackSplitPicker.typed {
		ui.stackSplitSetQuantity(ui.stackSplitPicker.quantity*10 + d)
		return
	}
	if d == 0 {
		return
	}
	ui.stackSplitSetQuantity(d)
	ui.stackSplitPicker.typed = true
}

// stackSplitBackspace deletes the last typed digit. Erasing the final digit
// leaves the clamped minimum 1 on screen but ENDS the typing run, so the next
// digit starts a fresh number instead of appending to it ("7", Backspace, "3"
// must read 3, not 13).
func (ui *UISystem) stackSplitBackspace() {
	shortened := ui.stackSplitPicker.quantity / 10
	ui.stackSplitSetQuantity(shortened)
	if shortened < 1 {
		ui.stackSplitPicker.typed = false
	}

}

// stackSplitConfirm executes the picker: a merchant sale sells the chosen
// units outright; split sources pick the fragment up onto the cursor.
func (ui *UISystem) stackSplitConfirm() {
	if !ui.stackSplitPicker.open {
		return
	}
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		ui.closeStackSplitPicker()
		return
	}
	s := ui.stackSplitPicker
	switch s.source {
	case stackSplitPickerMerchantSell:
		ui.closeStackSplitPicker()
		ui.game.sellInventoryUnits(s.from, s.quantity)
	case stackSplitPickerMerchantBuy:
		entry := ui.stackSplitStockEntry()
		ui.closeStackSplitPicker()
		ui.game.buyMerchantUnits(entry, s.quantity)
	default:
		ui.beginPickedUpStackSplit(item)
	}
}

// beginMerchantSellPicker opens the drag-to-sell dialog for bag entry idx.
// EVERY sale confirms here - a single sword is one misdrop from gone
// otherwise. Stacks default to the FULL stack, singles read as Sell/Cancel.
func (ui *UISystem) beginMerchantSellPicker(idx int) {
	g := ui.game
	if idx < 0 || idx >= len(g.party.Inventory) {
		return
	}
	item := g.party.Inventory[idx]
	if item.Attributes["value"] <= 0 {
		g.AddCombatMessage("This item has no value.")
		return
	}
	ui.openStackSplitPicker(stackSplitPickerMerchantSell, idx, item)
	ui.stackSplitPicker.quantity = item.Count() // sale default: the whole stack
}

// beginMerchantBuyPicker opens the drag-to-buy dialog for visible stock entry
// idx. A purchase defaults to ONE unit - the safe direction for spending -
// while the attainable maximum is shown in the quantity line (x1/7).
func (ui *UISystem) beginMerchantBuyPicker(idx int) {
	stock := ui.game.merchantVisibleStock()
	if idx < 0 || idx >= len(stock) || stock[idx] == nil {
		return
	}
	entry := stock[idx]
	if !entry.InStock() {
		ui.game.AddCombatMessage("That item is sold out.")
		return
	}
	if ui.game.merchantMaxUnits(entry) < 1 {
		// Unaffordable: let the shared purchase body say why, in its own words.
		ui.game.buyMerchantUnits(entry, 1)
		return
	}
	ui.openStackSplitPicker(stackSplitPickerMerchantBuy, idx, entry.Item)
	ui.stackSplitPicker.quantity = 1
}

// stackSplitInteractionActive exposes the UI-owned transient state to input
// handling. Keyboard navigation must not switch tabs, close the panel or move
// the party while a modal picker or its carried fragment owns the next click.
func (g *MMGame) stackSplitInteractionActive() bool {
	if g == nil || g.gameLoop == nil || g.gameLoop.ui == nil {
		return false
	}
	return g.gameLoop.ui.stackSplitPicker.open || g.dragPickedUp || g.stashDragPickedUp
}

func (g *MMGame) cancelStackSplitInteraction() {
	if g == nil {
		return
	}
	if g.gameLoop != nil && g.gameLoop.ui != nil {
		g.gameLoop.ui.closeStackSplitPicker()
	}
	if g.dragPickedUp {
		g.clearDrag()
	}
	if g.stashDragPickedUp {
		g.clearStashDrag()
	}
}

// stackSplitPickerItem resolves and validates the source held by the picker.
// No other bag mutation is allowed while the picker is open, but validating the
// item identity still makes stale UI state harmless.
func (ui *UISystem) stackSplitPickerItem() (items.Item, bool) {
	s := ui.stackSplitPicker
	if !s.open {
		return items.Item{}, false
	}
	g := ui.game
	var item items.Item
	switch s.source {
	case stackSplitPickerMerchantBuy:
		entry := ui.stackSplitStockEntry()
		if entry == nil {
			return items.Item{}, false
		}
		item = entry.Item
	case stackSplitPickerInventory, stackSplitPickerMerchantSell:
		if s.from < 0 || s.from >= len(g.party.Inventory) {
			return items.Item{}, false
		}
		item = g.party.Inventory[s.from]
	case stackSplitPickerStash:
		src, ok := decodeStashFrom(s.from)
		if !ok {
			return items.Item{}, false
		}
		if src.kind == stashKindBag {
			if src.idx < 0 || src.idx >= len(g.party.Inventory) {
				return items.Item{}, false
			}
			item = g.party.Inventory[src.idx]
		} else {
			cell := g.stashCellPtr(src)
			if cell == nil || stash.IsEmpty(*cell) {
				return items.Item{}, false
			}
			item = *cell
		}
	default:
		return items.Item{}, false
	}
	if !stackSplitSourceAccepts(s.source, item) || item.Name != s.name || item.Type != s.itemType ||
		(s.id != 0 && item.InstanceID != s.id) {
		return items.Item{}, false
	}
	return item, true
}

func (ui *UISystem) beginPickedUpStackSplit(item items.Item) {
	s := ui.stackSplitPicker
	quantity := s.quantity
	if quantity < 1 || quantity >= item.Count() {
		ui.closeStackSplitPicker()
		return
	}
	item.Quantity = quantity
	ui.closeStackSplitPicker()
	g := ui.game
	x, y := ebiten.CursorPosition()
	switch s.source {
	case stackSplitPickerInventory:
		g.clearDrag()
		g.dragActive = true
		g.dragPickedUp = true
		g.dragSrc = dragFromInventory
		g.dragInvIndex = s.from
		g.dragSplitQuantity = quantity
		g.dragItem = item
		g.dragCurX, g.dragCurY = x, y
	case stackSplitPickerStash:
		g.clearStashDrag()
		g.stashDragActive = true
		g.stashDragPickedUp = true
		g.stashDragFrom = s.from
		g.stashDragSplitQuantity = quantity
		g.stashDragItem = item
		g.stashDragCurX, g.stashDragCurY = x, y
	}
}

func drawStackSplitButton(screen *ebiten.Image, r image.Rectangle, label string, hovered bool) {
	bg := color.RGBA{70, 50, 30, 230}
	if hovered {
		bg = color.RGBA{120, 90, 50, 240}
	}
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), bg)
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 1, color.RGBA{170, 130, 70, 235})
	drawCenteredDebugText(screen, label, r.Min.X, r.Min.Y+2, r.Dx(), r.Dy()-2)
}

func (ui *UISystem) drawStackSplitPicker(screen *ebiten.Image) {
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		ui.closeStackSplitPicker()
		return
	}
	g := ui.game
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	r := stackSplitPickerRect(screenW, screenH)
	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 125})
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{30, 30, 60, 248})
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 2, color.RGBA{170, 130, 70, 235})
	title := "Split "
	switch ui.stackSplitPicker.source {
	case stackSplitPickerMerchantSell:
		title = "Sell "
	case stackSplitPickerMerchantBuy:
		title = "Buy "
	}
	drawCenteredDebugText(screen, title+truncateRunes(item.Name, 28, "..."), r.Min.X+12, r.Min.Y+12, r.Dx()-24, 16)

	L := stackSplitPickerLayout(r)
	minus, plus, half, take, cancel := L.minus, L.plus, L.half, L.take, L.cancel
	mouseX, mouseY := ebiten.CursorPosition()
	// The band between the -/+ buttons is narrow, so it carries the QUANTITY
	// only; a price can be an item currency plus gold ("2 red dragon scale +
	// 20000 g") and gets its own full-width line under the buttons.
	confirmLabel := "Pick up"
	quantityText := fmt.Sprintf("x%d of x%d", ui.stackSplitPicker.quantity, item.Count())
	priceLine := ""
	switch ui.stackSplitPicker.source {
	case stackSplitPickerMerchantSell:
		confirmLabel = "Sell"
		quantityText = fmt.Sprintf("x%d/%d", ui.stackSplitPicker.quantity, ui.stackSplitMaxQuantity(item))
		priceLine = fmt.Sprintf("%d gold", g.merchantSellPrice(item.Attributes["value"])*ui.stackSplitPicker.quantity)
	case stackSplitPickerMerchantBuy:
		confirmLabel = "Buy"
		quantityText = fmt.Sprintf("x%d/%d", ui.stackSplitPicker.quantity, ui.stackSplitMaxQuantity(item))
		priceLine = merchantTotalPriceLabel(g, ui.stackSplitStockEntry(), ui.stackSplitPicker.quantity)
	}
	drawStackSplitButton(screen, minus, "-", ptInRect(mouseX, mouseY, minus))
	drawStackSplitButton(screen, half, "1/2", ptInRect(mouseX, mouseY, half))
	drawStackSplitButton(screen, plus, "+", ptInRect(mouseX, mouseY, plus))
	drawStackSplitButton(screen, take, confirmLabel, ptInRect(mouseX, mouseY, take))
	drawStackSplitButton(screen, cancel, "Cancel", ptInRect(mouseX, mouseY, cancel))
	drawCenteredDebugText(screen, quantityText, L.quantity.Min.X, L.quantity.Min.Y, L.quantity.Dx(), L.quantity.Dy())
	if priceLine != "" {
		// Own line, full panel width, clipped: a long item-currency price must
		// never crawl under the -, 1/2 and + buttons.
		drawCenteredDebugText(screen, clipDebugText(priceLine, L.price.Dx()), L.price.Min.X, L.price.Min.Y, L.price.Dx(), L.price.Dy())
		if debugTextWidth(priceLine) > L.price.Dx() && ptInRect(mouseX, mouseY, r) {
			ui.queueTooltip([]string{priceLine}, mouseX+12, mouseY+8)
		}
	}

	if ui.topModalLayer() != modalLayerStackSplit {
		return
	}
	ui.onDisplayedInput(uiCommandClick, layoutRect{minus.Min.X, minus.Min.Y, (minus.Max.X) - (minus.Min.X), (minus.Max.Y) - (minus.Min.Y)}, func() {
		if g.consumeLeftClickIn(minus.Min.X, minus.Min.Y, minus.Max.X, minus.Max.Y) {
			ui.stackSplitAdjust(-1)
			return
		}
	})
	ui.onDisplayedInput(uiCommandClick, layoutRect{plus.Min.X, plus.Min.Y, (plus.Max.X) - (plus.Min.X), (plus.Max.Y) - (plus.Min.Y)}, func() {
		if g.consumeLeftClickIn(plus.Min.X, plus.Min.Y, plus.Max.X, plus.Max.Y) {
			ui.stackSplitAdjust(1)
			return
		}
	})
	ui.onDisplayedInput(uiCommandClick, layoutRect{half.Min.X, half.Min.Y, (half.Max.X) - (half.Min.X), (half.Max.Y) - (half.Min.Y)}, func() {
		if g.consumeLeftClickIn(half.Min.X, half.Min.Y, half.Max.X, half.Max.Y) {
			// A trade halves the PICKER's maximum (a buy's item is a one-unit shelf
			// template); a split halves the carried stack, as it always did.
			halfQ := item.Count() / 2
			if stackSplitIsMerchantTrade(ui.stackSplitPicker.source) {
				halfQ = ui.stackSplitMaxQuantity(item) / 2
			}
			ui.stackSplitSetQuantity(halfQ)
			return
		}
	})
	ui.onDisplayedInput(uiCommandClick, layoutRect{take.Min.X, take.Min.Y, (take.Max.X) - (take.Min.X), (take.Max.Y) - (take.Min.Y)}, func() {
		if g.consumeLeftClickIn(take.Min.X, take.Min.Y, take.Max.X, take.Max.Y) {
			ui.stackSplitConfirm()
			return
		}
	})
	// Cancel is a BUTTON (and ESC), never a stray click on the dim: this dialog
	// confirms a purchase or a sale, so an accidental press must not dismiss it
	// and leave the player wondering whether the trade went through.
	ui.onDisplayedInput(uiCommandClick, layoutRect{cancel.Min.X, cancel.Min.Y, (cancel.Max.X) - (cancel.Min.X), (cancel.Max.Y) - (cancel.Min.Y)}, func() {
		if g.consumeLeftClickIn(cancel.Min.X, cancel.Min.Y, cancel.Max.X, cancel.Max.Y) {
			ui.closeStackSplitPicker()
		}
	})
}
