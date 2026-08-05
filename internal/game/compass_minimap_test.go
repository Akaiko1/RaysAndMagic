package game

import (
	"image/color"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestJapaneseCastleCompassUsesAuthoredFloorColorsAndSprites(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "japanese_castle")
	castle := wm.GetCurrentWorld()
	if castle == nil {
		t.Fatal("japanese castle world missing")
	}

	g := newTestGame(cfg, castle)
	ui := NewUISystem(g)
	floorColors := make(map[color.RGBA]struct{})
	sprites := make(map[string]struct{})
	for y, row := range castle.Tiles {
		for x, tile := range row {
			fc := g.floorColorForTile(x, y, [3]int{60, 110, 60})
			visual := ui.compassTileAppearance(x, y, compassRGB(fc, 235))
			data := world.GlobalTileManager.GetTileData(tile)
			if data != nil && data.RenderType == config.TileRenderFloor {
				floorColors[visual.floor] = struct{}{}
			}
			if visual.sprite != "" {
				sprites[visual.sprite] = struct{}{}
			}
		}
	}

	if len(floorColors) < 4 {
		t.Fatalf("Japanese castle compass has %d floor colors, want the authored cobble, wood, tatami, and garden fields", len(floorColors))
	}
	for _, name := range []string{
		"japanese_castle_wall_0",
		"japanese_castle_wall_1",
		"japanese_castle_wall_2",
		"japanese_castle_wall_3",
	} {
		if _, ok := sprites[name]; !ok {
			t.Fatalf("Japanese castle compass is missing authored wall thumbnail %q", name)
		}
	}
}
