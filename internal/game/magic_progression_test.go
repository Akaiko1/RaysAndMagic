package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

func TestRecordSpellCastRaisesMagicLevelAndMasteryTogether(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := &character.MMCharacter{
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
			character.MagicSchoolWater: {
				Mastery:     character.MasteryNovice,
				KnownSpells: []spells.SpellID{spells.SpellID("ice_bolt")},
			},
		},
	}

	for i := 0; i < AutoMasteryCastsPerLevel; i++ {
		cs.recordSpellCast(char, spells.SpellID("ice_bolt"))
	}

	skill := char.MagicSchools[character.MagicSchoolWater]
	if skill.Mastery != character.MasteryExpert || skill.Level() != 2 {
		t.Fatalf("expected water level 2/expert after %d casts, got level=%d mastery=%s", AutoMasteryCastsPerLevel, skill.Level(), skill.Mastery)
	}
}
