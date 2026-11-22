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
	h := w.environmentHash(x, y, uint64(tileType)*0x94d049bb133111eb)
	return data.SpriteVariants[h%uint64(len(data.SpriteVariants))]
}

// environmentCoinSalt keeps the coin independent of the variant pick.
const environmentCoinSalt = 0xd6e8feb86659fd93

// EnvironmentCoin is a per-tile heads or tails from the same layout seed as
// the sprite variants: fixed while the world's layout stands, independent
// from tile to tile.
func (w *World3D) EnvironmentCoin(x, y int) bool {
	return w != nil && w.environmentHash(x, y, environmentCoinSalt)&1 == 1
}

// environmentHash mixes a tile and a salt with the world's layout seed. The
// coordinates are mixed before any remainder is taken, avoiding visible
// repeating stripes when a choice has only two or three outcomes.
func (w *World3D) environmentHash(x, y int, salt uint64) uint64 {
	h := w.environmentSpriteSeed ^ uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xbf58476d1ce4e5b9 ^ salt
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	return h ^ h>>31
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
