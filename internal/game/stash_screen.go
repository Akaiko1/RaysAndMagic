package game

import (
	"image"
	"image/color"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
)

// The tavern stash: a cross-save shared chest, shown whole beside a page of the
// party bag (the tavern's Stash tab, or this modal from the Storage Clerk).
// Drag or double-click items between them; each transfer is persisted
// immediately (stash.json) and the game is autosaved so the bag side is
// committed too - keeping the two stores in step.

// stashDragFrom encodes the drag SOURCE in one int, decoded by decodeStashFrom:
// a chest cell (0..SlotCount-1), a bag index (+stashDragInvBase), or a merchant
// shelf index (+stashShopDragBase). The bases are disjoint (each far above any
// real bank size), so a single field addresses every source the shared drag
// machine serves.
const (
	stashDragInvBase  = 100000
	stashShopDragBase = 200000
)

// Stash cell banks. kindBag addresses the party inventory (a slice, add/remove),
// kindShop the open merchant's visible stock (buy source only, never a drop
// target), kindChest the chest cells via stashCellPtr.
const (
	stashKindChest = iota
	stashKindBag
	stashKindShop
)

// stashAddr identifies one slot: a bank kind + index within it.
type stashAddr struct {
	kind int
	idx  int
}

// openStash lazy-loads the shared chest and shows the stash modal. Called from
// the tavern's "Manage your stash" action.
func (g *MMGame) openStash() bool {
	if !g.ensureStashLoaded() {
		g.AddCombatMessage("Could not open the stash.")
		return false
	}
	g.stashScreenOpen = true
	g.stashInvPage = 0
	g.clearStashDrag()
	return true
}

func (g *MMGame) clearStashDrag() {
	g.stashDragArmed = false
	g.stashDragActive = false
	g.stashDragDrop = false
	g.stashDragFrom = -1
	g.stashDragItem = items.Item{}
	g.stashDragSplitQuantity = 0
	g.stashDragPickedUp = false
}

// commitStashTransfer commits a mutation that already changed the in-memory bag
// and chest. A durable journal makes the independent stash and autosave files
// recoverable as one transfer after a crash or write failure.
func (g *MMGame) commitStashTransfer(snapshot stashTransferSnapshot) bool {
	// A chest cell-to-cell move changes neither the bag length nor the gold, so
	// the redraw barrier needs an explicit revision. Bumped on entry: even the
	// autosave-failure branch can leave memory on the committed state, and a
	// spurious bump on a full rollback only costs one conservative frame.
	g.bumpModalContentRev()
	if g.stash == nil {
		snapshot.restore(g)
		return false
	}
	before := snapshot.stash()
	journal := &stash.TransferJournal{
		ID:     strconv.FormatUint(items.NewInstanceID(), 10),
		Before: before,
		After:  *g.stash,
	}
	if err := stash.SaveTransferJournal(journal); err != nil {
		snapshot.restore(g)
		g.AddCombatMessage("Stash transfer failed - nothing was moved.")
		return false
	}
	if err := stash.Save(g.stash); err != nil {
		snapshot.restore(g) // chest never committed -> just undo the in-memory move
		_ = stash.ClearTransferJournal()
		g.AddCombatMessage("Stash transfer failed - nothing was moved.")
		return false
	}
	g.pendingStashTransferID = journal.ID
	if err := g.autosaveErr(); err != nil {
		// Do not roll memory back until the old chest snapshot is safely back on
		// disk. If that write also fails, memory stays on the committed state and
		// the prepared journal repairs it on the next stash access.
		if restoreErr := stash.Save(&before); restoreErr == nil {
			snapshot.restore(g)
			g.pendingStashTransferID = ""
			_ = stash.ClearTransferJournal()
			g.AddCombatMessage("Stash transfer failed - nothing was moved.")
		} else {
			g.AddCombatMessage("Stash saved, but autosave failed. Do not quit; save again.")
		}
		return false
	}
	// Keep the marker for later autosaves if journal cleanup itself fails. That
	// lets recovery select the committed after snapshot instead of rolling back.
	if err := stash.ClearTransferJournal(); err == nil {
		g.pendingStashTransferID = ""
	}
	return true
}

