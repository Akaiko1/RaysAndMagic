package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/world"
)

// With its Display choice on, an object is scaled down, uniformly and never up,
// until its visible art stays inside its tile; with the choice off nothing is
// measured or changed. The reach of the art is given here, not measured: the
// rule is under test, not the sprites.
func TestTileObjectsFitTheirTile(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	tm := world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := tm.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	previous := world.GlobalTileManager
	world.GlobalTileManager = tm
	t.Cleanup(func() { world.GlobalTileManager = previous })
	g := newTestGame(cfg, newTestWorld(cfg))
	g.sprites = graphics.NewSpriteManager()
	rh := NewRenderingHelper(g)

	// One tile per render type, taken from the catalog.
	byRender := map[string]world.TileType3D{}
	for key := range tm.ListTiles() {
		tileType, ok := tm.GetTileTypeFromKey(key)
		if !ok || tm.GetSprite(tileType) == "" || isFireflySwarmTile(tileType) {
			continue
		}
		if _, taken := byRender[tm.GetRenderType(tileType)]; !taken {
			byRender[tm.GetRenderType(tileType)] = tileType
		}
	}
	const (
		off = iota
		trees
		monuments
	)
	full := visibleReach{0.5, 0.5}
	for _, tc := range []struct {
		name, render string
		choice       int
		size         float64      // authored span before the fit
		reach        visibleReach // art reach from the frame centre
		want         float64
	}{
		// A cross lies on the diagonals: a frame-filling 2-tile tree reaches 1
		// tile and must come down to the corner at sqrt(2)/2.
		{"tree filling its frame", config.TileRenderCrossedStandee, trees, 2, full, math.Sqrt2 / 2},
		{"tree narrower than its frame", config.TileRenderCrossedStandee, trees, 2, visibleReach{0.25, 0.25}, 1},
		{"tree exactly at the corners", config.TileRenderCrossedStandee, trees, math.Sqrt2, full, 1},
		// A crossed prop authors height; its width follows the frame aspect.
		{"wide crossed prop", config.TileRenderCrossedProp, trees, 1.6, visibleReach{perWidth: 0.25, perHeight: 0.5}, math.Sqrt2 / 2 / 0.8},
		{"slim crossed prop", config.TileRenderCrossedProp, trees, 1.6, visibleReach{perWidth: 0.5, perHeight: 0.2}, 1},
		// A flat token turns to face the party: it clears the inscribed circle.
		{"flat token wider than a tile", config.TileRenderStandee, trees, 1.5, full, 0.5 / 0.75},
		{"flat token inside its tile", config.TileRenderStandee, trees, 0.8, full, 1},
		// A monument spins on its tile; it answers to the other choice.
		{"monument wider than a tile", config.TileRenderLandmarkStandee, monuments, 3, full, 0.5 / 1.5},
		{"monument inside its tile", config.TileRenderLandmarkStandee, monuments, 0.9, full, 1},
		// Each choice leaves the other group alone, and off leaves everything.
		{"monument, trees choice", config.TileRenderLandmarkStandee, trees, 3, full, 1},
		{"tree, monuments choice", config.TileRenderCrossedStandee, monuments, 2, full, 1},
		{"tree, choices off", config.TileRenderCrossedStandee, off, 2, full, 1},
		{"flat token, choices off", config.TileRenderStandee, off, 1.5, full, 1},
		{"monument, choices off", config.TileRenderLandmarkStandee, off, 3, full, 1},
		{"wall", config.TileRenderWall, trees, 3, full, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tileType, ok := byRender[tc.render]
			if !ok {
				t.Fatalf("catalog has no %s tile with a sprite", tc.render)
			}
			cfg.PlayerFitTreesToTile, cfg.PlayerFitMonumentsToTile = tc.choice == trees, tc.choice == monuments
			clear(rh.visibleReachCache)
			if tc.choice != off {
				rh.visibleReachCache[tm.GetSprite(tileType)] = tc.reach
			}
			if got := rh.tileFitScale(tileType, tc.size); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("fit scale = %.4f, want %.4f", got, tc.want)
			}
			// Off must not even measure the art: a lookup would request it.
			if tc.choice == off && len(rh.visibleReachCache) != 0 {
				t.Fatal("a disabled fit measured the sprite")
			}
		})
	}

	// The size helper every draw and hit test reads applies the fit: a
	// frame-filling tree of the tree class comes out corner to corner, and at
	// its authored class with the choice off.
	tree := byRender[config.TileRenderCrossedStandee]
	class := tm.GetSizeTiles(tree)
	clear(rh.visibleReachCache)
	rh.visibleReachCache[tm.GetSprite(tree)] = full
	for _, on := range []bool{false, true} {
		cfg.PlayerFitTreesToTile, cfg.PlayerFitMonumentsToTile = on, false
		want := class
		if on {
			want = math.Min(class, math.Sqrt2)
		}
		if got := rh.envHeightMultiplier(tree, 1); math.Abs(got-want) > 1e-9 {
			t.Fatalf("tree width with the choice %v = %.4f tiles, want %.4f", on, got, want)
		}
	}

	// Placed objects: monuments and scenery follow the monuments choice;
	// people never do.
	for _, tc := range []struct {
		name, category string
		monuments      bool
		wantFit        bool
	}{
		{"scenery", "scenery", true, true},
		{"landmark", "landmark", true, true},
		{"person", "npc", true, false},
		{"wall token", "wall_mounted", true, false},
		{"scenery, choice off", "scenery", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			npc := &character.NPC{Name: tc.name, Sprite: "fit_fixture", RenderCategory: tc.category, SizeClass: "structure"}
			clear(rh.visibleReachCache)
			clear(rh.visibleHeightScaleCache)
			// The frame is all art: the class is the frame span either way.
			rh.visibleHeightScaleCache[visibleHeightScaleKey{"fit_fixture", false}] = 1
			rh.visibleHeightScaleCache[visibleHeightScaleKey{"fit_fixture", true}] = 1
			rh.visibleReachCache["fit_fixture"] = full
			cfg.PlayerFitTreesToTile, cfg.PlayerFitMonumentsToTile = false, tc.monuments
			class, _ := config.ResolveSizeClassTiles(cfg.Graphics.SizeClasses, npc.SizeClass)
			want := class
			if tc.wantFit {
				want = 1 // the art reaches half the span: a 2-tile span comes down to 1
			}
			if got := rh.npcSizeTiles(npc); math.Abs(got-want) > 1e-9 {
				t.Fatalf("size = %.4f tiles, want %.4f", got, want)
			}
		})
	}
}
