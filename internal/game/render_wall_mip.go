package game

import (
	"image"
	"image/draw"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Minified wall slices: a RIPMAP (2D anisotropic mip grid) plus the shared
// LOD/crossover contract from render_mip.go.
//
// Why we pick the level ourselves: a wall slice is a one-pixel-wide, full-height
// quad, and Ebitengine chooses one mip level per DrawTriangles call as the
// MINIMUM over every triangle edge. A near column in the batch - or the barely
// minified vertical edge of a tall quad - drags the whole batch to level 0
// while distant slices cover 5-25 source texels per screen pixel. The result is
// the classic moire: bright and dark bands crawling over brickwork.
//
// Why a ripmap and not a single pyramid: the two axes minify INDEPENDENTLY.
// Walking along a wall at a grazing angle compresses X hard while Y stays
// sharp; approaching a far wall head-on compresses BOTH (a whole tile shrinks
// to ~35px each way at 20 tiles). An isotropic level picked for X smears the
// mortar rows at grazing angles; an X-only chain (shipped once) leaves the far
// head-on wall vertically unfiltered at ~7 texels/pixel, and the brick rows
// alias into light/dark horizontal bands that shimmer during the approach. A
// raycaster wall slice maps screen X to texture U and screen Y to texture V
// independently, so the axis-aligned ripmap is the exact answer, not an
// approximation: levels[iy][ix] halves width ix times and height iy times, and
// each axis picks its own level.
const (
	// wallMipRepeats is how many copies of the tile each level holds. Ebitengine
	// builds no address-repeat for a sampled source rect, so the neighbouring
	// copies are what keep a slice's U interval seam-free when it wraps.
	wallMipRepeats = 3
	// Vertical levels stop at height/2^6: past that a 256px tile is drawn under
	// 4px tall and the residual is invisible. Horizontal levels run down to a
	// single texel column - grazing angles genuinely reach there.
	wallMaxVerticalMipLevels = 6
	// Custom ripmaps are close to 12x the source RGBA payload. Keep their
	// resident share bounded so oversized campaign art falls back to the normal
	// wall path instead of exhausting a small GPU.
	wallRipmapBudgetBytes           int64 = 96 << 20
	wallRipmapPerTextureBudgetBytes int64 = 16 << 20
)

// wallRipmap owns the anisotropic level grid of one wall texture.
type wallRipmap struct {
	levels [][]*ebiten.Image // [iy][ix]
	owned  []*ebiten.Image
	bytes  int64
}

// wallMipBatch is one pending DrawTriangles worth of slices sharing a source.
type wallMipBatch struct {
	source  *ebiten.Image
	verts   []ebiten.Vertex
	indices []uint16
}

// The three batch slots, flushed in order so the layering algebra holds:
// base (opaque), then the X crossover, then the Y crossover on top.
// Composite = ay*L(lx,ly+1) + (1-ay)*(ax*L(lx+1,ly) + (1-ax)*L(lx,ly)) -
// separable trilinear along each axis; the only omitted term is the double
// crossover ax*ay*(L(lx+1,ly+1)-L(lx,ly+1)), which is bounded by both narrow
// blend windows at once and is not visible.
const (
	wallMipSlotBase = iota
	wallMipSlotCrossX
	wallMipSlotCrossY
	wallMipBatchSlots
)

const wallMipBatchVertexLimit = 1 << 16

var wallSliceTriangleIndices = [...]uint16{0, 1, 2, 1, 3, 2}

// wallTextureUsesMipmappedSlice leaves close pixel art on the legacy nearest
// path. Once either source axis is minified, a quad with true source bounds
// lets us select mip levels instead of hopping between full-res columns.
func wallTextureUsesMipmappedSlice(textureWidth, textureHeight, screenWidth int, leftU, rightU, wallHeight float64) bool {
	if screenWidth <= 0 {
		screenWidth = 1
	}
	return math.Abs(rightU-leftU)*float64(textureWidth) > float64(screenWidth) || wallHeight < float64(textureHeight)*0.5
}

// wallSliceFootprint is how many source texels one screen pixel of this slice
// covers horizontally.
func wallSliceFootprint(leftU, rightU, textureWidth float64, screenWidth int) float64 {
	if screenWidth <= 0 {
		screenWidth = 1
	}
	return math.Abs(rightU-leftU) * textureWidth / float64(screenWidth)
}

// wallSliceVerticalFootprint is the same rate along Y: the full texture height
// is always mapped onto the slice's drawn wallHeight pixels.
func wallSliceVerticalFootprint(textureHeight, wallHeight float64) float64 {
	if wallHeight <= 0 {
		return 1
	}
	return textureHeight / wallHeight
}

// wallRipmapSizes lists the level grid for one tile: sizes[iy][ix] halves the
// width ix times and the height iy times, independently.
func wallRipmapSizes(width, height int) [][]image.Point {
	if width <= 0 || height <= 0 {
		return nil
	}
	var rows [][]image.Point
	h := height
	for iy := 0; iy <= wallMaxVerticalMipLevels; iy++ {
		var row []image.Point
		for w := width; ; w /= 2 {
			row = append(row, image.Pt(w, h))
			if w == 1 {
				break
			}
		}
		rows = append(rows, row)
		if h == 1 {
			break
		}
		h /= 2
	}
	return rows
}

func wallRipmapByteSize(width, height int) int64 {
	var total int64
	for _, row := range wallRipmapSizes(width, height) {
		for _, size := range row {
			total += int64(size.X) * int64(wallMipRepeats) * int64(size.Y) * 4
		}
	}
	return total
}

// wallRipmapFor builds (once per wall sprite) the ripmap grid of the tile, each
// level repeated wallMipRepeats times horizontally. Every level is uploaded from
// CPU pixels so it enters Ebitengine as a managed source image and stays
// batchable, unlike an image drawn into another image (which becomes a render
// target).
func (r *Renderer) wallRipmapFor(sprite *ebiten.Image) *wallRipmap {
	return r.wallRipmapForCPU(sprite, nil)
}

// wallRipmapForCPU builds the same resident ripmap while reusing pixels held by
// the streaming loader. Runtime callers may pass nil and retain the legacy
// readback fallback for assets that were not prepared by the loader.
func (r *Renderer) wallRipmapForCPU(sprite *ebiten.Image, prepared *image.RGBA) *wallRipmap {
	if sprite == nil {
		return nil
	}
	if rm := r.wallRipmaps[sprite]; rm != nil {
		return rm
	}
	bounds := sprite.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	estimatedBytes := wallRipmapByteSize(width, height)
	if estimatedBytes <= 0 || estimatedBytes > wallRipmapPerTextureBudgetBytes ||
		estimatedBytes > wallRipmapBudgetBytes-r.wallRipmapBytes {
		if r.wallRipmaps == nil {
			r.wallRipmaps = make(map[*ebiten.Image]*wallRipmap)
		}
		// Cache the fallback decision for this source. Rechecking in Draw after a
		// different region is evicted would otherwise build the ripmap mid-frame.
		r.wallRipmaps[sprite] = &wallRipmap{}
		return nil
	}
	cpuRow := image.NewRGBA(image.Rect(0, 0, width, height))
	if prepared != nil && prepared.Bounds().Dx() == width && prepared.Bounds().Dy() == height {
		draw.Draw(cpuRow, cpuRow.Bounds(), prepared, prepared.Bounds().Min, draw.Src)
	} else {
		sprite.ReadPixels(cpuRow.Pix)
	}

	sizes := wallRipmapSizes(width, height)
	rm := &wallRipmap{levels: make([][]*ebiten.Image, 0, len(sizes))}
	for iy, rowSizes := range sizes {
		if iy > 0 {
			cpuRow = downsampleMip(cpuRow, rowSizes[0])
			if cpuRow == nil {
				break
			}
		}
		cpuLevel := cpuRow
		row := make([]*ebiten.Image, 0, len(rowSizes))
		for ix, size := range rowSizes {
			if ix > 0 {
				cpuLevel = downsampleMip(cpuLevel, size)
				if cpuLevel == nil {
					break
				}
			}
			img := ebiten.NewImageFromImage(tileHorizontally(cpuLevel, wallMipRepeats))
			row = append(row, img)
			rm.owned = append(rm.owned, img)
		}
		if len(row) == 0 {
			break
		}
		rm.levels = append(rm.levels, row)
	}
	if len(rm.levels) == 0 {
		return nil
	}
	if r.wallRipmaps == nil {
		r.wallRipmaps = make(map[*ebiten.Image]*wallRipmap)
	}
	r.wallRipmaps[sprite] = rm
	rm.bytes = estimatedBytes
	r.wallRipmapBytes += rm.bytes
	return rm
}

func (r *Renderer) deallocateWallRipmap(sprite *ebiten.Image) {
	if r == nil || sprite == nil {
		return
	}
	rm := r.wallRipmaps[sprite]
	if rm == nil {
		return
	}
	for _, img := range rm.owned {
		if img != nil {
			img.Deallocate()
		}
	}
	r.wallRipmapBytes -= rm.bytes
	if r.wallRipmapBytes < 0 {
		r.wallRipmapBytes = 0
	}
	delete(r.wallRipmaps, sprite)
}

// clearWallRipmaps releases every generated level at the same residency
// boundary as standee cores and mips. Source sprites belong to SpriteManager
// and remain valid; only the derived ripmap images are owned here.
func (r *Renderer) clearWallRipmaps() {
	if r == nil {
		return
	}
	for _, rm := range r.wallRipmaps {
		if rm == nil {
			continue
		}
		for _, img := range rm.owned {
			if img != nil {
				img.Deallocate()
			}
		}
	}
	r.wallRipmaps = nil
	r.wallRipmapBytes = 0
}

// tileHorizontally repeats an image side by side, so a slice whose U interval
// crosses the tile edge samples real neighbouring pixels instead of clamping.
func tileHorizontally(src *image.RGBA, copies int) *image.RGBA {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, width*copies, height))
	rowBytes := width * 4
	for y := 0; y < height; y++ {
		srcRow := src.Pix[y*src.Stride+bounds.Min.X*4 : y*src.Stride+bounds.Min.X*4+rowBytes]
		for copyIndex := 0; copyIndex < copies; copyIndex++ {
			off := y*dst.Stride + copyIndex*rowBytes
			copy(dst.Pix[off:off+rowBytes], srcRow)
		}
	}
	return dst
}

