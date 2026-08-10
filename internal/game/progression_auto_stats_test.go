package game

import (
	"testing"

	"ugataima/internal/character"
)

func TestAutoEnduranceTargets(t *testing.T) {
	tests := []struct {
		class character.CharacterClass
		want  int
	}{
		{character.ClassKnight, 28},
		{character.ClassBattleMage, 36},
		{character.ClassMonk, 28},
		{character.ClassArmsMaster, 26},
		{character.ClassPaladin, 24},
		{character.ClassCleric, 22},
		{character.ClassDruid, 20},
		{character.ClassArcher, 18},
		{character.ClassThief, 18},
		{character.ClassSorcerer, 16},
	}
	if len(tests) != len(character.PlayableClasses) {
		t.Fatalf("endurance table has %d classes, want all %d playable classes", len(tests), len(character.PlayableClasses))
	}
	for _, tt := range tests {
		if got := autoEnduranceTarget(tt.class); got != tt.want {
			t.Errorf("%s endurance target = %d, want %d", tt.class, got, tt.want)
		}
	}
}

func TestAutoSpeedTargets(t *testing.T) {
	tests := []struct {
		class character.CharacterClass
		want  int
	}{
		{character.ClassKnight, 16},
		{character.ClassPaladin, 16},
		{character.ClassArcher, 16},
		{character.ClassCleric, 16},
		{character.ClassSorcerer, 16},
		{character.ClassDruid, 16},
		{character.ClassThief, 16},
		{character.ClassArmsMaster, 16},
		{character.ClassMonk, 26},
		{character.ClassBattleMage, 16},
	}
	if len(tests) != len(character.PlayableClasses) {
		t.Fatalf("speed table has %d classes, want all %d playable classes", len(tests), len(character.PlayableClasses))
	}
	for _, tt := range tests {
		if got := autoSpeedTarget(tt.class); got != tt.want {
			t.Errorf("%s speed target = %d, want %d", tt.class, got, tt.want)
		}
	}
}

func TestAutoDistributeStatPointsPrioritiesAndLeavesSkillsAlone(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		class             character.CharacterClass
		primary           string
		secondary         string
		wantPrimaryGain   int
		wantSecondaryGain int
	}{
		{character.ClassKnight, "might", "", 6, 0},
		{character.ClassPaladin, "might", "personality", 4, 2},
		{character.ClassArcher, "accuracy", "intellect", 4, 2},
		{character.ClassCleric, "personality", "", 6, 0},
		{character.ClassSorcerer, "intellect", "", 6, 0},
		{character.ClassDruid, "intellect", "personality", 4, 2},
		{character.ClassThief, "accuracy", "intellect", 4, 2},
		{character.ClassArmsMaster, "might", "", 6, 0},
		{character.ClassMonk, "might", "personality", 4, 2},
		{character.ClassBattleMage, "intellect", "might", 4, 2},
	}
	if len(tests) != len(character.PlayableClasses) {
		t.Fatalf("priority table has %d classes, want all %d playable classes", len(tests), len(character.PlayableClasses))
	}

	statValue := func(member *character.MMCharacter, stat string) int {
		switch stat {
		case "might":
			return member.Might
		case "intellect":
			return member.Intellect
		case "personality":
			return member.Personality
		case "accuracy":
			return member.Accuracy
		default:
			return 0
		}
	}

	for _, tt := range tests {
		t.Run(tt.class.String(), func(t *testing.T) {
			member := character.CreateCharacter("Auto", tt.class, cfg)
			member.Might, member.Intellect, member.Personality = 10, 10, 10
			member.Accuracy, member.Speed = 10, autoSpeedTarget(tt.class)
			member.Endurance = autoEnduranceTarget(tt.class) - 2
			member.FreeStatPoints = 8
			member.OwedLevelChoices = []int{3}

			masteryBefore := make(map[character.SkillType]character.SkillMastery)
			for skillType, skill := range member.Skills {
				masteryBefore[skillType] = skill.Mastery
			}

			spent := autoDistributeStatPoints(member, cfg)
			if spent != 8 || member.FreeStatPoints != 0 {
				t.Fatalf("spent/free = %d/%d, want 8/0", spent, member.FreeStatPoints)
			}
			if member.Speed != autoSpeedTarget(tt.class) {
				t.Errorf("speed = %d, want %d", member.Speed, autoSpeedTarget(tt.class))
			}
			if member.Endurance != autoEnduranceTarget(tt.class) {
				t.Errorf("endurance = %d, want %d", member.Endurance, autoEnduranceTarget(tt.class))
			}
			if got := statValue(member, tt.primary); got != 10+tt.wantPrimaryGain {
				t.Errorf("%s = %d, want %d", tt.primary, got, 10+tt.wantPrimaryGain)
			}
			if tt.secondary != "" {
				if got := statValue(member, tt.secondary); got != 10+tt.wantSecondaryGain {
					t.Errorf("%s = %d, want %d", tt.secondary, got, 10+tt.wantSecondaryGain)
				}
			}
			if len(member.OwedLevelChoices) != 1 || member.OwedLevelChoices[0] != 3 {
				t.Errorf("owed choices changed: %v", member.OwedLevelChoices)
			}
			for skillType, mastery := range masteryBefore {
				if got := member.Skills[skillType].Mastery; got != mastery {
					t.Errorf("%s mastery changed from %v to %v", skillType, mastery, got)
				}
			}
		})
	}
}

