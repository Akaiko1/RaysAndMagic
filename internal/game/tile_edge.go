package game

// tileEdge identifies one undirected unit grid edge. Shared edges have one key.
type tileEdge struct {
	x, y     int
	vertical bool
}

// tileEdges follows auraCardinalDirections: east, west, south, north.
func tileEdges(tx, ty int) [4]tileEdge {
	return [4]tileEdge{{tx + 1, ty, true}, {tx, ty, true}, {tx, ty + 1, false}, {tx, ty, false}}
}

func (e tileEdge) points(ts float64) (x0, y0, x1, y1 float64) {
	x0, y0 = float64(e.x)*ts, float64(e.y)*ts
	x1, y1 = x0+ts, y0
	if e.vertical {
		x1, y1 = x0, y0+ts
	}
	return
}