// stashTransferSnapshot captures the pre-transfer in-memory state. The stack
// API replaces lineage slices rather than mutating them in place, so these value
// copies remain valid for rollback and durable journal recovery.
type stashTransferSnapshot struct {
	inventory []items.Item
	chest     stash.Stash
}

func (g *MMGame) stashSnapshot() stashTransferSnapshot {
	return stashTransferSnapshot{
		inventory: append([]items.Item(nil), g.party.Inventory...),
		chest:     g.stash.Clone(),
	}
}

func (snapshot stashTransferSnapshot) stash() stash.Stash { return snapshot.chest.Clone() }

func (snapshot stashTransferSnapshot) restore(g *MMGame) {
	g.party.Inventory = snapshot.inventory
	*g.stash = snapshot.chest.Clone()
}

// finishPendingStashTransfer retries the party-side autosave after the rare
// case where the chest write succeeded but its compensating rollback failed.
// New mutations must wait until that durable journal has a decision.
func (g *MMGame) finishPendingStashTransfer() bool {
	if g.pendingStashTransferID == "" {
		return true
	}
	if err := g.autosaveErr(); err != nil {
		g.AddCombatMessage("Stash autosave is still failing. Try again before moving more items.")
		return false
	}
	if err := stash.ClearTransferJournal(); err != nil {
		g.AddCombatMessage("Stash is waiting for journal cleanup. Try again before moving more items.")
		return false
	}
	g.pendingStashTransferID = ""
	return true
}

// updateStashDrag samples the raw mouse to drive the drag lifecycle while the
// stash is open. Source capture and drop resolution happen in drawStashScreen
// (where the cell rects are known), matching the quick-slot drag model.
// updateStashDrag advances the shared drag machine for the stash and merchant
// surfaces. While it runs, left clicks are RELEASE-driven: a press only arms a
// gesture, and only a release that never crossed the drag threshold becomes a
// click (queued here with the press coordinates). Without that arbitration the
// press that STARTS a drag is also consumed as a click by the Update-side
// handlers, so beginning a drag off a selected item would buy or sell a unit
// before the drag was even recognized.
func (ui *UISystem) updateStashDrag(now int64) bool {
	g := ui.game
	if g.stashDragPickedUp {
		if !g.stashDragSurfaceOpen() {
			g.clearStashDrag()
			return false
		}
		g.stashDragCurX, g.stashDragCurY = pointerPosition()
		if pointerLeftJustPressed() {
			g.stashDragDrop = true
			return true // destination click is a drag drop, never a stash click
		}
		return false
	}
	if ui.stackSplitPicker.open {
		return false
	}
	if !g.stashDragSurfaceOpen() {
		if g.stashDragArmed || g.stashDragActive {
			g.clearStashDrag()
		}
		return false
	}
	x, y := pointerPosition()
	if pointerLeftJustPressed() {
		g.stashDragArmed = true
		g.stashDragActive = false
		g.stashDragDrop = false
		g.stashDragFrom = -1
		g.stashDragStartX, g.stashDragStartY = x, y
		g.stashDragCurX, g.stashDragCurY = x, y
		g.stashDragItem = items.Item{}
		g.stashDragSplitQuantity = 0
	}
	if g.stashDragArmed && pointerLeftPressed() {
		g.stashDragCurX, g.stashDragCurY = x, y
		if !g.stashDragActive && (absInt(x-g.stashDragStartX) > quickDragThreshold || absInt(y-g.stashDragStartY) > quickDragThreshold) {
			g.stashDragActive = true
		}
	}
	if pointerLeftJustRelease() {
		wasArmed := g.stashDragArmed
		g.stashDragCurX, g.stashDragCurY = x, y
		if g.stashDragActive && g.stashDragFrom >= 0 {
			g.stashDragDrop = true // Update resolves against the displayed cell
		} else {
			if wasArmed && !g.stashDragActive {
				// A press that never became a drag IS the click - queue it now,
				// at the point it started, so grids act on completed gestures.
				g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: g.stashDragStartX, y: g.stashDragStartY, at: now})
				g.mouseLeftClickX, g.mouseLeftClickY = g.stashDragStartX, g.stashDragStartY
			}
			g.clearStashDrag()
		}
		g.stashDragArmed = false
	}
	return false
}

