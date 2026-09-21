package game

import (
	"fmt"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestTBStackSeparationPreservesTreeMotion(t *testing.T) {
	for _, phase := range []string{"", "climbing", "perched", "jumping", "descending"} {
		for _, mixed := range []bool{false, true} {
			for _, canopyFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/mixed=%v/first=%v", phase, mixed, canopyFirst), func(t *testing.T) {
					g, a, tile := lemurTestGame(t, "ring_tailed_lemur")
					g.turnBasedMode = true
					a.X, a.Y = 12.5*tile, 10.5*tile
					b := monster.NewMonster3DFromConfig(a.X, a.Y, "red_ruffed_lemur", g.config)
					a.ID, b.ID = "a", "b"
					if !canopyFirst {
						a.ID, b.ID = "b", "a"
					}
					if phase != "" {
						a.Arbor = monster.ArborealState{Phase: phase, Height: 1.15, FromX: a.X, FromY: a.Y, ToX: a.X + tile, ToY: a.Y, HoldSeconds: 5}
						if !mixed {
							b.Arbor = a.Arbor
						}
					} else {
						g.world.Tiles[10][12] = world.TileEmpty
					}
					g.world.Monsters = []*monster.Monster3D{a, b}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					beforeA, beforeB, x, y := a.Arbor, b.Arbor, a.X, a.Y
					g.separateStackedMonstersTB()
					for _, m := range []*monster.Monster3D{a, b} {
						if m.Arbor.Phase != "" && (m.X != x || m.Y != y) {
							t.Fatal("stack repair displaced a tree trajectory")
						}
					}
					if a.Arbor != beforeA || b.Arbor != beforeB {
						t.Fatal("stack repair changed tree state")
					}
					if (phase == "" || mixed) && a.X == b.X && a.Y == b.Y {
						t.Fatal("ordinary ground member did not separate")
					}
				})
			}
		}
	}
}

func TestLemurAwarenessRequiresSightAtEveryHeight(t *testing.T) {
	for _, elevated := range []bool{false, true} {
		for _, threat := range []string{"party", "predator"} {
			for _, blocked := range []bool{false, true} {
				for _, remembered := range []bool{false, true} {
					t.Run(fmt.Sprintf("up=%v/%s/wall=%v/memory=%v", elevated, threat, blocked, remembered), func(t *testing.T) {
						g, m, tile := lemurTestGame(t, "ring_tailed_lemur")
						m.X, m.Y = 12.5*tile, 10.5*tile
						if elevated {
							m.Arbor = monster.ArborealState{Phase: "perched", Height: 1.15}
						} else {
							g.world.Tiles[10][12] = world.TileEmpty
						}
						m.AlertRadius = 8 * tile
						placePlayerAtTile(g, 8, 10, tile)
						if threat == "predator" {
							predator := monster.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "fennec", g.config)
							predator.Prey = []string{m.Key}
							g.world.Monsters = append(g.world.Monsters, predator)
							placePlayerAtTile(g, 1, 1, tile)
						}
						if blocked {
							g.world.Tiles[10][10] = world.TileWall
						}
						if remembered {
							m.RememberAmbientThreat(8.5*tile, 10.5*tile)
						}
						g.prepareAmbientTarget(m)
						if want := !blocked || remembered; m.AmbientFlee != want {
							t.Fatalf("flee=%v, want %v", m.AmbientFlee, want)
						}
					})
				}
			}
		}
	}
}
