package game

import (
	"cmp"
	"math"
	"slices"
	damagecalc "ugataima/internal/damage"

	"github.com/hajimehoshi/ebiten/v2"
)

// Steam-zone bubbles (Hot Steam) rise across every covered tile and climb
// twice as high as the tile-edge curtain. Walls clip them; the shared painter
// pass supplies scenery and actor occlusion.
const (
	steamSamplesPerAxis    = 3                     // NxN bubble columns spread across each tile
	steamRiseMultiplier    = 2.0                   // twice the impassable-aura rise height
	steamBaseAlpha         = 0.5                   //
	steamRisePeriodSeconds = auraRisePeriodSeconds // reuse the aura's bubble travel period
	// Bubbles need enough pixels for a rim and a highlight to read at all, so the
	// steam field draws them bigger than the old squares and varies their size -
	// a uniform size looks like a mechanical grid, not boiling water.
	steamBubbleSizeFloor  = 3.5
	steamBubbleSizeCoef   = 0.11
	steamBubbleSizeJitter = 0.45
)

// steamBubbleColor is the light blue-white of hot steam.
var steamBubbleColor = [3]int{205, 228, 245}

type zoneVisualKind uint8

const (
	zoneVisualFlame zoneVisualKind = iota
	zoneVisualSteam
	zoneVisualTrapFlame
)

type zoneVisual struct {
	kind               zoneVisualKind
	x, y, maxDepth     float64
	tileX, tileY, slot int
}

// collectPersistentDamageZoneEffects puts every column in the same painter
// order as scenery and actors. Alpha silhouettes then occlude only their opaque
// pixels, while a field in front still covers the objects behind it.
func (r *Renderer) collectPersistentDamageZoneEffects(sprites []UnifiedSpriteRenderData) []UnifiedSpriteRenderData {
	r.zoneVisuals = r.zoneVisuals[:0]
	if len(r.game.persistentDamageZones) == 0 || r.game.world == nil {
		return sprites
	}
	add := func(v zoneVisual) {
		x, depth, ok := r.game.renderHelper.projectToScreenXF(v.x, v.y)
		if !ok || depth < auraMinDepth || depth > v.maxDepth {
			return
		}
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeZoneEffect, depthPerp: depth, screenX: int(x),
			tileX: len(r.zoneVisuals), // index into the reused zone-visual buffer
		})
		r.zoneVisuals = append(r.zoneVisuals, v)
	}
	ts := float64(r.game.config.GetTileSize())

	for zi := range r.game.persistentDamageZones {
		z := &r.game.persistentDamageZones[zi]
		if z.FramesLeft <= 0 || z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) {
			continue
		}
		// Whole zone beyond view distance -> nothing of it can render.
		zdx, zdy := z.X-r.game.camera.X, z.Y-r.game.camera.Y
		if reach := r.game.camera.ViewDist + z.Radius; zdx*zdx+zdy*zdy > reach*reach {
			continue
		}
		// A FIRE zone burns instead of bubbling. Keyed off the spell's school, so
		// any future fire zone gets flames without another YAML knob.
		flame := spellDamageTypeStr(z.SpellID) == string(damagecalc.Fire)
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
				// A wall zone burns along its axis in two staggered rows, so the
				// blaze has depth from any angle instead of a flat curtain.
				if flame && (z.AxisX != 0 || z.AxisY != 0) {
					for row := -1.0; row <= 1; row += 2 {
						for i := 0; i < flameWallColumns; i++ {
							f := (float64(i)+0.5+.25*row)/float64(flameWallColumns) - 0.5
							wx := z.X + z.AxisX*f*ts - z.AxisY*row*flameWallDepth*ts
							wy := z.Y + z.AxisY*f*ts + z.AxisX*row*flameWallDepth*ts
							add(zoneVisual{kind: zoneVisualFlame, x: wx, y: wy, maxDepth: maxDepth})
						}
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
							add(zoneVisual{kind: zoneVisualFlame, x: wx, y: wy, maxDepth: maxDepth})
							continue
						}
						add(zoneVisual{kind: zoneVisualSteam, x: wx, y: wy, tileX: tx, tileY: ty, slot: idx, maxDepth: maxDepth})
					}
				}
			}
		}
	}
	return sprites
}

