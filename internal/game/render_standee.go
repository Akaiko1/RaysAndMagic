package game

import (
	"cmp"
	"image"
	"image/draw"
	"math"
	"slices"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Standee rendering: an entity drawn as a thick wooden token standing in the
// world (a board-game standee) instead of a camera-facing billboard. The token
// is a slab: front and back sticker faces (the sprite, mirrored on the back)
// separated by a wooden core. Sampling follows the perpendicular depth of each
// column's ray intersection; adjacent columns share geometry where wall clipping
// permits it. The near edge renders larger than the far edge, the core rim stays
// visible at viewing angles, and walls occlude through the wall depth buffer.

const (
	standeeCoreShade    = 0.92 // token rim sits just out of the light vs the face
	standeeCoreShadeFar = 0.75 // the slab's far edge is in its own shadow
	standeeMaxShells    = config.MaxStandeeCoreLayers
	// Shell spacing is authored in 1920-wide screen pixels; above that width it
	// scales with the resolution (constant ANGULAR density), so 4K pays the same
	// layer count as 1080p instead of double.
	standeeShellRefWidth  = 1920
	standeeShellSpacingPx = 1.5
	// Composite every cross stack and fading token in one fragment pass. Even
	// the minimum two shells otherwise duplicate the column mesh four times.
	standeeVolumeMinShells = 2
	standeeMinDepth        = 4.0           // near clip for token columns (world units)
	standeeStaticYaw       = math.Pi / 4.0 // fixed diagonal for scenery and NPC tokens
	standeeTurnDefault     = 270.0         // deg/sec token swivel when config omits it
	containerSpinDegSec    = 60.0          // deg/sec idle spin for loot-bag / chest tokens
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

// Standee surfaces: standee_trilinear.kage branches on them by name and
// TestShaderConstantsMatchGo keeps its const block equal.
const (
	standeeLayerNearSticker = iota
	standeeLayerCore
	standeeLayerFarSticker
	standeeLayerEndWall // the core colour with its grain (standee_end_wall.kageinc)
)

// The half of a slab standee_volume.kage draws (standeeSlab.half, in its art
// U), packed above the shell count in steps of standeeVolumeHalfStride; the
// shader repeats these values.
const (
	standeeVolumeWhole     = 0
	standeeVolumeUpperHalf = 1 // art U from 0.5 up
	standeeVolumeLowerHalf = 2 // art U below 0.5
)

const standeeVolumeHalfStride = 32 // above config.MaxStandeeCoreLayers

func makeStandeeCoreKey(name string, img *ebiten.Image, stableImage bool) standeeCoreKey {
	if img == nil {
		return standeeCoreKey{name: name}
	}
	key := standeeCoreKey{name: name, bounds: img.Bounds()}
	if stableImage {
		key.img = img
	}
	return key
}

const standeeRenderSourceMaxPixels = 1024 * 1024

func standeeRenderSourceSize(width, height int) (int, int) {
	for width > 0 && height > 0 && int64(width)*int64(height) > standeeRenderSourceMaxPixels {
		width = max(1, (width+1)/2)
		height = max(1, (height+1)/2)
	}
	return width, height
}

// retainedSpriteCPU returns pixels still owned by either the current world-pass
// lazy-load stash or the active streaming task. It resolves a static sheet's
// SubImage frame to the matching CPU crop. Callers that get nil pay the
// ReadPixels readback they always did.
func (r *Renderer) retainedSpriteCPU(src *ebiten.Image) *image.RGBA {
	if r == nil || src == nil {
		return nil
	}
	cpuFor := func(img *ebiten.Image) *image.RGBA {
		if cpu := r.lazySpriteCPUPixels[img]; cpu != nil {
			return cpu
		}
		if task := r.mapRenderResourcePrewarmActive; r.mapRenderTaskCurrent(task) {
			return task.cpuImages[img]
		}
		return nil
	}
	if cpu := cpuFor(src); cpu != nil {
		return cpu
	}
	for sheet, frames := range r.animFrameCache {
		for _, frame := range frames {
			if frame != src {
				continue
			}
			cpu := cpuFor(sheet)
			if cpu == nil {
				return nil
			}
			sub, ok := cpu.SubImage(src.Bounds()).(*image.RGBA)
			if !ok || sub.Bounds().Empty() {
				return nil
			}
			return sub
		}
	}
	return nil
}

// boundedStandeeRenderSource prevents one high-resolution campaign sprite from
// multiplying into an equally large core plus two mip chains. Geometry still
// uses the authored size class; only the texture sampling source is reduced.
func (r *Renderer) boundedStandeeRenderSource(key standeeCoreKey, src *ebiten.Image) *ebiten.Image {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	targetW, targetH := standeeRenderSourceSize(w, h)
	if targetW == w && targetH == h {
		return src
	}
	if cached := r.standeeRenderSourceCache[key]; cached != nil {
		return cached
	}
	if cpu := r.retainedSpriteCPU(src); cpu != nil {
		bounded, cpuLevel := r.boundedStandeeRenderSourceFromCPU(key, src, cpu)
		if bounded != nil {
			// The silhouette build that follows receives the BOUNDED image, so
			// hand the reduced pixels forward under the same lifetime.
			if bounded != src && cpuLevel != nil {
				if r.lazySpriteCPUPixels == nil {
					r.lazySpriteCPUPixels = make(map[*ebiten.Image]*image.RGBA)
				}
				r.lazySpriteCPUPixels[bounded] = cpuLevel
			}
			return bounded
		}
	}
	pixels := make([]byte, 4*w*h)
	r.readRenderPixels(src, pixels)
	cpuLevel := &image.RGBA{
		Pix:    pixels,
		Stride: 4 * w,
		Rect:   image.Rect(0, 0, w, h),
	}
	for cpuLevel.Bounds().Dx() != targetW || cpuLevel.Bounds().Dy() != targetH {
		nextW := max(targetW, (cpuLevel.Bounds().Dx()+1)/2)
		nextH := max(targetH, (cpuLevel.Bounds().Dy()+1)/2)
		cpuLevel = downsampleMip(cpuLevel, image.Pt(nextW, nextH))
		if cpuLevel == nil {
			return src
		}
	}
	bounded := newStandeeTextureFromPixels(cpuLevel)
	if r.standeeRenderSourceCache == nil {
		r.standeeRenderSourceCache = make(map[standeeCoreKey]*ebiten.Image)
	}
	r.standeeRenderSourceCache[key] = bounded
	return bounded
}

// boundedStandeeRenderSourceFromCPU is the streaming-prewarm variant. The
// loader already owns these pixels, so keep all reduction work on the CPU and
// avoid synchronizing a freshly uploaded source back through ReadPixels.
func (r *Renderer) boundedStandeeRenderSourceFromCPU(key standeeCoreKey, src *ebiten.Image, cpu *image.RGBA) (*ebiten.Image, *image.RGBA) {
	if src == nil || cpu == nil {
		return r.boundedStandeeRenderSource(key, src), nil
	}
	w, h := cpu.Bounds().Dx(), cpu.Bounds().Dy()
	targetW, targetH := standeeRenderSourceSize(w, h)
	if targetW == w && targetH == h {
		return src, cpu
	}
	if cached := r.standeeRenderSourceCache[key]; cached != nil {
		return cached, nil
	}
	cpuLevel := cpu
	for cpuLevel.Bounds().Dx() != targetW || cpuLevel.Bounds().Dy() != targetH {
		nextW := max(targetW, (cpuLevel.Bounds().Dx()+1)/2)
		nextH := max(targetH, (cpuLevel.Bounds().Dy()+1)/2)
		cpuLevel = downsampleMip(cpuLevel, image.Pt(nextW, nextH))
		if cpuLevel == nil {
			return src, cpu
		}
	}
	bounded := newStandeeTextureFromPixels(cpuLevel)
	if r.standeeRenderSourceCache == nil {
		r.standeeRenderSourceCache = make(map[standeeCoreKey]*ebiten.Image)
	}
	r.standeeRenderSourceCache[key] = bounded
	return bounded, cpuLevel
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

// standeeCoreSilhouette returns (building and caching on first use) the sprite
// frame's silhouette filled with the configured wood-to-sprite-average color
// blend plus faint horizontal grain, so the token core follows the die-cut art.
func (r *Renderer) standeeCoreSilhouette(key standeeCoreKey, src *ebiten.Image) *ebiten.Image {
	if img, ok := r.standeeCoreCache[key]; ok {
		r.trackResidentStandeeKey(key, src)
		return img
	}
	if cpu := r.retainedSpriteCPU(src); cpu != nil {
		return r.standeeCoreSilhouetteFromCPU(key, src, cpu)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	buf := make([]byte, 4*w*h)
	r.readRenderPixels(src, buf)
	cpu := &image.RGBA{Pix: buf, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
	return r.standeeCoreSilhouetteFromCPU(key, src, cpu)
}

func (r *Renderer) standeeCoreSilhouetteFromCPU(key standeeCoreKey, src *ebiten.Image, cpu *image.RGBA) *ebiten.Image {
	if img, ok := r.standeeCoreCache[key]; ok {
		r.trackResidentStandeeKey(key, src)
		return img
	}
	if src == nil || cpu == nil {
		return nil
	}
	prepared := prepareStandeePixels(cpu, r.game.config.Graphics.Standee.CoreTint, false, r.standeeCoreFromEdge(key.name))
	_, core := r.commitPreparedStandeePixels(key, src, prepared)
	return core
}

// standeeCoreFromEdge reports whether a mob's core takes the silhouette-edge
// colour (monsters.yaml standee_core). Corpses share the mob key name.
func (r *Renderer) standeeCoreFromEdge(name string) bool {
	if r.standeeEdgeCores == nil {
		r.standeeEdgeCores = map[string]bool{}
		if cfg := monster.MonsterConfig; cfg != nil {
			for key, def := range cfg.Monsters {
				if def.StandeeCore == monster.StandeeCoreSilhouetteEdge {
					r.standeeEdgeCores[r.prefixedStandeeKeyName("mob", key)] = true
				}
			}
		}
	}
	return r.standeeEdgeCores[name]
}

// standeeCoreTone blends the wood tone toward the art's colour. By default that
// is the perceived colour, a chroma-weighted average of the opaque texels: a
// plain mean reads wrong - dark outlines and brown gear drown a goblin's green
// skin - so saturated pixels dominate and near-grey ones barely vote (the +0.02
// floor keeps monochrome sprites at their own grey). silhouetteEdge averages
// the border texels instead: the colour an extruded cut-out shows on its side.
func standeeCoreTone(sticker *image.RGBA, tint float64, silhouetteEdge bool) [3]float64 {
	buf, w, h := sticker.Pix, sticker.Bounds().Dx(), sticker.Bounds().Dy()
	opaque := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < w && y < h && buf[(y*w+x)*4+3] >= standeeCoreMinAlpha
	}
	var sumR, sumG, sumB, sumW float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !opaque(x, y) || silhouetteEdge && opaque(x-1, y) && opaque(x+1, y) && opaque(x, y-1) && opaque(x, y+1) {
				continue
			}
			i := (y*w + x) * 4
			a := float64(buf[i+3])
			// Un-premultiply to straight 0..1 color.
			cr := float64(buf[i]) / a
			cg := float64(buf[i+1]) / a
			cb := float64(buf[i+2]) / a
			weight := 1.0
			if !silhouetteEdge {
				weight = math.Max(cr, math.Max(cg, cb)) - math.Min(cr, math.Min(cg, cb)) + 0.02
			}
			sumR += cr * weight
			sumG += cg * weight
			sumB += cb * weight
			sumW += weight
		}
	}
	tone := standeeWoodTone
	if sumW > 0 {
		tint = max(0, min(1, tint))
		tone[0] += (sumR/sumW - tone[0]) * tint
		tone[1] += (sumG/sumW - tone[1]) * tint
		tone[2] += (sumB/sumW - tone[2]) * tint
	}
	return tone
}

type standeePreparedPixels struct {
	sticker     *image.RGBA
	core        *image.RGBA
	stickerMips []*image.RGBA
	coreMips    []*image.RGBA
	// rows bounds what each level of either chain can colour.
	rows []standeeLevelRows
}

func prepareStandeePixels(cpu *image.RGBA, tint float64, boundSource, silhouetteEdge bool) standeePreparedPixels {
	if cpu == nil {
		return standeePreparedPixels{}
	}
	b := cpu.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return standeePreparedPixels{}
	}
	sticker := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(sticker, sticker.Bounds(), cpu, b.Min, draw.Src)
	if boundSource {
		targetW, targetH := standeeRenderSourceSize(w, h)
		for sticker.Bounds().Dx() != targetW || sticker.Bounds().Dy() != targetH {
			nextW := max(targetW, (sticker.Bounds().Dx()+1)/2)
			nextH := max(targetH, (sticker.Bounds().Dy()+1)/2)
			sticker = downsampleMip(sticker, image.Pt(nextW, nextH))
			if sticker == nil {
				return standeePreparedPixels{}
			}
		}
	}
	buf := sticker.Pix
	w, h = sticker.Bounds().Dx(), sticker.Bounds().Dy()
	tone := standeeCoreTone(sticker, tint, silhouetteEdge)

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		// Horizontal grain: a slow brightness wave down the slab edge.
		grain := 1.0 + 0.08*math.Sin(float64(y)*0.9)
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			a := buf[i+3]
			if a < standeeCoreMinAlpha {
				continue
			}
			o := y*out.Stride + x*4
			out.Pix[o] = clampByte(tone[0] * grain * float64(a))
			out.Pix[o+1] = clampByte(tone[1] * grain * float64(a))
			out.Pix[o+2] = clampByte(tone[2] * grain * float64(a))
			out.Pix[o+3] = a
		}
	}
	prepared := standeePreparedPixels{
		sticker:     sticker,
		core:        out,
		stickerMips: prepareStandeeMipPixels(sticker),
		coreMips:    prepareStandeeMipPixels(out),
	}
	prepared.rows = standeeArtRows(prepared.stickerMips)
	if len(prepared.rows) > 0 {
		prepared.rows[0].endTone = [4]float32{float32(tone[0]), float32(tone[1]), float32(tone[2]), 1}
	}
	return prepared
}

