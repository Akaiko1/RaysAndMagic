package game

import (
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestMonsterMoveTurnBased_RoutesAroundBarrierViaFord drives the REAL turn-based
// movement path (monsterMoveTurnBased), not just the A* helper: a mob is cut off
// from the party by an impassable barrier (a wall stands in for the river) with a
// single walkable gap (the ford/bridge). Stepping turn by turn it must cross via
// the gap and reach the party - not oscillate at the bank, the bug that let the
// stranded gorilla be ranged down for free. Guards the greedy->A* fallthrough.
func TestMonsterMoveTurnBased_RoutesAroundBarrierViaFord(t *testing.T) {
	cfg := loadTestConfig(t)
	tile := float64(cfg.GetTileSize())

	const W, H = 10, 10
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = W, H
	w.Tiles = make([][]world.TileType3D, H)
	for y := 0; y < H; y++ {
		w.Tiles[y] = make([]world.TileType3D, W)
		for x := 0; x < W; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	// Impassable barrier down column 4 with a single walkable gap at (4,7) = the ford.
	const wallCol, fordRow = 4, 7
	for y := 0; y < H; y++ {
		if y == fordRow {
			continue
		}
		w.Tiles[y][wallCol] = world.TileWall
	}

	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)

	center := func(tx, ty int) (float64, float64) {
		return (float64(tx) + 0.5) * tile, (float64(ty) + 0.5) * tile
	}
	// Party (= the mob's AI target, via the camera) sits on the RIGHT of the barrier.
	g.camera.X, g.camera.Y = center(8, 2)

	mx, my := center(1, 2) // mob on the LEFT
	mob := &monsterPkg.Monster3D{
		ID: "ford_mob", Name: "Gorilla Titan", X: mx, Y: my,
		HitPoints: 100, MaxHitPoints: 100, AlertRadius: 8 * tile,
	}
	w.Monsters = append(w.Monsters, mob)
	g.collisionSystem.RegisterEntity(collision.NewEntity(mob.ID, mob.X, mob.Y, 32, 32, collision.CollisionTypeMonster, false))

	gl := &GameLoop{game: g}

	worldTile := func(p float64) int { return int(p / tile) }
	ptx, pty := worldTile(g.camera.X), worldTile(g.camera.Y)
	manhattan := func() int {
		dx, dy := worldTile(mob.X)-ptx, worldTile(mob.Y)-pty
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	crossed := false
	const maxSteps = 40
	steps := 0
	for ; steps < maxSteps && manhattan() > 1; steps++ {
		gl.monsterMoveTurnBased(mob)
		tx, ty := worldTile(mob.X), worldTile(mob.Y)
		if tx == wallCol && ty != fordRow {
			t.Fatalf("step %d: mob landed ON the barrier at (%d,%d)", steps, tx, ty)
		}
		if tx >= wallCol {
			crossed = true
		}
	}

	if manhattan() > 1 {
		t.Fatalf("mob never reached the party (oscillating at the bank); final Manhattan=%d after %d steps", manhattan(), steps)
	}
	if !crossed {
		t.Fatal("mob reached the party without ever crossing the barrier column - setup is wrong")
	}
	t.Logf("mob crossed the ford and reached the party in %d turn-based steps", steps)
}

// TestMonsterMoveTurnBased_EscapesPocketAwayFromParty reproduces the exact bug the
// real gorilla hit: it sat in a pocket whose only opening faced AWAY from the
// party (its party-side neighbor was an impassable log). A greedy-first mover
// oscillates - the naive step keeps pulling it toward the party (back into the
// pocket) while A* pulls it out the far side - so it never escapes and gets ranged
// down. A*-primary follows the path out. The straight-across ford test above does
// NOT catch this (there the naive step points into the wall and is simply blocked).
func TestMonsterMoveTurnBased_EscapesPocketAwayFromParty(t *testing.T) {
	cfg := loadTestConfig(t)
	tile := float64(cfg.GetTileSize())

	const W, H = 12, 12
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = W, H
	w.Tiles = make([][]world.TileType3D, H)
	for y := 0; y < H; y++ {
		w.Tiles[y] = make([]world.TileType3D, W)
		for x := 0; x < W; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	// Pocket around the mob at (2,5): walls box it in except the WEST opening (1,5).
	// The party is to the EAST, so the only exit faces away from it.
	for _, wt := range [][2]int{{3, 4}, {3, 5}, {3, 6}, {2, 4}, {2, 6}} {
		w.Tiles[wt[1]][wt[0]] = world.TileWall
	}

	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)

	center := func(tx, ty int) (float64, float64) {
		return (float64(tx) + 0.5) * tile, (float64(ty) + 0.5) * tile
	}
	g.camera.X, g.camera.Y = center(10, 5) // party EAST of the pocket

	mx, my := center(2, 5)
	mob := &monsterPkg.Monster3D{
		ID: "pocket_mob", Name: "Gorilla Titan", X: mx, Y: my,
		HitPoints: 100, MaxHitPoints: 100, AlertRadius: 12 * tile,
	}
	w.Monsters = append(w.Monsters, mob)
	g.collisionSystem.RegisterEntity(collision.NewEntity(mob.ID, mob.X, mob.Y, 32, 32, collision.CollisionTypeMonster, false))

	gl := &GameLoop{game: g}
	worldTile := func(p float64) int { return int(p / tile) }
	ptx, pty := worldTile(g.camera.X), worldTile(g.camera.Y)
	manhattan := func() int {
		dx, dy := worldTile(mob.X)-ptx, worldTile(mob.Y)-pty
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	const maxSteps = 50
	steps := 0
	for ; steps < maxSteps && manhattan() > 1; steps++ {
		gl.monsterMoveTurnBased(mob)
	}
	if manhattan() > 1 {
		t.Fatalf("mob never escaped the pocket / reached the party (greedy<->A* oscillation); final Manhattan=%d after %d steps", manhattan(), steps)
	}
	t.Logf("mob escaped the pocket and reached the party in %d turn-based steps", steps)
}

// TestMonsterMoveTurnBased_Save1DeepJungleGorillaWithSummons reproduces the
// real save1 bundle layout: party at (40,35), Gorilla Titan at (40,40), and the
// two Masked Huntress summons spawned by that gorilla at (40,41) and (40,46).
// The direct route is blocked by deep water, and the nearest summon blocks the
// first southward escape tile; the TB mover must still follow A* around the
// water instead of greedily bouncing toward the bank and back.
func TestMonsterMoveTurnBased_Save1DeepJungleGorillaWithSummons(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "deep_jungle")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("deep_jungle world did not load")
	}

	tile := float64(cfg.GetTileSize())
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)
	g.camera.X, g.camera.Y = TileCenterFromTile(40, 35, tile)
	g.camera.Angle = 1.5707963267948966
	g.collisionSystem = collision.NewCollisionSystem(w, tile)
	g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	at := func(key, id string, tx, ty int) *monsterPkg.Monster3D {
		x, y := TileCenterFromTile(tx, ty, tile)
		m := monsterPkg.NewMonster3DFromConfig(x, y, key, cfg)
		m.ID = id
		m.WasAttacked = true
		m.IsEngagingPlayer = true
		return m
	}

	gorilla := at("gorilla_titan", "monster_594", 40, 40)
	gorilla.HitPoints = 822
	gorilla.SummonFirstDone = true
	disableRandomBossSpecialsForTBPathTest(gorilla)
	nearSummon := at("masked_huntress", "monster_447", 40, 41)
	nearSummon.SummonedBy = gorilla.ID
	farSummon := at("masked_huntress", "monster_446", 40, 46)
	farSummon.SummonedBy = gorilla.ID

	w.Monsters = []*monsterPkg.Monster3D{gorilla, nearSummon, farSummon}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.refreshBoundAllyCache() // marks the struck gorilla as BossAggro.
	for _, m := range w.Monsters {
		g.refreshMonsterCollisionSolidity(m)
	}

	gl := &GameLoop{game: g}
	ptx, pty := g.GetPlayerTilePosition()
	chebyshev := func() int {
		tx, ty := int(gorilla.X/tile), int(gorilla.Y/tile)
		dx, dy := tx-ptx, ty-pty
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dy > dx {
			return dy
		}
		return dx
	}
	ready := func() bool {
		mtx, mty := int(gorilla.X/tile), int(gorilla.Y/tile)
		dx, dy := mtx-ptx, mty-pty
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx <= 1 && dy <= 1 && dx+dy > 0 {
			return g.collisionSystem.CheckLineOfSight(gorilla.X, gorilla.Y, g.camera.X, g.camera.Y)
		}
		if !gorilla.CanPounce() || gorilla.PounceCDTurns != 0 {
			return false
		}
		pounceTiles := int(gorilla.PounceRangePixels / tile)
		manhattan := dx + dy
		if manhattan < 2 || manhattan > pounceTiles {
			return false
		}
		for _, c := range [8][2]int{
			{ptx + 1, pty}, {ptx - 1, pty}, {ptx, pty + 1}, {ptx, pty - 1},
			{ptx + 1, pty + 1}, {ptx + 1, pty - 1}, {ptx - 1, pty + 1}, {ptx - 1, pty - 1},
		} {
			cx, cy := TileCenterFromTile(c[0], c[1], tile)
			if g.collisionSystem.CanMoveToWithHabitat(gorilla.ID, cx, cy, gorilla.HabitatPrefs, gorilla.Flying) {
				return true
			}
		}
		return false
	}

	startTile := [2]int{40, 40}
	leftStart := false
	visited := make([][2]int, 0, 40)
	for step := 0; step < 40 && !ready(); step++ {
		gl.monsterMoveTurnBased(gorilla)
		tx, ty := int(gorilla.X/tile), int(gorilla.Y/tile)
		cur := [2]int{tx, ty}
		visited = append(visited, cur)
		if cur != startTile {
			leftStart = true
		}
		if leftStart && cur == startTile {
			t.Fatalf("gorilla returned to its start tile after leaving it at step %d; path oscillated, visited=%v", step, visited)
		}
	}
	if !ready() {
		t.Fatalf("gorilla never reached an attack/pounce staging tile from save1 layout; final tile=(%d,%d), chebyshev=%d, visited=%v",
			int(gorilla.X/tile), int(gorilla.Y/tile), chebyshev(), visited)
	}
	t.Logf("gorilla routed around save1 water/summon layout to attack/pounce staging in %d TB steps: %v", len(visited), visited)
}

