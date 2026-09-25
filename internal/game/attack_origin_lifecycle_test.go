package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestBossOriginKeepsLifecycle(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"dormant", "evasive", "enrage", "inferno", "volley"} {
			for _, origin := range []string{"moss_rock", "water", "dragon_cliffs_chasm_floor"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, kind, origin), func(t *testing.T) {
					g, _ := newSpecialsTestGame(t)
					g.turnBasedMode = tb
					m := spawnSpecialsMonster(g, "dragon_green", 5, 2)
					m.Boss = true
					tile, ok := world.GlobalTileManager.GetTileTypeFromKey(origin)
					if !ok {
						t.Fatal(origin)
					}
					g.world.Tiles[2][5] = tile
					m.Flying = true
					m.BossCD, m.InfernoCDFrames = 2, 2
					m.TrapVolleyCDFrames, m.TrapVolleyTurnCD = 2, 2
					blocked := origin == "moss_rock"
					switch kind {
					case "dormant", "evasive":
						m.PassiveUntilQuest = "uncompleted_gate"
						if kind == "evasive" {
							m.EvadeRadiusTiles = 10
						}
					case "enrage":
						m.EnrageAtHP = 50
						m.HitPoints = 20
					case "inferno":
						m.InfernoChance = 1
						m.InfernoRangeTiles = 10
						m.InfernoDamage = 1
					case "volley":
						m.TrapVolleyCount = 8
						m.TrapVolleyRadiusTiles = 3
					}
					hp := partyHPSum(g)
					handled := g.combat.runBossSpecials(m, true, tb)
					if m.BossCD != 1 {
						t.Fatal("terrain stopped boss clock")
					}
					if !tb && m.InfernoCDFrames == 2 {
						t.Fatal("terrain stopped inferno clock")
					}
					if (kind == "dormant" || kind == "evasive") && !handled {
						t.Fatal("quest gate fell through to regular attacks")
					}
					if kind == "enrage" && !m.Enraged {
						t.Fatal("terrain suppressed passive enrage")
					}
					if kind == "volley" {
						if (tb && m.TrapVolleyTurnCD != 1) || (!tb && m.TrapVolleyCDFrames != 1) {
							t.Fatal("terrain stopped volley clock")
						}
						g.combat.runBossSpecials(m, true, tb)
						if (len(g.bossFireTraps) > 0) == blocked {
							t.Fatal("volley did not obey attack origin")
						}
					}
					if kind == "inferno" && (partyHPSum(g) < hp) == blocked {
						t.Fatal("inferno did not obey attack origin")
					}
				})
			}
		}
	}
}

func TestPounceOriginThroughCombatLoops(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, origin := range []string{"moss_rock", "water", "dragon_cliffs_chasm_floor"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, origin), func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				ts := float64(g.config.GetTileSize())
				g.turnBasedMode = tb
				placePlayerAtTile(g, 5, 6, ts)
				m := monster.NewMonster3DFromConfig(8.5*ts, 6.5*ts, "puma", g.config)
				m.WasAttacked = true
				m.BeginPlayerEngagement()
				m.State = monster.StatePursuing
				m.Flying = true
				tile, ok := world.GlobalTileManager.GetTileTypeFromKey(origin)
				if !ok {
					t.Fatal(origin)
				}
				g.world.Tiles[6][8] = tile
				g.world.Monsters = []*monster.Monster3D{m}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				if tb {
					g.refreshMonsterAIState()
					runOneMonsterTurn(g, gl)
				} else {
					g.combat.HandleMonsterInteractions()
				}
				pounced := m.PounceCDFrames > 0 || m.PounceCDTurns > 0
				if pounced != (origin != "moss_rock") {
					t.Fatalf("pounced=%v", pounced)
				}
			})
		}
	}
}

func TestPointerAimNeverRestoresCamera(t *testing.T) {
	for _, change := range []string{"none", "turn", "teleport", "nested"} {
		t.Run(change, func(t *testing.T) {
			g, _, _, m, _ := mouseCombatHarness(t, false)
			facing := g.cameraPose()
			restore := g.combat.beginPartyTargetAim(m)
			if g.cameraPose() != facing {
				t.Fatal("aim mutated camera")
			}
			want := math.Atan2(m.Y-g.camera.Y, m.X-g.camera.X)
			if g.combat.partyAttackAngle() != want {
				t.Fatal("attack lost explicit direction")
			}
			switch change {
			case "turn":
				g.snapFacing(math.Pi)
			case "teleport":
				g.setPartyPosition(g.camera.X+64, g.camera.Y+64)
			case "nested":
				other := *m
				other.Y += 64
				restoreInner := g.combat.beginPartyTargetAim(&other)
				restoreInner()
				if g.combat.partyAttackAngle() != want {
					t.Fatal("nested aim lost outer direction")
				}
			}
			after := g.cameraPose()
			restore()
			if g.cameraPose() != after || g.combat.partyAimTarget != nil {
				t.Fatal("aim restoration undid view change or leaked target")
			}
		})
	}
}

func TestReactionSourceSurvivesGameplayResets(t *testing.T) {
	for _, event := range []string{"mode", "world", "load", "reset"} {
		t.Run(event, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			calls := 0
			g.combat.reactionRoll = func() float64 { calls++; return 0.123 }
			switch event {
			case "mode":
				g.ToggleTurnBasedMode()
				g.ToggleTurnBasedMode()
			case "world":
				g.tactics.world = nil
				g.updateTacticalClocks()
			case "load":
				save := g.buildSave(wm)
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
			case "reset":
				g.resetOverwatch()
			}
			if g.combat.reactionRoll == nil || g.combat.reactionRoll() != 0.123 || calls != 1 {
				t.Fatal("gameplay reset replaced reaction source")
			}
		})
	}
}
