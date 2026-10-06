package game

// Shut culvert valves mark their tile with a pale aurora twice as tall as the
// ordinary boundary curtain. The saved Visited state remains the activation gate.
const (
	valveSteamRiseFraction = 1.1
	valveSteamBaseAlpha    = 0.6
)

var valveSteamColor = [3]int{225, 230, 236}

func (r *Renderer) collectClosedValveAuroras() {
	if r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	_, density, _ := r.auraEdgeParams()
	for _, n := range r.game.world.NPCs {
		if n == nil || !n.SteamWhenVisited || !n.Visited {
			continue
		}
		tx, ty := TileIndex(n.X, ts), TileIndex(n.Y, ts)
		for _, edge := range tileEdges(tx, ty) {
			x0, y0, x1, y1 := edge.points(ts)
			r.collectAuraSegment(x0, y0, x1, y1, ts, valveSteamRiseFraction, density,
				valveSteamBaseAlpha, r.game.camera.ViewDist, valveSteamColor)
		}
	}
}