// TestMonsterTurnBased_Save1GorillaRetargetsAfterSummonDiesAndPartyMoves
// reproduces the longer live sequence that exposed the freeze: the gorilla takes
// the real deep-jungle route through its own pass-through summons, the party
// kills one, then moves around the lake for several TB rounds.
// The gorilla must keep advancing or be in a legal attack/pounce staging tile;
// standing still out of reach means the runtime turn/collision state wedged.
func TestMonsterTurnBased_Save1GorillaRetargetsAfterSummonDiesAndPartyMoves(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "deep_jungle")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("deep_jungle world did not load")
	}

	tile := float64(cfg.GetTileSize())
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)
	g.camera.X, g.camera.Y = TileCenterFromTile(40, 35, tile)
	g.camera.Angle = 1.5707963267948966
	g.collisionSystem = collision.NewCollisionSystem(w, tile)
	g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	at := func(key, id string, tx, ty int) *monsterPkg.Monster3D {
		x, y := TileCenterFromTile(tx, ty, tile)
		m := monsterPkg.NewMonster3DFromConfig(x, y, key, cfg)
		m.ID = id
		m.WasAttacked = true
		m.IsEngagingPlayer = true
		return m
	}

	gorilla := at("gorilla_titan", "monster_594", 40, 40)
	gorilla.HitPoints = 822
	gorilla.SummonFirstDone = true
	disableRandomBossSpecialsForTBPathTest(gorilla)
	nearSummon := at("masked_huntress", "monster_447", 40, 41)
	nearSummon.SummonedBy = gorilla.ID
	farSummon := at("masked_huntress", "monster_446", 40, 46)
	farSummon.SummonedBy = gorilla.ID

	w.Monsters = []*monsterPkg.Monster3D{gorilla, nearSummon, farSummon}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.refreshBoundAllyCache()
	refreshTBMonsterSolidity(g)

	gl := &GameLoop{game: g}

	// Drive only the gorilla through the real route. Own summons are physically
	// pass-through now, so this must not depend on the old swap-only movement
	// path; the regression we care about is the later retarget after one dies.
	startTile := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
	movedTowardParty := false
	for step := 0; step < 12; step++ {
		g.refreshBoundAllyCache()
		gl.monsterMoveTurnBased(gorilla)
		refreshTBMonsterSolidity(g)

		if [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)} != startTile {
			movedTowardParty = true
			break
		}
	}
	if !movedTowardParty {
		t.Fatalf("setup failed: gorilla did not advance through its pass-through summons; gorilla=(%d,%d)",
			int(gorilla.X/tile), int(gorilla.Y/tile))
	}

	// Simulate the party shooting the displaced summon dead, including the same
	// collision cleanup path the frame loop uses for dead monsters.
	farSummon.HitPoints = 0
	g.combat.markMonsterHit(farSummon)
	g.deadMonsterIDs = append(g.deadMonsterIDs, farSummon.ID)
	if g.reusableDeadSet == nil {
		g.reusableDeadSet = make(map[string]bool)
	}
	if g.reusableEncounterRewardsMap == nil {
		g.reusableEncounterRewardsMap = make(map[*monsterPkg.EncounterRewards]int)
	}
	gl.removeDeadMonstersByID()
	g.partyActionsUsed = 1 // shooting before moving should grant the anti-kite extra monster pass.

	partyPath := [][2]int{
		{40, 34}, {39, 34}, {38, 34}, {37, 34}, {36, 34}, {36, 35}, {35, 35},
		{35, 36}, {35, 37}, {35, 38}, {35, 39},
	}
	visited := make([][2]int, 0, len(partyPath)*2)
	stuckOutOfReach := 0
	for i, p := range partyPath {
		prev := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
		x, y := TileCenterFromTile(p[0], p[1], tile)
		if !g.collisionSystem.CanMoveTo("player", x, y) && gorillaReadyToAttackOrPounceTB(g, gorilla, tile) {
			if ent := g.collisionSystem.GetEntityByID(gorilla.ID); ent != nil {
				probe := collision.NewBoundingBox(x, y, 16, 16)
				if probe.Intersects(ent.BoundingBox) {
					t.Logf("gorilla intercepted the party path before move %d to (%d,%d); visited=%v", i, p[0], p[1], visited)
					return
				}
			}
		}
		movePartyToTileForTBTest(t, g, p[0], p[1], tile)
		runFullMonsterTurnForTBTest(t, g, gl)
		refreshTBMonsterSolidity(g)

		cur := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
		visited = append(visited, cur)
		if cur == prev && !gorillaReadyToAttackOrPounceTB(g, gorilla, tile) {
			stuckOutOfReach++
		} else {
			stuckOutOfReach = 0
		}
		if stuckOutOfReach >= 2 {
			ptx, pty := g.GetPlayerTilePosition()
			t.Fatalf("gorilla froze out of reach after party move %d to (%d,%d); gorilla=(%d,%d), player=(%d,%d), visited=%v",
				i, p[0], p[1], cur[0], cur[1], ptx, pty, visited)
		}
		if i == 4 {
			for buffRound := 0; buffRound < 3; buffRound++ {
				prev = [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
				spendPartyBuffRoundForTBTest(t, g)
				runFullMonsterTurnForTBTest(t, g, gl)
				refreshTBMonsterSolidity(g)
				cur = [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
				visited = append(visited, cur)
				if cur == prev && !gorillaReadyToAttackOrPounceTB(g, gorilla, tile) {
					stuckOutOfReach++
				} else {
					stuckOutOfReach = 0
				}
				if stuckOutOfReach >= 2 {
					ptx, pty := g.GetPlayerTilePosition()
					t.Fatalf("gorilla froze out of reach after buff round %d; gorilla=(%d,%d), player=(%d,%d), visited=%v",
						buffRound, cur[0], cur[1], ptx, pty, visited)
				}
			}
		}
	}
	t.Logf("gorilla retargeted after summon death and party lake movement; visited=%v", visited)
}

func TestMonsterTurnBased_PounceFailFallsThroughToMovement(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 12, 12)
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)
	tile := float64(cfg.GetTileSize())
	g.camera.X, g.camera.Y = TileCenterFromTile(5, 5, tile)
	g.collisionSystem.UpdateEntity("player", g.camera.X, g.camera.Y)

	gorillaX, gorillaY := TileCenterFromTile(5, 2, tile)
	gorilla := monsterPkg.NewMonster3DFromConfig(gorillaX, gorillaY, "gorilla_titan", cfg)
	gorilla.ID = "pounce_fail_gorilla"
	gorilla.WasAttacked = true
	gorilla.IsEngagingPlayer = true
	gorilla.SummonFirstDone = true
	disableRandomBossSpecialsForTBPathTest(gorilla)
	gorilla.PounceCDTurns = 0
	w.Monsters = []*monsterPkg.Monster3D{gorilla}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.refreshBoundAllyCache()
	refreshTBMonsterSolidity(g)

	for _, c := range [8][2]int{
		{6, 5}, {4, 5}, {5, 6}, {5, 4},
		{6, 6}, {6, 4}, {4, 6}, {4, 4},
	} {
		x, y := TileCenterFromTile(c[0], c[1], tile)
		id := "landing_blocker"
		id += string(rune('a' + len(g.collisionSystem.GetAllEntities())))
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, x, y, 32, 32, collision.CollisionTypeMonsterEngaged, true))
	}

	gl := &GameLoop{game: g}
	start := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
	runOneMonsterTurn(g, gl)
	end := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}

	if end == start {
		t.Fatalf("gorilla failed pounce with all landing tiles blocked but did not fall through to movement; tile=%v", end)
	}
	if end != [2]int{5, 3} {
		t.Fatalf("gorilla moved to %v after failed pounce, want one normal step to [5 3]", end)
	}
	if gorilla.PounceCDTurns != 0 {
		t.Fatalf("failed pounce should not arm TB cooldown, got %d", gorilla.PounceCDTurns)
	}
}

