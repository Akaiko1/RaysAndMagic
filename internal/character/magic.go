package character

import (
	"strings"
	"ugataima/internal/spells"
)

// MagicSchoolID identifies a magic school. Values match the YAML `school` field
// (e.g. "fire", "body") so they round-trip through config without translation.
type MagicSchoolID string

const (
	// Self magic (clerics, paladins, druids).
	MagicSchoolBody   MagicSchoolID = "body"
	MagicSchoolMind   MagicSchoolID = "mind"
	MagicSchoolSpirit MagicSchoolID = "spirit"

	// Elemental magic (sorcerers, archers, druids).
	MagicSchoolFire  MagicSchoolID = "fire"
	MagicSchoolWater MagicSchoolID = "water"
	MagicSchoolAir   MagicSchoolID = "air"
	MagicSchoolEarth MagicSchoolID = "earth"

	// Greater magic (restricted classes).
	MagicSchoolLight MagicSchoolID = "light"
	MagicSchoolDark  MagicSchoolID = "dark"
)

// AllMagicSchools is the canonical, ordered list used by UI navigation and
// iteration to keep school presentation consistent across screens.
var AllMagicSchools = []MagicSchoolID{
	MagicSchoolFire,
	MagicSchoolWater,
	MagicSchoolAir,
	MagicSchoolEarth,
	MagicSchoolBody,
	MagicSchoolMind,
	MagicSchoolSpirit,
	MagicSchoolLight,
	MagicSchoolDark,
}

// String returns the raw YAML key of the school.
func (ms MagicSchoolID) String() string { return string(ms) }

// DisplayName returns the capitalized name shown in the UI, e.g. "Fire".
func (ms MagicSchoolID) DisplayName() string {
	if ms == "" {
		return "Unknown"
	}
	return strings.ToUpper(string(ms[:1])) + string(ms[1:])
}

// AvailableSpellIDs returns all spells that belong to this school.
func (ms MagicSchoolID) AvailableSpellIDs() ([]spells.SpellID, error) {
	return spells.GetSpellIDsBySchool(string(ms))
}

type MagicSkill struct {
	Mastery     SkillMastery
	KnownSpells []spells.SpellID // Dynamic - using SpellID strings for full flexibility
	CastCount   int              // Total casts in this school (for mastery progression)
}

func (ms *MagicSkill) Level() int {
	return int(ms.Mastery) + 1
}

func (ms *MagicSkill) IncreaseMastery() bool {
	if ms == nil || ms.Mastery >= MasteryGrandMaster {
		return false
	}
	ms.Mastery++
	return true
}
