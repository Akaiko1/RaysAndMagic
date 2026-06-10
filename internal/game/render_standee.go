package game

import (
	"image"
	"math"

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
	standeeCoreShade    = 0.92          // wood rim sits just out of the light vs the face
	standeeCoreShadeFar = 0.75          // the slab's far edge is in its own shadow
	standeeMaxShells    = 16            // cap on wood shell layers (perf guard at point-blank range)
	standeeMinDepth     = 4.0           // near clip for token columns (world units)
	standeeStaticYaw    = math.Pi / 4.0 // fixed diagonal for scenery and NPC tokens
	standeeTurnDefault  = 270.0         // deg/sec token swivel when config omits it
)

// Wood core look: base tone with subtle horizontal grain banding, tinted
// toward the sprite's own average color so each token's edge matches its art
// (a red dragon stands on reddish wood, a slime on greenish) — like dyed
// wooden meeples.
var standeeWoodTone = [3]float64{0.62, 0.45, 0.27}

const standeeCoreTint = 0.55 // 0 = pure wood, 1 = pure sprite average color

// standeeEnvYawState is the eased facing of one scenery token (keyed by tile
// in Renderer.standeeEnvYaw): current yaw plus the frame it was last advanced,
// so a token unseen for a while snaps instead of visibly catching up.
type standeeEnvYawState struct {
	yaw  float64
	tick int64
}

// standeeCoreKey identifies one sprite frame in the wood-silhouette cache.
// NPC frames are sheet SubImages recreated every draw — their pointers are
// unstable but their absolute bounds distinguish frames. Monster animation
// frames are standalone images built once at load — their bounds are all
// (0,0,w,h) and would collide (freezing the wood on one pose), but their
// pointers are stable: include the image pointer for them.
type standeeCoreKey struct {
	name   string
	bounds image.Rectangle
	img    *ebiten.Image // only set when the caller's frame images are stable
}

