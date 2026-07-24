package character

import (
	"strings"

	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
)

// MagicSchoolID identifies a magic school. Values match the YAML `school` field
// (e.g. "fire", "body") so they round-trip through config without translation.
type MagicSchoolID string

const (
	// Self magic (clerics, paladins, druids).
	MagicSchoolBody   MagicSchoolID = MagicSchoolID(damagecalc.Body)
	MagicSchoolMind   MagicSchoolID = MagicSchoolID(damagecalc.Mind)
	MagicSchoolSpirit MagicSchoolID = MagicSchoolID(damagecalc.Spirit)

	// Elemental magic. Light and Dark are promotion-gated, but use the same
	// mastery damage and Elemental Mastery rules as the four common elements.
	MagicSchoolFire  MagicSchoolID = MagicSchoolID(damagecalc.Fire)
	MagicSchoolWater MagicSchoolID = MagicSchoolID(damagecalc.Water)
	MagicSchoolAir   MagicSchoolID = MagicSchoolID(damagecalc.Air)
	MagicSchoolEarth MagicSchoolID = MagicSchoolID(damagecalc.Earth)

	// Promotion-gated elemental schools. They are separated here only to
	// document availability; IsElemental applies the same combat rules.
	MagicSchoolLight MagicSchoolID = MagicSchoolID(damagecalc.Light)
	MagicSchoolDark  MagicSchoolID = MagicSchoolID(damagecalc.Dark)
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

// IsElemental reports the schools whose Grandmaster mastery damage becomes
// typed true damage and whose resistance pierce comes from Elemental Mastery.
// Shared by combat and presentation so Light/Dark cannot drift into a separate
// policy from Fire/Water/Air/Earth.
func (ms MagicSchoolID) IsElemental() bool {
	switch ms {
	case MagicSchoolFire, MagicSchoolWater, MagicSchoolAir, MagicSchoolEarth,
		MagicSchoolLight, MagicSchoolDark:
		return true
	default:
		return false
	}
}

// ParseMagicSchoolID validates a YAML/UI school through the shared damage
// catalog, excluding physical because it is not a learnable magic school.
func ParseMagicSchoolID(raw string) (MagicSchoolID, bool) {
	school, err := damagecalc.ParseType(raw)
	if err != nil || school == damagecalc.Physical {
		return "", false
	}
	return MagicSchoolID(school), true
}

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
