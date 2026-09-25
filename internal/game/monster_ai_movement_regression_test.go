package game

import (
	"fmt"
	"testing"
	"ugataima/internal/monster"
)

// Matrix: voluntary locomotion x RT/TB x restrictions. Final-tick CC cases
// exercise the scheduler latch, rather than only testing a positive counter.
func TestAIMovementRestrictions(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, action := range []string{"pursue", "escort", "flee"} {
			for _, restriction := range []string{"none", "stationary", "root", "stun", "slow"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, action, restriction), func(t *testing.T) {
					g, gl, tile := tbBehaviorGame(t, 40, 40)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 10, 10, tile)
					m := monster.NewMonster3DFromConfig(15.5*tile, 10.5*tile, "kasa_obake", g.config)
					m.WasAttacked = true
					m.BeginPlayerEngagement()
					switch action {
					case "escort":
						m.Bound = true
					case "flee":
						m.State = monster.StateFleeing
					}
					switch restriction {
					case "stationary":
						m.Speed = 0
					case "root":
						g.combat.applyMonsterRoot(m, 1, 1)
					case "stun":
						g.combat.applyStunDR(m, 1, 1, false)
					case "slow":
						m.ApplySlow(100, 2*g.config.GetTPS(), 2)
					}
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					g.refreshMonsterAIState()
					x, y := m.X, m.Y
					if tb {
						runOneMonsterTurn(g, gl)
					} else {
						wr := CreateMonsterWrapper(m, g.collisionSystem, g.collisionSystem.Snapshot(), g)
						wr.Update()
						wr.ApplyCollisionUpdate()
					}
					moved := m.X != x || m.Y != y
					if restriction != "none" && moved {
						t.Fatalf("%s moved despite %s", action, restriction)
					}
					if action == "flee" && restriction == "none" && !moved {
						t.Fatal("unrestricted fleeing must still move")
					}
					if tb && action == "flee" && restriction != "stun" && m.StateTimer != g.config.GetTPS() {
						t.Fatalf("flee clock advanced %d, want one turn", m.StateTimer)
					}
				})
			}
		}
	}
}

func TestAISocialMovementRestrictions(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, action := range []string{"stack", "guard_stack", "hit_scatter", "death_scatter", "calm_separation"} {
			if action == "calm_separation" && !tb {
				continue
			}
			for _, restriction := range []string{"stationary", "stun", "root", "slow"} {
				for _, pinnedLeader := range []bool{false, true} {
					t.Run(fmt.Sprintf("TB=%v/%s/%s/leader=%v", tb, action, restriction, pinnedLeader), func(t *testing.T) {
						g, gl, tile := tbBehaviorGame(t, 40, 40)
						g.turnBasedMode = tb
						placePlayerAtTile(g, 15, 15, tile)
						a := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "karasu_tengu", g.config)
						b := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, "karasu_tengu", g.config)
						a.ID, b.ID = "a", "b"
						g.world.Monsters = []*monster.Monster3D{a, b}
						g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
						gl.updateMonsterBands()
						pinned := b
						if pinnedLeader {
							pinned = a
						}
						switch restriction {
						case "stationary":
							pinned.Speed = 0
						case "stun":
							g.combat.applyStunDR(pinned, 1, 1, false)
						case "root":
							g.combat.applyMonsterRoot(pinned, 1, 1)
						case "slow":
							pinned.ApplySlow(100, 2*g.config.GetTPS(), 2)
						}
						x, y := pinned.X, pinned.Y
						if tb {
							// Reconcile between the two action passes of the final CC turn.
							g.turnBasedExtraMonsterAction = true
							runOneMonsterTurn(g, gl)
						} else {
							wrappers := g.ConvertMonstersToWrappers()
							for _, w := range wrappers {
								w.Update()
							}
							for _, w := range wrappers {
								w.(*MonsterWrapper).ApplyCollisionUpdate()
							}
						}
						switch action {
						case "stack":
							gl.updateMonsterBands()
						case "guard_stack":
							a.LootGuarding, b.LootGuarding = true, true
							gl.stackMonsterBand(1, []*monster.Monster3D{a, b})
						case "hit_scatter":
							other := a
							if pinnedLeader {
								other = b
							}
							other.WasAttacked = true
							gl.updateMonsterBands()
						case "death_scatter":
							gl.scatterBand([]*monster.Monster3D{pinned}, []*monster.Monster3D{a, b}, tile, true)
						case "calm_separation":
							a.Banding, b.Banding = false, false
							leaveBand(a)
							leaveBand(b)
							g.separateStackedMonstersTB()
						}
						if pinned.X != x || pinned.Y != y {
							t.Fatalf("%s moved pinned actor: (%v,%v) -> (%v,%v)", action, x, y, pinned.X, pinned.Y)
						}
					})
				}
			}
		}
	}
}

// A full slow also hits the final position gate. Partial slow must pass through
// the action-level roll, including fleeing; it cannot be validated by slow=100.
func TestAIFleePartialSlowUsesMovementRoll(t *testing.T) {
	g, gl, tile := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(g, 10, 10, tile)
	m := monster.NewMonster3DFromConfig(12.5*tile, 10.5*tile, "kasa_obake", g.config)
	m.WasAttacked = true
	g.world.Monsters = []*monster.Monster3D{m}
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	m.ApplySlow(50, 10000*g.config.GetTPS(), 10000)
	moves := 0
	const trials = 512
	for range trials {
		m.X, m.Y = 12.5*tile, 10.5*tile
		m.State = monster.StateFleeing
		m.StateTimer = 0
		m.ResetPathfinding()
		g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		g.refreshMonsterAIState()
		x, y := m.X, m.Y
		runOneMonsterTurn(g, gl)
		if m.X != x || m.Y != y {
			moves++
		}
	}
	// Wide interval avoids timing/PRNG assumptions while rejecting either a
	// missing roll (512 moves) or a duplicated roll (about 128 moves).
	if moves < 185 || moves > 327 {
		t.Fatalf("50%% slow allowed %d/%d flee steps, want about half", moves, trials)
	}
}

func TestAIStationaryMonsterCanStillAttack(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, gl, tile := tbBehaviorGame(t, 40, 40)
			g.turnBasedMode = tb
			placePlayerAtTile(g, 15, 10, tile)
			m := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "dragon_brood_mother", g.config)
			m.SummonChance = 0
			m.InfernoChance = 0
			m.TrapVolleyCount = 0
			m.SummonFirstGuaranteed = false
			m.DragonBreathChance = 0
			m.WasAttacked = true
			m.BeginPlayerEngagement()
			g.world.Monsters = []*monster.Monster3D{m}
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			g.refreshMonsterAIState()
			if tb {
				runOneMonsterTurn(g, gl)
			} else {
				m.State = monster.StateAttacking
				m.StateTimer = 1
				gl.reconcileMonsterAttackPosts()
				g.combat.HandleMonsterInteractions()
			}
			if len(g.magicProjectiles) == 0 {
				t.Fatal("movement restriction disabled stationary ranged attack")
			}
		})
	}
}