func TestMonsterTurnBased_Save1GorillaDoesNotFreezeDuringTwentyBackAndForthMoves(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "deep_jungle")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("deep_jungle world did not load")
	}

	tile := float64(cfg.GetTileSize())
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)
	g.camera.X, g.camera.Y = TileCenterFromTile(35, 39, tile)
	g.camera.Angle = 1.5707963267948966
	g.collisionSystem = collision.NewCollisionSystem(w, tile)
	g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	gorillaX, gorillaY := TileCenterFromTile(38, 46, tile)
	gorilla := monsterPkg.NewMonster3DFromConfig(gorillaX, gorillaY, "gorilla_titan", cfg)
	gorilla.ID = "monster_594"
	gorilla.HitPoints = 822
	gorilla.WasAttacked = true
	gorilla.IsEngagingPlayer = true
	gorilla.SummonFirstDone = true
	disableRandomBossSpecialsForTBPathTest(gorilla)
	w.Monsters = []*monsterPkg.Monster3D{gorilla}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.refreshBoundAllyCache()
	refreshTBMonsterSolidity(g)

	gl := &GameLoop{game: g}
	bounce := [][2]int{{34, 39}, {35, 39}}
	visited := make([][2]int, 0, 20)
	stuckOutOfReach := 0
	for turn := 0; turn < 20; turn++ {
		prev := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
		p := bounce[turn%len(bounce)]
		movePartyToTileForTBTest(t, g, p[0], p[1], tile)
		runFullMonsterTurnForTBTest(t, g, gl)
		refreshTBMonsterSolidity(g)

		cur := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
		visited = append(visited, cur)
		if cur == prev && !gorillaReadyToAttackOrPounceTB(g, gorilla, tile) {
			stuckOutOfReach++
		} else {
			stuckOutOfReach = 0
		}
		if stuckOutOfReach >= 2 {
			ptx, pty := g.GetPlayerTilePosition()
			t.Fatalf("gorilla froze out of reach during 20-turn back/forth at turn %d; gorilla=(%d,%d), player=(%d,%d), visited=%v",
				turn, cur[0], cur[1], ptx, pty, visited)
		}
	}
	t.Logf("gorilla stayed active during 20 back/forth TB moves; visited=%v", visited)
}

