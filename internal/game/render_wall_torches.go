package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/world"
)

// Wall torches (map flag `wall_torches`): a flickering torch sits at every
// inner wall corner — a walkable tile with two perpendicular blocking
// neighbours. The flame is a small procedural particle cluster (same family as
// the impassable aura / valve steam: deterministic auraHash phases, additive
// glow, depth-tested), and each torch is a flickering point light.

const (
	wallTorchLightRadiusTiles = 2.0  // lit area per torch
	wallTorchLightIntensity   = 0.7  // peak brightness contribution
	wallTorchCornerInset      = 14.0 // world units from the corner into the room
)

// wallTorchPoint is one detected corner torch.
type wallTorchPoint struct {
	X, Y float64
	seed int
}

// buildWallTorches scans the world for inner wall corners and caches torch
// positions. Called from the same world-change hook that rebuilds the other
// per-map caches; clears the list when the map doesn't enable torches.
func (r *Renderer) buildWallTorches() {
	r.wallTorches = r.wallTorches[:0]
	w := r.game.world
	if w == nil || world.GlobalWorldManager == nil {
		return
	}
	mc := world.GlobalWorldManager.GetCurrentMapConfig()
	if mc == nil || !mc.WallTorches {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	blocked := func(x, y int) bool { return w.IsTileBlocking(x, y) }
	for ty := 0; ty < w.Height; ty++ {
		for tx := 0; tx < w.Width; tx++ {
			if blocked(tx, ty) {
				continue // torches hang in the room, not inside walls
			}
			// Each pair of perpendicular blocking neighbours = one inner corner.
			for _, c := range [4]struct {
				dx, dy int // blocked directions forming the corner
			}{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				if !blocked(tx+c.dx, ty) || !blocked(tx, ty+c.dy) {
					continue
				}
				px := float64(tx)*ts + wallTorchCornerInset
				if c.dx > 0 {
					px = float64(tx+1)*ts - wallTorchCornerInset
				}
				py := float64(ty)*ts + wallTorchCornerInset
				if c.dy > 0 {
					py = float64(ty+1)*ts - wallTorchCornerInset
				}
				r.wallTorches = append(r.wallTorches, wallTorchPoint{
					X: px, Y: py, seed: (tx*73+ty*19)*4 + (c.dx + 1) + (c.dy+1)/2,
				})
			}
		}
	}
}

// wallTorchFlicker returns the torch's brightness modulation for this frame —
// a slow breathing wave plus per-torch phase so neighbours don't pulse in sync.
func wallTorchFlicker(seed int, frameCount int64) float64 {
	phase := auraHash(seed, 0, 11, 0) * 2 * math.Pi
	f := 0.85 + 0.12*math.Sin(float64(frameCount)*0.13+phase) +
		0.03*math.Sin(float64(frameCount)*0.47+phase*2)
	return f
}

// drawWallTorchFlame renders one torch's flame cluster: a few rising fire
// motes plus crackling sparks, anchored partway up the wall at the corner.
// Called from the unified sprite pass so the flame depth-sorts against
// billboards and standees; walls still occlude via the depth buffer here.
func (r *Renderer) drawWallTorchFlame(screen *ebiten.Image, tp wallTorchPoint) {
	horizon := float64(r.game.config.GetScreenHeight()) / 2
	viewDist := r.game.camera.ViewDist
	depthBuf := r.game.depthBuffer
	fc := r.game.frameCount

	screenX, depth, ok := r.game.renderHelper.projectToScreenX(tp.X, tp.Y)
	if !ok || depth < auraMinDepth || depth > viewDist {
		return
	}
	if screenX >= 0 && screenX < len(depthBuf) && depth >= depthBuf[screenX] {
		return // behind a wall face
	}
	floorY := float64(r.game.renderHelper.calculateFloorScreenY(depth))
	halfWall := floorY - horizon
	if halfWall <= 0 {
		return
	}
	// Flame anchored at torch height (~60% up a 1-tile wall).
	baseY := floorY - halfWall*1.2
	size := math.Max(2, halfWall*0.10)
	flick := wallTorchFlicker(tp.seed, fc)

	// Fire motes: short risers cycling out of the sconce.
	for k := 0; k < 5; k++ {
		ph := math.Mod(float64(fc)/(26.0+6*auraHash(tp.seed, k, 1, 0))+auraHash(tp.seed, k, 2, 0), 1.0)
		rise := halfWall * 0.22 * ph
		sway := math.Sin((ph+auraHash(tp.seed, k, 3, 0))*2*math.Pi) * size * 0.5
		alpha := flick * (1 - ph) * 0.9
		col := mixColor([3]int{255, 90, 10}, [3]int{255, 220, 120}, 1-ph) // hot core → ember tip
		r.drawGlowRect(screen, float64(screenX)+sway, baseY-rise, size*(1.1-0.5*ph), col, alpha, additiveGlowBlend)
	}
	// Bright heart of the flame.
	r.drawGlowSprite(screen, float64(screenX), baseY, size*2.4*flick, [3]int{255, 180, 60}, 0.55*flick, additiveGlowBlend)

	// Crackling sparks: a few tiny embers that FLY out smoothly and die.
	// Each spark's flight is one phase cycle; the cycle index seeds a fresh
	// direction/length every flight, so trajectories never repeat in step —
	// and the per-torch seed keeps neighbouring torches out of sync.
	for k := 0; k < 3; k++ {
		period := 18.0 + 10.0*auraHash(tp.seed, k, 8, 0)
		cyc := float64(fc)/period + auraHash(tp.seed, k, 9, 0)
		ph := math.Mod(cyc, 1.0)
		flight := int(cyc) // changes once per flight → new random trajectory
		if auraHash(tp.seed, k, flight, 1) < 0.45 {
			continue // this spark sits this cycle out
		}
		ang := (0.6 + 1.8*auraHash(tp.seed, k, flight, 2)) * math.Pi // mostly upward fan
		reach := size * (2.0 + 3.0*auraHash(tp.seed, k, flight, 3))
		sx := float64(screenX) + math.Cos(ang)*reach*ph
		sy := baseY - size*0.5 - math.Abs(math.Sin(ang))*reach*ph
		r.drawGlowRect(screen, sx, sy, math.Max(1.5, size*0.35), [3]int{255, 250, 200},
			flick*(1-ph)*0.95, additiveGlowBlend)
	}
}
