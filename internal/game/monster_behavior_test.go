package game

import (
	"testing"

	"ugataima/internal/monster"
)

// These tests cover the turn-based / real-time monster movement and attack
// rules: tile-centering, cardinal-only melee that never enters the player's
// tile, ranged firing only when row/column-aligned, the 1-tile real-time melee
// reach, and the puma pounce landing on an adjacent tile (not on the player).

func absI(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tbBehaviorGame builds a turn-based game on an empty w×h world with the combat
// system wired, and returns it plus a GameLoop and the tile size.
func tbBehaviorGame(t *testing.T, w, h int) (*MMGame, *GameLoop, float64) {
	t.Helper()
	cfg := loadTestConfig(t)
	worldTest := newTestWorldSized(cfg, w, h)
	game := newTestGame(cfg, worldTest)
	game.turnBasedMode = true
	game.combat = NewCombatSystem(game)
	gl := &GameLoop{game: game, combat: game.combat}
	return game, gl, float64(cfg.GetTileSize())
}

func placePlayerAtTile(game *MMGame, tx, ty int, ts float64) {
	game.camera.X = float64(tx)*ts + ts/2
	game.camera.Y = float64(ty)*ts + ts/2
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
}

func spawnMonsterAtTile(game *MMGame, key string, tx, ty int, ts float64) *monster.Monster3D {
	m := monster.NewMonster3DFromConfig(float64(tx)*ts+ts/2, float64(ty)*ts+ts/2, key, game.config)
	m.IsEngagingPlayer = true // ensure it participates regardless of vision
	m.WasAttacked = true      // bypass any passive-until-attacked gating
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	return m
}

func runOneMonsterTurn(game *MMGame, gl *GameLoop) {
	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased()
}

func partyHPSum(game *MMGame) int {
	sum := 0
	for _, c := range game.party.Members {
		sum += c.HitPoints
	}
	return sum
}

func monsterTileCoords(m *monster.Monster3D, ts float64) (int, int) {
	return int(m.X / ts), int(m.Y / ts)
}

// Melee monsters approach one tile per turn, only ever strike from a
// cardinally-adjacent tile (Manhattan distance 1), and never step onto the
// player's own tile.
func TestTurnBased_MeleeOnlyHitsFromCardinalAdjacent(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	m := spawnMonsterAtTile(game, "goblin", 13, 10, ts) // 3 tiles east, same row

	everHit := false
	for turn := 0; turn < 8; turn++ {
		hpBefore := partyHPSum(game)
		runOneMonsterTurn(game, gl)
		tx, ty := monsterTileCoords(m, ts)

		if tx == ptx && ty == pty {
			t.Fatalf("turn %d: monster stepped onto the player's tile", turn)
		}
		if partyHPSum(game) < hpBefore {
			everHit = true
			if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
				t.Fatalf("turn %d: melee hit from a non-cardinal-adjacent tile (Manhattan %d)", turn, man)
			}
		}
	}
	if !everHit {
		t.Fatalf("melee monster never closed in to strike the party")
	}
}

// A melee monster diagonally adjacent to the player must NOT attack; it should
// reposition onto a cardinally-adjacent tile instead.
func TestTurnBased_MeleeDoesNotAttackDiagonally(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 11, ts) // diagonal neighbour

	hp0 := partyHPSum(game)
	runOneMonsterTurn(game, gl)

	if partyHPSum(game) < hp0 {
		t.Fatalf("melee monster attacked from a diagonal tile")
	}
	tx, ty := monsterTileCoords(m, ts)
	if man := absI(tx-10) + absI(ty-10); man != 1 {
		t.Fatalf("expected diagonal monster to reposition to a cardinal-adjacent tile, Manhattan=%d", man)
	}
}