func TestMonsterTurnBased_WasAttackedBossActsAfterTransientDisengageOutsideVision(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "deep_jungle")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("deep_jungle world did not load")
	}

	tile := float64(cfg.GetTileSize())
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	g.combat = NewCombatSystem(g)
	g.camera.X, g.camera.Y = TileCenterFromTile(35, 39, tile)
	g.collisionSystem = collision.NewCollisionSystem(w, tile)
	g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	gorillaX, gorillaY := TileCenterFromTile(38, 46, tile)
	gorilla := monsterPkg.NewMonster3DFromConfig(gorillaX, gorillaY, "gorilla_titan", cfg)
	gorilla.ID = "monster_594"
	gorilla.HitPoints = 822
	gorilla.WasAttacked = true
	gorilla.IsEngagingPlayer = false // corrupt transient live state; save/load restores this from WasAttacked.
	gorilla.SummonFirstDone = true
	disableRandomBossSpecialsForTBPathTest(gorilla)
	w.Monsters = []*monsterPkg.Monster3D{gorilla}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	g.refreshBoundAllyCache()
	refreshTBMonsterSolidity(g)
	if !gorilla.BossAggro {
		t.Fatal("setup failed: WasAttacked gorilla should recompute BossAggro")
	}
	if Distance(g.camera.X, g.camera.Y, gorilla.X, gorilla.Y) <= tile*TurnBasedVisionRangeTiles {
		t.Fatal("setup failed: gorilla must be outside TB vision radius")
	}

	gl := &GameLoop{game: g}
	start := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
	runOneMonsterTurn(g, gl)
	end := [2]int{int(gorilla.X / tile), int(gorilla.Y / tile)}
	if end == start {
		t.Fatalf("WasAttacked/BossAggro gorilla outside vision was skipped after transient de-aggro; tile=%v", end)
	}
	if !gorilla.IsEngagingPlayer {
		t.Fatal("gorilla should re-enter IsEngagingPlayer after acting in TB")
	}
}

