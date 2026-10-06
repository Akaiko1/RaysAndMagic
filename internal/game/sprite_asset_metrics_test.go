package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/world"
)

func TestVisibleHeightFrameScaleUsesRealAssetsAndCache(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")

	sprites := graphics.NewSpriteManager()
	ApplySpriteColorKey(sprites, cfg)
	bounds, frameWidth, frameHeight, known := sprites.SpriteVisibleFrameBounds("campfire")
	if !known {
		t.Fatal("campfire alpha bounds were not resolved from the real asset")
	}
	if frameWidth <= 0 || frameHeight <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("campfire frame = %dx%d, visible height = %d; want positive measured geometry", frameWidth, frameHeight, bounds.Dy())
	}

	g := &MMGame{config: cfg, sprites: sprites}
	rh := NewRenderingHelper(g)
	got := rh.visibleHeightFrameScale("campfire", false)
	want := float64(frameHeight) / float64(bounds.Dy())
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("campfire visible-height scale = %.12f, want %.12f", got, want)
	}
	if len(rh.visibleHeightScaleCache) != 1 {
		t.Fatalf("visible-height cache entries = %d, want 1", len(rh.visibleHeightScaleCache))
	}
	if second := rh.visibleHeightFrameScale("campfire", false); second != got || len(rh.visibleHeightScaleCache) != 1 {
		t.Fatalf("cached scale = %v with %d entries, want %v with 1", second, len(rh.visibleHeightScaleCache), got)
	}
	door := &character.NPC{Sprite: "campfire", RenderCategory: "door", SizeClass: "full_tile"}
	classTiles, ok := config.ResolveSizeClassTiles(cfg.Graphics.SizeClasses, door.SizeClass)
	if !ok {
		t.Fatal("full_tile size class is missing")
	}
	if doorTiles := rh.npcSizeTiles(door); math.Abs(doorTiles-classTiles*got) > 1e-9 {
		t.Fatalf("door visible-height size = %.12f, want %.12f", doorTiles, classTiles*got)
	}

	variants := sprites.GetSpriteVariants("grass")
	if len(variants) < 2 {
		t.Fatalf("grass variants = %v, want a real multi-asset family", variants)
	}
	fractionSum := 0.0
	for _, name := range variants {
		variantBounds, _, variantHeight, ok := sprites.SpriteVisibleFrameBounds(name)
		if !ok {
			t.Fatalf("variant %q alpha bounds were not resolved", name)
		}
		fractionSum += float64(variantBounds.Dy()) / float64(variantHeight)
	}
	wantVariantScale := float64(len(variants)) / fractionSum
	if gotVariantScale := rh.visibleHeightFrameScale("grass", false); math.Abs(gotVariantScale-wantVariantScale) > 1e-9 {
		t.Fatalf("grass family scale = %.12f, want %.12f", gotVariantScale, wantVariantScale)
	}
}

// A prop-class cross must size EXACTLY like the flat standee it replaced -
// authored class means VISIBLE HEIGHT - while a tree-class cross authors frame
// WIDTH and takes no alpha normalization. The two halves live apart
// (envHeightMultiplier normalizes, the draw picks the axis), so only their
// composition over real assets catches a contract flip that synthetic
// fixtures without alpha padding would miss.
func TestCrossedTileSizeContractComposesWithRealAssets(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")

	sprites := graphics.NewSpriteManager()
	ApplySpriteColorKey(sprites, cfg)

	previousTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previousTiles })
	world.GlobalTileManager = world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	g := &MMGame{config: cfg, sprites: sprites}
	rh := NewRenderingHelper(g)

	check := func(key string, wantVisibleHeight bool) {
		t.Helper()
		data := world.GlobalTileManager.GetTileDataByKey(key)
		if data == nil {
			t.Fatalf("tile %q is missing", key)
		}
		classTiles, ok := config.ResolveSizeClassTiles(cfg.Graphics.SizeClasses, data.SizeClass)
		if !ok {
			t.Fatalf("tile %q has unresolvable size_class %q", key, data.SizeClass)
		}
		tileType, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
		if !ok {
			t.Fatalf("tile %q has no tile type", key)
		}
		bounds, _, frameHeight, known := sprites.SpriteVisibleFrameBounds(data.Sprite)
		if !known {
			t.Fatalf("sprite %q bounds were not resolved from the real asset", data.Sprite)
		}
		mult := rh.envHeightMultiplier(tileType, 1.0)
		if !wantVisibleHeight {
			// Tree class: the authored value IS the frame width, unnormalized.
			if math.Abs(mult-classTiles) > 1e-9 {
				t.Fatalf("%s frame width = %.6f tiles, want the authored class %.6f", key, mult, classTiles)
			}
			return
		}
		if frameHeight == bounds.Dy() {
			t.Fatalf("sprite %q has no alpha padding; pick a padded one or this proves nothing", data.Sprite)
		}
		if visible := mult * float64(bounds.Dy()) / float64(frameHeight); math.Abs(visible-classTiles) > 1e-9 {
			t.Fatalf("%s visible height = %.6f tiles, want the authored class %.6f", key, visible, classTiles)
		}
	}

	check("steam_boiler", true) // prop-class cross: class = visible height
	check("gear_pile", true)    // flat standee control: same contract
	check("tree", false)        // tree class: class = frame width
}
