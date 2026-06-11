package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	// Wire the items↔config bridges so party setup can create weapons/items.
	// Without this, newTestGame-based tests only worked when another test
	// happened to run first and set these global accessors.
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	return cfg
}

func newTestWorld(cfg *config.Config) *world.World3D {
	w := world.NewWorld3D(cfg)
	w.Width = 2
	w.Height = 2
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	return w
}

func newTestGame(cfg *config.Config, w *world.World3D) *MMGame {
	game := &MMGame{
		config:           cfg,
		world:            w,
		party:            character.NewParty(cfg),
		camera:           &FirstPersonCamera{X: 64, Y: 64, Angle: 1.25},
		skyImg:           ebiten.NewImage(2, 2),
		groundImg:        ebiten.NewImage(2, 2),
		collisionSystem:  collision.NewCollisionSystem(w, float64(cfg.World.TileSize)),
		sessionStartTime: time.Now(),
	}
	game.collisionSystem.RegisterEntity(collision.NewEntity("player", game.camera.X, game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))
	return game
}

func TestSaveLoad_PersistsTurnBasedAndBuffs(t *testing.T) {
	cfg := loadTestConfig(t)

	wmSave := world.NewWorldManager(cfg)
	worldSave := newTestWorld(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{
		"forest": worldSave,
		"desert": newTestWorld(cfg),
	}
	wmSave.CurrentMapKey = "forest"

	game := newTestGame(cfg, worldSave)
	game.turnBasedMode = true
	game.currentTurn = 1
	game.partyActionsUsed = 2
	game.turnBasedMoveCooldown = 7
	game.turnBasedRotCooldown = 9
	game.monsterTurnResolved = true
	game.turnBasedSpRegenCount = 4

	game.torchLightActive = true
	game.torchLightDuration = 120
	game.torchLightRadius = 4.0
	game.wizardEyeActive = true
	game.wizardEyeDuration = 45
	game.walkOnWaterActive = true
	game.walkOnWaterDuration = 33
	game.blessActive = true
	game.blessDuration = 60
	game.blessStatBonus = 2
	game.waterBreathingActive = true
	game.waterBreathingDuration = 25
	game.underwaterReturnX = 96
	game.underwaterReturnY = 128
	game.underwaterReturnMap = "forest"
	game.statBonus = 2

	save := game.buildSave(wmSave)

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newTestWorld(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{
		"forest": worldLoad,
		"desert": newTestWorld(cfg),
	}
	wmLoad.CurrentMapKey = "forest"

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = oldWorldManager }()

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	if loaded.currentTurn != game.currentTurn {
		t.Fatalf("currentTurn: got %d want %d", loaded.currentTurn, game.currentTurn)
	}
	if loaded.partyActionsUsed != game.partyActionsUsed {
		t.Fatalf("partyActionsUsed: got %d want %d", loaded.partyActionsUsed, game.partyActionsUsed)
	}
	if loaded.turnBasedMoveCooldown != game.turnBasedMoveCooldown {
		t.Fatalf("turnBasedMoveCooldown: got %d want %d", loaded.turnBasedMoveCooldown, game.turnBasedMoveCooldown)
	}
	if loaded.turnBasedRotCooldown != game.turnBasedRotCooldown {
		t.Fatalf("turnBasedRotCooldown: got %d want %d", loaded.turnBasedRotCooldown, game.turnBasedRotCooldown)
	}
	if loaded.monsterTurnResolved != game.monsterTurnResolved {
		t.Fatalf("monsterTurnResolved: got %v want %v", loaded.monsterTurnResolved, game.monsterTurnResolved)
	}
	if loaded.turnBasedSpRegenCount != game.turnBasedSpRegenCount {
		t.Fatalf("turnBasedSpRegenCount: got %d want %d", loaded.turnBasedSpRegenCount, game.turnBasedSpRegenCount)
	}

	if loaded.torchLightActive != game.torchLightActive || loaded.torchLightDuration != game.torchLightDuration {
		t.Fatalf("torchLight: got %v/%d want %v/%d", loaded.torchLightActive, loaded.torchLightDuration, game.torchLightActive, game.torchLightDuration)
	}
	// The radius deliberately does NOT round-trip: on load an active torch
	// adopts the current balance constant, so old saves pick up retunes.
	if loaded.torchLightRadius != TorchLightRadiusTiles {
		t.Fatalf("torchLightRadius: got %v want balance constant %v", loaded.torchLightRadius, TorchLightRadiusTiles)
	}
	if loaded.wizardEyeActive != game.wizardEyeActive || loaded.wizardEyeDuration != game.wizardEyeDuration {
		t.Fatalf("wizardEye: got %v/%d want %v/%d", loaded.wizardEyeActive, loaded.wizardEyeDuration, game.wizardEyeActive, game.wizardEyeDuration)
	}
	if loaded.walkOnWaterActive != game.walkOnWaterActive || loaded.walkOnWaterDuration != game.walkOnWaterDuration {
		t.Fatalf("walkOnWater: got %v/%d want %v/%d", loaded.walkOnWaterActive, loaded.walkOnWaterDuration, game.walkOnWaterActive, game.walkOnWaterDuration)
	}
	if loaded.blessActive != game.blessActive || loaded.blessDuration != game.blessDuration || loaded.blessStatBonus != game.blessStatBonus {
		t.Fatalf("bless: got %v/%d/%d want %v/%d/%d", loaded.blessActive, loaded.blessDuration, loaded.blessStatBonus, game.blessActive, game.blessDuration, game.blessStatBonus)
	}
	if loaded.waterBreathingActive != game.waterBreathingActive || loaded.waterBreathingDuration != game.waterBreathingDuration {
		t.Fatalf("waterBreathing: got %v/%d want %v/%d", loaded.waterBreathingActive, loaded.waterBreathingDuration, game.waterBreathingActive, game.waterBreathingDuration)
	}
	if loaded.underwaterReturnX != game.underwaterReturnX || loaded.underwaterReturnY != game.underwaterReturnY || loaded.underwaterReturnMap != game.underwaterReturnMap {
		t.Fatalf("underwaterReturn: got %.0f/%.0f/%s want %.0f/%.0f/%s", loaded.underwaterReturnX, loaded.underwaterReturnY, loaded.underwaterReturnMap, game.underwaterReturnX, game.underwaterReturnY, game.underwaterReturnMap)
	}
	if loaded.statBonus != game.statBonus {
		t.Fatalf("statBonus: got %d want %d", loaded.statBonus, game.statBonus)
	}

	if loaded.utilitySpellStatuses == nil {
		t.Fatalf("utilitySpellStatuses not initialized")
	}
	if status, ok := loaded.utilitySpellStatuses[spells.SpellID("torch_light")]; !ok || status.Duration != loaded.torchLightDuration {
		t.Fatalf("utility torch_light missing or duration mismatch")
	}
	if status, ok := loaded.utilitySpellStatuses[spells.SpellID("bless")]; !ok || status.Duration != loaded.blessDuration {
		t.Fatalf("utility bless missing or duration mismatch")
	}
}

func TestApplySaveMigratesSkillLevelToMastery(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	worldTest := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": worldTest}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, worldTest)
	save := &GameSave{
		MapKey: "forest",
		Party: PartySave{
			Members: []CharacterSave{
				{
					Name:           "Migrated",
					Class:          int(character.ClassSorcerer),
					Level:          1,
					HitPoints:      10,
					MaxHitPoints:   10,
					SpellPoints:    10,
					MaxSpellPoints: 10,
					Skills: []SkillEntry{
						{Type: int(character.SkillSword), Level: 3, Mastery: int(character.MasteryNovice)},
					},
					MagicSchools: []MagicSchoolEntry{
						{School: string(character.MagicSchoolWater), Level: 3, Mastery: int(character.MasteryNovice)},
					},
				},
			},
		},
	}

	if err := game.applySave(wm, save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	member := game.party.Members[0]
	if got := member.Skills[character.SkillSword].Mastery; got != character.MasteryMaster {
		t.Fatalf("expected skill mastery migrated to master, got %s", got)
	}
	if got := member.MagicSchools[character.MagicSchoolWater].Mastery; got != character.MasteryMaster {
		t.Fatalf("expected magic mastery migrated to master, got %s", got)
	}
}

func TestSaveLoad_PreservesEncounterRewards(t *testing.T) {
	cfg := loadTestConfig(t)

	wmSave := world.NewWorldManager(cfg)
	worldSave := newTestWorld(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{
		"forest": worldSave,
	}
	wmSave.CurrentMapKey = "forest"

	rewardsA := &monster.EncounterRewards{
		Gold:              10,
		Experience:        5,
		CompletionMessage: "Encounter A",
		QuestID:           "encounter_a",
	}
	rewardsB := &monster.EncounterRewards{
		Gold:              7,
		Experience:        3,
		CompletionMessage: "Encounter B",
		QuestID:           "encounter_b",
	}

	monA := monster.NewMonster3DFromConfig(64, 64, "bandit", cfg)
	monA.IsEncounterMonster = true
	monA.EncounterRewards = rewardsA

	monB := monster.NewMonster3DFromConfig(128, 64, "bandit", cfg)
	monB.IsEncounterMonster = true
	monB.EncounterRewards = rewardsA

	monC := monster.NewMonster3DFromConfig(64, 128, "bandit", cfg)
	monC.IsEncounterMonster = true
	monC.EncounterRewards = rewardsB

	monD := monster.NewMonster3DFromConfig(128, 128, "bandit", cfg)

	worldSave.Monsters = []*monster.Monster3D{monA, monB, monC, monD}

	game := newTestGame(cfg, worldSave)
	save := game.buildSave(wmSave)

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newTestWorld(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{
		"forest": worldLoad,
	}
	wmLoad.CurrentMapKey = "forest"

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = oldWorldManager }()

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	loadedMonsters := wmLoad.LoadedMaps["forest"].Monsters
	if len(loadedMonsters) != 4 {
		t.Fatalf("loaded monsters: got %d want 4", len(loadedMonsters))
	}

	var encA []*monster.Monster3D
	var encB *monster.Monster3D
	for _, m := range loadedMonsters {
		if !m.IsEncounterMonster || m.EncounterRewards == nil {
			continue
		}
		switch m.EncounterRewards.QuestID {
		case "encounter_a":
			encA = append(encA, m)
		case "encounter_b":
			encB = m
		}
	}

	if len(encA) != 2 {
		t.Fatalf("encounter_a monsters: got %d want 2", len(encA))
	}
	if encB == nil {
		t.Fatalf("encounter_b monster missing")
	}
	if encA[0].EncounterRewards != encA[1].EncounterRewards {
		t.Fatalf("encounter_a rewards not shared")
	}
	if encA[0].EncounterRewards == encB.EncounterRewards {
		t.Fatalf("encounter rewards should be distinct between encounters")
	}
	if encA[0].EncounterRewards.Gold != rewardsA.Gold || encA[0].EncounterRewards.Experience != rewardsA.Experience {
		t.Fatalf("encounter_a rewards mismatch")
	}
	if encB.EncounterRewards.Gold != rewardsB.Gold || encB.EncounterRewards.Experience != rewardsB.Experience {
		t.Fatalf("encounter_b rewards mismatch")
	}
}

func TestSaveLoad_PersistsLootBags(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	worldTest := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": worldTest}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, worldTest)
	game.groundContainers = []GroundContainer{
		{Kind: ContainerKindLootBag, X: 100, Y: 200, Gold: 42, SizeMultiplier: 0.5, Items: []items.Item{{Name: "Iron Sword", Type: items.ItemWeapon}}},
		{Kind: ContainerKindLootBag, X: 320, Y: 256, Gold: 0, SizeMultiplier: 0.33, Items: []items.Item{{Name: "Leather Armor", Type: items.ItemArmor}}},
	}

	save := game.buildSave(wm)

	loaded := newTestGame(cfg, worldTest)
	loaded.groundContainers = []GroundContainer{{Kind: ContainerKindLootBag, X: 999, Y: 999, Gold: 1}} // should be wiped
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	if len(loaded.groundContainers) != 2 {
		t.Fatalf("expected 2 ground containers after load, got %d", len(loaded.groundContainers))
	}
	if loaded.groundContainers[0].Gold != 42 || loaded.groundContainers[0].X != 100 || loaded.groundContainers[0].Y != 200 {
		t.Fatalf("first container mismatch: %+v", loaded.groundContainers[0])
	}
	if loaded.groundContainers[0].SizeMultiplier != 0.5 {
		t.Fatalf("size multiplier not preserved: %v", loaded.groundContainers[0].SizeMultiplier)
	}
	if len(loaded.groundContainers[0].Items) != 1 || loaded.groundContainers[0].Items[0].Name != "Iron Sword" {
		t.Fatalf("first container items mismatch: %+v", loaded.groundContainers[0].Items)
	}
	if loaded.groundContainers[1].Items[0].Name != "Leather Armor" {
		t.Fatalf("second container items mismatch: %+v", loaded.groundContainers[1].Items)
	}
}

func TestSaveLoad_PersistsPoisonTimer(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	worldTest := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": worldTest}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, worldTest)
	if len(game.party.Members) == 0 {
		t.Fatalf("party expected to have members")
	}
	game.party.Members[0].PoisonFramesRemaining = 1500

	save := game.buildSave(wm)

	loaded := newTestGame(cfg, worldTest)
	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	if got := loaded.party.Members[0].PoisonFramesRemaining; got != 1500 {
		t.Fatalf("expected poison timer 1500 after load, got %d", got)
	}
}

// Old saves (written before charm / day-of-the-gods / hour-of-power / map-return
// fields existed) must still decode cleanly, with the new state defaulting to
// "off". Guards backward compatibility of the additive save-format changes.
func TestSaveLoad_OldSaveDecodesWithDefaults(t *testing.T) {
	// A minimal pre-change save: only fields that existed before this session.
	oldJSON := `{
		"map_key":"forest","player_x":100,"player_y":200,"turn_based":true,
		"bless_active":true,"bless_duration":300,"bless_stat_bonus":10,
		"monsters":[{"key":"skeleton","name":"Skeleton","x":64,"y":64,"hit_points":30}]
	}`
	var s GameSave
	if err := json.Unmarshal([]byte(oldJSON), &s); err != nil {
		t.Fatalf("old-format save failed to decode: %v", err)
	}
	// Pre-existing fields survive.
	if s.MapKey != "forest" || !s.TurnBased || !s.BlessActive || s.BlessStatBonus != 10 {
		t.Errorf("pre-existing fields not decoded: %+v", s)
	}
	// New fields default to off / empty (no crash, no spurious buffs).
	if len(s.CombatBuffs) != 0 {
		t.Errorf("new combat-buff list should default empty, got %+v", s.CombatBuffs)
	}
	if s.MapReturnPoses != nil {
		t.Errorf("absent map_return_poses should decode to nil (load makes it empty)")
	}
	if len(s.Monsters) != 1 || s.Monsters[0].Bound || s.Monsters[0].BoundFramesRemaining != 0 {
		t.Errorf("old monster should decode unbound, got %+v", s.Monsters)
	}
}

// A spent hide_when_visited statue must vanish from interaction yet stay in the
// world, so its Visited=true is captured by the save (NPC states only persist
// NPCs still present). Dropping it from the world — the old RemoveNPC behaviour —
// lost the spent state and resurrected the statue unspent on reload.
func TestSpentStatueHiddenButKeptInWorld(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	statue := &character.NPC{Name: "Black Dragon Statue", X: 80, Y: 64, Sprite: "dragon_statue", HideWhenVisited: true}
	w.NPCs = append(w.NPCs, statue)
	game := newTestGame(cfg, w)

	if game.GetNearestInteractableNPC() != statue {
		t.Fatalf("unspent statue should be interactable")
	}

	statue.Visited = true // mirrors summonDragonFromStatue: mark spent, keep in world

	if game.GetNearestInteractableNPC() != nil {
		t.Errorf("spent hide_when_visited statue must not be interactable")
	}
	kept := false
	for _, n := range w.NPCs {
		if n == statue {
			kept = true
		}
	}
	if !kept {
		t.Errorf("spent statue must stay in the world so its Visited state reaches the save")
	}
}

// Overlapping monsters must be pushed apart by the separation pass (engaged
// pairs that overlap veto each other's every normal move and would otherwise
// stay glued forever).
func TestSeparateOverlappingMonsters(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	gl := &GameLoop{game: g}

	// Park the player away from the pair — pushes refuse to land on the player.
	g.camera.X, g.camera.Y = 8, 8
	g.collisionSystem.UpdateEntity("player", 8, 8)
	a := monster.NewMonster3DFromConfig(64, 64, "goblin", cfg)
	b := monster.NewMonster3DFromConfig(66, 64, "goblin", cfg) // almost fully stacked
	a.IsEngagingPlayer = true                                  // calm pairs pass through by design; engaged ones glue
	w.Monsters = []*monster.Monster3D{a, b}
	g.registerSpawnedMonster(a)
	g.registerSpawnedMonster(b)

	aw, _ := a.GetSize()
	bw, _ := b.GetSize()
	need := (aw + bw) / 2
	for i := 0; i < 240; i++ {
		gl.separateOverlappingMonsters()
		if math.Abs(b.X-a.X) >= need || math.Abs(b.Y-a.Y) >= need {
			return // separated
		}
	}
	t.Fatalf("monsters still overlapping after separation pass: a=(%.0f,%.0f) b=(%.0f,%.0f) need %.0f",
		a.X, a.Y, b.X, b.Y, need)
}

// In a one-wide corridor (trees above and below) the least-penetration push is
// blocked on both sides — the pair must fall back to separating ALONG the
// corridor instead of staying glued (the goblins-stuck-between-trees bug).
func TestSeparateOverlappingMonsters_InCorridor(t *testing.T) {
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 7, 3
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			if y == 1 {
				w.Tiles[y][x] = world.TileEmpty // the corridor
			} else {
				w.Tiles[y][x] = world.TileTree
			}
		}
	}
	g := newTestGame(cfg, w)
	gl := &GameLoop{game: g}

	// Stacked mid-corridor, offset slightly along Y so the LEAST penetration
	// axis is the blocked cross-corridor one.
	a := monster.NewMonster3DFromConfig(64*3+32, 96, "goblin", cfg)
	b := monster.NewMonster3DFromConfig(64*3+34, 90, "goblin", cfg)
	a.IsEngagingPlayer = true
	b.IsEngagingPlayer = true
	w.Monsters = []*monster.Monster3D{a, b}
	g.registerSpawnedMonster(a)
	g.registerSpawnedMonster(b)

	aw, _ := a.GetSize()
	bw, _ := b.GetSize()
	need := (aw + bw) / 2
	for i := 0; i < 300; i++ {
		gl.separateOverlappingMonsters()
		if math.Abs(b.X-a.X) >= need || math.Abs(b.Y-a.Y) >= need {
			return // separated along the corridor
		}
	}
	t.Fatalf("corridor pair still glued: a=(%.0f,%.0f) b=(%.0f,%.0f) need %.0f",
		a.X, a.Y, b.X, b.Y, need)
}

// creditClearedKillQuests completes a region kill quest when its target_map is
// clear, even if the same monster type still lives on another map — so culling
// the cliff trolls turns the quest in despite trolls roaming the highlands, and
// a target slain before the quest was taken can still be credited.
func TestCreditClearedKillQuests_RegionScoped(t *testing.T) {
	cfg := loadTestConfig(t)
	qcfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}

	// NPC linked to the cliff troll cull (target_map: dragon_cliffs).
	npc := &character.NPC{DialogueData: &character.NPCDialogue{
		Choices: []*character.NPCDialogueChoice{
			{Action: "turn_in_quest", QuestID: "dragon_cliffs_troll_cull"},
		},
	}}

	setup := func(trollMap string) *MMGame {
		cliffs := newTestWorld(cfg)
		highlands := newTestWorld(cfg)
		troll := monster.NewMonster3DFromConfig(64, 64, "mountain_troll", cfg)
		switch trollMap {
		case "dragon_cliffs":
			cliffs.Monsters = append(cliffs.Monsters, troll)
		case "highlands":
			highlands.Monsters = append(highlands.Monsters, troll)
		}
		wm := world.NewWorldManager(cfg)
		wm.LoadedMaps = map[string]*world.World3D{"dragon_cliffs": cliffs, "highlands": highlands}
		wm.CurrentMapKey = "dragon_cliffs"
		world.GlobalWorldManager = wm

		g := newTestGame(cfg, cliffs)
		g.questManager = quests.NewQuestManager(qcfg)
		if err := g.questManager.ActivateQuest("dragon_cliffs_troll_cull"); err != nil {
			t.Fatalf("activate: %v", err)
		}
		return g
	}

	old := world.GlobalWorldManager
	defer func() { world.GlobalWorldManager = old }()

	// Trolls only in the highlands: the cliff region is clear → quest completes.
	g := setup("highlands")
	g.creditClearedKillQuests(npc)
	if q := g.questManager.GetQuest("dragon_cliffs_troll_cull"); q == nil || !q.Completed {
		t.Errorf("quest should complete when its target_map is clear despite trolls elsewhere; got %+v", q)
	}

	// A living troll in the cliffs themselves must still block completion.
	g = setup("dragon_cliffs")
	g.creditClearedKillQuests(npc)
	if q := g.questManager.GetQuest("dragon_cliffs_troll_cull"); q == nil || q.Completed {
		t.Errorf("quest must NOT complete while a troll lives in its target_map; got %+v", q)
	}
}

