package world

import (
	"strings"
	"testing"

	"ugataima/internal/config"
)

// Water bodies are 8-connected runs of type: water tiles (streams included),
// each named by exactly one entry; every authoring mistake fails the load
// with the map's config path.
func TestResolveWaterBodies(t *testing.T) {
	tm := installTestTileManager(t)
	tile := func(key string) TileType3D {
		v, ok := tm.GetTileTypeFromKey(key)
		if !ok {
			t.Fatalf("missing tile %s", key)
		}
		return v
	}
	legend := map[rune]TileType3D{'.': tile("empty"), 'W': tile("water"), 'Q': tile("deep_water"), 'S': tile("forest_stream")}
	grid := []string{
		".WW..S",
		".QW...",
		"...W..", // (3,2) touches the lake only diagonally
		"......",
		"SS....",
	}
	newWorld := func() *World3D {
		w := &World3D{Width: len(grid[0]), Height: len(grid)}
		for _, row := range grid {
			line := make([]TileType3D, 0, len(row))
			for _, c := range row {
				line = append(line, legend[c])
			}
			w.Tiles = append(w.Tiles, line)
		}
		return w
	}
	lake := config.MapWaterBody{At: [2]int{1, 0}, Kind: config.WaterBodyLake}
	brook := config.MapWaterBody{At: [2]int{5, 0}, Kind: config.WaterBodyStream}
	river := config.MapWaterBody{At: [2]int{0, 4}, Kind: config.WaterBodyRiver}

	for _, tc := range []struct {
		name    string
		entries []config.MapWaterBody
		err     string
	}{
		{"unlisted body", []config.MapWaterBody{lake, brook}, "unlisted water bodies {at: [0, 4], kind: ?} (2 cells)"},
		{"entry on land", []config.MapWaterBody{lake, brook, river, {At: [2]int{0, 0}, Kind: config.WaterBodyPool}}, "[3] at [0, 0]: not a water tile"},
		{"entry off the map", []config.MapWaterBody{lake, brook, river, {At: [2]int{9, 9}, Kind: config.WaterBodyPool}}, "[3] at [9, 9]: not a water tile"},
		{"two entries on one body", []config.MapWaterBody{lake, brook, river, {At: [2]int{3, 2}, Kind: config.WaterBodySea}}, "[3] at [3, 2]: the same body as entry 0"},
		{"unknown kind", []config.MapWaterBody{{At: [2]int{1, 0}, Kind: "lak"}, brook, river}, `[0] at [1, 0]: kind "lak" is not one of river|lake|sea|pool|stream`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := newWorld().ResolveWaterBodies("probe", tc.entries)
			if err == nil || !strings.Contains(err.Error(), "maps.probe.water_bodies") || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("error = %v, want one naming maps.probe.water_bodies and %q", err, tc.err)
			}
		})
	}

	w := newWorld()
	if err := w.ResolveWaterBodies("probe", []config.MapWaterBody{brook, river, lake}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		at   [2]int
		want config.WaterBodyKind
	}{
		{[2]int{2, 0}, config.WaterBodyLake},
		{[2]int{1, 1}, config.WaterBodyLake}, // deep water of the same lake
		{[2]int{3, 2}, config.WaterBodyLake}, // diagonal contact
		{[2]int{5, 0}, config.WaterBodyStream},
		{[2]int{1, 4}, config.WaterBodyRiver},
		{[2]int{0, 0}, ""},  // land
		{[2]int{-1, 0}, ""}, // off the map
	} {
		if got := w.WaterBodyKind(c.at[0], c.at[1]); got != c.want {
			t.Errorf("kind at %v = %q, want %q", c.at, got, c.want)
		}
	}
	// A tile changed after load answers for what it is now.
	w.Tiles[0][2] = legend['.']
	w.Tiles[3][0] = legend['W']
	if got := w.WaterBodyKind(2, 0); got != "" {
		t.Errorf("drained cell kind = %q, want none", got)
	}
	if got := w.WaterBodyKind(0, 3); got != "" {
		t.Errorf("water the map did not place kind = %q, want none", got)
	}
}
