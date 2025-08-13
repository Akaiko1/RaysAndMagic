package character

import (
	"testing"
	"ugataima/internal/config"
)

func TestCharacterCreation(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"knight": {
					Might: 18, Intellect: 10, Personality: 12, Endurance: 16,
					Accuracy: 14, Speed: 13, Luck: 11,
				},
				"sorcerer": {
					Might: 8, Intellect: 18, Personality: 16, Endurance: 10,
					Accuracy: 12, Speed: 14, Luck: 13,
				},
				"cleric": {
					Might: 12, Intellect: 14, Personality: 18, Endurance: 14,
					Accuracy: 13, Speed: 12, Luck: 15,
				},
				"archer": {
					Might: 14, Intellect: 12, Personality: 13, Endurance: 12,
					Accuracy: 18, Speed: 16, Luck: 14,
				},
			},
			HitPoints: config.HitPointsConfig{
				EnduranceMultiplier: 3,
				LevelMultiplier:     2,
			},
			SpellPoints: config.SpellPointsConfig{
				LevelMultiplier: 2,
			},
		},
	}

	tests := []struct {
		name          string
		characterName string
		class         CharacterClass
		expectedStats map[string]int
	}{
		{
			name:          "Knight Creation",
			characterName: "TestKnight",
			class:         ClassKnight,
			expectedStats: map[string]int{
				"might":     18,
				"endurance": 16,
				"level":     1,
			},
		},
		{
			name:          "Sorcerer Creation",
			characterName: "TestSorcerer",
			class:         ClassSorcerer,
			expectedStats: map[string]int{
				"intellect":   18,
				"personality": 16,
				"level":       1,
			},
		},
		{
			name:          "Cleric Creation",
			characterName: "TestCleric",
			class:         ClassCleric,
			expectedStats: map[string]int{
				"personality": 18,
				"intellect":   14,
				"level":       1,
			},
		},
		{
			name:          "Archer Creation",
			characterName: "TestArcher",
			class:         ClassArcher,
			expectedStats: map[string]int{
				"accuracy": 18,
				"speed":    16,
				"level":    1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := CreateCharacter(tt.characterName, tt.class, cfg)

			// Test basic properties
			if char.Name != tt.characterName {
				t.Errorf("Expected name %s, got %s", tt.characterName, char.Name)
			}
			if char.Class != tt.class {
				t.Errorf("Expected class %d, got %d", tt.class, char.Class)
			}

			// Test stats
			for statName, expectedValue := range tt.expectedStats {
				var actualValue int
				switch statName {
				case "might":
					actualValue = char.Might
				case "intellect":
					actualValue = char.Intellect
				case "personality":
					actualValue = char.Personality
				case "endurance":
					actualValue = char.Endurance
				case "accuracy":
					actualValue = char.Accuracy
				case "speed":
					actualValue = char.Speed
				case "level":
					actualValue = char.Level
				}

				if actualValue != expectedValue {
					t.Errorf("Expected %s %d, got %d", statName, expectedValue, actualValue)
				}
			}

			// Test that derived stats are calculated
			if char.MaxHitPoints <= 0 {
				t.Error("MaxHitPoints should be greater than 0")
			}
			if char.HitPoints != char.MaxHitPoints {
				t.Error("New character should start with full hit points")
			}
		})
	}
}

func TestCharacterSkills(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"knight":   {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
				"sorcerer": {Might: 8, Intellect: 18, Personality: 16, Endurance: 10, Accuracy: 12, Speed: 14, Luck: 13},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	tests := []struct {
		name           string
		class          CharacterClass
		expectedSkills []SkillType
		expectedMagic  []MagicSchool
	}{
		{
			name:           "Knight Skills",
			class:          ClassKnight,
			expectedSkills: []SkillType{SkillSword, SkillChain, SkillShield, SkillBodybuilding},
			expectedMagic:  []MagicSchool{},
		},
		{
			name:           "Sorcerer Skills",
			class:          ClassSorcerer,
			expectedSkills: []SkillType{SkillDagger, SkillLeather, SkillMeditation},
			expectedMagic:  []MagicSchool{MagicFire},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := CreateCharacter("TestChar", tt.class, cfg)

			// Test skills
			for _, expectedSkill := range tt.expectedSkills {
				if skill, exists := char.Skills[expectedSkill]; !exists {
					t.Errorf("Expected skill %d not found", expectedSkill)
				} else if skill.Level != 1 || skill.Mastery != MasteryNovice {
					t.Errorf("Expected skill %d at level 1 with Novice mastery", expectedSkill)
				}
			}

			// Test magic schools
			for _, expectedSchool := range tt.expectedMagic {
				if magicSkill, exists := char.MagicSchools[expectedSchool]; !exists {
					t.Errorf("Expected magic school %d not found", expectedSchool)
				} else if magicSkill.Level != 1 || magicSkill.Mastery != MasteryNovice {
					t.Errorf("Expected magic school %d at level 1 with Novice mastery", expectedSchool)
				}
			}
		})
	}
}

func TestCharacterUpdate(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"cleric": {Might: 12, Intellect: 14, Personality: 18, Endurance: 14, Accuracy: 13, Speed: 12, Luck: 15},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	char := CreateCharacter("TestCleric", ClassCleric, cfg)

	// Reduce spell points to test regeneration
	originalSP := char.SpellPoints
	char.SpellPoints = originalSP - 5

	// Simulate enough frames for spell regeneration (180 frames)
	for i := 0; i < 180; i++ {
		char.Update()
	}

	// Check spell point regeneration
	if char.SpellPoints != originalSP-3 {
		t.Errorf("Expected spell points to regenerate by 2, got %d", char.SpellPoints)
	}

	// Test that SP doesn't exceed maximum
	char.SpellPoints = char.MaxSpellPoints
	char.Update()

	if char.SpellPoints > char.MaxSpellPoints {
		t.Error("Spell points should not exceed maximum")
	}
}

func TestCharacterDisplayInfo(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			Classes: map[string]config.ClassStats{
				"knight": {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	char := CreateCharacter("TestKnight", ClassKnight, cfg)

	displayInfo := char.GetDisplayInfo()
	if displayInfo == "" {
		t.Error("Display info should not be empty")
	}

	detailedInfo := char.GetDetailedInfo()
	if detailedInfo == "" {
		t.Error("Detailed info should not be empty")
	}

	// Check that character name appears in display info
	if len(displayInfo) < len(char.Name) {
		t.Error("Display info should contain character name")
	}
}
