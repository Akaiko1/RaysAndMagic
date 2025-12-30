package game

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
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
	if loaded.torchLightRadius != game.torchLightRadius {
		t.Fatalf("torchLightRadius: got %v want %v", loaded.torchLightRadius, game.torchLightRadius)
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
