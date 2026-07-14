package game

import (
	"testing"

	"ugataima/internal/collision"
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

	// Arc 3 reaches the front AND both diagonal neighbours at range 1 - the core
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

// The TB front-diagonal pull must be invariant to the cosmetic Draw-time screen
// shake (melee/spell hits set it). Otherwise the per-frame +/- camera jitter flips
// the pulled slot / its LOS near a wall and the struck monster blinks - while a
// trap (no screen shake) does not. The fix computes the pull from the LOGICAL
// camera (camera minus screenShakeOffset), so the result can't move with shake.
func TestPulledSlot_StableUnderScreenShake(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	mon := monster.NewMonster3DFromConfig(float64(ptx+1)*ts+ts/2, float64(pty-1)*ts+ts/2, "goblin", game.config)
	mon.IsEngagingPlayer, mon.WasAttacked = true, true
	game.world.Monsters = []*monster.Monster3D{mon}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	side0, x0, y0, pulled0, ok0 := cs.pulledFrontSlot(mon)
	if !ok0 || !pulled0 {
		t.Fatalf("baseline: expected a pulled front-diagonal slot, got ok=%v pulled=%v", ok0, pulled0)
	}

	// Simulate one Draw frame mid-shake: camera nudged AND the offset recorded.
	const ox, oy = 5.0, 3.0
	game.camera.X += ox
	game.camera.Y += oy
	game.screenShakeOffsetX, game.screenShakeOffsetY = ox, oy

	side1, x1, y1, pulled1, ok1 := cs.pulledFrontSlot(mon)
	if side1 != side0 || pulled1 != pulled0 || ok1 != ok0 || x1 != x0 || y1 != y0 {
		t.Fatalf("pull moved under screen shake (should be shake-invariant):\n  shaken: side=%d pulled=%v ok=%v x=%.2f y=%.2f\n  base:   side=%d pulled=%v ok=%v x=%.2f y=%.2f",
			side1, pulled1, ok1, x1, y1, side0, pulled0, ok0, x0, y0)
	}
}

// The adjacency gate of the pull (monsterMeleeAdjacentToParty) must ALSO be
// shake-invariant: it reads the player tile from the camera, so a Draw-time
// screen shake that nudges the camera across a tile boundary would flip the gate
// (and thus the pull) unless it uses the logical camera. Park the camera 2px shy
// of a boundary so a +5 shake crosses it.
func TestMeleeAdjacency_StableUnderScreenShake(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	game.camera.X = 11*ts - 2    // int(/ts) = 10; a +5 shake -> tile 11
	game.camera.Y = 10*ts + ts/2 // tile 10
	game.camera.Angle = 0

	// Diagonally adjacent to the LOGICAL player tile (10,10): tile (9,9) -> dx=dy=1.
	// If the gate used the shaken camera (tile 11), dx would become 2 -> not adjacent.
	mon := monster.NewMonster3DFromConfig(9*ts+ts/2, 9*ts+ts/2, "goblin", game.config)
	game.world.Monsters = []*monster.Monster3D{mon}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	if !cs.monsterMeleeAdjacentToParty(mon) {
		t.Fatalf("baseline: monster should read as melee-adjacent")
	}
	game.camera.X += 5
	game.screenShakeOffsetX = 5
	if !cs.monsterMeleeAdjacentToParty(mon) {
		t.Fatalf("adjacency gate flipped under screen shake (should use the logical camera)")
	}
}

func TestTurnBased_MeleePulledFrontDiagonalArcAssist(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	g := func(dx, dy int) *monster.Monster3D {
		m := monster.NewMonster3DFromConfig(float64(ptx+dx)*ts+ts/2, float64(pty+dy)*ts+ts/2, "goblin", game.config)
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		return m
	}
	setMobs := func(mobs ...*monster.Monster3D) {
		game.world.Monsters = mobs
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	}
	hurtCount := func(mobs ...*monster.Monster3D) int {
		n := 0
		for _, m := range mobs {
			if m.HitPoints < m.MaxHitPoints {
				n++
			}
		}
		return n
	}

	sideOnly := g(1, -1)
	setMobs(sideOnly)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if sideOnly.HitPoints >= sideOnly.MaxHitPoints {
		t.Fatalf("arc 1 must hit a lone pulled front-diagonal monster")
	}

	left, right := g(1, -1), g(1, 1)
	setMobs(left, right)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 2}, false)
	if got := hurtCount(left, right); got != 1 {
		t.Fatalf("arc 2 without a front target must hit exactly one pulled side, got %d", got)
	}

	front, side := g(1, 0), g(1, -1)
	setMobs(front, side)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 2}, false)
	if got := hurtCount(front, side); got != 2 {
		t.Fatalf("arc 2 with front + pulled side must hit both, got %d", got)
	}

	front, side = g(1, 0), g(1, -1)
	setMobs(front, side)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if got := hurtCount(front, side); got != 1 || front.HitPoints >= front.MaxHitPoints {
		t.Fatalf("arc 1 with front + pulled side must prefer the front target only, hurt=%d frontHP=%d/%d", got, front.HitPoints, front.MaxHitPoints)
	}
}