// Each turn a participating monster snaps to the center of its tile (fixes
// off-center spawns / drift).
func TestTurnBased_CentersOnTileEachTurn(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	// Spawn off-center inside the cardinally-adjacent tile (11,10): it will
	// center, then attack (no move), so the final position is the tile center.
	m := monster.NewMonster3DFromConfig(11*ts+7, 10*ts+13, "goblin", game.config)
	m.IsEngagingPlayer = true
	m.WasAttacked = true
	game.world.Monsters = []*monster.Monster3D{m}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	runOneMonsterTurn(game, gl)

	wantX, wantY := 11*ts+ts/2, 10*ts+ts/2
	if m.X != wantX || m.Y != wantY {
		t.Fatalf("expected monster centered at (%.0f,%.0f), got (%.0f,%.0f)", wantX, wantY, m.X, m.Y)
	}
}

// Ranged monsters fire only when on the player's row or column; from a diagonal
// tile they reposition instead of shooting.
func TestTurnBased_RangedAttacksOnlyWhenAligned(t *testing.T) {
	// Diagonal → must NOT shoot.
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 10, 10, ts)
	diag := spawnMonsterAtTile(game, "elf_archer", 12, 12, ts)
	if len(game.arrows) != 0 {
		t.Fatalf("unexpected pre-existing arrows")
	}
	runOneMonsterTurn(game, gl)
	if len(game.arrows) != 0 {
		t.Fatalf("ranged monster fired from a diagonal (unaligned) tile")
	}
	dtx, dty := monsterTileCoords(diag, ts)
	if dtx == 12 && dty == 12 {
		t.Fatalf("diagonal ranged monster should reposition toward alignment, but stayed put")
	}

	// Aligned and in range → must shoot.
	game2, gl2, ts2 := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game2, 10, 10, ts2)
	spawnMonsterAtTile(game2, "elf_archer", 13, 10, ts2) // same row, 3 tiles (range 5)
	runOneMonsterTurn(game2, gl2)
	if len(game2.arrows) == 0 {
		t.Fatalf("aligned ranged monster in range should fire")
	}
}

// A pouncing monster (puma) leaps onto a cardinally-adjacent tile — never onto
// the player's tile — and strikes. Turn-based path.
func TestTurnBased_PounceLandsAdjacentAndStrikes(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 40, 40)
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	puma := spawnMonsterAtTile(game, "puma", 13, 10, ts) // 3 tiles east, within pounce range 4

	hp0 := partyHPSum(game)
	runOneMonsterTurn(game, gl)

	tx, ty := monsterTileCoords(puma, ts)
	if tx == ptx && ty == pty {
		t.Fatalf("puma pounced onto the player's tile")
	}
	if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
		t.Fatalf("puma should land on a cardinally-adjacent tile, Manhattan=%d", man)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("puma pounce should strike the party")
	}
}

// Real-time pounce lands on a cardinally-adjacent tile (not the player's) and
// deals damage.
func TestRealTime_PounceLandsAdjacentNotPlayerTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	puma := spawnMonsterAtTile(game, "puma", 13, 10, ts)

	hp0 := partyHPSum(game)
	game.combat.HandleMonsterInteractions()

	tx, ty := monsterTileCoords(puma, ts)
	if tx == ptx && ty == pty {
		t.Fatalf("real-time puma pounced onto the player's tile")
	}
	if man := absI(tx-ptx) + absI(ty-pty); man != 1 {
		t.Fatalf("real-time puma should land cardinally adjacent, Manhattan=%d", man)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("real-time puma pounce should strike the party")
	}
}

// Real-time melee reaches exactly one tile (inclusive): a monster sitting on an
// adjacent tile center (64px away == attack radius) still lands its hit.
func TestRealTime_MeleeHitsAtExactlyOneTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	placePlayerAtTile(game, 10, 10, ts)
	m := spawnMonsterAtTile(game, "goblin", 11, 10, ts) // exactly one tile away
	m.State = monster.StateAttacking
	m.StateTimer = 1 // attack fires on the first frame of the attacking state

	hp0 := partyHPSum(game)
	game.combat.HandleMonsterInteractions()
	if partyHPSum(game) >= hp0 {
		t.Fatalf("real-time melee should hit at exactly one tile (inclusive reach)")
	}
}
