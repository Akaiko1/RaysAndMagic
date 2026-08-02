package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// stepMonstersRT runs one production-shaped RT monster tick in game-loop order:
// the serial AI-target refresh and post reconciliation, then wrappers compute
// against ONE frozen snapshot and their positions/collision types are committed
// serially (the UpdateMonstersParallel contract, without the worker pool),
// then the post-movement reconciliation arbitrates shared tiles.
func stepMonstersRT(game *MMGame, gl *GameLoop) {
	game.refreshMonsterAIState()
	gl.reconcileMonsterAttackPosts()
	wrappers := game.ConvertMonstersToWrappers()
	for _, w := range wrappers {
		if w.IsAlive() {
			w.Update()
		}
	}
	for _, w := range wrappers {
		if w.IsAlive() {
			w.ApplyCollisionUpdate()
		}
	}
	gl.reconcileMonsterAttackPosts()
}

// Two stacked melee pursuers on the only open tile next to a dead-end party -
// the whole attack-post contract in one scene: exactly one wins the post and
// attacks; the loser is marked AttackTransit, the attack gate stays shut for
// it and open for the holder; the loser leaves the shared tile quickly (second
// ring) and never re-stacks onto the holder.
func TestRT_StackedMeleeExitsToSecondRing(t *testing.T) {
	const ptx, pty = 15, 15
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	cs := game.combat
	placePlayerAtTile(game, ptx, pty, ts)

	// Dead end: every tile around the party is a wall except the north one.
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 || (dx == 0 && dy == -1) {
				continue
			}
			game.world.Tiles[pty+dy][ptx+dx] = world.TileWall
		}
	}

	// spawnMonsterAtTile REPLACES world.Monsters; rebuild the list with both.
	m1 := spawnMonsterAtTile(game, "dust_slime", ptx, pty-1, ts)
	m2 := spawnMonsterAtTile(game, "dust_slime", ptx, pty-1, ts)
	game.world.Monsters = []*monster.Monster3D{m1, m2}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	for _, m := range []*monster.Monster3D{m1, m2} {
		m.State = 2 // StatePursuing
		m.AttackCDFrames = 0
	}

	hp0 := partyHPSum(game)
	separatedAt := -1
	restacks, moves := 0, 0
	lastX2, lastY2 := -1, -1
	gateShutForTransit := false
	gateOpenForHolder := false
	// 3s at 120 TPS: separation lands ~tick 59 and the party is still alive at
	// the end - after a party wipe posts dissolve and free wandering across the
	// old holder tile is legitimate pass-through, which would read as re-stacks.
	for i := 0; i < 360; i++ {
		stepMonstersRT(game, gl)

		x1, y1 := monsterTileCoords(m1, ts)
		x2, y2 := monsterTileCoords(m2, ts)
		if x1 == x2 && y1 == y2 {
			// (dist=0, range=1) is always in reach for melee, so the gate's
			// answer isolates the transit rule itself.
			for _, m := range []*monster.Monster3D{m1, m2} {
				switch {
				case m.AttackTransit:
					gateShutForTransit = true
					if cs.monsterCanAttackParty(m, 0, 1) {
						t.Fatalf("tick %d: stacked transit monster passes the attack gate", i)
					}
				case m.AttackPost:
					if cs.monsterCanAttackParty(m, 0, 1) {
						gateOpenForHolder = true
					}
				}
			}
		} else {
			if separatedAt < 0 {
				separatedAt = i
			}
		}
		if separatedAt >= 0 && x1 == x2 && y1 == y2 {
			restacks++
		}
		if separatedAt >= 0 && (x2 != lastX2 || y2 != lastY2) {
			moves++
		}
		lastX2, lastY2 = x2, y2

		cs.HandleMonsterInteractions()
	}

	if !gateShutForTransit || !gateOpenForHolder {
		t.Fatalf("stacked window never exercised both gate sides (transit=%v holder=%v)", gateShutForTransit, gateOpenForHolder)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatal("the post holder never landed a hit - the fight is not real")
	}
	if separatedAt < 0 {
		t.Fatal("stacked monsters never separated - the transit monster has no exit")
	}
	// The exit must be a quick, visible reposition, not minutes of post churn.
	if separatedAt > 360 {
		t.Fatalf("transit monster took %d ticks to leave the shared tile, want <= 360", separatedAt)
	}
	if restacks > 0 {
		t.Fatalf("transit monster re-entered the holder's tile %d times after separating", restacks)
	}
	t.Logf("separated at tick %d, post-separation tile moves=%d", separatedAt, moves)
}
