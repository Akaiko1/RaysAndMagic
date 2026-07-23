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
	standeeCoreShade    = 0.92 // token rim sits just out of the light vs the face
	standeeCoreShadeFar = 0.75 // the slab's far edge is in its own shadow
	standeeMaxShells    = 16   // cap on core shell layers (perf guard at point-blank range)
	// At high shell counts the exact same stack is composited in one fragment
	// pass. This is a render optimization, not a visual LOD.
	standeeVolumeMinShells = 6
	standeeMinDepth        = 4.0           // near clip for token columns (world units)
	standeeStaticYaw       = math.Pi / 4.0 // fixed diagonal for scenery and NPC tokens
	standeeTurnDefault     = 270.0         // deg/sec token swivel when config omits it
	containerSpinDegSec    = 60.0          // deg/sec idle spin for loot-bag / chest tokens
	standeeMaxMipLevel     = 6             // matches Ebitengine's own mipmap depth cap
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
	// The source pixels are already on the CPU here. Build both immutable mip
	// chains now instead of immediately reading the freshly uploaded core back
	// from the GPU in standeeMipChainFor (a sync point per visible frame).
	stickerPixels := &image.RGBA{
		Pix:    buf,
		Stride: 4 * w,
		Rect:   image.Rect(0, 0, w, h),
	}
	r.cacheStandeeMipChain(standeeMipKey{frame: key, layer: standeeMipSticker}, src, stickerPixels)
	r.cacheStandeeMipChain(standeeMipKey{frame: key, layer: standeeMipCore}, img, out)
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

// downsampleStandeeMip builds one premultiplied-alpha area-filtered mip on the
// CPU. Besides making transparent edges correct, CPU construction is important
// for batching: an ebiten.Image drawn into another ebiten.Image becomes a render
// target and is unlikely to share Ebitengine's automatic source atlas. A dense
// tree corridor then turns hundreds of otherwise compatible standee draws into
// separate GPU commands.
func downsampleStandeeMip(src *image.RGBA, size image.Point) *image.RGBA {
	if src == nil || size.X <= 0 || size.Y <= 0 {
		return nil
	}
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	if srcW == size.X*2 && srcH == size.Y*2 {
		// Every normal mip step is exactly 2x. Keep this hot load-time path
		// branch-free inside each 2x2 footprint; the generic area reducer below
		// only handles odd terminal dimensions.
		for y := 0; y < size.Y; y++ {
			srcRow0 := src.PixOffset(srcBounds.Min.X, srcBounds.Min.Y+y*2)
			srcRow1 := srcRow0 + src.Stride
			dstOff := y * dst.Stride
			for x := 0; x < size.X; x++ {
				s0 := srcRow0 + x*8
				s1 := srcRow1 + x*8
				for channel := 0; channel < 4; channel++ {
					sum := int(src.Pix[s0+channel]) + int(src.Pix[s0+4+channel]) +
						int(src.Pix[s1+channel]) + int(src.Pix[s1+4+channel])
					dst.Pix[dstOff+channel] = byte((sum + 2) / 4)
				}
				dstOff += 4
			}
		}
		return dst
	}
	for y := 0; y < size.Y; y++ {
		sy0 := y * srcH / size.Y
		sy1 := (y + 1) * srcH / size.Y
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < size.X; x++ {
			sx0 := x * srcW / size.X
			sx1 := (x + 1) * srcW / size.X
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var sums [4]int
			for sy := sy0; sy < sy1; sy++ {
				off := src.PixOffset(srcBounds.Min.X+sx0, srcBounds.Min.Y+sy)
				for sx := sx0; sx < sx1; sx++ {
					sums[0] += int(src.Pix[off])
					sums[1] += int(src.Pix[off+1])
					sums[2] += int(src.Pix[off+2])
					sums[3] += int(src.Pix[off+3])
					off += 4
				}
			}
			count := (sx1 - sx0) * (sy1 - sy0)
			off := y*dst.Stride + x*4
			for channel := range sums {
				dst.Pix[off+channel] = byte((sums[channel] + count/2) / count)
			}
		}
	}
	return dst
}

