package game

import (
	"testing"
	"ugataima/internal/character"
)

func TestCanCharacterLearnNPCSpell(t *testing.T) {
	spell := &character.NPCSpell{
		School: "water",
		Requirements: &character.SpellRequirements{
			MinLevel:      3,
			MinWaterSkill: 2,
		},
	}
	char := &character.MMCharacter{
		Level: 3,
		MagicSchools: map[character.MagicSchool]*character.MagicSkill{
			character.MagicWater: {Level: 2},
		},
	}

	if !canCharacterLearnNPCSpell(char, spell) {
		t.Fatalf("expected character to meet requirements")
	}

	char.Level = 2
	if canCharacterLearnNPCSpell(char, spell) {
		t.Fatalf("expected min level requirement to fail")
	}

	char.Level = 3
	char.MagicSchools[character.MagicWater] = &character.MagicSkill{Level: 1}
	if canCharacterLearnNPCSpell(char, spell) {
		t.Fatalf("expected water skill requirement to fail")
	}
}