// Monk AUTO covers four distinct class needs: the Fists/TB Speed threshold,
// unarmored HP, Fists' primary Might term, and Personality-scaled offensive
// self-magic fired by Spiritual Training. Every row enters through the real
// allocator at one phase boundary.
func TestMonkAutoLevelContractTable(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name                       string
		points                     int
		speed, endurance           int
		might, personality         int
		wantSpeed, wantEndurance   int
		wantMight, wantPersonality int
	}{
		{
			name:   "speed gate",
			points: 1, speed: 25, endurance: 27, might: 10, personality: 10,
			wantSpeed: 26, wantEndurance: 27, wantMight: 10, wantPersonality: 10,
		},
		{
			name:   "endurance alternates with primary",
			points: 2, speed: 26, endurance: 27, might: 10, personality: 10,
			wantSpeed: 26, wantEndurance: 28, wantMight: 11, wantPersonality: 10,
		},
		{
			name:   "primary alternates with secondary",
			points: 3, speed: 26, endurance: 28, might: 10, personality: 10,
			wantSpeed: 26, wantEndurance: 28, wantMight: 12, wantPersonality: 11,
		},
		{
			name:   "secondary soft cap leaves primary climbing",
			points: 2, speed: 26, endurance: 28, might: 10, personality: autoSecondarySoftCap,
			wantSpeed: 26, wantEndurance: 28, wantMight: 12, wantPersonality: autoSecondarySoftCap,
		},
		{
			name:   "maxed primary releases secondary and endurance",
			points: 2, speed: 26, endurance: 28, might: MaxStatValue, personality: autoSecondarySoftCap,
			wantSpeed: 26, wantEndurance: 29, wantMight: MaxStatValue, wantPersonality: autoSecondarySoftCap + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := character.CreateCharacter("Auto", character.ClassMonk, cfg)
			member.Speed, member.Endurance = tt.speed, tt.endurance
			member.Might, member.Personality = tt.might, tt.personality
			member.FreeStatPoints = tt.points

			if spent := autoDistributeStatPoints(member, cfg); spent != tt.points {
				t.Fatalf("spent = %d, want %d", spent, tt.points)
			}
			if member.Speed != tt.wantSpeed || member.Endurance != tt.wantEndurance ||
				member.Might != tt.wantMight || member.Personality != tt.wantPersonality {
				t.Fatalf("Speed/Endurance/Might/Personality = %d/%d/%d/%d, want %d/%d/%d/%d",
					member.Speed, member.Endurance, member.Might, member.Personality,
					tt.wantSpeed, tt.wantEndurance, tt.wantMight, tt.wantPersonality)
			}
		})
	}
}

