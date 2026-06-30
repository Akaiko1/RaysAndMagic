package game

import (
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// teleporterTileFx is one teleporter tile to decorate: its coords + glow colour
// (the tile's floor_color, now used as the effect tint since inherit_floor makes
// the ground itself take the surrounding biome floor).
type teleporterTileFx struct {
	tx, ty int
	color  [3]int
}

// drawSpawnTileBorder outlines the player's start tile with rising bubbles in the
// spawn tile's colour (red) on all four edges — the same edge technique as the
// impassable-tile aura, so the start cell reads as a marked square while its
// floor blends into the biome ground (inherit_floor). Drawn after walls/sprites
// so the wall depth buffer occludes it.
func (r *Renderer) drawSpawnTileBorder(screen *ebiten.Image) {
	w := r.game.GetCurrentWorld()
	if w == nil || w.StartX < 0 || w.StartY < 0 || world.GlobalTileManager == nil {
		return
	}
	rgb := world.GlobalTileManager.GetFloorColor(world.TileSpawn)
	if rgb == ([3]int{0, 0, 0}) {
		rgb = [3]int{255, 0, 0}
	}

	auraCfg := r.game.config.Graphics.ImpassableAura
	baseAlpha := auraCfg.Alpha
	if baseAlpha <= 0 {
		baseAlpha = 0.5
	}
	perEdge := auraCfg.BubblesPerEdge
	if perEdge <= 0 {
		perEdge = 3
	}
	radius := auraCfg.RadiusTiles
	if radius <= 0 {
		radius = 7
	}

	ts := float64(r.game.config.GetTileSize())
	maxDepth := float64(radius) * ts
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		r.emitAuraEdge(screen, w.StartX, w.StartY, d, ts, perEdge, baseAlpha, maxDepth, rgb)
	}
}

// Teleporter glow tuning: a DENSE field of varied-size motes that fall sky→ground
// across the whole tile (cf. the Golden Thief Bug's blink), in the teleporter's
// colour. Denser grid + more per-point than the valve steam, plus size jitter.
const (
	teleporterGrid         = 5   // 5x5 sample points across the tile (denser)
	teleporterPerPoint     = 3   // motes staggered per point
	teleporterRiseFraction = 2.8 // motes fall from ~1 tile above the floor (was 1.4 ≈ half a tile)
	teleporterPeriodTick   = 80.0
	teleporterColMin       = 0.55
	teleporterBaseAlpha    = 0.7
	teleporterJitterMin    = 0.5
	teleporterJitterSpan   = 1.0
	teleporterSizeJitter   = 0.6 // sizes vary ±60%
	// Fade distance set PAST the 50-tile view distance so a teleporter at the far
	// edge of view still glows (distFade = 1−depth/maxDepth → ~0.23 at 50 tiles)
	// instead of fading to nothing exactly at the view edge.
	teleporterMaxDepthTiles = 65
)

// drawTeleporterTileFx fills the whole tile of every teleporter with a dense
// field of falling, varied-size glow motes in the teleporter's colour — visible
// from 50 tiles as a beacon. Tile list cached per map in precomputeFloorColorCache.
func (r *Renderer) drawTeleporterTileFx(screen *ebiten.Image) {
	if len(r.teleporterTiles) == 0 || r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	maxDepth := float64(teleporterMaxDepthTiles) * ts
	camX, camY := r.game.camera.X, r.game.camera.Y

	for _, tp := range r.teleporterTiles {
		// Standee-mode trees don't write the wall depth buffer, so the bubble
		// columns' depthBuffer test alone can't hide a portal behind a tree. Add a
		// solid-tile line-of-sight check (trees/walls are solid) and skip the whole
		// tile's glow when the camera can't see its centre.
		cx := (float64(tp.tx) + 0.5) * ts
		cy := (float64(tp.ty) + 0.5) * ts
		if r.game.collisionSystem != nil && !r.game.collisionSystem.CheckLineOfSight(camX, camY, cx, cy) {
			continue
		}
		for gy := 0; gy < teleporterGrid; gy++ {
			for gx := 0; gx < teleporterGrid; gx++ {
				fx := (float64(gx) + 0.5) / float64(teleporterGrid)
				fy := (float64(gy) + 0.5) / float64(teleporterGrid)
				colBright := teleporterColMin + (1.0-teleporterColMin)*auraHash(tp.tx, tp.ty, gx+70, gy+70)
				r.emitBubbleColumn(screen, bubbleColumnFx{
					wx: (float64(tp.tx) + fx) * ts, wy: (float64(tp.ty) + fy) * ts,
					hx: tp.tx, hy: tp.ty, hi: gy*teleporterGrid + gx,
					maxDepth:     maxDepth,
					riseFraction: teleporterRiseFraction,
					baseAlpha:    teleporterBaseAlpha,
					colBright:    colBright,
					perColumn:    teleporterPerPoint,
					periodTick:   teleporterPeriodTick,
					jitterMin:    teleporterJitterMin,
					jitterSpan:   teleporterJitterSpan,
					sizeFloor:    2,
					sizeCoef:     0.06,
					wobbleCoef:   0.7,
					color:        tp.color,
					fall:         true, // sky → ground, like the bug's motes
					sizeJitter:   teleporterSizeJitter,
				})
			}
		}
	}
}
