package game

import (
	"image"
	"math"
	"sort"

	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Standee rendering: an entity drawn as a thick wooden token standing in the
// world (a board-game standee) instead of a camera-facing billboard. The token
// is a slab: front and back sticker faces (the sprite, mirrored on the back)
// separated by a wooden core. Each screen column whose ray crosses a surface
// gets a 1px texture slice at the intersection's perpendicular depth, so the
// near edge renders larger than the far edge, the core rim becomes visible at
// viewing angles, and walls occlude per column via the wall depth buffer.

const (
	standeeCoreShade    = 0.92          // token rim sits just out of the light vs the face
	standeeCoreShadeFar = 0.75          // the slab's far edge is in its own shadow
	standeeMaxShells    = 16            // cap on core shell layers (perf guard at point-blank range)
	standeeMinDepth     = 4.0           // near clip for token columns (world units)
	standeeStaticYaw    = math.Pi / 4.0 // fixed diagonal for scenery and NPC tokens
	standeeTurnDefault  = 270.0         // deg/sec token swivel when config omits it
	containerSpinDegSec = 60.0          // deg/sec idle spin for loot-bag / chest tokens
)

var standeeWoodTone = [3]float64{0.62, 0.45, 0.27}

// standeeEnvYawState is the eased facing of one scenery token (keyed by tile
// in Renderer.standeeEnvYaw): current yaw plus the frame it was last advanced,
// so a token unseen for a while snaps instead of visibly catching up.
type standeeEnvYawState struct {
	yaw  float64
	tick int64
}

// standeeCoreKey identifies one sprite frame in the wood-silhouette cache.
// NPC frames are sheet SubImages recreated every draw - their pointers are
// unstable but their absolute bounds distinguish frames. Monster animation
// frames are standalone images built once at load - their bounds are all
// (0,0,w,h) and would collide (freezing the wood on one pose), but their
// pointers are stable: include the image pointer for them.
type standeeCoreKey struct {
	name   string
	bounds image.Rectangle
	img    *ebiten.Image // only set when the caller's frame images are stable
}

// standeeCoreSilhouette returns (building and caching on first use) the sprite
// frame's silhouette filled with the configured wood-to-sprite-average color
// blend plus faint horizontal grain, so the token core follows the die-cut art.
func (r *Renderer) standeeCoreSilhouette(key standeeCoreKey, src *ebiten.Image) *ebiten.Image {
	if img, ok := r.standeeCoreCache[key]; ok {
		return img
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	buf := make([]byte, 4*w*h)
	src.ReadPixels(buf)

	// Perceived color of the art: a chroma-weighted average of the opaque
	// texels. A plain mean reads wrong - dark outlines and brown gear drown a
	// goblin's green skin - so saturated pixels dominate and near-grey ones
	// barely vote (the +0.02 floor keeps monochrome sprites at their own grey).
	var sumR, sumG, sumB, sumW float64
	for i := 0; i+3 < len(buf); i += 4 {
		a := float64(buf[i+3])
		if a < 24 {
			continue
		}
		// Un-premultiply to straight 0..1 color.
		cr := float64(buf[i]) / a
		cg := float64(buf[i+1]) / a
		cb := float64(buf[i+2]) / a
		chroma := math.Max(cr, math.Max(cg, cb)) - math.Min(cr, math.Min(cg, cb))
		w := chroma + 0.02
		sumR += cr * w
		sumG += cg * w
		sumB += cb * w
		sumW += w
	}
	tone := standeeWoodTone
	if sumW > 0 {
		tint := r.game.config.Graphics.Standee.CoreTint
		if tint < 0 {
			tint = 0
		} else if tint > 1 {
			tint = 1
		}
		tone[0] += (sumR/sumW - tone[0]) * tint
		tone[1] += (sumG/sumW - tone[1]) * tint
		tone[2] += (sumB/sumW - tone[2]) * tint
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		// Horizontal grain: a slow brightness wave down the slab edge.
		grain := 1.0 + 0.08*math.Sin(float64(y)*0.9)
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			a := buf[i+3]
			if a < 24 {
				continue
			}
			o := y*out.Stride + x*4
			out.Pix[o] = clampByte(tone[0] * grain * float64(a))
			out.Pix[o+1] = clampByte(tone[1] * grain * float64(a))
			out.Pix[o+2] = clampByte(tone[2] * grain * float64(a))
			out.Pix[o+3] = a
		}
	}
	img := ebiten.NewImageFromImage(out)
	if r.standeeCoreCache == nil {
		r.standeeCoreCache = make(map[standeeCoreKey]*ebiten.Image)
	}
	r.standeeCoreCache[key] = img
	return img
}

func clampByte(v float64) byte {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return byte(v)
}

// standeeColumnHit intersects one screen column's ray (origin cam, direction R,
// both in world space) with a surface segment P0 + u*(P1-P0), u in [0,1].
// Returns the ray parameter t - which IS the perpendicular depth when R is
// built as dir + plane*s with |dir| = 1 - and the segment fraction u.
func standeeColumnHit(camX, camY, rx, ry, p0x, p0y, dx, dy float64) (t, u float64, ok bool) {
	det := dx*ry - dy*rx
	if math.Abs(det) < 1e-9 {
		return 0, 0, false // ray parallel to the token plane (edge-on)
	}
	ex := p0x - camX
	ey := p0y - camY
	t = (dx*ey - dy*ex) / det
	u = (rx*ey - ry*ex) / det
	if t < standeeMinDepth || u < 0 || u > 1 {
		return 0, 0, false
	}
	return t, u, true
}

// clampYawFromEdgeOn keeps a token's long axis at least minRad away from the
// camera's sight line to the entity, so the slab never degenerates into an
// invisible edge-on sliver (a monster crossing the view would otherwise
// vanish). Slabs have period pi: deviation is measured in (-pi/2, pi/2] and the
// yaw is pushed to the nearest readable side.
func clampYawFromEdgeOn(yaw, viewAngle, minRad float64) float64 {
	d := math.Mod(yaw-viewAngle, math.Pi)
	if d <= -math.Pi/2 {
		d += math.Pi
	} else if d > math.Pi/2 {
		d -= math.Pi
	}
	if math.Abs(d) >= minRad {
		return yaw
	}
	if d >= 0 {
		return yaw + (minRad - d)
	}
	return yaw - (minRad + d)
}

// approachAngle turns cur toward target along the shortest arc, moving at most
// maxStep radians, and returns the new angle.
func approachAngle(cur, target, maxStep float64) float64 {
	diff := math.Mod(target-cur, 2*math.Pi)
	if diff > math.Pi {
		diff -= 2 * math.Pi
	} else if diff < -math.Pi {
		diff += 2 * math.Pi
	}
	if diff > maxStep {
		diff = maxStep
	} else if diff < -maxStep {
		diff = -maxStep
	}
	return cur + diff
}

// standeeSurface is one renderable layer of the token slab.
type standeeSurface struct {
	p0x, p0y, dx, dy float64
	img              *ebiten.Image
	mirrored         bool    // sample 1-u instead of u
	shade            float32 // multiplied into the color scale
}

// standeeSlab is a prepared token slab: the surface stack plus the billboard
// metrics the per-column draw needs. Built once per yaw by prepareStandeeSlab,
// then drawn (optionally column-clipped) by drawStandeeSlabColumns - a crossed
// tree reuses one slab across its two arms instead of re-preparing per arm.
type standeeSlab struct {
	surfaces     []standeeSurface
	firstSurface int // draw from here (face-on fast path collapses to the near sticker)
	minX, maxX   int // unclipped screen span
	centerSize   int
	centerDepth  float64
	bottomY      int
	rr, gg, bb   float32
}

// treeArm is one center->corner half of a crossed-tree diagonal standee: the
// slab it belongs to, its screen-column span, and its midpoint depth (the
// far->near sort key for exact painter order across the four arms).
type treeArm struct {
	slabIdx int
	lo, hi  int
	depth   float64
}

// drawStandeeSprite draws the sprite as a thick wooden token of world yaw `yaw`
// (the slab's long direction). centerDepth/centerSize/bottomY come from the
// entity's billboard metrics so the token matches the billboard's on-screen
// size at face-on and keeps its feet on the same floor anchor. rr/gg/bb is the
// pre-computed color scale (brightness + hit flash). coreKey identifies the
// sprite frame for the wood-silhouette cache.
//
// Mirroring: with mirrorBySide=true the sampling flips with the viewing side -
// both faces show the same image (static scenery/NPC tokens). With
// mirrorBySide=false the caller controls the flip via mirroredIn - used by
// monsters, whose directional walk art must face the world heading regardless
// of which side the camera is on.
//
// Returns false when the token can't be built this frame so the caller falls
// back to the billboard.
// worldLengthOverride (>0) forces the token's world footprint length instead of
// deriving it from the billboard width. Crossed trees use it for their full-tile
// footprint so their arms land on the tile edges.
func (r *Renderer) drawStandeeSprite(screen *ebiten.Image, sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw float64, centerDepth float64, centerSize int, bottomY int, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64) bool {
	if sprite == nil || centerDepth <= 0 || centerSize <= 0 {
		return false
	}
	slab, ok := r.prepareStandeeSlab(sprite, coreKey, entX, entY, yaw, centerDepth, centerSize, bottomY, rr, gg, bb, mirrorBySide, mirroredIn, worldLengthOverride, r.standeeSurfaces[:0])
	if ok {
		r.drawStandeeSlabColumns(screen, slab, -1, -1)
	}
	r.standeeSurfaces = slab.surfaces[:0] // reclaim the backing array (cap grows to the max needed)
	return true
}

// standeeShellCount is the number of core silhouette layers a slab needs at
// the given half-thickness/depth so shells never sit more than ~1.5 screen
// pixels apart (the die-cut wood rim reads solid, not banded). Pure geometry -
// extracted so a perf diagnostic can compute the SAME per-tree cost the
// renderer will pay without touching any texture (see
// debug_standee_cost_sim_test.go).
func standeeShellCount(halfThicknessWorld float64, screenW int, halfFovTan, centerDepth float64) int {
	thicknessPx := 2 * halfThicknessWorld * float64(screenW) / (2 * halfFovTan * centerDepth)
	shells := int(thicknessPx/1.5) + 1
	if shells < 2 {
		shells = 2
	}
	if shells > standeeMaxShells {
		shells = standeeMaxShells
	}
	return shells
}

// prepareStandeeSlab builds a token slab for one yaw into dst (a reused
// surface buffer): the far/near stickers with the wood shells between them, plus
// the unclipped screen span and the billboard metrics. ok=false means the slab
// projects fully off-screen (nothing to draw). The returned slab's `surfaces`
// aliases dst (grown), so the caller reclaims it after drawing. See
// drawStandeeSprite's doc for the parameter meanings.
func (r *Renderer) prepareStandeeSlab(sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw, centerDepth float64, centerSize, bottomY int, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64, dst []standeeSurface) (standeeSlab, bool) {
	screenW := r.game.config.GetScreenWidth()
	cam := r.game.camera
	halfFovTan := math.Tan(cam.FOV / 2)

	// World length of the token chosen so that, seen face-on at the entity's
	// current depth, it spans exactly the billboard's pixel width.
	length := r.spriteFootprintWorld(float64(centerSize), centerDepth)
	if worldLengthOverride > 0 {
		length = worldLengthOverride
	}
	sx := math.Cos(yaw)
	sy := math.Sin(yaw)
	// Slab normal and half-thickness: the front/back faces sit at +/-h along it.
	nx, ny := -sy, sx
	h := r.game.config.Graphics.Standee.ThicknessTiles * float64(r.game.config.GetTileSize()) / 2

	// Which side faces the camera?
	camSide := 1.0
	if nx*(cam.X-entX)+ny*(cam.Y-entY) < 0 {
		camSide = -1.0
	}
	// All layers share one flag so the silhouettes stay aligned.
	mirrored := mirroredIn
	if mirrorBySide {
		// Both stickers carry the SAME image, applied outward on each face;
		// flip the sampling with the viewing side so the art never mirrors.
		mirrored = camSide < 0
	}

	core := r.standeeCoreSilhouette(coreKey, sprite)

	surface := func(offset float64, img *ebiten.Image, shade float32) standeeSurface {
		ox := entX + nx*offset
		oy := entY + ny*offset
		return standeeSurface{
			p0x: ox - sx*length/2, p0y: oy - sy*length/2,
			dx: sx * length, dy: sy * length,
			img: img, mirrored: mirrored, shade: shade,
		}
	}

	// The wood between the stickers is a real volume: a dense stack of
	// silhouette shells (shell texturing) spaced <= ~1.5 screen pixels apart, so
	// at any viewing angle the rim reads as solid die-cut wood, not a plane.
	shells := standeeShellCount(h, screenW, halfFovTan, centerDepth)
	// Painter's order per column is fixed for parallel surfaces: build far -> near.
	surfaces := dst[:0]
	surfaces = append(surfaces, surface(-h*camSide, sprite, standeeCoreShadeFar)) // far sticker (its edge sliver)
	for i := 1; i <= shells; i++ {
		f := float64(i) / float64(shells+1)
		off := camSide * (-h + 2*h*f)
		// Wood darkens toward the slab's far edge - a cheap volumetric cue.
		shade := standeeCoreShadeFar + (standeeCoreShade-standeeCoreShadeFar)*float32(f)
		surfaces = append(surfaces, surface(off, core, shade))
	}
	surfaces = append(surfaces, surface(+h*camSide, sprite, 1.0)) // near sticker

	// Screen span: union of the two outer faces' projected endpoints (the far
	// face pokes out past the near one at viewing angles). The span is purely
	// an optimization - the per-column intersections are the ground truth - so
	// never fall back to a billboard from here: a token whose slab projects
	// fully off-screen is simply not visible (its billboard, being wider than
	// the slab's edge-on projection, would otherwise flash at the screen edge),
	// and an endpoint behind the camera plane just widens the span to the full
	// screen.
	far, near := surfaces[0], surfaces[len(surfaces)-1]
	fx0, _, fok0 := r.game.renderHelper.projectToScreenX(far.p0x, far.p0y)
	fx1, _, fok1 := r.game.renderHelper.projectToScreenX(far.p0x+far.dx, far.p0y+far.dy)
	nx0, _, nok0 := r.game.renderHelper.projectToScreenX(near.p0x, near.p0y)
	nx1, _, nok1 := r.game.renderHelper.projectToScreenX(near.p0x+near.dx, near.p0y+near.dy)
	minX, maxX := screenW, -1
	anyBehind := false
	for _, e := range [4]struct {
		x  int
		ok bool
	}{{fx0, fok0}, {fx1, fok1}, {nx0, nok0}, {nx1, nok1}} {
		if !e.ok {
			anyBehind = true
			continue
		}
		if e.x < minX {
			minX = e.x
		}
		if e.x > maxX {
			maxX = e.x
		}
	}
	if anyBehind {
		minX, maxX = 0, screenW-1
	} else if maxX < 0 || minX >= screenW {
		return standeeSlab{surfaces: surfaces}, false // slab entirely off-screen
	}
	if minX < 0 {
		minX = 0
	}
	if maxX >= screenW {
		maxX = screenW - 1
	}

	// Face-on fast path: when the slab's parallax is under a pixel, the near
	// sticker covers everything (all layers share the silhouette) - draw only it,
	// skipping the wood shells and the far face.
	first := 0
	if fok0 && fok1 && nok0 && nok1 && absInt(fx0-nx0) <= 1 && absInt(fx1-nx1) <= 1 {
		first = len(surfaces) - 1
	}

	return standeeSlab{
		surfaces:     surfaces,
		firstSurface: first,
		minX:         minX,
		maxX:         maxX,
		centerSize:   centerSize,
		centerDepth:  centerDepth,
		bottomY:      bottomY,
		rr:           rr,
		gg:           gg,
		bb:           bb,
	}, true
}

// drawStandeeSlabColumns rasterizes a prepared slab, optionally narrowed to
// [clipMinX,clipMaxX] (-1 disables - crossed trees split the draw at the planes'
// crossover column so each arm draws far->near). Each surface goes out as ONE
// DrawTriangles batch: parallel planes keep a single global depth order, so
// surface-major drawing far->near is identical to per-column ordering, and it
// avoids per-column SubImage slices (thousands of wrappers/frame that broke
// batching at every column).
func (r *Renderer) drawStandeeSlabColumns(screen *ebiten.Image, slab standeeSlab, clipMinX, clipMaxX int) {
	minX, maxX := slab.minX, slab.maxX
	if clipMinX >= 0 && clipMinX > minX {
		minX = clipMinX
	}
	if clipMaxX >= 0 && clipMaxX < maxX {
		maxX = clipMaxX
	}
	if maxX < minX {
		return
	}

	screenW := r.game.config.GetScreenWidth()
	horizon := float64(r.game.config.GetScreenHeight()) / 2
	cam := r.game.camera
	halfFovTan := math.Tan(cam.FOV / 2)
	camDirX := math.Cos(cam.Angle)
	camDirY := math.Sin(cam.Angle)
	planeX := math.Cos(cam.Angle+math.Pi/2) * halfFovTan
	planeY := math.Sin(cam.Angle+math.Pi/2) * halfFovTan

	depthBuf := r.game.depthBuffer
	wallTopBuf := r.game.wallTopBuffer

	centerSize := slab.centerSize
	centerDepth := slab.centerDepth
	bottomY := slab.bottomY

	for _, sf := range slab.surfaces[slab.firstSurface:] {
		if sf.img == nil {
			continue
		}
		bounds := sf.img.Bounds()
		texW := float64(bounds.Dx())
		texH := float64(bounds.Dy())
		if texW <= 0 || texH <= 0 {
			continue
		}
		cr := slab.rr * sf.shade
		cg := slab.gg * sf.shade
		cb := slab.bb * sf.shade
		verts := r.standeeVerts[:0]
		idx := r.standeeIdx[:0]
		for x := minX; x <= maxX; x++ {
			s := 2*(float64(x)+0.5)/float64(screenW) - 1
			rx := camDirX + planeX*s
			ry := camDirY + planeY*s
			t, u, ok := standeeColumnHit(cam.X, cam.Y, rx, ry, sf.p0x, sf.p0y, sf.dx, sf.dy)
			if !ok {
				continue
			}
			// Billboard metrics scale linearly in 1/depth: reuse the center
			// anchor and size so the feet stay on the floor across the width.
			colH := float32(float64(centerSize) * centerDepth / t)
			bottom := float32(horizon + (float64(bottomY)-horizon)*centerDepth/t)
			top := bottom - colH
			srcY0 := float32(bounds.Min.Y)
			srcY1 := float32(bounds.Min.Y) + float32(texH)
			drawBottom, srcYbot := bottom, srcY1
			if x < len(depthBuf) && t-r.standeeDepthBias >= depthBuf[x] {
				// Behind a wall: only the slice rising ABOVE the wall's top edge is
				// visible, so a short wall can't occlude a tall tree's canopy. Clip
				// the column's bottom to the wall top (1D depth alone would cull the
				// whole column, hiding everything above the wall too).
				wt := float32(wallTopBuf[x])
				if wt <= top {
					continue // wall covers this entire slice
				}
				if wt < drawBottom {
					srcYbot = srcY0 + (srcY1-srcY0)*((wt-top)/colH)
					drawBottom = wt
				}
			}
			texU := u
			if sf.mirrored {
				texU = 1 - u
			}
			srcX := float32(bounds.Min.X) + float32(math.Min(texU*texW, texW-1)) + 0.5
			x0, x1 := float32(x), float32(x+1)
			base := uint16(len(verts))
			verts = append(verts,
				ebiten.Vertex{DstX: x0, DstY: top, SrcX: srcX, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x1, DstY: top, SrcX: srcX, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x0, DstY: drawBottom, SrcX: srcX, SrcY: srcYbot, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x1, DstY: drawBottom, SrcX: srcX, SrcY: srcYbot, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
			)
			idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
		}
		if len(idx) > 0 {
			// Filtering by actual scale, like the billboard path: linear
			// (mipmapped) only when the token renders SMALLER than its texture -
			// distant tokens dissolved into nearest-sample noise. Up close the
			// columns are magnified and linear would smear the pixel art, so
			// they stay nearest. Column slices sample texel centers, so level-0
			// linear is bleed-free.
			filter := ebiten.FilterNearest
			if float64(centerSize) < texH*0.5 {
				filter = ebiten.FilterLinear
			}
			screen.DrawTriangles(verts, idx, sf.img, &ebiten.DrawTrianglesOptions{
				Blend:  ebiten.BlendSourceOver,
				Filter: filter,
			})
			r.statStandeeCalls++
		}
		r.standeeVerts = verts[:0]
		r.standeeIdx = idx[:0]
	}
}

// drawCrossedTreeStandees renders a tree tile as two normal standees crossed
// along the tile's DIAGONALS (an "X" from above, corner to corner), with the
// usual standee thickness. Both are two-sided and share the tile's billboard
// metrics (depth/size/floor anchor), so they stay grounded. The texture is the
// TILE's own configured sprite (data-driven), so each tree tile (forest oak,
// ancient tree, ...) keeps its own art.
// treeIsBillboardLOD reports whether a tree at this distance collapses to a
// single camera-facing plane instead of the crossed pair. Extracted (see
// standeeShellCount) so the perf diagnostic test applies the exact same
// distance cutoff the renderer does.
func treeIsBillboardLOD(distance, tileSize, lodTiles float64) bool {
	return lodTiles > 0 && distance > lodTiles*tileSize
}

func (r *Renderer) drawCrossedTreeStandees(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	spriteName := "tree"
	if world.GlobalTileManager != nil {
		if n := world.GlobalTileManager.GetSprite(s.tileType); n != "" {
			spriteName = n
		}
	}
	sprite := r.game.sprites.GetSprite(spriteName)
	if sprite == nil || s.spriteSize <= 0 || s.depthPerp <= 0 {
		return
	}
	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	distance := math.Sqrt(math.Pow(worldX-r.game.camera.X, 2) + math.Pow(worldY-r.game.camera.Y, 2))
	b := float32(r.applyTreeDepthShading(r.calculateBrightnessWithTorchLight(worldX, worldY, distance), distance))

	// HEIGHT scales by the sprite aspect (the platan, 1:2, is twice as tall as
	// the square oak); floor anchor unchanged so feet stay grounded.
	heightPx := s.spriteSize
	if texW := float64(sprite.Bounds().Dx()); texW > 0 {
		heightPx = int(float64(s.spriteSize) * float64(sprite.Bounds().Dy()) / texW)
	}
	bottomY := s.screenY + s.spriteSize
	key := standeeCoreKey{name: "tree:" + spriteName, bounds: sprite.Bounds(), img: sprite}

	const yawA, yawB = math.Pi / 4, 3 * math.Pi / 4
	// Footprint = the art's OWN projected width, like landmark monuments - a
	// tile-diagonal footprint squeezed the art horizontally (square oak drew
	// ~30% too thin once the square-projection FOV removed the old horizontal
	// stretch that was masking it).
	footprint := r.spriteFootprintWorld(float64(s.spriteSize), s.depthPerp)

	// Distance LOD (trees only): beyond the threshold the crossed pair's parallax
	// is sub-pixel, so collapse to a SINGLE camera-facing plane - ~4x fewer draws,
	// full silhouette from any angle. Static (no easing): trees don't sway, they
	// just present face-on to the camera this frame.
	if treeIsBillboardLOD(distance, tileSize, r.game.config.Graphics.TreeStandeeLODTiles) {
		faceYaw := math.Atan2(r.game.camera.Y-worldY, r.game.camera.X-worldX) + math.Pi/2
		r.drawStandeeSprite(screen, sprite, key, worldX, worldY, faceYaw, s.depthPerp, heightPx, bottomY, b, b, b, true, false, footprint)
		return
	}

	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, yawA, yawB, footprint, s.depthPerp, heightPx, bottomY, b)
}

// drawCrossedSlabs renders two perpendicular standee planes (yawA, yawB) crossing
// at (worldX,worldY) over a square footprint, split into four center->corner ARMS
// drawn far->near for exact occlusion. Shared by static crossed trees (diagonal
// yaws) and spinning landmark monuments (rotating yaws).
//
// The arms are disjoint in 3D (they meet only on the central axis), so any two
// project to screen segments that share only the center column - they can't swap
// depth order across the columns they overlap in. A far->near painter's order over
// the four arms is therefore exact: a nearer arm's opaque pixels occlude a farther
// arm and its transparent pixels reveal it, with no slab interpenetration even
// when one plane is edge-on. (Ordering the two whole planes can't: near the axis
// the slabs interleave and whichever draws last wins - the see-through artifact.)
// Each arm draws the WHOLE slab clipped to its columns, so the texture stays
// continuous and batched. Both arms of a plane share one slab, so each yaw's slab
// is prepared ONCE and reused; the two slabs stay live together for the
// interleaved draw, hence two reused buffers (A/B).
func (r *Renderer) drawCrossedSlabs(screen, sprite *ebiten.Image, key standeeCoreKey, worldX, worldY, yawA, yawB, footprint, depthPerp float64, heightPx, bottomY int, b float32) {
	slabs := [2]standeeSlab{}
	slabOK := [2]bool{}
	slabs[0], slabOK[0] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawA, depthPerp, heightPx, bottomY, b, b, b, true, false, footprint, r.standeeSurfaces[:0])
	slabs[1], slabOK[1] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawB, depthPerp, heightPx, bottomY, b, b, b, true, false, footprint, r.standeeSurfacesB[:0])

	cam := r.game.camera
	screenW := r.game.config.GetScreenWidth()
	xc, _, okc := r.game.renderHelper.projectToScreenX(worldX, worldY)
	clampCol := func(x int) int {
		if x < 0 {
			return 0
		}
		if x >= screenW {
			return screenW - 1
		}
		return x
	}
	arms := r.treeArms[:0]
	allOK := okc
	for si, yaw := range [2]float64{yawA, yawB} {
		dx, dy := math.Cos(yaw), math.Sin(yaw)
		for _, side := range [2]float64{+1, -1} {
			cornerX := worldX + dx*side*footprint/2
			cornerY := worldY + dy*side*footprint/2
			cc, _, okCorner := r.game.renderHelper.projectToScreenX(cornerX, cornerY)
			if !okCorner {
				allOK = false
			}
			lo, hi := xc, cc
			if lo > hi {
				lo, hi = hi, lo
			}
			arms = append(arms, treeArm{
				slabIdx: si,
				lo:      clampCol(lo),
				hi:      clampCol(hi),
				depth:   math.Hypot(worldX+dx*side*footprint/4-cam.X, worldY+dy*side*footprint/4-cam.Y),
			})
		}
	}
	if !allOK {
		// Camera atop the tile: center/corners fall behind the view plane and the
		// arm spans are meaningless. Best-effort whole-plane far->near.
		if slabOK[0] {
			r.drawStandeeSlabColumns(screen, slabs[0], -1, -1)
		}
		if slabOK[1] {
			r.drawStandeeSlabColumns(screen, slabs[1], -1, -1)
		}
	} else {
		sort.Slice(arms, func(i, j int) bool { return arms[i].depth > arms[j].depth }) // far -> near
		for _, a := range arms {
			if slabOK[a.slabIdx] {
				r.drawStandeeSlabColumns(screen, slabs[a.slabIdx], a.lo, a.hi)
			}
		}
	}
	r.treeArms = arms[:0]
	r.standeeSurfaces = slabs[0].surfaces[:0] // reclaim backing arrays (caps grow)
	r.standeeSurfacesB = slabs[1].surfaces[:0]
}