// stashDragSurfaceOpen is every surface the stash drag state machine serves:
// the stash manager (standalone or tavern tab) and the merchant dialog's
// drag-to-sell grid, which reuses the same bag-cell drag.
func (g *MMGame) stashDragSurfaceOpen() bool {
	return g.stashInteractionOpen() || g.merchantDragOpen()
}

// stashInteractionOpen includes the embedded tavern Stash tab as well as the
// legacy standalone stash screen. The drag state machine must see both as the
// same working surface or it clears a drag before the tab can resolve it.
func (g *MMGame) stashInteractionOpen() bool {
	if g.stashScreenOpen {
		return true
	}
	if !g.dialogActive || g.npcDialogKindFor(g.dialogNPC) != dialogKindTavern {
		return false
	}
	tab, ok := g.activeTavernTab(g.dialogNPC)
	return ok && tab.action == tavernStashAction
}

// decodeStashFrom turns the encoded stashDragFrom into a slot address.
func decodeStashFrom(from int) (stashAddr, bool) {
	switch {
	case from < 0:
		return stashAddr{}, false
	case from >= stashShopDragBase:
		return stashAddr{stashKindShop, from - stashShopDragBase}, true
	case from >= stashDragInvBase:
		return stashAddr{stashKindBag, from - stashDragInvBase}, true
	default:
		return stashAddr{stashKindChest, from}, true
	}
}

// stashCellPtr returns the backing item slot for a chest cell (nil for the
// bag, which is a slice handled separately, or an out-of-range index).
func (g *MMGame) stashCellPtr(a stashAddr) *items.Item {
	if a.kind == stashKindChest && a.idx >= 0 && a.idx < len(g.stash.Slots) {
		return &g.stash.Slots[a.idx]
	}
	return nil
}

// resolveStashDrop moves the carried thing to dst.
func (g *MMGame) resolveStashDrop(dst stashAddr) {
	if src, ok := decodeStashFrom(g.stashDragFrom); ok {
		g.moveStashItem(src, dst, g.stashDragSplitQuantity)
	}
}

// quickMoveStashItem is the double-click transfer: a chest item goes to the
// bag, stacking automatically; a bag item goes whole onto its matching chest
// stack, else into the first free cell.
func (g *MMGame) quickMoveStashItem(src stashAddr) {
	if src.kind == stashKindChest {
		g.moveStashItem(src, stashAddr{stashKindBag, stashBagAutomatic}, 0)
		return
	}
	if src.kind != stashKindBag || src.idx < 0 || src.idx >= len(g.party.Inventory) {
		return
	}
	it := g.party.Inventory[src.idx]
	free := -1
	for i, cell := range g.stash.Slots {
		if items.SameStack(cell, it) {
			g.moveStashItem(src, stashAddr{stashKindChest, i}, 0)
			return
		}
		if free < 0 && stash.IsEmpty(cell) {
			free = i
		}
	}
	if free < 0 {
		g.AddCombatMessage(uitext.Text("stash.chest_full"))
		return
	}
	g.moveStashItem(src, stashAddr{stashKindChest, free}, 0)
}

// A bag destination that stacks automatically (Bag.Place with a negative cell).
const stashBagAutomatic = -1

