package game

import (
	"sort"
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// A fly_over band splits the map in two: a flying green dragon paths straight
// across every authored fly_over terrain, a grounded one finds no way at all.
func TestGreenDragonCanPursueAcrossChasm(t *testing.T) {
	g, ts := summonTileWorld(t)
	var flyOver []string
	for key, data := range world.GlobalTileManager.ListTiles() {
		if data != nil && data.FlyOver {
			flyOver = append(flyOver, key)
		}
	}
	sort.Strings(flyOver)
	if len(flyOver) == 0 {
		t.Fatal("no tile is authored fly_over (positive control)")
	}

	for _, key := range flyOver {
		t.Run(key, func(t *testing.T) {
			w := newTestWorldSized(g.config, 40, 40)
			chasm, _ := world.GlobalTileManager.GetTileTypeFromKey(key)
			for y := 12; y <= 14; y++ {
				for x := range w.Tiles[y] {
					w.Tiles[y][x] = chasm
				}
			}
			partyX, partyY := TileCenterFromTile(20, 16, ts)
			dragonX, dragonY := TileCenterFromTile(20, 11, ts)
			dragon := monsterPkg.NewMonster3DFromConfig(dragonX, dragonY, "dragon_green", g.config)
			if dragon == nil || !dragon.Flying {
				t.Fatal("dragon_green must load as a flying monster")
			}
			dragon.ID = "chasm_guard"
			dragon.WasAttacked = true
			dragon.BeginPlayerEngagement()

			w.Monsters = []*monsterPkg.Monster3D{dragon}
			checker := collision.NewCollisionSystem(w, ts)
			checker.RegisterEntity(collision.NewEntity("player", partyX, partyY, 16, 16, collision.CollisionTypePlayer, false))
			w.RegisterMonstersWithCollisionSystem(checker)

			nextX, nextY, ok := dragon.NextPathStepTile(checker, partyX, partyY)
			if !ok {
				t.Fatal("flying green dragon found no route across the chasm")
			}
			if nextX != 20 || nextY != 12 {
				t.Fatalf("first flight step = (%d,%d), want the direct chasm step (20,12)", nextX, nextY)
			}

			dragon.Flying = false
			dragon.ResetPathfinding()
			if x, y, ok := dragon.NextPathStepTile(checker, partyX, partyY); ok {
				t.Fatalf("grounded dragon crossed the chasm via (%d,%d)", x, y)
			}
		})
	}
}
