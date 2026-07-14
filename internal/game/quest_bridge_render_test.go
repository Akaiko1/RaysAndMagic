package game

import (
	"testing"

	"ugataima/internal/world"
)

// After the wolf-cull completes, BOTH bridge tiles must bake to the SAME floor
// atlas index - "one plank, one stream" means the baked index map diverged
// from the world tiles.
func TestWolfCullBridge_FloorBakeIndices(t *testing.T) {
	cfg := loadTestConfig(t)

	wm, qm := loadRealWorldForTest(t, cfg, "forest")
	forest := wm.GetCurrentWorld()

	g := newTestGame(cfg, forest)
	g.questManager = qm
	r := NewRenderer(g)

	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("forest_wolf_cull")
	g.applyCompletedQuestTiles()
	// applyCompletedQuestTiles can't reach this renderer (no gameLoop in the
	// test) - rebake explicitly, same call the live path makes.
	r.precomputeFloorColorCache()

	for name, grp := range r.floorTexGroups {
		t.Logf("atlas group %q: start=%d count=%d", name, grp.start, grp.count)
	}

	def := qm.Definitions()["forest_wolf_cull"]
	var indices []int
	for _, tc := range def.OnCompleteTiles {
		tileType := forest.Tiles[tc.Y][tc.X]
		group := r.floorTextureGroupForTile(tc.X, tc.Y, tileType)
		idx, ok := r.floorTextureIndexForTile(tc.X, tc.Y, tileType)
		t.Logf("tile (%d,%d): type=%v key=%s group=%q atlasIdx=%d ok=%v",
			tc.X, tc.Y, tileType, world.GlobalTileManager.GetTileKey(tileType), group, idx, ok)
		if !ok {
			t.Errorf("tile (%d,%d) bakes with NO texture (group %q missing from atlas)", tc.X, tc.Y, group)
			continue
		}
		indices = append(indices, idx)
	}
	for i := 1; i < len(indices); i++ {
		if indices[i] != indices[0] {
			t.Errorf("bridge tiles bake to different atlas indices: %v", indices)
		}
	}
}
