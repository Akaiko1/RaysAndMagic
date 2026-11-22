package game

import (
	"fmt"
	"slices"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/status"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"
	"ugataima/internal/world"
)

// chamberBossGame is a 12x12 open room on map "forest" whose adventure boss
// is a real Solstice boss at (6,3) fighting a party 3 tiles south.
func chamberBossGame(t *testing.T, a *config.AdventureConfig) (*MMGame, *GameLoop, *monsterPkg.Monster3D) {
	t.Helper()
	g, gl := newSpecialsTestGame(t)
	g.gameLoop = gl
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{}, LoadedMaps: map[string]*world.World3D{"forest": g.world}}
	if a != nil {
		world.GlobalWorldManager.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
	}
	placePlayerAtTile(g, 6, 6, 64)
	boss := spawnSpecialsMonster(g, "solstice_aureth", 6, 3)
	boss.HomeMap = "forest"
	for _, c := range g.party.Members {
		c.HitPoints, c.MaxHitPoints = 100000, 100000
	}
	noPartyDodge(t, g.combat)
	return g, gl, boss
}

// runChamberBossAction drives one boss action through the real RT or TB
// monster pass, then flies its shots until they land, and reports whether
// the boss attacked the party, and whether it acted at all: attacked or, off
// a TB firing line, stepped.
func runChamberBossAction(t *testing.T, g *MMGame, gl *GameLoop, boss *monsterPkg.Monster3D, tb bool) (attacked, acted bool) {
	t.Helper()
	shots, hp := g.projectileCount(), partyHPSum(g)
	x, y := boss.X, boss.Y
	if tb {
		runTBMonsterTurns(g, gl, 1)
	} else {
		g.refreshMonsterAIState()
		boss.AttackCDFrames = 0
		boss.State = monsterPkg.StateAttacking
		boss.StateTimer = 1
		g.combat.HandleMonsterInteractions()
	}
	attacked = g.projectileCount() > shots || partyHPSum(g) < hp
	flyShots(t, g, gl)
	return attacked, attacked || boss.X != x || boss.Y != y
}

// flyShots runs the real projectile tick until every shot has landed or
// expired.
func flyShots(t *testing.T, g *MMGame, gl *GameLoop) {
	t.Helper()
	for frame := 0; frame < 10*g.config.GetTPS() && g.projectileCount() > 0; frame++ {
		flyShotsFrame(t, g, gl)
	}
}

// flyShotsFrame is one real projectile tick.
func flyShotsFrame(t *testing.T, g *MMGame, gl *GameLoop) {
	t.Helper()
	if g.threading == nil {
		g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
		t.Cleanup(g.threading.Shutdown)
	}
	gl.updateProjectilesAndImpacts()
}

