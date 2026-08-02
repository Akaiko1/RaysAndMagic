package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
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

// TestSpriteSizesGolden pins the projected full-frame span of every billboard
// entity (monster, NPC, environment tile) at fixed distances. It guards class
// assignments and the shared perspective formula without loading sprite files.
// Visible-alpha normalization and width-to-height aspect preservation are
// covered separately by focused unit and content tests.
//
// The golden file is generated from the current code once and committed; it is
// NOT regenerated as part of the refactor. Regenerate deliberately with
// RAM_UPDATE_SPRITE_GOLDEN=1 only when a real size change is intended.
func TestSpriteSizesGolden(t *testing.T) {
	got := computeGoldenSpriteSizes(t)
	path := filepath.Join("testdata", "sprite_sizes_golden.json")

	if os.Getenv("RAM_UPDATE_SPRITE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d golden sprite sizes to %s", len(got), path)
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (regenerate with RAM_UPDATE_SPRITE_GOLDEN=1): %v", err)
	}
	want := map[string]int{}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		gv, ok := got[k]
		if !ok {
			t.Errorf("%s: missing from current output (golden %d)", k, want[k])
			continue
		}
		if gv != want[k] {
			t.Errorf("%s: got %d px, want %d px (size scale changed the rendered size)", k, gv, want[k])
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s: new entity absent from golden (regenerate if intended)", k)
		}
	}
}

// computeGoldenSpriteSizes renders every entity straight ahead of the camera
// (so perpendicular distance == the requested distance) at a couple of ranges,
// through the exact metric functions the engine uses, and returns key -> px.
func computeGoldenSpriteSizes(t *testing.T) map[string]int {
	t.Helper()
	cfg := loadTestConfig(t) // config + monsters
	previousTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previousTiles })
	world.GlobalTileManager = world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	g := &MMGame{
		config: cfg,
		world:  &world.World3D{},
		camera: &FirstPersonCamera{X: 320, Y: 320, Angle: 0, FOV: squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight()), ViewDist: cfg.GetViewDistance()},
	}
	g.renderHelper = NewRenderingHelper(g)

	// Both distances clear the 5-tile environment near-cull and sit well inside
	// the 50-tile view distance.
	distances := []float64{384, 640}
	out := map[string]int{}

	for _, d := range distances {
		ex := g.camera.X + d // straight ahead: perpDist == d
		ey := g.camera.Y

		for _, key := range monster.MonsterConfig.GetAllMonsterKeys() {
			def := monster.MonsterConfig.Monsters[key]
			_, _, size, _ := g.renderHelper.CalculateMonsterSpriteMetrics(ex, ey, d, def.GetSizeGameMultiplier())
			out[fmt.Sprintf("monster/%s@%.0f", key, d)] = size
		}

		for key, data := range character.NPCConfigInstance.NPCs {
			npc := &character.NPC{
				Sprite:         data.Sprite,
				RenderCategory: data.RenderCategory,
				SizeClass:      data.SizeClass,
				GridSpanTiles:  data.GridSpanTiles,
				GridSpanDir:    data.GridSpanDir,
			}
			_, _, size, _ := g.renderHelper.NPCSpriteMetrics(npc, ex, ey, d)
			out[fmt.Sprintf("npc/%s@%.0f", key, d)] = size
		}

		for _, key := range world.GlobalTileManager.GetAllTileKeys() {
			data := world.GlobalTileManager.GetTileDataByKey(key)
			if data == nil {
				continue
			}
			switch data.RenderType {
			case config.TileRenderCrossedStandee, config.TileRenderCrossedProp,
				config.TileRenderStandee, config.TileRenderLandmarkStandee:
			default:
				continue
			}
			tt, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
			if !ok {
				continue
			}
			_, _, size, _ := g.renderHelper.CalculateEnvironmentSpriteMetrics(ex, ey, d, tt, 1.0)
			out[fmt.Sprintf("tile/%s@%.0f", key, d)] = size
		}
	}
	return out
}

// A prop-class cross must size EXACTLY like the flat standee it replaced -
// authored class means VISIBLE HEIGHT - while a tree-class cross authors frame
// WIDTH and takes no alpha normalization. The two halves live apart
// (envHeightMultiplier normalizes, the draw picks the axis), so only their
// composition over REAL assets catches a contract flip; TestSpriteSizesGolden
// cannot, because with no sprite manager the normalization degrades to 1 and
// every render type collapses to the same number.
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
