package character

import "ugataima/internal/config"

type Condition int

const (
	ConditionNormal Condition = iota
	ConditionPoisoned
	ConditionDiseased
	ConditionCursed
	ConditionAsleep
	ConditionFear
	ConditionParalyzed
	ConditionUnconscious
	ConditionDead
	ConditionStone
	ConditionEradicated
	ConditionBurning // on fire (ignite): poison-like DoT, 3x damage, stacks with poison
	ConditionStunned // skips actions (RT frames / TB turns)
)

// String returns the display name of the condition (Stringer interface): the
// status table's name for its row.
func (c Condition) String() string {
	if c == ConditionNormal {
		return "OK"
	}
	if key := c.StatusKey(); key != "" {
		return config.Status(key).Name
	}
	return "Unknown"
}

// StatusKey is the condition's row in assets/statuses.yaml ("" for Normal).
func (c Condition) StatusKey() string {
	switch c {
	case ConditionPoisoned:
		return "poison"
	case ConditionDiseased:
		return "diseased"
	case ConditionCursed:
		return "cursed"
	case ConditionAsleep:
		return "asleep"
	case ConditionFear:
		return "fear"
	case ConditionParalyzed:
		return "paralyzed"
	case ConditionUnconscious:
		return "unconscious"
	case ConditionDead:
		return "dead"
	case ConditionStone:
		return "stone"
	case ConditionEradicated:
		return "eradicated"
	case ConditionBurning:
		return "burn"
	case ConditionStunned:
		return "stun"
	}
	return ""
}
