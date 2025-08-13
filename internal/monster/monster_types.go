package monster

// Deprecated: MonsterType3D enum system is no longer used.
// All monsters are now defined in assets/monsters.yaml and referenced by string keys.
// This enum remains only for legacy Monster3D struct Type field.
type MonsterType3D int

const (
	// Legacy enum values - no longer used in logic
	MonsterGoblin MonsterType3D = iota
	MonsterOrc
	MonsterWolf
	MonsterBear
	MonsterSpider
	MonsterSkeleton
	MonsterTroll
	MonsterDragon
	// Forest-specific monsters
	MonsterForestOrc
	MonsterDireWolf
	MonsterForestSpider
	MonsterTreeant
	MonsterPixie
)

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
