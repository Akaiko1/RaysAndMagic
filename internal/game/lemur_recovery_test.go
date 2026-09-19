package game

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Rule: a canopy actor's position, elevation and trajectory remain coherent.
// Cases cross both species and RT/TB; every phase is held by Charm, persisted,
// and resumed. Invalid terrain recovers inside the population bounds, while
// an unavailable landing waits and retries. Ground Charm keeps its old policy.
func advanceLemurThroughScheduler(g *MMGame, m *monster.Monster3D, turn bool) {
	g.frameCount++
	g.refreshMonsterAIState()
	if turn {
		runOneMonsterTurn(g, &GameLoop{game: g})
	} else {
		w := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
		w.Update()
		w.ApplyCollisionUpdate()
	}
}
func restoreLemurForTest(t *testing.T, g *MMGame) *monster.Monster3D {
	t.Helper()
	data, err := json.Marshal(g.buildSave(world.GlobalWorldManager))
	if err != nil {
		t.Fatal(err)
	}
	var saved GameSave
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if err := g.applySave(world.GlobalWorldManager, &saved); err != nil {
		t.Fatal(err)
	}
	return g.world.Monsters[0]
}
func TestLemurCharmTrajectoryLifecycle(t *testing.T) {
	for _, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			for _, phase := range []string{"climbing", "perched", "jumping", "descending"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, turn, phase), func(t *testing.T) {
					g, m, _ := lemurTestGame(t, key)
					for i := 0; i < 2400 && m.Arbor.Phase != phase; i++ {
						stepLemur(g, m, false)
					}
					if m.Arbor.Phase != phase {
						t.Fatal("phase not reached")
					}
					// Move a little into transitions, rather than checking only their edge.
					if phase != "perched" {
						stepLemur(g, m, false)
					}
					g.turnBasedMode = turn
					g.combat.applyPacify(m, 120, "Charm")
					before, x, y := m.Arbor, m.X, m.Y
					for _, saved := range []bool{false, true} {
						if saved {
							m = restoreLemurForTest(t, g)
						}
						for i := 0; i < 240; i++ {
							advanceLemurThroughScheduler(g, m, turn)
						}
						if m.Arbor != before || m.X != x || m.Y != y {
							t.Fatalf("save=%v: Charm split trajectory and position: %+v (%f,%f)", saved, m.Arbor, m.X, m.Y)
						}
					}
					g.combat.breakPacifyOnHit(m)
					advanceLemurThroughScheduler(g, m, turn)
					if !turn && math.Hypot(m.X-x, m.Y-y) > g.config.GetTileSize()/8 {
						t.Fatal("Charm release teleported the actor")
					}
					for i := 0; i < 2400 && m.Arbor.Phase != ""; i++ {
						advanceLemurThroughScheduler(g, m, turn)
					}
					if m.Arbor.Phase != "" || m.Arbor.Height != 0 || !g.collisionSystem.CanMoveTo(m.ID, m.X, m.Y) {
						t.Fatal("resumed trajectory never reached legal ground")
					}
				})
			}
		}
	}
}
func TestLemurInvalidTerrainRecovery(t *testing.T) {
	for _, key := range []string{"ring_tailed_lemur", "red_ruffed_lemur"} {
		for _, turn := range []bool{false, true} {
			for _, scenario := range []string{"live_edit", "saved_edit", "no_landing", "rooted"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, turn, scenario), func(t *testing.T) {
					g, m, tile := lemurTestGame(t, key)
					for i := 0; i < 400 && m.Arbor.Phase != "perched"; i++ {
						stepLemur(g, m, false)
					}
					if m.Arbor.Phase != "perched" {
						t.Fatal("not perched")
					}
					g.turnBasedMode = turn
					g.world.Tiles[10][12] = world.TileWall
					if scenario == "saved_edit" {
						m = restoreLemurForTest(t, g)
					}
					// The spawn and an otherwise free southern neighbor are outside this
					// population. Recovery must not use either as an escape hatch.
					m.AmbientBounds = &[4]int{12, 10, 14, 11}
					m.Population = ""
					original := m.Arbor
					x, y := m.X, m.Y
					if scenario == "no_landing" {
						g.world.Tiles[10][13] = world.TileWall
					}
					if scenario == "rooted" {
						m.RootFramesRemaining, m.RootTurnsRemaining = 10000, 10000
					}
					advanceLemurThroughScheduler(g, m, turn)
					if scenario == "no_landing" || scenario == "rooted" {
						if m.X != x || m.Y != y || m.Arbor != original {
							t.Fatal("unavailable/held recovery moved actor")
						}
						g.world.Tiles[10][13] = g.world.Tiles[9][13]
						m.RootFramesRemaining, m.RootTurnsRemaining = 0, 0
						// A new frame/turn clears the corresponding movement latch.
						for i := 0; i < 4 && m.Arbor.Phase != ""; i++ {
							advanceLemurThroughScheduler(g, m, turn)
						}
					}
					if m.Arbor.Phase != "" || m.Arbor.Height != 0 || !g.collisionSystem.CanMoveTo(m.ID, m.X, m.Y) {
						t.Fatalf("recovery failed: %+v", m.Arbor)
					}
					if m.X < 12*tile || m.X >= 14*tile || m.Y < 10*tile || m.Y >= 11*tile {
						t.Fatal("recovery escaped population bounds")
					}
				})
			}
		}
	}
}