// Hostility must survive save/load: a provoked monster (WasAttacked) stays
// provoked, and an old save's quest-bearing encounter monster (no was_attacked
// field — e.g. a lair dragon) is migrated to hostile. A chest-bound encounter
// mob without a QuestID keeps normal aggro.
func TestSaveLoad_RestoresMonsterHostility(t *testing.T) {
	cfg := loadTestConfig(t)

	wmSave := world.NewWorldManager(cfg)
	worldSave := newTestWorld(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": worldSave}
	wmSave.CurrentMapKey = "forest"
	game := newTestGame(cfg, worldSave)

	provoked := monster.NewMonster3DFromConfig(64, 64, "bandit", cfg)
	provoked.WasAttacked = true
	calm := monster.NewMonster3DFromConfig(128, 64, "bandit", cfg)
	chestBound := monster.NewMonster3DFromConfig(64, 128, "bandit", cfg)
	chestBound.IsEncounterMonster = true
	chestBound.EncounterRewards = &monster.EncounterRewards{Gold: 10} // clear-encounter style: no QuestID
	worldSave.Monsters = []*monster.Monster3D{provoked, calm, chestBound}

	save := game.buildSave(wmSave)

	// Old-save migration case: a lair dragon saved before was_attacked existed.
	save.MapMonsters["forest"] = append(save.MapMonsters["forest"], MonsterSave{
		Key: "dragon_green", Name: "Dragon", X: 192, Y: 192, HitPoints: 1100,
		IsEncounterMonster: true,
		EncounterRewards:   &EncounterRewardSave{QuestID: "dragon_cliffs_bone_lair"},
	})

	wmLoad := world.NewWorldManager(cfg)
	worldLoad := newTestWorld(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": worldLoad}
	wmLoad.CurrentMapKey = "forest"
	oldWM := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = oldWM }()

	loaded := newTestGame(cfg, worldLoad)
	if err := loaded.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	byPos := map[[2]int]*monster.Monster3D{}
	for _, m := range worldLoad.Monsters {
		byPos[[2]int{int(m.X), int(m.Y)}] = m
	}
	if m := byPos[[2]int{64, 64}]; m == nil || !m.WasAttacked || !m.IsEngagingPlayer {
		t.Errorf("provoked monster must stay hostile after load")
	}
	if m := byPos[[2]int{128, 64}]; m == nil || m.WasAttacked || m.IsEngagingPlayer {
		t.Errorf("calm monster must stay calm after load")
	}
	if m := byPos[[2]int{64, 128}]; m == nil || m.WasAttacked || m.IsEngagingPlayer {
		t.Errorf("chest-bound encounter mob (no QuestID) must keep normal aggro")
	}
	if m := byPos[[2]int{192, 192}]; m == nil || !m.WasAttacked || !m.IsEngagingPlayer {
		t.Errorf("old-save lair dragon (QuestID, no was_attacked) must migrate to hostile")
	}
}
