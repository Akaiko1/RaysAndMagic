package world

import (
	"fmt"
	"ugataima/internal/config"
)

// Validate geometry before publishing a fresh visit or registering its actors.
func validateAdventureWorld(key string, w *World3D, a *config.AdventureConfig) error {
	if a == nil {
		return nil
	}
	inBounds := func(x, y int) bool { return x >= 0 && y >= 0 && x < w.Width && y < w.Height }
	fail := func(message string) error { return fmt.Errorf("map %q adventure: %s", key, message) }
	for _, link := range a.JumpLinks {
		for _, xy := range [][2]int{{link[0], link[1]}, {link[2], link[3]}} {
			if !inBounds(xy[0], xy[1]) || w.IsTileBlockingTerrainAt(xy[0], xy[1]) {
				return fail("blocked Jump endpoint")
			}
			for _, m := range w.Monsters {
				tile := float64(w.config.GetTileSize())
				if int(m.X/tile) == xy[0] && int(m.Y/tile) == xy[1] {
					return fail("monster on Jump endpoint")
				}
			}
		}
	}
	for _, e := range a.Effects {
		if !inBounds(e.Rect[0], e.Rect[1]) || !inBounds(e.Rect[2], e.Rect[3]) {
			return fail("effect outside map: " + e.ID)
		}
		if e.Kind != "transfer" {
			continue
		}
		x, y := e.Destination[0], e.Destination[1]
		if !inBounds(x, y) || w.IsTileBlockingTerrainAt(x, y) {
			return fail("blocked receiver: " + e.ID)
		}
		for _, other := range a.Effects {
			if other.Contains(x, y) && other.Kind != "lane" {
				return fail("receiver overlaps hazard: " + e.ID)
			}
		}
	}
	for _, c := range a.Controls {
		for _, t := range c.Tiles {
			if !inBounds(t.X, t.Y) {
				return fail("controlled tile outside map: " + c.ID)
			}
			if GlobalTileManager == nil || !GlobalTileManager.HasTileKey(t.Tile) {
				return fail("unknown controlled tile: " + t.Tile)
			}
		}
	}
	if a.Boss != nil {
		if b := a.Boss.Arena; b != nil && (!inBounds(b[0], b[1]) || !inBounds(b[2], b[3])) {
			return fail("boss arena outside map")
		}
		count := 0
		for _, m := range w.Monsters {
			if m.Key == a.Boss.Monster && m.IsBoss() {
				if b := a.Boss.Arena; b != nil {
					ts := float64(w.config.GetTileSize())
					x, y := int(m.X/ts), int(m.Y/ts)
					if x < b[0] || y < b[1] || x > b[2] || y > b[3] {
						return fail("boss spawn outside arena")
					}
				}
				count++
			}
		}
		if count != 1 {
			return fail("boss policy requires exactly one matching authored boss")
		}
	}
	w.adventure = a
	return nil
}
