package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
)

// A fleeing monster only flees: a summon in reach does not pull it back into a
// fight, and neither does its band's alarm. A relentless hunter never breaks
// off. Rows run through the real monster frame, RT and TB.
func TestFleeOutranksFoeAndBand(t *testing.T) {
	cases := []struct {
		name                string
		fleeing, relentless bool
		summon              bool // a bound ally stands beside the bandmate
		wantMode            monster.AIBehaviorMode
		wantFoe             bool
	}{
		{"fighting bandmate turns on the summon", false, false, true, monster.AIBehaviorFightFoe, true},
		{"fleeing bandmate ignores the band", true, false, false, monster.AIBehaviorFleeing, false},
		{"fleeing bandmate ignores the summon", true, false, true, monster.AIBehaviorFleeing, false},
		{"relentless bandmate fights on", true, true, true, monster.AIBehaviorFightFoe, true},
	}
	for _, tb := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("TB%v/%s", tb, tc.name), func(t *testing.T) {
				g, gl, tile := tbBehaviorGame(t, 40, 40)
				g.turnBasedMode = tb
				g.threading = threading.NewThreadingComponents(g.config)
				t.Cleanup(g.threading.Shutdown)
				placePlayerAtTile(g, 20, 10, tile)
				guardian := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "bronze_gatekeeper", g.config)
				mate := monster.NewMonster3DFromConfig(10.6*tile, 10.5*tile, "gale_novice", g.config)
				guardian.ID, mate.ID = "guardian", "mate"
				g.world.Monsters = []*monster.Monster3D{guardian, mate}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				frame := func() {
					if tb {
						g.currentTurn, g.monsterTurnResolved = 1, false
						g.turnBasedMonsterPassDelay, g.turnBasedMonsterPassesLeft = 0, 0
					}
					gl.runMonsterFrame()
				}

				// The party hits the band: it scatters and every member fights.
				gl.updateMonsterBands()
				g.combat.applyMonsterDamagePacket(guardian, singleMonsterDamagePacket(damagecalc.Parts{Normal: 1}, "physical", 0), monsterDamageOptions{IgnoreArmor: true})
				frame()
				if !mate.IsEngagingPlayer || !mate.WasAttacked {
					t.Fatal("setup: the band's hit did not rouse the bandmate")
				}

				// The bandmate breaks off the way the AI does after its attacks.
				if tc.fleeing {
					mate.State, mate.StateTimer, mate.IsEngagingPlayer = monster.StateFleeing, 0, false
				}
				mate.Relentless = tc.relentless
				var summon *monster.Monster3D
				summonHP := 0
				if tc.summon {
					summon = monster.NewMonster3DFromConfig(mate.X+tile, mate.Y, "bandit", g.config)
					summon.ID, summon.Bound = "summon", true
					summon.StunFramesRemaining = 1 << 20 // only the bandmate acts
					summonHP = summon.HitPoints
					g.world.Monsters = append(g.world.Monsters, summon)
					w, h := summon.GetSize()
					g.collisionSystem.RegisterEntity(collision.NewEntity(summon.ID, summon.X, summon.Y, w, h, collision.CollisionTypeMonster, false))
				}
				frame()

				if got := mate.CurrentAIBehavior(); got != tc.wantMode {
					t.Fatalf("mode = %v, want %v", got, tc.wantMode)
				}
				if got := summon != nil && mate.AIFoe == summon; got != tc.wantFoe {
					t.Fatalf("foe is the summon = %v, want %v", got, tc.wantFoe)
				}
				if tc.wantMode != monster.AIBehaviorFleeing {
					return
				}
				if mate.State != monster.StateFleeing || mate.IsEngagingPlayer {
					t.Fatalf("the flee was cut short: state %v, engaging %v", mate.State, mate.IsEngagingPlayer)
				}
				if summon != nil && summon.HitPoints != summonHP {
					t.Fatal("a fleeing bandmate struck the summon")
				}
				if !guardian.IsEngagingPlayer {
					t.Fatal("the rest of the band stood down")
				}
			})
		}
	}
}
