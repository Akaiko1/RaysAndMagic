package game

import (
	"math"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

const (
	defaultCanopyShadeRadiusTiles  = 3.0
	defaultCanopyShadeStartDensity = 2
	defaultCanopyShadeFullDensity  = 8
)

func (r *Renderer) clearCanopyShadeCache() {
	r.canopyShadeFactors = nil
	r.canopyShadeW = 0
	r.canopyShadeH = 0
	r.canopyViewerReady = false
	r.canopyViewerShade = 1
	r.canopyViewerFrame = 0
}

func canopyShadeFactorForDensity(density int, minAmbient float64, startDensity, fullDensity int) float64 {
	if minAmbient <= 0 || minAmbient >= 1 {
		return 1
	}
	if fullDensity <= startDensity {
		fullDensity = startDensity + 1
	}
	if density <= startDensity {
		return 1
	}
	if density >= fullDensity {
		return minAmbient
	}
	t := float64(density-startDensity) / float64(fullDensity-startDensity)
	return 1 - t*(1-minAmbient)
}

func (r *Renderer) buildCanopyShadeCache() {
	w := r.game.GetCurrentWorld()
	if w == nil || w.Width <= 0 || w.Height <= 0 {
		r.clearCanopyShadeCache()
		return
	}

	width, height := w.Width, w.Height
	factors := make([]float64, width*height)
	for i := range factors {
		factors[i] = 1
	}

	if world.GlobalTileManager != nil && world.GlobalWorldManager != nil {
		wm := world.GlobalWorldManager
		if r.game.openWorldActive() {
			// The unified world blends several maps: each region applies ITS
			// OWN canopy config to its rect only (a shadeless desert must not
			// inherit the forest's canopy, and vice versa).
			for i := range wm.OpenWorldRegions {
				region := &wm.OpenWorldRegions[i]
				mc := wm.MapConfigs[region.MapKey]
				if mc == nil || mc.CanopyShade == nil {
					continue
				}
				minAmbient, radiusTiles, startDensity, fullDensity := canopyShadeParams(mc.CanopyShade)
				r.applyCanopyShadeFactors(w, factors, minAmbient, radiusTiles, startDensity, fullDensity,
					region.OffsetX, region.OffsetY, region.OffsetX+region.Width, region.OffsetY+region.Height)
			}
		} else if mc := wm.GetCurrentMapConfig(); mc != nil && mc.CanopyShade != nil {
			minAmbient, radiusTiles, startDensity, fullDensity := canopyShadeParams(mc.CanopyShade)
			r.applyCanopyShadeFactors(w, factors, minAmbient, radiusTiles, startDensity, fullDensity, 0, 0, width, height)
		}
	}

	r.canopyShadeFactors = factors
	r.canopyShadeW = width
	r.canopyShadeH = height
	r.canopyViewerReady = false
}

func canopyShadeParams(shadeCfg *config.MapCanopyShadeConfig) (minAmbient, radiusTiles float64, startDensity, fullDensity int) {
	minAmbient = shadeCfg.MinAmbient
	radiusTiles = shadeCfg.RadiusTiles
	if radiusTiles <= 0 {
		radiusTiles = defaultCanopyShadeRadiusTiles
	}
	startDensity = shadeCfg.StartDensity
	if startDensity <= 0 {
		startDensity = defaultCanopyShadeStartDensity
	}
	fullDensity = shadeCfg.FullDensity
	if fullDensity <= 0 {
		fullDensity = defaultCanopyShadeFullDensity
	}
	return
}

// applyCanopyShadeFactors fills factors inside the [x0,x1)x[y0,y1) tile rect
// (a region on the unified world, the whole map otherwise). Tree density
// still counts neighbours beyond the rect so seams shade smoothly.
func (r *Renderer) applyCanopyShadeFactors(w *world.World3D, factors []float64, minAmbient, radiusTiles float64, startDensity, fullDensity, x0, y0, x1, y1 int) {
	if minAmbient <= 0 || minAmbient >= 1 || radiusTiles <= 0 {
		return
	}

	width, height := w.Width, w.Height
	treeTiles := make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if y >= len(w.Tiles) || x >= len(w.Tiles[y]) {
				continue
			}
			tileType := w.Tiles[y][x]
			if world.GlobalTileManager.GetRenderType(tileType) == "tree_sprite" {
				treeTiles[y*width+x] = true
			}
		}
	}

	x0, y0 = max(0, x0), max(0, y0)
	x1, y1 = min(width, x1), min(height, y1)
	radius := int(math.Ceil(radiusTiles))
	radiusSq := radiusTiles * radiusTiles
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			density := 0
			for yy := max(0, y-radius); yy <= min(height-1, y+radius); yy++ {
				for xx := max(0, x-radius); xx <= min(width-1, x+radius); xx++ {
					dx := float64(xx - x)
					dy := float64(yy - y)
					if dx*dx+dy*dy <= radiusSq && treeTiles[yy*width+xx] {
						density++
					}
				}
			}
			factors[y*width+x] = canopyShadeFactorForDensity(density, minAmbient, startDensity, fullDensity)
		}
	}
}