// moveStashItem moves quantity units of src to dst, or the whole item when
// quantity is 0, under the inventory's placement rule (items.DropOnto): onto a
// matching stack it merges, an empty cell keeps its own stack, a whole item
// trades cells with any other item, and part of a stack onto another item
// moves nothing. The automatic bag destination stacks like loot. A chest
// change is one journalled transfer; a bag-only move is ordinary inventory
// work.
func (g *MMGame) moveStashItem(src, dst stashAddr, quantity int) {
	moving := g.stashItem(src)
	if src == dst || stash.IsEmpty(moving) {
		return
	}
	n := carriedUnits(moving, quantity)
	if dst.kind != stashKindBag && g.stashCellPtr(dst) == nil { // a shop shelf or no cell
		return
	}
	if automatic := dst == (stashAddr{stashKindBag, stashBagAutomatic}); !automatic && g.refusedDrop(moving, g.stashItem(dst), n == moving.Count()) {
		return
	}
	bag := g.party.Bag()
	if src.kind == stashKindBag && dst.kind == stashKindBag {
		bag.MoveWithin(src.idx, dst.idx, n)
		return
	}
	if n < moving.Count() {
		g.resolvePartialStashDrop(src, dst, n)
		return
	}
	if !g.finishPendingStashTransfer() {
		return
	}
	rollback := g.stashSnapshot()
	switch {
	case src.kind != stashKindBag && dst.kind != stashKindBag: // cell <-> cell
		sp, dp := g.stashCellPtr(src), g.stashCellPtr(dst)
		if items.DropOnto(*sp, *dp, true) == items.DropMerge {
			dp.MergeStack(*sp)
			*sp = items.Item{}
		} else {
			*sp, *dp = *dp, *sp
		}
	case src.kind == stashKindBag: // bag -> cell
		dp := g.stashCellPtr(dst)
		bag.Remove(src.idx)
		if items.DropOnto(moving, *dp, true) == items.DropMerge {
			dp.MergeStack(moving)
			break
		}
		occ := *dp
		*dp = moving
		if !stash.IsEmpty(occ) {
			g.releaseFromChest(&occ)
			bag.Insert(src.idx, occ) // the displaced item takes the bag cell
		}
	default: // cell -> bag
		sp := g.stashCellPtr(src)
		*sp = items.Item{}
		g.releaseFromChest(&moving)
		*sp = bag.Place(moving, dst.idx) // a displaced bag item takes the chest cell
	}
	g.commitStashTransfer(rollback)
}

// stashItem is what a stash address holds; an empty chest cell, the bag's
// free tail and an address out of range read as an empty cell.
func (g *MMGame) stashItem(a stashAddr) items.Item {
	if a.kind == stashKindBag {
		return g.party.Bag().Cell(a.idx)
	}
	if cell := g.stashCellPtr(a); cell != nil {
		return *cell
	}
	return items.Item{}
}

// releaseFromChest readies units leaving the chest for the party: components
// the chest still holds elsewhere (a layout split inside the chest shares its
// IDs) get fresh IDs, so a later load never strips them as stash-owned. Call
// it once the units are out of their cell.
func (g *MMGame) releaseFromChest(it *items.Item) {
	claimed := map[uint64]bool{}
	g.stash.Each(func(other *items.Item) {
		for _, part := range other.StackLineageParts() {
			claimed[part.ID] = true
		}
	})
	it.RekeyClaimedLineages(func(id uint64) bool { return claimed[id] })
}

// resolvePartialStashDrop transfers part of a stack across the chest boundary
// or within the chest, once moveStashItem has applied the placement rule: the
// fragment merges onto a matching stack or fills an empty cell. Stack
// provenance lives on items.Item, so any matching chest stack accepts it.
func (g *MMGame) resolvePartialStashDrop(src, dst stashAddr, quantity int) {
	dstCell := g.stashCellPtr(dst)
	if !g.finishPendingStashTransfer() {
		return
	}
	rollback := g.stashSnapshot()
	var fragment items.Item
	var ok bool
	switch src.kind {
	case stashKindBag:
		fragment, ok = g.party.TakeStackUnits(src.idx, quantity)
	case stashKindChest:
		source := g.stashCellPtr(src)
		if source != nil {
			if dst.kind == stashKindBag {
				fragment, ok = source.SplitOffForStashWithdrawal(quantity)
			} else {
				fragment, ok = source.SplitOffWithinStash(quantity)
			}
		}
	}
	if !ok {
		return
	}

	switch {
	case dst.kind == stashKindBag:
		g.releaseFromChest(&fragment)
		g.party.Bag().Place(fragment, dst.idx)
	case stash.IsEmpty(*dstCell):
		*dstCell = fragment
	default:
		dstCell.MergeStack(fragment)
	}
	g.commitStashTransfer(rollback)
}

