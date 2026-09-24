package monster

import (
	"fmt"
	"math"
	"testing"
)

func TestAuditReturnHomeDetour(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		for _, query := range []bool{false, true} {
			t.Run(fmt.Sprintf("blocked%v/query%v", blocked, query), func(t *testing.T) {
				c := NewMockCollisionChecker(defaultTileSize)
				length := 30
				if blocked {
					length = 100
				}
				for y := -length; y <= length; y++ {
					c.BlockTile(20, y)
				}
				m := &Monster3D{ID: "strayed", X: 32, Y: 32, SpawnX: 40*64 + 32, SpawnY: 32, AlertRadius: 20 * 64, TetherRadius: 5 * 64, Speed: 2, HitPoints: 100, State: StatePatrolling}
				if query {
					_, _, ok := m.NextPathStepTileToAny(c, []TileCoord{{X: 40, Y: 0}}, nil)
					if ok == blocked {
						t.Fatalf("return path found=%v, barrier sealed=%v", ok, blocked)
					}
				} else {
					m.updatePatrolling(c)
					moved := m.X != 32 || m.Y != 32
					if moved == blocked {
						t.Fatalf("return movement=%v, barrier sealed=%v", moved, blocked)
					}
				}
			})
		}
	}
}

// Case table: reachable long detour preserves home; beyond-window detour or
// blocked home adopts the current position; a blocked step, a held actor, and
// read-only path queries never move the home. Persistence is tested in game.
func TestPatrolHomeFallback(t *testing.T) {
	for _, kind := range []string{"long_detour", "outside_window", "blocked_home", "blocked_step", "zero_speed", "rooted"} {
		t.Run(kind, func(t *testing.T) {
			checker := NewMockCollisionChecker(defaultTileSize)
			m := &Monster3D{ID: "strayed", X: 32, Y: 32, SpawnX: 40*64 + 32, SpawnY: 32, TetherRadius: 5 * 64, Speed: 2, HitPoints: 100, State: StatePatrolling}
			homeX, homeY := m.SpawnX, m.SpawnY
			var collision CollisionChecker = checker
			switch kind {
			case "long_detour", "outside_window":
				length := 30
				if kind == "outside_window" {
					length = 60
				}
				for y := -length; y <= length; y++ {
					checker.BlockTile(20, y)
				}
			case "blocked_home", "zero_speed", "rooted":
				checker.BlockTile(40, 0)
			case "blocked_step":
				collision = &blockedPatrolStep{checker}
			}
			if kind == "zero_speed" {
				m.Speed = 0
			}
			if kind == "rooted" {
				m.RootFramesRemaining = 60
			}
			if m.HasPathToTile(collision, 40, 0) && kind == "outside_window" {
				t.Fatal("fixture detour must exceed search window")
			}
			if m.SpawnX != homeX || m.SpawnY != homeY {
				t.Fatal("path query mutated patrol home")
			}
			m.updatePatrolling(collision)
			rebase := kind == "outside_window" || kind == "blocked_home"
			if rebase {
				if m.SpawnX != m.X || m.SpawnY != m.Y || m.SpawnX == homeX {
					t.Fatal("failed return search did not adopt current refuge")
				}
				if len(m.PathTiles) != 0 || m.HasMoveTarget {
					t.Fatal("failed home route remained cached")
				}
				beforeX, beforeY := m.X, m.Y
				// A deterministic local patrol goal proves the actor can resume roaming.
				m.setMoveTarget(StatePatrolling, 1, 0)
				for tick := 0; tick < 10; tick++ {
					m.StateTimer++
					m.updatePatrolling(collision)
				}
				if m.X == beforeX && m.Y == beforeY {
					t.Fatal("rebased monster remained stuck")
				}
			} else if m.SpawnX != homeX || m.SpawnY != homeY {
				t.Fatal("reachable or temporarily held monster lost its home")
			}
		})
	}
}

type blockedPatrolStep struct{ *MockCollisionChecker }

func (c *blockedPatrolStep) CanMoveToWithTileOverrides(id string, x, y float64, overrides []string, flying bool) bool {
	if math.Mod(x, defaultTileSize) != defaultTileSize/2 || math.Mod(y, defaultTileSize) != defaultTileSize/2 {
		return false
	}
	return c.MockCollisionChecker.CanMoveToWithTileOverrides(id, x, y, overrides, flying)
}

func TestPatrolHomeTemporaryBlockRetries(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprint(reset), func(t *testing.T) {
			c := &occupiedPatrolHome{MockCollisionChecker: NewMockCollisionChecker(defaultTileSize), occupied: true}
			m := &Monster3D{ID: "strayed", X: 32, Y: 32, SpawnX: 40*64 + 32, SpawnY: 32, TetherRadius: 5 * 64, Speed: 2, HitPoints: 100, State: StatePatrolling, StateTimer: 1}
			m.updatePatrolling(c)
			if m.SpawnX != 40*64+32 {
				t.Fatal("temporary occupant changed patrol origin")
			}
			first := c.goalChecks
			for tick := 0; tick < m.pathCheckFrequency()-1; tick++ {
				m.StateTimer++
				m.updatePatrolling(c)
			}
			if c.goalChecks != first {
				t.Fatal("blocked route searched again before retry interval")
			}
			if reset {
				m.ResetPathfinding()
			}
			c.occupied = false
			m.StateTimer++
			m.updatePatrolling(c)
			if m.X == 32 && m.Y == 32 {
				t.Fatal("cleared route did not resume after retry/cache reset")
			}
		})
	}
}

type occupiedPatrolHome struct {
	*MockCollisionChecker
	occupied   bool
	goalChecks int
}

func (c *occupiedPatrolHome) CanMoveToWithTileOverrides(id string, x, y float64, overrides []string, flying bool) bool {
	if x == 40*64+32 && y == 32 {
		c.goalChecks++
		if c.occupied {
			return false
		}
	}
	return c.MockCollisionChecker.CanMoveToWithTileOverrides(id, x, y, overrides, flying)
}
