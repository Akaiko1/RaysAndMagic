package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func newLootGuardTestGame(t *testing.T, turnBased bool) (*MMGame, *GameLoop, float64) {
	t.Helper()
	game, loop, tile := tbBehaviorGame(t, 24, 24)
	game.turnBasedMode = turnBased
	game.gameLoop = loop
	placePlayerAtTile(game, 1, 1, tile)
	return game, loop, tile
}

func addLootGuardTarget(game *MMGame, key, npcType string, tileX, tileY int, tile float64) *character.NPC {
	x, y := TileCenterFromTile(tileX, tileY, tile)
	target := &character.NPC{Key: key, Type: npcType, X: x, Y: y}
	game.world.NPCs = append(game.world.NPCs, target)
	return target
}

func addLootGuardTestMonster(t *testing.T, game *MMGame, id, key string, tileX, tileY int, tile float64) *monster.Monster3D {
	t.Helper()
	x, y := TileCenterFromTile(tileX, tileY, tile)
	m := monster.NewMonster3DFromConfig(x, y, key, game.config)
	if m == nil {
		t.Fatalf("monster %q is missing from monsters.yaml", key)
	}
	m.ID = id
	m.State = monster.StatePatrolling
	game.world.Monsters = append(game.world.Monsters, m)
	return m
}

func prepareLootGuardsForTest(game *MMGame, loop *GameLoop) {
	game.refreshBoundAllyCache()
	loop.prepareLootPropGuards()
}

func assertLootGuardTarget(t *testing.T, m *monster.Monster3D, target *character.NPC, tile float64) {
	t.Helper()
	if !m.LootGuarding {
		t.Fatalf("%s is not guarding", m.ID)
	}
	targetX, targetY := int(target.X/tile), int(target.Y/tile)
	if m.LootGuardTargetTileX != targetX || m.LootGuardTargetTileY != targetY || m.LootGuardTargetKey != target.Key {
		t.Fatalf("%s guard target = %q (%d,%d), want %q (%d,%d)",
			m.ID, m.LootGuardTargetKey, m.LootGuardTargetTileX, m.LootGuardTargetTileY,
			target.Key, targetX, targetY)
	}
	dx, dy := m.LootGuardMoveTileX-targetX, m.LootGuardMoveTileY-targetY
	if dx == 0 && dy == 0 || dx*dx+dy*dy > 4 {
		t.Fatalf("%s guard move tile = (%d,%d), want a patrol tile within radius two of (%d,%d)",
			m.ID, m.LootGuardMoveTileX, m.LootGuardMoveTileY, targetX, targetY)
	}
}

