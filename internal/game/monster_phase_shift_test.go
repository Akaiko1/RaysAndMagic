package game

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

const (
	phaseTestRow                   = 20
	phaseTestGapX0, phaseTestGapX1 = 17, 18 // barrier columns between the monster and the party
)

// phaseShiftScene: a monster west of a two-column barrier, the party east of
// it. rows bounds the barrier; a full-height barrier leaves no walking route.
func phaseShiftScene(t *testing.T, tb bool, barrier string, rows [2]int, key string, monsterX, partyX int) (*MMGame, *GameLoop, *monster.Monster3D, float64) {
	t.Helper()
	game, ts := summonTileWorld(t)
	game.turnBasedMode = tb
	game.maxMessages = 200
	gl := &GameLoop{game: game}
	if barrier != "" {
		tile := world.TileWall
		if barrier != "wall" {
			var ok bool
			if tile, ok = world.GlobalTileManager.GetTileTypeFromKey(barrier); !ok {
				t.Fatalf("tile %q missing", barrier)
			}
		}
		for y := rows[0]; y <= rows[1]; y++ {
			for x := phaseTestGapX0; x <= phaseTestGapX1; x++ {
				game.world.Tiles[y][x] = tile
			}
		}
		game.collisionSystem.UpdateTileChecker(game.world)
	}
	placePlayerAtTile(game, partyX, phaseTestRow, ts)
	m := spawnMonsterAtTile(game, key, monsterX, phaseTestRow, ts)
	if !m.CanPhaseShift() {
		t.Fatalf("%s must author phase_shift", key)
	}
	return game, gl, m, ts
}

// runPhaseShiftScene drives the real loop (RT frames or TB monster passes) and
// reports whether the monster blinked: one update that jumps it from the west
// bank to the east bank, which no walk can do.
func runPhaseShiftScene(game *MMGame, gl *GameLoop, m *monster.Monster3D, ts float64, updates int) (blinked bool, landing int) {
	for i := 0; i < updates; i++ {
		before := TileIndex(m.X, ts)
		if game.turnBasedMode {
			runOneMonsterTurn(game, gl)
		} else {
			stepMonstersRT(game, gl)
			game.combat.HandleMonsterInteractions()
		}
		if after := TileIndex(m.X, ts); before < phaseTestGapX0 && after > phaseTestGapX1 {
			return true, after
		}
	}
	return false, TileIndex(m.X, ts)
}

func phaseShiftLogged(game *MMGame) bool {
	for _, line := range game.GetCombatMessages() {
		if strings.Contains(line, "phase-shifts toward the party") {
			return true
		}
	}
	return false
}

