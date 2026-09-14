package game

import (
	"testing"
	"ugataima/internal/character"
)

// Buying a spell requires ONLY an open school (user rule): availability is
// controlled by the trader's price and stock, not by character level or mastery.
// And "the school" means ANY of the spell's schools - a catalog row carries one
// string, but Town Portal belongs to earth AND air.
//
// The row is identified by its CATALOG KEY (the spell id, validated at load),
// never by the authored display name - a renamed row must still sell.
func TestCanCharacterLearnNPCSpell_NeedsAnyOfTheSpellsSchoolsOpen(t *testing.T) {
	loadTestConfig(t)
	withSchools := func(open ...character.MagicSchoolID) *character.MMCharacter {
		char := &character.MMCharacter{Level: 1, MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{}}
		for _, s := range open {
			char.MagicSchools[s] = &character.MagicSkill{Mastery: character.MasteryNovice}
		}
		return char
	}
	// Level 1 with Novice water still qualifies - no level or mastery gate.
	if !canCharacterLearnNPCSpell(withSchools(character.MagicSchoolWater), "ice_bolt") {
		t.Fatal("an open school at Novice must be enough to learn a water spell")
	}
	// A closed school is the only rejection.
	if canCharacterLearnNPCSpell(withSchools(character.MagicSchoolWater), "fireball") {
		t.Fatal("a character with no fire school must not be able to learn fire spells")
	}

	// Town Portal is authored `schools: [earth, air]`. An AIR caster (a sorcerer
	// never opens Earth) must be able to buy it: LearnSpell files it under Air
	// without complaint, so a counter that refuses it is refusing a legal page.
	if !canCharacterLearnNPCSpell(withSchools(character.MagicSchoolAir), "town_portal") {
		t.Fatal("an Air caster must be able to buy Town Portal - the spell lists air among its schools")
	}
	if !canCharacterLearnNPCSpell(withSchools(character.MagicSchoolEarth), "town_portal") {
		t.Fatal("an Earth caster must still be able to buy Town Portal")
	}
	if canCharacterLearnNPCSpell(withSchools(character.MagicSchoolFire), "town_portal") {
		t.Fatal("a caster with neither of its schools must not be able to buy Town Portal")
	}
	// An unknown key sells nothing (and never panics).
	if canCharacterLearnNPCSpell(withSchools(character.MagicSchoolFire), "no_such_spell") {
		t.Fatal("an unknown catalog key must not resolve to a learnable spell")
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
