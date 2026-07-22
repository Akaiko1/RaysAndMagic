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
	standeeMaxMipLevel  = 6             // matches Ebitengine's own mipmap depth cap
	// Blend only around the nearest-mip crossover. Outside this band one
	// bilinearly sampled level is already stable, avoiding a second four-tap
	// sample across most distant standee pixels.
	standeeMipBlendStart = 0.35
	standeeMipBlendEnd   = 0.65
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

type standeeMipLayer uint8

const (
	standeeMipSticker standeeMipLayer = iota
	standeeMipCore
)

// standeeMipKey extends the existing stable frame identity with the physical
// token layer: sticker art and its generated wooden core have different pixels
// even though they share the same source-frame key.
type standeeMipKey struct {
	frame standeeCoreKey
	layer standeeMipLayer
}

// standeeMipChain owns normalized immutable copies of one standee texture.
// Ebitengine exposes no way to address its internal mip levels, so adjacent
// levels must be explicit images for true trilinear filtering.
type standeeMipChain struct {
	levels []*ebiten.Image
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

// standeeColumnIntersection intersects one screen ray (origin cam, direction
// R, both in world space) with the infinite line P0 + u*(P1-P0). It preserves
// u outside [0,1] so the rasterizer can map the two SCREEN-PIXEL BOUNDARIES to
// two distinct texture coordinates at a segment edge. That non-zero source
// footprint is what lets the standee minification shader filter the actual
// texel area covered by this pixel instead of sampling a phase-dependent point.
//
// t is the perpendicular depth when R is built as dir + plane*s with |dir|=1.
func standeeColumnIntersection(camX, camY, rx, ry, p0x, p0y, dx, dy float64) (t, u float64, ok bool) {
	det := dx*ry - dy*rx
	if math.Abs(det) < 1e-9 {
		return 0, 0, false // ray parallel to the token plane (edge-on)
	}
	ex := p0x - camX
	ey := p0y - camY
	t = (dx*ey - dy*ex) / det
	u = (rx*ey - ry*ex) / det
	if t < standeeMinDepth {
		return 0, 0, false
	}
	return t, u, true
}

// standeeColumnHit is standeeColumnIntersection clipped to the actual finite
// surface segment. Use it for the pixel-centre visibility decision; the draw
// path uses the unbounded helper only for that visible column's two edges.
func standeeColumnHit(camX, camY, rx, ry, p0x, p0y, dx, dy float64) (t, u float64, ok bool) {
	t, u, ok = standeeColumnIntersection(camX, camY, rx, ry, p0x, p0y, dx, dy)
	if !ok || u < 0 || u > 1 {
		return 0, 0, false
	}
	return t, u, true
}

// standeeRayAtScreenX returns the camera-plane ray through a screen coordinate.
// Both the standee rasterizer and wall-mounted interaction occlusion use this
// exact convention, so their backing-wall plane tests cannot diverge.
func standeeRayAtScreenX(screenX float64, screenWidth int, dirX, dirY, planeX, planeY float64) (float64, float64) {
	s := 2*screenX/float64(screenWidth) - 1
	return dirX + planeX*s, dirY + planeY*s
}

// standeeUsesMinificationSampling selects filtered sampling as soon as
// either source axis shrinks. Keeping nearest until half size made the 50%-100%
// band temporally alias: moving a few pixels switched the sampled texel phase
// between sharp and mushy.
//
// A standee can shrink in either direction: a face viewed almost edge-on is
// horizontally minified even when it is still tall on screen.
func standeeUsesMinificationSampling(projectedWidth, projectedHeight, textureWidth, textureHeight float64) bool {
	return projectedWidth < textureWidth || projectedHeight < textureHeight
}

// standeeTextureFootprint is the source texel span covered by one destination
// pixel along one axis. A value below one is clamped because it represents
// magnification, not a mip level.
func standeeTextureFootprint(sourceSpan, destinationSpan float32) float32 {
	if math.Abs(float64(destinationSpan)) < 1e-6 {
		return 1
	}
	footprint := float32(math.Abs(float64(sourceSpan / destinationSpan)))
	if footprint < 1 {
		return 1
	}
	return footprint
}

// standeeMipBlend selects the nearest mip level with a short trilinear crossover.
// The crossover is continuous: its upper endpoint is exactly the next level,
// which is also the pure image selected immediately after the band. Keeping the
// blend narrower than the full octave avoids paying eight texture taps where a
// single stable level is visually indistinguishable.
func standeeMipBlend(footprint float32, maxLevel int) (level int, blend float32) {
	if footprint <= 1 || maxLevel <= 0 || math.IsNaN(float64(footprint)) {
		return 0, 0
	}
	lod := math.Log2(float64(footprint))
	level = int(math.Floor(lod))
	if level < 0 {
		return 0, 0
	}
	if level >= maxLevel {
		return maxLevel, 0
	}
	fraction := lod - float64(level)
	if fraction <= standeeMipBlendStart {
		return level, 0
	}
	if fraction >= standeeMipBlendEnd {
		return level + 1, 0
	}
	return level, float32((fraction - standeeMipBlendStart) / (standeeMipBlendEnd - standeeMipBlendStart))
}

func standeeMipSizes(width, height int) []image.Point {
	if width <= 0 || height <= 0 {
		return nil
	}
	sizes := make([]image.Point, 0, standeeMaxMipLevel+1)
	for level := 0; level <= standeeMaxMipLevel; level++ {
		sizes = append(sizes, image.Pt(width, height))
		if width == 1 && height == 1 {
			break
		}
		if width > 1 {
			width /= 2
		}
		if height > 1 {
			height /= 2
		}
	}
	return sizes
}

// standeeMipChainFor builds each level by one 2x linear reduction, exactly the
// strategy used by Ebitengine's internal mipmap package. Level 0 is normalized
// to (0,0,w,h), which also gives recreated NPC SubImages a stable cached image.
func (r *Renderer) standeeMipChainFor(key standeeMipKey, src *ebiten.Image) *standeeMipChain {
	if chain := r.standeeMipCache[key]; chain != nil {
		return chain
	}
	sizes := standeeMipSizes(src.Bounds().Dx(), src.Bounds().Dy())
	if len(sizes) == 0 {
		return nil
	}
	chain := &standeeMipChain{levels: make([]*ebiten.Image, 0, len(sizes))}
	base := src
	if src.Bounds().Min != (image.Point{}) {
		// The shader's full-size coordinate reference is normalized. Most sprite
		// images already start at (0,0) and can be reused directly; only sheet
		// SubImages need this copy. Avoiding a duplicate level 0 cuts the mip
		// cache's dominant allocation without changing immutable source pixels.
		base = ebiten.NewImage(sizes[0].X, sizes[0].Y)
		base.DrawImage(src, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
	}
	chain.levels = append(chain.levels, base)
	previous := base
	for _, size := range sizes[1:] {
		level := ebiten.NewImage(size.X, size.Y)
		previousSize := previous.Bounds().Size()
		opts := &ebiten.DrawImageOptions{
			Blend:          ebiten.BlendCopy,
			Filter:         ebiten.FilterLinear,
			DisableMipmaps: true,
		}
		opts.GeoM.Scale(float64(size.X)/float64(previousSize.X), float64(size.Y)/float64(previousSize.Y))
		level.DrawImage(previous, opts)
		chain.levels = append(chain.levels, level)
		previous = level
	}
	if r.standeeMipCache == nil {
		r.standeeMipCache = make(map[standeeMipKey]*standeeMipChain)
	}
	r.standeeMipCache[key] = chain
	return chain
}

// Ebitengine chooses one integer mip level for an entire DrawTriangles call.
// That is correct spatial filtering, but a moving token visibly jumps between
// sharp and soft levels because the engine does not trilinearly blend them.
// Images 1 and 2 are adjacent explicit levels; image 0 is the full-size
// coordinate reference. In pixel mode Kage adjusts only atlas origins for
// imageSrcNAt, so these helpers explicitly map full-size coordinates into each
// level before performing bilinear sampling.
const standeeTrilinearShaderSrc = `//kage:unit pixels

package main

func sampleLinear1(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc1Size()
	scale := size / size0
	// In pixel mode imageSrc1At preserves image-0 pixel coordinates and only
	// adjusts the backing-atlas origin; it does not scale differently-sized
	// source images. Work in level-local pixels, then pass those coordinates
	// relative to image 0 so the built-in origin adjustment remains correct.
	q := clamp((p-origin0)*scale, vec2(0.5), size-vec2(0.5))
	q0 := q - 0.5
	q1 := q + 0.5
	p0 := q0 + origin0
	p1 := q1 + origin0
	c0 := imageSrc1At(p0)
	c1 := imageSrc1At(vec2(p1.x, p0.y))
	c2 := imageSrc1At(vec2(p0.x, p1.y))
	c3 := imageSrc1At(p1)
	rate := fract(q1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}

func sampleLinear2(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc2Size()
	scale := size / size0
	q := clamp((p-origin0)*scale, vec2(0.5), size-vec2(0.5))
	q0 := q - 0.5
	q1 := q + 0.5
	p0 := q0 + origin0
	p1 := q1 + origin0
	c0 := imageSrc2At(p0)
	c1 := imageSrc2At(vec2(p1.x, p0.y))
	c2 := imageSrc2At(vec2(p0.x, p1.y))
	c3 := imageSrc2At(p1)
	rate := fract(q1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	lo := sampleLinear1(srcPos)
	if custom.x <= 0.0 {
		return lo * color
	}
	return mix(lo, sampleLinear2(srcPos), clamp(custom.x, 0, 1)) * color
}
`

func (r *Renderer) ensureStandeeTrilinearShader() (*ebiten.Shader, error) {
	if r.standeeTrilinearShader != nil {
		return r.standeeTrilinearShader, nil
	}
	shader, err := ebiten.NewShader([]byte(standeeTrilinearShaderSrc))
	if err != nil {
		return nil, err
	}
	r.standeeTrilinearShader = shader
	return shader, nil
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
	mipKey           standeeMipKey
	mirrored         bool    // sample 1-u instead of u
	shade            float32 // multiplied into the color scale
}

// standeeWallOcclusion describes the exceptional wall relationship for the
// current standee draw. A mounted token may draw through precisely its backing
// wall plane, not through every wall inside an arbitrary depth band.
type standeeWallOcclusion struct {
	depthAllowance                 float64
	backingX, backingY, backingYaw float64
	hasBackingWall                 bool
}

// matchesBackingWall reports whether wallDepth comes from the mounted token's
// actual backing plane for this screen ray. Depth-buffer values are generated
// from the same camera-plane ray at the shipped one-ray-per-pixel setting, so
// a tiny numeric epsilon is enough; the small depthAllowance handles coarser
// diagnostic ray widths.
func (o standeeWallOcclusion) matchesBackingWall(camX, camY, rayX, rayY, wallDepth float64) bool {
	if !o.hasBackingWall {
		return false
	}
	normalX, normalY := -math.Sin(o.backingYaw), math.Cos(o.backingYaw)
	denom := normalX*rayX + normalY*rayY
	if math.Abs(denom) < 1e-9 {
		return false
	}
	backingDepth := (normalX*(o.backingX-camX) + normalY*(o.backingY-camY)) / denom
	return backingDepth >= standeeMinDepth && math.Abs(wallDepth-backingDepth) <= 0.01
}

// standeeSurfaceProjectedSize reports the surface's actual projected extent.
// The texture filter must follow this geometry rather than the face-on
// billboard size: a rotated scenery token can become only a few pixels wide
// while remaining hundreds of pixels tall.
func (r *Renderer) standeeSurfaceProjectedSize(sf standeeSurface, centerDepth, centerSize float64) (width, height float64) {
	width, height = centerSize, centerSize
	x0, d0, ok0 := r.game.renderHelper.projectToScreenXF(sf.p0x, sf.p0y)
	x1, d1, ok1 := r.game.renderHelper.projectToScreenXF(sf.p0x+sf.dx, sf.p0y+sf.dy)
	if !ok0 || !ok1 {
		return width, height
	}

	width = math.Abs(x1 - x0)
	if d0 > standeeMinDepth && d1 > standeeMinDepth {
		h0 := centerSize * centerDepth / d0
		h1 := centerSize * centerDepth / d1
		height = math.Min(h0, h1)
	}
	return width, height
}

// standeeColumnOccluded applies the numeric wall-depth fallback shared by all
// standees. Mounted tokens additionally recognize their exact backing plane;
// any other foreground wall still has to beat only this small allowance.
func standeeColumnOccluded(tokenDepth, wallDepth, wallAllowance float64) bool {
	return tokenDepth-wallAllowance >= wallDepth
}

// standeeSlab is a prepared token slab: the surface stack plus the billboard
// metrics the per-column draw needs. Built once per yaw by prepareStandeeSlab,
// then drawn (optionally column-clipped) by drawStandeeSlabColumns - a crossed
// tree reuses one slab across its two arms instead of re-preparing per arm.
type standeeSlab struct {
	surfaces     []standeeSurface
	firstSurface int // draw from here (face-on fast path collapses to the near sticker)
	minX, maxX   int // unclipped screen span
	// Float billboard metrics: the per-column scaling multiplies them by
	// centerDepth/t, so any int quantization here would make the whole token
	// hop a pixel at a time as the camera moves (visible at range).
	centerSize  float64
	centerDepth float64
	bottomY     float64
	rr, gg, bb  float32
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
func (r *Renderer) drawStandeeSprite(screen *ebiten.Image, sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw float64, centerDepth float64, centerSize float64, bottomY float64, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64) bool {
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
func (r *Renderer) prepareStandeeSlab(sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw, centerDepth float64, centerSize, bottomY float64, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64, dst []standeeSurface) (standeeSlab, bool) {
	screenW := r.game.config.GetScreenWidth()
	cam := r.game.camera
	halfFovTan := math.Tan(cam.FOV / 2)

	// World length of the token chosen so that, seen face-on at the entity's
	// current depth, it spans exactly the billboard's pixel width.
	length := r.spriteFootprintWorld(centerSize, centerDepth)
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

	surface := func(offset float64, img *ebiten.Image, mipLayer standeeMipLayer, shade float32) standeeSurface {
		ox := entX + nx*offset
		oy := entY + ny*offset
		return standeeSurface{
			p0x: ox - sx*length/2, p0y: oy - sy*length/2,
			dx: sx * length, dy: sy * length,
			img: img, mipKey: standeeMipKey{frame: coreKey, layer: mipLayer},
			mirrored: mirrored, shade: shade,
		}
	}

	// The wood between the stickers is a real volume: a dense stack of
	// silhouette shells (shell texturing) spaced <= ~1.5 screen pixels apart, so
	// at any viewing angle the rim reads as solid die-cut wood, not a plane.
	shells := standeeShellCount(h, screenW, halfFovTan, centerDepth)
	// Painter's order per column is fixed for parallel surfaces: build far -> near.
	surfaces := dst[:0]
	surfaces = append(surfaces, surface(-h*camSide, sprite, standeeMipSticker, standeeCoreShadeFar)) // far sticker (its edge sliver)
	for i := 1; i <= shells; i++ {
		f := float64(i) / float64(shells+1)
		off := camSide * (-h + 2*h*f)
		// Wood darkens toward the slab's far edge - a cheap volumetric cue.
		shade := standeeCoreShadeFar + (standeeCoreShade-standeeCoreShadeFar)*float32(f)
		surfaces = append(surfaces, surface(off, core, standeeMipCore, shade))
	}
	surfaces = append(surfaces, surface(+h*camSide, sprite, standeeMipSticker, 1.0)) // near sticker

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
	occlusion := r.standeeWallOcclusion

	centerSize := slab.centerSize
	centerDepth := slab.centerDepth
	bottomY := slab.bottomY
	// All slab surfaces are parallel and separated only by its tiny physical
	// thickness. Measure the outer faces once and use the smaller projection as
	// the conservative mip footprint for every sticker/core layer; doing the
	// same two projections for every wood shell would add work without changing
	// the chosen sampling level.
	projectedWidth, projectedHeight := r.standeeSurfaceProjectedSize(slab.surfaces[slab.firstSurface], centerDepth, centerSize)
	if slab.firstSurface == 0 && len(slab.surfaces) > 1 {
		otherWidth, otherHeight := r.standeeSurfaceProjectedSize(slab.surfaces[len(slab.surfaces)-1], centerDepth, centerSize)
		projectedWidth = math.Min(projectedWidth, otherWidth)
		projectedHeight = math.Min(projectedHeight, otherHeight)
	}

	for surfaceIndex := slab.firstSurface; surfaceIndex < len(slab.surfaces); surfaceIndex++ {
		sf := slab.surfaces[surfaceIndex]
		if sf.img == nil {
			continue
		}
		bounds := sf.img.Bounds()
		texW := float64(bounds.Dx())
		texH := float64(bounds.Dy())
		if texW <= 0 || texH <= 0 {
			continue
		}
		filtered := standeeUsesMinificationSampling(projectedWidth, projectedHeight, texW, texH)
		// Only the nearest sticker is a broad visible face. The far sticker and
		// wood shells are almost entirely covered and expose only thin rim slivers;
		// native mip filtering is sufficient there and avoids expensive two-level
		// sampling through every overdrawn core layer.
		trilinear := filtered && surfaceIndex == len(slab.surfaces)-1
		var mipChain *standeeMipChain
		if trilinear {
			mipChain = r.standeeMipChainFor(sf.mipKey, sf.img)
			if mipChain == nil {
				trilinear = false
			}
		}
		cr := slab.rr * sf.shade
		cg := slab.gg * sf.shade
		cb := slab.bb * sf.shade
		verts := r.standeeVerts[:0]
		idx := r.standeeIdx[:0]
		runMipLevel := -1
		drawRun := func() {
			if len(idx) == 0 {
				return
			}
			if trilinear {
				if shader, err := r.ensureStandeeTrilinearShader(); err == nil {
					nextLevel := runMipLevel + 1
					if nextLevel >= len(mipChain.levels) {
						nextLevel = runMipLevel
					}
					opts := &r.standeeTrilinearOpts
					opts.Blend = ebiten.BlendSourceOver
					// Image 0 defines SrcX/SrcY coordinates; the shader maps
					// them into differently sized images 1 and 2.
					opts.Images[0] = mipChain.levels[0]
					opts.Images[1] = mipChain.levels[runMipLevel]
					opts.Images[2] = mipChain.levels[nextLevel]
					opts.Images[3] = nil
					screen.DrawTrianglesShader(verts, idx, shader, opts)
				} else {
					// Tests compile the shader, but retain a valid built-in path
					// if a backend unexpectedly rejects it at runtime.
					screen.DrawTriangles(verts, idx, mipChain.levels[0], &ebiten.DrawTrianglesOptions{
						Blend:  ebiten.BlendSourceOver,
						Filter: ebiten.FilterLinear,
					})
				}
			} else {
				// Minified rim layers use Ebitengine's native mip path; 1:1 and
				// magnified art retains the authored nearest-pixel look.
				filter := ebiten.FilterNearest
				if filtered {
					filter = ebiten.FilterLinear
				}
				screen.DrawTriangles(verts, idx, sf.img, &ebiten.DrawTrianglesOptions{
					Blend:  ebiten.BlendSourceOver,
					Filter: filter,
				})
			}
			r.statStandeeCalls++
			verts = verts[:0]
			idx = idx[:0]
		}
		rayAt := func(screenX float64) (float64, float64) {
			return standeeRayAtScreenX(screenX, screenW, camDirX, camDirY, planeX, planeY)
		}
		geometryAt := func(t float64) (top, bottom, height float32) {
			height = float32(centerSize * centerDepth / t)
			bottom = float32(horizon + (bottomY-horizon)*centerDepth/t)
			return bottom - height, bottom, height
		}
		textureX := func(u float64) float32 {
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
			if sf.mirrored {
				u = 1 - u
			}
			origin := float32(bounds.Min.X)
			if trilinear {
				origin = 0 // mip chains are normalized to (0,0,w,h)
			}
			return origin + float32(math.Min(u*texW, texW-1)) + 0.5
		}
		for x := minX; x <= maxX; x++ {
			rcx, rcy := rayAt(float64(x) + 0.5)
			t, u, ok := standeeColumnHit(cam.X, cam.Y, rcx, rcy, sf.p0x, sf.p0y, sf.dx, sf.dy)
			if !ok {
				continue
			}

			top, bottom, height := geometryAt(t)
			top0, bottom0, height0 := top, bottom, height
			top1, bottom1, height1 := top, bottom, height
			u0, u1 := u, u
			if filtered {
				// Use each pixel boundary's true line intersection instead of
				// copying the centre texel to both vertices. The old zero-width
				// source mapping hid the horizontal texel footprint from the
				// mip filter, producing temporal shimmer at range.
				r0x, r0y := rayAt(float64(x))
				t0, edgeU0, ok0 := standeeColumnIntersection(cam.X, cam.Y, r0x, r0y, sf.p0x, sf.p0y, sf.dx, sf.dy)
				if !ok0 {
					t0, edgeU0 = t, u
				}
				r1x, r1y := rayAt(float64(x + 1))
				t1, edgeU1, ok1 := standeeColumnIntersection(cam.X, cam.Y, r1x, r1y, sf.p0x, sf.p0y, sf.dx, sf.dy)
				if !ok1 {
					t1, edgeU1 = t, u
				}
				top0, bottom0, height0 = geometryAt(t0)
				top1, bottom1, height1 = geometryAt(t1)
				u0, u1 = edgeU0, edgeU1
			}
			// Source coordinates name texel centres, matching textureX. This
			// keeps vertical filtering stable at the texture's top/bottom edges.
			srcOriginY := float32(bounds.Min.Y)
			if trilinear {
				srcOriginY = 0
			}
			srcY0 := srcOriginY + 0.5
			srcY1 := srcOriginY + float32(texH) - 0.5
			drawBottom0, srcYbot0 := bottom0, srcY1
			drawBottom1, srcYbot1 := bottom1, srcY1
			occluded := x < len(depthBuf) && standeeColumnOccluded(t, depthBuf[x], occlusion.depthAllowance)
			if occluded && occlusion.matchesBackingWall(cam.X, cam.Y, rcx, rcy, depthBuf[x]) {
				occluded = false
			}
			if occluded {
				// Behind a wall: only the slice rising ABOVE the wall's top edge is
				// visible, so a short wall can't occlude a tall tree's canopy. Clip
				// the column's bottom to the wall top (1D depth alone would cull the
				// whole column, hiding everything above the wall too).
				wt := float32(wallTopBuf[x])
				if wt <= top {
					continue // wall covers this entire slice
				}
				clipBottom := func(top, bottom, height float32) (float32, float32) {
					if wt <= top {
						return top, srcY0
					}
					if wt < bottom {
						return wt, srcY0 + (srcY1-srcY0)*((wt-top)/height)
					}
					return bottom, srcY1
				}
				drawBottom0, srcYbot0 = clipBottom(top0, bottom0, height0)
				drawBottom1, srcYbot1 = clipBottom(top1, bottom1, height1)
			}
			x0, x1 := float32(x), float32(x+1)
			srcX0, srcX1 := textureX(u0), textureX(u1)
			mipBlend := float32(0)
			if trilinear {
				// Match Ebitengine's conservative policy: the least-minified
				// axis controls LOD, avoiding blur on an anisotropic edge-on face.
				footprintX := standeeTextureFootprint(srcX1-srcX0, x1-x0)
				footprintY0 := standeeTextureFootprint(srcYbot0-srcY0, drawBottom0-top0)
				footprintY1 := standeeTextureFootprint(srcYbot1-srcY0, drawBottom1-top1)
				footprint := min(footprintX, footprintY0, footprintY1)
				mipLevel, blend := standeeMipBlend(footprint, len(mipChain.levels)-1)
				if runMipLevel >= 0 && mipLevel != runMipLevel {
					drawRun()
				}
				runMipLevel, mipBlend = mipLevel, blend
			}
			base := uint16(len(verts))
			verts = append(verts,
				ebiten.Vertex{DstX: x0, DstY: top0, SrcX: srcX0, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend},
				ebiten.Vertex{DstX: x1, DstY: top1, SrcX: srcX1, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend},
				ebiten.Vertex{DstX: x0, DstY: drawBottom0, SrcX: srcX0, SrcY: srcYbot0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend},
				ebiten.Vertex{DstX: x1, DstY: drawBottom1, SrcX: srcX1, SrcY: srcYbot1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend},
			)
			idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
		}
		drawRun()
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
	if sprite == nil || s.sizeF <= 0 || s.depthPerp <= 0 {
		return
	}
	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	distance := math.Sqrt(math.Pow(worldX-r.game.camera.X, 2) + math.Pow(worldY-r.game.camera.Y, 2))
	b := float32(r.applyTreeDepthShading(r.calculateBrightnessWithTorchLight(worldX, worldY, distance), distance))

	// HEIGHT scales by the sprite aspect (the platan, 1:2, is twice as tall as
	// the square oak); floor anchor unchanged so feet stay grounded.
	heightF := s.sizeF
	if texW := float64(sprite.Bounds().Dx()); texW > 0 {
		heightF = s.sizeF * float64(sprite.Bounds().Dy()) / texW
	}
	bottomF := s.bottomF
	key := standeeCoreKey{name: "tree:" + spriteName, bounds: sprite.Bounds(), img: sprite}

	const yawA, yawB = math.Pi / 4, 3 * math.Pi / 4
	// Footprint = the art's OWN projected width, like landmark monuments - a
	// tile-diagonal footprint squeezed the art horizontally (square oak drew
	// ~30% too thin once the square-projection FOV removed the old horizontal
	// stretch that was masking it).
	footprint := r.spriteFootprintWorld(s.sizeF, s.depthPerp)

	// Distance LOD (trees only): beyond the threshold the crossed pair's parallax
	// is sub-pixel, so collapse to a SINGLE camera-facing plane - ~4x fewer draws,
	// full silhouette from any angle. Static (no easing): trees don't sway, they
	// just present face-on to the camera this frame.
	if treeIsBillboardLOD(distance, tileSize, r.game.config.Graphics.TreeStandeeLODTiles) {
		faceYaw := math.Atan2(r.game.camera.Y-worldY, r.game.camera.X-worldX) + math.Pi/2
		r.drawStandeeSprite(screen, sprite, key, worldX, worldY, faceYaw, s.depthPerp, heightF, bottomF, b, b, b, true, false, footprint)
		return
	}

	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, yawA, yawB, footprint, s.depthPerp, heightF, bottomF, b)
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
func (r *Renderer) drawCrossedSlabs(screen, sprite *ebiten.Image, key standeeCoreKey, worldX, worldY, yawA, yawB, footprint, depthPerp float64, heightF, bottomF float64, b float32) {
	slabs := [2]standeeSlab{}
	slabOK := [2]bool{}
	slabs[0], slabOK[0] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawA, depthPerp, heightF, bottomF, b, b, b, true, false, footprint, r.standeeSurfaces[:0])
	slabs[1], slabOK[1] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawB, depthPerp, heightF, bottomF, b, b, b, true, false, footprint, r.standeeSurfacesB[:0])

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
// applying a backing-wall depth allowance so the sprite-vs-wall test doesn't
// reject it. When backingWall is true the renderer also recognizes the exact
// backing plane per column; depthAllowance is only a coarse-ray fallback in
// world pixels. Doors use no backing plane and their own seam allowance because
// they meet flanking walls only at their ends.
// bottomY sets the vertical anchor: floor-anchored for full NPC gates,
// mid-wall for shrunk decoration tiles. worldLength > 0 forces the slab's world
// span (doors must bridge their opening exactly - the projected billboard width
// rounds short and leaves cracks against the flanking walls); 0 keeps the
// projected width. Single source for both wall-mount draw sites (NPC +
// wall_prop tile). Returns drawStandeeSprite's drawn flag.
func (r *Renderer) drawWallStandee(screen *ebiten.Image, sprite *ebiten.Image, key standeeCoreKey, wx, wy, wyaw, depthPerp float64, spriteSize, bottomY float64, br float32, worldLength, depthAllowance float64, backingWall bool) bool {
	r.standeeWallOcclusion = standeeWallOcclusion{depthAllowance: depthAllowance}
	if backingWall {
		r.standeeWallOcclusion.backingX = wx
		r.standeeWallOcclusion.backingY = wy
		r.standeeWallOcclusion.backingYaw = wyaw
		r.standeeWallOcclusion.hasBackingWall = true
	}
	drew := r.drawStandeeSprite(screen, sprite, key, wx, wy, wyaw, depthPerp, spriteSize, bottomY, br, br, br, true, false, worldLength)
	r.standeeWallOcclusion = standeeWallOcclusion{}
	return drew
}

// wallMountedDepthAllowanceWorld is the small numeric fallback shared by
// rendering and interaction hit testing. The exact backing-wall plane is
// matched separately; the front sticker already sits half a slab thickness in
// front of it, and two world pixels cover coarse ray-column jitter without
// admitting an adjacent foreground wall.
const (
	wallMountedDepthJitterWorld = 2.0
	doorDepthBiasTiles          = 0.08 // perpendicular door slab: seam epsilon only
)

func wallMountedDepthAllowanceWorld(tileSize, thicknessTiles float64) float64 {
	return math.Max(0, thicknessTiles)*tileSize/2 + wallMountedDepthJitterWorld
}

func doorDepthAllowanceWorld(tileSize float64) float64 {
	return tileSize * doorDepthBiasTiles
}

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
func (r *Renderer) drawLandmarkStandee(screen, sprite *ebiten.Image, keyName string, worldX, worldY, spinYaw, depthPerp float64, sizeF, bottomF float64, b float32) bool {
	if sprite == nil || sizeF <= 0 || depthPerp <= 0 {
		return false
	}
	heightF := sizeF
	if texW := float64(sprite.Bounds().Dx()); texW > 0 {
		heightF = sizeF * float64(sprite.Bounds().Dy()) / texW
	}
	key := standeeCoreKey{name: keyName, bounds: sprite.Bounds(), img: sprite}
	footprint := r.spriteFootprintWorld(sizeF, depthPerp)
	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, spinYaw, spinYaw+math.Pi/2, footprint, depthPerp, heightF, bottomF, b)
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
