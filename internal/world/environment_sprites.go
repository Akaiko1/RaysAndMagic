package world

import "math/rand"

// EnvironmentSprite resolves authored natural variants without advancing an
// RNG during rendering. The seed belongs to the loaded world, so cache rebuilds,
// camera movement, region transitions and render modes cannot change its art.
func (w *World3D) EnvironmentSprite(tileType TileType3D, x, y int) string {
	if GlobalTileManager == nil {
		return ""
	}
	data := GlobalTileManager.GetTileData(tileType)
	if data == nil {
		return ""
	}
	if len(data.SpriteVariants) == 0 || w == nil {
		return data.Sprite
	}
	// Mix coordinates before taking the remainder, avoiding visible repeating
	// stripes when an authored list has only two or three entries.
	h := w.environmentSpriteSeed ^ uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xbf58476d1ce4e5b9 ^ uint64(tileType)*0x94d049bb133111eb
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	h ^= h >> 31
	return data.SpriteVariants[h%uint64(len(data.SpriteVariants))]
}

// RandomizeEnvironmentSprites starts a new visual layout when a save replaces
// the timeline. Save loading reuses worlds; ordinary map travel does not reroll.
func (wm *WorldManager) RandomizeEnvironmentSprites() {
	seen := make(map[*World3D]bool)
	reset := func(w *World3D) {
		if w != nil && !seen[w] {
			w.environmentSpriteSeed = rand.Uint64()
			seen[w] = true
		}
	}
	for _, w := range wm.LoadedMaps {
		reset(w)
	}
	reset(wm.OpenWorld)
}