// The stash shows the whole chest (stashChestCols x stashChestRows) beside
// one page of the bag at the same height, both sized from the host's area.
const (
	stashCellSize   = 44
	stashCellGap    = 6
	stashCellPitch  = stashCellSize + stashCellGap
	stashChestCols  = 8
	stashChestRows  = (stash.SlotCount + stashChestCols - 1) / stashChestCols
	stashPad        = 14
	stashHeadH      = 22 // section headings above the grids
	stashSectionGap = 28
	stashPopupW     = 740
	stashPopupH     = 540
	stashPopupHeadH = 56 // title and hint above the standalone modal's grids
)

// stashLayout is the resolved stash geometry. The renderer and the layout
// tests derive every rect from here so the two never drift.
type stashLayout struct {
	chest, bag layoutRect // the two cell grids
	bagCols    int
	headY      int // section headings
	noteY      int // the chest note and the bag pager, under the grids
}

func (L stashLayout) bagPageSize() int { return L.bagCols * stashChestRows }

// computeStashLayout places the standalone modal; its grids use the same
// area layout as the tavern tab.
func computeStashLayout(screenW, screenH int) (layoutRect, stashLayout) {
	popup := centeredRect(screenW, screenH, stashPopupW, stashPopupH)
	area := layoutRect{popup.x + 16, popup.y + stashPopupHeadH, popup.w - 32, popup.h - stashPopupHeadH - 8}
	return popup, computeStashLayoutForArea(area)
}

func computeStashLayoutForArea(area layoutRect) stashLayout {
	gridH := stashChestRows*stashCellPitch - stashCellGap
	chestW := stashChestCols*stashCellPitch - stashCellGap
	bagCols := max(1, (area.w-2*stashPad-chestW-stashSectionGap+stashCellGap)/stashCellPitch)
	bagW := bagCols*stashCellPitch - stashCellGap
	x := area.x + (area.w-chestW-stashSectionGap-bagW)/2
	top := area.y + stashPad + stashHeadH
	return stashLayout{
		chest:   layoutRect{x, top, chestW, gridH},
		bag:     layoutRect{x + chestW + stashSectionGap, top, bagW, gridH},
		bagCols: bagCols,
		headY:   area.y + stashPad,
		noteY:   top + gridH + 10,
	}
}

// stashGridCell is cell i of a grid cols wide.
func stashGridCell(grid layoutRect, cols, i int) image.Rectangle {
	x := grid.x + i%cols*stashCellPitch
	y := grid.y + i/cols*stashCellPitch
	return image.Rect(x, y, x+stashCellSize, y+stashCellSize)
}

// drawStashScreen renders the standalone chest modal around the shared stash
// manager. Closes on the X button or ESC.
func (ui *UISystem) drawStashScreen(screen *ebiten.Image) {
	g := ui.game
	if g.stash == nil {
		return
	}
	screenW := g.config.GetScreenWidth()
	screenH := g.config.GetScreenHeight()
	popup, L := computeStashLayout(screenW, screenH)

	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 150})
	ui.drawThemeFrame(screen, frameGold, popup.x, popup.y, popup.w, popup.h)
	drawUIText(screen, uitext.Text("stash.title"), popup.x+16, popup.y+14)
	drawUIText(screen, clipUIText(uitext.Text("stash.hint"), popup.w-72), popup.x+16, popup.y+34)

	interactive := ui.topModalLayer() == modalLayerStash
	ui.drawStashManager(screen, L, interactive)

	// ESC is handled in the Update input loop (edge-tracked) so it closes the
	// modal without leaking to the menu-open handler; here only the close button
	// (click-inert while a drag is in flight).
	ui.drawPopupCloseButton(screen, popup.right()-36, popup.y+10, 24, interactive && !g.stashDragActive && !ui.stackSplitPicker.open, func() {
		g.closeStashScreen()
	})
}

