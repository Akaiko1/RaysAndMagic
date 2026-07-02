package game

import (
	"image"
	"image/color"
	"strings"

	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// The tavern stash: a cross-save shared chest. The keeper's "Manage your stash"
// choice opens this modal. Drag items between the party bag and the 8 chest
// cells; each transfer is persisted immediately (stash.json) and the game is
// autosaved so the bag side is committed too — keeping the two stores in step.

// stashDragFrom encodes the drag SOURCE in one int, decoded by decodeStashFrom:
// a chest cell (0..SlotCount-1), a card cell (+stashCardDragBase), or a bag index
// (+stashDragInvBase). The bases are disjoint (bag base is far above any real
// inventory size), so a single field addresses all three banks.
const (
	stashCardDragBase = 1000
	stashDragInvBase  = 100000
)

// Stash cell banks. kindBag addresses the party inventory (a slice, add/remove),
// the others address fixed cell arrays via stashCellPtr.
const (
	stashKindChest = iota
	stashKindCard
	stashKindBag
)

// stashAddr identifies one slot: a bank kind + index within it.
type stashAddr struct {
	kind int
	idx  int
}

const (
	stashCellSize    = 56
	stashCellGap     = 10
	stashInvCols     = 4
	stashInvRows     = 2
	stashInvMaxShown = stashInvCols * stashInvRows
)

// openStash lazy-loads the shared chest and shows the stash modal. Called from
// the tavern's "Manage your stash" action.
func (g *MMGame) openStash() {
	if g.stash == nil {
		s, err := stash.Load()
		if err != nil {
			g.AddCombatMessage("Could not open the stash.")
			return
		}
		for i := range s.Slots {
			if !stash.IsEmpty(s.Slots[i]) {
				normalizeItemFromConfig(&s.Slots[i])
			}
		}
		for i := range s.CardSlots {
			if !stash.IsEmpty(s.CardSlots[i]) {
				normalizeItemFromConfig(&s.CardSlots[i])
			}
		}
		g.stash = s
	}
	g.stashScreenOpen = true
	g.stashInvPage = 0
	g.stashShowCards = false
	g.clearStashDrag()
}

func (g *MMGame) clearStashDrag() {
	g.stashDragArmed = false
	g.stashDragActive = false
	g.stashDragDrop = false
	g.stashDragFrom = -1
	g.stashDragItem = items.Item{}
}

// commitStashTransfer persists a transfer that already mutated BOTH stores in
// memory — the chest (stash.json) and the party bag (autosave) — atomically:
// either both land on disk or neither does. The chest is written first; only if
// that succeeds is the bag autosaved. If EITHER write fails the in-memory move is
// rolled back (and the chest re-written to its pre-move state when the bag write
// is the one that failed), so a later reload can't dupe or lose the item. Returns
// false (and tells the player) on failure.
func (g *MMGame) commitStashTransfer(rollback func()) bool {
	if g.stash == nil {
		rollback()
		return false
	}
	if err := stash.Save(g.stash); err != nil {
		rollback() // chest never committed → just undo the in-memory move
		g.AddCombatMessage("Stash transfer failed - nothing was moved.")
		return false
	}
	if err := g.autosaveErr(); err != nil {
		rollback()              // bag write failed after the chest committed:
		_ = stash.Save(g.stash) // re-commit the chest at its pre-move state too
		g.AddCombatMessage("Stash transfer failed - nothing was moved.")
		return false
	}
	return true
}

// stashSnapshot captures both stores so a failed transfer can be rolled back
// wholesale (the chest Slots is a value array; the bag is a slice we copy).
func (g *MMGame) stashSnapshot() func() {
	invSnap := append([]items.Item(nil), g.party.Inventory...)
	slotsSnap := g.stash.Slots
	cardsSnap := g.stash.CardSlots
	return func() {
		g.party.Inventory = invSnap
		g.stash.Slots = slotsSnap
		g.stash.CardSlots = cardsSnap
	}
}

// updateStashDrag samples the raw mouse to drive the drag lifecycle while the
// stash is open. Source capture and drop resolution happen in drawStashScreen
// (where the cell rects are known), matching the quick-slot drag model.
func (ui *UISystem) updateStashDrag() {
	g := ui.game
	if !g.stashScreenOpen {
		if g.stashDragArmed || g.stashDragActive {
			g.clearStashDrag()
		}
		return
	}
	x, y := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.stashDragArmed = true
		g.stashDragActive = false
		g.stashDragDrop = false
		g.stashDragFrom = -1
		g.stashDragStartX, g.stashDragStartY = x, y
		g.stashDragCurX, g.stashDragCurY = x, y
		g.stashDragItem = items.Item{}
	}
	if g.stashDragArmed && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.stashDragCurX, g.stashDragCurY = x, y
		if !g.stashDragActive && (absInt(x-g.stashDragStartX) > quickDragThreshold || absInt(y-g.stashDragStartY) > quickDragThreshold) {
			g.stashDragActive = true
		}
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		g.stashDragCurX, g.stashDragCurY = x, y
		if g.stashDragActive && g.stashDragFrom >= 0 {
			g.stashDragDrop = true // Draw resolves against the cell under the cursor
		} else {
			g.clearStashDrag()
		}
		g.stashDragArmed = false
	}
}

