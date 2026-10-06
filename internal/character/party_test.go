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
				"paladin":  {Might: 13, Intellect: 10, Personality: 13, Endurance: 13, Accuracy: 10, Speed: 10, Luck: 11},
				"druid":    {Might: 10, Intellect: 13, Personality: 13, Endurance: 10, Accuracy: 10, Speed: 11, Luck: 11},
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

// Party.UpdateWithMode is the game's per-frame party clock: RT frames pay each
// member's regen cadence and report it; TB frames never do (TB regen is paid
// per round by the game); an empty party is a no-op.
func TestPartyUpdateWithMode(t *testing.T) {
	cfg := &config.Config{
		Characters: config.CharacterConfig{
			StartingGold: 1000,
			StartingFood: 50,
			Classes: map[string]config.ClassStats{
				"knight":   {Might: 18, Intellect: 10, Personality: 12, Endurance: 16, Accuracy: 14, Speed: 13, Luck: 11},
				"sorcerer": {Might: 8, Intellect: 18, Personality: 16, Endurance: 10, Accuracy: 12, Speed: 14, Luck: 13},
				"cleric":   {Might: 12, Intellect: 14, Personality: 18, Endurance: 14, Accuracy: 13, Speed: 12, Luck: 15},
				"archer":   {Might: 14, Intellect: 12, Personality: 13, Endurance: 12, Accuracy: 18, Speed: 16, Luck: 14},
				"paladin":  {Might: 13, Intellect: 10, Personality: 13, Endurance: 13, Accuracy: 10, Speed: 10, Luck: 11},
				"druid":    {Might: 10, Intellect: 13, Personality: 13, Endurance: 10, Accuracy: 10, Speed: 11, Luck: 11},
			},
			HitPoints:   config.HitPointsConfig{EnduranceMultiplier: 3, LevelMultiplier: 2},
			SpellPoints: config.SpellPointsConfig{LevelMultiplier: 2},
		},
	}

	for _, tt := range []struct {
		name        string
		turnBased   bool
		wantRegen   int // SP gained per member over two intervals
		wantReports int // frames reporting a completed cadence
	}{
		{"real-time frames pay the cadence", false, 4, 2}, // Personality >= 10 => 2 SP per payout
		{"turn-based frames pay nothing", true, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			party := NewParty(cfg)
			start := make([]int, len(party.Members))
			for i, member := range party.Members {
				member.SpellPoints -= 5
				start[i] = member.SpellPoints
			}
			reports := 0
			for i := 0; i < 2*ManaRegenIntervalFrames; i++ {
				if party.UpdateWithMode(tt.turnBased) {
					reports++
				}
			}
			if reports != tt.wantReports {
				t.Errorf("frames reporting a cadence = %d, want %d", reports, tt.wantReports)
			}
			for i, member := range party.Members {
				if want := start[i] + tt.wantRegen; member.SpellPoints != want {
					t.Errorf("member %d: SP %d, want %d", i, member.SpellPoints, want)
				}
			}
		})
	}

	t.Run("empty party", func(t *testing.T) {
		party := &Party{Members: make([]*MMCharacter, 0, 4)}
		if party.UpdateWithMode(false) {
			t.Error("an empty party reported a completed cadence")
		}
	})
}
