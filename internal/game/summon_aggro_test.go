package game

import (
	"sort"
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
		g.refreshMonsterAIState()
		for _, m := range g.world.Monsters {
			if m == nil || !m.IsAlive() {
				continue
			}
			m.UpdateWithTarget(g.collisionSystem, g.camera.X, g.camera.Y, m.AITargetX, m.AITargetY)
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			g.refreshMonsterCollisionSolidity(m)
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
				game.refreshMonsterAIState()
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

func TestMonsterPrefersCloserPartyOrSummon(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 30, 30)
	enemy := monsterPkg.NewMonster3DFromConfig(15*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
	ally := monsterPkg.NewMonster3DFromConfig(18*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{enemy, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	placePlayerAtTile(game, 13, 10, ts) // party is 2 tiles away; ally is 3
	game.refreshMonsterAIState()
	if enemy.AIFoe != nil {
		t.Fatal("monster must prefer the closer party over a farther summon")
	}
	if enemy.AITargetX != game.camera.X || enemy.AITargetY != game.camera.Y {
		t.Fatalf("party target point = (%.0f,%.0f), want camera (%.0f,%.0f)",
			enemy.AITargetX, enemy.AITargetY, game.camera.X, game.camera.Y)
	}

	placePlayerAtTile(game, 10, 10, ts) // party is 5 tiles away; ally remains 3
	game.refreshMonsterAIState()
	if enemy.AIFoe != ally {
		t.Fatal("monster must prefer the closer summon over the party")
	}
	if enemy.AITargetX != ally.X || enemy.AITargetY != ally.Y {
		t.Fatalf("summon target point = (%.0f,%.0f), want ally (%.0f,%.0f)",
			enemy.AITargetX, enemy.AITargetY, ally.X, ally.Y)
	}
}

// Passive-until-attacked is bilateral: an unprovoked passive mob cannot choose
// a party summon, and that summon cannot silently open the fight either. The
// target cache feeds both RT workers and the TB scheduler, so exercise both
// modes and then verify a real hit restores ordinary crossfire.
func TestPassiveMonsterAndPartySummonIgnoreEachOtherUntilProvoked(t *testing.T) {
	for _, turnBased := range []bool{false, true} {
		mode := "RT"
		if turnBased {
			mode = "TB"
		}
		t.Run(mode, func(t *testing.T) {
			game, gl, ts := tbBehaviorGame(t, 40, 40)
			game.turnBasedMode = turnBased
			placePlayerAtTile(game, 5, 10, ts)
			passive := monsterPkg.NewMonster3DFromConfig(20*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
			passive.PassiveUntilAttacked = true
			passive.MaxHitPoints, passive.HitPoints = 4000, 4000
			ally := monsterPkg.NewMonster3DFromConfig(21*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
			ally.MaxHitPoints, ally.HitPoints = 4000, 4000
			markCardAlly(ally)
			game.world.Monsters = []*monsterPkg.Monster3D{passive, ally}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			passiveHP, allyHP := passive.HitPoints, ally.HitPoints
			game.refreshMonsterAIState()
			if passive.AIFoe != nil {
				t.Fatalf("unprovoked passive mob targeted summon: %v", passive.AIFoe)
			}
			if ally.AIFoe != nil {
				t.Fatalf("party summon targeted unprovoked passive mob: %v", ally.AIFoe)
			}
			if turnBased {
				runOneMonsterTurn(game, gl)
			} else {
				runRTFoeTicks(game, game.config.GetTPS())
			}
			if passive.HitPoints != passiveHP || ally.HitPoints != allyHP {
				t.Fatalf("passive/summon exchange dealt damage before a real hit: passive %d->%d ally %d->%d", passiveHP, passive.HitPoints, allyHP, ally.HitPoints)
			}

			passive.WasAttacked = true
			game.refreshMonsterAIState()
			if passive.AIFoe != ally {
				t.Fatalf("provoked mob AIFoe = %v, want party summon", passive.AIFoe)
			}
			if ally.AIFoe != passive {
				t.Fatalf("party summon AIFoe = %v, want provoked mob", ally.AIFoe)
			}
		})
	}
}

// cardSummonDuelTB sets up the real TB scheduler scenario: a hostile monster
// four tiles from a card ally, while the party is well out of the fight. It
// returns the enemy and summon so callers can assert the kind of attack they
// expect (a melee hit versus a launched projectile).
func cardSummonDuelTB(t *testing.T, enemyKey string) (*MMGame, *GameLoop, *monsterPkg.Monster3D, *monsterPkg.Monster3D) {
	t.Helper()
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 10, ts)
	enemy := monsterPkg.NewMonster3DFromConfig(20*ts+ts/2, 10*ts+ts/2, enemyKey, game.config)
	enemy.MaxHitPoints, enemy.HitPoints = 5000, 5000
	ally := monsterPkg.NewMonster3DFromConfig(24*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	ally.MaxHitPoints, ally.HitPoints = 5000, 5000
	markCardAlly(ally)
	game.world.Monsters = []*monsterPkg.Monster3D{enemy, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return game, gl, enemy, ally
}

func driveCardSummonDuelTB(t *testing.T, game *MMGame, gl *GameLoop, enemy, ally *monsterPkg.Monster3D, turns int) {
	t.Helper()
	for turn := 0; turn < turns; turn++ {
		game.refreshMonsterAIState()
		if enemy.AIFoe != ally {
			t.Fatalf("turn %d: %s AIFoe = %v, want card summon", turn, enemy.Name, enemy.AIFoe)
		}
		runOneMonsterTurn(game, gl)
	}
}

// Both ordinary attack delivery paths must be drawn to, and fire on, a card
// summon: a melee mob closes and lands a blow; a ranged mob launches a real
// crossfire projectile instead of continuing to target the party.
func TestOrdinaryMeleeAndRangedMobsFightCardSummonsTB(t *testing.T) {
	for _, tc := range []struct {
		key    string
		ranged bool
	}{
		{key: "goblin"},
		{key: "masked_huntress", ranged: true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			game, gl, enemy, ally := cardSummonDuelTB(t, tc.key)
			d0, hp0 := Distance(enemy.X, enemy.Y, ally.X, ally.Y), ally.HitPoints
			driveCardSummonDuelTB(t, game, gl, enemy, ally, 8)
			if d := Distance(enemy.X, enemy.Y, ally.X, ally.Y); !tc.ranged && d >= d0 {
				t.Fatalf("%s did not close on card summon (%.0f -> %.0f)", enemy.Name, d0, d)
			}
			if tc.ranged {
				if len(game.arrows) == 0 && len(game.magicProjectiles) == 0 {
					t.Fatalf("ranged %s never launched a crossfire projectile", enemy.Name)
				}
			} else if ally.HitPoints >= hp0 {
				t.Fatalf("melee %s never struck card summon (HP %d -> %d)", enemy.Name, hp0, ally.HitPoints)
			}
		})
	}
}

// Every YAML boss must retaliate against card summons once it is active. The
// test derives the roster through the explicit YAML boss flag, so new boss definitions cannot be
// omitted accidentally. Quest-sealed bosses are explicitly activated here:
// their sealed, inert pre-quest state is intentional and covered elsewhere.
func TestEveryActiveBossFightsCardSummonsTB(t *testing.T) {
	for _, key := range activeBossKeys(t) {
		t.Run(key, func(t *testing.T) {
			game, gl, enemy, ally := cardSummonDuelTB(t, key)
			// Golden Thief Bug and Samurai Warlord are deliberately inert before
			// their quests complete. This verifies their active combat behavior.
			enemy.PassiveUntilQuest = ""
			d0, hp0 := Distance(enemy.X, enemy.Y, ally.X, ally.Y), ally.HitPoints
			driveCardSummonDuelTB(t, game, gl, enemy, ally, 10)
			if d := Distance(enemy.X, enemy.Y, ally.X, ally.Y); d >= d0 {
				t.Fatalf("boss %s did not close on card summon (%.0f -> %.0f)", enemy.Name, d0, d)
			}
			if ally.HitPoints >= hp0 {
				t.Fatalf("boss %s never struck card summon (HP %d -> %d)", enemy.Name, hp0, ally.HitPoints)
			}
		})
	}
}

// activeBossKeys derives the boss roster from the explicit YAML classification
// rather than a copied content list. Any future YAML boss is covered
// by both the TB and RT summon-aggression matrices below.
func activeBossKeys(t *testing.T) []string {
	t.Helper()
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	probe, _, _ := tbBehaviorGame(t, 5, 5)
	keys := monsterPkg.MonsterConfig.GetAllMonsterKeys()
	sort.Strings(keys)
	bossKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if monsterPkg.NewMonster3DFromConfig(0, 0, key, probe.config).IsBoss() {
			bossKeys = append(bossKeys, key)
		}
	}
	return bossKeys
}

// The current arena duel is real-time. Verify the real-time movement and
// cadence path for ordinary melee/ranged mobs too, not only the TB scheduler.
func TestOrdinaryMeleeAndRangedMobsFightCardSummonsRT(t *testing.T) {
	for _, tc := range []struct {
		key    string
		ranged bool
	}{
		{key: "goblin"},
		{key: "masked_huntress", ranged: true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			game, _, enemy, ally := cardSummonDuelTB(t, tc.key)
			game.turnBasedMode = false
			d0, hp0 := Distance(enemy.X, enemy.Y, ally.X, ally.Y), ally.HitPoints
			runRTFoeTicks(game, 6*game.config.GetTPS())
			if d := Distance(enemy.X, enemy.Y, ally.X, ally.Y); !tc.ranged && d >= d0 {
				t.Fatalf("%s did not close on card summon in RT (%.0f -> %.0f)", enemy.Name, d0, d)
			}
			if tc.ranged {
				if len(game.arrows) == 0 && len(game.magicProjectiles) == 0 {
					t.Fatalf("ranged %s never launched a crossfire projectile in RT", enemy.Name)
				}
			} else if ally.HitPoints >= hp0 {
				t.Fatalf("melee %s never struck card summon in RT (HP %d -> %d)", enemy.Name, hp0, ally.HitPoints)
			}
		})
	}
}

// Crossfire is not a separate combat cadence. A normal enemy and a Bound
// Undead both arm their own AttackCDFrames after a hit, exactly as they do when
// fighting the party; the next frame may only tick that cooldown down.
func TestCrossfireUsesEachMonsterAttackCooldownRT(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bound bool
	}{
		{name: "enemy"},
		{name: "bound_undead", bound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			game, _, ts := tbBehaviorGame(t, 20, 20)
			game.turnBasedMode = false
			placePlayerAtTile(game, 2, 2, ts)
			attacker := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "skeleton", game.config)
			target := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
			target.MaxHitPoints, target.HitPoints = 5000, 5000
			if tc.bound {
				game.combat.applyBindUndead(attacker, 300, "Bind Undead")
			} else {
				markCardAlly(target)
			}
			game.world.Monsters = []*monsterPkg.Monster3D{attacker, target}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			game.refreshMonsterAIState()

			game.combat.HandleMonsterInteractions()
			if want := attacker.AttackCooldownFrames(); attacker.AttackCDFrames != want {
				t.Fatalf("crossfire cooldown = %d, want attacker cooldown %d", attacker.AttackCDFrames, want)
			}
			hpAfterFirst := target.HitPoints
			game.combat.HandleMonsterInteractions()
			if target.HitPoints != hpAfterFirst {
				t.Fatalf("crossfire bypassed attacker cooldown (HP %d -> %d)", hpAfterFirst, target.HitPoints)
			}
		})
	}
}

// Crossfire is a normal monster turn in TB too. A target being a summon or a
// bound undead must not collapse an authored multi-attack turn to one swing.
func TestCrossfireUsesEachMonsterAttackCountTurnBased(t *testing.T) {
	for _, tc := range []struct {
		name          string
		attackerBound bool
	}{
		{name: "enemy_vs_card_summon"},
		{name: "bound_undead_vs_enemy", attackerBound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			game, gl, ts := tbBehaviorGame(t, 20, 20)
			fillTestParty(t, game)
			placePlayerAtTile(game, 2, 2, ts)

			attacker := monsterPkg.NewMonster3DFromConfig(10*ts+ts/2, 10*ts+ts/2, "skeleton", game.config)
			attacker.DamageMin, attacker.DamageMax = 1, 1
			attacker.AttacksPerRound = 3
			target := monsterPkg.NewMonster3DFromConfig(11*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
			target.MaxHitPoints, target.HitPoints = 10, 10
			if tc.attackerBound {
				game.combat.applyBindUndead(attacker, 300, "Bind Undead")
			} else {
				markCardAlly(target)
			}

			game.world.Monsters = []*monsterPkg.Monster3D{attacker, target}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			game.refreshMonsterAIState()
			runOneMonsterTurn(game, gl)

			if want := 7; target.HitPoints != want {
				t.Fatalf("target HP after crossfire turn = %d, want %d (three authored attacks)", target.HitPoints, want)
			}
		})
	}
}

// Every active boss must also acquire, approach, and strike card summons in
// the real-time arena loop. Quest seals are lifted only for this active-state
// combat test; their dormant behavior is intentional.
func TestEveryActiveBossFightsCardSummonsRT(t *testing.T) {
	for _, key := range activeBossKeys(t) {
		t.Run(key, func(t *testing.T) {
			game, _, enemy, ally := cardSummonDuelTB(t, key)
			game.turnBasedMode = false
			enemy.PassiveUntilQuest = ""
			d0, hp0 := Distance(enemy.X, enemy.Y, ally.X, ally.Y), ally.HitPoints
			runRTFoeTicks(game, 6*game.config.GetTPS())
			if d := Distance(enemy.X, enemy.Y, ally.X, ally.Y); d >= d0 {
				t.Fatalf("boss %s did not close on card summon in RT (%.0f -> %.0f)", enemy.Name, d0, d)
			}
			if ally.HitPoints >= hp0 {
				t.Fatalf("boss %s never struck card summon in RT (HP %d -> %d)", enemy.Name, hp0, ally.HitPoints)
			}
		})
	}
}
