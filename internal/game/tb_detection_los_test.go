package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestTurnBasedAlarmRallyRequiresDirectSight guards the distinction between a
// bell's direct visual aggro and its deliberately wall-agnostic sound rally.
// A wall may not create the first aggro; after clear sight does, the bell may
// still wake nearby monsters through walls.
func TestTurnBasedAlarmRallyRequiresDirectSight(t *testing.T) {
	setup := func(t *testing.T, blocked bool) (*MMGame, *GameLoop, *monster.Monster3D, *monster.Monster3D) {
		t.Helper()
		game, loop, tile := tbBehaviorGame(t, 20, 20)
		placePlayerAtTile(game, 3, 5, tile)

		alarmX, alarmY := TileCenterFromTile(8, 5, tile)
		alarm := monster.NewMonster3DFromConfig(alarmX, alarmY, "alarm_clock", game.config)
		if alarm == nil {
			t.Fatal("alarm_clock missing from monsters.yaml")
		}
		// It is far outside its own detection, but within the bell's 15-tile
		// sound radius. It can only join via a real rally.
		sleeperX, sleeperY := TileCenterFromTile(10, 14, tile)
		sleeper := monster.NewMonster3DFromConfig(sleeperX, sleeperY, "grandfather_clock", game.config)
		if sleeper == nil {
			t.Fatal("grandfather_clock missing from monsters.yaml")
		}

		if blocked {
			game.world.Tiles[5][6] = world.TileWall
		}
		game.world.Monsters = []*monster.Monster3D{alarm, sleeper}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		return game, loop, alarm, sleeper
	}

	t.Run("blocked sight does not ring", func(t *testing.T) {
		game, loop, alarm, sleeper := setup(t, true)
		if game.collisionSystem.CheckLineOfSight(alarm.X, alarm.Y, game.camera.X, game.camera.Y) {
			t.Fatal("setup: wall must block the alarm's direct line of sight")
		}
		runOneMonsterTurn(game, loop)
		game.rallyAggroedAlarms()

		if alarm.IsEngagingPlayer || alarm.RallyDone {
			t.Fatalf("blocked alarm started/rang: engaging=%v rallyDone=%v", alarm.IsEngagingPlayer, alarm.RallyDone)
		}
		if sleeper.IsEngagingPlayer || sleeper.WasAttacked {
			t.Fatalf("blocked alarm woke a sleeper: engaging=%v wasAttacked=%v", sleeper.IsEngagingPlayer, sleeper.WasAttacked)
		}
	})

	t.Run("clear sight rings and rallies", func(t *testing.T) {
		game, loop, alarm, sleeper := setup(t, false)
		if !game.collisionSystem.CheckLineOfSight(alarm.X, alarm.Y, game.camera.X, game.camera.Y) {
			t.Fatal("setup: alarm must have a clear direct line of sight")
		}
		runOneMonsterTurn(game, loop)
		game.rallyAggroedAlarms()

		if !alarm.IsEngagingPlayer || !alarm.RallyDone {
			t.Fatalf("clear-sight alarm did not ring: engaging=%v rallyDone=%v", alarm.IsEngagingPlayer, alarm.RallyDone)
		}
		if !sleeper.IsEngagingPlayer || !sleeper.WasAttacked {
			t.Fatalf("clear-sight alarm did not wake sleeper: engaging=%v wasAttacked=%v", sleeper.IsEngagingPlayer, sleeper.WasAttacked)
		}
	})
}

type sightAggroScenario struct {
	name          string
	monsterKey    string
	distanceTiles float64
	blocked       bool
	lootGuard     bool
	want          bool
}