func refreshTBMonsterSolidity(g *MMGame) {
	for _, m := range g.world.Monsters {
		g.refreshMonsterCollisionSolidity(m)
	}
}

func disableRandomBossSpecialsForTBPathTest(m *monsterPkg.Monster3D) {
	if m.EnrageAtHP == 0 {
		m.EnrageAtHP = 1
	}
	m.SummonChance = 0
	m.SummonMonsters = nil
	m.TeleportChance = 0
	m.InfernoChance = 0
}

func movePartyToTileForTBTest(t *testing.T, g *MMGame, tx, ty int, tile float64) {
	t.Helper()
	x, y := TileCenterFromTile(tx, ty, tile)
	if !g.collisionSystem.CanMoveTo("player", x, y) {
		ok, reason := g.collisionSystem.DebugCanMoveTo("player", x, y)
		t.Fatalf("test party path tile (%d,%d) is blocked: ok=%v reason=%s", tx, ty, ok, reason)
	}
	g.camera.X, g.camera.Y = x, y
	g.collisionSystem.UpdateEntity("player", x, y)
	g.endPartyTurnAfterMovement()
}

func runFullMonsterTurnForTBTest(t *testing.T, g *MMGame, gl *GameLoop) {
	t.Helper()
	for frames := 0; g.currentTurn == 1 && frames < 180; frames++ {
		g.refreshBoundAllyCache()
		refreshTBMonsterSolidity(g)
		gl.updateMonstersTurnBased()
	}
	if g.currentTurn != 0 {
		t.Fatalf("monster turn did not finish; currentTurn=%d resolved=%v passesLeft=%d delay=%d",
			g.currentTurn, g.monsterTurnResolved, g.turnBasedMonsterPassesLeft, g.turnBasedMonsterPassDelay)
	}
}

