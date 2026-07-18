package game

import (
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// tbSepGame builds a small open world in turn-based mode with two live monsters
// registered with the collision system at the given pixel positions.
func tbSepGame(t *testing.T, ax, ay, bx, by float64) (*MMGame, *monsterPkg.Monster3D, *monsterPkg.Monster3D) {
	t.Helper()
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 8, 8
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	g := newTestGame(cfg, w)
	g.turnBasedMode = true

	mk := func(id string, x, y float64) *monsterPkg.Monster3D {
		m := &monsterPkg.Monster3D{ID: id, Name: "Bandit", X: x, Y: y, HitPoints: 100, MaxHitPoints: 100, IsEngagingPlayer: true}
		w.Monsters = append(w.Monsters, m)
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, x, y, 48, 48, collision.CollisionTypeMonster, false))
		return m
	}
	a := mk("mob_a", ax, ay)
	b := mk("mob_b", bx, by)
	return g, a, b
}

func tileOf(m *monsterPkg.Monster3D, tile float64) [2]int {
	return [2]int{int(m.X / tile), int(m.Y / tile)}
}

// TestSeparateStackedMonstersTB_SplitsAndIsIdempotent covers the fix: two mobs
// the real-time push left stacked on one tile are pulled onto DISTINCT tiles in
// turn-based mode, and a second pass leaves them put (idempotent = no jitter).
func TestSeparateStackedMonstersTB_SplitsAndIsIdempotent(t *testing.T) {
	// Both stacked on tile (3,3) center (tileSize 64 -> 3*64+32 = 224).
	g, a, b := tbSepGame(t, 224, 224, 224, 224)
	tile := float64(g.config.GetTileSize())

	g.separateStackedMonstersTB()

	ta, tb := tileOf(a, tile), tileOf(b, tile)
	if ta == tb {
		t.Fatalf("stacked mobs should end on distinct tiles, both on %v", ta)
	}
	// Both must sit on a tile CENTRE so a row/column shot connects.
	for _, m := range []*monsterPkg.Monster3D{a, b} {
		cx, cy := TileCenterFromTile(int(m.X/tile), int(m.Y/tile), tile)
		if m.X != cx || m.Y != cy {
			t.Errorf("%s off-centre at (%.0f,%.0f), want (%.0f,%.0f)", m.ID, m.X, m.Y, cx, cy)
		}
	}

	// Idempotence: already separated -> a second pass moves nobody (no jitter).
	ax, ay, bx, by := a.X, a.Y, b.X, b.Y
	g.separateStackedMonstersTB()
	if a.X != ax || a.Y != ay || b.X != bx || b.Y != by {
		t.Errorf("second pass moved a settled pair (jitter): a (%.0f,%.0f)->(%.0f,%.0f) b (%.0f,%.0f)->(%.0f,%.0f)",
			ax, ay, a.X, a.Y, bx, by, b.X, b.Y)
	}
}

// TestSeparateStackedMonstersTB_HalfTileOffsetSameTile covers the exact bug the
// player hit: the RT pixel push left the pair half a tile apart but on the SAME
// tile - TB centring alone would stack them, so they must still be split.
func TestSeparateStackedMonstersTB_HalfTileOffsetSameTile(t *testing.T) {
	// Both on tile (3,3): one at centre, one ~half a tile off but same tile int.
	g, a, b := tbSepGame(t, 224, 224, 224+24, 224)
	tile := float64(g.config.GetTileSize())
	if tileOf(a, tile) != tileOf(b, tile) {
		t.Fatalf("setup: mobs should start on the same tile")
	}
	g.separateStackedMonstersTB()
	if tileOf(a, tile) == tileOf(b, tile) {
		t.Fatalf("half-offset same-tile pair must be split onto distinct tiles")
	}
}

func TestSeparateStackedMonstersTB_PreservesOnlyCalmSocialStacks(t *testing.T) {
	g, a, b := tbSepGame(t, 224, 224, 224, 224)
	tile := float64(g.config.GetTileSize())
	for _, m := range []*monsterPkg.Monster3D{a, b} {
		m.Banding = true
		m.IsEngagingPlayer = false
		m.State = monsterPkg.StateIdle
	}

	g.separateStackedMonstersTB()
	if tileOf(a, tile) != tileOf(b, tile) {
		t.Fatal("a calm social band must remain stacked in turn-based mode")
	}

	for _, m := range []*monsterPkg.Monster3D{a, b} {
		m.WasAttacked = true
		m.State = monsterPkg.StateFleeing
	}
	g.separateStackedMonstersTB()
	if tileOf(a, tile) == tileOf(b, tile) {
		t.Fatal("a non-calm band member must no longer preserve an intentional stack")
	}
}
