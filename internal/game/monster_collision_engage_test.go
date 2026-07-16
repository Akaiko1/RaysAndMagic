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
// range of the player must NOT be flagged engaged/solid by proximity - else a
// stacked calm band turns solid against its own members and thrashes (unstuck
// vs banding). Drives the REAL RT per-monster step (MonsterWrapper.Update, which
// updateMonstersParallel uses) - the collision-engagement side-effect my first
// headless repro missed by calling Monster3D.Update directly.
func TestPassiveRangedInRange_StaysPassThrough(t *testing.T) {
	cfg := loadTestConfig(t)
	w := world.NewWorld3D(cfg)
	openArena(w, 10)
	g := newTestGame(cfg, w)
	ts := float64(cfg.GetTileSize())

	// elf_archer: ranged (attack radius 5 tiles). Place it 3 tiles from the
	// camera/player - inside its range, like the preview stage.
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
	mw.Update()               // the real RT step, Phase 1: AI + desired collision type
	mw.ApplyCollisionUpdate() // Phase 2: write it

	if got := g.collisionSystem.GetEntityByID(m.ID).CollisionType; got != collision.CollisionTypeMonster {
		t.Fatalf("dormant passive ranged mob in range = collision type %v, want Monster (pass-through)", got)
	}

	// Once provoked it may claim a logical post, but stays physically walkable.
	m.WasAttacked = true
	mw.snapshot = g.collisionSystem.Snapshot()
	mw.Update()
	mw.ApplyCollisionUpdate()
	if got := g.collisionSystem.GetEntityByID(m.ID).CollisionType; got != collision.CollisionTypeMonsterEngaged {
		t.Fatalf("provoked ranged mob in range = collision type %v, want logical MonsterEngaged post", got)
	}
	if g.collisionSystem.GetEntityByID(m.ID).Solid {
		t.Fatal("a claimed attack post must stay physically walkable")
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

	const n = 4 // preview stages a flock; band caps at 3 -> 3 stacked + 1 solo
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

func TestMonsterWalkabilityFollowsCurrentTarget(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 3, 3, ts)
	calm := monsterPkg.NewMonster3DFromConfig(4*ts+ts/2, 3*ts+ts/2, "goblin", game.config)
	game.world.Monsters = []*monsterPkg.Monster3D{calm}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.refreshMonsterCollisionSolidity(calm)
	if entity := game.collisionSystem.GetEntityByID(calm.ID); entity == nil || entity.Solid {
		t.Fatal("peaceful monster must be walkable for the party")
	}
	if !game.collisionSystem.Snapshot().CanMoveToWithHabitat(calm.ID, game.camera.X, game.camera.Y, calm.HabitatPrefs, calm.Flying) {
		t.Fatal("peaceful monster must be able to move through the party")
	}

	calm.IsEngagingPlayer, calm.WasAttacked = true, true
	calm.State = monsterPkg.StatePursuing
	game.refreshMonsterCollisionSolidity(calm)
	if entity := game.collisionSystem.GetEntityByID(calm.ID); entity == nil || entity.Solid || entity.CollisionType != collision.CollisionTypeMonster {
		t.Fatal("a party-targeting transit mob must remain physically walkable and hold no post")
	}
	if !game.collisionSystem.Snapshot().CanMoveToWithHabitat(calm.ID, game.camera.X, game.camera.Y, calm.HabitatPrefs, calm.Flying) {
		t.Fatal("a party-targeting transit mob must move through the party")
	}

	calm.State = monsterPkg.StateAttacking
	game.refreshMonsterCollisionSolidity(calm)
	if entity := game.collisionSystem.GetEntityByID(calm.ID); entity == nil || entity.Solid || entity.CollisionType != collision.CollisionTypeMonsterEngaged {
		t.Fatal("a party attack post must be logical-only, never solid")
	}
	if !game.collisionSystem.CanMoveTo("player", calm.X, calm.Y) {
		t.Fatal("the party must be able to walk through a claimed attack post")
	}

	ally := monsterPkg.NewMonster3DFromConfig(6*ts+ts/2, 3*ts+ts/2, "masked_huntress", game.config)
	markCardAlly(ally)
	placePlayerAtTile(game, 1, 3, ts) // card ally is now closer than the party
	game.world.Monsters = []*monsterPkg.Monster3D{calm, ally}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshBoundAllyCache()
	if calm.AIFoe != ally {
		t.Fatal("setup: closer card ally should redirect the monster")
	}
	game.refreshMonsterCollisionSolidity(calm)
	if entity := game.collisionSystem.GetEntityByID(calm.ID); entity == nil || entity.Solid || entity.CollisionType != collision.CollisionTypeMonsterEngaged {
		t.Fatal("monster fighting a summon must use a non-solid logical attack post")
	}
	if !calm.AttackPost || calm.AttackPostTargetID != ally.ID {
		t.Fatalf("summon-targeting attack post = (%v, %q), want (%v, %q)", calm.AttackPost, calm.AttackPostTargetID, true, ally.ID)
	}
	if !game.collisionSystem.Snapshot().CanMoveToWithHabitat(calm.ID, game.camera.X, game.camera.Y, calm.HabitatPrefs, calm.Flying) {
		t.Fatal("monster fighting a summon must be able to route through the party")
	}
}

func TestPartyTargetingMonsterIsEjectedFromPlayerCell(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	m := monsterPkg.NewMonster3DFromConfig(game.camera.X, game.camera.Y, "goblin", game.config)
	m.IsEngagingPlayer, m.WasAttacked = true, true
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	game.refreshBoundAllyCache()
	monsterEntity := game.collisionSystem.GetEntityByID(m.ID)
	playerEntity := game.collisionSystem.GetEntityByID("player")
	if monsterEntity == nil || playerEntity == nil || monsterEntity.BoundingBox.Intersects(playerEntity.BoundingBox) {
		t.Fatal("party-targeting monster must be ejected from the player cell")
	}
	mtx, mty := monsterTileCoords(m, ts)
	ptx, pty := game.GetPlayerTilePosition()
	if mtx == ptx && mty == pty {
		t.Fatal("party-targeting monster remained on the player tile")
	}
}

func TestHitFleeingMonsterCanPathThroughParty(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	m := monsterPkg.NewMonster3DFromConfig(9*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
	m.State = monsterPkg.StateFleeing
	m.WasAttacked = true
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshMonsterCollisionSolidity(m)

	entity := game.collisionSystem.GetEntityByID(m.ID)
	if entity == nil || entity.Solid || entity.CollisionType != collision.CollisionTypeMonster {
		t.Fatal("a fleeing monster must stay physically walkable and hold no attack post")
	}
	if player := game.collisionSystem.GetEntityByID("player"); player == nil || !player.Solid {
		t.Fatal("the player must block hostile monster pathfinding")
	}
	if !game.collisionSystem.Snapshot().CanMoveToWithHabitat(m.ID, game.camera.X, game.camera.Y, m.HabitatPrefs, m.Flying) {
		t.Fatal("a fleeing monster must be able to path through the party")
	}
}

func TestTeleportFallbackOnlyProtectsPartyFromCurrentTarget(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 20, 20)
	placePlayerAtTile(game, 10, 10, ts)
	m := monsterPkg.NewMonster3DFromConfig(9*ts+ts/2, 10*ts+ts/2, "goblin", game.config)
	game.world.Monsters = []*monsterPkg.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	// A monster chasing the party must keep its fallback destination outside
	// the party tile even when that tile would be geometrically closest.
	m.IsEngagingPlayer, m.WasAttacked = true, true
	game.refreshMonsterCollisionSolidity(m)
	x, y, _ := gl.pickBestTeleportOffset(m, ts, game.camera.X, game.camera.Y, [][2]int{{1, 0}}, 1e300)
	if int(x/ts) == 10 && int(y/ts) == 10 {
		t.Fatal("party-targeting monster teleported onto the party")
	}

	// When redirected to a summon, the party is walkable in both directions.
	foe := monsterPkg.NewMonster3DFromConfig(12*ts+ts/2, 10*ts+ts/2, "masked_huntress", game.config)
	m.AIFoe = foe
	game.refreshMonsterCollisionSolidity(m)
	x, y, _ = gl.pickBestTeleportOffset(m, ts, foe.X, foe.Y, [][2]int{{1, 0}}, 1e300)
	if int(x/ts) != 10 || int(y/ts) != 10 {
		t.Fatalf("summon-targeting monster teleport = (%d,%d), want player tile (10,10)", int(x/ts), int(y/ts))
	}
}
