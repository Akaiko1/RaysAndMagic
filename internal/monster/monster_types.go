package monster

type MonsterState int

const (
	StateIdle MonsterState = iota
	StatePatrolling
	StatePursuing
	StateAlert
	StateAttacking
	StateFleeing
)

type DamageType int

const (
	DamagePhysical DamageType = iota
	DamageFire
	DamageWater
	DamageAir
	DamageEarth
	DamageSpirit
	DamageMind
	DamageBody
	DamageLight
	DamageDark
)
