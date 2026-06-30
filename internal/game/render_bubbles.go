package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// bubbleColumnFx tunes one rising-bubble column. The impassable-tile aura, the
// hot-steam zone and the shut-valve steam are the same effect — a depth-tested
// stream of rising, fading glows at a sampled world point — differing only in
// these knobs. The projection, wall occlusion, phase and fade math lives once in
// emitBubbleColumn so the three callers only pick sample points and tuning.
type bubbleColumnFx struct {
	wx, wy       float64 // world anchor of the column
	hx, hy       int     // tile coords → deterministic, frame-stable per-tile phase
	salt         int     // per-edge/per-effect hash salt (decorrelates neighbours)
	hi           int     // per-column index; feeds the strong hash slot so columns desync
	maxDepth     float64 // far clip and distance-fade reference (world units)
	riseFraction float64 // bubble travel as a fraction of the tile's floor-rise
	baseAlpha    float64 // peak alpha before distance and per-stream dimming
	colBright    float64 // per-stream brightness in [0,1]
	perColumn    int     // staggered bubbles per column
	periodTick   float64 // base ticks for one bottom→top trip
	jitterMin    float64 // rise-speed jitter band lower bound
	jitterSpan   float64 // ... and width: speed ∈ [jitterMin, jitterMin+jitterSpan]×base
	sizeFloor    float64 // smallest bubble size in pixels
	sizeCoef     float64 // size = max(sizeFloor, (floorY-horizon)*sizeCoef)
	wobbleCoef   float64 // horizontal wobble amplitude in bubble-sizes
	color        [3]int
	centerDepth  float64 // billboard self-cull plane (bubbles behind it are hidden)
	hasCenter    bool    // false disables the self-cull (steam/valve fill the tile)
	fall         bool    // true = motes descend sky→ground (teleporter); default ground→sky (steam/aura)
	sizeJitter   float64 // 0 = uniform; >0 = per-bubble size varies in [1−j, 1+j]×base
}

// emitBubbleColumn projects one world point, culls it against the near/far clip,
// the tile's own billboard and the wall depth buffer, then draws perColumn rising
// glows that fade in at the floor and out at the top.
func (r *Renderer) emitBubbleColumn(screen *ebiten.Image, c bubbleColumnFx) {
	horizon := float64(r.game.config.GetScreenHeight()) / 2

	screenX, depth, ok := r.game.renderHelper.projectToScreenX(c.wx, c.wy)
	if !ok || depth < auraMinDepth || depth > c.maxDepth {
		return
	}
	if c.hasCenter && depth > c.centerDepth {
		return // far side of the billboard plane → hidden by the sprite
	}
	if screenX >= 0 && screenX < len(r.game.depthBuffer) && depth >= r.game.depthBuffer[screenX] {
		return // behind a wall
	}
	distFade := 1.0 - depth/c.maxDepth
	if distFade <= 0 {
		return
	}
	floorY := float64(r.game.renderHelper.calculateFloorScreenY(depth))
	rise := (floorY - horizon) * c.riseFraction
	if rise <= 0 {
		return
	}
	size := math.Max(c.sizeFloor, (floorY-horizon)*c.sizeCoef)

	for b := 0; b < c.perColumn; b++ {
		// Globally-unique bubble index in the hash's strong (golden-ratio) slot so
		// neighbouring columns scatter instead of marching in a linear walk.
		idx := c.hi*c.perColumn + b
		seed := auraHash(c.hx, c.hy, c.salt, idx)
		// Per-bubble rise speed so streams desync into random bubbling, not a band
		// rising in lockstep. +200 salt offset decorrelates speed from phase.
		speedSeed := auraHash(c.hx, c.hy, c.salt+200, idx)
		period := c.periodTick * (c.jitterMin + c.jitterSpan*speedSeed)
		phase := math.Mod(float64(r.game.frameCount)/period+seed, 1.0)
		// phase 0→1: rising goes floor→top; falling goes top→floor.
		by := floorY - phase*rise
		if c.fall {
			by = (floorY - rise) + phase*rise
		}
		alpha := c.baseAlpha * distFade * c.colBright * math.Sin(phase*math.Pi)
		if alpha <= 0.01 {
			continue
		}
		bsize := size
		if c.sizeJitter > 0 {
			sj := auraHash(c.hx, c.hy, c.salt+300, idx)
			bsize = size * (1 - c.sizeJitter + 2*c.sizeJitter*sj)
			if bsize < c.sizeFloor {
				bsize = c.sizeFloor
			}
		}
		bx := float64(screenX) + math.Sin((phase+seed)*2*math.Pi)*bsize*c.wobbleCoef
		r.drawGlowRect(screen, bx, by, bsize, c.color, alpha, additiveGlowBlend)
	}
}
