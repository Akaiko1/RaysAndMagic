package game

import (
	"reflect"
	"testing"

	"ugataima/internal/graphics"
	"ugataima/internal/world"
)

func TestEnvironmentVariantsSurviveRenderCacheRebuilds(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	setTestWorldManager(t, nil)
	tm := world.NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	world.GlobalTileManager = tm
	tree, _ := tm.GetTileTypeFromKey("tree")
	w := newTestWorldSized(cfg, 12, 12)
	for y := range w.Tiles {
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = tree
		}
	}
	w.Tiles[0][0] = world.TileEmpty
	g := newTestGame(cfg, w)
	g.sprites = graphics.NewSpriteManager()
	g.camera.X, g.camera.Y = cfg.GetTileSize()/2, cfg.GetTileSize()/2
	g.camera.Angle, g.camera.ViewDist = 0, cfg.GetTileSize()*20
	r := &Renderer{game: g}
	var first []TransparentSpriteData
	for _, crossed := range []bool{true, false, true} {
		cfg.Graphics.TreesAsBillboards = crossed
		r.buildTransparentSpriteCache()
		if first == nil {
			first = append(first, r.treeTilesCache...)
		} else if !reflect.DeepEqual(first, r.treeTilesCache) {
			t.Fatal("render-mode/cache rebuild rerolled the scenery")
		}
		plan := r.collectMapRenderPrewarmPlan("")
		seen := make(map[string]bool)
		for _, entry := range r.treeTilesCache {
			want := w.EnvironmentSprite(entry.tileType, entry.tileX, entry.tileY)
			if entry.spriteName != want || !containsString(plan.tileSprites, want) || !containsString(plan.treeSprites, want) {
				t.Fatalf("cell %d,%d: cache=%q want=%q, preload=%v", entry.tileX, entry.tileY, entry.spriteName, want, plan.tileSprites)
			}
			seen[entry.spriteName] = true
		}
		if len(seen) != 3 {
			t.Fatalf("expected original and both authored variants: %v", seen)
		}
		if allocs := testing.AllocsPerRun(100, func() {
			for _, entry := range r.treeTilesCache {
				r.selectEnvironmentSpriteName(entry.tileType, entry.tileX, entry.tileY)
			}
		}); allocs != 0 {
			t.Fatalf("cached tree selection allocates %g times per frame", allocs)
		}
		if !crossed {
			hits := r.performMultiHitRaycast(0, nil).Hits
			if len(hits) == 0 || hits[0].TileX != 1 || hits[0].TileY != 0 {
				t.Fatalf("flat tree ray lost its cell identity: %+v", hits)
			}
			if got := r.selectEnvironmentSpriteName(hits[0].TileType, hits[0].TileX, hits[0].TileY); got != w.EnvironmentSprite(tree, 1, 0) {
				t.Fatalf("flat and crossed selection disagree: %q", got)
			}
		}
	}
}

func TestSaveLoadRerollsEnvironmentVariants(t *testing.T) {
	g, wm, _ := travelFixture(t)
	tree, ok := world.GlobalTileManager.GetTileTypeFromKey("tree")
	if !ok {
		t.Fatal("missing tree tile")
	}
	layout := func() []string {
		var names []string
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				names = append(names, g.world.EnvironmentSprite(tree, x, y))
			}
		}
		return names
	}
	save := g.buildSave(wm)
	before := layout()
	for load := 0; load < 2; load++ {
		if err := g.applySave(wm, &save); err != nil {
			t.Fatal(err)
		}
		after := layout()
		if reflect.DeepEqual(before, after) {
			t.Fatal("loading a save retained the previous texture layout")
		}
		if !reflect.DeepEqual(after, layout()) {
			t.Fatal("the loaded texture layout did not settle")
		}
		before = after
	}
}

func TestLegacyEnvironmentSelectionUsesTileCache(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	setTestWorldManager(t, nil)
	tm := world.NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	world.GlobalTileManager = tm
	tree, _ := tm.GetTileTypeFromKey("tree")
	tm.GetTileData(tree).SpriteVariants = nil
	w := newTestWorldSized(cfg, 4, 4)
	w.Tiles[1][1] = tree
	g := newTestGame(cfg, w)
	g.sprites = graphics.NewSpriteManager()
	r := &Renderer{game: g}
	r.buildTransparentSpriteCache()
	want := r.resolveEnvironmentSpriteName(tree, 1, 1)
	if want == "" {
		t.Fatal("fixture has no legacy sprite")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if got := r.selectEnvironmentSpriteName(tree, 1, 1); got != want {
			t.Fatalf("cached sprite %q differs from legacy selection %q", got, want)
		}
	}); allocs != 0 {
		t.Fatalf("legacy sprite selection allocates %g times per hit", allocs)
	}
	// A rebuilt map must discard names chosen from the old tile definition.
	tm.GetTileData(tree).Sprite = "review_replacement_sprite"
	r.buildTransparentSpriteCache()
	if got := r.selectEnvironmentSpriteName(tree, 1, 1); got != "review_replacement_sprite" {
		t.Fatalf("cache rebuild retained old sprite %q", got)
	}
}
