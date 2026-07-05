package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// User-reported repro: party A finishes the wolf cull (the bridge planks
// appear on the forest lake), the player exits to the entry menu, builds a
// NEW party and starts a fresh game — the bridge must be GONE (the new run
// hasn't earned it). Exercises the real reset path: startNewGameWithParty →
// quests.Reset + WorldManager.Reset.
func TestNewGame_RevertsQuestBridge(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, qm := loadRealWorldForTest(t, cfg, "forest")
	quests.GlobalQuestManager = qm

	forest := wm.LoadedMaps["forest"]
	g := newTestGame(cfg, forest)
	g.questManager = qm

	def := qm.Definitions()["forest_wolf_cull"]
	if def == nil || len(def.OnCompleteTiles) == 0 {
		t.Fatal("forest_wolf_cull on_complete_tiles missing")
	}
	bridgeType, ok := world.GlobalTileManager.GetTileTypeFromKey(def.OnCompleteTiles[0].Tile)
	if !ok {
		t.Fatal("quest_bridge tile key unknown")
	}

	// Party A: complete the cull, the planks appear.
	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("forest_wolf_cull")
	g.applyCompletedQuestTiles()
	for _, tc := range def.OnCompleteTiles {
		if forest.Tiles[tc.Y][tc.X] != bridgeType {
			t.Fatalf("setup: bridge tile (%d,%d) not applied", tc.X, tc.Y)
		}
	}

	// Party B: fresh game from the entry menu.
	g.startNewGameWithParty(character.NewParty(cfg))

	freshForest := wm.LoadedMaps["forest"]
	if freshForest == nil {
		t.Fatal("forest missing after reset")
	}
	for _, tc := range def.OnCompleteTiles {
		got := freshForest.Tiles[tc.Y][tc.X]
		key := world.GlobalTileManager.GetTileKey(got)
		if got == bridgeType {
			t.Errorf("new game kept the quest bridge at (%d,%d) — world state leaked across runs", tc.X, tc.Y)
		} else {
			t.Logf("tile (%d,%d) after new game = %s (reverted ok)", tc.X, tc.Y, key)
		}
	}
	// And the quest itself must be back to not-completed for the fresh run.
	for _, q := range qm.GetAllQuests() {
		if q.ID == "forest_wolf_cull" && q.Completed {
			t.Error("quest completion leaked across new game")
		}
	}

	// The wolves respawn with the map reset — party B taking the cull must see
	// a full census, NOT the instant "already done" credit (which would lay the
	// bridge at accept time on a wolf-less map).
	g.world = freshForest
	living := g.countLivingQuestTargets("wolf", "forest")
	if living == 0 {
		t.Fatal("no living wolves after new game — accept would instantly credit the cull")
	}
	t.Logf("wolves after new game: %d", living)
	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("re-activate: %v", err)
	}
	quests.GlobalQuestManager.SetDynamicTarget("forest_wolf_cull", living)
	if g.creditQuestIfCleared("forest_wolf_cull") {
		t.Error("fresh run wrongly credited the cull as already done")
	}
	if q := qm.GetQuest("forest_wolf_cull"); q == nil || q.CurrentCount != 0 || q.Completed {
		t.Errorf("fresh quest state dirty: %+v", q)
	}
}

// Loading a save where the quest is NOT completed must take the bridge back
// out: SwitchToMap does NOT reload maps from disk, so the shared forest
// instance still carries the planks laid earlier this session — syncQuestTiles
// has to actively revert them (the forward-only apply left them forever).
func TestLoadOlderSave_RevertsQuestBridge(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, qm := loadRealWorldForTest(t, cfg, "forest")
	quests.GlobalQuestManager = qm

	forest := wm.LoadedMaps["forest"]
	g := newTestGame(cfg, forest)
	g.questManager = qm

	def := qm.Definitions()["forest_wolf_cull"]
	bridgeType, _ := world.GlobalTileManager.GetTileTypeFromKey(def.OnCompleteTiles[0].Tile)

	// Session: quest completed, bridge laid on the shared forest instance.
	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("forest_wolf_cull")
	g.syncQuestTiles()
	for _, tc := range def.OnCompleteTiles {
		if forest.Tiles[tc.Y][tc.X] != bridgeType {
			t.Fatalf("setup: bridge not laid at (%d,%d)", tc.X, tc.Y)
		}
	}

	// Load an OLDER save: quest state resets to baseline (not completed) and
	// applySave calls syncQuestTiles — maps are NOT reloaded on this path.
	qm.Reset()
	g.syncQuestTiles()

	for _, tc := range def.OnCompleteTiles {
		got := forest.Tiles[tc.Y][tc.X]
		if got == bridgeType {
			t.Errorf("bridge at (%d,%d) survived loading a save where the quest isn't done", tc.X, tc.Y)
		} else if key := world.GlobalTileManager.GetTileKey(got); key != "water" && key != "deep_water" {
			t.Errorf("tile (%d,%d) reverted to %q, want the pristine water", tc.X, tc.Y, key)
		}
	}

	// And completing it again re-lays the planks (sync is truly two-way).
	if err := qm.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("re-activate: %v", err)
	}
	qm.MarkCompleted("forest_wolf_cull")
	g.syncQuestTiles()
	for _, tc := range def.OnCompleteTiles {
		if forest.Tiles[tc.Y][tc.X] != bridgeType {
			t.Errorf("bridge not re-laid at (%d,%d) after completing again", tc.X, tc.Y)
		}
	}
}
