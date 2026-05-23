package game

import (
	"testing"
	"ugataima/internal/character"
)

func TestCanCharacterLearnNPCSpell(t *testing.T) {
	spell := &character.NPCSpell{
		School: "water",
		Requirements: &character.SpellRequirements{
			MinLevel: 3,
			Schools: []character.SpellSchoolRequirement{
				{
					School:   "water",
					MinLevel: 2,
				},
			},
		},
	}
	char := &character.MMCharacter{
		Level: 3,
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
			character.MagicSchoolWater: {Mastery: character.MasteryExpert},
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
	char.MagicSchools[character.MagicSchoolWater] = &character.MagicSkill{Mastery: character.MasteryNovice}
	if canCharacterLearnNPCSpell(char, spell) {
		t.Fatalf("expected water skill requirement to fail")
	}
}
