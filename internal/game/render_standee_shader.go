package game

// Both standee compositors use identical atlas coordinates and filtering.
// Keep the Kage source shared so changes cannot drift between draw paths.
const standeeSamplingShaderSrc = `//kage:unit pixels

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
	scale := size / size0
	q := clamp((p-origin0)*scale, vec2(0.5), size-vec2(0.5))
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

func sampleLinear3(p vec2) vec4 {
	origin0 := imageSrc0Origin()
	size0 := imageSrc0Size()
	size := imageSrc3Size()
	scale := size / size0
	// In pixel mode imageSrc3At preserves image-0 pixel coordinates and only
	// adjusts the backing-atlas origin; it does not scale differently-sized
	// source images. Work in level-local pixels, then pass those coordinates
	// relative to image 0 so the built-in origin adjustment remains correct.
	q := clamp((p-origin0)*scale, vec2(0.5), size-vec2(0.5))
	q0 := q - 0.5
	q1 := q + 0.5
	p0 := q0 + origin0
	p1 := q1 + origin0
	c0 := imageSrc3UnsafeAt(p0)
	c1 := imageSrc3UnsafeAt(vec2(p1.x, p0.y))
	c2 := imageSrc3UnsafeAt(vec2(p0.x, p1.y))
	c3 := imageSrc3UnsafeAt(p1)
	rate := fract(q1)
	return mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y)
}

// Adjacent mip levels share the same source-coordinate convention.
func stickerAt(p vec2, mipBlend float) vec4 {
	lo := sampleLinear1(p)
	if mipBlend <= 0.0 {
		return lo
	}
	return mix(lo, sampleLinear2(p), mipBlend)
}

func nearStickerAt(p vec2, filtered bool, mipBlend float, footprint vec2) vec4 {
	if !filtered {
		return imageSrc0UnsafeAt(p)
	}
	// Four taps extend the isotropic mip along only the compressed axis.
	// Grow the kernel continuously so entering anisotropic sampling cannot pop.
	texel := mix(imageSrc0Size()/imageSrc1Size(), imageSrc0Size()/imageSrc2Size(), mipBlend)
	span := max(footprint-1.5*texel, vec2(0))
	if span.x <= 0.0 && span.y <= 0.0 {
		return stickerAt(p, mipBlend)
	}
	return (stickerAt(p-span*0.375, mipBlend) + stickerAt(p-span*0.125, mipBlend) +
		stickerAt(p+span*0.125, mipBlend) + stickerAt(p+span*0.375, mipBlend)) * 0.25
}
`