// TestChamberBossLaneAndPushAreFree: the lane pattern and the push ride a
// landed boss attack without spending it, each when its own cooldown is over,
// and restart that cooldown in whole rounds; the push goes one tile toward
// the nearest lane in any lane state.
func TestChamberBossLaneAndPushAreFree(t *testing.T) {
	type cell struct {
		name        string
		laneRunning bool   // the lane cooldown is not over
		pushRunning bool   // the push cooldown is not over
		party       [2]int // party tile
		lane        [4]int // the lane rect
		laneState   string // "", "warning", "live"
		wall        *[2]int
		blocker     *[2]int // a monster on this tile
		wantLane    bool    // the lane is announced on this action
		wantParty   [2]int
		wantPushed  bool
	}
	row8 := [4]int{3, 8, 9, 8}
	cells := []cell{
		{name: "lane-only", pushRunning: true, party: [2]int{6, 6}, lane: row8, wantLane: true, wantParty: [2]int{6, 6}},
		{name: "push-only", laneRunning: true, party: [2]int{6, 6}, lane: row8, wantParty: [2]int{6, 7}, wantPushed: true},
		{name: "lane-then-push", party: [2]int{6, 6}, lane: row8, wantLane: true, wantParty: [2]int{6, 7}, wantPushed: true},
		{name: "neither", laneRunning: true, pushRunning: true, party: [2]int{6, 6}, lane: row8, wantParty: [2]int{6, 6}},
		{name: "into-warned-lane", laneRunning: true, party: [2]int{6, 7}, lane: row8, laneState: "warning", wantParty: [2]int{6, 8}, wantPushed: true},
		{name: "into-burning-lane", laneRunning: true, party: [2]int{6, 7}, lane: row8, laneState: "live", wantParty: [2]int{6, 8}, wantPushed: true},
		{name: "already-in-lane", laneRunning: true, party: [2]int{6, 8}, lane: row8, wantParty: [2]int{6, 8}},
		{name: "longer-axis-first", laneRunning: true, party: [2]int{6, 6}, lane: [4]int{9, 7, 9, 7}, wantParty: [2]int{7, 6}, wantPushed: true},
		{name: "wall-sidesteps", laneRunning: true, party: [2]int{6, 6}, lane: [4]int{9, 7, 9, 7}, wall: &[2]int{7, 6}, wantParty: [2]int{6, 7}, wantPushed: true},
		{name: "wall-blocks", laneRunning: true, party: [2]int{6, 6}, lane: row8, wall: &[2]int{6, 7}, wantParty: [2]int{6, 6}},
		{name: "monster-blocks", laneRunning: true, party: [2]int{6, 6}, lane: row8, blocker: &[2]int{6, 7}, wantParty: [2]int{6, 6}},
	}
	for _, c := range cells {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", c.name, tb), func(t *testing.T) {
				const laneCooldown = 6
				a := &config.AdventureConfig{
					Effects: []config.EnvironmentEffect{{ID: "lane", Kind: "lane", Name: "Test Lane", Rect: c.lane, School: "fire", Damage: 1, WarningSeconds: 1.2, WarningRounds: 1, DurationSeconds: 6}},
					Boss:    &config.AdventureBoss{Monster: "solstice_aureth", Lanes: []string{"lane"}, CooldownSeconds: laneCooldown},
				}
				g, gl, boss := chamberBossGame(t, a)
				boss.Abilities.Summon = monsterPkg.SummonAbility{}
				tps, pushCooldown := g.config.GetTPS(), boss.Abilities.Push.CooldownSeconds
				if c.laneRunning {
					boss.LaneCD.Arm(laneCooldown, tps)
				}
				if c.pushRunning {
					boss.PushCD.Arm(pushCooldown, tps)
				}
				placePlayerAtTile(g, c.party[0], c.party[1], 64)
				if c.wall != nil {
					g.world.Tiles[c.wall[1]][c.wall[0]] = world.TileWall
				}
				if c.blocker != nil {
					spawnSpecialsMonster(g, "goblin", c.blocker[0], c.blocker[1]).HomeMap = "forest"
				}
				v := g.adventureVisit("forest")
				switch c.laneState {
				case "warning":
					v.effectState("lane").Warning = roundClock()
				case "live":
					v.effectState("lane").Remaining = roundClock()
				}
				hp := partyHPSum(g)
				attacked, _ := runChamberBossAction(t, g, gl, boss, tb)
				if partyHPSum(g) >= hp {
					t.Fatal("the boss's bolt missed: the push came before it landed")
				}
				if !attacked {
					t.Fatal("a free boss action took the boss's attack")
				}
				if announced := c.laneState == "" && v.effectState("lane").Warning.Active(); announced != c.wantLane {
					t.Fatalf("lane announced=%v, want %v", announced, c.wantLane)
				}
				if x, y := g.GetPlayerTilePosition(); [2]int{x, y} != c.wantParty {
					t.Fatalf("party at %d,%d, want %v", x, y, c.wantParty)
				}
				if pushed := countCombatLog(g, "shoves the party") > 0; pushed != c.wantPushed {
					t.Fatalf("push logged=%v, want %v", pushed, c.wantPushed)
				}
				// A cooldown that was over restarts in full: its seconds in RT,
				// the same rounds in TB.
				for _, clock := range []struct {
					name    string
					running bool
					got     status.DualClock
					seconds int
				}{{"lane", c.laneRunning, boss.LaneCD, laneCooldown}, {"push", c.pushRunning, boss.PushCD, pushCooldown}} {
					if !clock.running && (clock.got.Frames != clock.seconds*tps || clock.got.Turns != config.CombatRounds(clock.seconds)) {
						t.Fatalf("%s cooldown %+v after firing, want %ds / %d turns", clock.name, clock.got, clock.seconds, config.CombatRounds(clock.seconds))
					}
				}
			})
		}
	}
}

