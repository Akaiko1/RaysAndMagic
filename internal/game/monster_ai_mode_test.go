package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestFleeingMonsterRetreatsInTurnBased(t *testing.T) {
	game, gl, tileSize := tbBehaviorGame(t, 24, 24)
	placePlayerAtTile(game, 10, 10, tileSize)

	// Puma exercises the pounce path too: fleeing must own the whole turn before
	// the scheduler reaches any party attack ability.
	fleer := monsterPkg.NewMonster3DFromConfig(12*tileSize+tileSize/2, 10*tileSize+tileSize/2, "puma", game.config)
	fleer.State = monsterPkg.StateFleeing
	fleer.WasAttacked = true
	fleer.IsEngagingPlayer = false
	game.world.Monsters = []*monsterPkg.Monster3D{fleer}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	before := Distance(fleer.X, fleer.Y, game.camera.X, game.camera.Y)
	hpBefore := partyHPSum(game)
	game.refreshMonsterAIState()
	runOneMonsterTurn(game, gl)

	if fleer.State != monsterPkg.StateFleeing {
		t.Fatalf("TB flee became state %v, want StateFleeing", fleer.State)
	}
	if after := Distance(fleer.X, fleer.Y, game.camera.X, game.camera.Y); after <= before {
		t.Fatalf("TB fleer moved toward/stayed by the party: distance %.0f -> %.0f", before, after)
	}
	if fleer.StateTimer != game.config.GetTPS() {
		t.Fatalf("TB flee clock = %d, want one turn (%d frames)", fleer.StateTimer, game.config.GetTPS())
	}
	if got := partyHPSum(game); got != hpBefore {
		t.Fatalf("TB fleeing monster damaged party: HP %d -> %d", hpBefore, got)
	}
}

func TestFleeingPouncerCannotAttackInRealTime(t *testing.T) {
	game, _, tileSize := tbBehaviorGame(t, 20, 20)
	game.turnBasedMode = false
	placePlayerAtTile(game, 10, 10, tileSize)

	puma := monsterPkg.NewMonster3DFromConfig(12*tileSize+tileSize/2, 10*tileSize+tileSize/2, "puma", game.config)
	puma.State = monsterPkg.StateFleeing
	puma.WasAttacked = true
	puma.IsEngagingPlayer = false
	game.world.Monsters = []*monsterPkg.Monster3D{puma}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	hpBefore := partyHPSum(game)
	xBefore, yBefore := puma.X, puma.Y
	game.refreshMonsterAIState()
	game.combat.HandleMonsterInteractions()

	if got := partyHPSum(game); got != hpBefore {
		t.Fatalf("RT fleeing pouncer damaged party: HP %d -> %d", hpBefore, got)
	}
	if puma.X != xBefore || puma.Y != yBefore {
		t.Fatalf("RT combat pass pounced while fleeing: (%.0f, %.0f) -> (%.0f, %.0f)",
			xBefore, yBefore, puma.X, puma.Y)
	}
}

func TestNonHostileBehaviorClearsStaleCombatInBothModes(t *testing.T) {
	for _, behavior := range []struct {
		name  string
		apply func(*monsterPkg.Monster3D, int)
	}{
		{
			name: "pacified",
			apply: func(m *monsterPkg.Monster3D, tps int) {
				m.Pacified = true
				m.PacifiedFramesRemaining = tps
			},
		},
		{
			name: "passive",
			apply: func(m *monsterPkg.Monster3D, _ int) {
				m.PassiveUntilAttacked = true
			},
		},
	} {
		for _, turnBased := range []bool{false, true} {
			mode := "RT"
			if turnBased {
				mode = "TB"
			}
			t.Run(behavior.name+"/"+mode, func(t *testing.T) {
				game, gl, tileSize := tbBehaviorGame(t, 12, 12)
				game.turnBasedMode = turnBased
				placePlayerAtTile(game, 6, 6, tileSize)

				m := monsterPkg.NewMonster3DFromConfig(7*tileSize+tileSize/2, 6*tileSize+tileSize/2, "goblin", game.config)
				m.State = monsterPkg.StateAttacking
				m.StateTimer = 1
				m.IsEngagingPlayer = true
				behavior.apply(m, game.config.GetTPS())
				game.world.Monsters = []*monsterPkg.Monster3D{m}
				game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

				hpBefore := partyHPSum(game)
				game.refreshMonsterAIState()
				if turnBased {
					runOneMonsterTurn(game, gl)
				} else {
					wrapper := CreateMonsterWrapper(m, game.collisionSystem, game.collisionSystem.Snapshot(), game)
					wrapper.Update()
					wrapper.ApplyCollisionUpdate()
					game.combat.HandleMonsterInteractions()
				}

				if m.State != monsterPkg.StateIdle || m.IsEngagingPlayer {
					t.Fatalf("%s %s monster kept stale combat state: state=%v engaging=%v",
						mode, behavior.name, m.State, m.IsEngagingPlayer)
				}
				if got := partyHPSum(game); got != hpBefore {
					t.Fatalf("%s %s monster damaged party: HP %d -> %d",
						mode, behavior.name, hpBefore, got)
				}
			})
		}
	}
}

