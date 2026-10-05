package game

import (
	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// collectTrapTileBorders collects every armed trap. A trap whose armed_fx names a
// bespoke style (trapFxStyleDraw) gets that centre-anchored effect instead of an
// outline, so traps stop reading as identical squares; traps without armed_fx
// use the shared aurora curtain from the impassable aura, tinted by
// border_color.
func (r *Renderer) collectTrapTileBorders(sprites []UnifiedSpriteRenderData) []UnifiedSpriteRenderData {
	traps := r.game.traps
	if len(traps) == 0 || r.game.world == nil {
		return sprites
	}
	baseAlpha, density, radius := r.auraEdgeParams()

	ts := float64(r.game.config.GetTileSize())
	maxDepth := float64(radius) * ts

	for i := range traps {
		t := &traps[i]
		if !mapKeyOnCurrentWorld(t.MapKey) {
			continue
		}
		def, ok := config.GetTrapDefinition(t.Key)
		if !ok {
			continue
		}
		rgb := [3]int{
			clampColor(def.BorderColor[0]),
			clampColor(def.BorderColor[1]),
			clampColor(def.BorderColor[2]),
		}
		if _, ok := trapFxStyleDraw[def.ArmedFx]; ok {
			if a, visible := r.trapFloorAnchor(t.TileX, t.TileY, ts, maxDepth); visible {
				sprites = append(sprites, UnifiedSpriteRenderData{
					spriteType: SpriteTypeArmedTrap, depthPerp: a.depth,
					tileX: i, // index into the current frame's armed traps
				})
			}
			continue
		}
		r.collectAuraTileEdges(t.TileX, t.TileY, ts, density, baseAlpha, maxDepth, rgb)
	}
	return sprites
}

func (r *Renderer) drawArmedTrap(screen *ebiten.Image, index int) {
	t := r.game.traps[index]
	def, ok := config.GetTrapDefinition(t.Key)
	if !ok {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	_, _, radius := r.auraEdgeParams()
	a, visible := r.trapFloorAnchor(t.TileX, t.TileY, ts, float64(radius)*ts)
	if draw := trapFxStyleDraw[def.ArmedFx]; visible && draw != nil {
		rgb := [3]int{clampColor(def.BorderColor[0]), clampColor(def.BorderColor[1]), clampColor(def.BorderColor[2])}
		// Tile-derived seeds preserve each trap's animation across frames.
		draw(r, screen, a, rgb, t.TileX*73+t.TileY*131)
	}
}

// Brood-fire trap edges use the same plume material as fire zones, scaled
// down to ground embers. Each sample joins the scene painter order.
const (
	trapFlamePerEdge   = 7
	trapFlameRiseMult  = .85
	trapFlameBaseAlpha = .8
	trapFlameFadeTiles = 14.0
)

func (r *Renderer) collectBossFireTrapBorders(sprites []UnifiedSpriteRenderData) []UnifiedSpriteRenderData {
	ts := float64(r.game.config.GetTileSize())
	maxDepth := trapFlameFadeTiles * ts
	for _, t := range r.game.bossFireTraps {
		for _, d := range auraCardinalDirections {
			for i := 0; i < trapFlamePerEdge; i++ {
				wx, wy := tileEdgeSamplePoint(t.TX, t.TY, d, ts, i, trapFlamePerEdge)
				sx, depth, ok := r.game.renderHelper.projectToScreenXF(wx, wy)
				if !ok || depth < auraMinDepth || depth > maxDepth {
					continue
				}
				sprites = append(sprites, UnifiedSpriteRenderData{
					spriteType: SpriteTypeZoneEffect, depthPerp: depth, screenX: int(sx),
					tileX: len(r.zoneVisuals),
				})
				r.zoneVisuals = append(r.zoneVisuals, zoneVisual{
					kind: zoneVisualTrapFlame, x: wx, y: wy, maxDepth: maxDepth,
				})
			}
		}
	}
	return sprites
}