func runSightAggroScenario(t *testing.T, scenario sightAggroScenario, turnBased bool) bool {
	t.Helper()
	game, loop, tile := tbBehaviorGame(t, 20, 20)
	game.turnBasedMode = turnBased
	placePlayerAtTile(game, 3, 5, tile)

	mobX := game.camera.X + scenario.distanceTiles*tile
	mob := monster.NewMonster3DFromConfig(mobX, game.camera.Y, scenario.monsterKey, game.config)
	if mob == nil {
		t.Fatalf("%s missing from monsters.yaml", scenario.monsterKey)
	}
	mob.AITargetX, mob.AITargetY = game.camera.X, game.camera.Y
	if scenario.lootGuard {
		mob.LootGuarding = true
		mob.LootGuardMoveTileX = int(mob.X / tile)
		mob.LootGuardMoveTileY = int(mob.Y / tile)
	}
	if scenario.blocked {
		game.world.Tiles[5][4] = world.TileWall
	}
	game.world.Monsters = []*monster.Monster3D{mob}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	if scenario.blocked && game.collisionSystem.CheckLineOfSight(mob.X, mob.Y, game.camera.X, game.camera.Y) {
		t.Fatal("setup: wall must block the monster's direct line of sight")
	}

	if turnBased {
		runOneMonsterTurn(game, loop)
	} else {
		wrapper := &MonsterWrapper{
			Monster: mob, collisionSystem: game.collisionSystem,
			snapshot: game.collisionSystem.Snapshot(), game: game,
		}
		wrapper.Update()
		wrapper.ApplyCollisionUpdate()
	}
	return mob.IsEngagingPlayer
}

// TestSightAggroMatchesRealTimeAndTurnBased is a mode-parity contract. Every
// normal first-sight case runs the real RT wrapper and the TB scheduler, so a
// future mode-local radius/LoS check cannot silently drift from Monster3D.
func TestSightAggroMatchesRealTimeAndTurnBased(t *testing.T) {
	cases := []sightAggroScenario{
		{name: "goblin at authored radius", monsterKey: "goblin", distanceTiles: 4, want: true},
		{name: "goblin outside authored radius", monsterKey: "goblin", distanceTiles: 5, want: false},
		{name: "alarm at authored seven-tile radius", monsterKey: "alarm_clock", distanceTiles: 7, want: true},
		{name: "alarm outside authored radius", monsterKey: "alarm_clock", distanceTiles: 8, want: false},
		{name: "wall blocks normal sight", monsterKey: "goblin", distanceTiles: 3, blocked: true, want: false},
		{name: "loot guard uses exact seven-tile range", monsterKey: "goblin", distanceTiles: 6.5, lootGuard: true, want: true},
		{name: "wall blocks loot-guard sight", monsterKey: "goblin", distanceTiles: 6.5, lootGuard: true, blocked: true, want: false},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			rt := runSightAggroScenario(t, scenario, false)
			tb := runSightAggroScenario(t, scenario, true)
			if rt != scenario.want {
				t.Fatalf("RT engaged=%v, want %v", rt, scenario.want)
			}
			if tb != scenario.want {
				t.Fatalf("TB engaged=%v, want %v", tb, scenario.want)
			}
		})
	}
}

func TestAlarmRallyRequiresBellVisionWithinItsAuthoredRadius(t *testing.T) {
	game, _, tile := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 3, 5, tile)

	alarmX, alarmY := TileCenterFromTile(11, 5, tile) // eight tiles: clear LoS, outside alert_radius 7
	alarm := monster.NewMonster3DFromConfig(alarmX, alarmY, "alarm_clock", game.config)
	wokenX, wokenY := TileCenterFromTile(11, 6, tile)
	wouldBeWoken := monster.NewMonster3DFromConfig(wokenX, wokenY, "goblin", game.config)
	if alarm == nil || wouldBeWoken == nil {
		t.Fatal("test monsters missing from monsters.yaml")
	}
	alarm.IsEngagingPlayer = true // e.g. an external hit; this must not by itself ring the bell.
	game.world.Monsters = []*monster.Monster3D{alarm, wouldBeWoken}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	if !game.collisionSystem.CheckLineOfSight(alarm.X, alarm.Y, game.camera.X, game.camera.Y) {
		t.Fatal("setup: bell must have clear line of sight")
	}

	game.rallyAggroedAlarms()
	if alarm.RallyDone || wouldBeWoken.IsEngagingPlayer || wouldBeWoken.WasAttacked {
		t.Fatalf("bell outside its alert radius rang anyway: done=%v targetEngaged=%v targetHit=%v",
			alarm.RallyDone, wouldBeWoken.IsEngagingPlayer, wouldBeWoken.WasAttacked)
	}
}

