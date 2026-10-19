package game

import "time"

// dragDeadZone is the cursor travel (px, either axis) that turns a held press
// into a drag; anything shorter stays a click.
const dragDeadZone = 5

// dragGesture is the one press -> drag recogniser behind every drag surface.
type dragGesture struct {
	armed, active  bool // button held since press / travelled past the dead zone
	startX, startY int  // press point (source hit-test)
	curX, curY     int  // live cursor (drop target, carried icon)
}

func (d *dragGesture) press(x, y int) {
	*d = dragGesture{armed: true, startX: x, startY: y, curX: x, curY: y}
}

// follow tracks a held press and reports the step it becomes a drag.
func (d *dragGesture) follow(x, y int) bool {
	if !d.armed {
		return false
	}
	d.curX, d.curY = x, y
	if d.active || absInt(x-d.startX) <= dragDeadZone && absInt(y-d.startY) <= dragDeadZone {
		return false
	}
	d.active = true
	return true
}

// Sample pointer edges once, then let the displayed dispatcher own actions.
func (ui *UISystem) updateMouseState() {
	ui.syncPointerScreen()
	if !pointerLeftPressed() {
		ui.scrollDrag = scrollbarDrag{}
	}
	leftJustPressed := pointerLeftJustPressed()
	rightJustPressed := pointerRightJustPress()
	now := time.Now().UnixMilli()
	if ui.modalRedrawBarrierActive() {
		ui.cancelScreenPointerGestures()
		ui.dropQueuedClicks()
		return
	}
	// Update owns queue lifetime, including early returns. A slow frame must
	// not discard an otherwise valid press; timestamps only describe gestures.
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
	} else if !ui.game.dragPickedUp && (ui.game.drag.armed || ui.game.drag.active) {
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
	} else if !ui.game.stashDragPickedUp && (ui.game.stashDrag.armed || ui.game.stashDrag.active || ui.game.stashDragDrop) {
		ui.game.clearStashDrag()
	}

	return suppressLeftClick, releaseDrivenClicks
}
