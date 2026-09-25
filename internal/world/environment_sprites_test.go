package world

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/config"
)

func TestAuthoredEnvironmentVariantsHaveMatchingFrames(t *testing.T) {
	tm := NewTileManager(testTileSizeClasses())
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	for key, data := range tm.tileData {
		if len(data.SpriteVariants) == 0 {
			continue
		}
		t.Run(key, func(t *testing.T) {
			width, height := 0, 0
			for _, name := range data.SpriteVariants {
				file, err := os.Open(filepath.Join("../../assets/sprites/environment/nature", name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				frame, err := png.DecodeConfig(file)
				file.Close()
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if width == 0 {
					width, height = frame.Width, frame.Height
				}
				if frame.Width*height != frame.Height*width {
					t.Fatalf("%s changes the crossed standee aspect: %dx%d, original %dx%d", name, frame.Width, frame.Height, width, height)
				}
			}
		})
	}
}

func TestEnvironmentSpriteSelection(t *testing.T) {
	previous := GlobalTileManager
	t.Cleanup(func() { GlobalTileManager = previous })
	tm := NewTileManager(testTileSizeClasses())
	tm.tileData["tree"] = &config.TileData{Sprite: "oak", SpriteVariants: []string{"oak", "forked", "leaning"}}
	tm.tileData["stone"] = &config.TileData{Sprite: "stone"}
	tm.createTypeMapping()
	GlobalTileManager = tm
	tree, _ := tm.GetTileTypeFromKey("tree")
	stone, _ := tm.GetTileTypeFromKey("stone")
	layout := func(w *World3D) []string {
		var names []string
		for y := -4; y < 12; y++ {
			for x := -4; x < 12; x++ {
				names = append(names, w.EnvironmentSprite(tree, x, y))
			}
		}
		return names
	}
	w := &World3D{environmentSpriteSeed: 42}
	first := layout(w)
	if !reflect.DeepEqual(first, layout(w)) {
		t.Fatal("the same world changed appearances between reads")
	}
	counts := make(map[string]int)
	for _, name := range first {
		counts[name]++
	}
	for _, name := range tm.tileData["tree"].SpriteVariants {
		if counts[name] < 40 {
			t.Fatalf("variant %q is missing or badly clustered: %v", name, counts)
		}
	}
	if len(counts) != 3 {
		t.Fatalf("selection escaped the authored list: %v", counts)
	}
	w.environmentSpriteSeed = 99
	if reflect.DeepEqual(first, layout(w)) {
		t.Fatal("different world seeds produced the same layout")
	}
	if got := w.EnvironmentSprite(stone, 9, 2); got != "stone" {
		t.Fatalf("single-sprite tile changed: %q", got)
	}
	if got := w.EnvironmentSprite(TileType3D(-1), 0, 0); got != "" {
		t.Fatalf("unknown tile returned %q", got)
	}
	wm := &WorldManager{LoadedMaps: map[string]*World3D{"one": w, "alias": w}, OpenWorld: &World3D{}}
	wm.RandomizeEnvironmentSprites()
	if reflect.DeepEqual(first, layout(w)) || w.environmentSpriteSeed == 99 || wm.OpenWorld.environmentSpriteSeed == 0 {
		t.Fatal("save load did not reroll split and stitched worlds")
	}
	a, b := NewWorld3D(nil), NewWorld3D(nil)
	if a.environmentSpriteSeed == b.environmentSpriteSeed {
		t.Fatal("new worlds reused a visual seed")
	}
}

func TestEnvironmentSpriteVariantValidation(t *testing.T) {
	for _, tc := range []struct {
		name, renderType string
		variants         []string
		valid            bool
	}{
		{"legacy", config.TileRenderCrossedStandee, nil, true},
		{"list", config.TileRenderCrossedStandee, []string{"oak", "oak_a", "oak_b"}, true},
		{"wrong_default", config.TileRenderCrossedStandee, []string{"other", "oak"}, false},
		{"empty", config.TileRenderCrossedStandee, []string{"oak", ""}, false},
		{"duplicate", config.TileRenderCrossedStandee, []string{"oak", "oak"}, false},
		{"path", config.TileRenderCrossedStandee, []string{"oak", "nature/other"}, false},
		{"extension", config.TileRenderCrossedStandee, []string{"oak", "other.png"}, false},
		{"space", config.TileRenderCrossedStandee, []string{"oak", " other"}, false},
		{"wrong_render_type", config.TileRenderCrossedProp, []string{"oak", "other"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm := NewTileManager(testTileSizeClasses())
			tm.tileData["sample"] = &config.TileData{
				Name: "Sample", Type: "nature", RenderType: tc.renderType,
				Sprite: "oak", SpriteVariants: tc.variants, SizeClass: "tree",
			}
			if err := tm.validateTileConfiguration(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
