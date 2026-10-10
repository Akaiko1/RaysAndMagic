package game

import (
	"image/color"
	"slices"
	"sort"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// A tile that inherits its floor takes the surrounding floor's texture group
// and colour. Explicit markers (inherit_floor: spawn, teleporters) also take
// the floor's near tint; ordinary props and walls match the floor exactly.
// Every castle floor is checked under every marker and every castle inheritor.
func TestJapaneseCastleInheritedFloors(t *testing.T) {
	const biome = "japanese_castle"
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, biome)
	castle := wm.GetCurrentWorld()
	g := newTestGame(cfg, castle)
	r := NewRenderer(g)
	tm := world.GlobalTileManager

	keys := allTileKeys(tm)
	sort.Strings(keys)
	var floors, markers, decor []string
	for _, key := range keys {
		data := tm.GetTileDataByKey(key)
		inCastle := slices.Contains(data.Biomes, biome)
		switch {
		case data.InheritFloor:
			markers = append(markers, key)
		case !inCastle:
		case data.RenderType == config.TileRenderFloor && data.FloorTextureGroup != "":
			floors = append(floors, key)
		case data.InheritsNeighbourFloor():
			decor = append(decor, key)
		}
	}
	if len(floors) == 0 || len(markers) == 0 || len(decor) == 0 {
		t.Fatalf("fixture needs castle floors, markers and decor: %d/%d/%d", len(floors), len(markers), len(decor))
	}
	mapDefault := wm.GetCurrentMapConfig().DefaultFloorColor
	rgba := func(c [3]int) color.RGBA { return color.RGBA{R: uint8(c[0]), G: uint8(c[1]), B: uint8(c[2]), A: 255} }

	const x, y = 5, 5
	for _, floorKey := range floors {
		floor, _ := tm.GetTileTypeFromKey(floorKey)
		floorData := tm.GetTileData(floor)
		base := floorData.FloorColor
		if base == ([3]int{}) {
			base = mapDefault
		}
		for _, propKey := range append(append([]string(nil), markers...), decor...) {
			prop, _ := tm.GetTileTypeFromKey(propKey)
			marker := tm.GetTileData(prop).InheritFloor
			t.Run(floorKey+"/"+propKey, func(t *testing.T) {
				for ty := y - 1; ty <= y+1; ty++ {
					for tx := x - 1; tx <= x+1; tx++ {
						castle.Tiles[ty][tx] = floor
					}
				}
				castle.Tiles[y][x] = prop
				r.precomputeFloorColorCache()

				if got := r.floorTextureGroupForTile(x, y, prop); got != floorData.FloorTextureGroup {
					t.Fatalf("floor texture group = %q, want %q", got, floorData.FloorTextureGroup)
				}
				if r.resolvedFloorMaterial(x, y, prop).atlas == 0 {
					t.Fatal("inherited floor baked no texture index")
				}
				want := rgba(base)
				if marker && tm.HasFloorNearColor(floor) {
					want = rgba(floorData.FloorNearColor)
				}
				if got := r.floorColorCache[[2]int{x, y}]; got != want {
					t.Fatalf("floor color = %#v, want %#v (marker=%v)", got, want, marker)
				}
			})
		}
	}
}