// woodSilhouette returns (building and caching on first use) the sprite frame's
// silhouette filled with wood: every opaque texel becomes the wood tone with a
// faint horizontal grain, so the token's core follows the die-cut artwork shape.
func (r *Renderer) woodSilhouette(key standeeCoreKey, src *ebiten.Image) *ebiten.Image {
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
	// texels. A plain mean reads wrong — dark outlines and brown gear drown a
	// goblin's green skin — so saturated pixels dominate and near-grey ones
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
		tone[0] += (sumR/sumW - tone[0]) * standeeCoreTint
		tone[1] += (sumG/sumW - tone[1]) * standeeCoreTint
		tone[2] += (sumB/sumW - tone[2]) * standeeCoreTint
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
// Returns the ray parameter t — which IS the perpendicular depth when R is
// built as dir + plane*s with |dir| = 1 — and the segment fraction u.
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
// vanish). Slabs have period π: deviation is measured in (-π/2, π/2] and the
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

// drawStandeeSprite draws the sprite as a thick wooden token of world yaw `yaw`
// (the slab's long direction). centerDepth/centerSize/bottomY come from the
// entity's billboard metrics so the token matches the billboard's on-screen
// size at face-on and keeps its feet on the same floor anchor. rr/gg/bb is the
// pre-computed color scale (brightness + hit flash). coreKey identifies the
// sprite frame for the wood-silhouette cache.
//
// Mirroring: with mirrorBySide=true the sampling flips with the viewing side —
// both faces show the same image (static scenery/NPC tokens). With
// mirrorBySide=false the caller controls the flip via mirroredIn — used by
// monsters, whose directional walk art must face the world heading regardless
// of which side the camera is on.
//
// Returns false when the token can't be built this frame so the caller falls
// back to the billboard.
func (r *Renderer) drawStandeeSprite(screen *ebiten.Image, sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw float64, centerDepth float64, centerSize int, bottomY int, rr, gg, bb float32, mirrorBySide, mirroredIn bool) bool {
	if sprite == nil || centerDepth <= 0 || centerSize <= 0 {
		return false
	}
	screenW := r.game.config.GetScreenWidth()
	screenH := r.game.config.GetScreenHeight()
	cam := r.game.camera
	horizon := float64(screenH) / 2
	halfFovTan := math.Tan(cam.FOV / 2)

	// World length of the token chosen so that, seen face-on at the entity's
	// current depth, it spans exactly the billboard's pixel width.
	length := float64(centerSize) * 2 * halfFovTan * centerDepth / float64(screenW)
	sx := math.Cos(yaw)
	sy := math.Sin(yaw)
	// Slab normal and half-thickness: the front/back faces sit at ±h along it.
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

	core := r.woodSilhouette(coreKey, sprite)

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
	// silhouette shells (shell texturing) spaced ≤ ~1.5 screen pixels apart, so
	// at any viewing angle the rim reads as solid die-cut wood, not a plane.
	thicknessPx := 2 * h * float64(screenW) / (2 * halfFovTan * centerDepth)
	shells := int(thicknessPx/1.5) + 1
	if shells < 2 {
		shells = 2
	}
	if shells > standeeMaxShells {
		shells = standeeMaxShells
	}
	// Painter's order per column is fixed for parallel surfaces: build far → near.
	surfaces := make([]standeeSurface, 0, shells+2)
	surfaces = append(surfaces, surface(-h*camSide, sprite, standeeCoreShadeFar)) // far sticker (its edge sliver)
	for i := 1; i <= shells; i++ {
		f := float64(i) / float64(shells+1)
		off := camSide * (-h + 2*h*f)
		// Wood darkens toward the slab's far edge — a cheap volumetric cue.
		shade := standeeCoreShadeFar + (standeeCoreShade-standeeCoreShadeFar)*float32(f)
		surfaces = append(surfaces, surface(off, core, shade))
	}
	surfaces = append(surfaces, surface(+h*camSide, sprite, 1.0)) // near sticker

	// Screen span: union of the two outer faces' projected endpoints (the far
	// face pokes out past the near one at viewing angles). The span is purely
	// an optimization — the per-column intersections are the ground truth — so
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
		return true // slab entirely off-screen: nothing to draw
	}
	if minX < 0 {
		minX = 0
	}
	if maxX >= screenW {
		maxX = screenW - 1
	}

	// Face-on fast path: when the slab's parallax is under a pixel, the near
	// sticker covers everything (all layers share the silhouette) — skip the
	// wood shells and the far face entirely.
	if fok0 && fok1 && nok0 && nok1 && absInt(fx0-nx0) <= 1 && absInt(fx1-nx1) <= 1 {
		surfaces = surfaces[len(surfaces)-1:]
	}

	camDirX := math.Cos(cam.Angle)
	camDirY := math.Sin(cam.Angle)
	planeX := math.Cos(cam.Angle+math.Pi/2) * halfFovTan
	planeY := math.Sin(cam.Angle+math.Pi/2) * halfFovTan

	depthBuf := r.game.depthBuffer

	// Surface-major rendering: parallel planes keep one global depth order, so
	// drawing whole surfaces far → near is identical to per-column ordering —
	// and it lets each surface go out as ONE DrawTriangles batch (per-column
	// SubImage slices allocated thousands of wrappers per frame and broke
	// sprite/wood batching at every column).
	for _, sf := range surfaces {
		if sf.img == nil {
			continue
		}
		bounds := sf.img.Bounds()
		texW := float64(bounds.Dx())
		texH := float64(bounds.Dy())
		if texW <= 0 || texH <= 0 {
			continue
		}
		cr := rr * sf.shade
		cg := gg * sf.shade
		cb := bb * sf.shade
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
			if x < len(depthBuf) && t >= depthBuf[x] {
				continue // behind a wall
			}
			// Billboard metrics scale linearly in 1/depth: reuse the center
			// anchor and size so the feet stay on the floor across the width.
			colH := float32(float64(centerSize) * centerDepth / t)
			bottom := float32(horizon + (float64(bottomY)-horizon)*centerDepth/t)
			texU := u
			if sf.mirrored {
				texU = 1 - u
			}
			srcX := float32(bounds.Min.X) + float32(math.Min(texU*texW, texW-1)) + 0.5
			srcY0 := float32(bounds.Min.Y)
			srcY1 := float32(bounds.Min.Y) + float32(texH)
			x0, x1 := float32(x), float32(x+1)
			base := uint16(len(verts))
			verts = append(verts,
				ebiten.Vertex{DstX: x0, DstY: bottom - colH, SrcX: srcX, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x1, DstY: bottom - colH, SrcX: srcX, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x0, DstY: bottom, SrcX: srcX, SrcY: srcY1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
				ebiten.Vertex{DstX: x1, DstY: bottom, SrcX: srcX, SrcY: srcY1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1},
			)
			idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
		}
		if len(idx) > 0 {
			// Filtering by actual scale, like the billboard path: linear
			// (mipmapped) only when the token renders SMALLER than its texture —
			// distant tokens dissolved into nearest-sample noise. Up close the
			// columns are magnified and linear would smear the pixel art, so
			// they stay nearest. Column slices sample texel centers, so level-0
			// linear is bleed-free.
			filter := ebiten.FilterNearest
			if float64(centerSize) < texH {
				filter = ebiten.FilterLinear
			}
			screen.DrawTriangles(verts, idx, sf.img, &ebiten.DrawTrianglesOptions{
				Blend:  ebiten.BlendSourceOver,
				Filter: filter,
			})
		}
		r.standeeVerts = verts[:0]
		r.standeeIdx = idx[:0]
	}
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
