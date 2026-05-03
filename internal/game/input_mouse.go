package game

type queuedClick struct {
	x, y int
	at   int64
}

const clickBufferMs = doubleClickWindowMs

// leftClickPosition returns the oldest queued left-click position.
// It returns ok=false if there is no click queued.
func (g *MMGame) leftClickPosition() (x, y int, ok bool) {
	if len(g.mouseLeftClicks) == 0 {
		return 0, 0, false
	}
	click := g.mouseLeftClicks[0]
	return click.x, click.y, true
}

// consumeLeftClick consumes the oldest queued left-click (no bounds check).
func (g *MMGame) consumeLeftClick() bool {
	if len(g.mouseLeftClicks) == 0 {
		return false
	}
	click := g.mouseLeftClicks[0]
	g.mouseLeftClicks = g.mouseLeftClicks[1:]
	g.mouseLeftClickX, g.mouseLeftClickY = click.x, click.y
	g.mouseLeftClickAt = click.at
	return true
}

// consumeLeftClickIn consumes the oldest queued left-click inside the bounds.
// Bounds are inclusive-exclusive: [x1,x2) and [y1,y2).
func (g *MMGame) consumeLeftClickIn(x1, y1, x2, y2 int) bool {
	for i, click := range g.mouseLeftClicks {
		if click.x >= x1 && click.x < x2 && click.y >= y1 && click.y < y2 {
			g.mouseLeftClicks = append(g.mouseLeftClicks[:i], g.mouseLeftClicks[i+1:]...)
			g.mouseLeftClickX, g.mouseLeftClickY = click.x, click.y
			g.mouseLeftClickAt = click.at
			return true
		}
	}
	return false
}

// consumeRightClickIn consumes the oldest queued right-click inside the bounds.
// Bounds are inclusive-exclusive: [x1,x2) and [y1,y2).
func (g *MMGame) consumeRightClickIn(x1, y1, x2, y2 int) bool {
	for i, click := range g.mouseRightClicks {
		if click.x >= x1 && click.x < x2 && click.y >= y1 && click.y < y2 {
			g.mouseRightClicks = append(g.mouseRightClicks[:i], g.mouseRightClicks[i+1:]...)
			g.mouseRightClickX, g.mouseRightClickY = click.x, click.y
			g.mouseRightClickAt = click.at
			return true
		}
	}
	return false
}

func (g *MMGame) pruneClickQueues(now int64) {
	if len(g.mouseLeftClicks) > 0 {
		keep := g.mouseLeftClicks[:0]
		for _, click := range g.mouseLeftClicks {
			if now-click.at <= clickBufferMs {
				keep = append(keep, click)
			}
		}
		g.mouseLeftClicks = keep
	}
	if len(g.mouseRightClicks) > 0 {
		keep := g.mouseRightClicks[:0]
		for _, click := range g.mouseRightClicks {
			if now-click.at <= clickBufferMs {
				keep = append(keep, click)
			}
		}
		g.mouseRightClicks = keep
	}
}
