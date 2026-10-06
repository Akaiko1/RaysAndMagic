package game

import (
	"image"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Shared ground-effect tuning. Curtain and particle heights follow floor
// perspective; distant effects fade before their configured range limit.
const (
	auraRiseFraction   = 0.55 // curtain height / particle travel relative to floor-rise
	auraRisePeriodTick = 95.0 // ticks for one bubble to travel bottom->top
	auraColorBoost     = 1.35 // brighten the tile colour so the boundary reads as a glow
	auraMinDepth       = 12.0 // near clip (world units) to avoid huge close-up blobs
	auraSpeedJitterMin = 0.55 // per-bubble rise speed varies in [min, 2-min]xbase period
)

var auraCardinalDirections = [...][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// auraEdgeParams returns the edge-effect tuning from the impassable-aura
// config with defaults applied - shared by the impassable aura, trap borders,
// and the spawn tile marker.
func (r *Renderer) auraEdgeParams() (baseAlpha, density float64, radius int) {
	cfg := r.game.config.Graphics.ImpassableAura
	baseAlpha = cfg.Alpha
	if baseAlpha <= 0 {
		baseAlpha = 0.5
	}
	density = cfg.FoldDensity
	if density <= 0 {
		density = 2
	}
	radius = cfg.RadiusTiles
	if radius <= 0 {
		radius = 7
	}
	return baseAlpha, density, radius
}

func (r *Renderer) collectAuraTileEdges(
	tx, ty int,
	ts float64,
	density float64,
	baseAlpha, maxDepth float64,
	rgb [3]int,
) {
	for _, direction := range auraCardinalDirections {
		r.collectAuraEdge(tx, ty, direction, ts, auraRiseFraction, density, baseAlpha, maxDepth, rgb)
	}
}

// collectImpassableTileAura outlines authored blockers with low light curtains.
// Only walkable-facing edges are drawn; the tint follows the tile's sprite or
// floor colour. The shared painter pass places every edge among scenery/actors.
func (r *Renderer) collectImpassableTileAura() {
	if !r.game.config.Graphics.ImpassableAura.Enabled || r.game.world == nil || world.GlobalTileManager == nil {
		return
	}

	baseAlpha, density, radius := r.auraEdgeParams()

	ts := float64(r.game.config.GetTileSize())
	camTX := TileIndex(r.game.camera.X, ts)
	camTY := TileIndex(r.game.camera.Y, ts)
	maxDepth := float64(radius) * ts

	// Cardinal neighbours: a light curtain is drawn only where the blocker faces a
	// walkable tile, so the aura outlines the boundary instead of filling clusters.
	for ty := camTY - radius; ty <= camTY+radius; ty++ {
		if ty < 0 || ty >= r.game.world.Height {
			continue
		}
		for tx := camTX - radius; tx <= camTX+radius; tx++ {
			if tx < 0 || tx >= r.game.world.Width {
				continue
			}
			if !r.game.world.IsTileBlocking(tx, ty) {
				continue
			}
			tile := r.game.world.Tiles[ty][tx]
			if !tileShowsImpassableAura(world.GlobalTileManager.GetTileData(tile)) {
				continue
			}
			// Interior tiles of a blocker cluster (all four neighbours also
			// block) have no walkable-facing edge - skip before the colour work.
			interior := true
			for _, d := range auraCardinalDirections {
				if !r.game.world.IsTileBlocking(tx+d[0], ty+d[1]) {
					interior = false
					break
				}
			}
			if interior {
				continue
			}
			// Curtain colour = average of the tile's sprite texture (so it matches
			// the rock/cliff), falling back to the tile's floor colour.
			base, ok := r.auraTileColor(tile)
			if !ok {
				clr := r.floorColorCache[[2]int{tx, ty}]
				base = [3]int{int(clr.R), int(clr.G), int(clr.B)}
			}
			rgb := [3]int{
				clampColor(int(float64(base[0]) * auraColorBoost)),
				clampColor(int(float64(base[1]) * auraColorBoost)),
				clampColor(int(float64(base[2]) * auraColorBoost)),
			}
			r.statAuraTiles++
			for _, d := range auraCardinalDirections {
				if r.game.world.IsTileBlocking(tx+d[0], ty+d[1]) {
					continue // edge faces another blocker -> interior, skip
				}
				r.collectAuraEdge(tx, ty, d, ts, auraRiseFraction, density, baseAlpha, maxDepth, rgb)
			}
		}
	}
}

// tileEdgeSamplePoint is the world position of sample s (of perEdge) along
// tile (tx,ty)'s edge in direction d, inset from the corners. THE shared
// sampling for particle edges; these points lie on the same tile boundary
// as the continuous aura curtain.
func tileEdgeSamplePoint(tx, ty int, d [2]int, ts float64, s, perEdge int) (wx, wy float64) {
	f := (float64(s) + 0.5) / float64(perEdge)
	if d[0] != 0 { // east/west edge: fixed X, vary Y
		if d[0] > 0 {
			wx = float64(tx+1) * ts
		} else {
			wx = float64(tx) * ts
		}
		return wx, (float64(ty) + f) * ts
	}
	// north/south edge: fixed Y, vary X
	if d[1] > 0 {
		wy = float64(ty+1) * ts
	} else {
		wy = float64(ty) * ts
	}
	return (float64(tx) + f) * ts, wy
}

// tileShowsImpassableAura reports the authored impassable_aura opt-in. Ground
// that looks walkable but is blocked (such as a chasm floor) needs this hint;
// solid scenery does not acquire a curtain merely from its render type.
func tileShowsImpassableAura(data *config.TileData) bool {
	return data != nil && data.ImpassableAura
}

// auraTileColor returns the average RGB of a tile's billboard sprite texture
// (ignoring near-transparent pixels), cached per tile type. ok=false when there
// is no usable sprite, so the caller falls back to the tile's floor colour.
func (r *Renderer) auraTileColor(tileType world.TileType3D) ([3]int, bool) {
	if r.auraTileColorCache == nil {
		r.auraTileColorCache = make(map[world.TileType3D][3]int)
	}
	if c, ok := r.auraTileColorCache[tileType]; ok {
		return c, c != [3]int{-1, -1, -1}
	}

	rgb, ok := r.computeAuraTileColor(tileType)
	if ok {
		r.auraTileColorCache[tileType] = rgb
	} else {
		r.auraTileColorCache[tileType] = [3]int{-1, -1, -1} // sentinel: "no sprite colour"
	}
	return rgb, ok
}

func (r *Renderer) computeAuraTileColor(tileType world.TileType3D) ([3]int, bool) {
	if world.GlobalTileManager == nil || r.game.sprites == nil {
		return [3]int{}, false
	}
	spriteName := world.GlobalTileManager.GetSprite(tileType)
	if spriteName == "" {
		return [3]int{}, false
	}
	img := r.game.sprites.GetSprite(spriteName)
	if img == nil {
		return [3]int{}, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return [3]int{}, false
	}
	buf := make([]byte, 4*w*h)
	img.ReadPixels(buf)
	return computeAuraTileColorFromPixels(&image.RGBA{
		Pix: buf, Stride: 4 * w, Rect: image.Rect(0, 0, w, h),
	})
}

func computeAuraTileColorFromPixels(img *image.RGBA) ([3]int, bool) {
	if img == nil {
		return [3]int{}, false
	}
	var rs, gs, bs, n uint64
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			i := img.PixOffset(x, y)
			if img.Pix[i+3] < 32 { // skip transparent texels - they are not the rock
				continue
			}
			rs += uint64(img.Pix[i])
			gs += uint64(img.Pix[i+1])
			bs += uint64(img.Pix[i+2])
			n++
		}
	}
	if n == 0 {
		return [3]int{}, false
	}
	return [3]int{int(rs / n), int(gs / n), int(bs / n)}, true
}

// auraHash returns a deterministic pseudo-random value in [0,1) for a particle,
// so phases vary between bubbles/edges/tiles without per-frame randomness.
func auraHash(a, b, c, d int) float64 {
	h := uint32(a)*73856093 ^ uint32(b)*19349663 ^ uint32(c)*83492791 ^ uint32(d)*2654435761
	return float64(h&0xffff) / 65536.0
}