// drawWallStandee draws a token flush to a wall (pose from wallStickPose),
// applying a backing-wall depth bias so the sprite-vs-wall test doesn't reject
// it. depthBiasTiles is that allowance in TILES: how far behind a wall's front
// face a column may sit and still draw. COPLANAR decor (pictures, levers - the
// slab lies ON the wall face, and at corners a perpendicular wall sliver can
// front-run it) needs a generous 0.6; a PERPENDICULAR door slab only meets its
// flanking walls at the seam columns, so it takes a small epsilon - a larger
// one lets the door's edge paint over the flanking wall at oblique angles.
// bottomY sets the vertical anchor: floor-anchored for full NPC gates,
// mid-wall for shrunk decoration tiles. worldLength > 0 forces the slab's world
// span (doors must bridge their opening exactly - the projected billboard width
// rounds short and leaves cracks against the flanking walls); 0 keeps the
// projected width. Single source for both wall-mount draw sites (NPC +
// wall_prop tile). Returns drawStandeeSprite's drawn flag.
func (r *Renderer) drawWallStandee(screen *ebiten.Image, sprite *ebiten.Image, key standeeCoreKey, wx, wy, wyaw, depthPerp float64, spriteSize, bottomY int, br float32, worldLength, depthBiasTiles float64) bool {
	r.standeeDepthBias = float64(r.game.config.GetTileSize()) * depthBiasTiles
	drew := r.drawStandeeSprite(screen, sprite, key, wx, wy, wyaw, depthPerp, spriteSize, bottomY, br, br, br, true, false, worldLength)
	r.standeeDepthBias = 0
	return drew
}

