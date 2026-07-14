package game

import (
	"math"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// A melee mob whose straight line to the party is blocked by a tree must route
// around it and reach attack range - not livelock shaking at the blocked
// diagonal.
func TestWolfRoutesAroundTreeToParty(t *testing.T) {
	cfg := loadTestConfig(t)

	prevTM := world.GlobalTileManager
	defer func() { world.GlobalTileManager = prevTM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 16, 16
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	// Party tile (8,8); trees hug it on the west and south - same shape as the
	// live save (party tile (31,31), trees at (30,31) and (31,32)).
	w.Tiles[8][7] = world.TileTree
	w.Tiles[9][8] = world.TileTree

	g := newTestGame(cfg, w)
	tile := float64(cfg.World.TileSize)
	// Party off-center in its tile (save offsets +25,+35): this is what blocks
	// LoS from the NW/SW diagonal tile centers through the tree corner.
	camX, camY := 8*tile+25, 8*tile+35
	g.camera.X, g.camera.Y = camX, camY
	g.collisionSystem.UpdateEntity("player", camX, camY)

	spawnWolf := func(tileX, tileY int) *monster.Monster3D {
		m := monster.NewMonster3DFromConfig(float64(tileX)*tile+tile/2, float64(tileY)*tile+tile/2, "wolf", cfg)
		// Aggroed on spawn; WasAttacked keeps the engagement sticky so distance
		// hysteresis cannot drop it mid-approach.
		m.IsEngagingPlayer = true
		m.WasAttacked = true
		m.State = monster.StateAlert
		w.Monsters = append(w.Monsters, m)
		return m
	}
	wolves := []*monster.Monster3D{spawnWolf(5, 7), spawnWolf(5, 9)}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)

	gl := &GameLoop{game: g}
	tps := cfg.GetTPS()
	attacked := make([]bool, len(wolves))
	walked := make([]float64, len(wolves))
	allAttacked := func() bool {
		for _, a := range attacked {
			if !a {
				return false
			}
		}
		return true
	}

	for tick := 0; tick < 30*tps && !allAttacked(); tick++ {
		g.frameCount++
		for i, m := range wolves {
			px, py := m.X, m.Y
			m.Update(g.collisionSystem, camX, camY)
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			g.refreshMonsterCollisionSolidity(m)
			walked[i] += math.Hypot(m.X-px, m.Y-py)
			if m.State == monster.StateAttacking {
				attacked[i] = true
			}
		}
		gl.separateOverlappingMonsters()
	}

	for i, m := range wolves {
		if !attacked[i] {
			t.Errorf("wolf %d never reached attack range in 30s: at (%.0f,%.0f) tile(%d,%d) state=%v dist=%.1f tiles, walked %.0fpx (walked>>net = shaking in place), path=%v",
				i, m.X, m.Y, int(m.X/tile), int(m.Y/tile), m.State,
				math.Hypot(m.X-camX, m.Y-camY)/tile, walked[i], m.PathTiles)
		}
	}
}
