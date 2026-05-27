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

func TestTrainerOptionsUseKnownSkillsAndNextMasteryCost(t *testing.T) {
	char := &character.MMCharacter{
		Skills: map[character.SkillType]*character.Skill{
			character.SkillSword: {Mastery: character.MasteryNovice},
			character.SkillBow:   {Mastery: character.MasteryGrandMaster},
		},
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
			character.MagicSchoolFire: {Mastery: character.MasteryExpert},
		},
	}

	options := trainerOptions(char)
	if len(options) != 2 {
		t.Fatalf("expected 2 trainable options, got %d", len(options))
	}

	if options[0].Label != "Sword" || options[0].Next != character.MasteryExpert {
		t.Fatalf("unexpected first option: %+v", options[0])
	}
	if options[0].Cost != character.TrainingCostForMastery(character.MasteryExpert) {
		t.Fatalf("unexpected sword training cost: %d", options[0].Cost)
	}

	if options[1].Label != "Fire Magic" || !options[1].IsMagic || options[1].Next != character.MasteryMaster {
		t.Fatalf("unexpected second option: %+v", options[1])
	}
}