// Depth-bias tiers for drawWallStandee (see its doc).
const (
	wallDecorDepthBiasTiles = 0.6  // coplanar wall decor
	doorDepthBiasTiles      = 0.08 // perpendicular door slab: seam epsilon only
)

// wallStickPose returns the render position + slab yaw for a wall-mounted standee:
// it slides from the tile centre toward the nearest SOLID (wall) orthogonal
// neighbour and orients the slab ALONG that wall, so the token sits flush on the
// wall face instead of floating mid-tile. Neighbours are checked N,E,S,W (fixed
// priority so a corner picks deterministically). ok=false when none is solid -
// the caller then draws the normal centred standee.
func (g *MMGame) wallStickPose(npcX, npcY float64) (x, y, yaw float64, ok bool) {
	w := g.GetCurrentWorld()
	if w == nil || world.GlobalTileManager == nil {
		return 0, 0, 0, false
	}
	ts := float64(g.config.GetTileSize())
	cx := (math.Floor(npcX/ts) + 0.5) * ts
	cy := (math.Floor(npcY/ts) + 0.5) * ts
	const off = 0.5 // flush against the wall face (the player's collision stops them short, so they never pass the plane)
	// dx,dy = neighbour direction; yaw = slab long axis (runs ALONG the wall).
	for _, d := range [4]struct {
		dx, dy, yaw float64
	}{
		{0, -1, 0},           // wall north  -> slab runs E-W
		{1, 0, math.Pi / 2},  // wall east   -> slab runs N-S
		{0, 1, 0},            // wall south  -> slab runs E-W
		{-1, 0, math.Pi / 2}, // wall west   -> slab runs N-S
	} {
		if world.GlobalTileManager.IsSolid(w.GetTileAt(cx+d.dx*ts, cy+d.dy*ts)) {
			return cx + d.dx*ts*off, cy + d.dy*ts*off, d.yaw, true
		}
	}
	return 0, 0, 0, false
}