// decodeStashFrom turns the encoded stashDragFrom into a slot address. The bag
// base sits far above the card base, so the bag test comes first.
func decodeStashFrom(from int) (stashAddr, bool) {
	switch {
	case from < 0:
		return stashAddr{}, false
	case from >= stashDragInvBase:
		return stashAddr{stashKindBag, from - stashDragInvBase}, true
	case from >= stashCardDragBase:
		return stashAddr{stashKindCard, from - stashCardDragBase}, true
	default:
		return stashAddr{stashKindChest, from}, true
	}
}

// stashCellPtr returns the backing item slot for a chest/card cell (nil for the
// bag, which is a slice handled separately, or an out-of-range index).
func (g *MMGame) stashCellPtr(a stashAddr) *items.Item {
	switch a.kind {
	case stashKindChest:
		if a.idx >= 0 && a.idx < len(g.stash.Slots) {
			return &g.stash.Slots[a.idx]
		}
	case stashKindCard:
		if a.idx >= 0 && a.idx < len(g.stash.CardSlots) {
			return &g.stash.CardSlots[a.idx]
		}
	}
	return nil
}

// stashAcceptsCardSlot reports whether an item may occupy a card slot: only a
// monster card, or nothing (empty). The card slots are the ONLY restricted bank.
func stashAcceptsCardSlot(it items.Item) bool {
	return stash.IsEmpty(it) || it.Type == items.ItemCard
}

// resolveStashDrop moves the carried thing to dst. Cell↔cell is a swap; bag↔cell
// moves with any displaced occupant returning to the bag. A card slot rejects
// anything that isn't a monster card (including the item a swap would push into it).
func (g *MMGame) resolveStashDrop(dst stashAddr) {
	src, ok := decodeStashFrom(g.stashDragFrom)
	if !ok || src == dst {
		return
	}
	rollback := g.stashSnapshot()
	switch {
	case src.kind != stashKindBag && dst.kind != stashKindBag: // cell <-> cell (swap)
		sp, dp := g.stashCellPtr(src), g.stashCellPtr(dst)
		if sp == nil || dp == nil {
			return
		}
		// After the swap *dp holds the carried item and *sp the displaced one;
		// a card slot on either end must end up holding a card.
		if (dst.kind == stashKindCard && !stashAcceptsCardSlot(*sp)) ||
			(src.kind == stashKindCard && !stashAcceptsCardSlot(*dp)) {
			g.AddCombatMessage("Only monster cards fit the card slots.")
			return
		}
		*sp, *dp = *dp, *sp
	case src.kind == stashKindBag: // bag -> cell
		idx := src.idx
		if idx < 0 || idx >= len(g.party.Inventory) {
			return
		}
		item := g.party.Inventory[idx]
		if dst.kind == stashKindCard && !stashAcceptsCardSlot(item) {
			g.AddCombatMessage("Only monster cards fit the card slots.")
			return
		}
		dp := g.stashCellPtr(dst)
		if dp == nil {
			return
		}
		occ := *dp
		g.party.RemoveItem(idx)
		*dp = item
		if !stash.IsEmpty(occ) {
			g.party.AddItem(occ) // displaced item returns to the bag
		}
	case dst.kind == stashKindBag: // cell -> bag
		sp := g.stashCellPtr(src)
		if sp == nil || stash.IsEmpty(*sp) {
			return
		}
		g.party.AddItem(*sp)
		*sp = items.Item{}
	default:
		return // bag -> bag: no-op
	}
	g.commitStashTransfer(rollback)
}