// Phase Shift crosses a chasm or water toward the party only when walking
// cannot get there or the detour is far longer; walls stop it, and every
// other state keeps the ordinary pursuit.
func TestPhaseShiftCrossesGapsOnlyWhenWalkingFails(t *testing.T) {
	full := [2]int{0, 39}
	for _, tc := range []struct {
		name     string
		barrier  string
		rows     [2]int
		key      string
		monsterX int
		partyX   int // 0 = 22
		prep     func(*MMGame, *monster.Monster3D)
		blink    bool
		walks    bool // positive control: reaches the party's side on foot
		holds    bool // waits on its plan's launch tile for the cooldown
	}{
		{name: "chasm_no_route", barrier: "solstice_gap", rows: full, key: "solstice_cinder_hound", monsterX: 8, blink: true},
		{name: "water_no_route", barrier: "water", rows: full, key: "solstice_kiln_automaton", monsterX: 8, blink: true},
		{name: "chasm_long_detour", barrier: "solstice_gap", rows: [2]int{4, 36}, key: "solstice_cinder_hound", monsterX: 12, blink: true},
		{name: "ranged_out_of_range", barrier: "solstice_gap", rows: full, key: "solstice_ember_elemental", monsterX: 12, blink: true},
		{name: "wall_blocks", barrier: "wall", rows: full, key: "solstice_cinder_hound", monsterX: 12},
		{name: "chasm_short_detour_walks", barrier: "solstice_gap", rows: [2]int{19, 21}, key: "solstice_cinder_hound", monsterX: 12, walks: true},
		{name: "open_floor_walks", key: "solstice_cinder_hound", monsterX: 12, walks: true},
		{name: "ranged_shoots_across", barrier: "solstice_gap", rows: full, key: "solstice_ember_elemental", monsterX: 16, partyX: 21},
		{name: "cooldown_spent", barrier: "solstice_gap", rows: full, key: "solstice_cinder_hound", monsterX: 12, holds: true, prep: func(g *MMGame, m *monster.Monster3D) {
			m.PhaseShiftCD.Frames, m.PhaseShiftCD.Turns = 1_000_000, 1_000
		}},
		// The rest of the cooldown counts as walking: a long wait loses to the
		// detour, a short one still beats it.
		{name: "long_detour_long_wait_walks", barrier: "solstice_gap", rows: [2]int{14, 26}, key: "solstice_cinder_hound", monsterX: 12, walks: true, prep: func(g *MMGame, m *monster.Monster3D) {
			m.PhaseShiftCD.Frames, m.PhaseShiftCD.Turns = 1_000_000, 1_000
		}},
		{name: "long_detour_short_wait_blinks", barrier: "solstice_gap", rows: [2]int{14, 26}, key: "solstice_cinder_hound", monsterX: 12, blink: true, prep: func(g *MMGame, m *monster.Monster3D) {
			m.PhaseShiftCD.Frames, m.PhaseShiftCD.Turns = g.config.GetTPS(), 1
		}},
		{name: "rooted", barrier: "solstice_gap", rows: full, key: "solstice_cinder_hound", monsterX: 14, prep: func(g *MMGame, m *monster.Monster3D) {
			m.Root.Frames, m.Root.Turns = 1_000_000, 1_000
		}},
		{name: "calm", barrier: "solstice_gap", rows: full, key: "solstice_cinder_hound", monsterX: 12, prep: func(g *MMGame, m *monster.Monster3D) {
			m.IsEngagingPlayer, m.WasAttacked = false, false
		}},
	} {
		for _, tb := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/RT", true: "/TB"}[tb], func(t *testing.T) {
				partyX := cmp.Or(tc.partyX, 22)
				game, gl, m, ts := phaseShiftScene(t, tb, tc.barrier, tc.rows, tc.key, tc.monsterX, partyX)
				if tc.prep != nil {
					tc.prep(game, m)
				}
				updates := 2400
				if tb {
					updates = 30
				}
				hp := partyHPSum(game)
				blinked, landing := runPhaseShiftScene(game, gl, m, ts, updates)
				if blinked != tc.blink || phaseShiftLogged(game) != tc.blink {
					t.Fatalf("blinked=%v logged=%v, want %v (monster at tile %d)", blinked, phaseShiftLogged(game), tc.blink, TileIndex(m.X, ts))
				}
				if !tc.blink {
					if m.PhaseShiftCD.Frames < 0 || tc.prep == nil && (m.PhaseShiftCD.Frames != 0 || m.PhaseShiftCD.Turns != 0) {
						t.Fatal("a monster that never blinked armed the cooldown")
					}
					if tc.walks && TileIndex(m.X, ts) <= phaseTestGapX1 {
						t.Fatal("walking pursuit never reached the party's side (positive control)")
					}
					if p := m.PhasePlan; tc.holds && (!p.Active || TileIndex(m.X, ts) != p.Launch.X || TileIndex(m.Y, ts) != p.Launch.Y) {
						t.Fatalf("expected a hold on the launch tile, monster at (%d,%d) plan %+v", TileIndex(m.X, ts), TileIndex(m.Y, ts), p)
					}
					// This driver runs no projectile or telegraph clock, so a bolt in
					// flight or a warned strike counts as the attack.
					attacked := partyHPSum(game) < hp || len(game.magicProjectiles) > 0 || m.Telegraph.Warning.Active() || m.Telegraph.Released
					if tc.name == "ranged_shoots_across" && !attacked {
						t.Fatal("a ranged monster in range must attack across the gap instead of blinking")
					}
					return
				}
				if landing-phaseTestGapX1 > m.Abilities.PhaseShift.RangeTiles || landing >= partyX {
					t.Fatalf("landed on tile %d: past the reach or onto the party", landing)
				}
				if m.PhaseShiftCD.Frames <= 0 || m.PhaseShiftCD.Turns <= 0 {
					t.Fatalf("blink did not arm both cooldown clocks: %d frames / %d turns", m.PhaseShiftCD.Frames, m.PhaseShiftCD.Turns)
				}
			})
		}
	}
}

