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

// TileFan spreads monsters that share a tile in the picture only. Index of
// Count is the occupant's slot this tick and X, Y the drawn offset (world
// units) easing toward its place. A Combat fan keeps every occupant inside
// its tile and never draws one closer to the camera than it stands.
type TileFan struct {
	Index, Count   int
	Active, Combat bool
	X, Y           float64
}
