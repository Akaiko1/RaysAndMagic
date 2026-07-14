package game

import (
	"testing"

	"ugataima/internal/character"
)

// Creature and NPC tokens must remain projectable right up to the camera, like
// loot containers. They may still disappear once behind the camera, but no
// near-cull may make them pop before the player reaches their tile.
func TestMonsterAndNPCSpritesHaveNoNearCull(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 8, 8)
	game.renderHelper = NewRenderingHelper(game)
	game.camera.Angle = 0
	game.camera.FOV = game.config.GetCameraFOV()
	game.camera.ViewDist = game.config.GetViewDistance()
	x, y := game.camera.X+2, game.camera.Y

	if _, _, _, visible := game.renderHelper.CalculateMonsterSpriteMetrics(x, y, 2, 1); !visible {
		t.Fatal("monster sprite was near-culled")
	}
	npc := &character.NPC{RenderCategory: "scenery", SizeTiles: 1}
	if _, _, _, visible := game.renderHelper.NPCSpriteMetrics(npc, x, y, 2); !visible {
		t.Fatal("NPC sprite was near-culled")
	}
}