// drawStashManager renders and operates the chest and bag grids. The caller
// owns the surrounding panel, which lets the same manager live directly in a
// tavern tab without opening a second modal.
func (ui *UISystem) drawStashManager(screen *ebiten.Image, L stashLayout, interactive bool) {
	g := ui.game
	if g.stash == nil {
		return
	}
	mouseX, mouseY := uiCursorPosition()
	heading := color.RGBA{220, 180, 90, 255}
	muted := color.RGBA{150, 140, 120, 255}

	used := 0
	for _, it := range g.stash.Slots {
		if !stash.IsEmpty(it) {
			used++
		}
	}
	counter := strconv.Itoa(used) + " / " + strconv.Itoa(stash.SlotCount)
	drawUITextColored(screen, uitext.Text("stash.chest"), L.chest.x, L.headY, heading)
	drawUITextColored(screen, counter, L.chest.right()-uiTextWidth(counter), L.headY, muted)
	for i := range stash.SlotCount {
		r := stashGridCell(L.chest, stashChestCols, i)
		ui.drawStashSlot(screen, stashAddr{stashKindChest, i}, g.stash.Slots[i], r, interactive, mouseX, mouseY)
	}
	drawUITextColored(screen, clipUIText(uitext.Text("stash.shared"), L.chest.w), L.chest.x, L.noteY, muted)

	drawUITextColored(screen, uitext.Text("stash.bag"), L.bag.x, L.headY, heading)
	pageSize := L.bagPageSize()
	pages := pageCount(len(g.party.Inventory), pageSize)
	clampPage(&g.stashInvPage, pages)
	start := g.stashInvPage * pageSize
	for slot := range pageSize {
		idx := start + slot
		var it items.Item
		if idx < len(g.party.Inventory) {
			it = g.party.Inventory[idx]
		}
		ui.drawStashSlot(screen, stashAddr{stashKindBag, idx}, it, stashGridCell(L.bag, L.bagCols, slot), interactive, mouseX, mouseY)
	}
	pagerOpen := interactive && !g.stashDragActive && !ui.stackSplitPicker.open
	ui.drawPager(screen, L.bag.x, L.noteY, L.bag.w, &g.stashInvPage, pages, pagerOpen)
	// The wheel turns bag pages, also while an item is carried.
	ui.onWheelOver(L.bag, interactive && !ui.stackSplitPicker.open, func(dir int) {
		g.stashInvPage += dir
		clampPage(&g.stashInvPage, pages)
	})

	// Carried icon, drawn last so it floats above everything; then clear the drop.
	ui.drawStashCarriedIcon(screen)
	ui.onDisplayedInput(uiCommandDrag, layoutRect{}, func() {
		if interactive && g.stashDragDrop {
			g.clearStashDrag()
		}
	})
}

// drawStashSlot draws one chest or bag cell and registers its gestures: drag
// source, drop target, right-click split and double-click transfer. A bag
// address past the end of the inventory is an empty cell that still takes a
// drop.
func (ui *UISystem) drawStashSlot(screen *ebiten.Image, a stashAddr, it items.Item, r image.Rectangle, interactive bool, mouseX, mouseY int) {
	g := ui.game
	from := a.idx
	if a.kind == stashKindBag {
		from += stashDragInvBase
	}
	filled := !stash.IsEmpty(it)
	if interactive && filled {
		if a.kind == stashKindChest {
			ui.stashCellSource(a.idx, r)
		} else {
			ui.stashInvSource(a.idx, r)
		}
	}
	ui.drawStashCell(screen, it, r, g.stashDragActive && g.stashDragFrom == from && g.stashDragSplitQuantity == 0)
	if ptInRect(mouseX, mouseY, r) {
		hover := color.RGBA{210, 170, 80, 230}
		if a.kind == stashKindBag {
			hover = color.RGBA{120, 200, 120, 220}
		}
		drawRectBorder(screen, r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, 2, hover)
	}
	ui.stashCellTooltip(it, r, mouseX, mouseY)
	if !interactive {
		return
	}
	if filled {
		ui.stashSplitPickerTrigger(from, it, r)
		zone := "stash_bag"
		if a.kind == stashKindChest {
			zone = "stash_chest"
		}
		ui.onDisplayedInput(uiCommandClick, layoutRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, func() {
			if !g.stashDragActive && !ui.stackSplitPicker.open && g.consumeDoubleClickIn(zone, a.idx, r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) {
				g.quickMoveStashItem(a)
			}
		})
	}
	ui.onDisplayedInput(uiCommandDrag, layoutRect{}, func() {
		if g.stashDragDrop && g.stashDragFrom >= 0 && ptInRect(g.stashDragCurX, g.stashDragCurY, r) {
			g.resolveStashDrop(a)
		}
	})
}