// A walker fighting a party summon across a gap blinks toward it as toward
// the party (the summon soaks its aggro while shooting it from the far bank),
// melee or ranged, and the log names the summon. The ranged walker's summon
// stands out of range of the whole near bank. A blink never lands on the
// party: in the last scene walls leave the party's row the only line across,
// with the summon nearer to the walker three rows up. That scene is TB only:
// an RT walker on the launch tile already has the party nearer than any
// summon across the gap and blinks toward the party instead.
func TestPhaseShiftCrossesTowardTheSummonItFights(t *testing.T) {
	east := phaseTestGapX1 + 1
	for _, tc := range []struct {
		name, key         string
		walkerX, summonX  int
		party             [2]int // tile
		partyOnTheLanding bool   // TB only
	}{
		{"melee", "solstice_cinder_hound", 12, east, [2]int{22, phaseTestRow}, false},
		{"ranged", "solstice_ember_elemental", 14, east + 3, [2]int{24, phaseTestRow}, false},
		{"party on the landing", "solstice_cinder_hound", 12, east, [2]int{east, phaseTestRow + 3}, true},
	} {
		for _, tb := range []bool{false, true} {
			if tc.partyOnTheLanding && !tb {
				continue
			}
			t.Run(tc.name+"/"+map[bool]string{false: "RT", true: "TB"}[tb], func(t *testing.T) {
				game, gl, m, ts := phaseShiftScene(t, tb, "solstice_gap", [2]int{0, 39}, tc.key, tc.walkerX, 22)
				// This driver runs no telegraph clock: a Cinder Charge wind-up at the
				// summon would hold the hound for good.
				m.Abilities.Telegraph = nil
				if tc.partyOnTheLanding {
					// The far bank: the party's row and a path from it round to
					// the summon's posts, walls elsewhere.
					open := map[[2]int]bool{{east, phaseTestRow}: true, {east, phaseTestRow + 3}: true,
						{east + 1, phaseTestRow + 3}: true, {east + 1, phaseTestRow + 2}: true, {east + 1, phaseTestRow + 1}: true}
					for y := range game.world.Height {
						for x := east; x < game.world.Width; x++ {
							if !open[[2]int{x, y}] {
								game.world.Tiles[y][x] = world.TileWall
							}
						}
					}
					game.collisionSystem.UpdateTileChecker(game.world)
				}
				placePlayerAtTile(game, tc.party[0], tc.party[1], ts)
				x, y := TileCenterFromTile(tc.summonX, phaseTestRow, ts) // nearer than the party
				summon := monster.NewMonster3DFromConfig(x, y, "frost_elemental", game.config)
				markCardAlly(summon)
				summon.Speed = 0 // holds its tile: distances stay as staged
				game.world.Monsters = append(game.world.Monsters, summon)
				game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
				game.refreshMonsterAIState()
				if m.AIFoe != summon {
					t.Fatalf("fixture: the walker fights %v, want the summon", m.AIFoe)
				}
				updates := 2400
				if tb {
					updates = 30
				}
				if blinked, at := runPhaseShiftScene(game, gl, m, ts, updates); !blinked {
					t.Fatalf("no blink toward the summon (walker at tile %d)", at)
				}
				if landing := [2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}; landing == tc.party {
					t.Fatalf("blinked onto the party's tile %v", landing)
				}
				want := "phase-shifts toward " + summon.Name
				if !slices.ContainsFunc(game.GetCombatMessages(), func(line string) bool { return strings.Contains(line, want) }) {
					t.Fatalf("no %q line", want)
				}
			})
		}
	}
}