// spriteFootprintWorld returns the world length that spans spriteSizePx
// face-on at depthPerp - THE px->world projection (slab lengths, hit-shake
// amplitudes). For slabs it is the length that shows the art at its own
// proportions (the texture maps u in [0,1] across it centered on the entity,
// so a cross's intersection axis lands on the texture centre). Falls back to
// the tile diagonal when the projection degenerates.
func (r *Renderer) spriteFootprintWorld(spriteSizePx, depthPerp float64) float64 {
	halfFovTan := math.Tan(r.game.camera.FOV / 2)
	footprint := spriteSizePx * 2 * halfFovTan * depthPerp / float64(r.game.config.GetScreenWidth())
	if footprint <= 0 {
		footprint = float64(r.game.config.GetTileSize()) * math.Sqrt2
	}
	return footprint
}

// drawLandmarkStandee renders a render_type:"landmark" entity (mage tower, church,
// city gate, lich nexus, fountain) as a TALL crossed standee that slowly spins in
// place - the static-token showcase spin, but as a perpendicular cross so it reads
// as a 3D monument from any angle. spinYaw is the showcase yaw the caller already
// computes (so it matches the single-standee spin); the second plane is spinYaw+90deg.
// Height scales by sprite aspect (tall art -> tall monument), feet stay anchored.
func (r *Renderer) drawLandmarkStandee(screen, sprite *ebiten.Image, keyName string, worldX, worldY, spinYaw, depthPerp float64, spriteSize, screenY int, b float32) bool {
	if sprite == nil || spriteSize <= 0 || depthPerp <= 0 {
		return false
	}
	heightPx := spriteSize
	if texW := float64(sprite.Bounds().Dx()); texW > 0 {
		heightPx = int(float64(spriteSize) * float64(sprite.Bounds().Dy()) / texW)
	}
	key := standeeCoreKey{name: keyName, bounds: sprite.Bounds(), img: sprite}
	footprint := r.spriteFootprintWorld(float64(spriteSize), depthPerp)
	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, spinYaw, spinYaw+math.Pi/2, footprint, depthPerp, heightPx, screenY+spriteSize, b)
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