// TestChamberBossFreeActionCadenceInTurns: on a TB boss that lands every
// attack, a 2-round lane cooldown takes its turn at a pattern on turns 1, 3
// and 5, the authored 3-round push shoves on turns 1 and 4, and a phase's
// 1-round lane cooldown takes every turn.
func TestChamberBossFreeActionCadenceInTurns(t *testing.T) {
	for _, tc := range []struct {
		name     string
		phase    bool
		wantLane []int
	}{
		{name: "default", wantLane: []int{1, 3, 5}},
		{name: "one-round-phase", phase: true, wantLane: []int{1, 2, 3, 4, 5, 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &config.AdventureConfig{
				Effects: []config.EnvironmentEffect{{ID: "lane", Kind: "lane", Name: "Test Lane", Rect: [4]int{3, 9, 9, 9}, School: "fire", Damage: 1, WarningSeconds: 1.2, WarningRounds: 1, DurationSeconds: 3}},
				Boss:    &config.AdventureBoss{Monster: "solstice_aureth", Lanes: []string{"lane"}, CooldownSeconds: 6},
			}
			if tc.phase {
				a.Boss.Phases = []config.AdventureBossPhase{{BelowPercent: 100, Patterns: [][]string{{"lane"}}, CooldownSeconds: 3}}
			}
			g, gl, boss := chamberBossGame(t, a)
			boss.Abilities.Summon = monsterPkg.SummonAbility{}
			if boss.Abilities.Push.CooldownSeconds != 9 {
				t.Fatalf("authored push cooldown %ds, want 9 (3 rounds)", boss.Abilities.Push.CooldownSeconds)
			}
			v := g.adventureVisit("forest")
			var lanes, pushes []int
			for turn := 1; turn <= 6; turn++ {
				before, shoves := v.LanePatterns, countCombatLog(g, "shoves the party")
				if attacked, _ := runChamberBossAction(t, g, gl, boss, true); !attacked {
					t.Fatalf("turn %d: the boss did not attack", turn)
				}
				if v.LanePatterns > before {
					lanes = append(lanes, turn)
				}
				if countCombatLog(g, "shoves the party") > shoves {
					pushes = append(pushes, turn)
				}
			}
			if !slices.Equal(lanes, tc.wantLane) || !slices.Equal(pushes, []int{1, 4}) {
				t.Fatalf("lane turns %v, push turns %v; want %v and [1 4]", lanes, pushes, tc.wantLane)
			}
		})
	}
}