// Every melee chamber mob with Phase Shift crosses every authored jump link of
// its chamber toward a party waiting past the far end, in both modes and both
// directions: the gaps that used to leave them staring at the party. Ranged
// walkers shoot across from there instead (the scene table pins that).
func TestPhaseShiftCrossesEveryChamberJumpLink(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	oldTM := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = oldTM })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatal(err)
	}
	chambers := map[string][][4]int{}
	for key, mc := range wm.MapConfigs {
		if mc.Adventure != nil && len(mc.Adventure.JumpLinks) > 0 {
			chambers[key] = mc.Adventure.JumpLinks
		} else if key != "forest" {
			delete(wm.MapConfigs, key)
		}
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatal(err)
	}
	ts := float64(cfg.GetTileSize())
	tested := 0
	for chamber, links := range chambers {
		w := wm.LoadedMaps[chamber]
		kinds := map[string]bool{}
		for _, m := range w.Monsters {
			if m.CanPhaseShift() && !m.HasRangedAttack() {
				kinds[m.Key] = true
			}
		}
		for kind := range kinds {
			for _, link := range links {
				for _, reverse := range []bool{false, true} {
					for _, tb := range []bool{false, true} {
						ax, ay, bx, by := link[0], link[1], link[2], link[3]
						if reverse {
							ax, ay, bx, by = bx, by, ax, ay
						}
						sx, sy := cmp.Compare(bx, ax), cmp.Compare(by, ay)
						g := newTestGame(cfg, w)
						g.turnBasedMode, g.maxMessages = tb, 100
						g.combat = NewCombatSystem(g)
						gl := &GameLoop{game: g}
						placePlayerAtTile(g, bx+sx, by+sy, ts)
						m := monster.NewMonster3DFromConfig(float64(ax-sx)*ts+ts/2, float64(ay-sy)*ts+ts/2, kind, g.config)
						m.IsEngagingPlayer, m.WasAttacked = true, true
						w.Monsters = []*monster.Monster3D{m}
						w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
						updates := 1800
						if tb {
							updates = 12
						}
						for i := 0; i < updates && !phaseShiftLogged(g); i++ {
							if tb {
								runOneMonsterTurn(g, gl)
							} else {
								// The real frame's telegraph clock: a chamber mob may
								// strike across the gap first, then blink.
								g.tickMonsterTelegraphs(false)
								stepMonstersRT(g, gl)
								g.combat.HandleMonsterInteractions()
							}
						}
						if !phaseShiftLogged(g) || TileIndex(m.X, ts) != bx || TileIndex(m.Y, ts) != by {
							t.Errorf("%s %s link %v reverse=%v TB=%v: ended on (%d,%d), want a blink onto (%d,%d)",
								chamber, kind, link, reverse, tb, TileIndex(m.X, ts), TileIndex(m.Y, ts), bx, by)
						}
						tested++
					}
				}
			}
		}
	}
	if tested == 0 {
		t.Fatal("no chamber with jump links and Phase Shift walkers (fixture)")
	}
}

// An RT plan ends the frame its monster stops being eligible, even when
// neither tile moved: a charmed mob must not keep walking to its launch tile.
func TestPhaseShiftPlanDropsWhenEligibilityEnds(t *testing.T) {
	game, _, m, _ := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 22)
	game.refreshMonsterAIState()
	if !m.PhasePlan.Active {
		t.Fatal("fixture: no blink plan toward the party")
	}
	game.combat.applyPacify(m, 30, "Charm")
	game.refreshMonsterAIState()
	if m.PhasePlan.Active {
		t.Fatal("the blink plan still steers a charmed mob")
	}
}

