package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
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

	tm := world.NewTileManager(testTileSizeClasses())
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
	// The same stand-in doubles for every map the shipped quest data targets -
	// validation only needs the key to resolve.
	wm.LoadedMaps = map[string]*world.World3D{
		"forest": w, "dragon_cliffs": w, "pyramid_3": w, "water": w,
	}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm

	g := newTestGame(cfg, w)
	g.questManager = qm
	return g, w
}

// The shipped quests.yaml tile changes must reference real tile keys.
func TestQuestTileChanges_ShippedDataValid(t *testing.T) {
	g, _ := loadRealQuestTileData(t)
	if err := validateQuestWorldReferences(g.questManager); err != nil {
		t.Fatalf("shipped quest tile data invalid: %v", err)
	}
}

func TestQuestWorldReferencesRejectUnknownSummonQuest(t *testing.T) {
	loadTestConfig(t)
	previous := character.NPCConfigInstance
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"test_statue": {
			Summons: []*character.NPCSummon{{
				Statuette: "Black Dragon Statuette",
				Monster:   "elder_dragon",
				QuestID:   "missing_quest",
			}},
		},
	}}
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{}})
	err := validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), `unknown quest "missing_quest"`) {
		t.Fatalf("validator error = %v, want unknown summon quest", err)
	}
}

func TestQuestWorldReferencesRejectInvalidDialogueQuestLinks(t *testing.T) {
	loadTestConfig(t)
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	tests := []struct {
		name   string
		choice *character.NPCDialogueChoice
		want   string
	}{
		{
			name:   "empty give quest ID",
			choice: &character.NPCDialogueChoice{Action: "give_quest"},
			want:   `action "give_quest" has empty quest_id`,
		},
		{
			name:   "unknown nested requirement",
			choice: &character.NPCDialogueChoice{Action: "info", RequiresQuest: "missing_quest"},
			want:   `unknown requires_quest "missing_quest"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
				"test_giver": {
					Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{
						Action: "info",
						Choices: []*character.NPCDialogueChoice{
							tt.choice,
						},
					}}},
				},
			}}
			qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
				"known_quest": {Name: "Known"},
			}})
			err := validateQuestWorldReferences(qm)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validator error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestQuestWorldReferencesRejectInvalidRewardPoolItem(t *testing.T) {
	loadTestConfig(t)
	previous := character.NPCConfigInstance
	character.NPCConfigInstance = nil
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"bad_reward": {
			Name: "Bad Reward",
			Rewards: quests.QuestRewards{
				ItemPool: []string{"missing_item"},
			},
		},
	}})
	err := validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), `rewards.item_pool[0]`) ||
		!strings.Contains(err.Error(), `missing_item`) {
		t.Fatalf("validator error = %v, want invalid reward pool item", err)
	}
}

func TestQuestRewardItemFailureLeavesClaimRetryable(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"bad_reward": {
			Name: "Bad Reward",
			Rewards: quests.QuestRewards{
				ItemPool: []string{"missing_item"},
			},
		},
	}})
	if err := qm.ActivateQuest("bad_reward"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("bad_reward")
	g.questManager = qm

	if g.claimQuestReward("bad_reward") {
		t.Fatal("claim with an invalid item key succeeded")
	}
	if quest := qm.GetQuest("bad_reward"); quest == nil || quest.RewardsClaimed {
		t.Fatal("failed item creation permanently consumed the quest reward")
	}
	if countCombatLog(g, "Cannot claim reward:") != 1 {
		t.Fatal("failed item creation was not reported to the player")
	}
}

// Taking the wolf cull AFTER the wolves are already dead credits it on the
// spot - the journal must never show "0/21" on a finished job.
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
	if got := countCombatLog(g, "completed!"); got != 1 {
		t.Fatalf("completion announcements = %d, want exactly 1", got)
	}
	if got := countCombatLog(g, "already done"); got != 0 {
		t.Fatalf("legacy completion announcements = %d, want 0", got)
	}
}

func TestWolfCull_ProgressIgnoresRuntimeSummonedWolves(t *testing.T) {
	g, w := loadRealQuestTileData(t)

	prevQM := quests.GlobalQuestManager
	quests.GlobalQuestManager = g.questManager
	t.Cleanup(func() { quests.GlobalQuestManager = prevQM })

	makeWolf := func(ignore bool) *monster.Monster3D {
		return &monster.Monster3D{
			Name:                 "Wolf",
			HitPoints:            10,
			MaxHitPoints:         10,
			QuestProgressIgnored: ignore,
		}
	}
	first := makeWolf(false)
	second := makeWolf(false)
	extra := makeWolf(true) // e.g. a Dead Branch random summon
	w.Monsters = append(w.Monsters, first, second, extra)

	ih := &InputHandler{game: g}
	ih.handleGiveQuest("forest_wolf_cull")
	q := g.questManager.GetQuest("forest_wolf_cull")
	if q == nil {
		t.Fatal("quest not activated")
	}
	// Dynamic target = live, quest-eligible census at pickup: the two real wolves
	// (the ignored runtime summon is excluded). Progress starts at 0/2.
	if q.Target() != 2 {
		t.Fatalf("dynamic target = %d, want 2 (live census, ignored summon excluded)", q.Target())
	}
	if q.CurrentCount != 0 {
		t.Fatalf("progress after pickup = %d/%d, want 0/2", q.CurrentCount, q.Target())
	}
	if q.Completed {
		t.Fatal("two real wolves are still alive; quest must not complete")
	}

	// Killing the runtime-summoned (ignored) wolf must not advance or complete it.
	extra.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf")
	if q.CurrentCount != 0 {
		t.Fatalf("ignored wolf changed progress to %d/%d, want 0/2", q.CurrentCount, q.Target())
	}
	if q.Completed {
		t.Fatal("ignored extra wolf death must not complete while real wolves live")
	}

	first.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf")
	if q.CurrentCount != 1 {
		t.Fatalf("one real wolf left progress = %d/%d, want 1/2", q.CurrentCount, q.Target())
	}
	if q.Completed {
		t.Fatal("one real wolf still alive; quest must not complete")
	}

	second.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf")
	if !q.Completed {
		t.Fatal("quest should complete after the last real wolf dies")
	}
	if q.CurrentCount != q.Target() {
		t.Fatalf("completed progress = %d/%d, want full", q.CurrentCount, q.Target())
	}
	g.completeClearedKillQuestsForTarget("wolf")
	if got := countCombatLog(g, "completed!"); got != 1 {
		t.Fatalf("completion announcements after repeated sync = %d, want exactly 1", got)
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

	// Wolf alive -> no completion, no bridge.
	g.completeClearedKillQuestsForTarget("wolf")
	g.applyCompletedQuestTiles()
	if g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest completed while a wolf lives")
	}
	if w.Tiles[24][22] == bridgeType {
		t.Fatal("bridge laid while a wolf lives")
	}

	// Last wolf dies -> quest completes and the bridge appears.
	wolf.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf")
	g.applyCompletedQuestTiles()
	if !g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest should complete once the map is cleared")
	}
	if w.Tiles[24][22] != bridgeType || w.Tiles[24][23] != bridgeType {
		t.Errorf("bridge tiles not laid: (22,24)=%v (23,24)=%v want %v",
			w.Tiles[24][22], w.Tiles[24][23], bridgeType)
	}
}