// TestTurnBasedPackAggroRequiresOwnPartyLoS documents the one intentional
// mode difference. A visible same-key neighbour may join after a party hit in
// TB even beyond its personal alert radius; a neighbour behind a wall may not.
// RT has no such pack response.
func TestTurnBasedPackAggroRequiresOwnPartyLoS(t *testing.T) {
	setup := func(t *testing.T, turnBased bool) (*MMGame, *monster.Monster3D, *monster.Monster3D) {
		t.Helper()
		game, _, tile := tbBehaviorGame(t, 20, 20)
		game.turnBasedMode = turnBased
		placePlayerAtTile(game, 3, 4, tile)

		hitX, hitY := TileCenterFromTile(6, 4, tile)
		hit := monster.NewMonster3DFromConfig(hitX, hitY, "goblin", game.config)
		visibleX, visibleY := TileCenterFromTile(7, 4, tile) // four tiles from party, beyond goblin alert radius 3
		visible := monster.NewMonster3DFromConfig(visibleX, visibleY, "goblin", game.config)
		hiddenX, hiddenY := TileCenterFromTile(6, 8, tile)
		hidden := monster.NewMonster3DFromConfig(hiddenX, hiddenY, "goblin", game.config)
		if hit == nil || visible == nil || hidden == nil {
			t.Fatal("goblin missing from monsters.yaml")
		}
		for x := 0; x < 12; x++ {
			game.world.Tiles[6][x] = world.TileWall
		}
		game.world.Monsters = []*monster.Monster3D{hit, visible, hidden}
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		if game.collisionSystem.CheckLineOfSight(hidden.X, hidden.Y, game.camera.X, game.camera.Y) {
			t.Fatal("setup: hidden neighbour must not see the party")
		}
		hit.TakeDamage(1, monster.DamagePhysical)
		game.combat.markMonsterHit(hit)
		return game, visible, hidden
	}

	_, visibleTB, hiddenTB := setup(t, true)
	if !visibleTB.IsEngagingPlayer {
		t.Fatal("visible same-key neighbour did not join the TB pack response")
	}
	if hiddenTB.IsEngagingPlayer {
		t.Fatal("wall-blocked same-key neighbour joined the TB pack response")
	}

	_, visibleRT, hiddenRT := setup(t, false)
	if visibleRT.IsEngagingPlayer || hiddenRT.IsEngagingPlayer {
		t.Fatalf("RT unexpectedly ran TB pack aggro: visible=%v hidden=%v", visibleRT.IsEngagingPlayer, hiddenRT.IsEngagingPlayer)
	}
}

