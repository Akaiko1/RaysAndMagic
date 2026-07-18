package game

import (
	"encoding/json"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestNormalizeWeaponFromConfigRefreshesSavedRarity(t *testing.T) {
	loadTestConfig(t)

	item := items.Item{
		Name:        "Fists",
		Type:        items.ItemWeapon,
		Description: "old saved description",
		Rarity:      "common",
		Attributes:  map[string]int{"value": 999},
		InstanceID:  42,
	}

	normalizeItemFromConfig(&item)

	if item.Rarity != "legendary" {
		t.Fatalf("saved Fists rarity = %q, want YAML legendary", item.Rarity)
	}
	if item.InstanceID != 42 {
		t.Fatalf("normalizing a weapon must preserve instance id, got %d", item.InstanceID)
	}
	if item.Description == "old saved description" {
		t.Fatal("weapon description was not refreshed from YAML")
	}
	if len(item.Attributes) != 0 {
		t.Fatalf("weapon attributes should be refreshed from YAML, got %+v", item.Attributes)
	}
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
	game.turnBasedExtraMonsterAction = true

	game.torchLightActive = true
	game.torchLightDuration = 120
	game.torchLightRadius = 4.0
	game.wizardEyeActive = true
	game.wizardEyeDuration = 45
	game.walkOnWaterActive = true
	game.walkOnWaterDuration = 33
	game.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 60, Bonuses: character.UniformStatBonuses(2)})
	game.waterBreathingActive = true
	game.waterBreathingDuration = 25
	game.underwaterReturnX = 96
	game.underwaterReturnY = 128
	game.underwaterReturnMap = "forest"
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
	if loaded.turnBasedExtraMonsterAction != game.turnBasedExtraMonsterAction {
		t.Fatalf("turnBasedExtraMonsterAction: got %v want %v", loaded.turnBasedExtraMonsterAction, game.turnBasedExtraMonsterAction)
	}

	if loaded.torchLightActive != game.torchLightActive || loaded.torchLightDuration != game.torchLightDuration {
		t.Fatalf("torchLight: got %v/%d want %v/%d", loaded.torchLightActive, loaded.torchLightDuration, game.torchLightActive, game.torchLightDuration)
	}
	// The radius deliberately does NOT round-trip: on load an active torch
	// adopts the CURRENT spells.yaml vision_radius_tiles, so old saves pick
	// up retunes.
	torchDef, err := spells.GetSpellDefinitionByID("torch_light")
	if err != nil {
		t.Fatalf("torch_light def: %v", err)
	}
	if loaded.torchLightRadius != torchDef.VisionRadiusTiles {
		t.Fatalf("torchLightRadius: got %v want spells.yaml value %v", loaded.torchLightRadius, torchDef.VisionRadiusTiles)
	}
	if loaded.wizardEyeActive != game.wizardEyeActive || loaded.wizardEyeDuration != game.wizardEyeDuration {
		t.Fatalf("wizardEye: got %v/%d want %v/%d", loaded.wizardEyeActive, loaded.wizardEyeDuration, game.wizardEyeActive, game.wizardEyeDuration)
	}
	if loaded.walkOnWaterActive != game.walkOnWaterActive || loaded.walkOnWaterDuration != game.walkOnWaterDuration {
		t.Fatalf("walkOnWater: got %v/%d want %v/%d", loaded.walkOnWaterActive, loaded.walkOnWaterDuration, game.walkOnWaterActive, game.walkOnWaterDuration)
	}
	loadedBless, ok := loaded.statBuffByID("bless")
	wantBless, _ := game.statBuffByID("bless")
	if !ok || loadedBless != wantBless {
		t.Fatalf("bless buff: got %+v (ok=%v) want %+v", loadedBless, ok, wantBless)
	}
	if loaded.waterBreathingActive != game.waterBreathingActive || loaded.waterBreathingDuration != game.waterBreathingDuration {
		t.Fatalf("waterBreathing: got %v/%d want %v/%d", loaded.waterBreathingActive, loaded.waterBreathingDuration, game.waterBreathingActive, game.waterBreathingDuration)
	}
	if loaded.underwaterReturnX != game.underwaterReturnX || loaded.underwaterReturnY != game.underwaterReturnY || loaded.underwaterReturnMap != game.underwaterReturnMap {
		t.Fatalf("underwaterReturn: got %.0f/%.0f/%s want %.0f/%.0f/%s", loaded.underwaterReturnX, loaded.underwaterReturnY, loaded.underwaterReturnMap, game.underwaterReturnX, game.underwaterReturnY, game.underwaterReturnMap)
	}
	if loaded.statBonuses != game.statBonuses {
		t.Fatalf("statBonuses: got %+v want %+v", loaded.statBonuses, game.statBonuses)
	}

	if loaded.utilitySpellStatuses == nil {
		t.Fatalf("utilitySpellStatuses not initialized")
	}
	if status, ok := loaded.utilitySpellStatuses[spells.SpellID("torch_light")]; !ok || status.Duration != loaded.torchLightDuration {
		t.Fatalf("utility torch_light missing or duration mismatch")
	}
	if status, ok := loaded.utilitySpellStatuses[spells.SpellID("bless")]; !ok || status.Duration != wantBless.Frames {
		t.Fatalf("utility bless icon missing right after load (ok=%v)", ok)
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
		{Kind: ContainerKindLootBag, X: 100, Y: 200, Gold: 42, SizeTiles: 0.5, Items: []items.Item{{Name: "Iron Sword", Type: items.ItemWeapon}}},
		{Kind: ContainerKindLootBag, X: 320, Y: 256, Gold: 0, SizeTiles: 0.33, Items: []items.Item{{Name: "Leather Armor", Type: items.ItemArmor}}},
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
	if loaded.groundContainers[0].SizeTiles != 0.5 {
		t.Fatalf("size multiplier not preserved: %v", loaded.groundContainers[0].SizeTiles)
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
// NPCs still present). Dropping it from the world - the old RemoveNPC behaviour -
// lost the spent state and resurrected the statue unspent on reload.
func TestSpentStatueHiddenButKeptInWorld(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	statue := &character.NPC{RenderCategory: "scenery", Name: "Black Dragon Statue", X: 80, Y: 64, Sprite: "dragon_statue", HideWhenVisited: true}
	w.NPCs = append(w.NPCs, statue)
	game := newTestGame(cfg, w)
	game.renderHelper = NewRenderingHelper(game)
	prevWM := world.GlobalWorldManager
	world.GlobalWorldManager = nil // interact focus reads GetCurrentWorld; pin it to w
	t.Cleanup(func() { world.GlobalWorldManager = prevWM })
	game.camera.Angle = 0 // face the statue: it sits at +X from the camera
	game.camera.FOV = cfg.GetCameraFOV()

	game.updateFocusedNPC()
	if game.focusedNPC != statue {
		t.Fatalf("unspent statue should take interact focus")
	}

	statue.Visited = true // mirrors summonDragonFromStatue: mark spent, keep in world

	game.updateFocusedNPC()
	if game.focusedNPC != nil {
		t.Errorf("spent hide_when_visited statue must not take interact focus")
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

	// Park the player away from the pair - pushes refuse to land on the player.
	g.camera.X, g.camera.Y = 8, 8
	g.collisionSystem.UpdateEntity("player", 8, 8)
	a := monster.NewMonster3DFromConfig(64, 64, "goblin", cfg)
	b := monster.NewMonster3DFromConfig(66, 64, "goblin", cfg) // almost fully stacked
	a.IsEngagingPlayer = true
	a.AIFoe = b // preserve the non-party-fight separation contract under pass-through party combat
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
// blocked on both sides - the pair must fall back to separating ALONG the
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
	a.AIFoe = b
	b.AIFoe = a
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
// clear, even if the same monster type still lives on another map - so culling
// the cliff trolls turns the quest in despite trolls roaming the highlands, and
// a target slain before the quest was taken can still be credited.
func TestCreditClearedKillQuests_RegionScoped(t *testing.T) {
	cfg := loadTestConfig(t)
	qcfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}

	// NPC linked to the cliff troll cull (target_map: dragon_cliffs).
	npc := &character.NPC{RenderCategory: "npc", DialogueData: &character.NPCDialogue{
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

	// Trolls only in the highlands: the cliff region is clear -> quest completes.
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

// A map can enter an old save before its authored markers are valid (the clock
// tower briefly used uppercase monster letters). Such a save serializes an
// empty roster. Once the map gains valid respawn_days spawns, that untracked
// empty snapshot must not erase them again; an actual cleared farming map has
// a respawn stamp and remains empty until its normal refresh window elapses.
func TestSaveLoad_UntrackedEmptyRespawnRosterUsesAuthoredSpawns(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "respawn_test"

	newAuthoredWorld := func() *world.World3D {
		w := newTestWorld(cfg)
		w.MonsterSpawns = []world.MonsterSpawn{{X: 0, Y: 0, MonsterKey: "bandit"}}
		w.RespawnAuthoredMonsters()
		return w
	}
	newManager := func(w *world.World3D) *world.WorldManager {
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		wm.MapConfigs = map[string]*config.MapConfig{mapKey: {RespawnDays: 3}}
		return wm
	}

	for _, tc := range []struct {
		name             string
		respawnStamp     map[string]int
		wantMonsters     int
		wantRespawnStamp int
	}{
		{name: "legacy empty snapshot", wantMonsters: 1, wantRespawnStamp: 1},
		{name: "tracked cleared roster", respawnStamp: map[string]int{mapKey: 3}, wantMonsters: 0, wantRespawnStamp: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wmSave := newManager(newAuthoredWorld())
			gameSave := newTestGame(cfg, wmSave.LoadedMaps[mapKey])
			save := gameSave.buildSave(wmSave)
			save.MapKey = mapKey
			save.MapMonsters = map[string][]MonsterSave{mapKey: []MonsterSave{}}
			save.MapRespawnDay = tc.respawnStamp

			worldLoad := newAuthoredWorld()
			wmLoad := newManager(worldLoad)
			oldWM := world.GlobalWorldManager
			world.GlobalWorldManager = wmLoad
			defer func() { world.GlobalWorldManager = oldWM }()

			loaded := newTestGame(cfg, worldLoad)
			if err := loaded.applySave(wmLoad, &save); err != nil {
				t.Fatalf("apply save: %v", err)
			}
			if got := len(worldLoad.Monsters); got != tc.wantMonsters {
				t.Fatalf("monsters after load = %d, want %d", got, tc.wantMonsters)
			}
			resaved := loaded.buildSave(wmLoad)
			if got := len(resaved.MapMonsters[mapKey]); got != tc.wantMonsters {
				t.Fatalf("monsters after resave = %d, want %d", got, tc.wantMonsters)
			}
			if got := resaved.MapRespawnDay[mapKey]; got != tc.wantRespawnStamp {
				t.Fatalf("respawn stamp after resave = %d, want %d", got, tc.wantRespawnStamp)
			}
			if tc.wantMonsters > 0 {
				if worldLoad.Monsters[0].Key != "bandit" {
					t.Fatalf("restored roster key = %q, want bandit", worldLoad.Monsters[0].Key)
				}
				if worldLoad.LastRespawnDay != loaded.dayNightDay+1 {
					t.Fatalf("respawn stamp = %d, want %d", worldLoad.LastRespawnDay, loaded.dayNightDay+1)
				}
			}
		})
	}
}

// A respawn_days roster with NO stamp is of unknown age (a pre-stamp save):
// arrival must rewind it to the CURRENT authored spawns immediately, so old
// saves pick up re-authored maps on first entry. A stamped roster keeps its
// normal refresh window.
func TestRespawnOnArrival_UnstampedRosterRewindsToAuthored(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "respawn_test"

	setup := func(stamp int) (*MMGame, *world.World3D) {
		w := newTestWorld(cfg)
		w.MonsterSpawns = []world.MonsterSpawn{
			{X: 0, Y: 0, MonsterKey: "bandit"},
			{X: 1, Y: 0, MonsterKey: "bandit"},
		}
		// Stale roster from an old save: one survivor of a smaller authoring.
		w.Monsters = []*monster.Monster3D{monster.NewMonster3DFromConfig(64, 64, "bandit", cfg)}
		w.LastRespawnDay = stamp
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		wm.MapConfigs = map[string]*config.MapConfig{mapKey: {RespawnDays: 3}}
		oldWM := world.GlobalWorldManager
		world.GlobalWorldManager = wm
		t.Cleanup(func() { world.GlobalWorldManager = oldWM })
		return newTestGame(cfg, w), w
	}

	t.Run("unstamped: rewound on arrival", func(t *testing.T) {
		g, w := setup(0)
		g.maybeRespawnMapMonsters()
		if got := len(w.Monsters); got != 2 {
			t.Fatalf("unstamped roster must rewind to authored spawns, got %d monsters, want 2", got)
		}
		if w.LastRespawnDay != g.dayNightDay+1 {
			t.Fatalf("rewind must stamp the day, got %d", w.LastRespawnDay)
		}
	})
	t.Run("fresh stamp: untouched", func(t *testing.T) {
		g, w := setup(1) // spawned "today" (dayNightDay 0 -> stamp 1)
		g.maybeRespawnMapMonsters()
		if got := len(w.Monsters); got != 1 {
			t.Fatalf("freshly stamped roster must keep its refresh window, got %d monsters, want 1", got)
		}
	})
	t.Run("expired stamp: rewound", func(t *testing.T) {
		g, w := setup(1)
		g.dayNightDay = 3 // 3 full phases later
		g.maybeRespawnMapMonsters()
		if got := len(w.Monsters); got != 2 {
			t.Fatalf("expired stamp must rewind, got %d monsters, want 2", got)
		}
	})
}

// Hostility must survive save/load: a provoked monster (WasAttacked) stays
// provoked, and an old save's quest-bearing encounter monster (no was_attacked
// field - e.g. a lair dragon) is migrated to hostile. A chest-bound encounter
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

// A sealed boss (passive-until-quest, no evade radius) that wandered off its
// throne in a pre-fix save must snap back to its MAP spawn on load while its
// quest is unfinished - the saved (wandered) position is discarded. Once the
// quest completes the boss has gone aggressive and may have legitimately moved,
// so it keeps its saved position. Regression for save 1, where the Samurai
// Warlord was baked in mid-castle far from his throne.
func TestSaveLoad_SealedBossSnapsToSpawn(t *testing.T) {
	cfg := loadTestConfig(t)
	const throneX, throneY = 100.0, 100.0   // fresh map spawn (the throne)
	const wanderX, wanderY = 1500.0, 1500.0 // where the buggy save captured him

	restoreBoss := func(status quests.QuestStatus) *monster.Monster3D {
		// Save side: a valid party serialized via buildSave; boss overridden below.
		wmSave := world.NewWorldManager(cfg)
		gSave := newTestGame(cfg, newTestWorld(cfg))
		wmSave.LoadedMaps = map[string]*world.World3D{"japanese_castle": gSave.world}
		wmSave.CurrentMapKey = "japanese_castle"
		save := gSave.buildSave(wmSave)
		save.MapKey = "japanese_castle"
		save.Monsters = nil
		save.MapMonsters = map[string][]MonsterSave{
			"japanese_castle": {{Key: "old_samurai", X: wanderX, Y: wanderY, HitPoints: 1600}},
		}
		save.Quests = []QuestSave{{ID: "castle_armory", Status: string(status)}}

		// Load side: SwitchToMap is skipped (same key), so this freshly-spawned
		// boss on the throne is the spawn the migration captures.
		wmLoad := world.NewWorldManager(cfg)
		worldLoad := newTestWorld(cfg)
		worldLoad.Monsters = []*monster.Monster3D{
			monster.NewMonster3DFromConfig(throneX, throneY, "old_samurai", cfg),
		}
		wmLoad.LoadedMaps = map[string]*world.World3D{"japanese_castle": worldLoad}
		wmLoad.CurrentMapKey = "japanese_castle"
		oldWM := world.GlobalWorldManager
		world.GlobalWorldManager = wmLoad
		defer func() { world.GlobalWorldManager = oldWM }()

		loaded := newTestGame(cfg, worldLoad)
		if err := loaded.applySave(wmLoad, &save); err != nil {
			t.Fatalf("apply save: %v", err)
		}
		for _, m := range worldLoad.Monsters {
			if m.Key == "old_samurai" {
				return m
			}
		}
		t.Fatal("samurai missing after restore")
		return nil
	}

	if b := restoreBoss(quests.QuestStatusActive); b.X != throneX || b.Y != throneY {
		t.Errorf("sealed boss must snap to throne (%.0f,%.0f), got (%.0f,%.0f)", throneX, throneY, b.X, b.Y)
	} else if !b.BossDormant {
		// Set at restore time, not waiting for refreshMonsterAIState (which runs
		// after input) - else a first-frame player action could damage the sealed boss.
		t.Error("sealed boss must be flagged BossDormant immediately on load")
	}
	if b := restoreBoss(quests.QuestStatusCompleted); b.X != wanderX || b.Y != wanderY {
		t.Errorf("unsealed boss must keep its saved position (%.0f,%.0f), got (%.0f,%.0f)", wanderX, wanderY, b.X, b.Y)
	} else if b.BossDormant {
		t.Error("a boss whose quest is complete must not be flagged dormant on load")
	}
}

// TestSaveLoad_IdolWardSetOnRestore guards the idol-ward immediate-init: a warded
// boss must be flagged BossWarded the instant a save loads (refreshMonsterAIState
// runs AFTER input, so without the restore-time pass a first-frame player action
// could damage a still-warded warlord). And with no live idol it must NOT be warded.
func TestSaveLoad_IdolWardSetOnRestore(t *testing.T) {
	cfg := loadTestConfig(t)

	restore := func(idolAlive bool) *monster.Monster3D {
		wSave := newTestWorld(cfg)
		boss := monster.NewMonster3DFromConfig(64, 64, "orc_hero_boss", cfg)
		idol := monster.NewMonster3DFromConfig(128, 64, "jungle_idol", cfg)
		if boss == nil || idol == nil {
			t.Fatal("deep_jungle boss/idol configs must load")
		}
		if !idolAlive {
			idol.HitPoints = 0
		}
		wSave.Monsters = []*monster.Monster3D{boss, idol}
		wmSave := world.NewWorldManager(cfg)
		wmSave.LoadedMaps = map[string]*world.World3D{"deep_jungle": wSave}
		wmSave.CurrentMapKey = "deep_jungle"
		save := newTestGame(cfg, wSave).buildSave(wmSave)
		save.MapKey = "deep_jungle"

		wLoad := newTestWorld(cfg)
		wmLoad := world.NewWorldManager(cfg)
		wmLoad.LoadedMaps = map[string]*world.World3D{"deep_jungle": wLoad}
		wmLoad.CurrentMapKey = "deep_jungle"
		oldWM := world.GlobalWorldManager
		world.GlobalWorldManager = wmLoad
		defer func() { world.GlobalWorldManager = oldWM }()
		if err := newTestGame(cfg, wLoad).applySave(wmLoad, &save); err != nil {
			t.Fatalf("apply save: %v", err)
		}
		for _, m := range wLoad.Monsters {
			if m.Key == "orc_hero_boss" {
				return m
			}
		}
		t.Fatal("warlord missing after restore")
		return nil
	}

	if b := restore(true); !b.BossWarded {
		t.Error("warlord must be flagged BossWarded immediately on load while an idol lives")
	}
	if b := restore(false); b.BossWarded {
		t.Error("warlord must NOT be warded on load when no idol lives")
	}
}

func TestSaveLoad_PersistsSummonedByForBossAdds(t *testing.T) {
	cfg := loadTestConfig(t)
	wSave := newTestWorld(cfg)
	boss := monster.NewMonster3DFromConfig(64, 64, "old_samurai", cfg)
	add := monster.NewMonster3DFromConfig(128, 64, "ashigaru_firelock", cfg)
	if boss == nil || add == nil {
		t.Fatal("japanese castle boss/add configs must load")
	}
	boss.SummonFirstDone = true
	add.SummonedBy = boss.ID
	add.CharmedByParty = true
	add.IsEngagingPlayer = true
	add.WasAttacked = true
	wSave.Monsters = []*monster.Monster3D{boss, add}

	wmSave := world.NewWorldManager(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"japanese_castle": wSave}
	wmSave.CurrentMapKey = "japanese_castle"
	gSave := newTestGame(cfg, wSave)
	save := gSave.buildSave(wmSave)
	save.MapKey = "japanese_castle"

	wLoad := newTestWorld(cfg)
	wmLoad := world.NewWorldManager(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"japanese_castle": wLoad}
	wmLoad.CurrentMapKey = "japanese_castle"
	gLoad := newTestGame(cfg, wLoad)
	if err := gLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	foundBoss := false
	foundAdd := false
	for _, m := range wLoad.Monsters {
		if m.Key == "old_samurai" {
			foundBoss = true
			if !m.SummonFirstDone {
				t.Fatal("boss SummonFirstDone must survive save/load")
			}
		}
		if m.Key == "ashigaru_firelock" {
			foundAdd = true
			if m.SummonedBy != boss.ID {
				t.Fatalf("summoned add SummonedBy = %q, want %q", m.SummonedBy, boss.ID)
			}
			if !m.CharmedByParty {
				t.Fatal("charmed provenance must survive save/load")
			}
		}
	}
	if !foundBoss {
		t.Fatal("boss missing after load")
	}
	if !foundAdd {
		t.Fatal("summoned add missing after load")
	}
}

func TestSaveLoad_RestoresCurrentMonsterSpecialsFromYAML(t *testing.T) {
	cfg := loadTestConfig(t)
	wSave := newTestWorld(cfg)
	wSave.Monsters = []*monster.Monster3D{
		monster.NewMonster3DFromConfig(64, 64, "ashigaru_firelock", cfg),
		monster.NewMonster3DFromConfig(96, 64, "ronin_marksman", cfg),
		monster.NewMonster3DFromConfig(128, 64, "ningyo", cfg),
		monster.NewMonster3DFromConfig(192, 64, "vengeful_ningyo", cfg),
	}

	wmSave := world.NewWorldManager(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"japanese_castle": wSave}
	wmSave.CurrentMapKey = "japanese_castle"
	gSave := newTestGame(cfg, wSave)
	save := gSave.buildSave(wmSave)
	save.MapKey = "japanese_castle"

	wLoad := newTestWorld(cfg)
	wmLoad := world.NewWorldManager(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"japanese_castle": wLoad}
	wmLoad.CurrentMapKey = "japanese_castle"
	gLoad := newTestGame(cfg, wLoad)
	if err := gLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}

	byKey := map[string]*monster.Monster3D{}
	for _, m := range wLoad.Monsters {
		byKey[m.Key] = m
	}
	ashigaru := byKey["ashigaru_firelock"]
	if ashigaru == nil {
		t.Fatal("ashigaru missing after load")
	}
	if ashigaru.PiercingShotChance != 0.50 || ashigaru.PiercingShotTargets != 2 {
		t.Fatalf("ashigaru specials after load = chance %.2f targets %d, want 0.50/2",
			ashigaru.PiercingShotChance, ashigaru.PiercingShotTargets)
	}
	ronin := byKey["ronin_marksman"]
	if ronin == nil {
		t.Fatal("ronin missing after load")
	}
	if ronin.PiercingShotChance != 0.25 || ronin.PiercingShotTargets != 2 {
		t.Fatalf("ronin specials after load = chance %.2f targets %d, want 0.25/2",
			ronin.PiercingShotChance, ronin.PiercingShotTargets)
	}

	for _, key := range []string{"ningyo", "vengeful_ningyo"} {
		m := byKey[key]
		if m == nil {
			t.Fatalf("%s missing after load", key)
		}
		if m.AllyHealChance != 0.15 || m.AllyHealAmount != 200 || m.AllyHealRadiusPixels <= 0 {
			t.Fatalf("%s heal special after load = chance %.2f amount %d radius %.1f, want 0.15/200/>0",
				key, m.AllyHealChance, m.AllyHealAmount, m.AllyHealRadiusPixels)
		}
	}
}

// The entry menu loads saves from the DRAW pass, where the camera angle is
// swapped to the eased display angle for rendering. The end-of-draw restore
// must not clobber the facing a mid-draw load just applied, and the load must
// snap the rendered view too (no easing from the pre-load heading).
func TestSaveLoad_FacingSurvivesDrawTimeLoad(t *testing.T) {
	cfg := loadTestConfig(t)

	wm := world.NewWorldManager(cfg)
	w := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	defer func() { world.GlobalWorldManager = oldWorldManager }()

	saved := newTestGame(cfg, w)
	saved.camera.Angle = 2.5
	save := saved.buildSave(wm)

	// Cold boot: the fresh game faces the default heading with a stale glide.
	g := newTestGame(cfg, w)
	g.camera.Angle = 0
	g.viewAngleRender = 0
	g.viewTurnFramesLeft = 3

	restore := g.beginViewAngleSwap()
	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	restore()

	if g.camera.Angle != 2.5 {
		t.Fatalf("camera.Angle after draw-time load = %v, want the saved 2.5", g.camera.Angle)
	}
	if g.viewAngleRender != 2.5 || g.viewTurnFramesLeft != 0 {
		t.Fatalf("load must snap the rendered view: render=%v framesLeft=%d, want 2.5/0", g.viewAngleRender, g.viewTurnFramesLeft)
	}
}

// Without a mid-draw camera write the swap still restores the logical angle.
func TestBeginViewAngleSwap_RestoresLogicalAngle(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.camera.Angle = 1.0
	g.viewAngleRender = 0.5 // mid-glide display angle

	restore := g.beginViewAngleSwap()
	if g.camera.Angle != 0.5 {
		t.Fatalf("draw must render at the display angle, got %v", g.camera.Angle)
	}
	restore()
	if g.camera.Angle != 1.0 {
		t.Fatalf("restore must return the logical angle, got %v", g.camera.Angle)
	}
}
