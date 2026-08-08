package game

import (
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"

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
	// Bubbles need enough pixels for a rim and a highlight to read at all, so the
	// steam field draws them bigger than the old squares and varies their size -
	// a uniform size looks like a mechanical grid, not boiling water.
	steamBubbleSizeFloor  = 2.5
	steamBubbleSizeCoef   = 0.085
	steamBubbleSizeJitter = 0.45
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
		// A FIRE zone burns instead of bubbling. Keyed off the spell's school, so
		// any future fire zone gets flames without another YAML knob.
		flame := false
		if def, err := spells.GetSpellDefinitionByID(spells.SpellID(z.SpellID)); err == nil {
			flame = normalizeDamageTypeStr(def.School) == string(damagecalc.Fire)
		}
		// Fade reference. Deriving it from the radius suits a wide blob (Hot Steam
		// is 3 tiles across) but blacks out a NARROW cell: Firewall's cells are
		// 0.55 tiles, so a radius-derived depth faded the wall to nothing two tiles
		// away - fire gets its own reach instead.
		maxDepth := z.Radius + 2*ts
		if flame {
			maxDepth = z.Radius + flameFadeTiles*ts
		}
		ctx, cty := TileIndex(z.X, ts), TileIndex(z.Y, ts)
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
				// A wall zone emits along its axis only: a flat curtain of fire.
				if flame && (z.AxisX != 0 || z.AxisY != 0) {
					for i := 0; i < flameWallColumns; i++ {
						f := (float64(i)+0.5)/float64(flameWallColumns) - 0.5
						wx := z.X + z.AxisX*f*ts
						wy := z.Y + z.AxisY*f*ts
						r.emitFlameColumn(screen, wx, wy, tx, ty, i, maxDepth)
					}
					continue
				}
				samples := steamSamplesPerAxis
				if flame {
					samples = flameSamplesPerAxis
				}
				for sy := 0; sy < samples; sy++ {
					for sx := 0; sx < samples; sx++ {
						fx := (float64(sx) + 0.5) / float64(samples)
						fy := (float64(sy) + 0.5) / float64(samples)
						wx := (float64(tx) + fx) * ts
						wy := (float64(ty) + fy) * ts
						idx := sy*samples + sx
						if flame {
							r.emitFlameColumn(screen, wx, wy, tx, ty, idx, maxDepth)
							continue
						}
						r.emitSteamColumn(screen, wx, wy, tx, ty, idx, maxDepth)
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
		sizeFloor:    steamBubbleSizeFloor,
		sizeCoef:     steamBubbleSizeCoef,
		wobbleCoef:   0.8,
		sizeJitter:   steamBubbleSizeJitter,
		round:        true, // real bubbles (rim + highlight), not glow squares
		color:        steamBubbleColor,
	})
}

// Flame field (Firewall). Same rising-column machinery as the steam bubbles,
// tuned into fire: denser sampling, tall fast tongues that taper as they climb,
// and a colour ramp from white-hot at the base through orange to a dark smoky
// red at the tip. Additive, depth-tested, so a wall behind it still occludes.
const (
	flameSamplesPerAxis = 4    // columns per tile axis
	flameRiseMultiplier = 2.4  // ~1.3 tiles of flame height (aura fraction is 0.55)
	flameBaseAlpha      = 0.85 // per tongue, source-over: the wall hides the ground behind it
	flamePeriodTick     = 26.0 // fast travel = flicker rather than drift
	flameTipSizeScale   = 0.25 // tongue narrows to a quarter of its base width
	flameSizeJitter     = 0.35
	flameTongueAspect   = 2.8  // each tongue is this many times taller than wide
	flameWallColumns    = 22   // dense enough that neighbouring tongues MERGE into a sheet
	flamePerColumn      = 4    // staggered tongues per column
	flameFadeTiles      = 16.0 // a burning wall must still read from across a room
)

var (
	flameCoreColor = [3]int{255, 176, 48} // orange base
	flameTipColor  = [3]int{132, 26, 10}  // dark ember tip
)

// Burning MOB flames: the wall's tuning, shrunk. A mob is not a curtain - fewer,
// shorter tongues, and no merging.
const (
	monsterFlameRiseMultiplier = 1.3 // ~0.7 tiles of flame: up the body, not over it
	monsterFlameColumns        = 5   // tongues across the mob's width
	monsterFlamePerColumn      = 4
	monsterFlameSizeFloor      = 4.0  // the wall uses 5.0 - a mob's tongues are smaller
	monsterFlameSizeCoef       = 0.17 // ... and thinner (the wall's 0.26)
	monsterFlameFadeTiles      = 12.0
	monsterFlameFrontOffset    = 0.28 // tiles toward the camera, so the mob does not occlude its own fire
)

// monsterFlameMaxDepth is the far clip for a burning mob's tongues.
func monsterFlameMaxDepth(tileSize float64) float64 {
	return monsterFlameFadeTiles * tileSize
}

// emitMonsterFlameColumn draws one short flame tongue stack on a burning mob,
// through the same emitter the Firewall uses.
func (r *Renderer) emitMonsterFlameColumn(screen *ebiten.Image, wx, wy float64, tx, ty, sIdx int, maxDepth float64) {
	r.emitBubbleColumn(screen, bubbleColumnFx{
		wx: wx, wy: wy,
		hx: tx, hy: ty, hi: sIdx,
		salt:         11, // decorrelate from the steam and wall streams
		maxDepth:     maxDepth,
		riseFraction: auraRiseFraction * monsterFlameRiseMultiplier,
		baseAlpha:    flameBaseAlpha,
		colBright:    1.0,
		perColumn:    monsterFlamePerColumn,
		periodTick:   flamePeriodTick,
		jitterMin:    auraSpeedJitterMin,
		jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
		sizeFloor:    monsterFlameSizeFloor,
		sizeCoef:     monsterFlameSizeCoef,
		wobbleCoef:   0.5,
		sizeJitter:   flameSizeJitter,
		soft:         true,
		sizeTaper:    flameTipSizeScale,
		color:        flameCoreColor,
		heightScale:  flameTongueAspect,
		srcOver:      true,
		colorTop:     flameTipColor,
	})
}

// emitFlameColumn draws one flame tongue stack at a sampled point of a fire zone.
func (r *Renderer) emitFlameColumn(screen *ebiten.Image, wx, wy float64, tx, ty, sIdx int, maxDepth float64) {
	r.emitBubbleColumn(screen, bubbleColumnFx{
		wx: wx, wy: wy,
		hx: tx, hy: ty, hi: sIdx,
		salt:         7, // decorrelate from the steam field's hash stream
		maxDepth:     maxDepth,
		riseFraction: auraRiseFraction * flameRiseMultiplier,
		baseAlpha:    flameBaseAlpha,
		colBright:    1.0,
		perColumn:    flamePerColumn,
		periodTick:   flamePeriodTick,
		jitterMin:    auraSpeedJitterMin,
		jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
		sizeFloor:    5.0,
		sizeCoef:     0.26,
		wobbleCoef:   0.5,
		sizeJitter:   flameSizeJitter,
		soft:         true, // round soft glows read as flame; rects read as bricks
		sizeTaper:    flameTipSizeScale,
		color:        flameCoreColor,
		heightScale:  flameTongueAspect, // tongues, not puddles
		srcOver:      true,              // fire OCCLUDES what is behind it
		colorTop:     flameTipColor,
	})
}