func TestRealTime_PlayerMeleeHitsMonsterOnPartyTile(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	mon := spawnMonsterAtTile(game, "goblin", ptx, pty, ts)

	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if mon.HitPoints >= mon.MaxHitPoints {
		t.Fatalf("RT melee should hit a monster that overlapped into the party tile")
	}
}

func TestRealTime_PlayerMeleeHitsContactMonstersAtTileEdges(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}

	tests := []struct {
		name string
		x    float64
		y    float64
	}{
		{
			name: "same tile far corner",
			x:    float64(ptx+1)*ts - 1,
			y:    float64(pty+1)*ts - 1,
		},
		{
			name: "front neighbour far edge",
			x:    float64(ptx+2)*ts - 1,
			y:    float64(pty)*ts + ts/2,
		},
		{
			name: "front diagonal far corner",
			x:    float64(ptx+2)*ts - 1,
			y:    float64(pty-1)*ts + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mon := monster.NewMonster3DFromConfig(tt.x, tt.y, "goblin", game.config)
			game.world.Monsters = []*monster.Monster3D{mon}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)

			cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
			if mon.HitPoints >= mon.MaxHitPoints {
				t.Fatalf("RT arc 1 should hit contact monster at x=%.1f y=%.1f tile=(%d,%d)",
					tt.x, tt.y, int(tt.x/ts), int(tt.y/ts))
			}
		})
	}
}

func TestRealTime_PlayerMeleeArcAssistFrontDiagonalContact(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.turnBasedMode = false
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	axe, err := items.TryCreateWeaponFromYAML("steel_axe")
	if err != nil {
		t.Fatalf("steel_axe: %v", err)
	}
	g := func(dx, dy int) *monster.Monster3D {
		return spawnMonsterAtTile(game, "goblin", ptx+dx, pty+dy, ts)
	}
	setMobs := func(mobs ...*monster.Monster3D) {
		game.world.Monsters = mobs
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	}
	hurtCount := func(mobs ...*monster.Monster3D) int {
		n := 0
		for _, m := range mobs {
			if m.HitPoints < m.MaxHitPoints {
				n++
			}
		}
		return n
	}

	sideOnly := g(1, -1)
	setMobs(sideOnly)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 1}, false)
	if sideOnly.HitPoints >= sideOnly.MaxHitPoints {
		t.Fatalf("RT arc 1 should hit a lone front-diagonal contact monster")
	}

	left, right := g(1, -1), g(1, 1)
	setMobs(left, right)
	cs.performMeleeHitDetection(axe, 40, &config.MeleeAttackConfig{ArcType: 2}, false)
	if got := hurtCount(left, right); got != 1 {
		t.Fatalf("RT arc 2 without a front target should hit exactly one front-diagonal contact monster, got %d", got)
	}
}

func TestTurnBased_PlayerProjectileAssistsPulledFrontDiagonalTarget(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat
	const ptx, pty = 10, 10
	placePlayerAtTile(game, ptx, pty, ts)
	game.camera.Angle = 0 // face +X (East)

	target := spawnMonsterAtTile(game, "goblin", ptx+1, pty-1, ts)
	beforeHP := target.HitPoints
	projectile := MagicProjectile{
		ID:        "test_firebolt",
		X:         game.camera.X,
		Y:         game.camera.Y,
		VelX:      8,
		VelY:      0,
		Damage:    20,
		LifeTime:  10,
		Active:    true,
		SpellType: "firebolt",
		Size:      16,
		Owner:     ProjectileOwnerPlayer,
	}
	game.magicProjectiles = append(game.magicProjectiles, projectile)
	game.collisionSystem.RegisterEntity(collision.NewEntity(projectile.ID, projectile.X, projectile.Y, 16, 16, collision.CollisionTypeProjectile, false))

	// Fresh from the muzzle (still at the camera): the assist must NOT fire yet -
	// the bolt has to visibly travel toward the pulled sprite first.
	cs.CheckProjectileMonsterCollisions()
	if target.HitPoints < beforeHP {
		t.Fatalf("projectile must not assist-hit at spawn; the bolt should still be in flight")
	}
	if !game.magicProjectiles[0].Active {
		t.Fatalf("projectile was consumed before it travelled")
	}

	// Advance the bolt out to the pulled slot's drawn position; now it connects.
	game.magicProjectiles[0].X = game.camera.X + 1.0*ts
	game.magicProjectiles[0].Y = game.camera.Y
	cs.CheckProjectileMonsterCollisions()
	if target.HitPoints >= beforeHP {
		t.Fatalf("player projectile should assist-hit once it reaches the pulled front-diagonal slot")
	}
	if game.magicProjectiles[0].Active {
		t.Fatalf("projectile should be consumed after the assisted hit")
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
