package game

import (
	"testing"

	"ugataima/internal/world"
)

// The wolf-cull bridge must target real water tiles on the REAL forest map and
// swap them both - a coordinate typo here renders as "one plank, one water".
func TestWolfCullBridge_RealForestCoordinates(t *testing.T) {
	cfg := loadTestConfig(t)

	wm, qm := loadRealWorldForTest(t, cfg, "")
	forest := wm.LoadedMaps["forest"]
	if forest == nil {
		t.Fatal("forest map missing")
	}

	questCfg := qm.Definitions()
	def := questCfg["forest_wolf_cull"]
	if def == nil || len(def.OnCompleteTiles) == 0 {
		t.Fatal("forest_wolf_cull on_complete_tiles missing")
	}

	// Every targeted tile must be WATER on the pristine map (the gap the
	// bridge spans), not some other tile.
	for _, tc := range def.OnCompleteTiles {
		got := forest.Tiles[tc.Y][tc.X]
		key := world.GlobalTileManager.GetTileKey(got)
		t.Logf("pre-quest tile (%d,%d) = %v (%s)", tc.X, tc.Y, got, key)
		if key != "water" && key != "deep_water" {
			t.Errorf("on_complete_tiles targets (%d,%d) which is %q, not water - wrong coordinates", tc.X, tc.Y, key)
		}
	}

	g := newTestGame(cfg, forest)
	g.questManager = qm
	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("forest_wolf_cull")
	g.applyCompletedQuestTiles()

	wantType, ok := world.GlobalTileManager.GetTileTypeFromKey(def.OnCompleteTiles[0].Tile)
	if !ok {
		t.Fatalf("bridge tile key %q unknown", def.OnCompleteTiles[0].Tile)
	}
	for _, tc := range def.OnCompleteTiles {
		if forest.Tiles[tc.Y][tc.X] != wantType {
			t.Errorf("post-quest tile (%d,%d) = %v, want bridge %v", tc.X, tc.Y, forest.Tiles[tc.Y][tc.X], wantType)
		}
	}
}