// cacheStandeeMipChain builds each level by one 2x area reduction from pixels
// already resident on the CPU. Every reduced level enters Ebitengine as a
// managed source image, allowing mip levels from different standees to share
// the automatic texture atlas. Level 0 is normalized to (0,0,w,h) only when
// the source is a sheet SubImage.
func (r *Renderer) cacheStandeeMipChain(key standeeMipKey, src *ebiten.Image, cpuLevel *image.RGBA) *standeeMipChain {
	if chain := r.standeeMipCache[key]; chain != nil {
		return chain
	}
	if src == nil || cpuLevel == nil {
		return nil
	}
	sizes := standeeMipSizes(cpuLevel.Bounds().Dx(), cpuLevel.Bounds().Dy())
	if len(sizes) == 0 {
		return nil
	}

	chain := &standeeMipChain{levels: make([]*ebiten.Image, 0, len(sizes))}
	base := src
	if src.Bounds().Min != (image.Point{}) {
		// The shader's full-size coordinate reference is normalized. Most sprite
		// images already start at (0,0) and can be reused directly; only sheet
		// SubImages need this managed-source copy. Avoiding a duplicate level 0
		// for standalone images cuts the mip cache's dominant allocation.
		base = ebiten.NewImageFromImage(cpuLevel)
	}
	chain.levels = append(chain.levels, base)
	for _, size := range sizes[1:] {
		cpuLevel = downsampleStandeeMip(cpuLevel, size)
		chain.levels = append(chain.levels, ebiten.NewImageFromImage(cpuLevel))
	}
	if r.standeeMipCache == nil {
		r.standeeMipCache = make(map[standeeMipKey]*standeeMipChain)
	}
	r.standeeMipCache[key] = chain
	return chain
}

// standeeMipChainFor is the fallback for callers that did not pass through
// standeeCoreSilhouette (mainly focused shader tests). Normal rendering builds
// both sticker/core chains from the CPU buffers already present there and never
// takes this GPU-readback path.
func (r *Renderer) standeeMipChainFor(key standeeMipKey, src *ebiten.Image) *standeeMipChain {
	if chain := r.standeeMipCache[key]; chain != nil {
		return chain
	}
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	pixels := make([]byte, 4*bounds.Dx()*bounds.Dy())
	src.ReadPixels(pixels)
	cpuLevel := &image.RGBA{
		Pix:    pixels,
		Stride: 4 * bounds.Dx(),
		Rect:   image.Rect(0, 0, bounds.Dx(), bounds.Dy()),
	}
	return r.cacheStandeeMipChain(key, src, cpuLevel)
}

// Ebitengine chooses one integer mip level for an entire DrawTriangles call.
// That is correct spatial filtering, but a moving token visibly jumps between
// sharp and soft levels because the engine does not trilinearly blend them.
// Images 1 and 2 are adjacent sticker levels, image 3 is the selected core
// level, and image 0 is the full-size coordinate reference. In pixel mode Kage
// adjusts only atlas origins for imageSrcNAt, so these helpers explicitly map
// full-size coordinates into each level before sampling.
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

func sampleNearest1(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc1Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	return imageSrc1At(q + origin0)
}

