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

// Damage school keys are shared by YAML/config/UI boundaries. Monster damage
// converts the canonical string to DamageType before indexing resistances.
const (
	DamageSchoolPhysical = "physical"
	DamageSchoolFire     = "fire"
	DamageSchoolWater    = "water"
	DamageSchoolAir      = "air"
	DamageSchoolEarth    = "earth"
	DamageSchoolSpirit   = "spirit"
	DamageSchoolMind     = "mind"
	DamageSchoolBody     = "body"
	DamageSchoolLight    = "light"
	DamageSchoolDark     = "dark"
)

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
