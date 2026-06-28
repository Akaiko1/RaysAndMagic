package character

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

// String returns the display name of the condition (Stringer interface).
func (c Condition) String() string {
	switch c {
	case ConditionNormal:
		return "OK"
	case ConditionPoisoned:
		return "Poisoned"
	case ConditionDiseased:
		return "Diseased"
	case ConditionCursed:
		return "Cursed"
	case ConditionAsleep:
		return "Asleep"
	case ConditionFear:
		return "Fear"
	case ConditionParalyzed:
		return "Paralyzed"
	case ConditionUnconscious:
		return "Unconscious"
	case ConditionDead:
		return "Dead"
	case ConditionStone:
		return "Stone"
	case ConditionEradicated:
		return "Eradicated"
	case ConditionBurning:
		return "Burning"
	case ConditionStunned:
		return "Stunned"
	default:
		return "Unknown"
	}
}
