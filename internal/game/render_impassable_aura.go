package game

import (
	"math"

	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Impassable-aura tuning that isn't worth a config knob. Rise height and bubble
// size are derived from the tile's on-screen size so near tiles get a taller,
// chunkier column and far ones stay faint.
const (
	auraRiseFraction   = 0.55 // fraction of the tile's floor-rise used as bubble travel
	auraBubblesPerCol  = 2    // particles per sampled column (staggered phases)
	auraRisePeriodTick = 95.0 // ticks for one bubble to travel bottom→top
	auraColorBoost     = 1.35 // brighten the tile colour so bubbles read as a glow
	auraMinDepth       = 12.0 // near clip (world units) to avoid huge close-up blobs
	auraColBrightMin   = 0.4  // dimmest a stream can be (1.0 = full); rest is random per stream
	auraSpeedJitterMin = 0.55 // per-bubble rise speed varies in [min, 2-min]×base period
)

// auraEdgeParams returns the edge-bubble tuning from the impassable-aura
// config with defaults applied — shared by the impassable aura, trap borders,
// and the spawn tile marker.
func (r *Renderer) auraEdgeParams() (baseAlpha float64, perEdge, radius int) {
	cfg := r.game.config.Graphics.ImpassableAura
	baseAlpha = cfg.Alpha
	if baseAlpha <= 0 {
		baseAlpha = 0.5
	}
	perEdge = cfg.BubblesPerEdge
	if perEdge <= 0 {
		perEdge = 3
	}
	radius = cfg.RadiusTiles
	if radius <= 0 {
		radius = 7
	}
	return baseAlpha, perEdge, radius
}

// drawImpassableTileAura draws a subtle stream of rising "bubble" pixels along
// the ground edges of impassable billboard tiles (rocks/cliffs) that border a
// walkable tile. Trees and textured walls are skipped — they already read as
// solid. The effect tells the player which tiles block movement without
// cluttering the scene; bubbles take the tile's own floor colour so they blend
// in, and are depth-tested against walls so they hide correctly behind geometry.
func (r *Renderer) drawImpassableTileAura(screen *ebiten.Image) {
	if !r.game.config.Graphics.ImpassableAura.Enabled || r.game.world == nil || world.GlobalTileManager == nil {
		return
	}

	baseAlpha, perEdge, radius := r.auraEdgeParams()

	ts := float64(r.game.config.GetTileSize())
	camTX := int(r.game.camera.X / ts)
	camTY := int(r.game.camera.Y / ts)
	maxDepth := float64(radius) * ts

	// Cardinal neighbours: a bubble edge is drawn only where the blocker faces a
	// walkable tile, so the aura outlines the boundary instead of filling clusters.
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

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
			showAura := isAuraBillboardRenderType(world.GlobalTileManager.GetRenderType(tile))
			if td := world.GlobalTileManager.GetTileData(tile); td != nil && td.ImpassableAura {
				showAura = true // floor pit (chasm): blocks but reads like ground
			}
			if !showAura {
				continue // trees, textured walls, ordinary floors: not ambiguous
			}
			// Interior tiles of a blocker cluster (all four neighbours also
			// block) have no walkable-facing edge — skip before the colour work.
			interior := true
			for _, d := range dirs {
				if !r.game.world.IsTileBlocking(tx+d[0], ty+d[1]) {
					interior = false
					break
				}
			}
			if interior {
				continue
			}
			// Bubble colour = average of the tile's sprite texture (so it matches
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
			for _, d := range dirs {
				if r.game.world.IsTileBlocking(tx+d[0], ty+d[1]) {
					continue // edge faces another blocker → interior, skip
				}
				r.emitAuraEdge(screen, tx, ty, d, ts, perEdge, baseAlpha, maxDepth, rgb)
			}
		}
	}
}

