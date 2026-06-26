package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// Melee arc mechanic: reach is counted in TILE steps (a diagonal neighbour is one
// step), so range 1 covers all 8 adjacent tiles; the arc type gates the angular
// cone around the player's facing.
func TestMelee_ArcAndDiagonalReach(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	axe, err := items.TryCreateWeaponFromYAML("steel_axe") // range 1
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}

	// swing places a fresh goblin at each offset, runs one swing of arcType, and
	// reports which goblins took damage.
	swing := func(weapon items.Item, arcType int, offs [][2]int) []bool {
		mobs := make([]*monster.Monster3D, len(offs))
		for i, o := range offs {
			mobs[i] = monster.NewMonster3DFromConfig(
				float64(ptx+o[0])*ts+ts/2, float64(pty+o[1])*ts+ts/2, "goblin", game.config)
		}
		game.world.Monsters = mobs
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		cs.performMeleeHitDetection(weapon, 40, &config.MeleeAttackConfig{ArcType: arcType}, false)
		hurt := make([]bool, len(offs))
		for i, m := range mobs {
			hurt[i] = m.HitPoints < m.MaxHitPoints
		}
		return hurt
	}

	front, fr, fl, side := [2]int{1, 0}, [2]int{1, 1}, [2]int{1, -1}, [2]int{0, 1}

	// Arc 3 reaches the front AND both diagonal neighbours at range 1 — the core
	// fix (range 1 no longer falls 2px short of the diagonals).
	if h := swing(axe, 3, [][2]int{front, fr, fl}); !h[0] || !h[1] || !h[2] {
		t.Fatalf("arc 3 must hit front + both diagonals at range 1, got %v", h)
	}
	// Arc 1 strikes ONLY straight ahead.
	if h := swing(axe, 1, [][2]int{front, fr, fl}); !h[0] || h[1] || h[2] {
		t.Fatalf("arc 1 must hit only the front tile, got %v", h)
	}
	// Arc 4 additionally reaches the perpendicular sides; arc 3 does not.
	if h := swing(axe, 4, [][2]int{front, side}); !h[0] || !h[1] {
		t.Fatalf("arc 4 must hit the front and the side, got %v", h)
	}
	if h := swing(axe, 3, [][2]int{front, side}); !h[0] || h[1] {
		t.Fatalf("arc 3 must NOT hit the perpendicular side, got %v", h)
	}
}

// Regression: a real-time melee monster diagonally adjacent to the party must
// COMMIT to attacking from its diagonal tile, not loop its walk animation in
// place. Before the fix the AI gated StateAttacking on pixel distance (~1.41
// tiles > the 1-tile range), so a diagonal attacker either jittered or relocated
// to a cardinal tile. Here it must stay on its diagonal tile AND land a hit.
func TestRealTime_MonsterCommitsToDiagonalMelee(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0.7 // party at an angle (the reported situation)

	orc := spawnMonsterAtTile(game, "orc", ptx+1, pty+1, ts) // diagonal neighbour
	orc.State = monster.StatePursuing
	orc.AttackCDFrames = 0

	hp0 := partyHPSum(game)
	for i := 0; i < 30; i++ {
		orc.Update(game.collisionSystem, game.camera.X, game.camera.Y)
		cs.HandleMonsterInteractions()
	}

	if tx, ty := monsterTileCoords(orc, ts); tx != ptx+1 || ty != pty+1 {
		t.Fatalf("orc should attack from its diagonal tile, but relocated to (%d,%d)", tx, ty)
	}
	if partyHPSum(game) >= hp0 {
		t.Fatalf("diagonal-adjacent orc never landed a hit (stuck walking)")
	}
}

// A range-2 weapon (spear) with arc 1 pierces the line two tiles deep, but spares
// off-axis tiles.
func TestMelee_RangeTwoPiercesLine(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	spear, err := items.TryCreateWeaponFromYAML("iron_spear") // range 2
	if err != nil {
		t.Fatalf("iron_spear: %v", err)
	}
	g := func(dx, dy int) *monster.Monster3D {
		return monster.NewMonster3DFromConfig(
			float64(ptx+dx)*ts+ts/2, float64(pty+dy)*ts+ts/2, "goblin", game.config)
	}
	near, far, off := g(1, 0), g(2, 0), g(2, 1) // two straight ahead + one off-axis
	game.world.Monsters = []*monster.Monster3D{near, far, off}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	cs.performMeleeHitDetection(spear, 40, &config.MeleeAttackConfig{ArcType: 1}, false)

	if near.HitPoints >= near.MaxHitPoints || far.HitPoints >= far.MaxHitPoints {
		t.Fatalf("range-2 arc-1 spear must pierce both tiles straight ahead")
	}
	if off.HitPoints < off.MaxHitPoints {
		t.Fatalf("arc 1 must not hit the off-axis tile")
	}
}
