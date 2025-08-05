package character

import (
	"ugataima/internal/spells"
)

// MagicSchoolID represents dynamic magic school identifiers (replaces hardcoded enum!)
type MagicSchoolID string

// Dynamic magic school ID constants (loaded from config at runtime)
const (
	// Self magic (available to clerics, paladins, druids)
	MagicSchoolBody   MagicSchoolID = "body"
	MagicSchoolMind   MagicSchoolID = "mind"
	MagicSchoolSpirit MagicSchoolID = "spirit"

	// Elemental magic (available to sorcerers, archers, druids)
	MagicSchoolFire  MagicSchoolID = "fire"
	MagicSchoolWater MagicSchoolID = "water"
	MagicSchoolAir   MagicSchoolID = "air"
	MagicSchoolEarth MagicSchoolID = "earth"

	// Greater magic (restricted classes only)
	MagicSchoolLight MagicSchoolID = "light"
	MagicSchoolDark  MagicSchoolID = "dark"
)

// Legacy MagicSchool enum for backward compatibility during migration
type MagicSchool int

const (
	MagicBody MagicSchool = iota
	MagicMind
	MagicSpirit
	MagicFire
	MagicWater
	MagicAir
	MagicEarth
	MagicLight
	MagicDark
)

// Convert legacy MagicSchool to dynamic MagicSchoolID
func MagicSchoolToID(school MagicSchool) MagicSchoolID {
	switch school {
	case MagicBody:
		return MagicSchoolBody
	case MagicMind:
		return MagicSchoolMind
	case MagicSpirit:
		return MagicSchoolSpirit
	case MagicFire:
		return MagicSchoolFire
	case MagicWater:
		return MagicSchoolWater
	case MagicAir:
		return MagicSchoolAir
	case MagicEarth:
		return MagicSchoolEarth
	case MagicLight:
		return MagicSchoolLight
	case MagicDark:
		return MagicSchoolDark
	default:
		return MagicSchoolFire // Default fallback
	}
}

// Convert dynamic MagicSchoolID to legacy MagicSchool (for compatibility)
func MagicSchoolIDToLegacy(schoolID MagicSchoolID) MagicSchool {
	switch schoolID {
	case MagicSchoolBody:
		return MagicBody
	case MagicSchoolMind:
		return MagicMind
	case MagicSchoolSpirit:
		return MagicSpirit
	case MagicSchoolFire:
		return MagicFire
	case MagicSchoolWater:
		return MagicWater
	case MagicSchoolAir:
		return MagicAir
	case MagicSchoolEarth:
		return MagicEarth
	case MagicSchoolLight:
		return MagicLight
	case MagicSchoolDark:
		return MagicDark
	default:
		return MagicFire // Default fallback
	}
}

// String returns the string representation of a magic school ID
func (ms MagicSchoolID) String() string {
	return string(ms)
}

type MagicSkill struct {
	Level       int
	Mastery     SkillMastery
	KnownSpells []spells.SpellID // Dynamic - using SpellID strings for full flexibility
}

// Legacy GetSchoolName function for backward compatibility
func (ms MagicSchool) GetSchoolName() string {
	schoolID := MagicSchoolToID(ms)
	return string(schoolID)
}

// GetAvailableSpellsForSchool returns all spell types available for this magic school (legacy)
func (ms MagicSchool) GetAvailableSpellsForSchool() []spells.SpellType {
	return spells.GetSpellsBySchool(ms.GetSchoolName())
}

// GetAvailableSpellIDsForSchool returns all spell IDs for a given magic school (dynamic version)
func (schoolID MagicSchoolID) GetAvailableSpellIDsForSchool() []spells.SpellID {
	return spells.GetSpellIDsBySchool(string(schoolID))
}

// GetAvailableSpellsForSchoolID returns spell types for school ID (compatibility bridge)
func GetAvailableSpellsForSchoolID(schoolID MagicSchoolID) []spells.SpellType {
	return spells.GetSpellsBySchool(string(schoolID))
}
