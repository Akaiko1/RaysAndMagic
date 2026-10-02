package game

import (
	"strings"
	"testing"
	"ugataima/internal/config"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// loadRealQuestTileData loads the real tiles.yaml + quests.yaml and wires a
// small fake forest world for the forest quests.
func loadRealQuestTileData(t *testing.T) (*MMGame, *world.World3D) {
	t.Helper()
	cfg := loadTestConfig(t)

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	t.Cleanup(func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	})

	// The prop validator checks every prop's loot table against the shipped pools,
	// so this fixture owns that catalog too - relying on whichever earlier test
	// happened to load it makes the assertion order-dependent (-shuffle=on).
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("loots: %v", err)
	}

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
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm

	g := newTestGame(cfg, w)
	g.questManager = qm
	return g, w
}

// The shipped quests.yaml tile changes must reference real tile keys.
func TestQuestTileChanges_ShippedDataValid(t *testing.T) {
	// This is shipped-content validation, so use the shipped placement census as
	// well. A synthetic map with zero NPCs is now correctly a trustworthy empty
	// world, not an implicit request to skip reachability checks.
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, true)
	if err := g.validateQuestWorldReferences(g.questManager); err != nil {
		t.Fatalf("shipped quest tile data invalid: %v", err)
	}
}

// One case table for the cross-catalog quest links the boot validator owns:
// each broken row names its own fault; the giver's own chain stays valid.
func TestQuestWorldReferencesRejectBrokenQuestLinks(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	giver := func(d *character.NPCDialogue) *character.NPCConfig {
		return &character.NPCConfig{NPCs: map[string]*character.NPCData{"test_giver": {Dialogue: d}}}
	}
	nested := func(c *character.NPCDialogueChoice) *character.NPCConfig {
		return giver(&character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{
			Action: "info", Choices: []*character.NPCDialogueChoice{c},
		}}})
	}
	known := func() map[string]*quests.QuestDefinition {
		return map[string]*quests.QuestDefinition{"known_quest": {Name: "Known"}}
	}
	chain := func() map[string]*quests.QuestDefinition {
		return map[string]*quests.QuestDefinition{"own_quest": {Name: "Own"}, "foreign_quest": {Name: "Foreign"}}
	}
	ownChain := func(extra ...*character.NPCDialogueChoice) []*character.NPCDialogueChoice {
		return append(extra, &character.NPCDialogueChoice{Text: "Take it", Action: "give_quest", QuestID: "own_quest"})
	}

	for _, tc := range []struct {
		name   string
		npcs   *character.NPCConfig
		quests map[string]*quests.QuestDefinition
		want   []string // every fragment must appear; empty = the chain rule accepts
	}{
		{
			name: "unknown summon quest",
			npcs: &character.NPCConfig{NPCs: map[string]*character.NPCData{"test_statue": {
				Summons: []*character.NPCSummon{{Statuette: "Black Dragon Statuette", Monster: "elder_dragon", QuestID: "missing_quest"}},
			}}},
			quests: map[string]*quests.QuestDefinition{},
			want:   []string{`unknown quest "missing_quest"`},
		},
		{
			name:   "empty give quest ID",
			npcs:   nested(&character.NPCDialogueChoice{Action: "give_quest"}),
			quests: known(),
			want:   []string{`action "give_quest" has empty quest_id`},
		},
		{
			name:   "unknown nested requirement",
			npcs:   nested(&character.NPCDialogueChoice{Action: "info", RequiresQuest: "missing_quest"}),
			quests: known(),
			want:   []string{`unknown requires_quest "missing_quest"`},
		},
		{
			name:   "unknown quest step",
			npcs:   nested(&character.NPCDialogueChoice{Action: "info", QuestStep: "missing_quest"}),
			quests: known(),
			want:   []string{`unknown quest_step "missing_quest"`},
		},
		{
			name:   "unknown quest_messages quest",
			npcs:   giver(&character.NPCDialogue{QuestMessages: map[string]character.NPCQuestMessages{"missing_quest": {Offer: "Missing"}}}),
			quests: map[string]*quests.QuestDefinition{},
			want:   []string{`quest_messages references unknown quest "missing_quest"`},
		},
		{
			name: "invalid reward pool item",
			quests: map[string]*quests.QuestDefinition{"bad_reward": {
				Name: "Bad Reward", Rewards: quests.QuestRewards{ItemPool: []string{"missing_item"}},
			}},
			want: []string{`rewards.item_pool[0]`, `missing_item`},
		},
		{
			// Per-step copy and choices are selected by activeChainQuestID, which
			// only returns a quest THIS giver hands out or takes in.
			name: "quest_messages for another giver's quest",
			npcs: giver(&character.NPCDialogue{
				Choices:       ownChain(),
				QuestMessages: map[string]character.NPCQuestMessages{"foreign_quest": {Offer: "Hi"}},
			}),
			quests: chain(),
			want:   []string{`quest_messages references quest "foreign_quest" that this NPC never gives or takes in`},
		},
		{
			name:   "quest_step pinned to another giver's quest",
			npcs:   giver(&character.NPCDialogue{Choices: ownChain(&character.NPCDialogueChoice{Text: "Ask", Action: "info", QuestStep: "foreign_quest"})}),
			quests: chain(),
			want:   []string{`pins quest_step "foreign_quest" that this NPC never gives or takes in`},
		},
		{
			name: "own chain links on both fields",
			npcs: giver(&character.NPCDialogue{
				Choices:       ownChain(&character.NPCDialogueChoice{Text: "Ask", Action: "info", QuestStep: "own_quest"}),
				QuestMessages: map[string]character.NPCQuestMessages{"own_quest": {Offer: "Hi"}},
			}),
			quests: chain(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			character.NPCConfigInstance = tc.npcs
			err := g.validateQuestWorldReferences(quests.NewQuestManager(&quests.QuestConfig{Quests: tc.quests}))
			if len(tc.want) == 0 {
				// Only the chain rule: the validator also walks the GLOBAL monster
				// catalog, whose own quest links are unrelated to this scene.
				if err != nil && strings.Contains(err.Error(), "never gives or takes in") {
					t.Fatalf("own-chain links rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validator accepted the fixture, want %q", tc.want)
			}
			for _, fragment := range tc.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("validator error = %v, want %q", err, fragment)
				}
			}
		})
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
	if q.CurrentCount != 0 || q.Target() != 0 {
		t.Errorf("already-cleared journal shows %d/%d, want 0/0", q.CurrentCount, q.Target())
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
	g.completeClearedKillQuestsForTarget("wolf", false)
	if q.CurrentCount != 0 {
		t.Fatalf("ignored wolf changed progress to %d/%d, want 0/2", q.CurrentCount, q.Target())
	}
	if q.Completed {
		t.Fatal("ignored extra wolf death must not complete while real wolves live")
	}

	first.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf", false)
	if q.CurrentCount != 1 {
		t.Fatalf("one real wolf left progress = %d/%d, want 1/2", q.CurrentCount, q.Target())
	}
	if q.Completed {
		t.Fatal("one real wolf still alive; quest must not complete")
	}

	second.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf", false)
	if !q.Completed {
		t.Fatal("quest should complete after the last real wolf dies")
	}
	if q.CurrentCount != q.Target() {
		t.Fatalf("completed progress = %d/%d, want full", q.CurrentCount, q.Target())
	}
	g.completeClearedKillQuestsForTarget("wolf", false)
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
	bridgeTypes := make([]world.TileType3D, len(tileChanges))
	for i, tc := range tileChanges {
		tile, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.Tile)
		if !ok {
			t.Fatalf("quest bridge tile key %q missing", tc.Tile)
		}
		bridgeTypes[i] = tile
	}
	bridgeLaid := func() int {
		laid := 0
		for i, tc := range tileChanges {
			if g.worldByKey(tc.Map).Tiles[tc.Y][tc.X] == bridgeTypes[i] {
				laid++
			}
		}
		return laid
	}

	// Wolf alive -> no completion, no bridge.
	g.completeClearedKillQuestsForTarget("wolf", false)
	g.applyCompletedQuestTiles()
	if g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest completed while a wolf lives")
	}
	if laid := bridgeLaid(); laid != 0 {
		t.Fatalf("%d bridge tiles laid while a wolf lives", laid)
	}

	// Last wolf dies -> quest completes and the bridge appears.
	wolf.HitPoints = 0
	g.completeClearedKillQuestsForTarget("wolf", false)
	g.applyCompletedQuestTiles()
	if !g.questManager.GetQuest("forest_wolf_cull").Completed {
		t.Fatal("quest should complete once the map is cleared")
	}
	if laid := bridgeLaid(); laid != len(tileChanges) {
		t.Errorf("bridge tiles laid = %d, want all %d", laid, len(tileChanges))
	}
}

