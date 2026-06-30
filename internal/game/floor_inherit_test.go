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
