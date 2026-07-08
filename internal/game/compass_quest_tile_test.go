package game

import (
	"testing"

	"ugataima/internal/world"
)

// The compass minimap's tile layer is cached the same way the floor renderer
// bakes floor colors (see the comment in syncQuestTiles), and goes stale for
// the same reason: a quest can swap a tile out from under a player who never
// crosses the cache's tile-boundary trigger (forest_wolf_cull lays its bridge
// under a party that hasn't moved). Both caches must be invalidated together.
func TestSyncQuestTiles_InvalidatesCompassMinimapCache(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, qm := loadRealWorldForTest(t, cfg, "forest")
	forest := wm.GetCurrentWorld()

	g := newTestGame(cfg, forest)
	g.questManager = qm
	g.gameLoop = &GameLoop{game: g, ui: NewUISystem(g), renderer: NewRenderer(g)}

	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	def := qm.Definitions()["forest_wolf_cull"]
	if len(def.OnCompleteTiles) == 0 {
		t.Fatal("test quest has no on_complete_tiles; picked the wrong quest")
	}
	tc := def.OnCompleteTiles[0]

	// Simulate a compass tile layer already cached for a player who is - and
	// stays - on some tile, well before the quest completes.
	g.gameLoop.ui.rebuildCompassTileLayer(0, 0, 5, 4, 40)
	if g.gameLoop.ui.compassCacheWorld == nil {
		t.Fatal("setup failed: compass cache didn't populate")
	}

	qm.MarkCompleted("forest_wolf_cull")
	g.applyCompletedQuestTiles()

	if forest.Tiles[tc.Y][tc.X] == world.TileType3D(0) {
		t.Fatal("setup failed: on_complete_tiles didn't change the world tile")
	}
	if g.gameLoop.ui.compassCacheWorld != nil {
		t.Fatal("compass minimap cache survived a current-map quest tile change; " +
			"the minimap will show the pre-quest tile until the player crosses a tile boundary")
	}
}