func spendPartyBuffRoundForTBTest(t *testing.T, g *MMGame) {
	t.Helper()
	if g.currentTurn != 0 {
		t.Fatalf("cannot spend party buff actions outside party turn: currentTurn=%d", g.currentTurn)
	}
	g.ensureSelectedCharCanAct()
	for spent := 0; g.currentTurn == 0 && spent < 32; spent++ {
		beforeTurn := g.currentTurn
		g.consumeSelectedCharAction()
		if g.currentTurn == beforeTurn && g.partyAllExhausted() {
			t.Fatal("party exhausted without ending turn")
		}
	}
	if g.currentTurn != 1 {
		t.Fatalf("buff round did not hand control to monsters; currentTurn=%d", g.currentTurn)
	}
}

func gorillaReadyToAttackOrPounceTB(g *MMGame, gorilla *monsterPkg.Monster3D, tile float64) bool {
	ptx, pty := g.GetPlayerTilePosition()
	mtx, mty := int(gorilla.X/tile), int(gorilla.Y/tile)
	dx, dy := mtx-ptx, mty-pty
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx <= 1 && dy <= 1 && dx+dy > 0 {
		return g.collisionSystem.CheckLineOfSight(gorilla.X, gorilla.Y, g.camera.X, g.camera.Y)
	}
	if !gorilla.CanPounce() || gorilla.PounceCDTurns != 0 {
		return false
	}
	pounceTiles := int(gorilla.PounceRangePixels / tile)
	manhattan := dx + dy
	if manhattan < 2 || manhattan > pounceTiles {
		return false
	}
	for _, c := range [8][2]int{
		{ptx + 1, pty}, {ptx - 1, pty}, {ptx, pty + 1}, {ptx, pty - 1},
		{ptx + 1, pty + 1}, {ptx + 1, pty - 1}, {ptx - 1, pty + 1}, {ptx - 1, pty - 1},
	} {
		cx, cy := TileCenterFromTile(c[0], c[1], tile)
		if g.collisionSystem.CanMoveToWithHabitat(gorilla.ID, cx, cy, gorilla.HabitatPrefs, gorilla.Flying) {
			return true
		}
	}
	return false
}
