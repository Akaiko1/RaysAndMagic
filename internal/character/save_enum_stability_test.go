package character

import "testing"

// Save-format stability tests.
//
// SkillType, CharacterClass, Condition, and Promotion are all persisted in
// save files as raw ints (SkillEntry.Type, CharacterSave.Class/Conditions/
// Promotion). Renumbering an existing constant - which happens silently the
// moment a new value is inserted ANYWHERE but the end of its const block -
// reinterprets every already-saved character as having different skills,
// class, conditions, or promotion on next load. This happened once for real:
// SkillMartialArts was added between SkillStaff and SkillLeather, which
// shifted every skill after it by one and corrupted every save written
// before the fix (a Druid's Bodybuilding was read back as Shield).
//
// Each test below pins every CURRENT value of one of these types AND asserts
// the pinned set is complete (same length as the canonical list/known count).
// A pinned-value mismatch means an existing constant got renumbered - revert
// it and append the new one at the end instead. A length mismatch means a new
// value was added without pinning it here - add it at its current value so it
// can never silently move later.

func TestSkillTypeValuesAreStableForSaveCompatibility(t *testing.T) {
	want := map[SkillType]int{
		SkillSword:             0,
		SkillDagger:            1,
		SkillAxe:               2,
		SkillSpear:             3,
		SkillBow:               4,
		SkillMace:              5,
		SkillStaff:             6,
		SkillLeather:           7,
		SkillChain:             8,
		SkillPlate:             9,
		SkillShield:            10,
		SkillBodybuilding:      11,
		SkillMeditation:        12,
		SkillMerchant:          13,
		SkillRepair:            14,
		SkillIdentifyItem:      15,
		SkillDisarmTrap:        16,
		SkillLearning:          17,
		SkillArmsMaster:        18,
		SkillTrapper:           19,
		SkillSleightOfHand:     20,
		SkillDualWielding:      21,
		SkillIronBody:          22,
		SkillSpiritualTraining: 23,
		SkillMartialArts:       24,
	}
	if len(want) != len(AllSkills) {
		t.Fatalf("AllSkills has %d entries but only %d are pinned here - a skill was added without "+
			"locking its value; add it to `want` at its current numeric value", len(AllSkills), len(want))
	}
	for skill, wantVal := range want {
		if int(skill) != wantVal {
			t.Errorf("%s = %d, want %d - a save-breaking renumbering (append new skills at the "+
				"end of the const block in skills.go, never insert them)", skill.String(), int(skill), wantVal)
		}
	}
}

func TestCharacterClassValuesAreStableForSaveCompatibility(t *testing.T) {
	want := map[CharacterClass]int{
		ClassKnight:     0,
		ClassPaladin:    1,
		ClassArcher:     2,
		ClassCleric:     3,
		ClassSorcerer:   4,
		ClassDruid:      5,
		ClassThief:      6,
		ClassArmsMaster: 7,
		ClassMonk:       8,
	}
	if len(want) != len(PlayableClasses) {
		t.Fatalf("PlayableClasses has %d entries but only %d are pinned here - a class was added "+
			"without locking its value; add it to `want` at its current numeric value", len(PlayableClasses), len(want))
	}
	for class, wantVal := range want {
		if int(class) != wantVal {
			t.Errorf("%s = %d, want %d - a save-breaking renumbering (append new classes at the "+
				"end of the const block in character.go, never insert them)", class.String(), int(class), wantVal)
		}
	}
}

func TestConditionValuesAreStableForSaveCompatibility(t *testing.T) {
	want := map[Condition]int{
		ConditionNormal:      0,
		ConditionPoisoned:    1,
		ConditionDiseased:    2,
		ConditionCursed:      3,
		ConditionAsleep:      4,
		ConditionFear:        5,
		ConditionParalyzed:   6,
		ConditionUnconscious: 7,
		ConditionDead:        8,
		ConditionStone:       9,
		ConditionEradicated:  10,
		ConditionBurning:     11,
		ConditionStunned:     12,
	}
	for cond, wantVal := range want {
		if int(cond) != wantVal {
			t.Errorf("%s = %d, want %d - a save-breaking renumbering (append new conditions at the "+
				"end of the const block in conditions.go, never insert them)", cond.String(), int(cond), wantVal)
		}
	}
}

func TestPromotionValuesAreStableForSaveCompatibility(t *testing.T) {
	want := map[Promotion]int{
		PromotionNone:     0,
		PromotionArchmage: 1,
		PromotionLich:     2,
	}
	for promo, wantVal := range want {
		if int(promo) != wantVal {
			t.Errorf("promotion %d = %d, want %d - a save-breaking renumbering (append new promotions "+
				"at the end of the const block in character.go, never insert them)", promo, int(promo), wantVal)
		}
	}
}