// Battle Mage's AUTO contract is a four-stat priority ladder. Each row enters
// through the real allocator at a different phase so a missing class switch or
// reordered production branch fails at the behavior boundary, not only in a
// classifier helper.
func TestBattleMageAutoLevelContractTable(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name                     string
		points                   int
		speed, endurance         int
		intellect, might         int
		wantSpeed, wantEndurance int
		wantIntellect, wantMight int
	}{
		{
			name:   "speed gate",
			points: 1, speed: 15, endurance: 35, intellect: 10, might: 10,
			wantSpeed: 16, wantEndurance: 35, wantIntellect: 10, wantMight: 10,
		},
		{
			name:   "endurance alternates with primary",
			points: 2, speed: 16, endurance: 35, intellect: 10, might: 10,
			wantSpeed: 16, wantEndurance: 36, wantIntellect: 11, wantMight: 10,
		},
		{
			name:   "primary alternates with secondary",
			points: 3, speed: 16, endurance: 36, intellect: 10, might: 10,
			wantSpeed: 16, wantEndurance: 36, wantIntellect: 12, wantMight: 11,
		},
		{
			name:   "secondary soft cap leaves primary climbing",
			points: 2, speed: 16, endurance: 36, intellect: 10, might: autoSecondarySoftCap,
			wantSpeed: 16, wantEndurance: 36, wantIntellect: 12, wantMight: autoSecondarySoftCap,
		},
		{
			name:   "maxed primary releases secondary and endurance",
			points: 2, speed: 16, endurance: 36, intellect: MaxStatValue, might: autoSecondarySoftCap,
			wantSpeed: 16, wantEndurance: 37, wantIntellect: MaxStatValue, wantMight: autoSecondarySoftCap + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := character.CreateCharacter("Auto", character.ClassBattleMage, cfg)
			member.Speed, member.Endurance = tt.speed, tt.endurance
			member.Intellect, member.Might = tt.intellect, tt.might
			member.FreeStatPoints = tt.points

			if spent := autoDistributeStatPoints(member, cfg); spent != tt.points {
				t.Fatalf("spent = %d, want %d", spent, tt.points)
			}
			if member.Speed != tt.wantSpeed || member.Endurance != tt.wantEndurance ||
				member.Intellect != tt.wantIntellect || member.Might != tt.wantMight {
				t.Fatalf("Speed/Endurance/Intellect/Might = %d/%d/%d/%d, want %d/%d/%d/%d",
					member.Speed, member.Endurance, member.Intellect, member.Might,
					tt.wantSpeed, tt.wantEndurance, tt.wantIntellect, tt.wantMight)
			}
		})
	}
}

func TestAutoDistributeAlternatesEnduranceBeforePrimary(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name          string
		points        int
		wantEndurance int
		wantMight     int
	}{
		{"one point", 1, 28, 10},
		{"paired points", 2, 28, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := character.CreateCharacter("Auto", character.ClassKnight, cfg)
			member.Speed = autoStatSpeedTarget
			member.Endurance = autoEnduranceTarget(member.Class) - 1
			member.Might = 10
			member.FreeStatPoints = tt.points

			autoDistributeStatPoints(member, cfg)

			if member.Endurance != tt.wantEndurance || member.Might != tt.wantMight {
				t.Fatalf("endurance/might = %d/%d, want %d/%d",
					member.Endurance, member.Might, tt.wantEndurance, tt.wantMight)
			}
		})
	}
}

func TestAutoDistributeAlternatesPrimaryBeforeSecondary(t *testing.T) {
	cfg := loadTestConfig(t)
	member := character.CreateCharacter("Auto", character.ClassThief, cfg)
	member.Speed = autoStatSpeedTarget
	member.Endurance = autoEnduranceTarget(member.Class)
	member.Accuracy = 10
	member.Intellect = 10
	member.FreeStatPoints = 3

	autoDistributeStatPoints(member, cfg)

	if member.Accuracy != 12 || member.Intellect != 11 {
		t.Fatalf("accuracy/intellect = %d/%d, want 12/11", member.Accuracy, member.Intellect)
	}
}

func TestAutoDistributeUsesAvailableStatWhenOtherIsMaxed(t *testing.T) {
	cfg := loadTestConfig(t)
	member := character.CreateCharacter("Auto", character.ClassDruid, cfg)
	member.Speed = autoStatSpeedTarget
	member.Endurance = autoEnduranceTarget(member.Class)
	member.Intellect = MaxStatValue
	member.Personality = 10
	member.FreeStatPoints = 3

	if spent := autoDistributeStatPoints(member, cfg); spent != 3 {
		t.Fatalf("spent = %d, want 3", spent)
	}
	if member.Intellect != MaxStatValue || member.Personality != 13 {
		t.Fatalf("intellect/personality = %d/%d, want %d/13",
			member.Intellect, member.Personality, MaxStatValue)
	}
}

