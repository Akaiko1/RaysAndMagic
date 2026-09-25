package game

import "time"

// Sample pointer edges once, then let the displayed dispatcher own actions.
func (ui *UISystem) updateMouseState() {
	ui.syncPointerScreen()
	leftJustPressed := pointerLeftJustPressed()
	rightJustPressed := pointerRightJustPress()
	now := time.Now().UnixMilli()
	if ui.modalRedrawBarrierActive() {
		ui.cancelScreenPointerGestures()
		ui.dropQueuedClicks()
		return
	}
	// Buffered clicks never cross a UI-layer boundary: on a modal<->world flip
	// drop the queues (a click aimed at one layer must not fire in the next).
	// Runs before this frame's clicks enqueue; within one layer (dialog
	// double-clicks) no flip occurs.
	if allowed := ui.game.worldClickAllowed(); allowed != ui.game.prevWorldClickAllowed {
		ui.game.mouseLeftClicks = ui.game.mouseLeftClicks[:0]
		ui.game.mouseRightClicks = ui.game.mouseRightClicks[:0]
		ui.game.prevWorldClickAllowed = allowed
	}
	ui.game.pruneClickQueues(now)
	suppressLeftClick, releaseDrivenClicks := ui.recognizeGameplayPointer(now)

	if leftJustPressed && !suppressLeftClick && !releaseDrivenClicks {
		x, y := pointerPosition()
		ui.game.mouseLeftClicks = append(ui.game.mouseLeftClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseLeftClickX, ui.game.mouseLeftClickY = x, y
	}
	if rightJustPressed {
		x, y := pointerPosition()
		ui.game.mouseRightClicks = append(ui.game.mouseRightClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseRightClickX, ui.game.mouseRightClickY = x, y
	}
}

// Gameplay drag recognizers never inspect title-screen or party-creation input.
func (ui *UISystem) recognizeGameplayPointer(now int64) (bool, bool) {
	if ui.game.appScreen != AppScreenInGame {
		return false, false
	}
	inputLayer := ui.topModalLayer()
	suppressLeftClick := false
	if inputLayer == modalLayerNone {
		suppressLeftClick = ui.updateQuickDrag()
	} else if !ui.game.dragPickedUp && (ui.game.dragArmed || ui.game.dragActive) {
		// Ownership lost mid-gesture: the release edge is a one-tick event and
		// would be missed while the updater is skipped, leaving an armed/active
		// drag to resurrect when the layer returns. Cancel it; only a picked-up
		// split fragment is deliberate state that survives the freeze.
		ui.game.clearDrag()
	}
	// The stash is either its standalone modal or a manager embedded in the NPC
	// dialog. A higher modal must not let a release resolve later against a
	// newly uncovered stash cell - same rule: cancel everything transient, keep
	// only the deliberately picked-up split fragment.
	//
	// While the drag machine runs it OWNS the left button: it queues the click
	// itself, on a release that stayed under the drag threshold (see
	// updateStashDrag). Queueing on press as well would let the press that
	// begins a drag double as a buy/sell click.
	// The release-driven mode belongs to the SURFACE, not the layer: an ordinary
	// dialog (quest, tavern service, trainer) has no drag machine to queue its
	// clicks, so gating on the layer alone would leave it with no clicks at all.
	dragSurface := ui.game.stashDragSurfaceOpen()
	releaseDrivenClicks := false
	if dragSurface && (inputLayer == modalLayerStash || inputLayer == modalLayerDialog) {
		suppressLeftClick = ui.updateStashDrag(now) || suppressLeftClick
		releaseDrivenClicks = true
	} else if !ui.game.stashDragPickedUp && (ui.game.stashDragArmed || ui.game.stashDragActive || ui.game.stashDragDrop) {
		ui.game.clearStashDrag()
	}

	return suppressLeftClick, releaseDrivenClicks
}