const (
	stashPopupW   = 560
	stashPopupH   = 440
	stashSubtitle = "Shared across all your saves. Drag items between your bag and the chest."
)

// stashSectionRows is the row count shared by the top storage grid and the bag
// grid (both stashInvCols wide). One place so layout + drop-scan can't drift.
const stashSectionRows = 2

// The top storage grid has a tab toggle (Items <-> Cards) where a pager would
// sit; it flips between the general chest slots and the card-only vault.
const (
	stashToggleW = 96
	stashToggleH = 18
)

// stashLayout holds the resolved anchor coordinates of the stash modal. Both the
// renderer and the layout-collision test derive their geometry from here so the
// two never drift.
type stashLayout struct {
	popupX, popupY     int
	centerX            int
	chestTop, invTop   int
	gridW, pagerY      int
	chestRows, bagRows int
}

func computeStashLayout(screenW, screenH int) stashLayout {
	popupX := (screenW - stashPopupW) / 2
	popupY := (screenH - stashPopupH) / 2
	var L stashLayout
	L.popupX, L.popupY = popupX, popupY
	L.centerX = popupX + stashPopupW/2
	L.chestTop = popupY + 76 // clear of the title + explanatory lines above
	L.invTop = L.chestTop + stashSectionRows*(stashCellSize+stashCellGap) + 30
	L.gridW = stashInvCols*stashCellSize + (stashInvCols-1)*stashCellGap
	L.pagerY = L.invTop + stashSectionRows*(stashCellSize+stashCellGap) + 8
	L.chestRows, L.bagRows = stashSectionRows, stashSectionRows
	return L
}

// stashToggleRect is the Items/Cards tab button, sitting at the right end of the
// top-storage heading row. Single-sourced so draw + collision test agree.
func stashToggleRect(L stashLayout) image.Rectangle {
	x := L.popupX + stashPopupW - stashToggleW - 16
	y := L.chestTop - 20
	return image.Rect(x, y, x+stashToggleW, y+stashToggleH)
}

// stashCellRect returns the rect of top-storage cell i (2 rows of stashInvCols).
func stashCellRect(originX, originY, i int) image.Rectangle {
	r, c := i/stashInvCols, i%stashInvCols
	gridW := stashInvCols*stashCellSize + (stashInvCols-1)*stashCellGap
	startX := originX - gridW/2
	x := startX + c*(stashCellSize+stashCellGap)
	y := originY + r*(stashCellSize+stashCellGap)
	return image.Rect(x, y, x+stashCellSize, y+stashCellSize)
}