// stashCellSource captures a chest cell as a drag source.
func (ui *UISystem) stashCellSource(i int, r image.Rectangle) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandDrag, layoutRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, func() { ui.stashCellSource(i, r) })
		return
	}

	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	if stash.IsEmpty(g.stash.Slots[i]) || !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	ui.beginStashDrag(i, g.stash.Slots[i])
}

// stashInvSource captures a bag cell as a drag source.
func (ui *UISystem) stashInvSource(idx int, r image.Rectangle) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandDrag, layoutRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, func() { ui.stashInvSource(idx, r) })
		return
	}

	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	if idx < 0 || idx >= len(g.party.Inventory) || !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	ui.beginStashDrag(stashDragInvBase+idx, g.party.Inventory[idx])
}

func (ui *UISystem) beginStashDrag(from int, item items.Item) {
	g := ui.game
	g.stashDragFrom = from
	g.stashDragItem = item
	g.stashDragSplitQuantity = 0
	if item.Stackable() && item.Count() > 1 && shiftModifierHeld() {
		g.stashDragSplitQuantity = 1
		g.stashDragItem.Quantity = 1
	}
}

// stashSplitPickerTrigger opens the exact quantity picker on right-click. The
// regular Shift-drag one-unit shortcut remains available for rapid transfers.
func (ui *UISystem) stashSplitPickerTrigger(from int, item items.Item, r image.Rectangle) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandClick, layoutRect{}, func() { ui.stashSplitPickerTrigger(from, item, r) })
		return
	}

	g := ui.game
	if g.stashDragActive || ui.stackSplitPicker.open || !item.Stackable() || item.Count() < 2 {
		return
	}
	if g.consumeRightClickIn(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) {
		ui.openStackSplitPicker(stackSplitPickerStash, from, item)
	}
}

// drawStashCarriedIcon floats the item carried by the stash drag machine at
// the cursor - shared by the stash manager and the merchant's drag-to-sell.
func (ui *UISystem) drawStashCarriedIcon(screen *ebiten.Image) {
	g := ui.game
	if g.stashDragActive && g.stashDragFrom >= 0 {
		const sz = 48
		ui.drawInventoryItemIcon(screen, g.stashDragItem, g.stashDragCurX-sz/2, g.stashDragCurY-sz/2, sz, sz, 0, true)
	}
}

// drawStashCell draws one slot: the item icon in a gold frame when filled, a
// quieter bronze frame when empty. hidden suppresses the icon for the cell
// currently being carried out.
func (ui *UISystem) drawStashCell(screen *ebiten.Image, it items.Item, r image.Rectangle, hidden bool) {
	frame := frameBronze
	if !stash.IsEmpty(it) {
		frame = frameGold
	}
	ui.drawThemeFrame(screen, frame, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	if !stash.IsEmpty(it) && !hidden {
		ui.drawInventoryItemIcon(screen, it, r.Min.X+2, r.Min.Y+2, r.Dx()-4, r.Dy()-4, 0, true)
	}
}

// stashCellTooltip queues the item card for a hovered, filled storage cell
// (chest, stash bag, the clerk's bag). Storage shows the base card, like a
// shop: no hero is holding the item. Nothing shows while a drag is active.
func (ui *UISystem) stashCellTooltip(it items.Item, cell image.Rectangle, mouseX, mouseY int) {
	g := ui.game
	if g.stashDragActive || stash.IsEmpty(it) || !ptInRect(mouseX, mouseY, cell) {
		return
	}
	if key := itemCardKey(it); key != "" {
		ui.fullArtCardKey = key
	}
	if tip := GetItemTooltipRows(it, nil, g.combat, tooltipDetailHeld()); len(tip) > 0 {
		ui.queueItemTooltip(tip, it, nil, mouseX+16, mouseY+8)
	}
}