// TestBossTimedSummon: summon.cooldown_seconds calls one add each time the
// timer runs out while below max, spending the boss's action, inside its
// movement bounds and at its auto-level; the timer restarts either way and
// does not wait for an attack moment.
func TestBossTimedSummon(t *testing.T) {
	const level = 35
	cells := []struct {
		name      string
		running   bool   // the timer has not run out
		existing  string // "", "live", "dead"
		kited     bool   // RT: no attack moment this frame
		wantAdd   bool
		wantShots bool
	}{
		{name: "ready", wantAdd: true},
		{name: "ready-after-add-died", existing: "dead", wantAdd: true},
		{name: "ready-but-capped", existing: "live", wantShots: true},
		{name: "timer-running", running: true, wantShots: true},
		{name: "ready-while-kited", kited: true, wantAdd: true},
	}
	for _, c := range cells {
		for _, tb := range []bool{false, true} {
			if c.kited && tb {
				continue // every TB turn is the boss's action
			}
			t.Run(fmt.Sprintf("%s/tb=%v", c.name, tb), func(t *testing.T) {
				g, gl, boss := chamberBossGame(t, nil)
				if boss.Abilities.Summon.CooldownSeconds != 12 || boss.Abilities.Summon.Max != 1 {
					t.Fatalf("authored summon = %+v, want a 12 s timer, max 1", boss.Abilities.Summon)
				}
				scaleAdventureMonster(g.config.AdventureScaling, boss, level)
				if c.running {
					boss.SummonCD.Arm(boss.Abilities.Summon.CooldownSeconds, g.config.GetTPS())
				}
				if c.existing != "" {
					add := spawnSpecialsMonster(g, "solstice_ember_elemental", 1, 1)
					add.SummonedBy = boss.ID
					if c.existing == "dead" {
						add.HitPoints = 0
					}
				}
				// After every spawn: the helper re-registers all bodies.
				arena := collision.MovementBounds{Enabled: true, MinX: 4 * 64, MinY: 2 * 64, MaxX: 9 * 64, MaxY: 5 * 64}
				g.collisionSystem.SetMovementBounds(boss.ID, arena)
				before := g.combat.countLiveSummons(boss)
				var attacked bool
				if c.kited {
					boss.AttackCDFrames = 100
					shots := g.projectileCount()
					g.refreshMonsterAIState()
					g.combat.HandleMonsterInteractions()
					attacked = g.projectileCount() > shots
				} else {
					attacked, _ = runChamberBossAction(t, g, gl, boss, tb)
				}
				var added *monsterPkg.Monster3D
				for _, m := range g.world.Monsters {
					if m.SummonedBy == boss.ID && m.IsAlive() && m.Y < 6*64 && m.X > 2*64 {
						added = m
					}
				}
				if got := g.combat.countLiveSummons(boss) - before; (got == 1) != c.wantAdd || got > 1 {
					t.Fatalf("summoned %d adds, want add=%v", got, c.wantAdd)
				}
				if attacked != c.wantShots {
					t.Fatalf("attacked=%v, want %v (a summon spends the action)", attacked, c.wantShots)
				}
				if !boss.SummonCD.Active() {
					t.Fatal("the timer did not restart when it ran out, or ran out early")
				}
				if !c.wantAdd {
					return
				}
				if !tb && boss.AttackCDFrames <= 0 {
					t.Fatal("an RT summon did not spend the boss's next attack")
				}
				if added == nil || added.Key != "solstice_ember_elemental" {
					t.Fatal("the add is not the boss's elemental")
				}
				if !arena.Allows(collision.NewBoundingBox(added.X, added.Y, 0, 0)) {
					t.Fatalf("add spawned at %.0f,%.0f outside the boss's bounds", added.X, added.Y)
				}
				want := AdventureScaledStats(g.config.AdventureScaling, monsterPkg.MonsterConfig.Monsters[added.Key], false, level)
				if added.AdventureScaleLevel != level || added.MaxHitPoints != want.HP || added.HitPoints != want.HP || added.DamageMax != want.DamageMax {
					t.Fatalf("add level %d HP %d/%d dmg max %d, want level %d HP %d dmg max %d",
						added.AdventureScaleLevel, added.HitPoints, added.MaxHitPoints, added.DamageMax, level, want.HP, want.DamageMax)
				}
			})
		}
	}
}