func setupLootGuardPair(t *testing.T, turnBased bool) (*MMGame, *GameLoop, float64, *character.NPC, *monster.Monster3D, *monster.Monster3D) {
	t.Helper()
	game, loop, tile := newLootGuardTestGame(t, turnBased)
	target := addLootGuardTarget(game, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
	first := addLootGuardTestMonster(t, game, "guard-a", "goblin", 6, 10, tile)
	second := addLootGuardTestMonster(t, game, "guard-b", "rat", 6, 11, tile)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	prepareLootGuardsForTest(game, loop)
	if !first.LootGuarding || !second.LootGuarding {
		t.Fatal("setup: expected both nearby solo mobs to reserve the crate")
	}
	return game, loop, tile, target, first, second
}

func runLootGuardRealTimeStep(game *MMGame, loop *GameLoop) {
	snapshot := game.collisionSystem.Snapshot()
	wrappers := make([]*MonsterWrapper, 0, len(game.world.Monsters))
	for _, m := range game.world.Monsters {
		wrappers = append(wrappers, &MonsterWrapper{
			Monster: m, collisionSystem: game.collisionSystem, snapshot: snapshot, game: game,
		})
	}
	for _, wrapper := range wrappers {
		wrapper.Update()
	}
	for _, wrapper := range wrappers {
		wrapper.ApplyCollisionUpdate()
	}
	loop.reconcileLootPropGuardBands()
}

func TestLootGuardsReserveMixedPairThenStackAtPost(t *testing.T) {
	game, loop, tile := newLootGuardTestGame(t, false)
	target := addLootGuardTarget(game, "guarded_lectern", character.NPCTypeSpellLectern, 10, 10, tile)
	first := addLootGuardTestMonster(t, game, "a-goblin", "goblin", 6, 10, tile)
	second := addLootGuardTestMonster(t, game, "b-rat", "rat", 6, 11, tile)
	third := addLootGuardTestMonster(t, game, "c-wolf", "wolf", 6, 9, tile)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	prepareLootGuardsForTest(game, loop)

	if first.Key == second.Key {
		t.Fatal("test setup needs different monster types")
	}
	for _, guard := range []*monster.Monster3D{first, second} {
		assertLootGuardTarget(t, guard, target, tile)
		if guard.BandID != 0 || guard.BandStackCount != 0 {
			t.Fatalf("%s was stacked before reaching the post: id:%d count:%d", guard.ID, guard.BandID, guard.BandStackCount)
		}
	}
	if third.LootGuarding {
		t.Fatal("third nearby mob joined a full two-slot guard band")
	}
	postX, postY := first.LootGuardMoveTileX, first.LootGuardMoveTileY
	if second.LootGuardMoveTileX != postX || second.LootGuardMoveTileY != postY {
		t.Fatalf("reserved pair received different posts: (%d,%d) vs (%d,%d)",
			postX, postY, second.LootGuardMoveTileX, second.LootGuardMoveTileY)
	}
	pendingLeader := stableLootGuardLeader([]*monster.Monster3D{first, second})
	pendingLeader.X, pendingLeader.Y = TileCenterFromTile(postX, postY, tile)
	game.collisionSystem.UpdateEntity(pendingLeader.ID, pendingLeader.X, pendingLeader.Y)
	prepareLootGuardsForTest(game, loop)
	if pendingLeader.LootGuardMoveTileX != postX || pendingLeader.LootGuardMoveTileY != postY {
		t.Fatal("pending guard pair changed posts before the second guard arrived")
	}
	for _, guard := range []*monster.Monster3D{first, second} {
		guard.X, guard.Y = TileCenterFromTile(postX, postY, tile)
		game.collisionSystem.UpdateEntity(guard.ID, guard.X, guard.Y)
	}
	loop.reconcileLootPropGuardBands()
	for _, guard := range []*monster.Monster3D{first, second} {
		if guard.BandID == 0 || guard.BandStackCount != maxLootGuardBandSize {
			t.Fatalf("%s did not stack after reaching the post: id:%d count:%d", guard.ID, guard.BandID, guard.BandStackCount)
		}
	}
	if first.BandID != second.BandID {
		t.Fatalf("mixed guard pair has different band IDs: %d vs %d", first.BandID, second.BandID)
	}

	leader := first
	if second.BandStackIndex == 0 {
		leader = second
	}
	oldX, oldY := leader.LootGuardMoveTileX, leader.LootGuardMoveTileY
	leader.X, leader.Y = TileCenterFromTile(oldX, oldY, tile)
	game.collisionSystem.UpdateEntity(leader.ID, leader.X, leader.Y)
	prepareLootGuardsForTest(game, loop)
	if leader.LootGuardPatrolUntil <= game.frameCount {
		t.Fatal("stacked guard did not start its pause at the patrol post")
	}
	game.frameCount = leader.LootGuardPatrolUntil
	prepareLootGuardsForTest(game, loop)
	if leader.LootGuardMoveTileX == oldX && leader.LootGuardMoveTileY == oldY {
		t.Fatal("guard did not advance through the radius-two patrol")
	}
	assertLootGuardTarget(t, leader, target, tile)

	// Ordinary same-key banding must leave the mixed guard pair alone.
	loop.updateMonsterBands()
	if !first.LootGuarding || !second.LootGuarding || first.BandID != second.BandID {
		t.Fatal("ordinary banding changed the active mixed guard pair")
	}
}

func TestLootGuardDoesNotClaimMobsAlreadyInAnotherBand(t *testing.T) {
	game, loop, tile := newLootGuardTestGame(t, false)
	_ = addLootGuardTarget(game, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
	bandA := addLootGuardTestMonster(t, game, "band-a", "goblin", 6, 10, tile)
	bandB := addLootGuardTestMonster(t, game, "band-b", "goblin", 6, 11, tile)
	free := addLootGuardTestMonster(t, game, "free", "rat", 6, 9, tile)
	for _, m := range []*monster.Monster3D{bandA, bandB} {
		m.BandID, m.BandLeaderID, m.BandStackCount = 71, "band-a", 2
	}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	prepareLootGuardsForTest(game, loop)

	if bandA.LootGuarding || bandB.LootGuarding {
		t.Fatal("an existing normal band was recruited as crate guards")
	}
	if !free.LootGuarding {
		t.Fatal("a nearby solo mob should still be able to guard the crate")
	}
}

func TestLootGuardsFillNearbyTargetsIndependently(t *testing.T) {
	tests := []struct {
		name    string
		targets []struct {
			key, kind string
			x, y      int
			want      int
		}
		guards []struct {
			id   string
			x, y int
		}
	}{
		{
			name: "two crates fill two plus two",
			targets: []struct {
				key, kind string
				x, y      int
				want      int
			}{
				{key: "crate-a", kind: character.NPCTypeLootCrate, x: 8, y: 10, want: 2},
				{key: "crate-b", kind: character.NPCTypeLootCrate, x: 16, y: 10, want: 2},
			},
			guards: []struct {
				id   string
				x, y int
			}{
				{id: "a-0", x: 5, y: 10}, {id: "a-1", x: 6, y: 9},
				{id: "b-0", x: 18, y: 10}, {id: "b-1", x: 19, y: 9},
			},
		},
		{
			name: "three crates fill two plus two plus one",
			targets: []struct {
				key, kind string
				x, y      int
				want      int
			}{
				{key: "crate-a", kind: character.NPCTypeLootCrate, x: 6, y: 10, want: 2},
				{key: "crate-b", kind: character.NPCTypeLootCrate, x: 10, y: 10, want: 2},
				{key: "lectern-c", kind: character.NPCTypeSpellLectern, x: 14, y: 10, want: 1},
			},
			guards: []struct {
				id   string
				x, y int
			}{
				{id: "a-0", x: 3, y: 10}, {id: "a-1", x: 4, y: 9},
				{id: "b-0", x: 9, y: 10}, {id: "b-1", x: 9, y: 9},
				{id: "c-0", x: 16, y: 10},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			game, loop, tile := newLootGuardTestGame(t, false)
			for _, target := range tc.targets {
				addLootGuardTarget(game, target.key, target.kind, target.x, target.y, tile)
			}
			for _, guard := range tc.guards {
				addLootGuardTestMonster(t, game, guard.id, "goblin", guard.x, guard.y, tile)
			}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			prepareLootGuardsForTest(game, loop)

			counts := make(map[string]int)
			for _, guard := range game.world.Monsters {
				if !guard.LootGuarding {
					t.Fatalf("%s was left unassigned despite an open nearby guard slot", guard.ID)
				}
				counts[guard.LootGuardTargetKey]++
			}
			for _, target := range tc.targets {
				if got := counts[target.key]; got != target.want {
					t.Fatalf("%s has %d guards, want %d; all counts: %v", target.key, got, target.want, counts)
				}
			}

			// Let every reserved group reach its own post, then verify that only
			// the two members assigned to the same prop become a physical band.
			targetByID, targets := loop.activeLootGuardTargets()
			groups := loop.lootGuardGroups(targetByID)
			for _, target := range targets {
				for _, guard := range groups[target.id] {
					guard.X, guard.Y = TileCenterFromTile(guard.LootGuardMoveTileX, guard.LootGuardMoveTileY, tile)
					game.collisionSystem.UpdateEntity(guard.ID, guard.X, guard.Y)
				}
			}
			loop.reconcileLootPropGuardBands()

			groups = loop.lootGuardGroups(targetByID)
			bandOwners := make(map[int]string)
			postOwners := make(map[lootGuardPostID]string)
			for _, target := range targets {
				members := groups[target.id]
				if leader := stableLootGuardLeader(members); leader != nil {
					post := lootGuardPostID{tileX: leader.LootGuardMoveTileX, tileY: leader.LootGuardMoveTileY}
					if previous, exists := postOwners[post]; exists && previous != target.id.key {
						t.Fatalf("different props reserved the same patrol post (%d,%d): %s and %s", post.tileX, post.tileY, previous, target.id.key)
					}
					postOwners[post] = target.id.key
				}
				if len(members) == maxLootGuardBandSize {
					if !lootGuardBandIsStacked(members) {
						t.Fatalf("%s guards did not stack together", target.id.key)
					}
					if previous, exists := bandOwners[members[0].BandID]; exists && previous != target.id.key {
						t.Fatalf("guard band %d merged %s and %s", members[0].BandID, previous, target.id.key)
					}
					bandOwners[members[0].BandID] = target.id.key
				} else if len(members) == 1 && members[0].BandID != 0 {
					t.Fatalf("single guard for %s incorrectly formed a band", target.id.key)
				}
			}
		})
	}
}

func TestNearbyLootTargetsReserveDifferentPatrolPosts(t *testing.T) {
	game, loop, tile := newLootGuardTestGame(t, false)
	addLootGuardTarget(game, "crate-left", character.NPCTypeLootCrate, 10, 10, tile)
	addLootGuardTarget(game, "crate-right", character.NPCTypeLootCrate, 12, 10, tile)
	for _, guard := range []struct {
		id   string
		x, y int
	}{
		{id: "left-a", x: 8, y: 10}, {id: "left-b", x: 8, y: 9},
		{id: "right-a", x: 14, y: 10}, {id: "right-b", x: 14, y: 9},
	} {
		addLootGuardTestMonster(t, game, guard.id, "goblin", guard.x, guard.y, tile)
	}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	prepareLootGuardsForTest(game, loop)

	// Both route cursors deliberately point at the OTHER crate's tile: east-outer
	// of the left crate and west-outer of the right one. Both pairs must take
	// free posts instead of standing on a crate or piling four guards together.
	targetByID, targets := loop.activeLootGuardTargets()
	groups := loop.lootGuardGroups(targetByID)
	for _, target := range targets {
		members := groups[target.id]
		if len(members) != maxLootGuardBandSize {
			t.Fatalf("%s has %d guards, want two", target.id.key, len(members))
		}
		side, alt := 0, true
		if target.id.key == "crate-right" {
			side, alt = 3, false
		}
		for _, guard := range members {
			guard.LootGuardSide = side
			guard.LootGuardPatrolAlt = alt
		}
	}
	prepareLootGuardsForTest(game, loop)

	groups = loop.lootGuardGroups(targetByID)
	posts := make(map[lootGuardPostID]string)
	targetTiles := lootGuardTargetReservations(targetByID)
	for _, target := range targets {
		leader := stableLootGuardLeader(groups[target.id])
		if leader == nil {
			t.Fatalf("%s lost its guard leader", target.id.key)
		}
		post := lootGuardPostID{tileX: leader.LootGuardMoveTileX, tileY: leader.LootGuardMoveTileY}
		if previous, exists := posts[post]; exists {
			t.Fatalf("nearby crates shared patrol post (%d,%d): %s and %s", post.tileX, post.tileY, previous, target.id.key)
		}
		if _, isTargetTile := targetTiles[post]; isTargetTile {
			t.Fatalf("%s chose loot prop tile (%d,%d) as a patrol post", target.id.key, post.tileX, post.tileY)
		}
		posts[post] = target.id.key
	}
}

func TestLootGuardReleasesSpentProp(t *testing.T) {
	game, loop, _, target, first, second := setupLootGuardPair(t, false)
	target.Visited = true

	prepareLootGuardsForTest(game, loop)

	for _, m := range []*monster.Monster3D{first, second} {
		if m.LootGuarding || m.BandID != 0 {
			t.Errorf("%s kept guarding a spent prop: guard=%v band=%d", m.ID, m.LootGuarding, m.BandID)
		}
	}
}

func TestLootGuardAttackScattersTheWholePair(t *testing.T) {
	game, loop, _, _, first, second := setupLootGuardPair(t, false)
	secondX, secondY := second.X, second.Y
	first.TakeDamage(1, monster.DamagePhysical)

	prepareLootGuardsForTest(game, loop)

	for _, m := range []*monster.Monster3D{first, second} {
		if m.LootGuarding || m.BandID != 0 {
			t.Errorf("%s remained in the guard band after a direct attack", m.ID)
		}
		if !m.IsEngagingPlayer || !m.WasAttacked {
			t.Errorf("%s did not inherit hostile hit state: engaging=%v hit=%v", m.ID, m.IsEngagingPlayer, m.WasAttacked)
		}
	}
	if second.X != secondX || second.Y != secondY {
		t.Fatalf("pending guard was teleported while scattering: got (%.1f,%.1f), want (%.1f,%.1f)", second.X, second.Y, secondX, secondY)
	}
}

func TestStackedLootGuardPairScattersWhenBothAlertInSameTick(t *testing.T) {
	game, loop, tile, _, first, second := setupLootGuardPair(t, false)
	postX, postY := first.LootGuardMoveTileX, first.LootGuardMoveTileY
	for _, guard := range []*monster.Monster3D{first, second} {
		guard.X, guard.Y = TileCenterFromTile(postX, postY, tile)
		game.collisionSystem.UpdateEntity(guard.ID, guard.X, guard.Y)
	}
	loop.reconcileLootPropGuardBands()
	if !lootGuardBandIsStacked([]*monster.Monster3D{first, second}) {
		t.Fatal("setup: guard pair did not form a stacked band")
	}

	for _, guard := range []*monster.Monster3D{first, second} {
		guard.IsEngagingPlayer = true
		guard.State = monster.StateAlert
	}
	loop.reconcileLootPropGuardBands()

	positions := map[[2]int]bool{}
	for _, guard := range []*monster.Monster3D{first, second} {
		if guard.LootGuarding || guard.BandID != 0 || !guard.IsEngagingPlayer {
			t.Fatalf("%s did not dissolve the alerted guard band: guarding=%v band=%d engaging=%v",
				guard.ID, guard.LootGuarding, guard.BandID, guard.IsEngagingPlayer)
		}
		pos := [2]int{int(guard.X / tile), int(guard.Y / tile)}
		if positions[pos] {
			t.Fatalf("alerted guard pair remained stacked on tile %v", pos)
		}
		positions[pos] = true
	}
}

func TestLootGuardUsesSevenTileSightInRealTime(t *testing.T) {
	game, loop, tile := newLootGuardTestGame(t, false)
	_ = addLootGuardTarget(game, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
	guard := addLootGuardTestMonster(t, game, "guard", "goblin", 8, 10, tile)
	guard.AlertRadius = 2 * tile
	// The post is deliberately far beyond the monster's ordinary tether. Guard
	// sight must still be exactly seven tiles, not the outside-tether multiplier.
	guard.SpawnX, guard.SpawnY = guard.X-10*tile, guard.Y
	guard.TetherRadius = tile
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	prepareLootGuardsForTest(game, loop)
	game.camera.X, game.camera.Y = guard.X-6.5*tile, guard.Y
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
	game.refreshBoundAllyCache()

	radius, _ := guard.PlayerDetectionRange(game.collisionSystem, game.camera.X, game.camera.Y)
	if radius != monster.LootGuardMinAlertTiles*tile {
		t.Fatalf("outside-tether guard sight = %.1f, want %.1f", radius, monster.LootGuardMinAlertTiles*tile)
	}
	runLootGuardRealTimeStep(game, loop)

	if guard.LootGuarding || !guard.IsEngagingPlayer {
		t.Fatalf("guard ignored party inside the seven-tile guard radius: guard=%v engaging=%v", guard.LootGuarding, guard.IsEngagingPlayer)
	}
}

func TestLootGuardPatrolSkipsBlockedCellsWithinRadiusTwo(t *testing.T) {
	game, loop, tile := newLootGuardTestGame(t, false)
	target := addLootGuardTarget(game, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
	guard := addLootGuardTestMonster(t, game, "guard", "goblin", 8, 10, tile)
	game.world.Tiles[10][11] = world.TileWall // first route point: east inner
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	_, _, moveX, moveY, ok := loop.lootGuardPatrolTile(lootGuardTarget{
		id: lootGuardTargetID{key: target.Key, tileX: 10, tileY: 10}, npc: target,
	}, []*monster.Monster3D{guard}, 0, false)
	if !ok {
		t.Fatal("guard found no alternate patrol tile")
	}
	if moveX == 11 && moveY == 10 {
		t.Fatal("guard selected the blocked first patrol tile")
	}
	dx, dy := moveX-10, moveY-10
	if dx == 0 && dy == 0 || dx*dx+dy*dy > 4 {
		t.Fatalf("guard selected (%d,%d), outside patrol radius two", moveX, moveY)
	}
	x, y := TileCenterFromTile(moveX, moveY, tile)
	if !game.collisionSystem.CanMoveToWithHabitat(guard.ID, x, y, guard.HabitatPrefs, guard.Flying) {
		t.Fatalf("guard selected non-walkable patrol tile (%d,%d)", moveX, moveY)
	}
}

func TestLootGuardTurnBasedUsesSevenTileBoundary(t *testing.T) {
	setup := func(t *testing.T, distanceTiles float64) (*MMGame, *GameLoop, *monster.Monster3D, float64) {
		t.Helper()
		game, loop, tile := newLootGuardTestGame(t, true)
		_ = addLootGuardTarget(game, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
		guard := addLootGuardTestMonster(t, game, "guard", "goblin", 8, 10, tile)
		guard.AlertRadius = 2 * tile
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		prepareLootGuardsForTest(game, loop)
		game.camera.X, game.camera.Y = guard.X-distanceTiles*tile, guard.Y
		game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
		game.refreshBoundAllyCache()
		return game, loop, guard, tile
	}

	t.Run("outside seven", func(t *testing.T) {
		game, loop, guard, _ := setup(t, 7.5)
		startX, startY := guard.X, guard.Y
		runOneMonsterTurn(game, loop)
		if guard.IsEngagingPlayer || !guard.LootGuarding {
			t.Fatalf("guard outside seven tiles changed combat state: guard=%v engaging=%v", guard.LootGuarding, guard.IsEngagingPlayer)
		}
		if guard.X != startX || guard.Y != startY {
			t.Fatalf("distant TB guard moved outside the normal activation bubble: got (%.1f,%.1f), want (%.1f,%.1f)", guard.X, guard.Y, startX, startY)
		}
	})

	t.Run("inside seven", func(t *testing.T) {
		game, loop, guard, _ := setup(t, 6.5)
		runOneMonsterTurn(game, loop)
		if !guard.IsEngagingPlayer {
			t.Fatal("guard inside seven tiles did not engage in turn-based mode")
		}
		loop.reconcileLootPropGuardBands()
		if guard.LootGuarding {
			t.Fatal("engaged turn-based guard stayed assigned to the prop")
		}
	})
}

func TestLootGuardTurnBasedStackedPairMovesThroughLeaderOnly(t *testing.T) {
	game, loop, tile, _, first, second := setupLootGuardPair(t, true)
	postX, postY := first.LootGuardMoveTileX, first.LootGuardMoveTileY
	for _, guard := range []*monster.Monster3D{first, second} {
		guard.X, guard.Y = TileCenterFromTile(postX, postY, tile)
		guard.AlertRadius = 2 * tile
		game.collisionSystem.UpdateEntity(guard.ID, guard.X, guard.Y)
	}
	loop.reconcileLootPropGuardBands()

	leader, follower := first, second
	if second.BandStackIndex == 0 {
		leader, follower = second, first
	}
	if leader.BandID == 0 || follower.BandID != leader.BandID {
		t.Fatal("setup: guards did not form one positional band at their post")
	}

	startLeaderX, startLeaderY := leader.X, leader.Y
	startFollowerX, startFollowerY := follower.X, follower.Y
	loop.moveLootGuardTurnBased(leader) // at the post: choose the next patrol tile
	if leader.X != startLeaderX || leader.Y != startLeaderY || follower.X != startFollowerX || follower.Y != startFollowerY {
		t.Fatal("stacked guard pair moved while choosing its next TB patrol tile")
	}

	loop.moveLootGuardTurnBased(leader)
	leaderMoved := absInt(int(leader.X/tile)-int(startLeaderX/tile)) + absInt(int(leader.Y/tile)-int(startLeaderY/tile))
	if leaderMoved != 1 {
		t.Fatalf("TB guard leader moved %d tiles, want exactly one", leaderMoved)
	}
	if follower.X != startFollowerX || follower.Y != startFollowerY {
		t.Fatal("TB guard follower acted independently instead of following its leader")
	}

	loop.reconcileLootPropGuardBands()
	if follower.X != leader.X || follower.Y != leader.Y {
		t.Fatalf("guard follower was not re-stacked after leader movement: follower=(%.1f,%.1f) leader=(%.1f,%.1f)",
			follower.X, follower.Y, leader.X, leader.Y)
	}
}

func TestLootGuardTurnBasedPendingLeaderKeepsPostUntilPartnerArrives(t *testing.T) {
	game, loop, tile, _, first, second := setupLootGuardPair(t, true)
	leader := stableLootGuardLeader([]*monster.Monster3D{first, second})
	postX, postY := leader.LootGuardMoveTileX, leader.LootGuardMoveTileY
	leader.X, leader.Y = TileCenterFromTile(postX, postY, tile)
	leader.AlertRadius = 2 * tile
	second.AlertRadius = 2 * tile
	game.collisionSystem.UpdateEntity(leader.ID, leader.X, leader.Y)

	loop.moveLootGuardTurnBased(leader)
	if leader.LootGuardMoveTileX != postX || leader.LootGuardMoveTileY != postY {
		t.Fatal("pending TB guard leader changed posts before its partner arrived")
	}
	if first.BandID != 0 || second.BandID != 0 {
		t.Fatal("pending TB guards became a physical band before meeting at the post")
	}
}

func TestLootGuardDeathScattersSurvivor(t *testing.T) {
	game, _, _, _, first, second := setupLootGuardPair(t, false)
	first.HitPoints = 0
	game.combat.finishMonsterKill(first)

	if second.LootGuarding || second.BandID != 0 {
		t.Fatalf("survivor stayed in guard band after partner death: guard=%v band=%d", second.LootGuarding, second.BandID)
	}
	if !second.IsEngagingPlayer || !second.WasAttacked {
		t.Fatalf("survivor did not become sticky-hostile: engaging=%v hit=%v", second.IsEngagingPlayer, second.WasAttacked)
	}
}

func TestSaveLoadPreservesLootGuardReservation(t *testing.T) {
	cfg := loadTestConfig(t)
	wSave := newTestWorldSized(cfg, 24, 24)
	wmSave := world.NewWorldManager(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": wSave}
	wmSave.CurrentMapKey = "forest"

	saveGame := newTestGame(cfg, wSave)
	tile := float64(cfg.GetTileSize())
	target := addLootGuardTarget(saveGame, "guarded_crate", character.NPCTypeLootCrate, 10, 10, tile)
	guard := addLootGuardTestMonster(t, saveGame, "saved-guard", "goblin", 9, 10, tile)
	guard.LootGuarding = true
	guard.LootGuardTargetKey = target.Key
	guard.LootGuardTargetTileX, guard.LootGuardTargetTileY = 10, 10
	guard.LootGuardSide = 2
	guard.LootGuardPatrolAlt = true

	save := saveGame.buildSave(wmSave)
	saved := save.MapMonsters["forest"]
	if len(saved) != 1 || !saved[0].LootGuarding || saved[0].LootGuardTargetKey != target.Key ||
		saved[0].LootGuardTargetTileX != 10 || saved[0].LootGuardTargetTileY != 10 ||
		saved[0].LootGuardSide != 2 || !saved[0].LootGuardPatrolAlt {
		t.Fatalf("guard reservation was not serialized: %+v", saved)
	}

	wLoad := newTestWorldSized(cfg, 24, 24)
	wmLoad := world.NewWorldManager(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
	wmLoad.CurrentMapKey = "forest"
	loadGame := newTestGame(cfg, wLoad)
	loadGame.combat = NewCombatSystem(loadGame)
	_ = addLootGuardTarget(loadGame, target.Key, character.NPCTypeLootCrate, 10, 10, tile)

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmLoad
	defer func() { world.GlobalWorldManager = oldWorldManager }()
	if err := loadGame.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if len(wLoad.Monsters) != 1 {
		t.Fatalf("loaded monsters = %d, want 1", len(wLoad.Monsters))
	}
	loaded := wLoad.Monsters[0]
	if !loaded.LootGuarding || loaded.LootGuardTargetKey != target.Key ||
		loaded.LootGuardTargetTileX != 10 || loaded.LootGuardTargetTileY != 10 ||
		loaded.LootGuardSide != 2 || !loaded.LootGuardPatrolAlt {
		t.Fatalf("guard reservation was not restored: %+v", loaded)
	}

	loop := &GameLoop{game: loadGame}
	loadGame.gameLoop = loop
	prepareLootGuardsForTest(loadGame, loop)
	assertLootGuardTarget(t, loaded, wLoad.NPCs[0], tile)
}