// wallMipSelection is one slice's resolved ripmap sampling: the base level on
// each axis plus the crossover fraction toward the next level.
type wallMipSelection struct {
	lx, ly int
	ax, ay float32
}

// wallMipSelect picks both axis levels for a slice.
func wallMipSelect(rm *wallRipmap, leftU, rightU, tileWidth float64, width int, textureHeight, wallHeight float64) wallMipSelection {
	var sel wallMipSelection
	maxLy := len(rm.levels) - 1
	maxLx := len(rm.levels[0]) - 1
	sel.lx, sel.ax = mipLevelBlend(float32(wallSliceFootprint(leftU, rightU, tileWidth, width)), maxLx)
	sel.ly, sel.ay = mipLevelBlend(float32(wallSliceVerticalFootprint(textureHeight, wallHeight)), maxLy)
	// Deeper Y rows can be shorter in X (grid rows all share the same length
	// here, but guard the crossover targets anyway).
	if sel.lx+1 > maxLx {
		sel.ax = 0
	}
	if sel.ly+1 > maxLy {
		sel.ay = 0
	}
	return sel
}

// drawMipmappedSpriteWallSlice is the direct fallback for a transparent wall:
// those hits must remain in the ray's back-to-front order, so they cannot join
// the opaque batches. Level selection and layering are identical.
func (r *Renderer) drawMipmappedSpriteWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, width, wallSide int, distance, wallTop, wallHeight, leftU, rightU float64) bool {
	rm, tileWidth, textureHeight, ok := r.wallMipSource(sprite)
	if !ok {
		return false
	}
	sel := wallMipSelect(rm, leftU, rightU, tileWidth, width, textureHeight, wallHeight)
	brightness := r.wallSliceBrightness(screenX, distance, wallSide)
	r.drawWallMipQuad(screen, rm.levels[sel.ly][sel.lx], sel.lx, sel.ly, tileWidth, textureHeight,
		screenX, width, wallTop, wallHeight, leftU, rightU, brightness, 1)
	if sel.ax > 0 {
		r.drawWallMipQuad(screen, rm.levels[sel.ly][sel.lx+1], sel.lx+1, sel.ly, tileWidth, textureHeight,
			screenX, width, wallTop, wallHeight, leftU, rightU, brightness, float64(sel.ax))
	}
	if sel.ay > 0 {
		r.drawWallMipQuad(screen, rm.levels[sel.ly+1][sel.lx], sel.lx, sel.ly+1, tileWidth, textureHeight,
			screenX, width, wallTop, wallHeight, leftU, rightU, brightness, float64(sel.ay))
	}
	return true
}

