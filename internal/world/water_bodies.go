package world

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"ugataima/internal/config"
)

// waterBodiesError is an authoring mistake in water_bodies: it fails the boot
// for split maps too, not just the one map (LoadAllMaps).
type waterBodiesError struct{ msg string }

func (e *waterBodiesError) Error() string { return e.msg }

// isWaterBodiesError reports whether err is a water_bodies authoring error.
func isWaterBodiesError(err error) bool {
	var wb *waterBodiesError
	return errors.As(err, &wb)
}

// resolveWaterBodies labels every cell of each 8-connected body of water
// tiles (TileManager.IsWater) with the kind of the one water_bodies entry
// naming it: index+1 into config.WaterBodyKinds, 0 off water. It reads the
// parsed .map tiles, NPC and monster ground_tile stamps included, before the
// open-world removals, so both world modes resolve the same bodies. An entry
// off water, two entries on one body, an unknown kind or an unlisted body is
// a waterBodiesError. nil when the map has no water.
func resolveWaterBodies(mapKey string, entries []config.MapWaterBody, tiles [][]TileType3D, width, height int) ([]uint8, error) {
	if GlobalTileManager == nil {
		return nil, nil
	}
	where := "assets/map_configs.yaml: maps." + mapKey + ".water_bodies"
	isWater := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < width && y < height && GlobalTileManager.IsWater(tiles[y][x])
	}
	body := make([]int, width*height) // body number + 1; 0 off water
	var first [][2]int                // the first cell of each body in row order
	var size []int
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !isWater(x, y) || body[y*width+x] != 0 {
				continue
			}
			first = append(first, [2]int{x, y})
			id := len(first)
			body[y*width+x] = id
			cells := 0
			for stack := [][2]int{{x, y}}; len(stack) > 0; {
				c := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cells++
				for ny := c[1] - 1; ny <= c[1]+1; ny++ {
					for nx := c[0] - 1; nx <= c[0]+1; nx++ {
						if isWater(nx, ny) && body[ny*width+nx] == 0 {
							body[ny*width+nx] = id
							stack = append(stack, [2]int{nx, ny})
						}
					}
				}
			}
			size = append(size, cells)
		}
	}
	kind := make([]uint8, len(first))
	entryOf := make([]int, len(first)) // entry index + 1
	fail := func(format string, args ...any) ([]uint8, error) {
		return nil, &waterBodiesError{fmt.Sprintf(format, args...)}
	}
	for i, e := range entries {
		at := fmt.Sprintf("%s[%d] at [%d, %d]", where, i, e.At[0], e.At[1])
		k := slices.Index(config.WaterBodyKinds, e.Kind)
		if k < 0 {
			return fail("%s: kind %q is not one of %s", at, e.Kind, config.WaterBodyKindList())
		}
		if !isWater(e.At[0], e.At[1]) {
			return fail("%s: not a water tile", at)
		}
		id := body[e.At[1]*width+e.At[0]]
		if prev := entryOf[id-1]; prev != 0 {
			return fail("%s: the same body as entry %d", at, prev-1)
		}
		entryOf[id-1] = i + 1
		kind[id-1] = uint8(k + 1)
	}
	var missing []string
	for i, cell := range first {
		if entryOf[i] == 0 {
			missing = append(missing, fmt.Sprintf("{at: [%d, %d], kind: ?} (%d cells)", cell[0], cell[1], size[i]))
		}
	}
	if len(missing) > 0 {
		return fail("%s: unlisted water bodies %s; kind is %s", where, strings.Join(missing, ", "), config.WaterBodyKindList())
	}
	if len(first) == 0 {
		return nil, nil
	}
	out := make([]uint8, width*height)
	for i, id := range body {
		if id != 0 {
			out[i] = kind[id-1]
		}
	}
	return out, nil
}

// ResolveWaterBodies classifies the world's water from its map config entries
// (see resolveWaterBodies). The map loader calls it on the parsed .map tiles.
func (w *World3D) ResolveWaterBodies(mapKey string, entries []config.MapWaterBody) error {
	kinds, err := resolveWaterBodies(mapKey, entries, w.Tiles, w.Width, w.Height)
	if err != nil {
		return err
	}
	w.waterBodies = kinds
	return nil
}

// WaterBodyKind is the authored kind of the water body a tile belongs to; ""
// off the map, off water, and on water a terrain change placed after load.
func (w *World3D) WaterBodyKind(x, y int) config.WaterBodyKind {
	if w == nil || x < 0 || y < 0 || x >= w.Width || y >= w.Height || len(w.waterBodies) != w.Width*w.Height ||
		GlobalTileManager == nil || !GlobalTileManager.IsWater(w.Tiles[y][x]) {
		return ""
	}
	if v := w.waterBodies[y*w.Width+x]; v != 0 {
		return config.WaterBodyKinds[v-1]
	}
	return ""
}
