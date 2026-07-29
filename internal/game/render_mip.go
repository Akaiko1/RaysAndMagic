package game

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared mip machinery for every surface that picks its own mip level: standee
// tokens (render_standee.go, isotropic pyramid) and wall slices
// (render_wall_mip.go, anisotropic ripmap grid). Ebitengine
// exposes no way to address its internal mip levels - and for DrawTriangles it
// picks ONE level per call as the minimum over the quad's edges, which a tall
// one-pixel-wide wall slice always drags down to level 0 - so adjacent levels
// must be explicit images we select and cross-fade ourselves.
//
// One LOD contract for all of them: footprint (source texels per screen pixel)
// -> level + crossover fraction, with the same blend window everywhere, so the
// level transition feels identical on monsters, scenery and walls and is tuned
// in one place. Only the pyramid SHAPE differs per surface (see mipSizes*).

const (
	maxMipLevel = 6 // matches Ebitengine's own mipmap depth cap
	// Blend only around the nearest-mip crossover. Outside this band one
	// bilinearly sampled level is already stable, avoiding a second four-tap
	// sample across most distant pixels.
	mipBlendStart = 0.35
	mipBlendEnd   = 0.65
)

// mipChain owns normalized immutable copies of one texture's mip levels.
type mipChain struct {
	levels []*ebiten.Image
	// owned lists only images generated for this chain. Level 0 often aliases
	// immutable SpriteManager art and must survive region-cache eviction.
	owned []*ebiten.Image
}

// mipLevelBlend selects the nearest mip level with a short trilinear crossover.
// The crossover is continuous: its upper endpoint is exactly the next level,
// which is also the pure image selected immediately after the band. Keeping the
// blend narrower than the full octave avoids paying eight texture taps where a
// single stable level is visually indistinguishable.
func mipLevelBlend(footprint float32, maxLevel int) (level int, blend float32) {
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
	if fraction <= mipBlendStart {
		return level, 0
	}
	if fraction >= mipBlendEnd {
		return level + 1, 0
	}
	return level, float32((fraction - mipBlendStart) / (mipBlendEnd - mipBlendStart))
}

// mipSizesUniform halves both axes per level - the isotropic pyramid used by
// standee tokens, whose on-screen footprint shrinks in both directions at once.
// Walls use the 2D ripmap grid instead (wallRipmapSizes).
func mipSizesUniform(width, height int) []image.Point {
	if width <= 0 || height <= 0 {
		return nil
	}
	sizes := make([]image.Point, 0, maxMipLevel+1)
	for level := 0; level <= maxMipLevel; level++ {
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

// downsampleMip builds one premultiplied-alpha area-filtered mip on the CPU.
// Besides making transparent edges correct, CPU construction is important for
// batching: an ebiten.Image drawn into another ebiten.Image becomes a render
// target and is unlikely to share Ebitengine's automatic source atlas. A dense
// tree corridor - or a long distant wall - then turns hundreds of otherwise
// compatible draws into separate GPU commands.
func downsampleMip(src *image.RGBA, size image.Point) *image.RGBA {
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
		// only handles odd terminal dimensions and width-only steps.
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