// drawStashScreen renders the chest modal: 8 chest cells over the party bag grid,
// with drag-and-drop between them. Clicks/drag are resolved inline (roster-screen
// convention). Closes on the X button or ESC.
func (ui *UISystem) drawStashScreen(screen *ebiten.Image) {
	g := ui.game
	if g.stash == nil {
		return
	}
	screenW := g.config.GetScreenWidth()
	screenH := g.config.GetScreenHeight()
	L := computeStashLayout(screenW, screenH)
	popupX, popupY := L.popupX, L.popupY
	popupW, popupH := stashPopupW, stashPopupH
	centerX := L.centerX

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 150})
	drawFilledRect(screen, popupX, popupY, popupW, popupH, color.RGBA{30, 30, 60, 244})
	drawRectBorder(screen, popupX, popupY, popupW, popupH, 2, color.RGBA{150, 110, 52, 230})
	drawDebugText(screen, "Tavern Stash", popupX+16, popupY+14)
	drawDebugText(screen, stashSubtitle, popupX+16, popupY+34)

	mouseX, mouseY := ebiten.CursorPosition()

	// Top storage: a tabbed grid. The Items tab shows the general chest; the Cards
	// tab shows the card-only vault (violet-tinted). A toggle where a pager would
	// sit flips the tab in place of numeric pagination.
	chestTop := L.chestTop
	cards := g.stashShowCards
	heading := "Chest"
	kind := stashKindChest
	count := stash.SlotCount
	hoverBorder := color.RGBA{210, 170, 80, 230}
	if cards {
		heading = "Card Vault - cards only"
		kind = stashKindCard
		count = stash.CardSlotCount
		hoverBorder = color.RGBA{190, 120, 220, 235}
	}
	drawCenteredDebugText(screen, heading, popupX, chestTop-16, popupW, 14)
	ui.drawStashTabToggle(screen, L, mouseX, mouseY)
	for i := 0; i < count; i++ {
		r := stashCellRect(centerX, chestTop, i)
		var it items.Item
		from := i
		if cards {
			it = g.stash.CardSlots[i]
			from = stashCardDragBase + i
			ui.stashCardSource(i, r)
		} else {
			it = g.stash.Slots[i]
			ui.stashCellSource(i, r)
		}
		ui.drawStashCell(screen, it, r, g.stashDragActive && g.stashDragFrom == from, cards)
		if ptInRect(mouseX, mouseY, r) {
			drawRectBorder(screen, r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, 2, hoverBorder)
		}
		ui.stashCellTooltip(it, r, mouseX, mouseY)
		if g.stashDragDrop && g.stashDragFrom >= 0 && ptInRect(g.stashDragCurX, g.stashDragCurY, r) {
			g.resolveStashDrop(stashAddr{kind, i})
		}
	}

	// Party bag grid (bottom), paginated.
	invTop := L.invTop
	drawCenteredDebugText(screen, "Your Bag", popupX, invTop-16, popupW, 14)
	invPages := pageCount(len(g.party.Inventory), stashInvMaxShown)
	if g.stashInvPage >= invPages {
		g.stashInvPage = invPages - 1
	}
	if g.stashInvPage < 0 {
		g.stashInvPage = 0
	}
	invStart := g.stashInvPage * stashInvMaxShown
	gridW := stashInvCols*stashCellSize + (stashInvCols-1)*stashCellGap
	for slot := 0; slot < stashInvMaxShown; slot++ {
		idx := invStart + slot
		r, c := slot/stashInvCols, slot%stashInvCols
		x := centerX - gridW/2 + c*(stashCellSize+stashCellGap)
		y := invTop + r*(stashCellSize+stashCellGap)
		cell := image.Rect(x, y, x+stashCellSize, y+stashCellSize)
		var it items.Item
		has := idx >= 0 && idx < len(g.party.Inventory)
		if has {
			it = g.party.Inventory[idx]
			ui.stashInvSource(idx, cell)
		}
		ui.drawStashCell(screen, it, cell, g.stashDragActive && g.stashDragFrom == stashDragInvBase+idx, false)
		if ptInRect(mouseX, mouseY, cell) {
			drawRectBorder(screen, cell.Min.X-2, cell.Min.Y-2, cell.Dx()+4, cell.Dy()+4, 2, color.RGBA{120, 200, 120, 220})
		}
		ui.stashCellTooltip(it, cell, mouseX, mouseY)
		// Dropping onto any bag cell returns a carried chest/card item to the bag.
		if g.stashDragDrop && g.stashDragFrom >= 0 && ptInRect(g.stashDragCurX, g.stashDragCurY, cell) {
			g.resolveStashDrop(stashAddr{stashKindBag, idx})
		}
	}
	pagerY := L.pagerY
	ui.drawPager(screen, centerX-gridW/2, pagerY, gridW, &g.stashInvPage, invPages, true)

	// ESC is handled in the Update input loop (edge-tracked) so it closes the
	// modal without leaking to the menu-open handler; here only the close button
	// (click-inert while a drag is in flight).
	if ui.drawPopupCloseButton(screen, popupX+popupW-36, popupY+10, 24, !g.stashDragActive) {
		g.stashScreenOpen = false
		g.clearStashDrag()
	}

	// Carried icon, drawn last so it floats above everything; then clear the drop.
	if g.stashDragActive && g.stashDragFrom >= 0 {
		const sz = 48
		ui.drawInventoryItemIcon(screen, g.stashDragItem, g.stashDragCurX-sz/2, g.stashDragCurY-sz/2, sz, sz, 0, true)
	}
	if g.stashDragDrop {
		g.clearStashDrag()
	}
}