func prepareStandeeMipPixels(base *image.RGBA) []*image.RGBA {
	if base == nil {
		return nil
	}
	sizes := mipSizesUniform(base.Bounds().Dx(), base.Bounds().Dy())
	if len(sizes) == 0 {
		return nil
	}
	levels := make([]*image.RGBA, 0, len(sizes))
	level := base
	levels = append(levels, level)
	for _, size := range sizes[1:] {
		level = downsampleMip(level, size)
		if level == nil {
			return nil
		}
		levels = append(levels, level)
	}
	return levels
}

// standeeMipBaseNeedsCopy is the shared level-0 ownership rule for both the
// synchronous and streaming standee builders. Shader coordinates are expressed
// in a normalized (0,0,w,h) space, so a sheet SubImage with a non-zero origin
// cannot be bound directly. A prepared base with a different size likewise
// needs its own image. Standalone sources already matching that space may alias
// level 0 to avoid the mip cache's largest duplicate allocation.
func standeeMipBaseNeedsCopy(src *ebiten.Image, preparedBase *image.RGBA) bool {
	if src == nil || preparedBase == nil {
		return false
	}
	return src.Bounds().Min != (image.Point{}) ||
		src.Bounds().Size() != preparedBase.Bounds().Size()
}

func (r *Renderer) commitPreparedStandeePixels(key standeeCoreKey, src *ebiten.Image, prepared standeePreparedPixels) (*ebiten.Image, *ebiten.Image) {
	if src == nil || prepared.sticker == nil || prepared.core == nil {
		return src, nil
	}
	if existing := r.standeeCoreCache[key]; existing != nil {
		sticker := src
		if bounded := r.standeeRenderSourceCache[key]; bounded != nil {
			sticker = bounded
		}
		r.trackResidentStandeeKey(key, src)
		return sticker, existing
	}
	sticker := src
	if src.Bounds().Size() != prepared.sticker.Bounds().Size() {
		sticker = newStandeeTextureFromPixels(prepared.sticker)
		if r.standeeRenderSourceCache == nil {
			r.standeeRenderSourceCache = make(map[standeeCoreKey]*ebiten.Image)
		}
		r.standeeRenderSourceCache[key] = sticker
	}
	img := newStandeeTextureFromPixels(prepared.core)
	if r.standeeCoreCache == nil {
		r.standeeCoreCache = make(map[standeeCoreKey]*ebiten.Image)
	}
	r.standeeCoreCache[key] = img
	stickerKey, coreKey := standeeMipKey{frame: key, layer: standeeMipSticker}, standeeMipKey{frame: key, layer: standeeMipCore}
	var sheet *standeeSheet
	if r.standeeMipCache[stickerKey] == nil && r.standeeMipCache[coreKey] == nil {
		sheet = newStandeeSheet(prepared.stickerMips, prepared.coreMips)
	}
	if sheet == nil {
		r.cachePreparedStandeeMipChain(stickerKey, sticker, prepared.stickerMips, nil, prepared.rows)
		r.cachePreparedStandeeMipChain(coreKey, img, prepared.coreMips, nil, prepared.rows)
	} else {
		// The sticker chain owns the sheet; the core levels are views too.
		if chain := r.cachePreparedStandeeMipChain(stickerKey, sticker, prepared.stickerMips, sheet.sticker, prepared.rows); chain != nil {
			chain.owned = append(chain.owned, sheet.image)
		}
		r.cachePreparedStandeeMipChain(coreKey, img, prepared.coreMips, sheet.core, prepared.rows)
		if r.standeeSheets == nil {
			r.standeeSheets = make(map[standeeCoreKey]*standeeSheet)
		}
		r.standeeSheets[key] = sheet
	}
	r.trackResidentStandeeKey(key, src)
	return sticker, img
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
// u outside [0,1] for volume-shell interpolation. Visibility and the material
// path use finite pixel-centre hits; explicit mip footprints handle filtering.
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
// surface segment. The material draw derives geometry and UV from this same
// pixel-centre hit, including when a face projects to less than one pixel.
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

// cacheStandeeMipChain builds each level by one 2x area reduction from pixels
// already resident on the CPU. Every reduced level enters Ebitengine as a
// managed source image, allowing mip levels from different standees to share
// the automatic texture atlas. Level 0 is normalized to (0,0,w,h) only when
// the source is a sheet SubImage.
func (r *Renderer) cacheStandeeMipChain(key standeeMipKey, src *ebiten.Image, cpuLevel *image.RGBA) *mipChain {
	levels := prepareStandeeMipPixels(cpuLevel)
	return r.cachePreparedStandeeMipChain(key, src, levels, nil, standeeArtRows(levels))
}

// views, when set, are the token sheet's views for the reduced levels: the
// chain fills them instead of allocating an image per level, and the caller
// owns the sheet. rows are the levels' measured bounds.
func (r *Renderer) cachePreparedStandeeMipChain(key standeeMipKey, src *ebiten.Image, cpuLevels []*image.RGBA, views []*ebiten.Image, rows []standeeLevelRows) *mipChain {
	if chain := r.standeeMipCache[key]; chain != nil {
		return chain
	}
	if src == nil || len(cpuLevels) == 0 || cpuLevels[0] == nil {
		return nil
	}

	chain := &mipChain{levels: make([]*ebiten.Image, 0, len(cpuLevels)), rows: rows}
	base := src
	if standeeMipBaseNeedsCopy(src, cpuLevels[0]) {
		// The shader's full-size coordinate reference is normalized. Most sprite
		// images already start at (0,0) and can be reused directly; only sheet
		// SubImages need this managed-source copy. Avoiding a duplicate level 0
		// for standalone images cuts the mip cache's dominant allocation.
		base = newStandeeTextureFromPixels(cpuLevels[0])
		chain.owned = append(chain.owned, base)
	}
	chain.levels = append(chain.levels, base)
	for k, cpuLevel := range cpuLevels[1:] {
		if views != nil {
			bounds := cpuLevel.Bounds()
			start := cpuLevel.PixOffset(bounds.Min.X, bounds.Min.Y)
			views[k+1].WritePixels(cpuLevel.Pix[start : start+4*bounds.Dx()*bounds.Dy()])
			chain.levels = append(chain.levels, views[k+1])
			continue
		}
		level := newStandeeTextureFromPixels(cpuLevel)
		chain.levels = append(chain.levels, level)
		chain.owned = append(chain.owned, level)
	}
	if r.standeeMipCache == nil {
		r.standeeMipCache = make(map[standeeMipKey]*mipChain)
	}
	r.standeeMipCache[key] = chain
	return chain
}

// standeeMipChainFor is the fallback for callers that did not pass through
// standeeCoreSilhouette (mainly focused shader tests). Normal rendering builds
// both sticker/core chains from the CPU buffers already present there and never
// takes this GPU-readback path.
func (r *Renderer) standeeMipChainFor(key standeeMipKey, src *ebiten.Image) *mipChain {
	if chain := r.standeeMipCache[key]; chain != nil {
		return chain
	}
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	pixels := make([]byte, 4*bounds.Dx()*bounds.Dy())
	r.readRenderPixels(src, pixels)
	cpuLevel := &image.RGBA{
		Pix:    pixels,
		Stride: 4 * bounds.Dx(),
		Rect:   image.Rect(0, 0, bounds.Dx(), bounds.Dy()),
	}
	return r.cacheStandeeMipChain(key, src, cpuLevel)
}

func (r *Renderer) ensureStandeeTrilinearShader() (*ebiten.Shader, error) {
	err := compileShader(&r.standeeTrilinearShader, "standee_trilinear.kage")
	return r.standeeTrilinearShader, err
}

func (r *Renderer) ensureStandeeFlipBlendShader() (*ebiten.Shader, error) {
	err := compileShader(&r.standeeFlipBlendShader, "standee_flip_blend.kage")
	return r.standeeFlipBlendShader, err
}

func (r *Renderer) ensureStandeeVolumeShader() (*ebiten.Shader, error) {
	err := compileShader(&r.standeeVolumeShader, "standee_volume.kage")
	return r.standeeVolumeShader, err
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

// standeeEndWallSurface is the strip of thickness across a slab at art U u,
// between its outer faces: the end wall of rows whose art ends there. Its
// colour comes from the vertices and the shader, never from a texture.
func standeeEndWallSurface(slab standeeSlab, u float64) standeeSurface {
	far, near := slab.surfaces[0], slab.surfaces[len(slab.surfaces)-1]
	if near.mirrored {
		u = 1 - u // faces sample mirrored art from the far side
	}
	x0, y0 := far.p0x+far.dx*u, far.p0y+far.dy*u
	x1, y1 := near.p0x+near.dx*u, near.p0y+near.dy*u
	return standeeSurface{p0x: x0, p0y: y0, dx: x1 - x0, dy: y1 - y0, shade: standeeCoreShade}
}

// visibleStandeeEndWalls lists the end walls (standeeLevelRows.endWalls) that
// face the camera, farthest first, in a buffer reused by the next call. A wall
// faces away from its rows' art: the camera sees it from beyond that end.
// Neighbouring walls join while they are less than a screen pixel tall,
// at the outermost of their ends.
func (r *Renderer) visibleStandeeEndWalls(slab standeeSlab, rows *standeeLevelRows, camX, camY float64) []standeeEndWall {
	near := slab.surfaces[len(slab.surfaces)-1]
	lengthSq := near.dx*near.dx + near.dy*near.dy
	if lengthSq <= 0 {
		return nil
	}
	camU := ((camX-near.p0x)*near.dx + (camY-near.p0y)*near.dy) / lengthSq
	if near.mirrored {
		camU = 1 - camU
	}
	minSpan := float32(1 / max(slab.centerSize, 1))
	walls := r.standeeEndWallScratch[:0]
	for end, list := range rows.endWalls {
		first := len(walls)
		for _, w := range list {
			if end == 0 && camU >= float64(w.u) || end == 1 && camU <= float64(w.u) {
				continue
			}
			if u := float64(w.u); near.mirrored && !slab.showsU(1-u) || !near.mirrored && !slab.showsU(u) {
				continue
			}
			if n := len(walls); n > first && walls[n-1].v1 == w.v0 && walls[n-1].v1-walls[n-1].v0 < minSpan {
				prev := &walls[n-1]
				prev.v1 = w.v1
				if end == 0 {
					prev.u = min(prev.u, w.u)
				} else {
					prev.u = max(prev.u, w.u)
				}
				continue
			}
			walls = append(walls, w)
		}
	}
	slices.SortFunc(walls, func(a, b standeeEndWall) int {
		return cmp.Compare(math.Abs(camU-float64(b.u)), math.Abs(camU-float64(a.u)))
	})
	r.standeeEndWallScratch = walls
	return walls
}

// Renderer.standeeLayers slots. The fade layer has its own, so a pass drawing
// into one layer never also reads it.
const (
	standeeFlipMainLayerSlot = iota
	standeeFlipOtherLayerSlot
	standeeFadeLayerSlot
	standeeLayerSlots
)

// standeeLayer returns *slot shaped like bounds, made on first use or a size
// change. These targets alternate between writes and reads: dedicated backing
// keeps them out of the shared atlases.
func standeeLayer(slot **ebiten.Image, bounds image.Rectangle) *ebiten.Image {
	if *slot == nil || (*slot).Bounds() != bounds {
		if *slot != nil {
			(*slot).Deallocate()
		}
		*slot = ebiten.NewImageWithOptions(bounds, &ebiten.NewImageOptions{Unmanaged: true})
	}
	return *slot
}

// drawStandeeIntoLayer clears the band of layer, a target shaped like screen,
// that slab over columns minX..maxX can cover, draws the slab there and
// returns the band. A Panini view draws the layer through its own rasterizer
// of the same projection.
func (r *Renderer) drawStandeeIntoLayer(layer, screen *ebiten.Image, slab standeeSlab, minX, maxX int) image.Rectangle {
	bounds := screen.Bounds()
	x0, x1 := float64(minX), float64(maxX+1)
	if p := worldRaster(screen); p != nil {
		x0, _ = p.presentedPoint(x0, p.projection.horizon)
		x1, _ = p.presentedPoint(x1, p.projection.horizon)
		r.standeeLayerRaster.configure(layer, p.projection)
		previous := activeWorldRaster
		activeWorldRaster = &r.standeeLayerRaster
		defer func() { activeWorldRaster = previous; r.standeeLayerRaster.target = nil }()
	}
	band := image.Rect(int(math.Floor(min(x0, x1)))-2, bounds.Min.Y, int(math.Ceil(max(x0, x1)))+2, bounds.Max.Y).Intersect(bounds)
	if band.Empty() {
		return band
	}
	layer.SubImage(band).(*ebiten.Image).Clear()
	r.drawStandeeSlabColumns(layer, slab, minX, maxX)
	return band
}

// drawStandeeFadeLayer draws a fading slab opaque into a layer, then fades
// that band onto screen once.
func (r *Renderer) drawStandeeFadeLayer(screen *ebiten.Image, slab standeeSlab, minX, maxX int) {
	opaque := slab
	opaque.fade = 0
	layer := standeeLayer(&r.standeeLayers[standeeFadeLayerSlot], screen.Bounds())
	band := r.drawStandeeIntoLayer(layer, screen, opaque, minX, maxX)
	if band.Empty() {
		return
	}
	a := 1 - slab.fade
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(band.Min.X), float64(band.Min.Y))
	op.ColorScale.Scale(a, a, a, a) // premultiplied colour fades in all four
	screen.DrawImage(layer.SubImage(band).(*ebiten.Image), &op)
}

// drawStandeeFlipBlend draws a crossed plane near edge-on as both readings of
// its art, mixed by flipBlend: each into its own layer, then one exact mix
// onto screen. Where both readings are solid the background never shows;
// where only one is, it fades as that reading's share does.
func (r *Renderer) drawStandeeFlipBlend(screen *ebiten.Image, slab standeeSlab, minX, maxX int) {
	main := slab
	main.flipBlend = 0
	shader, err := r.ensureStandeeFlipBlendShader()
	if err != nil {
		r.drawStandeeSlabColumns(screen, main, minX, maxX)
		return
	}
	// The other reading mirrors every surface; the prepared slab may be shared
	// with the plane's other arm, so it is copied.
	other := main
	other.surfaces = append(r.standeeFlipSurfaces[:0], slab.surfaces...)
	for i := range other.surfaces {
		other.surfaces[i].mirrored = !other.surfaces[i].mirrored
	}
	r.standeeFlipSurfaces = other.surfaces[:0]
	bounds := screen.Bounds()
	mainLayer := standeeLayer(&r.standeeLayers[standeeFlipMainLayerSlot], bounds)
	otherLayer := standeeLayer(&r.standeeLayers[standeeFlipOtherLayerSlot], bounds)
	band := r.drawStandeeIntoLayer(mainLayer, screen, main, minX, maxX)
	if band.Empty() {
		return
	}
	r.drawStandeeIntoLayer(otherLayer, screen, other, minX, maxX)
	opts := &r.standeeFlipBlendOpts
	if opts.Uniforms == nil {
		// Boxed once: storing a float32 would allocate on every draw.
		opts.Uniforms = map[string]any{"Weight": make([]float32, 1)}
	}
	opts.Uniforms["Weight"].([]float32)[0] = slab.flipBlend
	opts.Images[0] = mainLayer.SubImage(band).(*ebiten.Image)
	opts.Images[1] = otherLayer.SubImage(band).(*ebiten.Image)
	opts.GeoM.Reset()
	opts.GeoM.Translate(float64(band.Min.X), float64(band.Min.Y))
	screen.DrawRectShader(band.Dx(), band.Dy(), shader, opts)
}

// releaseStandeeLayers frees the fade and flip-blend targets.
func (r *Renderer) releaseStandeeLayers() {
	for i, layer := range r.standeeLayers {
		if layer != nil {
			layer.Deallocate()
			r.standeeLayers[i] = nil
		}
	}
}

// standeeRaisedColumns narrows lo..hi to the columns whose pixel-centre rays
// meet the raised part of surface sf; false when none can.
func (r *Renderer) standeeRaisedColumns(slab standeeSlab, sf standeeSurface, lo, hi int) (int, int, bool) {
	if slab.half == 0 {
		return lo, hi, true
	}
	a, b := slab.raisedSpan()
	xa, xb, ok := r.game.renderHelper.projectSegmentSpanXF(sf.p0x+sf.dx*a, sf.p0y+sf.dy*a, sf.p0x+sf.dx*b, sf.p0y+sf.dy*b)
	if !ok {
		return 0, -1, false
	}
	lo, hi = max(lo, int(math.Ceil(xa-0.5))), min(hi, int(math.Floor(xb-0.5)))
	return lo, hi, lo <= hi
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
	firstSurface int     // draw from here (zero side opacity skips hidden layers)
	sideFade     float32 // 0 = full thickness, 1 = only the front face
	fade         float32 // opacity loss of the whole token; zero preserves the ordinary material
	gold         goldMaterial
	minX, maxX   int // unclipped screen span
	// faceWidth is the outer faces' wider projected width in pixels and
	// thicknessWidth the wider of its ends' thickness on screen. An end behind
	// the camera means the slab runs past it, never along the view: +Inf and 0.
	faceWidth, thicknessWidth float64
	// half is 0 for the whole plane, +1 or -1 for only its +yaw or -yaw half:
	// a linked panel's lone half is cut at the tile centre in the world, since
	// cutting it by screen columns shows the other half end-on.
	half int8
	// flipBlend is the share of the other side's art reading in a crossed
	// plane seen near edge-on (prepareCrossPlane): 0 outside the window.
	flipBlend float32
	// volumeComposite is set only for crosses: they and fading tokens use the
	// one-pass volume compositor; other standees keep the general material
	// path, including trilinear minification and wall mounts.
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
	side    int8 // +1 the plane's +yaw half, -1 its -yaw half (standeeSlab.half)
}

// crossedStandeeArms projects the disjoint centre-to-end arms a crossed
// layout raises and returns how many there are. Camera-space perpendicular
// depth is the renderer-wide painter key; using Euclidean distance here can
// invert two arms near the edge of the view even though the unified sprite
// pass orders everything else by perpendicular depth.
func (r *Renderer) crossedStandeeArms(worldX, worldY float64, layout crossLayout) ([maxCrossArms]treeArm, int, bool) {
	var arms [maxCrossArms]treeArm
	if r == nil || r.game == nil || r.game.camera == nil || r.game.renderHelper == nil {
		return arms, 0, false
	}

	screenW := r.game.worldWidth()
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
	halfThickness := r.game.config.Graphics.Standee.ThicknessTiles * float64(r.game.config.GetTileSize()) / 2
	anyVisible := false
	armIndex := 0
	for slabIdx, plane := range layout.planes[:layout.count] {
		dx, dy := math.Cos(plane.yaw), math.Sin(plane.yaw)
		footprint := plane.footprint
		for sideIndex, side := range [2]float64{+1, -1} {
			if !plane.arms[sideIndex] {
				continue
			}
			cornerX := worldX + dx*side*footprint/2
			cornerY := worldY + dy*side*footprint/2
			// Segment projection, not point projection: pressed up against the
			// cross, the CENTER (or a corner) sits behind the camera plane while
			// the arm is still on screen. A failed point projection here used to
			// abort the whole split, dropping the draw to the two-whole-slabs
			// fallback - the "back arm on top of the front arm" artifact.
			lo, hi, visible := r.game.renderHelper.projectSegmentSpanX(worldX, worldY, cornerX, cornerY)
			// The arm is as thick as the slab: seen along the plane, its faces
			// and end walls lie beside the centre line's projection. Its draw is
			// cut to its own half in the world, so a wider span only costs
			// columns, never repaints the other half.
			for _, offset := range [2]float64{-halfThickness, halfThickness} {
				ox, oy := -dy*offset, dx*offset
				if a, b, ok := r.game.renderHelper.projectSegmentSpanXF(worldX+ox, worldY+oy, cornerX+ox, cornerY+oy); ok {
					if !visible {
						lo, hi, visible = int(math.Floor(a)), int(math.Ceil(b)), true
					}
					lo, hi = min(lo, int(math.Floor(a))), max(hi, int(math.Ceil(b)))
				}
			}
			midX := worldX + dx*side*footprint/4
			midY := worldY + dy*side*footprint/4
			arm := treeArm{
				slabIdx: slabIdx,
				depth:   (midX-cam.X)*camDirX + (midY-cam.Y)*camDirY,
				side:    int8(side),
			}
			arm.lo, arm.hi = 1, 0 // empty span: behind the camera or beside the view
			if visible {
				anyVisible = true
				// An arm wholly beside the view must not be clamped onto the
				// edge column: it would draw its plane's other half there, and
				// a linked panel may not have raised that half.
				if hi >= 0 && lo < screenW {
					arm.lo, arm.hi = clampCol(lo), clampCol(hi)
				}
			}
			arms[armIndex] = arm
			armIndex++
		}
	}
	return arms, armIndex, anyVisible
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
// Returns false without a sprite or projection, so a caller can fall back to the
// billboard; a token off-screen this frame draws nothing and returns true.
// worldLengthOverride (>0) forces the token's world footprint length instead of
// deriving it from the billboard width. Crossed trees use it for their full-tile
// footprint so their arms land on the tile edges. fade (0 = opaque) fades the
// token out as one solid (canUseStandeeVolume, or drawStandeeFadeLayer once
// its end walls show). gold shades the faces as gold
// metal; the zero value keeps the paint.
func (r *Renderer) drawStandeeSprite(screen *ebiten.Image, sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw float64, centerDepth float64, centerSize float64, bottomY float64, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64, fade float32, gold goldMaterial) bool {
	if sprite == nil || centerDepth <= 0 || centerSize <= 0 {
		return false
	}
	slab, ok := r.prepareStandeeSlab(sprite, coreKey, entX, entY, yaw, centerDepth, centerSize, bottomY, rr, gg, bb, mirrorBySide, mirroredIn, worldLengthOverride, r.standeeSurfaces[:0], -1)
	if ok {
		slab.fade = fade
		slab.gold = gold
		r.drawStandeeSlabColumns(screen, slab, -1, -1)
	}
	r.standeeSurfaces = slab.surfaces[:0] // reclaim the backing array (cap grows to the max needed)
	return true
}

// standeeShellCount is the number of core silhouette layers a slab needs at
// the given half-thickness/depth so shells never sit more than
// standeeShellSpacingPx 1920-normalized pixels apart (the die-cut wood rim
// reads solid, not banded). Pure geometry - extracted so a perf diagnostic can
// compute the SAME per-tree cost the renderer will pay without touching any
// texture (see debug_standee_cost_sim_test.go).
func standeeShellCount(halfThicknessWorld float64, screenW int, halfFovTan, centerDepth float64) int {
	thicknessPx := 2 * halfThicknessWorld * float64(screenW) / (2 * halfFovTan * centerDepth)
	shells := int(thicknessPx/standeeShellSpacing(screenW)) + 1
	if shells < 2 {
		shells = 2
	}
	if shells > standeeMaxShells {
		shells = standeeMaxShells
	}
	return shells
}

// standeeShellSpacing is the screen distance between neighbouring shells.
func standeeShellSpacing(screenW int) float64 {
	if screenW > standeeShellRefWidth {
		return standeeShellSpacingPx * float64(screenW) / standeeShellRefWidth
	}
	return standeeShellSpacingPx
}

// standeeEndWallOpacity keeps the end walls where the shells cannot stand in
// for them: seen nearly along its plane, a slab's faces are narrower on screen
// than its thickness, and they and the shells are lines with gaps between them.
// The walls are whole while the faces are no wider than the thickness (or two
// shell spacings, for a slab too far away to show either) and gone once they
// are twice as wide.
func standeeEndWallOpacity(faceWidth, thicknessWidth float64, screenW int) float32 {
	reach := max(thicknessWidth, 2*standeeShellSpacing(screenW))
	return float32(1 - max(0, min(1, (faceWidth-reach)/reach)))
}

// prepareStandeeSlab builds a token slab for one yaw into dst (a reused
// surface buffer): the far/near stickers with the wood shells between them, plus
// the unclipped screen span and the billboard metrics. ok=false means the slab
// projects fully off-screen (nothing to draw). The returned slab's `surfaces`
// aliases dst (grown), so the caller reclaims it after drawing. See
// drawStandeeSprite's doc for the parameter meanings.
// coreLayers < 0 keeps projected-thickness sampling; zero keeps outer faces only.
func (r *Renderer) prepareStandeeSlab(sprite *ebiten.Image, coreKey standeeCoreKey, entX, entY, yaw, centerDepth float64, centerSize, bottomY float64, rr, gg, bb float32, mirrorBySide, mirroredIn bool, worldLengthOverride float64, dst []standeeSurface, coreLayers int) (standeeSlab, bool) {
	sprite = r.boundedStandeeRenderSource(coreKey, sprite)
	if sprite == nil {
		return standeeSlab{}, false
	}
	screenW := r.game.worldWidth()
	cam := r.game.camera
	halfFovTan := r.cameraBasis().halfFovTan

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
	// silhouette shells (shell texturing) at constant angular spacing, so at
	// any viewing angle the rim reads as solid die-cut wood, not a plane.
	shells := coreLayers
	if shells < 0 {
		shells = standeeShellCount(h, screenW, halfFovTan, centerDepth)
	}
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
	fx0f, _, fok0 := r.game.renderHelper.projectToScreenXF(far.p0x, far.p0y)
	fx0 := int(fx0f)
	fx1f, _, fok1 := r.game.renderHelper.projectToScreenXF(far.p0x+far.dx, far.p0y+far.dy)
	fx1 := int(fx1f)
	nx0f, _, nok0 := r.game.renderHelper.projectToScreenXF(near.p0x, near.p0y)
	nx0 := int(nx0f)
	nx1f, _, nok1 := r.game.renderHelper.projectToScreenXF(near.p0x+near.dx, near.p0y+near.dy)
	nx1 := int(nx1f)
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

	// Use unrounded endpoints: integer differences oscillate as the camera
	// moves even when the actual parallax changes smoothly. Mip alpha makes
	// hidden layers affect the crown, so fade them instead of dropping them.
	sideFade := float32(0)
	faceWidth, parallax := math.Inf(1), 0.0
	if fok0 && fok1 && nok0 && nok1 {
		parallax = math.Max(math.Abs(fx0f-nx0f), math.Abs(fx1f-nx1f))
		sideFade = standeeSideFade(parallax)
		faceWidth = math.Max(math.Abs(fx1f-fx0f), math.Abs(nx1f-nx0f))
	}
	slab := standeeSlab{
		surfaces:       surfaces,
		minX:           minX,
		maxX:           maxX,
		faceWidth:      faceWidth,
		thicknessWidth: parallax,
		centerSize:     centerSize,
		centerDepth:    centerDepth,
		bottomY:        bottomY,
		rr:             rr,
		gg:             gg,
		bb:             bb,
	}
	slab.thin(sideFade)
	return slab, true
}

// showsU reports whether the slab shows the point u (0 to 1 along its
// surfaces from their start); the halves meet at 0.5 without overlap.
func (s standeeSlab) showsU(u float64) bool {
	switch s.half {
	case 1:
		return u >= 0.5
	case -1:
		return u < 0.5
	}
	return true
}

// raisedSpan is the part of its surfaces the slab shows, 0 to 1 along them
// from their start.
func (s standeeSlab) raisedSpan() (float64, float64) {
	switch s.half {
	case 1:
		return 0.5, 1
	case -1:
		return 0, 0.5
	}
	return 0, 1
}

// thin hides the side layers by amount (0 = full thickness, 1 = only the
// front face, whose hidden layers are skipped); the thinner request wins.
func (s *standeeSlab) thin(amount float32) {
	s.sideFade = max(s.sideFade, amount)
	if s.sideFade == 1 {
		s.firstSurface = len(s.surfaces) - 1
	}
}

// standeeSideFade removes subpixel thickness continuously, independently of
// the projected pixel grid. Full thickness is retained above two pixels.
func standeeSideFade(parallax float64) float32 {
	t := max(0.0, min(1.0, (parallax-0.5)/1.5))
	return float32(1 - t*t*(3-2*t))
}

// standeeProjectedFootprint keeps the isotropic mip no coarser than the less
// compressed axis. A grazing face may be subpixel-wide but hundreds of pixels
// tall: using its width to reduce both axes smears the wood above the crown.
// The shared sticker sampler handles the remaining major-axis footprint.
func standeeProjectedFootprint(projectedWidth, projectedHeight, textureWidth, textureHeight float64) float32 {
	footprintX, footprintY := standeeAxisFootprints(projectedWidth, projectedHeight, textureWidth, textureHeight)
	footprint := math.Min(footprintX, footprintY)
	if footprint < 1 {
		return 1
	}
	return float32(footprint)
}

func standeeAxisFootprints(projectedWidth, projectedHeight, textureWidth, textureHeight float64) (float64, float64) {
	// A collapsed axis still needs a finite footprint for shader sampling.
	// The other axis determines how much isotropic reduction is appropriate.
	return textureWidth / math.Max(projectedWidth, 1e-6), textureHeight / math.Max(projectedHeight, 1e-6)
}

// canUseStandeeVolume: crosses take the one-pass compositor for speed, a fading
// token because only the resolved solid fades without its shells showing
// through. The compositor carries one brightness, so a tinted token stays on
// the material path.
func canUseStandeeVolume(slab standeeSlab) bool {
	// Zero/one core layer intentionally keeps the material path. Fewer draw
	// calls alone do not prove the volume shader cheaper for such small stacks;
	// change this threshold only after a dense-scene GPU comparison.
	return (slab.volumeComposite || slab.fade > 0) &&
		slab.firstSurface == 0 && slab.sideFade == 0 &&
		slab.rr == slab.gg && slab.gg == slab.bb &&
		len(slab.surfaces)-2 >= standeeVolumeMinShells
}

// standeeVolumeEdge is one screen column of a volume strip: the far and near
// faces' reciprocal depth and source U over depth, and the wall clip. All are
// affine in screen X.
type standeeVolumeEdge struct {
	farInv, nearInv, farUQ, nearUQ, clipInv, clipTop float64
}

// toward is the edge a fraction t of the way to o; t may run past either end.
func (e standeeVolumeEdge) toward(o standeeVolumeEdge, t float64) standeeVolumeEdge {
	return standeeVolumeEdge{
		e.farInv + (o.farInv-e.farInv)*t, e.nearInv + (o.nearInv-e.nearInv)*t,
		e.farUQ + (o.farUQ-e.farUQ)*t, e.nearUQ + (o.nearUQ-e.nearUQ)*t,
		e.clipInv + (o.clipInv-e.clipInv)*t, e.clipTop + (o.clipTop-e.clipTop)*t,
	}
}

// sourceU is the faces' source U at this column.
func (e standeeVolumeEdge) sourceU() (far, near float64) {
	if e.farInv <= 0 || e.nearInv <= 0 {
		return 0, 1
	}
	return e.farUQ / e.farInv, e.nearUQ / e.nearInv
}

func (r *Renderer) drawStandeeSlabVolume(screen *ebiten.Image, slab standeeSlab, minX, maxX int, stickerMips, coreMips *mipChain, mipLevel, nextMipLevel, coreMipLevel int, mipBlend float32, filtered bool, trim standeeTrim) bool {
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

	screenW := r.game.worldWidth()
	screenH := r.game.worldHeight()
	cam := r.game.camera
	viewDistance := cam.ViewDist
	if viewDistance <= standeeMinDepth {
		return false
	}
	basis := r.cameraBasis()
	dirX, dirY := basis.dirX, basis.dirY
	planeX, planeY := basis.planeX, basis.planeY
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
	projection := slab.projection(r.game.viewHorizon())

	vertices := r.standeeVerts[:0]
	indices := r.standeeMaterialIdx[:0]
	f0Depth, f0U, fok0 := intersection(far, float64(minX))
	n0Depth, n0U, nok0 := intersection(near, float64(minX))
	if !fok0 || !nok0 {
		return false
	}
	farEnd, _, farOK := intersection(far, float64(maxX+1))
	nearEnd, _, nearOK := intersection(near, float64(maxX+1))
	if !farOK || !nearOK {
		return false
	}
	spanWidth := float64(maxX + 1 - minX)
	farInv, nearInv := 1/f0Depth, 1/n0Depth
	farInvStep := (1/farEnd - farInv) / spanWidth
	nearInvStep := (1/nearEnd - nearInv) / spanWidth
	heightScale := float32(projection.heightScale)
	bottomScale := float32(projection.bottomScale)
	// The unfiltered case also needs a margin above the integer shell count.
	filterData := float32(0.0625)
	if filtered {
		filterData = 0.25 + 0.5*mipBlend
	}
	// The half drawn rides above the shell count, in the shader's art U.
	half := standeeVolumeWhole
	if slab.half != 0 {
		half = standeeVolumeLowerHalf
		if slab.half > 0 != near.mirrored {
			half = standeeVolumeUpperHalf
		}
	}
	packedShells := float32(len(slab.surfaces)-2) + float32(standeeVolumeHalfStride*half) + filterData
	depthBuffer := r.game.depthBuffer
	wallTopBuffer := r.game.wallTopBuffer
	sourceOrigin := stickerMips.levels[0].Bounds().Min
	// Ignore walls wholly behind both faces, then coalesce affine shader
	// clipping inputs. Reciprocal depth is affine across the screen, so its
	// endpoint minimum conservatively bounds every layer throughout a column.
	// A small margin keeps floating-point ties on the original clipping path.
	wallAt := func(x int) (clip standeeClipSample) {
		clip = standeeClipSample{state: standeeClipClear, inverseDepth: 1 / viewDistance}
		if x >= 0 && x < len(depthBuffer) {
			if d := depthBuffer[x]; d > 0 && d < viewDistance {
				offset := float64(x - minX)
				minInv := min(farInv+farInvStep*offset, farInv+farInvStep*(offset+1),
					nearInv+nearInvStep*offset, nearInv+nearInvStep*(offset+1))
				if float64(float32(d))*minInv > 1.00001 {
					return
				}
				clip.state, clip.inverseDepth = standeeClipWall, 1/d
				if x < len(wallTopBuffer) {
					clip.top = min(max(wallTopBuffer[x], 0), float64(screenH))
				}
			}
		}
		return
	}
	emit := func(x, end int, leftClip, rightClip standeeClipSample) bool {
		f1Depth, f1U, fok1 := intersection(far, float64(end))
		n1Depth, n1U, nok1 := intersection(near, float64(end))
		if !fok1 || !nok1 {
			r.standeeVerts = vertices[:0]
			r.standeeMaterialIdx = indices[:0]
			return false
		}
		if math.Max(math.Max(f0U, f1U), math.Max(n0U, n1U)) < 0 ||
			math.Min(math.Min(f0U, f1U), math.Min(n0U, n1U)) > 1 {
			f0Depth, f0U, n0Depth, n0U = f1Depth, f1U, n1Depth, n1U
			return true
		}

		// Each projected edge is linear. The endpoint union conservatively
		// encloses both faces; sourcePosition rejects any uncovered pixels.
		// Reciprocal depth, U over depth and the wall inputs are affine in
		// screen X, so a piece's inner edges interpolate the run's.
		left := standeeVolumeEdge{1 / f0Depth, 1 / n0Depth, mirrorU(f0U) / f0Depth, mirrorU(n0U) / n0Depth, leftClip.inverseDepth, leftClip.top}
		right := standeeVolumeEdge{1 / f1Depth, 1 / n1Depth, mirrorU(f1U) / f1Depth, mirrorU(n1U) / n1Depth, rightClip.inverseDepth, rightClip.top}
		width := float64(end - x)
		vertex := func(px int, e standeeVolumeEdge) ebiten.Vertex {
			return ebiten.Vertex{
				DstX: float32(px), SrcX: float32(sourceOrigin.X) + heightScale, SrcY: float32(sourceOrigin.Y) + bottomScale,
				Custom0: float32(e.farInv), Custom1: float32(e.nearInv),
				Custom2: float32(e.farUQ), Custom3: float32(e.nearUQ),
				ColorR: slab.rr, ColorG: float32(e.clipInv), ColorB: float32(e.clipTop), ColorA: packedShells,
			}
		}
		pieces := trim.pieces(end - x)
		trimmed := pieces > 0
		pieces = max(pieces, 1)
		for piece := 0; piece < pieces; piece++ {
			xa, xb := x+(end-x)*piece/pieces, x+(end-x)*(piece+1)/pieces
			ea, eb := left, right
			if piece > 0 {
				ea = left.toward(right, float64(xa-x)/width)
			}
			if piece < pieces-1 {
				eb = left.toward(right, float64(xb-x)/width)
			}
			fTopA, fBottomA, fHeightA := projection.atInverseDepth(ea.farInv)
			nTopA, nBottomA, nHeightA := projection.atInverseDepth(ea.nearInv)
			fTopB, fBottomB, fHeightB := projection.atInverseDepth(eb.farInv)
			nTopB, nBottomB, nHeightB := projection.atInverseDepth(eb.nearInv)
			topA, bottomA := min(fTopA, nTopA), max(fBottomA, nBottomA)
			topB, bottomB := min(fTopB, nTopB), max(fBottomB, nBottomB)
			if trimmed {
				// Every layer's U lies between the faces', a margin out.
				before := left.toward(right, (float64(xa-x)-standeeTrimMargin)/width)
				after := left.toward(right, (float64(xb-x)+standeeTrimMargin)/width)
				u0, u1 := before.sourceU()
				u2, u3 := after.sourceU()
				vTop, vBottom, ok := trim.span(min(u0, u1, u2, u3), max(u0, u1, u2, u3), float64(min(fHeightA, nHeightA, fHeightB, nHeightB)))
				if !ok {
					continue
				}
				// Rows of the art move with depth; the faces bound the layers.
				// Art that reaches the frame's edge keeps that edge to the
				// bit: a recomputed one could move a pixel centre lying on it
				// to the other side.
				if top := float32(vTop); top > 0 {
					topA, topB = min(fTopA+top*fHeightA, nTopA+top*nHeightA), min(fTopB+top*fHeightB, nTopB+top*nHeightB)
				}
				if bottom := float32(vBottom); bottom < 1 {
					bottomA, bottomB = max(fTopA+bottom*fHeightA, nTopA+bottom*nHeightA), max(fTopB+bottom*fHeightB, nTopB+bottom*nHeightB)
				}
			}
			base := uint32(len(vertices))
			va, vb := vertex(xa, ea), vertex(xb, eb)
			va.DstY, vb.DstY = topA, topB
			vertices = append(vertices, va, vb)
			va.DstY, vb.DstY = bottomA, bottomB
			vertices = append(vertices, va, vb)
			indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
		}
		f0Depth, f0U, n0Depth, n0U = f1Depth, f1U, n1Depth, n1U
		return true
	}
	run := standeeClipRun{start: minX, first: wallAt(minX)}
	for x := minX + 1; x <= maxX; x++ {
		clip := wallAt(x)
		if clip.state != standeeClipWall && clip.state == run.first.state {
			continue
		}
		if !run.extend(x, clip) {
			if !run.emit(x, emit) {
				return false
			}
			run = standeeClipRun{start: x, first: clip}
		}
	}
	if !run.emit(maxX+1, emit) {
		return false
	}
	if len(indices) == 0 {
		r.standeeVerts = vertices[:0]
		r.standeeMaterialIdx = indices[:0]
		return true
	}

	opts := &r.standeeVolumeOpts
	if opts.Uniforms == nil {
		opts.Uniforms = map[string]any{"Horizon": make([]float32, 1), "Fade": make([]float32, 1)}
	}
	opts.Uniforms["Horizon"].([]float32)[0] = float32(r.game.viewHorizon())
	opts.Uniforms["Fade"].([]float32)[0] = slab.fade
	setStandeeGold(opts, slab.gold)
	opts.Blend = ebiten.BlendSourceOver
	opts.Images[0] = stickerMips.levels[0]
	opts.Images[1] = stickerMips.levels[mipLevel]
	opts.Images[2] = stickerMips.levels[nextMipLevel]
	opts.Images[3] = coreMips.levels[coreMipLevel]
	worldDrawColumnShader32(screen, vertices, indices, shader, opts)
	r.statStandeeCalls++
	r.statStandeeVertices += len(vertices)
	r.standeeVerts = vertices[:0]
	r.standeeMaterialIdx = indices[:0]
	return true
}

// drawStandeeSlabColumns rasterizes a prepared slab, optionally narrowed to
// [clipMinX,clipMaxX] (-1 disables - crossed trees split the draw at the planes'
// crossover column so each arm draws far->near). All far sticker, core-shell
// and near-sticker triangles are submitted in painter order through one shader
// call. Wide perspective-correct strips avoid rebuilding geometry per column;
// narrow clips and near-plane cases retain individual column intersections.
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
	if slab.flipBlend > 0 && slab.fade == 0 {
		r.drawStandeeFlipBlend(screen, slab, minX, maxX)
		return
	}

	screenW := r.game.worldWidth()
	projection := slab.projection(r.game.viewHorizon())
	cam := r.game.camera
	basis := r.cameraBasis()
	camDirX, camDirY := basis.dirX, basis.dirY
	planeX, planeY := basis.planeX, basis.planeY

	depthBuf := r.game.depthBuffer
	wallTopBuf := r.game.wallTopBuffer
	occlusion := r.standeeWallOcclusion

	centerSize := slab.centerSize
	centerDepth := slab.centerDepth
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
	gx, gy := worldPixelScale(screen, float64(minX+maxX)*.5, slab.bottomY-slab.centerSize*.5)
	projectedWidth *= gx
	projectedHeight *= gy
	filtered := standeeUsesMinificationSampling(projectedWidth, projectedHeight, texW, texH)
	mipLevel, mipBlend := 0, float32(0)
	if filtered {
		mipLevel, mipBlend = mipLevelBlend(
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
	// The vertices carry the horizontal scale; the shader takes the vertical
	// one from the pixel rows, picks the major axis and keeps every magnified
	// axis sharp.
	footprintX, footprintY := standeeAxisFootprints(projectedWidth, projectedHeight, texW, texH)
	horizontalFootprint := float32(footprintX)
	filteredFlag := float32(0)
	if filtered {
		filteredFlag = 1
	}
	// Very thin faces retain per-surface material coverage. That path chooses
	// spans or individual columns independently from this projected-width gate.
	trim := newStandeeTrim(stickerMips, coreMips, mipLevel, nextMipLevel, coreMipLevel, filtered)
	// Every token shows its ends when seen along its plane. Decided before a
	// fade thins the slab, so a fading token keeps its thickness to the end.
	// The walls are part of the thickness: a slab built as its outer faces
	// alone (a crossed_standee_layers band of 0) has none.
	var endWalls []standeeEndWall
	endWallOpacity := (1 - slab.sideFade) * (1 - slab.fade) * standeeEndWallOpacity(slab.faceWidth, slab.thicknessWidth, screenW)
	var endRows *standeeLevelRows
	if len(stickerMips.rows) > 0 {
		endRows = &stickerMips.rows[0]
	}
	if endWallOpacity > 0 && endRows != nil && endRows.endTone[3] > 0 && len(slab.surfaces) > 2 {
		endWalls = r.visibleStandeeEndWalls(slab, endRows, cam.X, cam.Y)
	}
	// A fading token's faces and end walls overlap, and fading each would
	// stack their alpha: draw it opaque into a layer and fade that once.
	if slab.fade > 0 && len(endWalls) > 0 {
		r.drawStandeeFadeLayer(screen, slab, minX, maxX)
		return
	}
	// End walls go between the shells and the near sticker, which the
	// one-pass compositor cannot interleave: a slab showing them takes the
	// material path. They show only near edge-on, over a few columns.
	drewVolume := len(endWalls) == 0 && projectedWidth >= 2 && canUseStandeeVolume(slab) &&
		r.drawStandeeSlabVolume(screen, slab, minX, maxX, stickerMips, coreMips, mipLevel, nextMipLevel, coreMipLevel, mipBlend, filtered, trim)
	if drewVolume {
		return
	}
	if slab.fade > 0 {
		// Translucent layers drawn one by one would stack into an opaque body:
		// without the compositor a fading token is its front face.
		slab.thin(1)
	}

	shader, err := r.ensureStandeeTrilinearShader()
	if err != nil {
		return
	}
	opts := &r.standeeTrilinearOpts
	// Reduced levels draw from the token's sheet: its shader reads no image
	// region, so successive tokens merge into one GPU command.
	var sheet *standeeSheet
	if filtered && mipLevel >= 1 && coreMipLevel == mipLevel && projection.heightScale > 0 && !math.IsInf(projection.heightScale, 0) {
		blended := mipLevel
		if mipBlend > 0 {
			blended = nextMipLevel
		}
		if s := r.standeeSheets[sticker.mipKey.frame]; s.holds(stickerMips, coreMips, mipLevel, blended) {
			if _, err := r.ensureStandeeSheetShader(); err == nil {
				sheet = s
			}
		}
	}
	if sheet != nil {
		shader, opts = r.standeeSheetShader, &r.standeeSheetOpts
		if opts.Uniforms == nil {
			opts.Uniforms = map[string]any{"Horizon": make([]float32, 1)}
		}
		opts.Uniforms["Horizon"].([]float32)[0] = float32(projection.horizon)
		opts.Images[0] = sheet.image
	} else {
		opts.Images[0] = stickerMips.levels[0]
		opts.Images[1] = stickerMips.levels[mipLevel]
		opts.Images[2] = stickerMips.levels[nextMipLevel]
		opts.Images[3] = coreMips.levels[coreMipLevel]
	}
	setStandeeGold(opts, slab.gold)
	opts.Blend = ebiten.BlendSourceOver

	if filtered {
		trim = trim.reaching(footprintX, footprintY, stickerBounds.Size(), stickerMips.levels[mipLevel].Bounds().Size())
	}
	verts := r.standeeVerts[:0]
	idx := r.standeeMaterialIdx[:0]
	rayAt := func(screenX float64) (float64, float64) {
		return standeeRayAtScreenX(screenX, screenW, camDirX, camDirY, planeX, planeY)
	}
	geometryAt := func(t float64) (top, bottom, height float32) {
		return projection.atInverseDepth(1 / t)
	}

	// drawSurface adds one surface over columns lo..hi. tone scales its colour;
	// wall, when set, limits it to the rows of that end wall and to the columns
	// it projects to.
	drawSurface := func(sf standeeSurface, opacity, layerMode float32, tone [3]float32, wall *standeeEndWall, lo, hi int) {
		cr := slab.rr * sf.shade * opacity * tone[0]
		cg := slab.gg * sf.shade * opacity * tone[1]
		cb := slab.bb * sf.shade * opacity * tone[2]
		material := ebiten.Vertex{ColorR: cr, ColorG: cg, ColorB: cb, ColorA: opacity,
			Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag, Custom3: horizontalFootprint}
		if sheet != nil {
			// standee_sheet.kage layout: the block origin carries the surface
			// and the blend in its fractions, away from the integer boundaries.
			block := sheet.blocks[mipLevel]
			material.SrcX = float32(block.X) + 0.25 + layerMode/8
			material.SrcY = float32(block.Y) + 0.125 + 0.75*mipBlend
			material.Custom2 = float32(projection.bottomScale / projection.heightScale)
		}
		v0, v1 := float32(0), float32(1)
		if wall != nil {
			v0, v1 = wall.v0, wall.v1
			xa, ta, okA := r.game.renderHelper.projectToScreenXF(sf.p0x, sf.p0y)
			xb, tb, okB := r.game.renderHelper.projectToScreenXF(sf.p0x+sf.dx, sf.p0y+sf.dy)
			if okA && okB {
				left, right := int(math.Floor(min(xa, xb))), int(math.Ceil(max(xa, xb)))
				// A wall is planar and a few pixels wide: one quad between its
				// two edges, unless it leaves the draw's columns (a cross arm)
				// or a world wall may clip one of them.
				whole := left >= lo && right <= hi+1
				lo, hi = max(lo, left-1), min(hi, right)
				for x := max(lo, 0); whole && x <= hi && x < len(depthBuf); x++ {
					whole = !standeeColumnOccluded(max(ta, tb), depthBuf[x], occlusion.depthAllowance)
				}
				if whole {
					base := uint32(len(verts))
					for _, edge := range [2]struct{ x, t float64 }{{xa, ta}, {xb, tb}} {
						top, bottom, height := geometryAt(edge.t)
						runBottom := top + v1*height
						if v1 >= 1 {
							runBottom = bottom
						}
						// The sheet shader finds the row from the height; the
						// material shader reads it from SrcY. U is unused.
						corner := material
						corner.DstX = float32(edge.x)
						if sheet != nil {
							corner.Custom0, corner.Custom1 = height, 0
							corner.DstY = top + v0*height
							verts = append(verts, corner)
							corner.DstY = runBottom
							verts = append(verts, corner)
							continue
						}
						corner.SrcX = 0.5
						corner.DstY, corner.SrcY = top+v0*height, 0.5+(float32(texH)-1)*v0
						verts = append(verts, corner)
						corner.DstY, corner.SrcY = runBottom, 0.5+(float32(texH)-1)*v1
						verts = append(verts, corner)
					}
					idx = append(idx, base, base+2, base+1, base+2, base+3, base+1)
					return
				}
			}
		} else {
			var spanned bool
			verts, idx, spanned = r.appendStandeeSurfaceSpans(verts, idx, slab, sf, lo, hi, material, stickerMips.levels[0].Bounds().Min, sheet != nil, trim)
			if spanned {
				return
			}
		}
		textureX := func(u float64) float32 {
			return float32(math.Min(standeeSourceU(u, sf.mirrored)*texW, texW-1)) + 0.5
		}
		for x := lo; x <= hi; x++ {
			rcx, rcy := rayAt(float64(x) + 0.5)
			t, u, ok := standeeColumnHit(cam.X, cam.Y, rcx, rcy, sf.p0x, sf.p0y, sf.dx, sf.dy)
			// A wall's u runs across the thickness; its rows are cut already.
			if !ok || wall == nil && !slab.showsU(u) {
				continue
			}

			top, bottom, height := geometryAt(t)
			// Geometry and source U describe the same pixel-centre ray. At a
			// grazing angle, averaging the two boundary intersections can map
			// this pixel to a different depth (or beyond the finite surface).
			// Filtering already uses explicit mip levels and a footprint below;
			// it does not require distorted boundary UVs for automatic mip choice.
			// Source coordinates name texel centres, matching textureX. This
			// keeps vertical filtering stable at the texture's top/bottom edges.
			srcY0 := float32(0.5)
			srcY1 := float32(texH) - 0.5
			clipBottom := bottom
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
				clipBottom = min(bottom, wt)
			}
			x0, x1 := float32(x), float32(x+1)
			srcX := textureX(u)
			runTop, runBottom := top+v0*height, top+v1*height
			if v1 >= 1 {
				runBottom = bottom
			}
			drawBottom := min(runBottom, clipBottom)
			if drawBottom <= runTop {
				continue
			}
			srcYtop := srcY0 + (srcY1-srcY0)*v0
			srcYbot := srcY0 + (srcY1-srcY0)*v1
			if drawBottom < runBottom {
				srcYbot = srcY0 + (srcY1-srcY0)*((drawBottom-top)/height)
			}
			if sheet != nil {
				// One column of the span layout: constant height and U.
				column := material
				column.Custom0 = height
				column.Custom1 = float32(projection.heightScale * standeeSourceU(u, sf.mirrored) / t)
				base := uint32(len(verts))
				for _, corner := range [4][2]float32{{x0, runTop}, {x1, runTop}, {x0, drawBottom}, {x1, drawBottom}} {
					column.DstX, column.DstY = corner[0], corner[1]
					verts = append(verts, column)
				}
				idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
				continue
			}
			base := uint32(len(verts))
			verts = append(verts,
				ebiten.Vertex{DstX: x0, DstY: runTop, SrcX: srcX, SrcY: srcYtop, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: opacity, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag, Custom3: horizontalFootprint},
				ebiten.Vertex{DstX: x1, DstY: runTop, SrcX: srcX, SrcY: srcYtop, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: opacity, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag, Custom3: horizontalFootprint},
				ebiten.Vertex{DstX: x0, DstY: drawBottom, SrcX: srcX, SrcY: srcYbot, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: opacity, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag, Custom3: horizontalFootprint},
				ebiten.Vertex{DstX: x1, DstY: drawBottom, SrcX: srcX, SrcY: srcYbot, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: opacity, Custom0: mipBlend, Custom1: layerMode, Custom2: filteredFlag, Custom3: horizontalFootprint},
			)
			idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
		}
	}
	// The end walls, farthest first, go right before the near sticker: every
	// ray to a wall crosses the near face first, so its art covers the walls,
	// while the far face lies behind them. Each covers only the columns it
	// projects to.
	drawEndWalls := func() {
		for i := range endWalls {
			wall := &endWalls[i]
			tone := [3]float32{endRows.endTone[0], endRows.endTone[1], endRows.endTone[2]}
			drawSurface(standeeEndWallSurface(slab, float64(wall.u)), endWallOpacity, standeeLayerEndWall, tone, wall, minX, maxX)
		}
	}
	for surfaceIndex := slab.firstSurface; surfaceIndex < len(slab.surfaces); surfaceIndex++ {
		if surfaceIndex == len(slab.surfaces)-1 {
			drawEndWalls()
		}
		sf := slab.surfaces[surfaceIndex]
		if sf.img == nil {
			continue
		}
		opacity := float32(1)
		if surfaceIndex != len(slab.surfaces)-1 {
			opacity -= slab.sideFade
		}
		layerMode := float32(standeeLayerNearSticker)
		if sf.mipKey.layer == standeeMipCore {
			layerMode = standeeLayerCore
		} else if surfaceIndex != len(slab.surfaces)-1 {
			layerMode = standeeLayerFarSticker
		}
		if lo, hi, ok := r.standeeRaisedColumns(slab, sf, minX, maxX); ok {
			drawSurface(sf, opacity*(1-slab.fade), layerMode, [3]float32{1, 1, 1}, nil, lo, hi)
		}
	}
	if len(idx) > 0 {
		worldDrawColumnShader32(screen, verts, idx, shader, opts)
		r.statStandeeCalls++
		r.statStandeeVertices += len(verts)
	}
	r.standeeVerts = verts[:0]
	r.standeeMaterialIdx = idx[:0]
}

// standeeSourceU is the art U of a surface hit: clamped to the face, flipped
// on a mirrored surface.
func standeeSourceU(u float64, mirrored bool) float64 {
	u = max(0, min(1, u))
	if mirrored {
		return 1 - u
	}
	return u
}

// tileIsNaturalCross reports whether a crossed tile is the natural cross
// (render_type crossed_standee: trees, rocks, dunes). Only these author frame
// WIDTH, take the distance/lane interior budget, canopy shade, earthquake toppling and
// the foliage depth shading; a crossed_prop is a static built object that
// authors visible height like the flat standee it replaced.
func tileIsNaturalCross(tileType world.TileType3D) bool {
	return world.GlobalTileManager != nil &&
		world.GlobalTileManager.GetRenderType(tileType) == config.TileRenderCrossedStandee
}

// crossedTileDrawsAsStandee reports whether a crossed tile takes the crossed
// sprite pass or the flat per-column raycast fallback. trees_as_billboards
// governs TREES only - a flat billboard faces the camera, which is the one
// thing a crossed_prop must never do.
func (r *Renderer) crossedTileDrawsAsStandee(renderType string) bool {
	switch renderType {
	case config.TileRenderCrossedProp:
		return true
	case config.TileRenderCrossedStandee:
		return r != nil && r.game != nil && r.game.config.Graphics.TreesAsBillboards
	}
	return false
}

// crossedProjectedWidth is a crossed entry's on-screen width: a natural cross
// authors it directly (sizeF), a crossed_prop authors HEIGHT so the width
// follows the source aspect. Split and draw both use this, so arm columns can
// never disagree with the drawn slab.
func (r *Renderer) crossedProjectedWidth(s UnifiedSpriteRenderData) float64 {
	return r.crossWidth(s.tileType, s.spriteName, s.sizeF)
}

// crossWidth is the drawn width of a crossed tile whose authored size is
// size: a tree authors its width, a prop-class cross its height, with the
// width following the art's aspect.
func (r *Renderer) crossWidth(tileType world.TileType3D, spriteName string, size float64) float64 {
	if tileIsNaturalCross(tileType) || r.game == nil || r.game.sprites == nil {
		return size
	}
	if spriteName == "" {
		spriteName = treeStandeeSpriteName(tileType)
	}
	sprite := r.game.sprites.GetSprite(spriteName)
	if sprite == nil {
		return size
	}
	return spriteWidthForHeight(size, sprite.Bounds().Dx(), sprite.Bounds().Dy())
}

func treeStandeeSpriteName(tileType world.TileType3D) string {
	if world.GlobalTileManager != nil {
		if name := world.GlobalTileManager.GetSprite(tileType); name != "" {
			return name
		}
	}
	return "tree"
}

// standeeHeightForWidth preserves the source texture aspect when a crossed
// standee's authored size is its projected width. This is shared by trees and
// landmarks so a portrait texture (for example 512x1024) becomes 2x4 rather
// than being stretched into a 2x2 square.
func standeeHeightForWidth(width float64, textureWidth, textureHeight int) float64 {
	if width <= 0 || textureWidth <= 0 || textureHeight <= 0 {
		return width
	}
	return width * float64(textureHeight) / float64(textureWidth)
}

// spriteWidthForHeight is standeeHeightForWidth's twin for the flat billboard
// path, where the authored size is the projected HEIGHT (a prop's visible-height
// class) and the width follows the source aspect. A wide texture stays wide
// instead of being squeezed into one hardcoded ratio for every prop.
func spriteWidthForHeight(height float64, textureWidth, textureHeight int) float64 {
	if height <= 0 || textureWidth <= 0 || textureHeight <= 0 {
		return height
	}
	return height * float64(textureWidth) / float64(textureHeight)
}

// reserveStandeeBuffers allocates the worst normal fallback geometry once at
// map load instead of repeatedly growing it as the party approaches a token.
func (r *Renderer) reserveStandeeBuffers() {
	screenW := r.game.worldWidth()
	// Fading and grazing slabs retain the material path regardless of shell
	// count. Keep its existing warm capacity independent of volume eligibility.
	const maxFallbackSurfaces = 7
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
	for i := range r.standeeSurfacesMore {
		if cap(r.standeeSurfacesMore[i]) < surfaceCapacity {
			r.standeeSurfacesMore[i] = make([]standeeSurface, 0, surfaceCapacity)
		}
	}
}

// drawCrossedTreeStandees keeps both planes at every distance. Split arms and
// the whole-cross fallback share the same centre-based interior budget.
func (r *Renderer) drawCrossedTreeStandees(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if s.treeArmOnly {
		if s.treeArmLo > s.treeArmHi {
			return
		}
		if slab, ok, found := r.crossedGeometry.lookup(s); found {
			if ok {
				r.drawCrossArm(screen, slab, s)
			}
			return
		}
	}
	spriteName := s.spriteName
	if spriteName == "" {
		spriteName = treeStandeeSpriteName(s.tileType)
	}
	// An arm takes its pose and size from its tree's centre: its own depth can
	// lie behind the camera while the arm still reaches into view.
	centerDepth := s.depthPerp
	if s.treeArmOnly {
		centerDepth = s.treeCenterDepth
	}
	sprite := r.game.sprites.GetSprite(spriteName)
	if sprite == nil || s.sizeF <= 0 || centerDepth <= 0 {
		return
	}
	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	dx, dy := worldX-r.game.camera.X, worldY-r.game.camera.Y
	distance := math.Hypot(dx, dy)
	isTree := tileIsNaturalCross(s.tileType)
	coreLayers := -1
	if isTree {
		// Budget is a logical-world decision, not a projection decision: camera
		// interpolation and screen shake must not toggle shells at a boundary.
		pose := r.game.logicalCameraPose()
		budgetDX, budgetDY := worldX-pose.x, worldY-pose.y
		// Unlike cameraSpaceXY, use the logical pose and world-space lateral
		// distance without FOV scaling. The corridor keeps its authored tile width.
		lateral := (-budgetDX*math.Sin(pose.angle) + budgetDY*math.Cos(pose.angle)) / tileSize
		coreLayers = r.game.config.Graphics.Standee.CrossedStandeeLayers.LayerCount(math.Hypot(budgetDX, budgetDY)/tileSize, lateral)
	}

	// Foliage depth shading is for trees; a boiler lights like the flat standee
	// it replaced.
	brightness := r.calculateBrightnessWithTorchLight(worldX, worldY, distance)
	if isTree {
		brightness = r.applyTreeDepthShading(brightness, distance)
	}
	b := float32(brightness)

	// A tree authors its projected WIDTH and height follows the sprite aspect
	// (the platan, 1:2, is twice as tall as the square oak); a prop-class cross
	// authors visible HEIGHT like every other prop and width follows the aspect.
	// Floor anchor unchanged either way, so feet stay grounded.
	widthF := s.sizeF
	heightF := standeeHeightForWidth(s.sizeF, sprite.Bounds().Dx(), sprite.Bounds().Dy())
	if !isTree {
		widthF = r.crossedProjectedWidth(s)
		heightF = s.sizeF
	}
	bottomF := s.bottomF
	key := makeStandeeCoreKey(r.prefixedStandeeKeyName("tree", spriteName), sprite, true)

	// Footprint = the art's OWN projected width, like landmark monuments - a
	// tile-diagonal footprint squeezed the art horizontally (square oak drew
	// ~30% too thin once the square-projection FOV removed the old horizontal
	// stretch that was masking it). Linked panels span their tile instead.
	layout := r.crossLayoutOf(s, r.spriteFootprintWorld(widthF, centerDepth))
	// A cross takes the one-pass volume compositor. A panel ends against the
	// walls and panels beside it, so it keeps the material path, which carries
	// the seam allowance.
	volume := s.panelArms == 0

	// The collector globally sorts all four arms. Cache a prepared plane until
	// its other arm draws, even when other objects render between the arms.
	if s.treeArmOnly {
		// Point-blank, an arm fully behind the camera carries an empty span;
		// skip before paying the slab preparation.
		if s.treeArmLo > s.treeArmHi {
			return
		}
		if s.treeArmSlab < 0 || s.treeArmSlab >= layout.count {
			return
		}
		plane := layout.planes[s.treeArmSlab]
		cache := &r.crossedGeometry
		cacheable := cache.active && cache.used < maxFrameCrossSlabs
		surfaces := r.standeeSurfaces[:0]
		if cacheable {
			surfaces = cache.buffer()
		}
		slab, ok := r.prepareCrossPlane(sprite, key, worldX, worldY, plane, centerDepth, heightF, bottomF, b, surfaces, coreLayers)
		slab.volumeComposite = volume
		if cacheable {
			cache.store(s, slab, ok)
		}
		if ok {
			r.drawCrossArm(screen, slab, s)
		}
		if !cacheable {
			r.standeeSurfaces = slab.surfaces[:0]
		}
		return
	}

	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, layout, s.depthPerp, heightF, bottomF, b, volume, coreLayers)
}

// prepareCrossPlane prepares one plane of a crossed layout. Its art reads
// unmirrored from either side; within cross_flip_blend_deg of edge-on the
// other side's reading mixes in (standeeSlab.flipBlend), since flipping at
// once swaps which end of an uneven silhouette is nearest and the cross would
// jump as the party walks across its plane. Like the end walls, the blend is
// part of the thickness: a plane built as its outer faces alone (a
// crossed_standee_layers band of 0, far or beside the view) flips at once.
// The caller sets the half an arm draws (standeeSlab.half).
func (r *Renderer) prepareCrossPlane(sprite *ebiten.Image, key standeeCoreKey, worldX, worldY float64, plane crossPlane, depthPerp, heightF, bottomF float64, b float32, dst []standeeSurface, coreLayers int) (standeeSlab, bool) {
	slab, ok := r.prepareStandeeSlab(sprite, key, worldX, worldY, plane.yaw, depthPerp, heightF, bottomF, b, b, b, true, false, plane.footprint, dst, coreLayers)
	if len(slab.surfaces) > 2 {
		slab.flipBlend = r.crossFlipBlend(worldX, worldY, plane.yaw)
	}
	return slab, ok
}

// crossFlipBlend is the share of the far side's reading in a plane through
// (x, y) at yaw: from 0 at cross_flip_blend_deg off edge-on to one half on it.
func (r *Renderer) crossFlipBlend(x, y, yaw float64) float32 {
	window := r.game.config.Graphics.Standee.CrossFlipBlendDeg * math.Pi / 180
	cam := r.game.camera
	vx, vy := cam.X-x, cam.Y-y
	dist := math.Hypot(vx, vy)
	if window <= 0 || dist <= 0 {
		return 0
	}
	off := math.Asin(max(-1, min(1, (-math.Sin(yaw)*vx+math.Cos(yaw)*vy)/dist)))
	t := max(0, min(1, (off/window+1)/2))
	near := t * t * (3 - 2*t) // the + side reading's share
	return float32(min(near, 1-near))
}

// drawCrossArm draws one arm of a split cross: its half of its plane's slab,
// over the columns the arm spans. A linked panel is given the door's seam
// allowance, since its ends meet the flanking walls the same way.
func (r *Renderer) drawCrossArm(screen *ebiten.Image, slab standeeSlab, s UnifiedSpriteRenderData) {
	if s.panelArms != 0 {
		r.standeeWallOcclusion = standeeWallOcclusion{depthAllowance: doorDepthAllowanceWorld(float64(r.game.config.GetTileSize()))}
	}
	slab.half = s.treeArmSide
	r.drawStandeeSlabColumns(screen, slab, s.treeArmLo, s.treeArmHi)
	r.standeeWallOcclusion = standeeWallOcclusion{}
}

// drawCrossedSlabs renders the planes of a crossed layout meeting at
// (worldX,worldY), split into centre->end ARMS drawn far->near for exact
// occlusion. Shared by static crossed trees (diagonal yaws), spinning landmark
// monuments (rotating yaws) and linked panels (the arms toward neighbours).
//
// The arms are disjoint in 3D (they meet only on the central axis), so a
// far->near painter's order over them is exact: a nearer arm's opaque pixels
// occlude a farther arm and its transparent pixels reveal it, with no slab
// interpenetration even when one plane is edge-on. (Ordering the two whole
// planes can't: near the axis the slabs interleave and whichever draws last
// wins - the see-through artifact.) Each arm draws its own half of the slab,
// cut in the world (standeeSlab.half): its thickness spreads it over columns of
// the other half seen along the plane. Both arms of a plane share one slab, so each yaw's slab
// is prepared ONCE and reused; the two slabs stay live together for the
// interleaved draw, hence one reused buffer per plane.
func (r *Renderer) drawCrossedSlabs(screen, sprite *ebiten.Image, key standeeCoreKey, worldX, worldY float64, layout crossLayout, depthPerp float64, heightF, bottomF float64, b float32, volumeComposite bool, coreLayers int) {
	slabs := [maxCrossPlanes]standeeSlab{}
	slabOK := [maxCrossPlanes]bool{}
	for i, plane := range layout.planes[:layout.count] {
		buffer := r.standeeSurfaces[:0]
		if i > 0 {
			buffer = r.standeeSurfacesMore[i-1][:0]
		}
		slabs[i], slabOK[i] = r.prepareCrossPlane(sprite, key, worldX, worldY, plane, depthPerp, heightF, bottomF, b, buffer, coreLayers)
		slabs[i].volumeComposite = volumeComposite
	}

	arms, count, allOK := r.crossedStandeeArms(worldX, worldY, layout)
	if !allOK {
		// No arm is in front of the camera, so the arm spans are meaningless:
		// best-effort whole planes, far->near. A plane with one arm raised (a
		// linked panel's corner or end) draws only that half.
		for i := 0; i < layout.count; i++ {
			if !slabOK[i] {
				continue
			}
			slab := slabs[i]
			switch layout.planes[i].arms {
			case [2]bool{true, false}:
				slab.half = 1
			case [2]bool{false, true}:
				slab.half = -1
			}
			r.drawStandeeSlabColumns(screen, slab, -1, -1)
		}
	} else {
		// A handful of arms: an inline insertion sort avoids sort.Slice's
		// reflection/closure allocation in every crossed tree every frame.
		for i := 1; i < count; i++ {
			for j := i; j > 0 && arms[j-1].depth < arms[j].depth; j-- {
				arms[j-1], arms[j] = arms[j], arms[j-1]
			}
		}
		for _, a := range arms[:count] {
			if slabOK[a.slabIdx] {
				slab := slabs[a.slabIdx]
				slab.half = a.side
				r.drawStandeeSlabColumns(screen, slab, a.lo, a.hi)
			}
		}
	}
	// Reclaim the backing arrays (caps grow).
	r.standeeSurfaces = slabs[0].surfaces[:0]
	for i := 1; i < layout.count; i++ {
		r.standeeSurfacesMore[i-1] = slabs[i].surfaces[:0]
	}
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
	drew := r.drawStandeeSprite(screen, sprite, key, wx, wy, wyaw, depthPerp, spriteSize, bottomY, br, br, br, true, false, worldLength, 0, goldMaterial{})
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
	halfFovTan := math.Tan(r.game.viewFOV() / 2)
	footprint := spriteSizePx * 2 * halfFovTan * depthPerp / float64(r.game.worldWidth())
	if footprint <= 0 {
		footprint = float64(r.game.config.GetTileSize()) * math.Sqrt2
	}
	return footprint
}

// drawLandmarkStandee renders a render_type:"landmark_standee" entity (mage tower, church,
// city gate, lich nexus, fountain) as a TALL crossed standee that slowly spins in
// place - the static-token showcase spin, but as a perpendicular cross so it reads
// as a 3D monument from any angle. spinYaw is the showcase yaw the caller already
// computes (so it matches the single-standee spin); the second plane is spinYaw+90deg.
// Height scales by sprite aspect (tall art -> tall monument), feet stay anchored.
func (r *Renderer) drawLandmarkStandee(screen, sprite *ebiten.Image, keyName string, worldX, worldY, spinYaw, depthPerp float64, sizeF, bottomF float64, b float32) bool {
	if sprite == nil || sizeF <= 0 || depthPerp <= 0 {
		return false
	}
	heightF := standeeHeightForWidth(sizeF, sprite.Bounds().Dx(), sprite.Bounds().Dy())
	key := makeStandeeCoreKey(keyName, sprite, true)
	footprint := r.spriteFootprintWorld(sizeF, depthPerp)
	r.drawCrossedSlabs(screen, sprite, key, worldX, worldY, twoPlaneCross(spinYaw, spinYaw+math.Pi/2, footprint), depthPerp, heightF, bottomF, b, false, -1)
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
