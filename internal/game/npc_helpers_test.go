package game

import (
	"testing"
	"ugataima/internal/character"
)

// Buying a spell requires ONLY an open school (user rule): availability is
// controlled by the trader's price and stock, not by character level or mastery.
func TestCanCharacterLearnNPCSpell_NeedsOpenSchoolOnly(t *testing.T) {
	spell := &character.NPCSpell{School: "water"}
	char := &character.MMCharacter{
		Level: 1,
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
			character.MagicSchoolWater: {Mastery: character.MasteryNovice},
		},
	}

	// Level 1 with Novice water still qualifies - no level or mastery gate.
	if !canCharacterLearnNPCSpell(char, spell) {
		t.Fatal("an open school at Novice must be enough to learn a water spell")
	}

	// A closed school is the only rejection.
	if canCharacterLearnNPCSpell(char, &character.NPCSpell{School: "fire"}) {
		t.Fatal("a character with no fire school must not be able to learn fire spells")
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
