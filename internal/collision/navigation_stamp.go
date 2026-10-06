package collision

import "hash/maphash"

var navigationHashSeed = maphash.MakeSeed()

type navigationEntity struct {
	id           string
	box          BoundingBox
	kind         CollisionType
	solid, sight bool
	bounds       MovementBounds
}

// NavigationStamp observes the live collision state between update phases.
// Projectiles cannot block monster navigation and do not invalidate its preview.
// Hashing values also catches entity replacements and in-place bounds changes.
func (cs *CollisionSystem) NavigationStamp() uint64 {
	if cs == nil {
		return 0
	}
	var stamp uint64
	for _, e := range cs.entities {
		if e.BoundingBox == nil {
			continue
		}
		if e.CollisionType == CollisionTypeProjectile && !e.Solid && !e.blocksSight {
			continue
		}
		stamp ^= maphash.Comparable(navigationHashSeed, navigationEntity{e.ID, *e.BoundingBox, e.CollisionType, e.Solid, e.blocksSight, e.movementBounds})
	}
	return stamp
}