// The TB overlay shows a coming Phase Shift as movement: the landing is in
// reach while the monster stands on its launch tile with the cooldown spent by
// its turn, and the cached overlay notices the cooldown.
func TestPhaseShiftOverlayShowsTheLanding(t *testing.T) {
	game, _, m, ts := phaseShiftScene(t, true, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 22)
	plan := game.combat.planPhaseShift(m, 0)
	if !plan.Active {
		t.Fatal("fixture: no blink plan")
	}
	m.X, m.Y = TileCenterFromTile(plan.Launch.X, plan.Launch.Y, ts)
	game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	if p := game.combat.planPhaseShift(m, 0); !p.Active || p.Launch != plan.Launch {
		t.Fatalf("fixture: from the launch tile the plan is %+v", p)
	}
	landing := [2]int{plan.Landing.X, plan.Landing.Y}
	c := &combatAuraScratch{tiles: map[[2]int]bool{}}
	px, py := game.combat.logicalCameraXY()
	for _, tc := range []struct {
		cooldown int
		shown    bool
	}{{0, true}, {2, false}} {
		m.PhaseShiftCD.Turns = tc.cooldown
		game.combatAuraMovement(c, m, game.combatAuraMoveSteps(m), ts)
		if c.tiles[landing] != tc.shown {
			t.Errorf("cooldown %d turns: landing shown=%v want %v", tc.cooldown, c.tiles[landing], tc.shown)
		}
	}
	m.PhaseShiftCD.Turns = 0
	ready := game.combatAuraStateKey(px, py, 10, ts)
	m.PhaseShiftCD.Turns = 2
	if game.combatAuraStateKey(px, py, 10, ts) == ready {
		t.Error("the overlay cache key ignores the Phase Shift cooldown")
	}
	// One tile short of the launch: the blink needs a pass after the walk.
	m.PhaseShiftCD.Turns = 0
	m.X, m.Y = TileCenterFromTile(plan.Launch.X-1, plan.Launch.Y, ts)
	game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	game.currentTurn = 1
	for _, tc := range []struct {
		passes int
		shown  bool
	}{{1, false}, {2, true}} {
		game.turnBasedMonsterPassesLeft = tc.passes
		game.combatAuraMovement(c, m, game.combatAuraMoveSteps(m), ts)
		if c.tiles[landing] != tc.shown {
			t.Errorf("%d passes, one step from the launch: landing shown=%v want %v", tc.passes, c.tiles[landing], tc.shown)
		}
	}
}

// A plan that switches on steers the very next RT steps: the walk to the
// launch tile does not wait out the pursuit's repath delay on its old route.
func TestPhaseShiftLaunchWalkStartsAtOnce(t *testing.T) {
	game, gl, m, ts := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 5)
	for i := 0; i < 20; i++ {
		stepMonstersRT(game, gl) // pursuing the party west, on its own bank
	}
	if startX, _ := TileCenterFromTile(12, phaseTestRow, ts); m.PhasePlan.Active || m.X >= startX {
		t.Fatalf("fixture: the hound should walk west toward the party (x=%.0f, plan %+v)", m.X, m.PhasePlan)
	}
	placePlayerAtTile(game, 22, phaseTestRow, ts) // the party is across the gap now
	x0 := m.X
	for i := 0; i < 5; i++ {
		stepMonstersRT(game, gl)
	}
	if !m.PhasePlan.Active || m.X <= x0 {
		t.Fatalf("five frames after the plan switched on the hound went from x=%.0f to %.0f (plan %+v)", x0, m.X, m.PhasePlan)
	}
}

// A turn-based ranged walker attacks only along the party's row or column.
// Standing in range but off every lane, with no lane on its own bank, it blinks
// across instead of holding forever (real time simply shoots from there).
func TestPhaseShiftTBRangedBlinksWithoutAReachableLane(t *testing.T) {
	game, gl, m, ts := phaseShiftScene(t, true, "solstice_gap", [2]int{0, 39}, "solstice_ember_elemental", 12, 19)
	for x := 14; x <= 16; x++ { // the party's row west of the gap is walled off
		game.world.Tiles[phaseTestRow][x] = world.TileWall
	}
	game.collisionSystem.UpdateTileChecker(game.world)
	m.X, m.Y = TileCenterFromTile(16, phaseTestRow-3, ts)
	game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	px, py := game.combat.logicalCameraXY()
	if game.combat.monsterAttackPointInReachTB(m, px, py) || Distance(m.X, m.Y, px, py) > m.GetAttackRangePixels() {
		t.Fatal("fixture: the walker must stand in range but off every lane")
	}
	if blinked, _ := runPhaseShiftScene(game, gl, m, ts, 30); !blinked {
		t.Fatalf("held at (%d,%d) without a lane instead of blinking", TileIndex(m.X, ts), TileIndex(m.Y, ts))
	}
}