func (r *Renderer) drawWallMipQuad(screen *ebiten.Image, source *ebiten.Image, lx, ly int,
	tileWidth, textureHeight float64, screenX, width int, wallTop, wallHeight, leftU, rightU, brightness, alpha float64) {
	vertices := appendWallMipSliceVertices(r.wallSliceVerts[:0],
		tileWidth/float64(int(1)<<lx), tileWidth,
		textureHeight/float64(int(1)<<ly), textureHeight,
		screenX, width, wallTop, wallHeight, leftU, rightU, brightness, alpha)
	r.drawMipmappedWallTriangles(screen, vertices, wallSliceTriangleIndices[:], source)
}

// queueMipmappedSpriteWallSlice gathers independent opaque ray slices that share
// one ripmap level of one texture. Each rectangle retains its original four
// vertices, so batching changes submission cost only, not wall geometry. The
// crossover copies go to slots that always flush after the base one, so a
// blended level lands on top of the level it fades into.
func (r *Renderer) queueMipmappedSpriteWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, width, wallSide int, distance, wallTop, wallHeight, leftU, rightU float64) bool {
	rm, tileWidth, textureHeight, ok := r.wallMipSource(sprite)
	if !ok {
		return false
	}
	sel := wallMipSelect(rm, leftU, rightU, tileWidth, width, textureHeight, wallHeight)
	brightness := r.wallSliceBrightness(screenX, distance, wallSide)

	r.queueWallMipQuad(screen, wallMipSlotBase, rm.levels[sel.ly][sel.lx], sel.lx, sel.ly, tileWidth, textureHeight,
		screenX, width, wallTop, wallHeight, leftU, rightU, brightness, 1)
	if sel.ax > 0 {
		r.queueWallMipQuad(screen, wallMipSlotCrossX, rm.levels[sel.ly][sel.lx+1], sel.lx+1, sel.ly, tileWidth, textureHeight,
			screenX, width, wallTop, wallHeight, leftU, rightU, brightness, float64(sel.ax))
	}
	if sel.ay > 0 {
		r.queueWallMipQuad(screen, wallMipSlotCrossY, rm.levels[sel.ly+1][sel.lx], sel.lx, sel.ly+1, tileWidth, textureHeight,
			screenX, width, wallTop, wallHeight, leftU, rightU, brightness, float64(sel.ay))
	}
	return true
}

