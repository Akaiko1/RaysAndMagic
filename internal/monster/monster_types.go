package monster

import damagecalc "ugataima/internal/damage"

type MonsterState int

const (
	StateIdle MonsterState = iota
	StatePatrolling
	StatePursuing
	StateAlert
	StateAttacking
	StateFleeing
)

// DamageType remains as a compatibility alias for monster APIs. The canonical
// school type and catalog live in internal/damage.
type DamageType = damagecalc.Type

const (
	DamagePhysical = damagecalc.Physical
	DamageFire     = damagecalc.Fire
	DamageWater    = damagecalc.Water
	DamageAir      = damagecalc.Air
	DamageEarth    = damagecalc.Earth
	DamageSpirit   = damagecalc.Spirit
	DamageMind     = damagecalc.Mind
	DamageBody     = damagecalc.Body
	DamageLight    = damagecalc.Light
	DamageDark     = damagecalc.Dark
)

func DamageTypes() []DamageType {
	return damagecalc.Types()
}

func ParseDamageType(school string) (DamageType, error) {
	return damagecalc.ParseType(school)
}
