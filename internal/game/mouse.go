package game

// leftClickPosition returns the queued left-click position for this frame.
// It returns ok=false if there is no click queued or the click was already consumed.
func (g *MMGame) leftClickPosition() (x, y int, ok bool) {
	if !g.mouseLeftClickQueued || g.mousePressed {
		return 0, 0, false
	}
	return g.mouseLeftClickX, g.mouseLeftClickY, true
}

// rightClickPosition returns the queued right-click position for this frame.
// It returns ok=false if there is no click queued or the click was already consumed.
func (g *MMGame) rightClickPosition() (x, y int, ok bool) {
	if !g.mouseRightClickQueued || g.mouseRightPressed {
		return 0, 0, false
	}
	return g.mouseRightClickX, g.mouseRightClickY, true
}

// consumeLeftClick consumes the queued left-click for this frame (no bounds check).
func (g *MMGame) consumeLeftClick() bool {
	if !g.mouseLeftClickQueued || g.mousePressed {
		return false
	}
	g.mousePressed = true
	g.mouseLeftClickQueued = false
	return true
}

// consumeRightClick consumes the queued right-click for this frame (no bounds check).
func (g *MMGame) consumeRightClick() bool {
	if !g.mouseRightClickQueued || g.mouseRightPressed {
		return false
	}
	g.mouseRightPressed = true
	g.mouseRightClickQueued = false
	return true
}

// consumeLeftClickIn consumes the queued left-click for this frame if it is inside the bounds.
// Bounds are inclusive-exclusive: [x1,x2) and [y1,y2).
func (g *MMGame) consumeLeftClickIn(x1, y1, x2, y2 int) bool {
	if !g.mouseLeftClickQueued || g.mousePressed {
		return false
	}
	x, y := g.mouseLeftClickX, g.mouseLeftClickY
	if x >= x1 && x < x2 && y >= y1 && y < y2 {
		g.mousePressed = true
		g.mouseLeftClickQueued = false
		return true
	}
	return false
}

// consumeRightClickIn consumes the queued right-click for this frame if it is inside the bounds.
// Bounds are inclusive-exclusive: [x1,x2) and [y1,y2).
func (g *MMGame) consumeRightClickIn(x1, y1, x2, y2 int) bool {
	if !g.mouseRightClickQueued || g.mouseRightPressed {
		return false
	}
	x, y := g.mouseRightClickX, g.mouseRightClickY
	if x >= x1 && x < x2 && y >= y1 && y < y2 {
		g.mouseRightPressed = true
		g.mouseRightClickQueued = false
		return true
	}
	return false
}