func (r *Renderer) queueWallMipQuad(screen *ebiten.Image, slot int, source *ebiten.Image, lx, ly int,
	tileWidth, textureHeight float64, screenX, width int, wallTop, wallHeight, leftU, rightU, brightness, alpha float64) {
	batch := &r.wallMipBatches[slot]
	if batch.source != nil && (batch.source != source || len(batch.verts)+4 > wallMipBatchVertexLimit) {
		// All slots flush together so a crossover quad is never separated from
		// the base quad it must cover.
		r.flushMipmappedWallBatch(screen)
		batch = &r.wallMipBatches[slot]
	}
	if batch.source == nil {
		batch.source = source
	}
	base := uint16(len(batch.verts))
	batch.verts = appendWallMipSliceVertices(batch.verts,
		tileWidth/float64(int(1)<<lx), tileWidth,
		textureHeight/float64(int(1)<<ly), textureHeight,
		screenX, width, wallTop, wallHeight, leftU, rightU, brightness, alpha)
	batch.indices = append(batch.indices, base, base+1, base+2, base+1, base+3, base+2)
}

func (r *Renderer) flushMipmappedWallBatch(screen *ebiten.Image) {
	for slot := range r.wallMipBatches {
		batch := &r.wallMipBatches[slot]
		if len(batch.verts) == 0 {
			batch.source = nil
			continue
		}
		r.drawMipmappedWallTriangles(screen, batch.verts, batch.indices, batch.source)
		batch.source = nil
		batch.verts = batch.verts[:0]
		batch.indices = batch.indices[:0]
	}
}

// wallMipSource returns the sprite's ripmap and the size of ONE tile at level
// (0,0). U/V stay in original-tile units so every wall slice uses the same
// projection contract regardless of the levels it ends up sampling.
func (r *Renderer) wallMipSource(sprite *ebiten.Image) (rm *wallRipmap, tileWidth, textureHeight float64, ok bool) {
	rm = r.wallRipmapFor(sprite)
	if rm == nil || len(rm.levels) == 0 || len(rm.levels[0]) == 0 {
		return nil, 0, 0, false
	}
	bounds := sprite.Bounds()
	tileWidth = float64(bounds.Dx())
	textureHeight = float64(bounds.Dy())
	if tileWidth <= 0 || textureHeight <= 0 {
		return nil, 0, 0, false
	}
	return rm, tileWidth, textureHeight, true
}

