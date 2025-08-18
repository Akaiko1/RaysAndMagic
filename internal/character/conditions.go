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
)