func sampleNearest3(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc3Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	return imageSrc3At(q + origin0)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	// custom.y: 0 = visible near sticker, 1 = core, 2 = mostly hidden far
	// sticker. Core/far layers use one tap from an already reduced mip; paying
	// full trilinear cost through every overdrawn shell erased batching's GPU
	// win. custom.z marks minification for the visible sticker.
	if custom.y > 1.5 {
		return sampleNearest1(srcPos) * color
	}
	if custom.y > 0.5 {
		return sampleNearest3(srcPos) * color
	}
	if custom.z <= 0.0 {
		return imageSrc0At(srcPos) * color
	}
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

// The close-tree volume shader composites the exact shell stack in one
// fragment invocation. It receives all per-slab data through vertices so
// successive trees with the same texture remain batchable:
//
//	custom.xy  far/near perpendicular depth
//	custom.zw  far/near source U
//	srcPos.xy  height/bottom projection invariants
//	color      brightness, wall depth, wall top, shell count
//
// Geometry covers the projected union of both outer faces. Every virtual
// layer reconstructs the same perspective height, source coordinate, wall
// clipping, shade, and front-to-back alpha blend as the physical shell path.
const standeeVolumeShaderSrc = `//kage:unit pixels

package main

func sampleLinear1(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc1Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	q0 := q - 0.5
	q1 := q + 0.5
	p0 := q0 + origin0
	p1 := q1 + origin0
	c0 := imageSrc1UnsafeAt(p0)
	c1 := imageSrc1UnsafeAt(vec2(p1.x, p0.y))
	c2 := imageSrc1UnsafeAt(vec2(p0.x, p1.y))
	c3 := imageSrc1UnsafeAt(p1)
	rate := fract(q1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}

func sampleLinear2(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc2Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	q0 := q - 0.5
	q1 := q + 0.5
	p0 := q0 + origin0
	p1 := q1 + origin0
	c0 := imageSrc2UnsafeAt(p0)
	c1 := imageSrc2UnsafeAt(vec2(p1.x, p0.y))
	c2 := imageSrc2UnsafeAt(vec2(p0.x, p1.y))
	c3 := imageSrc2UnsafeAt(p1)
	rate := fract(q1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}

func sampleNearest1(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc1Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	return imageSrc1UnsafeAt(q + origin0)
}

func sampleNearest3(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc3Size()
	q := clamp((p-origin0)*(size/size0), vec2(0.5), size-vec2(0.5))
	return imageSrc3UnsafeAt(q + origin0)
}

func nearStickerAt(p vec2, filtered bool, mipBlend float) vec4 {
	if !filtered {
		return imageSrc0UnsafeAt(p)
	}
	lo := sampleLinear1(p)
	if mipBlend <= 0.0 {
		return lo
	}
	return mix(lo, sampleLinear2(p), mipBlend)
}

func sourcePosition(dstY float, depth float, u float, heightScale float, bottomScale float) (vec2, bool) {
	if depth <= 0.0 || u < 0.0 || u > 1.0 {
		return vec2(0), false
	}
	height := heightScale / depth
	bottom := imageDstSize().y/2.0 + bottomScale/depth
	v := (dstY - (bottom - height)) / height
	if v < 0.0 || v > 1.0 {
		return vec2(0), false
	}
	size := imageSrc0Size()
	local := vec2(min(u*size.x, size.x-1.0)+0.5, v*(size.y-1.0)+0.5)
	return local + imageSrc0Origin(), true
}

func tinted(c vec4, brightness float, shade float) vec4 {
	return vec4(c.rgb*brightness*shade, c.a)
}

func addFront(acc vec4, c vec4) vec4 {
	return acc + c*(1.0-acc.a)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	farDepth := custom.x
	nearDepth := custom.y
	farU := custom.z
	nearU := custom.w
	projection := srcPos - imageSrc0Origin()
	dstY := dstPos.y - imageDstOrigin().y
	wallDepth := color.g
	wallTop := color.b
	packed := color.a
	shellCount := floor(packed)
	filterData := fract(packed)
	filtered := filterData >= 0.125
	mipBlend := clamp((filterData-0.25)/0.5, 0.0, 1.0)
	acc := vec4(0)

	if (nearDepth < wallDepth || dstY < wallTop) {
		if p, ok := sourcePosition(dstY, nearDepth, nearU, projection.x, projection.y); ok {
			acc = addFront(acc, tinted(nearStickerAt(p, filtered, mipBlend), color.r, 1.0))
		}
	}
	if acc.a >= 0.999 {
		return acc
	}

	for i := 0; i < 16; i++ {
		if float(i) < shellCount && acc.a < 0.999 {
			f := (shellCount-float(i))/(shellCount+1.0)
			depth := mix(farDepth, nearDepth, f)
			u := mix(farU, nearU, f)
			if (depth < wallDepth || dstY < wallTop) {
				if p, ok := sourcePosition(dstY, depth, u, projection.x, projection.y); ok {
					shade := 0.75 + (0.92-0.75)*f
					acc = addFront(acc, tinted(sampleNearest3(p), color.r, shade))
				}
			}
		}
	}
	if acc.a >= 0.999 {
		return acc
	}

	if (farDepth < wallDepth || dstY < wallTop) {
		if p, ok := sourcePosition(dstY, farDepth, farU, projection.x, projection.y); ok {
			acc = addFront(acc, tinted(sampleNearest1(p), color.r, 0.75))
		}
	}
	return acc
}
`

func (r *Renderer) ensureStandeeVolumeShader() (*ebiten.Shader, error) {
	if r.standeeVolumeShader != nil {
		return r.standeeVolumeShader, nil
	}
	shader, err := ebiten.NewShader([]byte(standeeVolumeShaderSrc))
	if err != nil {
		return nil, err
	}
	r.standeeVolumeShader = shader
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
	// volumeComposite is set only for crossed trees. Their close, high-shell
	// slabs use the exact one-pass volume compositor; other standees keep the
	// general material path, including trilinear minification and wall mounts.
	volumeComposite bool
	// Float billboard metrics: the per-column scaling multiplies them by
	// centerDepth/t, so any int quantization here would make the whole token
	// hop a pixel at a time as the camera moves (visible at range).
	centerSize  float64
	centerDepth float64
	bottomY     float64
	rr, gg, bb  float32
}

// treeArm is one center->corner half of a crossed standee: the slab it belongs
// to, its screen-column span, and its camera-space midpoint depth. The same
// geometry feeds both the cross's internal order and the unified sprite sort
// when another standee has to render between its arms.
type treeArm struct {
	slabIdx int
	lo, hi  int
	depth   float64
}

// crossedStandeeArms projects the four disjoint center-to-corner arms of a
// crossed standee. Camera-space perpendicular depth is the renderer-wide
// painter key; using Euclidean distance here can invert two arms near the edge
// of the view even though the unified sprite pass orders everything else by
// perpendicular depth.
func (r *Renderer) crossedStandeeArms(worldX, worldY, yawA, yawB, footprint float64) ([4]treeArm, bool) {
	var arms [4]treeArm
	if r == nil || r.game == nil || r.game.camera == nil || r.game.renderHelper == nil {
		return arms, false
	}

	screenW := r.game.config.GetScreenWidth()
	xc, _, okc := r.game.renderHelper.projectToScreenX(worldX, worldY)
	if !okc {
		return arms, false
	}
	clampCol := func(x int) int {
		if x < 0 {
			return 0
		}
		if x >= screenW {
			return screenW - 1
		}
		return x
	}

	cam := r.game.camera
	camDirX, camDirY := math.Cos(cam.Angle), math.Sin(cam.Angle)
	allOK := true
	armIndex := 0
	for slabIdx, yaw := range [2]float64{yawA, yawB} {
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
			midX := worldX + dx*side*footprint/4
			midY := worldY + dy*side*footprint/4
			arms[armIndex] = treeArm{
				slabIdx: slabIdx,
				lo:      clampCol(lo),
				hi:      clampCol(hi),
				depth:   (midX-cam.X)*camDirX + (midY-cam.Y)*camDirY,
			}
			armIndex++
		}
	}
	return arms, allOK
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

// standeeProjectedFootprint selects one conservative texture footprint for a
// slab. Ebitengine makes the same one-LOD-per-DrawTriangles choice: the least
// minified axis wins so an edge-on but still tall face does not become a blur.
func standeeProjectedFootprint(projectedWidth, projectedHeight, textureWidth, textureHeight float64) float32 {
	footprintX := math.Inf(1)
	if projectedWidth > 1e-6 {
		footprintX = textureWidth / projectedWidth
	}
	footprintY := math.Inf(1)
	if projectedHeight > 1e-6 {
		footprintY = textureHeight / projectedHeight
	}
	footprint := math.Min(footprintX, footprintY)
	if footprint < 1 {
		return 1
	}
	return float32(footprint)
}

func canUseStandeeVolume(slab standeeSlab) bool {
	return slab.volumeComposite &&
		slab.firstSurface == 0 &&
		len(slab.surfaces)-2 >= standeeVolumeMinShells
}

func (r *Renderer) drawStandeeSlabVolume(screen *ebiten.Image, slab standeeSlab, minX, maxX int, stickerMips, coreMips *standeeMipChain, mipLevel, nextMipLevel, coreMipLevel int, mipBlend float32, filtered bool) bool {
	shader, err := r.ensureStandeeVolumeShader()
	if err != nil {
		return false
	}
	if stickerMips == nil || coreMips == nil ||
		mipLevel < 0 || mipLevel >= len(stickerMips.levels) ||
		nextMipLevel < 0 || nextMipLevel >= len(stickerMips.levels) ||
		coreMipLevel < 0 || coreMipLevel >= len(coreMips.levels) {
		return false
	}

	screenW := r.game.config.GetScreenWidth()
	screenH := r.game.config.GetScreenHeight()
	cam := r.game.camera
	viewDistance := cam.ViewDist
	if viewDistance <= standeeMinDepth {
		return false
	}
	halfFovTan := math.Tan(cam.FOV / 2)
	dirX, dirY := math.Cos(cam.Angle), math.Sin(cam.Angle)
	planeX := math.Cos(cam.Angle+math.Pi/2) * halfFovTan
	planeY := math.Sin(cam.Angle+math.Pi/2) * halfFovTan
	far := slab.surfaces[0]
	near := slab.surfaces[len(slab.surfaces)-1]
	if far.mirrored != near.mirrored {
		return false
	}
	mirrorU := func(u float64) float64 {
		if near.mirrored {
			return 1 - u
		}
		return u
	}
	intersection := func(surface standeeSurface, screenX float64) (depth, u float64, ok bool) {
		rayX, rayY := standeeRayAtScreenX(screenX, screenW, dirX, dirY, planeX, planeY)
		return standeeColumnIntersection(cam.X, cam.Y, rayX, rayY, surface.p0x, surface.p0y, surface.dx, surface.dy)
	}
	geometryAt := func(depth float64) (top, bottom float32) {
		horizon := float64(screenH) / 2
		height := slab.centerSize * slab.centerDepth / depth
		bottomF := horizon + (slab.bottomY-horizon)*slab.centerDepth/depth
		return float32(bottomF - height), float32(bottomF)
	}

	vertices := r.standeeVerts[:0]
	indices := r.standeeMaterialIdx[:0]
	f0Depth, f0U, fok0 := intersection(far, float64(minX))
	n0Depth, n0U, nok0 := intersection(near, float64(minX))
	if !fok0 || !nok0 {
		return false
	}
	heightScale := float32(slab.centerSize * slab.centerDepth)
	bottomScale := float32((slab.bottomY - float64(screenH)/2) * slab.centerDepth)
	filterData := float32(0)
	if filtered {
		filterData = 0.25 + 0.5*mipBlend
	}
	packedShells := float32(len(slab.surfaces)-2) + filterData
	depthBuffer := r.game.depthBuffer
	wallTopBuffer := r.game.wallTopBuffer
	sourceOrigin := stickerMips.levels[0].Bounds().Min
	for x := minX; x <= maxX; x++ {
		f1Depth, f1U, fok1 := intersection(far, float64(x+1))
		n1Depth, n1U, nok1 := intersection(near, float64(x+1))
		if !fok1 || !nok1 {
			r.standeeVerts = vertices[:0]
			r.standeeMaterialIdx = indices[:0]
			return false
		}
		if math.Max(math.Max(f0U, f1U), math.Max(n0U, n1U)) < 0 ||
			math.Min(math.Min(f0U, f1U), math.Min(n0U, n1U)) > 1 {
			f0Depth, f0U, n0Depth, n0U = f1Depth, f1U, n1Depth, n1U
			continue
		}

		fTop0, fBottom0 := geometryAt(f0Depth)
		nTop0, nBottom0 := geometryAt(n0Depth)
		fTop1, fBottom1 := geometryAt(f1Depth)
		nTop1, nBottom1 := geometryAt(n1Depth)
		top0, bottom0 := min(fTop0, nTop0), max(fBottom0, nBottom0)
		top1, bottom1 := min(fTop1, nTop1), max(fBottom1, nBottom1)
		x0, x1 := float32(x), float32(x+1)
		base := uint32(len(vertices))

		wallDepth := viewDistance
		wallTop := 0.0
		if x >= 0 && x < len(depthBuffer) {
			if depth := depthBuffer[x]; depth > 0 && depth < wallDepth {
				wallDepth = depth
				if x < len(wallTopBuffer) {
					wallTop = float64(wallTopBuffer[x])
				}
			}
		}
		if wallTop < 0 {
			wallTop = 0
		} else if wallTop > float64(screenH) {
			wallTop = float64(screenH)
		}
		left := ebiten.Vertex{
			DstX: x0, SrcX: float32(sourceOrigin.X) + heightScale, SrcY: float32(sourceOrigin.Y) + bottomScale,
			Custom0: float32(f0Depth), Custom1: float32(n0Depth),
			Custom2: float32(mirrorU(f0U)), Custom3: float32(mirrorU(n0U)),
			ColorR: slab.rr, ColorG: float32(wallDepth), ColorB: float32(wallTop), ColorA: packedShells,
		}
		right := ebiten.Vertex{
			DstX: x1, SrcX: float32(sourceOrigin.X) + heightScale, SrcY: float32(sourceOrigin.Y) + bottomScale,
			Custom0: float32(f1Depth), Custom1: float32(n1Depth),
			Custom2: float32(mirrorU(f1U)), Custom3: float32(mirrorU(n1U)),
			ColorR: slab.rr, ColorG: float32(wallDepth), ColorB: float32(wallTop), ColorA: packedShells,
		}
		left.DstY, right.DstY = top0, top1
		vertices = append(vertices, left, right)
		left.DstY, right.DstY = bottom0, bottom1
		vertices = append(vertices, left, right)
		indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
		f0Depth, f0U, n0Depth, n0U = f1Depth, f1U, n1Depth, n1U
	}
	if len(indices) == 0 {
		r.standeeVerts = vertices[:0]
		r.standeeMaterialIdx = indices[:0]
		return true
	}

	opts := &r.standeeVolumeOpts
	opts.Blend = ebiten.BlendSourceOver
	opts.Images[0] = stickerMips.levels[0]
	opts.Images[1] = stickerMips.levels[mipLevel]
	opts.Images[2] = stickerMips.levels[nextMipLevel]
	opts.Images[3] = coreMips.levels[coreMipLevel]
	screen.DrawTrianglesShader32(vertices, indices, shader, opts)
	r.statStandeeCalls++
	r.standeeVerts = vertices[:0]
	r.standeeMaterialIdx = indices[:0]
	return true
}

// drawStandeeSlabColumns rasterizes a prepared slab, optionally narrowed to
// [clipMinX,clipMaxX] (-1 disables - crossed trees split the draw at the planes'
// crossover column so each arm draws far->near). All far sticker, core-shell
// and near-sticker triangles are submitted in painter order through one shader
// call. This preserves the old geometry and blending while avoiding a separate
// Ebitengine/GPU command for every physical shell.
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

	sticker := slab.surfaces[len(slab.surfaces)-1]
	if sticker.img == nil {
		return
	}
	stickerBounds := sticker.img.Bounds()
	texW := float64(stickerBounds.Dx())
	texH := float64(stickerBounds.Dy())
	if texW <= 0 || texH <= 0 {
		return
	}
	var core standeeSurface
	for i := slab.firstSurface; i < len(slab.surfaces); i++ {
		if slab.surfaces[i].mipKey.layer == standeeMipCore {
			core = slab.surfaces[i]
			break
		}
	}
	if core.img == nil {
		core = sticker // face-on fast path only samples the near sticker
	}

	stickerMips := r.standeeMipChainFor(sticker.mipKey, sticker.img)
	coreMips := r.standeeMipChainFor(core.mipKey, core.img)
	if stickerMips == nil || coreMips == nil {
		return
	}
	filtered := standeeUsesMinificationSampling(projectedWidth, projectedHeight, texW, texH)
	mipLevel, mipBlend := 0, float32(0)
	if filtered {
		mipLevel, mipBlend = standeeMipBlend(
			standeeProjectedFootprint(projectedWidth, projectedHeight, texW, texH),
			len(stickerMips.levels)-1,
		)
	}
	nextMipLevel := mipLevel + 1
	if nextMipLevel >= len(stickerMips.levels) {
		nextMipLevel = mipLevel
	}
	coreMipLevel := mipLevel
	if coreMipLevel >= len(coreMips.levels) {
		coreMipLevel = len(coreMips.levels) - 1
	}
	filteredFlag := float32(0)
	if filtered {
		filteredFlag = 1
	}
	if canUseStandeeVolume(slab) &&
		r.drawStandeeSlabVolume(screen, slab, minX, maxX, stickerMips, coreMips, mipLevel, nextMipLevel, coreMipLevel, mipBlend, filtered) {
		return
	}

	shader, err := r.ensureStandeeTrilinearShader()
	if err != nil {
		return
	}
	opts := &r.standeeTrilinearOpts
	opts.Blend = ebiten.BlendSourceOver
	opts.Images[0] = stickerMips.levels[0]
	opts.Images[1] = stickerMips.levels[mipLevel]
	opts.Images[2] = stickerMips.levels[nextMipLevel]
	opts.Images[3] = coreMips.levels[coreMipLevel]

	verts := r.standeeVerts[:0]
	idx := r.standeeMaterialIdx[:0]
	rayAt := func(screenX float64) (float64, float64) {
		return standeeRayAtScreenX(screenX, screenW, camDirX, camDirY, planeX, planeY)
	}
	geometryAt := func(t float64) (top, bottom, height float32) {
		height = float32(centerSize * centerDepth / t)
		bottom = float32(horizon + (bottomY-horizon)*centerDepth/t)
		return bottom - height, bottom, height
	}

	for surfaceIndex := slab.firstSurface; surfaceIndex < len(slab.surfaces); surfaceIndex++ {
		sf := slab.surfaces[surfaceIndex]
		if sf.img == nil {
			continue
		}
		cr := slab.rr * sf.shade
		cg := slab.gg * sf.shade
		cb := slab.bb * sf.shade
		textureX := func(u float64) float32 {
			if u < 0 {
				u = 0
			} else if u > 1 {
				u = 1
			}
			if sf.mirrored {
				u = 1 - u
			}
			return float32(math.Min(u*texW, texW-1)) + 0.5
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
			srcY0 := float32(0.5)
			srcY1 := float32(texH) - 0.5
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
			layerMode := float32(0)
			if sf.mipKey.layer == standeeMipCore {
				layerMode = 1
			} else if surfaceIndex != len(slab.surfaces)-1 {
				layerMode = 2
			}
			base := uint32(len(verts))
			verts = append(verts,
				ebiten.Vertex{DstX: x0, DstY: top0, SrcX: srcX0, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag},
				ebiten.Vertex{DstX: x1, DstY: top1, SrcX: srcX1, SrcY: srcY0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag},
				ebiten.Vertex{DstX: x0, DstY: drawBottom0, SrcX: srcX0, SrcY: srcYbot0, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag},
				ebiten.Vertex{DstX: x1, DstY: drawBottom1, SrcX: srcX1, SrcY: srcYbot1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: 1, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag},
			)
			idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
		}
	}
	if len(idx) > 0 {
		screen.DrawTrianglesShader32(verts, idx, shader, opts)
		r.statStandeeCalls++
	}
	r.standeeVerts = verts[:0]
	r.standeeMaterialIdx = idx[:0]
}