func TestAlarmRallyCapsTargetsAndDoesNotRelay(t *testing.T) {
	game, _, tile := tbBehaviorGame(t, 32, 32)
	spawn := func(t *testing.T, key string, tx, ty int) *monster.Monster3D {
		t.Helper()
		x, y := TileCenterFromTile(tx, ty, tile)
		m := monster.NewMonster3DFromConfig(x, y, key, game.config)
		if m == nil {
			t.Fatalf("%s missing from monsters.yaml", key)
		}
		return m
	}

	// The source sees the party by itself. The second bell is its first calm
	// target; its own far target lies outside the source bell's 15-tile radius.
	source := spawn(t, "alarm_clock", 5, 5)
	source.IsEngagingPlayer = true
	relayedBell := spawn(t, "alarm_clock", 10, 5)
	near := []*monster.Monster3D{
		spawn(t, "goblin", 6, 5),
		spawn(t, "goblin", 7, 5),
		spawn(t, "goblin", 8, 5),
		spawn(t, "goblin", 9, 5),
		spawn(t, "goblin", 10, 6),
	}
	farOnlyForRelay := spawn(t, "goblin", 21, 5) // source distance 16; relay distance 11
	game.world.Monsters = append([]*monster.Monster3D{source, relayedBell}, near...)
	game.world.Monsters = append(game.world.Monsters, farOnlyForRelay)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if source.RallyMaxTargets != 4 {
		t.Fatalf("alarm rally_max_targets = %d, want 4 from monsters.yaml", source.RallyMaxTargets)
	}
	game.rallyAggroedAlarms()

	woken := 0
	for _, m := range append(append([]*monster.Monster3D{relayedBell}, near...), farOnlyForRelay) {
		if m.IsEngagingPlayer || m.WasAttacked {
			woken++
		}
	}
	if woken != 4 {
		t.Fatalf("source alarm woke %d monsters, want cap of 4", woken)
	}
	if !relayedBell.IsEngagingPlayer || !relayedBell.WasAttacked || !relayedBell.RallyDone {
		t.Fatalf("relayed bell = engaging:%v attacked:%v done:%v, want hostile but relay-suppressed", relayedBell.IsEngagingPlayer, relayedBell.WasAttacked, relayedBell.RallyDone)
	}
	if farOnlyForRelay.IsEngagingPlayer || farOnlyForRelay.WasAttacked {
		t.Fatal("a bell awakened by another bell relayed to its private far target")
	}

	// A second pass must remain inert: the source rang once and its relay target
	// is permanently suppressed, including in the save-persisted state.
	game.rallyAggroedAlarms()
	if farOnlyForRelay.IsEngagingPlayer || farOnlyForRelay.WasAttacked {
		t.Fatal("alarm relay started on a later pass")
	}
}

func TestAlarmRallyDoesNotSpendCapOnDeadMonster(t *testing.T) {
	game, _, tile := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 3, 5, tile)

	spawn := func(key string, tx, ty int) *monster.Monster3D {
		x, y := TileCenterFromTile(tx, ty, tile)
		m := monster.NewMonster3DFromConfig(x, y, key, game.config)
		if m == nil {
			t.Fatalf("%s missing from monsters.yaml", key)
		}
		return m
	}
	alarm := spawn("alarm_clock", 5, 5)
	alarm.BeginPlayerEngagement()
	dead := spawn("goblin", 6, 5)
	dead.HitPoints = 0 // Still present until the end-of-frame removal sweep.
	live := spawn("goblin", 7, 5)
	alarm.RallyMaxTargets = 1
	game.world.Monsters = []*monster.Monster3D{alarm, dead, live}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.rallyAggroedAlarms()
	if live.IsEngagingPlayer != true || !live.WasAttacked {
		t.Fatal("dead alarm target consumed the only rally slot")
	}
	if dead.IsEngagingPlayer || dead.WasAttacked {
		t.Fatal("alarm must not engage a dead monster pending removal")
	}
}

func TestPacifiedAlarmCannotRing(t *testing.T) {
	game, _, tile := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 3, 5, tile)

	alarmX, alarmY := TileCenterFromTile(5, 5, tile)
	alarm := monster.NewMonster3DFromConfig(alarmX, alarmY, "alarm_clock", game.config)
	targetX, targetY := TileCenterFromTile(6, 5, tile)
	target := monster.NewMonster3DFromConfig(targetX, targetY, "goblin", game.config)
	if alarm == nil || target == nil {
		t.Fatal("test monsters missing from monsters.yaml")
	}
	alarm.BeginPlayerEngagement()
	game.combat.applyPacify(alarm, 60, "Charm")
	game.world.Monsters = []*monster.Monster3D{alarm, target}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.rallyAggroedAlarms()
	if alarm.RallyDone || target.IsEngagingPlayer || target.WasAttacked {
		t.Fatalf("pacified alarm rang: done=%v targetEngaged=%v targetHit=%v",
			alarm.RallyDone, target.IsEngagingPlayer, target.WasAttacked)
	}
}