func (r *Renderer) wallSliceBrightness(screenX int, distance float64, wallSide int) float64 {
	brightness := r.wallPointBrightness(screenX, distance)
	if wallSide == 1 {
		brightness *= 0.7
	}
	return brightness
}

// appendWallMipSliceVertices emits the axis-aligned source-mapped rectangle for
// one slice. levelTileWidth/levelTexHeight are the tile's dimensions AT THE
// CHOSEN LEVELS; U/V stay in original-tile units, and the middle of the
// wallMipRepeats copies is sampled so an interval reaching past either edge
// still finds real pixels.
//
// The half-texel alignment with the nearest path is HALF A FULL-RES TEXEL on
// each axis at every level (a constant offset in U/V space). Expressing it as
// half a texel of the current level - the obvious "+0.5" - shifts the brick
// pattern by up to an eighth of a tile between levels, so neighbouring level
// bands misalign and the masonry jumps on every level switch during an
// approach. That bug shipped once; TestWallMipPatternPositionIsLevelInvariant
// pins the invariant on both axes.
//
// Colors are premultiplied (matching wallMipTriangleOptions' ColorScaleMode),
// so the crossover alpha scales all four components.
func appendWallMipSliceVertices(vertices []ebiten.Vertex, levelTileWidth, fullTileWidth, levelTexHeight, fullTexHeight float64,
	screenX, width int, wallTop, wallHeight, leftU, rightU, brightness, alpha float64) []ebiten.Vertex {
	halfFullTexelU := 0.5 / fullTileWidth
	halfFullTexelV := 0.5 / fullTexHeight
	leftSourceX := float32(levelTileWidth + (leftU+halfFullTexelU)*levelTileWidth)
	rightSourceX := float32(levelTileWidth + (rightU+halfFullTexelU)*levelTileWidth)
	topSourceY := float32(halfFullTexelV * levelTexHeight)
	bottomSourceY := float32((1 - halfFullTexelV) * levelTexHeight)
	color := float32(brightness * alpha)
	a := float32(alpha)
	return append(vertices,
		ebiten.Vertex{DstX: float32(screenX), DstY: float32(wallTop), SrcX: leftSourceX, SrcY: topSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: a},
		ebiten.Vertex{DstX: float32(screenX + width), DstY: float32(wallTop), SrcX: rightSourceX, SrcY: topSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: a},
		ebiten.Vertex{DstX: float32(screenX), DstY: float32(wallTop + wallHeight), SrcX: leftSourceX, SrcY: bottomSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: a},
		ebiten.Vertex{DstX: float32(screenX + width), DstY: float32(wallTop + wallHeight), SrcX: rightSourceX, SrcY: bottomSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: a},
	)
}

// wallMipTriangleOptions is the draw state for every wall-slice triangle call.
//
//   - DisableMipmaps: the levels are already chosen per slice, and Ebitengine's
//     own per-call guess (the minimum over the quad's edges) would only drag
//     them back to level 0.
//   - ColorScaleModePremultipliedAlpha: our vertex colors are premultiplied.
//     The DrawTriangles DEFAULT is straight alpha, under which the engine
//     multiplies RGB by A a second time - the crossover quad then darkens by
//     its own blend factor and the level-transition zone reads as a shadow
//     crawling along the wall. That bug shipped once; the mode is pinned by
//     TestWallMipTriangleOptions.
func wallMipTriangleOptions() ebiten.DrawTrianglesOptions {
	return ebiten.DrawTrianglesOptions{
		Blend:          ebiten.BlendSourceOver,
		Filter:         ebiten.FilterLinear,
		ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha,
		DisableMipmaps: true,
	}
}

func (r *Renderer) drawMipmappedWallTriangles(screen *ebiten.Image, vertices []ebiten.Vertex, indices []uint16, source *ebiten.Image) {
	r.wallSliceTriOpts = wallMipTriangleOptions()
	screen.DrawTriangles(vertices, indices, source, &r.wallSliceTriOpts)
}
