package game

import (
	"context"
	"fmt"
	"image"
	"math"

	"ugataima/internal/graphics"
)

// Increment only for algorithm changes. Numeric settings and source pixels
// participate directly, so tuning a limit or palette invalidates old entries.
const standeePixelCacheVersion = "standee-pixels-v1"
const floorPixelCacheVersion = "floor-atlas-v1"

func prepareCachedStandeePixels(ctx context.Context, cache graphics.PixelCache, cpu *image.RGBA, tint float64) standeePreparedPixels {
	if cpu == nil || cpu.Bounds().Empty() {
		return standeePreparedPixels{}
	}
	if cache.Dir == "" {
		return prepareStandeePixels(cpu, tint, true)
	}
	key := graphics.PixelCacheKey(fmt.Sprintf("%s:tint=%016x:max_pixels=%d:mips=%d:wood=%016x,%016x,%016x",
		standeePixelCacheVersion, math.Float64bits(tint), standeeRenderSourceMaxPixels, maxMipLevel,
		math.Float64bits(standeeWoodTone[0]), math.Float64bits(standeeWoodTone[1]), math.Float64bits(standeeWoodTone[2])), cpu)
	w, h := standeeRenderSourceSize(cpu.Bounds().Dx(), cpu.Bounds().Dy())
	sizes := mipSizesUniform(w, h)
	allSizes := append(append([]image.Point(nil), sizes...), sizes...)
	if images, ok := cache.Load(ctx, key, allSizes); ok {
		n := len(sizes)
		return standeePreparedPixels{sticker: images[0], core: images[n], stickerMips: images[:n], coreMips: images[n:]}
	}
	if ctx.Err() != nil {
		return standeePreparedPixels{}
	}
	prepared := prepareStandeePixels(cpu, tint, true)
	cache.Store(ctx, key, append(append([]*image.RGBA(nil), prepared.stickerMips...), prepared.coreMips...))
	return prepared
}

// Source decoding stays in the existing worker. Cache only the derived atlas,
// so source edits and ordered texture selection always participate in its key.
func prepareCachedFloorAtlas(ctx context.Context, cache graphics.PixelCache, textures []floorTexture) (*image.RGBA, int, int, int) {
	if len(textures) == 0 || cache.Dir == "" {
		return prepareFloorAtlas(textures)
	}
	sources := make([]*image.RGBA, 0, len(textures))
	for _, tex := range textures {
		sources = append(sources, &image.RGBA{Pix: tex.pixels, Stride: tex.width * 4, Rect: image.Rect(0, 0, tex.width, tex.height)})
	}
	key := graphics.PixelCacheKey(fmt.Sprintf("%s:mips=%d", floorPixelCacheVersion, maxFloorMipLevels), sources...)
	w, h, mip, atlasH := floorAtlasLayout(textures)
	if images, ok := cache.Load(ctx, key, []image.Point{{X: w * len(textures), Y: atlasH}}); ok {
		return images[0], w, h, mip
	}
	if ctx.Err() != nil {
		return nil, 0, 0, 0
	}
	pixels, w, h, mip := prepareFloorAtlas(textures)
	cache.Store(ctx, key, []*image.RGBA{pixels})
	return pixels, w, h, mip
}
