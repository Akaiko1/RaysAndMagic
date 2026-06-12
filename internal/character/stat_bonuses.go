package character

import "ugataima/internal/stats"

// StatBonuses is defined ONCE in internal/stats (struct fields = the canonical
// stat list); this alias keeps the established character-centric name.
type StatBonuses = stats.StatBonuses

// UniformStatBonuses returns +n to all seven stats (the classic Bless shape).
func UniformStatBonuses(n int) StatBonuses { return stats.Uniform(n) }

// StatBonusesFromMap builds a StatBonuses from a lowercase stat-name map (the
// spells.yaml `stat_bonuses:` authoring shape).
func StatBonusesFromMap(m map[string]int) StatBonuses { return stats.FromMap(m) }