// TestArenaPullLandsLikeAStep: the arena lock pulls the party next to the
// boss onto an open tile and lands it there: a plate under it fires, and like
// any move the distance charges movement gear (user call 2026-10-15).
func TestArenaPullLandsLikeAStep(t *testing.T) {
	arena := [4]int{4, 1, 8, 5} // centre (6,3) holds the boss
	a := &config.AdventureConfig{
		OpeningOwned: true,
		Effects:      []config.EnvironmentEffect{{ID: "plates", Kind: "trap", Name: "Plates", Rect: [4]int{5, 2, 7, 4}, School: "fire", Damage: 50, RearmSeconds: 6, RearmRounds: 2}},
		Boss:         &config.AdventureBoss{Monster: "solstice_aureth", CooldownSeconds: 6, Arena: &arena},
	}
	g, _, boss := chamberBossGame(t, a)
	placePlayerAtTile(g, 6, 8, 64)
	sun := g.party.Members[0]
	sun.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_sun_crucible")
	boss.BeginPlayerEngagement()
	hp := partyHPSum(g)
	g.updateAdventureArena()
	x, y := g.GetPlayerTilePosition()
	if !g.adventureVisit("forest").ArenaLocked || x < 5 || x > 7 || y < 2 || y > 4 || (x == 6 && y == 3) {
		t.Fatalf("party pulled to %d,%d, want an open tile beside the boss and a sealed arena", x, y)
	}
	if partyHPSum(g) >= hp || !g.adventureVisit("forest").effectState("plates").Triggered {
		t.Fatal("the plate under the pulled party did not fire")
	}
	if sun.Equipment[items.SlotMainHand].Signature.Charges != 1 {
		t.Fatal("the pull's distance did not charge the Sun Crucible")
	}
}

// TestChamberBossFreeActionsNeedALandedAttack: lanes and the push fire when
// a boss attack lands, on the party or on another actor, and only then: a
// kited RT boss, a TB boss that only steps, or a shot the party slipped
// fires nothing and keeps both cooldowns ready.
func TestChamberBossFreeActionsNeedALandedAttack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tb      bool
		landed  bool
		prepare func(g *MMGame, gl *GameLoop, boss *monsterPkg.Monster3D)
	}{
		{name: "kited-rt", prepare: func(g *MMGame, _ *GameLoop, boss *monsterPkg.Monster3D) {
			boss.AttackCDFrames = 100
			g.refreshMonsterAIState()
			g.combat.HandleMonsterInteractions()
		}},
		{name: "stepping-tb", tb: true, prepare: func(g *MMGame, gl *GameLoop, _ *monsterPkg.Monster3D) {
			placePlayerAtTile(g, 9, 9, 64) // off every firing line and out of reach
			runTBMonsterTurns(g, gl, 1)
		}},
		{name: "slipped-shot", prepare: func(g *MMGame, gl *GameLoop, boss *monsterPkg.Monster3D) {
			g.refreshMonsterAIState()
			boss.AttackCDFrames, boss.State, boss.StateTimer = 0, monsterPkg.StateAttacking, 1
			g.combat.HandleMonsterInteractions()
			placePlayerAtTile(g, 9, 6, 64) // the bolt flies on past
		}},
		{name: "pounce-rt", landed: true, prepare: func(g *MMGame, _ *GameLoop, boss *monsterPkg.Monster3D) {
			// The real-time pounce strikes on landing, outside the usual swing.
			placePlayerAtTile(g, 6, 4, 64)
			g.refreshMonsterAIState()
			g.combat.commitMonsterAttack(boss, monsterAttackDestination{}, monsterAttackPounce)
		}},
		{name: "shot-at-an-ally", landed: true, prepare: func(g *MMGame, _ *GameLoop, boss *monsterPkg.Monster3D) {
			ally := spawnSpecialsMonster(g, "goblin", 6, 5)
			ally.Bound, ally.BoundClock = true, framesClock(100000)
			g.combat.performMonsterAttackAgainstMonster(boss, ally, ProjectileOwnerMonsterAtActor)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &config.AdventureConfig{
				Effects: []config.EnvironmentEffect{{ID: "lane", Kind: "lane", Name: "Test Lane", Rect: [4]int{3, 8, 9, 8}, School: "fire", Damage: 1, WarningSeconds: 1.2, WarningRounds: 1, DurationSeconds: 6}},
				Boss:    &config.AdventureBoss{Monster: "solstice_aureth", Lanes: []string{"lane"}, CooldownSeconds: 6},
			}
			g, gl, boss := chamberBossGame(t, a)
			boss.Abilities.Summon = monsterPkg.SummonAbility{}
			g.turnBasedMode = tc.tb
			tc.prepare(g, gl, boss)
			flyShots(t, g, gl)
			lane := g.adventureVisit("forest").effectState("lane").Warning.Active()
			if lane != tc.landed || boss.LaneCD.Active() != tc.landed || boss.PushCD.Active() != tc.landed {
				t.Fatalf("lane=%v lane cooldown=%v push cooldown=%v, want all %v", lane, boss.LaneCD.Active(), boss.PushCD.Active(), tc.landed)
			}
		})
	}
}