// Kill progress matches a monster by its normalized display name, so every
// kill target must name at least one monster that way: a renamed monster or a
// typo fails the boot instead of silently freezing the quest.
func TestKillQuestTargetsMatchMonsterNames(t *testing.T) {
	cfg := loadTestConfig(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	prevWM := world.GlobalWorldManager
	world.GlobalWorldManager = nil // map references are another validator's job
	t.Cleanup(func() { world.GlobalWorldManager = prevWM })
	g := &MMGame{config: cfg}
	shipped, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.validateQuestWorldReferences(quests.NewQuestManager(shipped)); err != nil {
		t.Fatalf("shipped quests: %v", err)
	}
	for _, tc := range []struct {
		name    string
		def     quests.QuestDefinition
		wantErr bool
	}{
		{"display name", quests.QuestDefinition{TargetMonster: "forest spider"}, false},
		{"name shared by a group", quests.QuestDefinition{TargetMonster: "elder_dragon"}, false},
		{"monster key that is not its name", quests.QuestDefinition{TargetMonster: "rat"}, true},
		{"typo", quests.QuestDefinition{TargetMonster: "forest_spidr"}, true},
		{"one bad entry in a list", quests.QuestDefinition{TargetMonsters: []string{"wolf", "wolff"}}, true},
		{"interact tags are not monsters", quests.QuestDefinition{Type: quests.QuestTypeInteract, TargetMonster: "valve"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := tc.def
			if def.Type == "" {
				def.Type = quests.QuestTypeKill
			}
			qc := &quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"probe": &def}}
			for id, q := range shipped.Quests {
				qc.Quests[id] = q
			}
			err := g.validateQuestWorldReferences(quests.NewQuestManager(qc))
			if got := err != nil && strings.Contains(err.Error(), "matches no monster name"); got != tc.wantErr || (!tc.wantErr && err != nil) {
				t.Fatalf("validation error = %v, want the target error %v", err, tc.wantErr)
			}
		})
	}
}
