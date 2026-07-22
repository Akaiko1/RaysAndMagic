package game

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// Steam-zone bubble field (Hot Steam). Like the impassable-tile aura, but the
// bubbles rise over the WHOLE square of every covered tile (not just edges) and
// climb twice as high. Drawn each frame, depth-tested against walls.
const (
	steamSamplesPerAxis = 3                  // NxN bubble columns spread across each tile
	steamRiseMultiplier = 2.0                // twice the impassable-aura rise height
	steamBaseAlpha      = 0.5                //
	steamRisePeriodTick = auraRisePeriodTick // reuse the aura's bubble travel period
)

// steamBubbleColor is the light blue-white of hot steam.
var steamBubbleColor = [3]int{205, 228, 245}

// drawSteamZoneBubbles renders rising steam bubbles across every tile covered by
// an active Hot Steam zone, in both real-time and turn-based modes.
func (r *Renderer) drawSteamZoneBubbles(screen *ebiten.Image) {
	if len(r.game.steamZones) == 0 || r.game.world == nil {
		return
	}
	ts := float64(r.game.config.GetTileSize())

	for zi := range r.game.steamZones {
		z := &r.game.steamZones[zi]
		if z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) {
			continue
		}
		// Whole zone beyond view distance -> nothing of it can render.
		zdx, zdy := z.X-r.game.camera.X, z.Y-r.game.camera.Y
		if reach := r.game.camera.ViewDist + z.Radius; zdx*zdx+zdy*zdy > reach*reach {
			continue
		}
		maxDepth := z.Radius + 2*ts // bright across the zone, fading just past its edge
		ctx, cty := int(z.X/ts), int(z.Y/ts)
		rt := int(z.Radius/ts) + 1
		for ty := cty - rt; ty <= cty+rt; ty++ {
			if ty < 0 || ty >= r.game.world.Height {
				continue
			}
			for tx := ctx - rt; tx <= ctx+rt; tx++ {
				if tx < 0 || tx >= r.game.world.Width {
					continue
				}
				// Include the tile if its centre is within the zone's circle.
				cxw, cyw := (float64(tx)+0.5)*ts, (float64(ty)+0.5)*ts
				if Distance(z.X, z.Y, cxw, cyw) > z.Radius {
					continue
				}
				for sy := 0; sy < steamSamplesPerAxis; sy++ {
					for sx := 0; sx < steamSamplesPerAxis; sx++ {
						fx := (float64(sx) + 0.5) / float64(steamSamplesPerAxis)
						fy := (float64(sy) + 0.5) / float64(steamSamplesPerAxis)
						wx := (float64(tx) + fx) * ts
						wy := (float64(ty) + fy) * ts
						r.emitSteamColumn(screen, wx, wy, tx, ty, sy*steamSamplesPerAxis+sx, maxDepth)
					}
				}
			}
		}
	}
}

// emitSteamColumn draws one rising bubble at a sampled point inside a steam-zone
// tile, occluded by walls and faded with distance.
func (r *Renderer) emitSteamColumn(screen *ebiten.Image, wx, wy float64, tx, ty, sIdx int, maxDepth float64) {
	r.emitBubbleColumn(screen, bubbleColumnFx{
		wx: wx, wy: wy,
		hx: tx, hy: ty, hi: sIdx,
		maxDepth:     maxDepth,
		riseFraction: auraRiseFraction * steamRiseMultiplier, // climbs twice as high as the aura
		baseAlpha:    steamBaseAlpha,
		colBright:    1.0,
		perColumn:    1,
		periodTick:   steamRisePeriodTick,
		jitterMin:    auraSpeedJitterMin,
		jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
		sizeFloor:    1.5,
		sizeCoef:     0.05,
		wobbleCoef:   0.6,
		color:        steamBubbleColor,
	})
}
