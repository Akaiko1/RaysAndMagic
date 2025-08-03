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
	ConditionDead
	ConditionStone
	ConditionEradicated
)