// With every attack and queue position around the party taken (near and
// second ring), a phase shifter waits: no blink plan, which could only carry
// it somewhere it can neither attack nor queue from.
func TestPhaseShiftWaitsWhenEveryAttackPositionIsTaken(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[tb], func(t *testing.T) {
			game, _, m, ts := phaseShiftScene(t, tb, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 22)
			if !game.combat.planPhaseShift(m, 0).Active {
				t.Fatal("fixture: with free attack positions the walker plans a blink")
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					x, y := TileCenterFromTile(22+dx, phaseTestRow+dy, ts)
					game.collisionSystem.RegisterEntity(collision.NewEntity(fmt.Sprintf("post-%d-%d", dx, dy), x, y, ts/2, ts/2, collision.CollisionTypeMonsterEngaged, false))
				}
			}
			if p := game.combat.planPhaseShift(m, 0); p.Active {
				t.Fatalf("every attack position is taken, yet the plan blinks onto %+v", p.Landing)
			}
		})
	}
}

// Walkers waiting on one launch tile together share it in transit: the tile
// fan draws them apart (pixel-on-pixel stacking is the illegal case), and
// each keeps its own plan.
func TestPhaseShiftWalkersWaitingTogetherFanOut(t *testing.T) {
	game, gl, a, _ := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 16, 22)
	b := monster.NewMonster3DFromConfig(a.X, a.Y, a.Key, game.config)
	b.IsEngagingPlayer, b.WasAttacked = true, true
	game.world.Monsters = append(game.world.Monsters, b)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	for _, m := range game.world.Monsters { // waiting, so neither blinks yet
		m.PhaseShiftCD.Frames, m.PhaseShiftCD.Turns = 1_000_000, 1_000
	}
	for i := 0; i < 3; i++ {
		stepMonstersRT(game, gl)
	}
	gl.updateTileFans()
	if !a.PhasePlan.Active || !b.PhasePlan.Active {
		t.Fatalf("both walkers should hold a blink plan: %+v / %+v", a.PhasePlan, b.PhasePlan)
	}
	if a.Fan.Count != 2 || b.Fan.Count != 2 || a.Fan.Index == b.Fan.Index {
		t.Fatalf("no tile fan for two walkers on one tile: %d/%d slots %d/%d", a.Fan.Count, b.Fan.Count, a.Fan.Index, b.Fan.Index)
	}
}

// A launch the walk cannot reach drops the plan for the pursuit until the next
// plan, instead of trading the shared path cache between both routes and
// searching twice every tick.
func TestPhaseShiftUnreachableLaunchDoesNotSearchEveryTick(t *testing.T) {
	game, gl, m, ts := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 22)
	pocket := monster.TileCoord{X: 8, Y: 10} // floor walled in on all four sides
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		game.world.Tiles[pocket.Y+d[1]][pocket.X+d[0]] = world.TileWall
	}
	game.collisionSystem.UpdateTileChecker(game.world)
	px, py := game.combat.logicalCameraXY()
	m.PhasePlan = monster.PhaseShiftPlan{Planned: true, Active: true, Launch: pocket,
		From:   monster.TileCoord{X: TileIndex(m.X, ts), Y: TileIndex(m.Y, ts)},
		Target: monster.TileCoord{X: TileIndex(px, ts), Y: TileIndex(py, ts)}}
	before := m.PathSearchCount
	for i := 0; i < 60; i++ {
		stepMonstersRT(game, gl)
	}
	if n := m.PathSearchCount - before; n > 10 {
		t.Fatalf("%d path searches in 60 ticks toward an unreachable launch", n)
	}
}

