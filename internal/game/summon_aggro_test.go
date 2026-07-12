package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// meleeMobsThatShouldChaseSummons: a plain mob, a tougher mob, and a melee boss.
// Every melee monster must approach and fight a party summon that is peppering
// it from range, even while the party stands well off - not stand and watch.
var meleeMobsThatShouldChaseSummons = []string{"goblin", "orc_hero_boss"}

// runRTFoeTicks drives the REAL real-time monster loop: refresh the AI foe/target
// cache, move each monster toward its AITarget (the wrapper's real input), then
// resolve interactions - the faithful equivalent of one game frame.
func runRTFoeTicks(g *MMGame, ticks int) {
	for i := 0; i < ticks; i++ {
		g.frameCount++
		g.refreshBoundAllyCache()
		for _, m := range g.world.Monsters {
			if m == nil || !m.IsAlive() {
				continue
			}
			m.Update(g.collisionSystem, m.AITargetX, m.AITargetY)
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			g.refreshMonsterCollisionSolidity(m, g.camera.X, g.camera.Y)
		}
		g.combat.HandleMonsterInteractions()
	}
}

func summonAggroWorld(t *testing.T, mobKey string) (*MMGame, *GameLoop, *monsterPkg.Monster3D, *monsterPkg.Monster3D) {
	t.Helper()
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 10, ts) // party far to the west, well out of the way

	mob := monsterPkg.NewMonster3DFromConfig(float64(20)*ts+ts/2, float64(10)*ts+ts/2, mobKey, game.config)
	mob.MaxHitPoints, mob.HitPoints = 4000, 4000 // survive the whole exchange
	huntress := monsterPkg.NewMonster3DFromConfig(float64(24)*ts+ts/2, float64(10)*ts+ts/2, "masked_huntress", game.config)
	huntress.MaxHitPoints, huntress.HitPoints = 4000, 4000
	markCardAlly(huntress)
	game.world.Monsters = []*monsterPkg.Monster3D{mob, huntress}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return game, gl, mob, huntress
}

// TB: every melee mob closes on the ranged summon that is shooting it.
func TestSummonDrawsMeleeMobTB(t *testing.T) {
	for _, key := range meleeMobsThatShouldChaseSummons {
		t.Run(key, func(t *testing.T) {
			game, gl, mob, huntress := summonAggroWorld(t, key)
			d0 := Distance(mob.X, mob.Y, huntress.X, huntress.Y)
			for i := 0; i < 12; i++ {
				game.refreshBoundAllyCache()
				if game.combat.monsterAIFoeMonster(mob) != huntress {
					t.Fatalf("turn %d: mob AIFoe should be the summon", i)
				}
				runOneMonsterTurn(game, gl)
			}
			if d := Distance(mob.X, mob.Y, huntress.X, huntress.Y); d >= d0 {
				t.Fatalf("melee %s did not close on the summon over 12 turns (%.0f -> %.0f)", key, d0, d)
			}
		})
	}
}

// RT: same, through the real-time loop - the mob approaches and eventually
// draws the summon's HP down (it reached and struck her).
func TestSummonDrawsMeleeMobRT(t *testing.T) {
	for _, key := range meleeMobsThatShouldChaseSummons {
		t.Run(key, func(t *testing.T) {
			game, _, mob, huntress := summonAggroWorld(t, key)
			d0 := Distance(mob.X, mob.Y, huntress.X, huntress.Y)
			hp0 := huntress.HitPoints
			runRTFoeTicks(game, 4*game.config.GetTPS()) // a few seconds
			d := Distance(mob.X, mob.Y, huntress.X, huntress.Y)
			if d >= d0 {
				t.Errorf("melee %s did not close on the summon in RT (%.0f -> %.0f)", key, d0, d)
			}
			if huntress.HitPoints >= hp0 {
				t.Errorf("melee %s never struck the summon in RT (HP %d -> %d)", key, hp0, huntress.HitPoints)
			}
		})
	}
}