// drawCrossedTreeStandees renders a tree tile as two normal standees crossed
// along the tile's DIAGONALS (an "X" from above, corner to corner), with the
// usual standee thickness. Both are two-sided and share the tile's billboard
// metrics (depth/size/floor anchor), so they stay grounded. The texture is the
// TILE's own configured sprite (data-driven), so each tree tile keeps its art.
func treeIsBillboardLOD(distance, tileSize, lodTiles float64) bool {
	return tileSize > 0 && lodTiles > 0 && distance > lodTiles*tileSize
}

func treeStandeeSpriteName(tileType world.TileType3D) string {
	if world.GlobalTileManager != nil {
		if name := world.GlobalTileManager.GetSprite(tileType); name != "" {
			return name
		}
	}
	return "tree"
}

// prewarmTreeStandeeResources moves map-specific PNG decoding, core generation,
// mip construction, and shader compilation out of the first gameplay frame in
// which each tree type becomes visible. The cache scan already knows every tree
// on the current map, so it is the single place where this resource set can be
// prepared without guessing what a later camera pose may reveal.
func (r *Renderer) prewarmTreeStandeeResources() {
	if r == nil || r.game == nil || r.game.sprites == nil ||
		!r.game.config.Graphics.TreesAsBillboards || len(r.treeTilesCache) == 0 {
		return
	}

	seen := make(map[string]struct{})
	newImages := make(map[*ebiten.Image]struct{})
	var shaderStickerMips, shaderCoreMips *standeeMipChain
	for i := range r.treeTilesCache {
		spriteName := r.treeTilesCache[i].spriteName
		if spriteName == "" {
			spriteName = treeStandeeSpriteName(r.treeTilesCache[i].tileType)
		}
		if _, ok := seen[spriteName]; ok {
			continue
		}
		seen[spriteName] = struct{}{}

		sprite := r.game.sprites.GetSprite(spriteName)
		if sprite == nil {
			continue
		}
		key := standeeCoreKey{name: "tree:" + spriteName, bounds: sprite.Bounds(), img: sprite}
		_, alreadyCached := r.standeeCoreCache[key]
		r.standeeCoreSilhouette(key, sprite)
		if alreadyCached {
			continue
		}
		for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
			chain := r.standeeMipCache[standeeMipKey{frame: key, layer: layer}]
			if chain == nil {
				continue
			}
			if layer == standeeMipSticker && shaderStickerMips == nil {
				shaderStickerMips = chain
			}
			if layer == standeeMipCore && shaderCoreMips == nil {
				shaderCoreMips = chain
			}
			for _, img := range chain.levels {
				if img != nil {
					newImages[img] = struct{}{}
				}
			}
		}
	}

	// Both paths remain reachable as shell count and viewing angle change.
	_, _ = r.ensureStandeeTrilinearShader()
	_, _ = r.ensureStandeeVolumeShader()
	r.flushStandeeImageUploads(newImages, shaderStickerMips, shaderCoreMips)

	// Below the volume-shader threshold the fallback submits every virtual
	// surface. Reserve its worst full-screen case once at map load instead of
	// repeatedly growing these buffers as the party approaches a tree.
	screenW := r.game.config.GetScreenWidth()
	maxFallbackSurfaces := standeeVolumeMinShells + 1
	vertexCapacity := 4 * screenW * maxFallbackSurfaces
	indexCapacity := 6 * screenW * maxFallbackSurfaces
	if cap(r.standeeVerts) < vertexCapacity {
		r.standeeVerts = make([]ebiten.Vertex, 0, vertexCapacity)
	}
	if cap(r.standeeMaterialIdx) < indexCapacity {
		r.standeeMaterialIdx = make([]uint32, 0, indexCapacity)
	}
	surfaceCapacity := standeeMaxShells + 2
	if cap(r.standeeSurfaces) < surfaceCapacity {
		r.standeeSurfaces = make([]standeeSurface, 0, surfaceCapacity)
	}
	if cap(r.standeeSurfacesB) < surfaceCapacity {
		r.standeeSurfacesB = make([]standeeSurface, 0, surfaceCapacity)
	}
}