// emitAuraEdge samples points along the shared border between blocker tile
// (tx,ty) and its walkable neighbour in direction d, then draws rising bubbles
// at each sample.
func (r *Renderer) emitAuraEdge(screen *ebiten.Image, tx, ty int, d [2]int, ts float64, perEdge int, baseAlpha, maxDepth float64, rgb [3]int) {
	// The billboard sprite stands at the tile centre. Bubbles on the FAR side of
	// that plane are hidden by the sprite (which doesn't write the depth buffer),
	// so cull them geometrically against the centre's perpendicular depth — but
	// only where the sprite's silhouette actually covers them on screen: a
	// side-on edge extends past the billboard, and a depth-only cull erased the
	// far half of the barrier.
	cx := (float64(tx) + 0.5) * ts
	cy := (float64(ty) + 0.5) * ts
	_, centerDepth, centerOK := r.game.renderHelper.projectToScreenX(cx, cy)
	spanL, spanR := math.MinInt, math.MaxInt
	if centerOK {
		// Tile-wide camera-facing plane at the centre: offset perpendicular to the
		// view direction, so both endpoints share the centre's depth.
		ang := r.game.camera.Angle
		ox, oy := -math.Sin(ang)*ts/2, math.Cos(ang)*ts/2
		lx, _, lok := r.game.renderHelper.projectToScreenX(cx-ox, cy-oy)
		rx, _, rok := r.game.renderHelper.projectToScreenX(cx+ox, cy+oy)
		if lok && rok {
			spanL, spanR = min(lx, rx), max(lx, rx)
		}
	}
	edgeKey := d[0]*2 + d[1]

	for s := 0; s < perEdge; s++ {
		// Fractional position along the edge (0..1), inset from the corners.
		f := (float64(s) + 0.5) / float64(perEdge)

		var wx, wy float64
		if d[0] != 0 { // east/west edge: fixed X, vary Y
			if d[0] > 0 {
				wx = float64(tx+1) * ts
			} else {
				wx = float64(tx) * ts
			}
			wy = (float64(ty) + f) * ts
		} else { // north/south edge: fixed Y, vary X
			if d[1] > 0 {
				wy = float64(ty+1) * ts
			} else {
				wy = float64(ty) * ts
			}
			wx = (float64(tx) + f) * ts
		}

		// Per-stream brightness so the wall of bubbles shimmers unevenly instead
		// of being a flat band.
		colBright := auraColBrightMin + (1.0-auraColBrightMin)*auraHash(tx, ty, edgeKey+100, s)
		r.emitBubbleColumn(screen, bubbleColumnFx{
			wx: wx, wy: wy,
			hx: tx, hy: ty, salt: edgeKey, hi: s,
			maxDepth:     maxDepth,
			riseFraction: auraRiseFraction,
			baseAlpha:    baseAlpha,
			colBright:    colBright,
			perColumn:    auraBubblesPerCol,
			periodTick:   auraRisePeriodTick,
			jitterMin:    auraSpeedJitterMin,
			jitterSpan:   (1.0 - auraSpeedJitterMin) * 2,
			sizeFloor:    1.5,
			sizeCoef:     0.045,
			wobbleCoef:   0.6,
			color:        rgb,
			centerDepth:  centerDepth,
			hasCenter:    centerOK,
			centerSpanL:  spanL,
			centerSpanR:  spanR,
		})
	}
}

// isAuraBillboardRenderType reports whether a tile's render type is an
// "ambiguous" impassable billboard (rock/cliff/bush) that benefits from the
// ground-bubble hint. Trees (tree_sprite) and textured walls already read as
// solid, and floor_only tiles aren't blockers.
func isAuraBillboardRenderType(rt string) bool {
	return rt == "environment_sprite" || rt == "flooring_object"
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
	var rs, gs, bs, n uint64
	for i := 0; i+3 < len(buf); i += 4 {
		if buf[i+3] < 32 { // skip (near-)transparent texels — they aren't the rock
			continue
		}
		rs += uint64(buf[i])
		gs += uint64(buf[i+1])
		bs += uint64(buf[i+2])
		n++
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
