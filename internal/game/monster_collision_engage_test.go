package game

import (
	"math"
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// openArena builds an NxN all-walkable world for movement/collision tests.
func openArena(w *world.World3D, n int) {
	w.Width, w.Height = n, n
	w.Tiles = make([][]world.TileType3D, n)
	for y := 0; y < n; y++ {
		w.Tiles[y] = make([]world.TileType3D, n)
		for x := 0; x < n; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
}

// TestPassiveRangedInRange_StaysPassThrough guards the map-editor mob-preview
// jitter fix: a DORMANT passive monster within its (long, for ranged) attack
// range of the player must NOT be flagged engaged/solid by proximity — else a
// stacked calm band turns solid against its own members and thrashes (unstuck
// vs banding). Drives the REAL RT per-monster step (MonsterWrapper.Update, which
// updateMonstersParallel uses) — the collision-engagement side-effect my first
// headless repro missed by calling Monster3D.Update directly.
func TestPassiveRangedInRange_StaysPassThrough(t *testing.T) {
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	openArena(w, 10)
	g := newTestGame(cfg, w)
	ts := float64(cfg.GetTileSize())

	// elf_archer: ranged (attack radius 5 tiles). Place it 3 tiles from the
	// camera/player — inside its range, like the preview stage.
	m := monsterPkg.NewMonster3DFromConfig(g.camera.X+3*ts, g.camera.Y, "elf_archer", cfg)
	if !m.HasRangedAttack() {
		t.Fatal("setup: elf_archer should be ranged")
	}
	m.PassiveUntilAttacked = true
	m.AITargetX, m.AITargetY = g.camera.X, g.camera.Y
	w.Monsters = append(w.Monsters, m)
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)

	mw := &MonsterWrapper{Monster: m, collisionSystem: g.collisionSystem, game: g}
	mw.snapshot = g.collisionSystem.Snapshot()
	mw.Update() // the real RT step, Phase 1: AI + desired collision type
	mw.ApplyCollisionUpdate() // Phase 2: write it

	if got := g.collisionSystem.GetEntityByID(m.ID).CollisionType; got != collision.CollisionTypeMonster {
		t.Fatalf("dormant passive ranged mob in range = collision type %v, want Monster (pass-through)", got)
	}

	// Once provoked it is genuinely fighting → solid, so the proximity engage
	// still works for real combatants.
	m.WasAttacked = true
	mw.snapshot = g.collisionSystem.Snapshot()
	mw.Update()
	mw.ApplyCollisionUpdate()
	if got := g.collisionSystem.GetEntityByID(m.ID).CollisionType; got != collision.CollisionTypeMonsterEngaged {
		t.Fatalf("provoked ranged mob in range = collision type %v, want MonsterEngaged (solid)", got)
	}
}

// TestPassiveRangedBand_NoTeleportThrash reproduces the preview scene (a passive
// ranged flock stacked on one tile) through the real update path and asserts the
// band doesn't teleport: pre-fix the members went solid and unstuck-vs-banding
// flung them ~a tile per pass.
func TestPassiveRangedBand_NoTeleportThrash(t *testing.T) {
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	openArena(w, 14)
	g := newTestGame(cfg, w)
	gl := &GameLoop{game: g}
	ts := float64(cfg.GetTileSize())

	const n = 4 // preview stages a flock; band caps at 3 → 3 stacked + 1 solo
	sx, sy := g.camera.X+3*ts, g.camera.Y
	wraps := make([]*MonsterWrapper, n)
	for i := 0; i < n; i++ {
		m := monsterPkg.NewMonster3DFromConfig(sx, sy, "elf_archer", cfg)
		m.PassiveUntilAttacked = true
		m.TetherRadius = 2 * ts
		m.AITargetX, m.AITargetY = g.camera.X, g.camera.Y
		w.Monsters = append(w.Monsters, m)
		wraps[i] = &MonsterWrapper{Monster: m, collisionSystem: g.collisionSystem, game: g}
	}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)

	prev := make([]struct{ x, y float64 }, n)
	for i, m := range w.Monsters {
		prev[i].x, prev[i].y = m.X, m.Y
	}
	maxJump := 0.0
	for tick := 0; tick < 200; tick++ {
		// Mirror the real two-phase RT tick: one snapshot shared by every
		// wrapper's Update() (Phase 1), then apply all the writes (Phase 2).
		snapshot := g.collisionSystem.Snapshot()
		for _, mw := range wraps {
			mw.snapshot = snapshot
			mw.Update()
		}
		for _, mw := range wraps {
			mw.ApplyCollisionUpdate()
		}
		gl.separateOverlappingMonsters()
		gl.updateMonsterBands()
		for i, m := range w.Monsters {
			if tick > 2 { // skip the initial band-formation snap
				if d := math.Hypot(m.X-prev[i].x, m.Y-prev[i].y); d > maxJump {
					maxJump = d
				}
			}
			prev[i].x, prev[i].y = m.X, m.Y
		}
	}
	// Calm patrol moves at most a few px/tick; a thrash was ~a full tile (56px+).
	if maxJump > float64(cfg.GetTileSize())/2 {
		t.Fatalf("passive ranged band teleported %.0fpx/tick (thrash); want calm (<%.0f)", maxJump, ts/2)
	}
}