// TestCurrentArrivalTiles: a current resolves its whole source tile, in any
// effect order, and its receiver arrives like a step: a plate there fires,
// also when a lane released the current.
func TestCurrentArrivalTiles(t *testing.T) {
	for _, tc := range []struct {
		name          string
		plateAtSource bool // else at the receiver
		plateFirst    bool
		laneRelease   bool
	}{
		{name: "source-plate-after-current", plateAtSource: true},
		{name: "source-plate-before-current", plateAtSource: true, plateFirst: true},
		{name: "receiver-plate"},
		{name: "receiver-plate-lane-release", laneRelease: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, gl := newSpecialsTestGame(t)
			g.gameLoop = gl
			world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{}, LoadedMaps: map[string]*world.World3D{"forest": g.world}}
			a := &config.AdventureConfig{}
			world.GlobalWorldManager.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
			placePlayerAtTile(g, 6, 6, 64)
			plateTile := [4]int{4, 6, 4, 6}
			if tc.plateAtSource {
				plateTile = [4]int{6, 6, 6, 6}
			}
			plate := config.EnvironmentEffect{ID: "plate", Kind: "trap", Name: "Plate", Rect: plateTile, School: "fire", Damage: 5, RearmSeconds: 6, RearmRounds: 2}
			current := config.EnvironmentEffect{ID: "current", Kind: "transfer", Name: "Current", Rect: [4]int{6, 6, 6, 6}, Destination: [2]int{4, 6}}
			a.Effects = []config.EnvironmentEffect{current, plate}
			if tc.plateFirst {
				a.Effects = []config.EnvironmentEffect{plate, current}
			}
			hp := partyHPSum(g)
			if tc.laneRelease {
				g.resolveEnvironmentArrival(false)
			} else {
				g.applyEnvironmentArrival()
			}
			if x, y := g.GetPlayerTilePosition(); x != 4 || y != 6 {
				t.Fatalf("party at %d,%d, want carried to 4,6", x, y)
			}
			if partyHPSum(g) >= hp {
				t.Fatal("the plate did not fire")
			}
		})
	}
}

// A current never chains: one under the receiving tile sleeps until the party
// steps off it, even when a lane later releases transfers.
func TestCurrentReceiverInsideAnotherCurrent(t *testing.T) {
	g, gl := newSpecialsTestGame(t)
	g.gameLoop = gl
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest", MapConfigs: map[string]*config.MapConfig{}, LoadedMaps: map[string]*world.World3D{"forest": g.world}}
	a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{
		{ID: "a", Kind: "transfer", Name: "A", Rect: [4]int{6, 6, 6, 6}, Destination: [2]int{4, 6}},
		{ID: "b", Kind: "transfer", Name: "B", Rect: [4]int{4, 6, 4, 6}, Destination: [2]int{2, 6}},
	}}
	world.GlobalWorldManager.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
	placePlayerAtTile(g, 6, 6, 64)
	g.applyEnvironmentArrival()
	g.resolveEnvironmentArrival(false) // a lane releases transfers
	if x, y := g.GetPlayerTilePosition(); x != 4 || y != 6 {
		t.Fatalf("party at %d,%d, want held at the receiver 4,6", x, y)
	}
}
