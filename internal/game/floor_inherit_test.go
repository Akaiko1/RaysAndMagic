package game

import (
	"image/color"
	"testing"

	"ugataima/internal/world"
)

func TestJapaneseCastleSpawn_InheritsCobbleFloor(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "japanese_castle")
	castle := wm.GetCurrentWorld()

	g := newTestGame(cfg, castle)
	r := NewRenderer(g)
	r.precomputeFloorColorCache()

	x, y := castle.StartX, castle.StartY
	tileType := castle.Tiles[y][x]
	if key := world.GlobalTileManager.GetTileKey(tileType); key != "spawn" {
		t.Fatalf("start tile key = %q, want spawn", key)
	}
	if group := r.floorTextureGroupForTile(x, y, tileType); group != "cobble" {
		t.Fatalf("spawn floor texture group = %q, want cobble", group)
	}
	if _, ok := r.floorTextureIndexForTile(x, y, tileType); !ok {
		t.Fatal("spawn tile should bake a floor texture index")
	}
	// The castle cobble floor applies its existing near-floor tint during cache bake.
	want := color.RGBA{118, 118, 126, 255}
	if got := r.floorColorCache[[2]int{x, y}]; got != want {
		t.Fatalf("spawn floor color = %#v, want %#v", got, want)
	}
}

func TestJapaneseCastleDecor_InheritsDominantNeighbourFloor(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "japanese_castle")
	castle := wm.GetCurrentWorld()
	g := newTestGame(cfg, castle)
	r := NewRenderer(g)

	tests := []struct {
		name        string
		propLetter  string
		floorLetter string
		wantGroup   string
		x, y        int
	}{
		{name: "lantern over wood", propLetter: "L", floorLetter: ",", wantGroup: "wood", x: 5, y: 5},
		{name: "bonsai over tatami", propLetter: "Y", floorLetter: ":", wantGroup: "tatami", x: 15, y: 15},
		{name: "shoji over garden", propLetter: "H", floorLetter: ";", wantGroup: "garden", x: 25, y: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prop, ok := world.GlobalTileManager.GetTileTypeFromLetterForBiome(tt.propLetter, "japanese_castle")
			if !ok {
				t.Fatalf("missing %q tile", tt.propLetter)
			}
			floor, ok := world.GlobalTileManager.GetTileTypeFromLetterForBiome(tt.floorLetter, "japanese_castle")
			if !ok {
				t.Fatalf("missing %q floor", tt.floorLetter)
			}
			if !world.GlobalTileManager.InheritsFloor(prop) {
				t.Fatal("decor without an authored floor should inherit it")
			}

			for y := tt.y - 1; y <= tt.y+1; y++ {
				for x := tt.x - 1; x <= tt.x+1; x++ {
					castle.Tiles[y][x] = floor
				}
			}
			castle.Tiles[tt.y][tt.x] = prop
			r.precomputeFloorColorCache()

			if got := r.floorTextureGroupForTile(tt.x, tt.y, prop); got != tt.wantGroup {
				t.Fatalf("floor texture group = %q, want %q", got, tt.wantGroup)
			}
			wantRGB := world.GlobalTileManager.GetFloorColor(floor)
			wantColor := color.RGBA{R: uint8(wantRGB[0]), G: uint8(wantRGB[1]), B: uint8(wantRGB[2]), A: 255}
			if got := r.floorColorCache[[2]int{tt.x, tt.y}]; got != wantColor {
				t.Fatalf("floor color = %#v, want %#v", got, wantColor)
			}
		})
	}
}
