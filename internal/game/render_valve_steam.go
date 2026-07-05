package game

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// Valve steam fills the whole tile of a shut culvert valve with rising columns -
// same look as the impassable-tile aura but ~2x taller and sampled across the
// tile interior (not just its edges).
const (
	valveSteamRiseFraction = 1.1 // ~2x the impassable aura's rise
	valveSteamGrid         = 4   // NxN sample points across the tile
	valveSteamPerPoint     = 2
	valveSteamPeriodTick   = 90.0
	valveSteamColMin       = 0.45
	valveSteamBaseAlpha    = 0.6
	valveSteamJitterMin    = 0.6 // rise-speed jitter band lower bound
	valveSteamJitterSpan   = 0.8 // ... and width: speed in [0.6, 1.4]xbase
)

var valveSteamColor = [3]int{225, 230, 236} // pale gray-white

// drawClosedValveSteam draws rising steam across the tile of every shut valve
// (SteamWhenVisited NPCs that have been Visited), hugging the tile bounds and
// depth-tested against walls. Procedural per-frame, mirroring drawImpassableTileAura.
func (r *Renderer) drawClosedValveSteam(screen *ebiten.Image) {
	if r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	maxDepth := r.game.camera.ViewDist

	for _, n := range r.game.world.NPCs {
		if n == nil || !n.SteamWhenVisited || !n.Visited {
			continue
		}
		tx := int(n.X / ts)
		ty := int(n.Y / ts)
		for gy := 0; gy < valveSteamGrid; gy++ {
			for gx := 0; gx < valveSteamGrid; gx++ {
				fx := (float64(gx) + 0.5) / float64(valveSteamGrid)
				fy := (float64(gy) + 0.5) / float64(valveSteamGrid)
				colBright := valveSteamColMin + (1.0-valveSteamColMin)*auraHash(tx, ty, gx+50, gy+50)
				r.emitBubbleColumn(screen, bubbleColumnFx{
					wx: (float64(tx) + fx) * ts, wy: (float64(ty) + fy) * ts,
					hx: tx, hy: ty, hi: gy*valveSteamGrid + gx,
					maxDepth:     maxDepth,
					riseFraction: valveSteamRiseFraction,
					baseAlpha:    valveSteamBaseAlpha,
					colBright:    colBright,
					perColumn:    valveSteamPerPoint,
					periodTick:   valveSteamPeriodTick,
					jitterMin:    valveSteamJitterMin,
					jitterSpan:   valveSteamJitterSpan,
					sizeFloor:    2,
					sizeCoef:     0.05,
					wobbleCoef:   0.7,
					color:        valveSteamColor,
				})
			}
		}
	}
}