// flushStandeeImageUploads submits every newly built managed mip as a source
// before gameplay. Reading the tiny destination synchronizes their buffered
// WritePixels uploads while leaving the sources themselves atlas-friendly;
// calling ReadPixels on a source image would isolate it and destroy batching.
func (r *Renderer) flushStandeeImageUploads(images map[*ebiten.Image]struct{}, stickerMips, coreMips *standeeMipChain) {
	if len(images) == 0 {
		return
	}
	target := ebiten.NewImage(len(images)+2, 1)
	defer target.Dispose()
	x := 0
	for img := range images {
		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			continue
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(-bounds.Min.X), float64(-bounds.Min.Y))
		opts.GeoM.Scale(1/float64(bounds.Dx()), 1/float64(bounds.Dy()))
		opts.GeoM.Translate(float64(x), 0)
		target.DrawImage(img, opts)
		x++
	}

	if stickerMips != nil && coreMips != nil &&
		len(stickerMips.levels) > 0 && len(coreMips.levels) > 0 {
		stickerLevel1 := min(1, len(stickerMips.levels)-1)
		stickerLevel2 := min(2, len(stickerMips.levels)-1)
		coreLevel := min(1, len(coreMips.levels)-1)
		origin := stickerMips.levels[0].Bounds().Min
		srcX, srcY := float32(origin.X)+0.5, float32(origin.Y)+0.5
		indices := []uint32{0, 1, 2, 1, 3, 2}
		shaderOpts := func() *ebiten.DrawTrianglesShaderOptions {
			opts := &ebiten.DrawTrianglesShaderOptions{}
			opts.Images[0] = stickerMips.levels[0]
			opts.Images[1] = stickerMips.levels[stickerLevel1]
			opts.Images[2] = stickerMips.levels[stickerLevel2]
			opts.Images[3] = coreMips.levels[coreLevel]
			return opts
		}

		if r.standeeTrilinearShader != nil {
			left := float32(x)
			vertices := []ebiten.Vertex{
				{DstX: left, DstY: 0, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
				{DstX: left + 1, DstY: 0, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
				{DstX: left, DstY: 1, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
				{DstX: left + 1, DstY: 1, SrcX: srcX, SrcY: srcY, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom2: 1},
			}
			target.DrawTrianglesShader32(vertices, indices, r.standeeTrilinearShader, shaderOpts())
			x++
		}
		if r.standeeVolumeShader != nil {
			left := float32(x)
			vertices := []ebiten.Vertex{
				{DstX: left, DstY: 0, SrcX: srcX + 1, SrcY: srcY, ColorR: 1, ColorG: 100, ColorA: standeeVolumeMinShells, Custom0: 2, Custom1: 1, Custom2: 0.5, Custom3: 0.5},
				{DstX: left + 1, DstY: 0, SrcX: srcX + 1, SrcY: srcY, ColorR: 1, ColorG: 100, ColorA: standeeVolumeMinShells, Custom0: 2, Custom1: 1, Custom2: 0.5, Custom3: 0.5},
				{DstX: left, DstY: 1, SrcX: srcX + 1, SrcY: srcY, ColorR: 1, ColorG: 100, ColorA: standeeVolumeMinShells, Custom0: 2, Custom1: 1, Custom2: 0.5, Custom3: 0.5},
				{DstX: left + 1, DstY: 1, SrcX: srcX + 1, SrcY: srcY, ColorR: 1, ColorG: 100, ColorA: standeeVolumeMinShells, Custom0: 2, Custom1: 1, Custom2: 0.5, Custom3: 0.5},
			}
			target.DrawTrianglesShader32(vertices, indices, r.standeeVolumeShader, shaderOpts())
		}
	}
	target.ReadPixels(make([]byte, 4*(len(images)+2)))
}

func (r *Renderer) prewarmPendingTreeStandeeResources() {
	if r == nil || !r.treeStandeeResourcePrewarmPending {
		return
	}
	r.treeStandeeResourcePrewarmPending = false
	r.prewarmTreeStandeeResources()
}

func (r *Renderer) drawCrossedTreeStandees(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	spriteName := treeStandeeSpriteName(s.tileType)
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
	centerDepth := s.depthPerp
	if s.treeArmOnly {
		centerDepth = s.treeCenterDepth
	}
	// Footprint = the art's OWN projected width, like landmark monuments - a
	// tile-diagonal footprint squeezed the art horizontally (square oak drew
	// ~30% too thin once the square-projection FOV removed the old horizontal
	// stretch that was masking it).
	footprint := r.spriteFootprintWorld(s.sizeF, centerDepth)

	// Most crosses remain one unified painter entry and prepare both slabs once.
	// When another nearby standee overlaps this cross's depth interval, the
	// collector emits one entry per arm so that object can render between the
	// cross's far and near halves. Prepare only the selected slab here.
	if s.treeArmOnly {
		yaw := yawA
		if s.treeArmSlab == 1 {
			yaw = yawB
		}
		slab, ok := r.prepareStandeeSlab(
			sprite, key, worldX, worldY, yaw, centerDepth, heightF, bottomF,
			b, b, b, true, false, footprint, r.standeeSurfaces[:0],
		)
		slab.volumeComposite = true
		if ok {
			r.drawStandeeSlabColumns(screen, slab, s.treeArmLo, s.treeArmHi)
		}
		r.standeeSurfaces = slab.surfaces[:0]
		return
	}

	// Far crossed parallax is sub-pixel, so one camera-facing thick standee
	// retains the silhouette at a fraction of the cost.
	if treeIsBillboardLOD(distance, tileSize, r.game.config.Graphics.TreeStandeeLODTiles) {
		faceYaw := math.Atan2(r.game.camera.Y-worldY, r.game.camera.X-worldX) + math.Pi/2
		r.drawStandeeSprite(screen, sprite, key, worldX, worldY, faceYaw, s.depthPerp, heightF, bottomF, b, b, b, true, false, footprint)
		return
	}

	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, yawA, yawB, footprint, s.depthPerp, heightF, bottomF, b, true)
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
func (r *Renderer) drawCrossedSlabs(screen, sprite *ebiten.Image, key standeeCoreKey, worldX, worldY, yawA, yawB, footprint, depthPerp float64, heightF, bottomF float64, b float32, volumeComposite bool) {
	slabs := [2]standeeSlab{}
	slabOK := [2]bool{}
	slabs[0], slabOK[0] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawA, depthPerp, heightF, bottomF, b, b, b, true, false, footprint, r.standeeSurfaces[:0])
	slabs[1], slabOK[1] = r.prepareStandeeSlab(sprite, key, worldX, worldY, yawB, depthPerp, heightF, bottomF, b, b, b, true, false, footprint, r.standeeSurfacesB[:0])
	slabs[0].volumeComposite = volumeComposite
	slabs[1].volumeComposite = volumeComposite

	arms, allOK := r.crossedStandeeArms(worldX, worldY, yawA, yawB, footprint)
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
		sort.Slice(arms[:], func(i, j int) bool { return arms[i].depth > arms[j].depth }) // far -> near
		for _, a := range arms {
			if slabOK[a.slabIdx] {
				r.drawStandeeSlabColumns(screen, slabs[a.slabIdx], a.lo, a.hi)
			}
		}
	}
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
	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, spinYaw, spinYaw+math.Pi/2, footprint, depthPerp, heightF, bottomF, b, false)
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
