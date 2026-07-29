package game

import (
	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawTrapTileBorders outlines every armed trap's tile with rising bubble
// pixels in the trap's thematic border colour - the same edge-bubble technique
// as the impassable aura, but on all four edges of the trap tile so the armed
// square reads clearly on the floor.
func (r *Renderer) drawTrapTileBorders(screen *ebiten.Image) {
	traps := r.game.traps
	if len(traps) == 0 || r.game.world == nil {
		return
	}
	baseAlpha, perEdge, radius := r.auraEdgeParams()

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
		r.emitAuraTileEdges(screen, t.TileX, t.TileY, ts, perEdge, baseAlpha, maxDepth, rgb)
	}
}

// drawBossFireTrapBorders marks the Brood Mother's field: every armed tile is
// edged with MANY SMALL fire-glows - the same edge emitter as trap borders,
// tuned dense and dim so the ground reads as smouldering, not bubbling (user
// spec: many tiny flames along the tile edges).
func (r *Renderer) drawBossFireTrapBorders(screen *ebiten.Image) {
	field := r.game.bossFireTraps
	if len(field) == 0 {
		return
	}
	baseAlpha, perEdge, radius := r.auraEdgeParams()
	baseAlpha *= 0.8
	perEdge = perEdge*2 + 2 // many small embers instead of a few bubbles

	ts := float64(r.game.config.GetTileSize())
	maxDepth := float64(radius) * ts
	ember := [3]int{255, 130, 40}
	for _, t := range field {
		r.emitAuraTileEdges(screen, t.TX, t.TY, ts, perEdge, baseAlpha, maxDepth, ember)
	}
}
