package game

import (
	"fmt"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// The serial resolver reads live target positions, while RT workers retain the
// immutable frame snapshot. A vacant corridor cell must not consume a TB turn.
func TestMonsterTargetPositionFreshness(t *testing.T) {
	for _, key := range []string{"skeleton", "lich"} {
		for _, bound := range []bool{false, true} {
			for _, dead := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/bound=%v/dead=%v", key, bound, dead), func(t *testing.T) {
					g, gl, ts := tbBehaviorGame(t, 12, 12)
					g.turnBasedMode = true
					placePlayerAtTile(g, 1, 5, ts)
					for y := range g.world.Tiles {
						for x := range g.world.Tiles[y] {
							if y != 5 {
								g.world.Tiles[y][x] = world.TileWall
							}
						}
					}
					m := monster.NewMonster3DFromConfig(3.5*ts, 5.5*ts, key, g.config)
					foe := monster.NewMonster3DFromConfig(4.5*ts, 5.5*ts, "goblin", g.config)
					g.world.Monsters = []*monster.Monster3D{foe, m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					m.Bound = bound
					m.BeginCombatEngagement()
					m.AIFoe = foe
					m.AITargetX, m.AITargetY = foe.X, foe.Y
					foe.State = monster.StateFleeing
					if !gl.commitMonsterMoveTB(foe, 5.5*ts, 5.5*ts) {
						t.Fatal("fixture: foe could not leave its tile")
					}
					if dead {
						foe.HitPoints = 0
					}
					// This is the same snapshot resolver the parallel wrapper calls.
					_, x, y, ok := monsterAttackTargetAt(m, g.camera.X, g.camera.Y, true)
					if !ok || x != 4.5*ts || y != 5.5*ts {
						t.Fatal("worker snapshot changed after another actor moved/died")
					}
					blocked := gl.attackTargetTile(m)
					if dead {
						if blocked != nil {
							t.Fatal("dead foe still reserves a target tile")
						}
						return
					}
					if blocked == nil || blocked.X != 5 || blocked.Y != 5 {
						t.Fatalf("serial target is stale: %+v", blocked)
					}
					if gl.commitMonsterMoveTB(m, foe.X, foe.Y) {
						t.Fatal("movement entered the live target tile")
					}
					// Force the same narrow contact reach for ranged and melee paths.
					m.RangedAttackRange = ts
					gl.monsterMoveTurnBased(m)
					if TileIndex(m.X, ts) != 4 || TileIndex(m.Y, ts) != 5 {
						t.Fatalf("vacated corridor blocked: actor at (%d,%d)", TileIndex(m.X, ts), TileIndex(m.Y, ts))
					}
				})
			}
		}
	}
}
