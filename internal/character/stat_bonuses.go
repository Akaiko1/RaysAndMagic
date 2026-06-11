package character

// StatBonuses is a per-stat additive bonus block — the unit of every temporary
// stat buff. Spells author either a uniform `stat_bonus: N` (all seven stats,
// e.g. Bless) or a per-stat `stat_bonuses:` map; the game aggregates active
// buffs into one StatBonuses and pushes it onto each member's BuffBonuses, so
// combat formulas AND derived stats (MaxHP/MaxSP) see the same numbers.
type StatBonuses struct {
	Might       int
	Intellect   int
	Personality int
	Endurance   int
	Accuracy    int
	Speed       int
	Luck        int
}

// UniformStatBonuses returns +n to all seven stats (the classic Bless shape).
func UniformStatBonuses(n int) StatBonuses {
	return StatBonuses{n, n, n, n, n, n, n}
}

// Add returns the per-stat sum of two bonus blocks.
func (b StatBonuses) Add(o StatBonuses) StatBonuses {
	return StatBonuses{
		b.Might + o.Might,
		b.Intellect + o.Intellect,
		b.Personality + o.Personality,
		b.Endurance + o.Endurance,
		b.Accuracy + o.Accuracy,
		b.Speed + o.Speed,
		b.Luck + o.Luck,
	}
}

// IsZero reports whether the block carries no bonus at all.
func (b StatBonuses) IsZero() bool {
	return b == StatBonuses{}
}

// StatBonusesFromMap builds a StatBonuses from a lowercase stat-name map (the
// spells.yaml `stat_bonuses:` authoring shape). Unknown keys are rejected at
// spell-config load; here they are simply ignored.
func StatBonusesFromMap(m map[string]int) StatBonuses {
	return StatBonuses{
		Might:       m["might"],
		Intellect:   m["intellect"],
		Personality: m["personality"],
		Endurance:   m["endurance"],
		Accuracy:    m["accuracy"],
		Speed:       m["speed"],
		Luck:        m["luck"],
	}
}

// ValueByName returns the bonus for a canonical lowercase stat name
// (config.StatNames). The inverse of StatBonusesFromMap — the two are the only
// name→field mappings; keep them in sync when adding a stat.
func (b StatBonuses) ValueByName(name string) int {
	switch name {
	case "might":
		return b.Might
	case "intellect":
		return b.Intellect
	case "personality":
		return b.Personality
	case "endurance":
		return b.Endurance
	case "accuracy":
		return b.Accuracy
	case "speed":
		return b.Speed
	case "luck":
		return b.Luck
	}
	return 0
}
