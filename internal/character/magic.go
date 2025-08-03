package character

type MagicSchool int

const (
	// Self magic (available to clerics, paladins, druids)
	MagicBody MagicSchool = iota
	MagicMind
	MagicSpirit

	// Elemental magic (available to sorcerers, archers, druids)
	MagicFire
	MagicWater
	MagicAir
	MagicEarth

	// Greater magic (restricted classes only)
	MagicLight
	MagicDark
)

type MagicSkill struct {
	Level   int
	Mastery SkillMastery
	Spells  []Spell // Known spells in this school
}

type Spell struct {
	Name        string
	School      MagicSchool
	Level       int // Spell level (1-9)
	SpellPoints int
	Description string
}