func TestInertIdolPoisonTicksInBothModes(t *testing.T) {
	run := func(t *testing.T, turnBased bool) int {
		t.Helper()
		game, gl, tileSize := tbBehaviorGame(t, 12, 12)
		game.turnBasedMode = turnBased
		placePlayerAtTile(game, 6, 6, tileSize)

		idol := monsterPkg.NewMonster3DFromConfig(8*tileSize+tileSize/2, 6*tileSize+tileSize/2, "goblin", game.config)
		idol.WarlordIdol = true
		idol.MaxHitPoints = 200
		idol.HitPoints = 200
		idol.ApplyPoison(game.config.GetTPS())
		game.world.Monsters = []*monsterPkg.Monster3D{idol}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

		if turnBased {
			runOneMonsterTurn(game, gl)
		} else {
			for range game.config.GetTPS() {
				wrapper := CreateMonsterWrapper(idol, game.collisionSystem, game.collisionSystem.Snapshot(), game)
				wrapper.Update()
				wrapper.ApplyCollisionUpdate()
			}
		}
		if idol.StateTimer != 0 {
			t.Fatalf("inert idol ran its state machine: timer=%d", idol.StateTimer)
		}
		return idol.HitPoints
	}

	rtHP := run(t, false)
	tbHP := run(t, true)
	if rtHP != 198 || tbHP != 198 {
		t.Fatalf("one poison interval on inert idol: RT HP=%d, TB HP=%d; want 198 in both", rtHP, tbHP)
	}
}

func TestPounceRequiresActivePartyTargetAndLineOfSight(t *testing.T) {
	t.Run("RT calm mob cannot create aggro", func(t *testing.T) {
		game, _, tileSize := tbBehaviorGame(t, 16, 16)
		game.turnBasedMode = false
		placePlayerAtTile(game, 6, 6, tileSize)
		puma := monsterPkg.NewMonster3DFromConfig(9*tileSize+tileSize/2, 6*tileSize+tileSize/2, "puma", game.config)
		game.world.Monsters = []*monsterPkg.Monster3D{puma}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

		hpBefore := partyHPSum(game)
		xBefore, yBefore := puma.X, puma.Y
		game.combat.HandleMonsterInteractions()
		if got := partyHPSum(game); got != hpBefore || puma.X != xBefore || puma.Y != yBefore {
			t.Fatalf("calm RT pouncer attacked without first aggro: HP %d -> %d, pos (%.0f,%.0f) -> (%.0f,%.0f)",
				hpBefore, got, xBefore, yBefore, puma.X, puma.Y)
		}
	})

	for _, turnBased := range []bool{false, true} {
		mode := "RT"
		if turnBased {
			mode = "TB"
		}
		t.Run(mode+" wall blocks leap", func(t *testing.T) {
			game, gl, tileSize := tbBehaviorGame(t, 16, 16)
			game.turnBasedMode = turnBased
			placePlayerAtTile(game, 5, 6, tileSize)
			puma := monsterPkg.NewMonster3DFromConfig(9*tileSize+tileSize/2, 6*tileSize+tileSize/2, "puma", game.config)
			puma.WasAttacked = true
			puma.BeginPlayerEngagement()
			puma.State = monsterPkg.StatePursuing
			game.world.Tiles[6][7] = world.TileWall
			game.world.Monsters = []*monsterPkg.Monster3D{puma}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			if game.collisionSystem.CheckLineOfSight(puma.X, puma.Y, game.camera.X, game.camera.Y) {
				t.Fatal("setup: wall must block pounce line of sight")
			}

			hpBefore := partyHPSum(game)
			if turnBased {
				game.refreshMonsterAIState()
				runOneMonsterTurn(game, gl)
			} else {
				game.combat.HandleMonsterInteractions()
			}
			if got := partyHPSum(game); got != hpBefore {
				t.Fatalf("%s pouncer leapt through a wall: HP %d -> %d", mode, hpBefore, got)
			}
			if puma.PounceCDFrames != 0 || puma.PounceCDTurns != 0 {
				t.Fatalf("%s blocked pounce armed cooldowns: frames=%d turns=%d",
					mode, puma.PounceCDFrames, puma.PounceCDTurns)
			}
		})
	}
}

func TestDeadCachedFoeCannotFallThroughToPartyAttack(t *testing.T) {
	for _, turnBased := range []bool{false, true} {
		mode := "RT"
		if turnBased {
			mode = "TB"
		}
		t.Run(mode, func(t *testing.T) {
			game, gl, tileSize := tbBehaviorGame(t, 12, 12)
			game.turnBasedMode = turnBased
			placePlayerAtTile(game, 6, 6, tileSize)

			attacker := monsterPkg.NewMonster3DFromConfig(7*tileSize+tileSize/2, 6*tileSize+tileSize/2, "goblin", game.config)
			attacker.State = monsterPkg.StateAttacking
			attacker.StateTimer = 1
			attacker.IsEngagingPlayer = true
			foe := monsterPkg.NewMonster3DFromConfig(8*tileSize+tileSize/2, 6*tileSize+tileSize/2, "skeleton", game.config)
			foe.Bound = true
			foe.HitPoints = 0 // killed earlier in the same frame/monster pass
			attacker.AIFoe = foe
			attacker.AITargetX, attacker.AITargetY = foe.X, foe.Y
			game.world.Monsters = []*monsterPkg.Monster3D{attacker, foe}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			hpBefore := partyHPSum(game)
			if turnBased {
				runOneMonsterTurn(game, gl)
			} else {
				game.combat.HandleMonsterInteractions()
			}
			if got := partyHPSum(game); got != hpBefore {
				t.Fatalf("%s monster with dead cached foe attacked party: HP %d -> %d", mode, hpBefore, got)
			}
		})
	}
}
