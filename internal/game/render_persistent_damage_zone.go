package game

import (
	"math"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// Steam-zone bubbles (Hot Steam) rise across every covered tile and climb
// twice as high as the tile-edge curtain. Walls clip them; the shared painter
// pass supplies scenery and actor occlusion.
const (
	steamSamplesPerAxis = 3                  // NxN bubble columns spread across each tile
	steamRiseMultiplier = 2.0                // twice the impassable-aura rise height
	steamBaseAlpha      = 0.5                //
	steamRisePeriodTick = auraRisePeriodTick // reuse the aura's bubble travel period
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
	zoneVisualHeat
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

	for _, key := range r.visibleZoneHeatTiles() {
		tx, ty := key[0], key[1]
		x, y := (float64(tx)+.5)*ts, (float64(ty)+.5)*ts
		add(zoneVisual{kind: zoneVisualHeat, x: x, y: y, maxDepth: flameFadeTiles * ts})
	}
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
						add(zoneVisual{kind: zoneVisualFlame, x: wx, y: wy, maxDepth: maxDepth})
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

func (r *Renderer) drawZoneVisual(screen *ebiten.Image, v zoneVisual) {
	switch v.kind {
	case zoneVisualHeat:
		r.drawZonePlume(screen, v.x, v.y, .75, .3, v.maxDepth, .3, flameCoreColor)
	case zoneVisualFlame:
		r.emitFlameColumn(screen, v.x, v.y, v.maxDepth)
	case zoneVisualTrapFlame:
		r.drawZonePlume(screen, v.x, v.y, .075, auraRiseFraction*trapFlameRiseMult, v.maxDepth, trapFlameBaseAlpha, flameCoreColor)
	case zoneVisualSteam:
		r.emitSteamColumn(screen, v.x, v.y, v.tileX, v.tileY, v.slot, v.maxDepth)
	}
}

// visibleZoneHeatTiles computes the exterior of the whole live flame field,
// not each cell independently. Culling precedes terrain/door ray checks.
func (r *Renderer) visibleZoneHeatTiles() [][2]int {
	g := r.game
	ts := float64(g.config.GetTileSize())
	core := map[[2]int]bool{}
	var zones []*PersistentDamageZone
	for i := range g.persistentDamageZones {
		z := &g.persistentDamageZones[i]
		def, _ := config.GetSpellDefinition(z.SpellID)
		if z.FramesLeft <= 0 || z.MapKey != "" && !mapKeyOnCurrentWorld(z.MapKey) || def == nil || normalizeDamageTypeStr(def.School) != string(damagecalc.Fire) {
			continue
		}
		cx, cy := TileIndex(z.X, ts), TileIndex(z.Y, ts)
		reach := int(z.Radius/ts) + 1
		for dy := -reach; dy <= reach; dy++ {
			for dx := -reach; dx <= reach; dx++ {
				x, y := (float64(cx+dx)+.5)*ts, (float64(cy+dy)+.5)*ts
				if z.coversMonster(x, y, ts) {
					core[[2]int{cx + dx, cy + dy}] = true
				}
			}
		}
		if def.ZoneEdgeTiles > 0 && z.isWallCell() {
			zones = append(zones, z)
		}
	}
	var tiles [][2]int
	drawn := map[[2]int]bool{}
	for _, z := range zones {
		def, _ := config.GetSpellDefinition(z.SpellID)
		reach := def.ZoneEdgeTiles
		// A conservative field bound keeps visible edge tiles when the core is offscreen.
		if !r.zoneHeatVisible(z.X, z.Y, float64(reach+1)*ts) {
			continue
		}
		cx, cy := TileIndex(z.X, ts), TileIndex(z.Y, ts)
		for dy := -reach; dy <= reach; dy++ {
			for dx := -reach; dx <= reach; dx++ {
				key := [2]int{cx + dx, cy + dy}
				x, y := (float64(key[0])+.5)*ts, (float64(key[1])+.5)*ts
				if core[key] || drawn[key] || !r.zoneHeatVisible(x, y, ts/2) {
					continue
				}
				if g.combat.zoneDamageBand(z, x, y, ts) == 1 {
					drawn[key] = true
					tiles = append(tiles, key)
				}
			}
		}
	}
	return tiles
}

func (r *Renderer) zoneHeatVisible(x, y, margin float64) bool {
	cam := r.game.camera
	dx, dy := x-cam.X, y-cam.Y
	limit := min(cam.ViewDist, flameFadeTiles*float64(r.game.config.GetTileSize())) + margin
	if dx*dx+dy*dy > limit*limit {
		return false
	}
	forward := dx*math.Cos(cam.Angle) + dy*math.Sin(cam.Angle)
	side := -dx*math.Sin(cam.Angle) + dy*math.Cos(cam.Angle)
	return forward+margin > 0 && math.Abs(side) <= max(0, forward)*math.Tan(r.game.viewFOV()/2)+margin
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

// Fire zones and burning creatures share shader plumes at different scales.
// Both use source-over blending so bright terrain does not wash out the flame.
// World fields and attached actor flames share the depth-sorted sprite pass.
const (
	flameSamplesPerAxis = 2    // columns per tile axis
	flameBaseAlpha      = 0.85 // opacity of the smaller burning-creature plumes
	flameWallColumns    = 3    // overlapping shader plumes across one tile of the wall
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

// emitFlameColumn draws one independently moving plume within a fire zone.
func (r *Renderer) emitFlameColumn(screen *ebiten.Image, wx, wy, maxDepth float64) {
	r.drawZonePlume(screen, wx, wy, .42, 2.8, maxDepth, .92, flameCoreColor)
}
