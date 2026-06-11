package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// loadRealQuestTileData loads the real tiles.yaml + quests.yaml so the test
// validates the shipped data, and wires a small fake forest world.
func loadRealQuestTileData(t *testing.T) (*MMGame, *world.World3D) {
	t.Helper()
	cfg := loadTestConfig(t)

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	t.Cleanup(func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	})

	tm := world.NewTileManager()
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	world.GlobalTileManager = tm

	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	qm := quests.NewQuestManager(questCfg)

	// A 30x30 forest stand-in large enough to hold the bridge coordinates.
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 30, 30
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
	}
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm

	g := newTestGame(cfg, w)
	g.questManager = qm
	return g, w
}

// The shipped quests.yaml tile changes must reference real tile keys.
func TestQuestTileChanges_ShippedDataValid(t *testing.T) {
	g, _ := loadRealQuestTileData(t)
	if err := validateQuestTileChanges(g.questManager); err != nil {
		t.Fatalf("shipped quest tile data invalid: %v", err)
	}
}

// Taking the wolf cull AFTER the wolves are already dead credits it on the
// spot — the journal must never show "0/18" on a finished job.
func TestWolfCull_TakenAfterWipeCompletesImmediately(t *testing.T) {
	g, _ := loadRealQuestTileData(t) // no wolves placed: the map is already "cleared"

	prevQM := quests.GlobalQuestManager
	quests.GlobalQuestManager = g.questManager
	t.Cleanup(func() { quests.GlobalQuestManager = prevQM })
	ih := &InputHandler{game: g}
	ih.handleGiveQuest("forest_wolf_cull")

	q := g.questManager.GetQuest("forest_wolf_cull")
	if q == nil {
		t.Fatal("quest not activated")
	}
	if !q.Completed {
		t.Fatal("quest taken after the wipe must complete immediately")
	}
	if q.CurrentCount != q.Definition.TargetCount {
		t.Errorf("journal shows %d/%d, want full count", q.CurrentCount, q.Definition.TargetCount)
	}
	tc := q.Definition.OnCompleteTiles[0]
	bridgeType, _ := world.GlobalTileManager.GetTileTypeFromKey(tc.Tile)
	if g.worldByKey(tc.Map).Tiles[tc.Y][tc.X] != bridgeType {
		t.Error("bridge should be laid the moment the cleared quest is credited")
	}
}

// The wolf cull completes the moment the last forest wolf dies (extermination
// semantics, not the kill counter) and lays the bridge tiles.
func TestWolfCull_ExterminationLaysBridge(t *testing.T) {
	g, w := loadRealQuestTileData(t)

	if err := g.questManager.ActivateQuest("forest_wolf_cull"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	wolf := &monster.Monster3D{Name: "Wolf", HitPoints: 10, MaxHitPoints: 10}
	w.Monsters = append(w.Monsters, wolf)

	tileChanges := g.questManager.Definitions()["forest_wolf_cull"].OnCompleteTiles
	if len(tileChanges) == 0 {
		t.Fatal("forest_wolf_cull has no on_complete_tiles")
	}
	bridgeType, ok := world.GlobalTileManager.GetTileTypeFromKey(tileChanges[0].Tile)
	if !ok {
		t.Fatalf("quest bridge tile key %q missing", tileChanges[0].Tile)
	}

	// Wolf alive → no completion, no bridge.
	g.completeExterminationQuests("wolf")
	g.applyCompletedQuestTiles()
	if g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest completed while a wolf lives")
	}
	if w.Tiles[24][22] == bridgeType {
		t.Fatal("bridge laid while a wolf lives")
	}

	// Last wolf dies → quest completes and the bridge appears.
	wolf.HitPoints = 0
	g.completeExterminationQuests("wolf")
	g.applyCompletedQuestTiles()
	if !g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest should complete once the map is cleared")
	}
	if w.Tiles[24][22] != bridgeType || w.Tiles[24][23] != bridgeType {
		t.Errorf("bridge tiles not laid: (22,24)=%v (23,24)=%v want %v",
			w.Tiles[24][22], w.Tiles[24][23], bridgeType)
	}
}