// A real-time walker blinks from its launch tile's center, where the blink was
// planned and checked, never from the edge it stepped in at: the jump covers
// at most range_tiles and follows the checked line.
func TestPhaseShiftBlinksFromTheLaunchTileCenter(t *testing.T) {
	game, gl, m, ts := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 12, 22)
	reach := float64(m.Abilities.PhaseShift.RangeTiles) * ts
	for i := 0; i < 2400; i++ {
		stepMonstersRT(game, gl)
		x0, y0, plan := m.X, m.Y, m.PhasePlan
		game.combat.HandleMonsterInteractions()
		if !phaseShiftLogged(game) {
			continue
		}
		cx, cy := TileCenterFromTile(plan.Launch.X, plan.Launch.Y, ts)
		if math.Hypot(x0-cx, y0-cy) > 0.5 {
			t.Fatalf("blinked from (%.1f,%.1f), off the launch tile center (%.1f,%.1f)", x0, y0, cx, cy)
		}
		if d := math.Hypot(m.X-x0, m.Y-y0); d > reach+1e-6 {
			t.Fatalf("the blink travelled %.2f tiles, range %d", d/ts, m.Abilities.PhaseShift.RangeTiles)
		}
		return
	}
	t.Fatal("never blinked")
}

// A blink runs straight along a row or column onto the first walkable tile
// beyond the gap; with the party on that tile it passes over the party.
func TestPhaseShiftBlinksInAStraightLine(t *testing.T) {
	for _, tc := range []struct {
		partyX, landingX int
	}{{22, phaseTestGapX1 + 1}, {phaseTestGapX1 + 1, phaseTestGapX1 + 2}} {
		game, _, m, _ := phaseShiftScene(t, false, "solstice_gap", [2]int{0, 39}, "solstice_cinder_hound", 16, tc.partyX)
		p := game.combat.planPhaseShift(m, 0)
		want := monster.TileCoord{X: tc.landingX, Y: phaseTestRow}
		if !p.Active || p.Launch != (monster.TileCoord{X: 16, Y: phaseTestRow}) || p.Landing != want {
			t.Errorf("party at x=%d: plan %+v, want a straight blink from (16,%d) onto %+v", tc.partyX, p, phaseTestRow, want)
		}
	}
}

// The TB wait counts the passes left this round, then the usual passes of each
// later round: the cooldown ticks once a round.
func TestPhaseShiftTBWaitCountsTheRoundsPasses(t *testing.T) {
	_, _, m, _ := phaseShiftScene(t, true, "", [2]int{}, "solstice_cinder_hound", 12, 22)
	for _, tc := range []struct{ cooldown, passesLeft, perRound, actions int }{
		{0, 2, 1, 0}, {1, 2, 1, 2}, {2, 2, 1, 3}, {3, 1, 2, 5},
	} {
		m.PhaseShiftCD.Turns = tc.cooldown
		if got, want := m.PhaseShiftWaitTiles(true, tc.passesLeft, tc.perRound), m.PeekTurnBasedSteps(tc.actions); got != want {
			t.Errorf("cooldown %d, %d passes left, %d a round: wait %d tiles, want %d (%d actions)", tc.cooldown, tc.passesLeft, tc.perRound, got, want, tc.actions)
		}
	}
}

// Draw shakes the camera; the TB overlay's cache key and the ranged walkers'
// firing lanes use the logical party position, so a shake changes neither.
func TestPhaseShiftOverlayIgnoresScreenShake(t *testing.T) {
	game, _, m, ts := phaseShiftScene(t, true, "solstice_gap", [2]int{0, 39}, "solstice_ember_elemental", 12, 22)
	px, py := game.combat.logicalCameraXY()
	key, lanes := game.combatAuraStateKey(px, py, 10, ts), game.turnBasedRangedGoalTiles(m)
	game.screenShakeOffsetX, game.screenShakeOffsetY = 9, -7 // as Draw applies it
	game.camera.X, game.camera.Y = game.camera.X+9, game.camera.Y-7
	if game.combatAuraStateKey(px, py, 10, ts) != key {
		t.Error("a screen shake changed the overlay cache key")
	}
	if !slices.Equal(game.turnBasedRangedGoalTiles(m), lanes) {
		t.Error("a screen shake moved the ranged firing lanes")
	}
}