func (r *Renderer) canopyShadeFactorAt(worldX, worldY float64) float64 {
	if len(r.canopyShadeFactors) == 0 || r.canopyShadeW <= 0 || r.canopyShadeH <= 0 {
		return 1
	}
	tileSize := float64(r.game.config.GetTileSize())
	if tileSize <= 0 {
		return 1
	}
	fx := worldX/tileSize - 0.5
	fy := worldY/tileSize - 0.5
	tx := int(math.Floor(fx))
	ty := int(math.Floor(fy))
	ux := fx - float64(tx)
	uy := fy - float64(ty)
	f00 := r.canopyShadeFactorTile(tx, ty)
	f10 := r.canopyShadeFactorTile(tx+1, ty)
	f01 := r.canopyShadeFactorTile(tx, ty+1)
	f11 := r.canopyShadeFactorTile(tx+1, ty+1)
	top := f00 + (f10-f00)*ux
	bottom := f01 + (f11-f01)*ux
	return top + (bottom-top)*uy
}

func (r *Renderer) canopyShadeFactorTile(tx, ty int) float64 {
	if tx < 0 || ty < 0 || tx >= r.canopyShadeW || ty >= r.canopyShadeH {
		return 1
	}
	factor := r.canopyShadeFactors[ty*r.canopyShadeW+tx]
	if factor <= 0 || factor > 1 {
		return 1
	}
	return factor
}

// canopyAmbient composes the map ambient with the canopy shade (the deeper of
// the surface's and the viewer's factor). Multiplicative, so the shade scales
// with the day/night light level: full-strength contrast at noon, dimming with
// dusk, nearly gone in the 0.7 night ambient. At full daylight (ambient 1.0)
// this is identical to the old min() composition.
func canopyAmbient(mapAmbient, surfaceShade, viewerShade float64) float64 {
	ambient := 1.0
	if mapAmbient > 0 {
		ambient = mapAmbient
	}
	shade := 1.0
	if surfaceShade > 0 && surfaceShade < shade {
		shade = surfaceShade
	}
	if viewerShade > 0 && viewerShade < shade {
		shade = viewerShade
	}
	return ambient * shade
}

func (r *Renderer) mapAmbient() float64 {
	if r.ambientLight > 0 {
		return r.ambientLight
	}
	return 1
}

func (r *Renderer) viewerCanopyShadeFactor() float64 {
	if r.game == nil || r.game.camera == nil {
		return 1
	}
	return r.canopyShadeFactorAt(r.game.camera.X, r.game.camera.Y)
}

// viewerShadeSmoothed is the viewer's PURE canopy shade factor with temporal
// smoothing - deliberately not composed with the map ambient, so callers can
// feed it back into canopyAmbient without applying ambient twice (that
// double-count made sprites darker than the floor on night forests).
func (r *Renderer) viewerShadeSmoothed() float64 {
	target := r.viewerCanopyShadeFactor()
	if !r.canopyViewerReady {
		r.canopyViewerShade = target
		r.canopyViewerFrame = r.game.frameCount
		r.canopyViewerReady = true
		return target
	}

	dtFrames := r.game.frameCount - r.canopyViewerFrame
	r.canopyViewerFrame = r.game.frameCount
	if dtFrames <= 0 {
		return r.canopyViewerShade
	}
	tps := r.game.config.GetTPS()
	if tps <= 0 {
		tps = 60
	}
	if dtFrames > int64(tps) {
		r.canopyViewerShade = target
		return target
	}
	alpha := 1 - math.Exp(-4.0*float64(dtFrames)/float64(tps))
	r.canopyViewerShade += (target - r.canopyViewerShade) * alpha
	return r.canopyViewerShade
}

// viewerAmbient is the composed ambient-x-viewer-shade value the floor shader
// consumes (its min(Ambient, ViewerAmbient) picks this when the viewer stands
// under canopy, matching the CPU surfaces lit via localAmbientAt).
func (r *Renderer) viewerAmbient() float64 {
	return canopyAmbient(r.mapAmbient(), 1, r.viewerShadeSmoothed())
}

func (r *Renderer) localAmbientAt(worldX, worldY float64) float64 {
	return canopyAmbient(r.mapAmbient(), r.canopyShadeFactorAt(worldX, worldY), r.viewerShadeSmoothed())
}
