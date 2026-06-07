package game

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
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
