package game

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
