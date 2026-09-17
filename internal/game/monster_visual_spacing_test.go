package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/monster"
)

func TestMonsterStackCannotMoveTowardParty(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, angle := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
			for _, stack := range []string{"band", "transit"} {
				t.Run(fmt.Sprintf("tb=%v/angle=%.2f/%s", tb, angle, stack), func(t *testing.T) {
					g := deathTestGame(t)
					g.turnBasedMode = tb
					ts := g.config.GetTileSize()
					g.camera.X, g.camera.Y, g.camera.Angle = 3.5*ts, 3.5*ts, angle
					dx, dy := math.Cos(angle), math.Sin(angle)
					m := monster.NewMonster3DFromConfig(g.camera.X+dx*ts, g.camera.Y+dy*ts, "goblin", g.config)
					m.BeginPlayerEngagement()
					if stack == "transit" {
						m.TransitStackCount = 2
						m.TransitStackOffsetX = -dx * .2 * ts
						m.TransitStackOffsetY = -dy * .2 * ts
					} else {
						m.BandStackCount = 8
					}
					r := &Renderer{game: g}
					for i := 0; i < 8; i++ {
						m.BandStackIndex = i
						x, y := r.monsterVisualPosition(m)
						if (x-g.camera.X)*dx+(y-g.camera.Y)*dy < ts-1e-8 {
							t.Fatal("stack fan moved hostile closer than its attack post")
						}
						m.HitPoints = 0
						g.monsterCorpses = nil
						g.beginMonsterDeath(m)
						if len(g.monsterCorpses) != 1 {
							t.Fatal("missing death fixture")
						}
						c := g.monsterCorpses[0]
						if math.Hypot(c.x-x, c.y-y) > 1e-8 {
							t.Fatal("death capture moved the sprite toward the party")
						}
						m.HitPoints = m.MaxHitPoints
					}
				})
			}
		}
	}
}

func TestHostileApproachKeepsPartyTileClear(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"goblin", "forest_spider"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, key), func(t *testing.T) {
				g, gl, ts := tbBehaviorGame(t, 30, 30)
				g.turnBasedMode = tb
				placePlayerAtTile(g, 14, 14, ts)
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 || dx == 1 && dy == 0 {
							continue
						}
						h := hostileMonsterAt(g, 14+dx, 14+dy, ts)
						h.State = monster.StateAttacking
						g.world.Monsters = append(g.world.Monsters, h)
					}
				}
				m := monster.NewMonster3DFromConfig(13.5*ts, 14.5*ts, key, g.config)
				m.BeginPlayerEngagement()
				m.State = monster.StatePursuing
				m.AITargetX, m.AITargetY = g.camera.X, g.camera.Y
				g.world.Monsters = append(g.world.Monsters, m)
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				gl.reconcileMonsterAttackPosts()
				oldX, oldY := m.X, m.Y
				if tb && gl.commitMonsterMoveTB(m, g.camera.X, g.camera.Y) {
					t.Fatal("TB commit accepted target tile")
				}
				for i := 0; i < 120; i++ {
					if tb {
						gl.monsterMoveTurnBased(m)
					} else {
						wrapper := &MonsterWrapper{Monster: m, collisionSystem: g.collisionSystem, snapshot: g.collisionSystem.Snapshot(), frame: g.monsterFrameContext()}
						wrapper.Update()
						wrapper.ApplyCollisionUpdate()
					}
					if TileIndex(m.X, ts) == 14 && TileIndex(m.Y, ts) == 14 {
						t.Fatal("production movement entered party tile")
					}
				}
				if m.X == oldX && m.Y == oldY {
					t.Fatal("pursuer froze instead of routing to a free attack post")
				}
			})
		}
	}
}
