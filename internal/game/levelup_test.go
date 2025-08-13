package game

import (
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

func TestLevelUpSystem(t *testing.T) {
	// Load configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create a test character manually without the full party setup
	testChar := &character.MMCharacter{
		Name:           "TestHero",
		Level:          1,
		Experience:     0,
		FreeStatPoints: 0,
		Might:          15,
		Intellect:      10,
		Personality:    10,
		Endurance:      15,
		Accuracy:       10,
		Speed:          10,
		Luck:           10,
		Class:          character.ClassKnight,
	}

	// Calculate initial derived stats based on config
	testChar.CalculateDerivedStats(cfg)

	// Create a minimal game instance for the combat system
	game := &MMGame{
		config: cfg,
	}

	// Create combat system
	combatSystem := NewCombatSystem(game)

	t.Run("Basic Level Up", func(t *testing.T) {
		// Reset character to level 1 with 0 experience
		testChar.Level = 1
		testChar.Experience = 0
		testChar.FreeStatPoints = 0

		// Store initial values
		initialLevel := testChar.Level
		initialStatPoints := testChar.FreeStatPoints
		initialMaxHP := testChar.MaxHitPoints
		initialMaxSP := testChar.MaxSpellPoints

		// Give enough experience for level 2 (level 1 requires 100 exp)
		testChar.Experience = 100

		// Call checkLevelUp
		combatSystem.checkLevelUp(testChar)

		// Verify level increased
		if testChar.Level != initialLevel+1 {
			t.Errorf("Expected level %d, got %d", initialLevel+1, testChar.Level)
		}

		// Verify experience was consumed
		if testChar.Experience != 0 {
			t.Errorf("Expected experience 0 after level up, got %d", testChar.Experience)
		}

		// Verify stat points were granted
		expectedStatPoints := initialStatPoints + 5
		if testChar.FreeStatPoints != expectedStatPoints {
			t.Errorf("Expected %d free stat points, got %d", expectedStatPoints, testChar.FreeStatPoints)
		}

		// Verify derived stats were recalculated (should be higher)
		if testChar.MaxHitPoints <= initialMaxHP {
			t.Errorf("Expected MaxHitPoints to increase from %d, got %d", initialMaxHP, testChar.MaxHitPoints)
		}

		if testChar.MaxSpellPoints <= initialMaxSP {
			t.Errorf("Expected MaxSpellPoints to increase from %d, got %d", initialMaxSP, testChar.MaxSpellPoints)
		}

		// Verify full health/mana restoration
		if testChar.HitPoints != testChar.MaxHitPoints {
			t.Errorf("Expected full HP restoration: %d/%d", testChar.HitPoints, testChar.MaxHitPoints)
		}

		if testChar.SpellPoints != testChar.MaxSpellPoints {
			t.Errorf("Expected full SP restoration: %d/%d", testChar.SpellPoints, testChar.MaxSpellPoints)
		}
	})

	t.Run("Multiple Level Ups", func(t *testing.T) {
		// Reset character
		testChar.Level = 1
		testChar.Experience = 0
		testChar.FreeStatPoints = 0

		// Give enough experience for multiple levels
		// Level 1->2 needs 100, Level 2->3 needs 200, Level 3->4 needs 300
		testChar.Experience = 600 // Should reach level 4

		initialLevel := testChar.Level

		// Call checkLevelUp
		combatSystem.checkLevelUp(testChar)

		// Should reach level 4 (1->2->3->4)
		expectedLevel := 4
		if testChar.Level != expectedLevel {
			t.Errorf("Expected level %d after multiple level ups, got %d", expectedLevel, testChar.Level)
		}

		// Should have consumed 100+200+300 = 600 experience
		if testChar.Experience != 0 {
			t.Errorf("Expected experience 0 after multiple level ups, got %d", testChar.Experience)
		}

		// Should have gained 15 stat points (3 levels * 5 points)
		expectedStatPoints := (testChar.Level - initialLevel) * 5
		if testChar.FreeStatPoints != expectedStatPoints {
			t.Errorf("Expected %d free stat points from multiple level ups, got %d", expectedStatPoints, testChar.FreeStatPoints)
		}
	})

	t.Run("Experience Calculation", func(t *testing.T) {
		// Test the experience requirement formula
		testCases := []struct {
			level       int
			expectedExp int
		}{
			{1, 100},   // Level 1->2 needs 100
			{2, 200},   // Level 2->3 needs 200
			{3, 300},   // Level 3->4 needs 300
			{10, 1000}, // Level 10->11 needs 1000
		}

		for _, tc := range testCases {
			// Reset to test level
			testChar.Level = tc.level
			testChar.Experience = tc.expectedExp - 1 // Just below threshold

			combatSystem.checkLevelUp(testChar)
			if testChar.Level != tc.level {
				t.Errorf("Character shouldn't level up with %d experience at level %d", tc.expectedExp-1, tc.level)
			}

			// Now give exact amount needed
			testChar.Experience = tc.expectedExp
			combatSystem.checkLevelUp(testChar)
			if testChar.Level != tc.level+1 {
				t.Errorf("Character should level up from %d to %d with %d experience", tc.level, tc.level+1, tc.expectedExp)
			}
		}
	})

	t.Run("Turn-Based Mode Level Up", func(t *testing.T) {
		// Reset character for this test
		testChar.Level = 1
		testChar.Experience = 0
		testChar.FreeStatPoints = 0
		testChar.CalculateDerivedStats(cfg)

		// Create a minimal game instance to test turn-based mode
		fullGame := &MMGame{
			config:         cfg,
			combatMessages: make([]string, 0),
			maxMessages:    3,
			turnBasedMode:  true, // Set turn-based mode
		}

		// Create combat system for the full game
		fullCombatSystem := NewCombatSystem(fullGame)

		// Give enough experience for level up
		testChar.Experience = 100

		// Capture initial combat messages count
		initialMessageCount := len(fullGame.combatMessages)

		// Call checkLevelUp
		fullCombatSystem.checkLevelUp(testChar)

		// Verify level up occurred
		if testChar.Level != 2 {
			t.Errorf("Expected level 2 in turn-based mode, got %d", testChar.Level)
		}

		// Verify stat points were granted
		if testChar.FreeStatPoints != 5 {
			t.Errorf("Expected 5 stat points in turn-based mode, got %d", testChar.FreeStatPoints)
		}

		// Verify combat message was added
		if len(fullGame.combatMessages) <= initialMessageCount {
			t.Error("Expected level up combat message to be added in turn-based mode")
		}

		// Check if the message contains level up information
		found := false
		for _, msg := range fullGame.combatMessages {
			if strings.Contains(msg, "reached level") && strings.Contains(msg, "stat points") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Level up message not found in combat messages: %v", fullGame.combatMessages)
		}

		t.Logf("Turn-based level up successful. Messages: %v", fullGame.combatMessages)
	})
}
