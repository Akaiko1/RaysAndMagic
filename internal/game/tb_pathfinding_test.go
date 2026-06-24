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
// the gap and reach the party — not oscillate at the bank, the bug that let the
// stranded gorilla be ranged down for free. Guards the greedy→A* fallthrough.
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

	gl := &GameLoop{game: g, combat: g.combat}

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
		t.Fatal("mob reached the party without ever crossing the barrier column — setup is wrong")
	}
	t.Logf("mob crossed the ford and reached the party in %d turn-based steps", steps)
}

// TestMonsterMoveTurnBased_EscapesPocketAwayFromParty reproduces the exact bug the
// real gorilla hit: it sat in a pocket whose only opening faced AWAY from the
// party (its party-side neighbor was an impassable log). A greedy-first mover
// oscillates — the naive step keeps pulling it toward the party (back into the
// pocket) while A* pulls it out the far side — so it never escapes and gets ranged
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

	gl := &GameLoop{game: g, combat: g.combat}
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
		t.Fatalf("mob never escaped the pocket / reached the party (greedy↔A* oscillation); final Manhattan=%d after %d steps", manhattan(), steps)
	}
	t.Logf("mob escaped the pocket and reached the party in %d turn-based steps", steps)
}
