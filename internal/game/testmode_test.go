package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// loadTestArenaData loads the extra YAML the test-arena path reads beyond what
// loadTestConfig already wires (loot tables, level-up choices, NPC encounters,
// quests).
func loadTestArenaData(t *testing.T) {
	t.Helper()
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
		t.Fatalf("load level_up: %v", err)
	}
	character.MustLoadNPCConfig("../../assets/npcs.yaml")
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	quests.GlobalQuestManager = quests.NewQuestManager(qc)
	quests.GlobalQuestManager.InitializeStartingQuests()
}

func TestApplyTestArena(t *testing.T) {
	cfg := loadTestConfig(t)
	loadTestArenaData(t)

	// Forest world with monsters whose loot tables are non-empty. Enough
	// treants (400 XP each) so the party clears level 6 — high enough that the
	// 16-speed / 20-endurance targets are affordable for every class.
	forest := newTestWorld(cfg)
	for _, key := range []string{"bandit", "wolf", "treant"} {
		forest.Monsters = append(forest.Monsters, monster.NewMonster3DFromConfig(64, 64, key, cfg))
	}
	for i := 0; i < 12; i++ {
		forest.Monsters = append(forest.Monsters, monster.NewMonster3DFromConfig(64, 64, "treant", cfg))
	}

	church := newTestWorld(cfg)
	church.Monsters = append(church.Monsters, monster.NewMonster3DFromConfig(64, 64, "skeleton", cfg))

	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": forest, "church": church}
	wm.CurrentMapKey = "forest"
	wm.MapConfigs = map[string]*config.MapConfig{
		"church": {
			ClearEncounter: &config.MapClearEncounterConfig{
				Rewards: &config.MapEncounterRewardsConfig{
					Gold: 250,
					TreasureChest: &config.MapTreasureChestRewardConfig{
						RandomWeaponCount: 1,
					},
				},
			},
		},
	}
	world.GlobalWorldManager = wm

	game := newTestGame(cfg, forest)
	game.combat = NewCombatSystem(game) // required for XP-driven level-ups
	for _, m := range forest.Monsters {
		game.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 16, 16, collision.CollisionTypeMonster, false))
	}
	// Snapshot base stats before progression so we can verify exactly the
	// earned level-up points were distributed (nothing invented, nothing lost).
	type baseStats struct{ speed, end, main int }
	mainStat := func(m *character.MMCharacter) int {
		switch m.Class {
		case character.ClassSorcerer, character.ClassDruid:
			return m.Intellect
		case character.ClassCleric:
			return m.Personality
		case character.ClassArcher:
			return m.Accuracy
		default:
			return m.Might
		}
	}
	base := make([]baseStats, len(game.party.Members))
	for i, m := range game.party.Members {
		base[i] = baseStats{m.Speed, m.Endurance, mainStat(m)}
	}

	goldBefore := game.party.Gold
	itemsBefore := game.party.GetTotalItems()

	game.ApplyTestArena()

	// Party progression. Level is earned from clearing XP (not set directly), so
	// it must have climbed well past the level-3 skill-choice threshold.
	for i, m := range game.party.Members {
		if m.Level < 4 {
			t.Errorf("%s level = %d, want >= 4 (earned from clearing XP)", m.Name, m.Level)
		}
		if m.FreeStatPoints != 0 {
			t.Errorf("%s free stat points = %d, want 0 (all spent)", m.Name, m.FreeStatPoints)
		}
		if m.Speed != testArenaSpeed {
			t.Errorf("%s speed = %d, want %d", m.Name, m.Speed, testArenaSpeed)
		}
		if m.Endurance != testArenaEndurance {
			t.Errorf("%s endurance = %d, want %d", m.Name, m.Endurance, testArenaEndurance)
		}
		if m.HitPoints != m.MaxHitPoints || m.HitPoints <= 0 {
			t.Errorf("%s hp = %d/%d, want full and positive", m.Name, m.HitPoints, m.MaxHitPoints)
		}
		// Stats must be level-consistent: exactly the earned points
		// (StatPointsPerLevel per level gained) distributed across the three
		// pumped stats, no more, no less.
		earned := StatPointsPerLevel * (m.Level - 1)
		spent := (m.Speed - base[i].speed) + (m.Endurance - base[i].end) + (mainStat(m) - base[i].main)
		if spent != earned {
			t.Errorf("%s distributed %d stat points, want %d (level %d)", m.Name, spent, earned, m.Level)
		}
	}

	// Every member should have an unspent skill choice waiting.
	for i := range game.party.Members {
		if !game.hasLevelUpChoiceForChar(i) {
			t.Errorf("member %d has no pending level-up choice", i)
		}
	}
	if game.levelUpChoiceOpen {
		t.Error("level-up choice should hang unopened, not be auto-opened")
	}

	// Forest cleared.
	if len(forest.Monsters) != 0 {
		t.Errorf("forest monsters = %d, want 0", len(forest.Monsters))
	}
	if len(church.Monsters) != 0 {
		t.Errorf("church monsters = %d, want 0", len(church.Monsters))
	}

	// Loot + encounter gold granted. Monster drops are rolled by real chance
	// (not handed out), so no single drop is guaranteed — but the church chest
	// always yields one random weapon, so the inventory must grow regardless.
	if game.party.GetTotalItems() <= itemsBefore {
		t.Errorf("inventory did not grow: before=%d after=%d", itemsBefore, game.party.GetTotalItems())
	}
	if game.party.Gold <= goldBefore {
		t.Errorf("gold did not increase: before=%d after=%d", goldBefore, game.party.Gold)
	}

	// Shipwreck quest registered + completed (rewards auto-claimed).
	found := false
	for _, q := range quests.GlobalQuestManager.GetAllQuests() {
		if q.ID == "shipwreck_bandits" {
			found = true
			if !q.Completed {
				t.Error("shipwreck_bandits quest exists but is not completed")
			}
		}
	}
	if !found {
		t.Error("shipwreck_bandits quest was not registered")
	}
}

// learnAllSchoolSpells (test-arena) gives a caster every available spell of each
// magic school it already has.
func TestLearnAllSchoolSpells_FillsEverySchool(t *testing.T) {
	cfg := loadTestConfig(t)
	c := character.CreateCharacter("Tester", character.ClassSorcerer, cfg)
	if len(c.MagicSchools) == 0 {
		t.Fatal("sorcerer should start with magic schools")
	}
	learnAllSchoolSpells(c)
	for school, skill := range c.MagicSchools {
		want, err := school.AvailableSpellIDs()
		if err != nil {
			t.Fatalf("available spells for %s: %v", school, err)
		}
		if len(want) == 0 {
			t.Errorf("school %s has no available spells to learn", school)
		}
		if len(skill.KnownSpells) != len(want) {
			t.Errorf("school %s: learned %d spells, want all %d", school, len(skill.KnownSpells), len(want))
		}
	}
}
