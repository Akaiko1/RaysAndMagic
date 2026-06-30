package game

import (
	"image"
	"image/color"

	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// The tavern stash: a cross-save shared chest. The keeper's "Manage your stash"
// choice opens this modal. Drag items between the party bag and the 8 chest
// cells; each transfer is persisted immediately (stash.json) and the game is
// autosaved so the bag side is committed too — keeping the two stores in step.

// stashDragInvBase offsets inventory indices in stashDragFrom so a single field
// can encode either a chest cell (0..SlotCount-1) or a bag index (+ base).
const stashDragInvBase = 100000

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
		g.stash = s
	}
	g.stashScreenOpen = true
	g.stashInvPage = 0
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
		g.AddCombatMessage("Stash transfer failed — nothing was moved.")
		return false
	}
	if err := g.autosaveErr(); err != nil {
		rollback()              // bag write failed after the chest committed:
		_ = stash.Save(g.stash) // re-commit the chest at its pre-move state too
		g.AddCombatMessage("Stash transfer failed — nothing was moved.")
		return false
	}
	return true
}

// stashSnapshot captures both stores so a failed transfer can be rolled back
// wholesale (the chest Slots is a value array; the bag is a slice we copy).
func (g *MMGame) stashSnapshot() func() {
	invSnap := append([]items.Item(nil), g.party.Inventory...)
	slotsSnap := g.stash.Slots
	return func() {
		g.party.Inventory = invSnap
		g.stash.Slots = slotsSnap
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

// resolveStashDropToCell moves the carried thing into chest cell t.
func (g *MMGame) resolveStashDropToCell(t int) {
	from := g.stashDragFrom
	rollback := g.stashSnapshot()
	switch {
	case from >= 0 && from < stash.SlotCount: // chest -> chest: swap
		if from != t {
			g.stash.Slots[from], g.stash.Slots[t] = g.stash.Slots[t], g.stash.Slots[from]
		}
	case from >= stashDragInvBase: // bag -> chest
		idx := from - stashDragInvBase
		if idx < 0 || idx >= len(g.party.Inventory) {
			return
		}
		item := g.party.Inventory[idx]
		g.party.RemoveItem(idx)
		occ := g.stash.Slots[t]
		g.stash.Slots[t] = item
		if !stash.IsEmpty(occ) {
			g.party.AddItem(occ) // displaced item returns to the bag
		}
	default:
		return
	}
	g.commitStashTransfer(rollback)
}

// resolveStashDropToBag moves a carried chest item back into the party bag.
func (g *MMGame) resolveStashDropToBag() {
	from := g.stashDragFrom
	if from < 0 || from >= stash.SlotCount {
		return // bag -> bag is a no-op
	}
	if stash.IsEmpty(g.stash.Slots[from]) {
		return
	}
	rollback := g.stashSnapshot()
	g.party.AddItem(g.stash.Slots[from])
	g.stash.Slots[from] = items.Item{}
	g.commitStashTransfer(rollback)
}

const (
	stashPopupW   = 560
	stashPopupH   = 440
	stashSubtitle = "Shared across all your saves. Drag items between your bag and the chest."
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
	L.invTop = L.chestTop + 2*(stashCellSize+stashCellGap) + 30
	L.gridW = stashInvCols*stashCellSize + (stashInvCols-1)*stashCellGap
	L.pagerY = L.invTop + 2*(stashCellSize+stashCellGap) + 8
	L.chestRows, L.bagRows = 2, 2
	return L
}

// stashCellRect returns the rect of chest cell i (2 rows of stashInvCols).
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

	// Chest cells (top). Pushed clear of the explanatory line above.
	chestTop := L.chestTop
	drawCenteredDebugText(screen, "Chest", popupX, chestTop-16, popupW, 14)
	for i := 0; i < stash.SlotCount; i++ {
		r := stashCellRect(centerX, chestTop, i)
		ui.stashCellSource(i, r)
		ui.drawStashCell(screen, g.stash.Slots[i], r, g.stashDragActive && g.stashDragFrom == i)
		if ptInRect(mouseX, mouseY, r) {
			drawRectBorder(screen, r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, 2, color.RGBA{210, 170, 80, 230})
		}
		if g.stashDragDrop && g.stashDragFrom >= 0 && ptInRect(g.stashDragCurX, g.stashDragCurY, r) {
			g.resolveStashDropToCell(i)
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
		ui.drawStashCell(screen, it, cell, g.stashDragActive && g.stashDragFrom == stashDragInvBase+idx)
		if ptInRect(mouseX, mouseY, cell) {
			drawRectBorder(screen, cell.Min.X-2, cell.Min.Y-2, cell.Dx()+4, cell.Dy()+4, 2, color.RGBA{120, 200, 120, 220})
		}
		// Dropping onto any bag cell returns a carried chest item to the bag.
		if g.stashDragDrop && g.stashDragFrom >= 0 && ptInRect(g.stashDragCurX, g.stashDragCurY, cell) {
			g.resolveStashDropToBag()
		}
	}
	pagerY := L.pagerY
	ui.drawPager(screen, centerX-gridW/2, pagerY, gridW, &g.stashInvPage, invPages, true)

	// Close button.
	closeX := popupX + popupW - 36
	closeY := popupY + 10
	if mouseX >= closeX && mouseX < closeX+24 && mouseY >= closeY && mouseY < closeY+24 {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{200, 60, 60, 220})
	} else {
		drawFilledRect(screen, closeX, closeY, 24, 24, color.RGBA{120, 60, 60, 180})
	}
	ui.drawInterfaceIcon(screen, "icon_close", closeX+2, closeY+2, 20, 20)
	// ESC is handled in the Update input loop (edge-tracked) so it closes the
	// modal without leaking to the menu-open handler; here only the close button.
	if !g.stashDragActive && g.consumeLeftClickIn(closeX, closeY, closeX+24, closeY+24) {
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
func (ui *UISystem) drawStashCell(screen *ebiten.Image, it items.Item, r image.Rectangle, hidden bool) {
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{20, 20, 38, 230})
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 1, color.RGBA{90, 90, 130, 200})
	if !stash.IsEmpty(it) && !hidden {
		ui.drawInventoryItemIcon(screen, it, r.Min.X+2, r.Min.Y+2, r.Dx()-4, r.Dy()-4, 0, true)
	}
}
