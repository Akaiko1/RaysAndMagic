package character

import (
	"testing"
	"ugataima/internal/config"
)

func TestPartyCreation(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			StartingGold: 1000,
			StartingFood: 50,
			Classes: map[string]config.ClassStats{
				"knight":   {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
				"sorcerer": {Might: 8, Intellect: 18, Personality: 16, Endurance: 10, Accuracy: 12, Speed: 14, Luck: 13},
				"cleric":   {Might: 12, Intellect: 14, Personality: 18, Endurance: 14, Accuracy: 13, Speed: 12, Luck: 15},
				"archer":   {Might: 14, Intellect: 12, Personality: 13, Endurance: 12, Accuracy: 18, Speed: 16, Luck: 14},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	t.Run("Default Party Creation", func(t *testing.T) {
		party := NewParty(cfg)

		// Test party initialization
		if party.Gold != cfg.Characters.StartingGold {
			t.Errorf("Expected starting gold %d, got %d", cfg.Characters.StartingGold, party.Gold)
		}

		if party.Food != cfg.Characters.StartingFood {
			t.Errorf("Expected starting food %d, got %d", cfg.Characters.StartingFood, party.Food)
		}

		// Test party members
		expectedMembers := 4
		if len(party.Members) != expectedMembers {
			t.Errorf("Expected %d party members, got %d", expectedMembers, len(party.Members))
		}

		// Test default party composition
		expectedNames := []string{"Gareth", "Lysander", "Celestine", "Silvelyn"}
		expectedClasses := []CharacterClass{ClassKnight, ClassSorcerer, ClassCleric, ClassArcher}

		for i, member := range party.Members {
			if member.Name != expectedNames[i] {
				t.Errorf("Expected member %d to be named %s, got %s", i, expectedNames[i], member.Name)
			}
			if member.Class != expectedClasses[i] {
				t.Errorf("Expected member %d to be class %d, got %d", i, expectedClasses[i], member.Class)
			}
		}
	})

	t.Run("Party Member Addition", func(t *testing.T) {
		party := &Party{
			Members: make([]*MMCharacter, 0, 4),
			Gold:    100,
			Food:    10,
		}

		// Add members one by one
		knight := CreateCharacter("TestKnight", ClassKnight, cfg)
		sorcerer := CreateCharacter("TestSorcerer", ClassSorcerer, cfg)
		cleric := CreateCharacter("TestCleric", ClassCleric, cfg)
		archer := CreateCharacter("TestArcher", ClassArcher, cfg)
		druid := CreateCharacter("TestDruid", ClassDruid, cfg)

		party.AddMember(knight)
		party.AddMember(sorcerer)
		party.AddMember(cleric)
		party.AddMember(archer)

		if len(party.Members) != 4 {
			t.Errorf("Expected 4 members after adding 4, got %d", len(party.Members))
		}

		// Try to add a 5th member (should be rejected)
		party.AddMember(druid)
		if len(party.Members) != 4 {
			t.Errorf("Party should still have 4 members after trying to add 5th, got %d", len(party.Members))
		}
	})
}

func TestPartyUpdate(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			StartingGold: 1000,
			StartingFood: 50,
			Classes: map[string]config.ClassStats{
				"knight":   {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
				"sorcerer": {Might: 8, Intellect: 18, Personality: 16, Endurance: 10, Accuracy: 12, Speed: 14, Luck: 13},
				"cleric":   {Might: 12, Intellect: 14, Personality: 18, Endurance: 14, Accuracy: 13, Speed: 12, Luck: 15},
				"archer":   {Might: 14, Intellect: 12, Personality: 13, Endurance: 12, Accuracy: 18, Speed: 16, Luck: 14},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	party := NewParty(cfg)

	// Reduce spell points for all members
	originalSP := make([]int, len(party.Members))
	for i, member := range party.Members {
		originalSP[i] = member.SpellPoints
		member.SpellPoints -= 5
	}

	// Simulate enough frames for spell regeneration (180 frames)
	for i := 0; i < 180; i++ {
		party.Update()
	}

	// Check that all members were updated (spell points should regenerate)
	for i, member := range party.Members {
		expectedSP := originalSP[i] - 3 // Should regenerate 2 points
		if member.SpellPoints != expectedSP {
			t.Errorf("Member %d: expected SP %d, got %d", i, expectedSP, member.SpellPoints)
		}
	}
}

func TestPartyValidation(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			StartingGold: 1000,
			StartingFood: 50,
			Classes: map[string]config.ClassStats{
				"knight": {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	t.Run("Empty Party", func(t *testing.T) {
		party := &Party{
			Members: make([]*MMCharacter, 0, 4),
			Gold:    0,
			Food:    0,
		}

		// Party should handle empty member list gracefully
		party.Update() // Should not panic
	})

	t.Run("Party Capacity", func(t *testing.T) {
		party := &Party{
			Members: make([]*MMCharacter, 0, 4),
			Gold:    100,
			Food:    10,
		}

		// Fill party to capacity
		for i := 0; i < 4; i++ {
			knight := CreateCharacter("Knight"+string(rune('A'+i)), ClassKnight, cfg)
			party.AddMember(knight)
		}

		if len(party.Members) != 4 {
			t.Errorf("Expected party size 4, got %d", len(party.Members))
		}

		// Verify party is at capacity
		extraMember := CreateCharacter("ExtraKnight", ClassKnight, cfg)
		initialSize := len(party.Members)
		party.AddMember(extraMember)

		if len(party.Members) != initialSize {
			t.Error("Party should not accept members beyond capacity of 4")
		}
	})
}
