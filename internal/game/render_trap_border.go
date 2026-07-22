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
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

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
		for _, d := range dirs {
			r.emitAuraEdge(screen, t.TileX, t.TileY, d, ts, perEdge, baseAlpha, maxDepth, rgb)
		}
	}
}
