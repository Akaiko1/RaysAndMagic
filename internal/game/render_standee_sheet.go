package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// standeeSheet is the one image holding a token's reduced sticker and core
// levels; the chains' levels above 0 are views of it. Ebitengine merges shader
// draws only when their uniforms match, and separate level images differ in the
// region uniforms a shader reads. standee_sheet.kage reads none: it takes the
// block origin from the vertices and the sizes from the header texels, so
// tokens whose sheets share an atlas draw in one GPU command.
//
// Level k >= 1 is a block: a header row (full size, level size), below it the
// sticker and to its right the core. Blocks stack downwards. Level 0 stays a
// separate image: it is the largest allocation, often the sprite itself.
type standeeSheet struct {
	image         *ebiten.Image
	blocks        []image.Point   // header origin of each level; index 0 unused
	sticker, core []*ebiten.Image // level views; index 0 unused
}

const (
	// The header packs each side into 12 bits of an opaque texel.
	standeeSheetMaxSide = 1<<12 - 1
	// A sheet must fit a shared atlas page on every supported device.
	standeeSheetMaxEdge = 2048
)

func standeeSheetHeader(size image.Point) [4]byte {
	return [4]byte{byte(size.X >> 4), byte(size.X&15<<4 | size.Y>>8), byte(size.Y), 0xff}
}

// standeeSheetLayout places the blocks for one chain's level sizes. It reports
// false for sizes the shader cannot derive: every level after the first
// reduced one must halve its predecessor.
func standeeSheetLayout(sizes []image.Point) (blocks []image.Point, size image.Point, ok bool) {
	if len(sizes) < 2 {
		return nil, image.Point{}, false
	}
	blocks = make([]image.Point, len(sizes))
	for k, level := range sizes {
		if level.X <= 0 || level.Y <= 0 || level.X > standeeSheetMaxSide || level.Y > standeeSheetMaxSide {
			return nil, image.Point{}, false
		}
		if k == 0 {
			continue
		}
		if k > 1 && level != image.Pt(max(1, sizes[k-1].X/2), max(1, sizes[k-1].Y/2)) {
			return nil, image.Point{}, false
		}
		blocks[k] = image.Pt(0, size.Y)
		size.X = max(size.X, 2, 2*level.X)
		size.Y += 1 + level.Y
	}
	if size.X > standeeSheetMaxEdge || size.Y > standeeSheetMaxEdge {
		return nil, image.Point{}, false
	}
	return blocks, size, true
}

// newStandeeSheet allocates the sheet for a token's prepared chains and writes
// its headers; the caller writes each level into its view. It returns nil when
// the chains do not share one packable set of sizes, and they keep an image
// per level.
func newStandeeSheet(sticker, core []*image.RGBA) *standeeSheet {
	if len(sticker) != len(core) {
		return nil
	}
	sizes := make([]image.Point, len(sticker))
	for k := range sticker {
		if sticker[k] == nil || core[k] == nil || sticker[k].Bounds().Size() != core[k].Bounds().Size() {
			return nil
		}
		sizes[k] = sticker[k].Bounds().Size()
		// Views take whole rows of pixels.
		if k > 0 && (sticker[k].Stride != 4*sizes[k].X || core[k].Stride != 4*sizes[k].X) {
			return nil
		}
	}
	blocks, size, ok := standeeSheetLayout(sizes)
	if !ok {
		return nil
	}
	sheet := &standeeSheet{
		image:   ebiten.NewImage(size.X, size.Y),
		blocks:  blocks,
		sticker: make([]*ebiten.Image, len(sizes)),
		core:    make([]*ebiten.Image, len(sizes)),
	}
	full := standeeSheetHeader(sizes[0])
	for k := 1; k < len(sizes); k++ {
		reduced := standeeSheetHeader(sizes[k])
		header := append(full[:len(full):len(full)], reduced[:]...)
		graphics.WritePixelsRegion(sheet.image, image.Rect(blocks[k].X, blocks[k].Y, blocks[k].X+2, blocks[k].Y+1), header)
		rect := image.Rectangle{Min: blocks[k].Add(image.Pt(0, 1))}
		rect.Max = rect.Min.Add(sizes[k])
		sheet.sticker[k] = sheet.image.SubImage(rect).(*ebiten.Image)
		sheet.core[k] = sheet.image.SubImage(rect.Add(image.Pt(sizes[k].X, 0))).(*ebiten.Image)
	}
	return sheet
}

// holds reports whether the chains' levels at level, and at next when the
// draw blends into it, are this sheet's views. A slab without a core surface
// passes its sticker chain twice and samples no core.
func (s *standeeSheet) holds(sticker, core *mipChain, level, next int) bool {
	view := func(layer standeeMipLayer, chain *mipChain, k int) bool {
		return k > 0 && k < len(chain.levels) && s.views(layer, k, chain.levels[k])
	}
	if s == nil || !view(standeeMipSticker, sticker, level) || (next != level && !view(standeeMipSticker, sticker, next)) {
		return false
	}
	return core == sticker || view(standeeMipCore, core, level)
}

// views reports whether img is this sheet's view of a layer's level k. A
// token without a sheet has none.
func (s *standeeSheet) views(layer standeeMipLayer, k int, img *ebiten.Image) bool {
	list := s.stickerViews()
	if layer == standeeMipCore {
		list = s.coreViews()
	}
	return k > 0 && k < len(list) && list[k] == img
}

// stickerViews and coreViews are nil for a token without a sheet.
func (s *standeeSheet) stickerViews() []*ebiten.Image {
	if s == nil {
		return nil
	}
	return s.sticker
}

func (s *standeeSheet) coreViews() []*ebiten.Image {
	if s == nil {
		return nil
	}
	return s.core
}

func (r *Renderer) ensureStandeeSheetShader() (*ebiten.Shader, error) {
	err := compileShader(&r.standeeSheetShader, "standee_sheet.kage")
	return r.standeeSheetShader, err
}