// collectFireHeat lists the fire cells in reach, nearest first, as softly
// flickering heat sources the floor shader tints warm.
func (r *Renderer) collectFireHeat(tile float64) {
	r.heatSources = r.heatSources[:0]
	cam := r.game.camera
	for i := range r.game.persistentDamageZones {
		z := &r.game.persistentDamageZones[i]
		if z.FramesLeft <= 0 || z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) || spellDamageTypeStr(z.SpellID) != string(damagecalc.Fire) {
			continue
		}
		radius := z.Radius + fireHeatGlowTiles*tile
		if dx, dy := z.X-cam.X, z.Y-cam.Y; dx*dx+dy*dy > (cam.ViewDist+radius)*(cam.ViewDist+radius) {
			continue
		}
		clock := r.game.worldSeconds() + auraHash(int(z.X), int(z.Y), 7, 9)*11
		flicker := .88 + .08*math.Sin(clock*5.3) + .04*math.Sin(clock*11.7)
		r.heatSources = append(r.heatSources, LightSource{X: z.X, Y: z.Y, Radius: radius, Intensity: fireHeatGlowStrength * flicker})
	}
	if len(r.heatSources) > maxFloorHeatSources {
		slices.SortFunc(r.heatSources, func(a, b LightSource) int {
			return cmp.Compare(Distance(a.X, a.Y, cam.X, cam.Y), Distance(b.X, b.Y, cam.X, cam.Y))
		})
	}
}

func (r *Renderer) drawZoneVisual(screen *ebiten.Image, v zoneVisual) {
	switch v.kind {
	case zoneVisualFlame:
		r.emitFlameColumn(screen, v.x, v.y, v.maxDepth)
	case zoneVisualTrapFlame:
		r.drawZonePlume(screen, v.x, v.y, .075, auraRiseFraction*trapFlameRiseMult, v.maxDepth, trapFlameBaseAlpha, flameCoreColor)
	case zoneVisualSteam:
		r.emitSteamColumn(screen, v.x, v.y, v.tileX, v.tileY, v.slot, v.maxDepth)
	}
}

// emitSteamColumn draws one rising bubble at a sampled point inside a steam-zone
// tile, occluded by walls and faded with distance.
func (r *Renderer) emitSteamColumn(screen *ebiten.Image, wx, wy float64, tx, ty, sIdx int, maxDepth float64) {
	r.emitBubbleColumn(screen, bubbleColumnFx{
		wx: wx, wy: wy,
		hx: tx, hy: ty, hi: sIdx,
		maxDepth:      maxDepth,
		riseFraction:  auraRiseFraction * steamRiseMultiplier, // climbs twice as high as the aura
		baseAlpha:     steamBaseAlpha,
		colBright:     1.0,
		perColumn:     1,
		periodSeconds: steamRisePeriodSeconds,
		jitterMin:     auraSpeedJitterMin,
		jitterSpan:    (1.0 - auraSpeedJitterMin) * 2,
		sizeFloor:     steamBubbleSizeFloor,
		sizeCoef:      steamBubbleSizeCoef,
		wobbleCoef:    0.8,
		sizeJitter:    steamBubbleSizeJitter,
		round:         true, // real bubbles (rim + highlight), not glow squares
		color:         steamBubbleColor,
	})
}

// Fire zones and burning creatures share shader plumes at different scales.
// Both use source-over blending so bright terrain does not wash out the flame.
// World fields and attached actor flames share the depth-sorted sprite pass.
const (
	flameSamplesPerAxis = 2    // columns per tile axis
	flameBaseAlpha      = 0.85 // opacity of the smaller burning-creature plumes
	flameWallColumns    = 3    // overlapping blaze columns across one tile of the wall, per row
	flameWallDepth      = .16  // half the spacing of the wall's two rows, in tiles
	flameFadeTiles      = 16.0 // a burning wall must still read from across a room
)

var flameCoreColor = [3]int{255, 176, 48} // orange base

// Burning creatures carry a compact group of shader plumes up their bodies.
const (
	monsterFlameRiseMultiplier = .65 // about 0.36 of the projected floor-rise
	monsterFlameColumns        = 3
	monsterFlameFadeTiles      = 12.0
)

func monsterFlameMaxDepth(tileSize float64) float64 {
	return monsterFlameFadeTiles * tileSize
}

// emitFlameColumn draws one column of a fire zone's blaze. Wide columns
// overlap their neighbours so the zone burns as one fire.
func (r *Renderer) emitFlameColumn(screen *ebiten.Image, wx, wy, maxDepth float64) {
	r.drawFirePlume(screen, wx, wy, .55, 3.0, maxDepth, 1, flameCoreColor, int(wx), int(wy), firePlumeBlaze)
}