// stashCellSource captures a chest cell as a drag source.
func (ui *UISystem) stashCellSource(i int, r image.Rectangle) {
	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	if stash.IsEmpty(g.stash.Slots[i]) || !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	g.stashDragFrom = i
	g.stashDragItem = g.stash.Slots[i]
}

// stashInvSource captures a bag cell as a drag source.
func (ui *UISystem) stashInvSource(idx int, r image.Rectangle) {
	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	if idx < 0 || idx >= len(g.party.Inventory) || !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	g.stashDragFrom = stashDragInvBase + idx
	g.stashDragItem = g.party.Inventory[idx]
}

// drawStashCell draws one slot: the item icon when filled, an empty frame
// otherwise. hidden suppresses the icon for the cell currently being carried out.
// card tints the frame violet so the card-only bank reads apart from the chest.
func (ui *UISystem) drawStashCell(screen *ebiten.Image, it items.Item, r image.Rectangle, hidden, card bool) {
	fill := color.RGBA{20, 20, 38, 230}
	border := color.RGBA{90, 90, 130, 200}
	if card {
		fill = color.RGBA{40, 22, 52, 230}
		border = color.RGBA{150, 90, 190, 210}
	}
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fill)
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 1, border)
	if !stash.IsEmpty(it) && !hidden {
		ui.drawInventoryItemIcon(screen, it, r.Min.X+2, r.Min.Y+2, r.Dx()-4, r.Dy()-4, 0, true)
	}
}

// drawStashTabToggle draws the Items/Cards tab button and flips the tab on click.
// It reads "Cards >" on the items tab and "< Items" on the cards tab. Disabled
// while a drag is in flight so the source tab can't change mid-transfer.
func (ui *UISystem) drawStashTabToggle(screen *ebiten.Image, L stashLayout, mouseX, mouseY int) {
	g := ui.game
	rt := stashToggleRect(L)
	label := "Cards >"
	base := color.RGBA{70, 60, 110, 220}
	if g.stashShowCards {
		label = "< Items"
		base = color.RGBA{96, 64, 120, 230}
	}
	if ptInRect(mouseX, mouseY, rt) {
		base.R, base.G, base.B = base.R+30, base.G+30, base.B+30
	}
	drawFilledRect(screen, rt.Min.X, rt.Min.Y, rt.Dx(), rt.Dy(), base)
	drawRectBorder(screen, rt.Min.X, rt.Min.Y, rt.Dx(), rt.Dy(), 1, color.RGBA{170, 140, 200, 230})
	drawCenteredDebugText(screen, label, rt.Min.X, rt.Min.Y+1, rt.Dx(), rt.Dy()-2)
	if !g.stashDragActive && g.consumeLeftClickIn(rt.Min.X, rt.Min.Y, rt.Max.X, rt.Max.Y) {
		g.stashShowCards = !g.stashShowCards
		g.clearStashDrag()
	}
}

// stashCellTooltip queues the item tooltip for a hovered, filled cell (chest,
// card, or bag) — matching the inventory: nothing shows while a drag is active.
func (ui *UISystem) stashCellTooltip(it items.Item, cell image.Rectangle, mouseX, mouseY int) {
	g := ui.game
	if g.stashDragActive || stash.IsEmpty(it) || !ptInRect(mouseX, mouseY, cell) {
		return
	}
	char := g.party.Members[g.selectedChar]
	tip := GetItemTooltip(it, char, g.combat, tooltipDetailHeld())
	if tip == "" {
		return
	}
	lines := strings.Split(tip, "\n")
	plate, titleText := ui.itemTitleColors(it)
	var bodyColors []color.Color
	if titleText != nil {
		bodyColors = ui.rarityBodyColors(it, len(lines))
	}
	ui.queueTitledTooltipIcon(lines, bodyColors, plate, titleText, itemTooltipIconName(it), mouseX+16, mouseY+8)
}

// stashCardSource captures a card cell as a drag source.
func (ui *UISystem) stashCardSource(i int, r image.Rectangle) {
	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	if stash.IsEmpty(g.stash.CardSlots[i]) || !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	g.stashDragFrom = stashCardDragBase + i
	g.stashDragItem = g.stash.CardSlots[i]
}
