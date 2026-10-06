package monster

import "strings"

// Creature types with mechanical rules (immunities, Bind Undead, Dark Elf
// Binding). Every other type is a label.
const (
	TypeUndead   = "undead"
	TypeDragon   = "dragon"
	TypeFormless = "formless"
)

// CanonicalMonsterType is the one spelling every type comparison uses; the
// catalog stores it from load on.
func CanonicalMonsterType(t string) string { return strings.ToLower(strings.TrimSpace(t)) }
