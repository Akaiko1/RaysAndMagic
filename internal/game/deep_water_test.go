package game

import (
	"testing"

	"ugataima/internal/world"
)

// findDeepWaterTile scans the unified world for any deep water tile.
func findDeepWaterTile(t *testing.T, w *world.World3D) (int, int) {
	t.Helper()
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if w.Tiles[y][x] == world.TileDeepWater {
				return x, y
			}
		}
	}
	t.Skip("no deep water tile on the unified world")
	return 0, 0
}

// TestDeepWaterNoSpamWhileAirborne: Fly and Walk on Water keep the party
// above the surface - stepping over deep water must neither warn nor move
// them. With no protection at all the party is pulled ashore ONCE instead of
// warning on every subsequent step.
func TestDeepWaterNoSpamWhileAirborne(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	ih := g.gameLoop.inputHandler
	ts := cfg.GetTileSize()
	tx, ty := findDeepWaterTile(t, wm.OpenWorld)
	overWater := func() {
		g.camera.X, g.camera.Y = TileCenterFromTile(tx, ty, ts)
	}

	// Flying across the lake: silent, stays put.
	overWater()
	g.flyActive = true
	msgs := len(g.combatLogHistory)
	ih.checkDeepWater()
	if len(g.combatLogHistory) != msgs {
		t.Errorf("flying over deep water logged %q", g.combatLogHistory[len(g.combatLogHistory)-1].Text)
	}
	if int(g.camera.X/ts) != tx || int(g.camera.Y/ts) != ty {
		t.Error("flying over deep water moved the party")
	}

	// Walk on Water: same.
	g.flyActive = false
	g.walkOnWaterActive = true
	msgs = len(g.combatLogHistory)
	ih.checkDeepWater()
	if len(g.combatLogHistory) != msgs {
		t.Errorf("walking on deep water logged %q", g.combatLogHistory[len(g.combatLogHistory)-1].Text)
	}

	// No protection at all (buff lapsed mid-lake): rescued ashore, one message.
	g.walkOnWaterActive = false
	msgs = len(g.combatLogHistory)
	ih.checkDeepWater()
	if len(g.combatLogHistory) != msgs+1 {
		t.Fatalf("unprotected deep water logged %d messages, want exactly 1", len(g.combatLogHistory)-msgs)
	}
	ntx, nty := int(g.camera.X/ts), int(g.camera.Y/ts)
	if wm.OpenWorld.Tiles[nty][ntx] == world.TileDeepWater {
		t.Fatal("party left stranded on deep water")
	}
}

// TestWalkOnWaterExpiryRescue: the walk_on_water buff lapsing while the party
// stands on water pulls them ashore; on dry land it does nothing.
func TestWalkOnWaterExpiryRescue(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := cfg.GetTileSize()
	tx, ty := findDeepWaterTile(t, wm.OpenWorld)
	g.camera.X, g.camera.Y = TileCenterFromTile(tx, ty, ts)

	g.settleAfterWalkOnWater()
	ntx, nty := int(g.camera.X/ts), int(g.camera.Y/ts)
	if wm.OpenWorld.Tiles[nty][ntx] == world.TileDeepWater {
		t.Fatal("expiry left the party on deep water")
	}

	// Dry land: a lapse must not teleport anyone.
	dx, dy := g.camera.X, g.camera.Y
	g.settleAfterWalkOnWater()
	if g.camera.X != dx || g.camera.Y != dy {
		t.Error("expiry on dry land moved the party")
	}

	// Still protected by Fly: stays on the water.
	g.camera.X, g.camera.Y = TileCenterFromTile(tx, ty, ts)
	g.flyActive = true
	g.settleAfterWalkOnWater()
	if int(g.camera.X/ts) != tx || int(g.camera.Y/ts) != ty {
		t.Error("expiry moved the party despite active Fly")
	}
}
