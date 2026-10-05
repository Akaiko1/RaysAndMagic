package game

import (
	"context"
	"fmt"
	"image"

	"ugataima/internal/graphics"
)

// Increment only for algorithm changes. Numeric settings and source pixels
// participate directly, so tuning a limit or palette invalidates old entries.
const floorPixelCacheVersion = "floor-atlas-v1"

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
