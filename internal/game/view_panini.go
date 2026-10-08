package game

import "math"

// Panini uses x = sin(theta)/(d+cos(theta)). Normalizing at the original
// horizontal FOV endpoints keeps both side rays. Its vertical scale is >= 1
// throughout the frame, so no overscan buffer or corner filling is needed.
// The center grows and loses some vertical coverage; the horizon stays fixed.
type paniniProjection struct {
	center, horizon, focal, distance, scale float64
}

func (g *MMGame) widePaniniProjection() paniniProjection {
	if !g.combatPreferences.WideView || g.combatPreferences.PaniniDisabled || g.config.Graphics.View.WidePaniniDistance == 0 {
		return paniniProjection{}
	}
	focal := g.viewFocal()
	center := float64(g.worldWidth()) / 2
	d := g.config.Graphics.View.WidePaniniDistance
	return paniniProjection{center, g.viewHorizon(), focal, d, 1 + d*math.Hypot(1, center/focal)}
}

// sourcePoint maps a displayed pixel into the perspective coordinates used by
// visibility and picking. Keep this mapping in sync with panini.kageinc; input
// never reads pixels from GPU.
func (p paniniProjection) sourcePoint(x, y float64) (float64, float64) {
	if p.distance == 0 {
		return x, y
	}
	v := (x - p.center) / (p.focal * p.scale)
	d2 := p.distance * p.distance
	s := (1 + p.distance*math.Sqrt(1+(1-d2)*v*v)) / (p.scale * (1 - d2*v*v))
	return p.center + (x-p.center)*s, p.horizon + (y-p.horizon)*s
}

// presentedY is used by held-target visibility: a perspective column can still
// have a visibility entry after its whole sprite has left the displayed view.
func (p paniniProjection) presentedY(x, y float64) float64 {
	if p.distance == 0 {
		return y
	}
	s := p.columnMagnification(x)
	return p.horizon + (y-p.horizon)*s
}

func (g *MMGame) uiToScenePoint(x, y int) (int, int) {
	sx := (float64(x) + 0.5) * float64(g.worldWidth()) / float64(g.config.GetScreenWidth())
	sy := (float64(y) + 0.5) * float64(g.worldHeight()) / float64(g.config.GetScreenHeight())
	// Do not warp outside-window input back into the frame near the inverse
	// projection's asymptote. HUD ownership is resolved in UI coordinates first.
	if x < 0 || y < 0 || x >= g.config.GetScreenWidth() || y >= g.config.GetScreenHeight() {
		return -1, -1
	}
	if g.gameLoop != nil {
		sx, sy = g.gameLoop.worldProjection.sourcePoint(sx, sy)
	}
	return int(sx), int(sy)
}

// presentedPoint projects geometry before rasterization; textures and procedural
// materials are then evaluated at the final pixel density.
func (p paniniProjection) presentedPoint(x, y float64) (float64, float64) {
	if p.distance == 0 {
		return x, y
	}
	s := p.columnMagnification(x)
	return p.center + (x-p.center)*s, p.horizon + (y-p.horizon)*s
}

// columnMagnification owns the forward Panini scale used by geometry, cached
// columns and held-target visibility. The inverse also lives in panini.kageinc.
func (p paniniProjection) columnMagnification(x float64) float64 {
	if p.distance == 0 {
		return 1
	}
	return p.scale / (1 + p.distance*math.Hypot(1, (x-p.center)/p.focal))
}
