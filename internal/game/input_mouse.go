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

// consumeClickIn consumes the oldest click in the queue that falls inside the
// bounds, recording its position/time into the given destination fields. Shared
// by the left and right click queues. Bounds are inclusive-exclusive: [x1,x2)
// and [y1,y2).
func consumeClickIn(queue *[]queuedClick, dstX, dstY *int, dstAt *int64, x1, y1, x2, y2 int) bool {
	clicks := *queue
	for i, click := range clicks {
		if click.x >= x1 && click.x < x2 && click.y >= y1 && click.y < y2 {
			*queue = append(clicks[:i], clicks[i+1:]...)
			*dstX, *dstY = click.x, click.y
			*dstAt = click.at
			return true
		}
	}
	return false
}

// consumeLeftClickIn consumes the oldest queued left-click inside the bounds.
func (g *MMGame) consumeLeftClickIn(x1, y1, x2, y2 int) bool {
	return consumeClickIn(&g.mouseLeftClicks, &g.mouseLeftClickX, &g.mouseLeftClickY, &g.mouseLeftClickAt, x1, y1, x2, y2)
}

// consumeRightClickIn consumes the oldest queued right-click inside the bounds.
func (g *MMGame) consumeRightClickIn(x1, y1, x2, y2 int) bool {
	return consumeClickIn(&g.mouseRightClicks, &g.mouseRightClickX, &g.mouseRightClickY, &g.mouseRightClickAt, x1, y1, x2, y2)
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
