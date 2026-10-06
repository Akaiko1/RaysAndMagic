package game

import (
	"image/color"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Every castle tile reaches the compass through its own authored data: a
// floor with its own floor_color paints that color (inheriting markers take a
// neighbour's), any other tile with art shows its own sprite thumbnail.
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
			data := world.GlobalTileManager.GetTileData(tile)
			if data == nil {
				continue
			}
			fc := g.floorColorForTile(x, y, [3]int{60, 110, 60})
			visual := ui.compassTileAppearance(x, y, compassRGB(fc, 235))
			switch {
			case data.RenderType == config.TileRenderFloor && data.FloorColor != [3]int{} && !world.GlobalTileManager.InheritsFloor(tile):
				if want := compassRGB(data.FloorColor, 235); visual.floor != want || visual.sprite != "" {
					t.Fatalf("floor %q at (%d,%d) = %v/%q, want its authored color %v", world.GlobalTileManager.GetTileKey(tile), x, y, visual.floor, visual.sprite, want)
				}
				floorColors[visual.floor] = struct{}{}
			case data.RenderType != config.TileRenderFloor && data.Sprite != "":
				if want := normalizedAuthoredSpriteName(data.Sprite); visual.sprite != want {
					t.Fatalf("tile %q at (%d,%d) shows %q, want its own sprite %q", world.GlobalTileManager.GetTileKey(tile), x, y, visual.sprite, want)
				}
				sprites[visual.sprite] = struct{}{}
			}
		}
	}
	// Positive controls: the castle authors several floor fields and walls.
	if len(floorColors) < 2 || len(sprites) == 0 {
		t.Fatalf("castle compass drew %d authored floor colors and %d sprites, want several of each", len(floorColors), len(sprites))
	}
}