func TestAutoDistributeSecondarySoftCapsAt50WhilePrimaryClimbs(t *testing.T) {
	cfg := loadTestConfig(t)
	member := character.CreateCharacter("Auto", character.ClassDruid, cfg) // primary Intellect, secondary Personality
	member.Speed = autoStatSpeedTarget
	member.Endurance = autoEnduranceTarget(member.Class)
	member.Intellect, member.Personality = 10, 10
	member.FreeStatPoints = 120

	autoDistributeStatPoints(member, cfg)

	// 80 pts bring both to 50 (alternating); the secondary then stops at 50 and the
	// remaining 40 pts climb the primary alone (50 -> 90). Primary not yet maxed, so
	// the secondary is NOT lifted past 50.
	if member.Personality != autoSecondarySoftCap {
		t.Fatalf("secondary personality = %d, want soft cap %d", member.Personality, autoSecondarySoftCap)
	}
	if member.Intellect != 90 {
		t.Fatalf("primary intellect = %d, want 90 (climbed alone after secondary capped)", member.Intellect)
	}
	if member.FreeStatPoints != 0 {
		t.Fatalf("free points = %d, want 0", member.FreeStatPoints)
	}
}

func TestAutoDistributeLiftsSecondaryAndEnduranceOnlyAfterPrimaryMaxed(t *testing.T) {
	cfg := loadTestConfig(t)
	member := character.CreateCharacter("Auto", character.ClassDruid, cfg)
	member.Speed = autoStatSpeedTarget
	member.Endurance = autoEnduranceTarget(member.Class)
	member.Intellect, member.Personality = 10, 10
	member.FreeStatPoints = 400 // plenty to reach every priority target

	autoDistributeStatPoints(member, cfg)

	if member.Intellect != MaxStatValue {
		t.Errorf("primary intellect = %d, want %d", member.Intellect, MaxStatValue)
	}
	if member.Personality != MaxStatValue {
		t.Errorf("secondary personality = %d, want %d (lifted past 50 after primary maxed)", member.Personality, MaxStatValue)
	}
	if member.Endurance != MaxStatValue {
		t.Errorf("endurance = %d, want %d", member.Endurance, MaxStatValue)
	}
	// The button must never strand points.
	if member.FreeStatPoints != 0 {
		t.Errorf("free points = %d, want 0 (AUTO must spend everything)", member.FreeStatPoints)
	}
}

func TestAutoDistributeNeverStrandsPoints(t *testing.T) {
	cfg := loadTestConfig(t)
	// A no-secondary class with an absurd surplus: every priority target maxes out,
	// the catch-all must still drain the rest into other stats.
	member := character.CreateCharacter("Auto", character.ClassKnight, cfg)
	member.FreeStatPoints = 600

	autoDistributeStatPoints(member, cfg)

	if member.FreeStatPoints != 0 {
		t.Fatalf("free points = %d, want 0 (AUTO left points unspent)", member.FreeStatPoints)
	}
}

func TestAutoDistributeStopsAtSpeedBeforeEndurance(t *testing.T) {
	cfg := loadTestConfig(t)
	member := character.CreateCharacter("Auto", character.ClassKnight, cfg)
	member.Speed = 10
	member.Endurance = 10
	member.FreeStatPoints = 4

	autoDistributeStatPoints(member, cfg)

	if member.Speed != 14 || member.Endurance != 10 {
		t.Fatalf("speed/endurance = %d/%d, want 14/10", member.Speed, member.Endurance)
	}
}

func TestAutoDistributePartySpendsEveryMembersPoints(t *testing.T) {
	cfg := loadTestConfig(t)
	knight := character.CreateCharacter("Knight", character.ClassKnight, cfg)
	sorcerer := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cfg)
	knight.FreeStatPoints = 5
	sorcerer.FreeStatPoints = 5
	knight.Speed, sorcerer.Speed = 10, 10

	if spent := autoDistributePartyStatPoints([]*character.MMCharacter{knight, sorcerer}, cfg); spent != 10 {
		t.Fatalf("party AUTO spent %d, want 10", spent)
	}
	if knight.FreeStatPoints != 0 || sorcerer.FreeStatPoints != 0 {
		t.Fatalf("free points remain: knight=%d sorcerer=%d", knight.FreeStatPoints, sorcerer.FreeStatPoints)
	}
}
