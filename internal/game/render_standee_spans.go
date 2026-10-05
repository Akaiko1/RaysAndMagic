package game

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type standeeProjection struct {
	horizon, heightScale, bottomScale float64
}

func (slab standeeSlab) projection(screenHeight int) standeeProjection {
	horizon := float64(screenHeight) / 2
	return standeeProjection{horizon, slab.centerSize * slab.centerDepth, (slab.bottomY - horizon) * slab.centerDepth}
}

func (p standeeProjection) atInverseDepth(inv float64) (top, bottom, height float32) {
	height = float32(p.heightScale * inv)
	bottom = float32(p.horizon + p.bottomScale*inv)
	return bottom - height, bottom, height
}

// appendStandeeSurfaceSpans keeps the material path's painter order, tint and
// filtering, but coalesces columns with the same wall clipping into one quad.
// Reciprocal depth, U/depth and projected vertical edges are affine in screen X;
// the fragment shader reconstructs the same pixel-centre sample from them.
// Near-plane crossings and clips narrower than three columns use the fallback.
func (r *Renderer) appendStandeeSurfaceSpans(vertices []ebiten.Vertex, indices []uint32, slab standeeSlab, sf standeeSurface, minX, maxX int, material ebiten.Vertex, sourceOrigin image.Point) ([]ebiten.Vertex, []uint32, bool) {
	if maxX-minX < 2 {
		return vertices, indices, false
	}
	cam := r.game.camera
	basis := r.cameraBasis()
	w, h := r.game.worldWidth(), r.game.worldHeight()
	intersection := func(x float64) (float64, float64, bool) {
		rx, ry := standeeRayAtScreenX(x, w, basis.dirX, basis.dirY, basis.planeX, basis.planeY)
		return standeeColumnIntersection(cam.X, cam.Y, rx, ry, sf.p0x, sf.p0y, sf.dx, sf.dy)
	}
	d0, u0, ok0 := intersection(float64(minX))
	d1, u1, ok1 := intersection(float64(maxX + 1))
	if !ok0 || !ok1 {
		return vertices, indices, false
	}
	if sf.mirrored {
		u0, u1 = 1-u0, 1-u1
	}
	width := float64(maxX + 1 - minX)
	inv0, uq0 := 1/d0, u0/d0
	invStep, uqStep := (1/d1-inv0)/width, (u1/d1-uq0)/width
	projection := slab.projection(h)
	depths, tops := r.game.depthBuffer, r.game.wallTopBuffer
	occlusion := r.standeeWallOcclusion
	unclipped := float32(math.Inf(1))

	// A negative Custom2 distinguishes strips from the narrow-column fallback.
	// Integers encode the layer/filter flags, the fraction keeps the mip blend.
	// Keep the blend inside each flag interval, away from floor boundaries even
	// when interpolating constant attributes introduces a few ULPs of error.
	material.Custom2 = -(1 + 4*material.Custom1 + 2*material.Custom2 + 0.25 + 0.5*material.Custom0)
	column := func(x int) (float32, bool) {
		offset := float64(x-minX) + 0.5
		inv, uq := inv0+invStep*offset, uq0+uqStep*offset
		if uq < 0 || uq > inv {
			return 0, false
		}
		top, bottom, _ := projection.atInverseDepth(inv)
		if bottom <= 0 || top >= float32(h) {
			return 0, false
		}
		if x < len(depths) && standeeColumnOccluded(1/inv, depths[x], occlusion.depthAllowance) {
			rx, ry := standeeRayAtScreenX(float64(x)+0.5, w, basis.dirX, basis.dirY, basis.planeX, basis.planeY)
			if !occlusion.matchesBackingWall(cam.X, cam.Y, rx, ry, depths[x]) {
				wallTop := float32(tops[x])
				if wallTop <= top || wallTop <= 0 {
					return 0, false
				}
				if wallTop < bottom {
					return wallTop, true
				}
			}
		}
		return unclipped, true
	}
	endpoint := func(x int, clip float32) (ebiten.Vertex, ebiten.Vertex) {
		offset := float64(x - minX)
		inv := inv0 + invStep*offset
		top, bottom, height := projection.atInverseDepth(inv)
		v := material
		v.DstX = float32(x)
		v.DstY = top
		v.SrcX = float32(sourceOrigin.X) + top
		v.SrcY = float32(sourceOrigin.Y) + height
		v.Custom0 = float32(inv)
		v.Custom1 = float32(uq0 + uqStep*offset)
		lo := v
		lo.DstY = bottom
		if clip != unclipped {
			lo.DstY = clip
		}
		return v, lo
	}
	start, activeClip := -1, unclipped
	for x := minX; x <= maxX+1; x++ {
		clip, visible := unclipped, false
		if x <= maxX {
			clip, visible = column(x)
		}
		if start >= 0 && (!visible || clip != activeClip) {
			leftTop, leftBottom := endpoint(start, activeClip)
			rightTop, rightBottom := endpoint(x, activeClip)
			base := uint32(len(vertices))
			vertices = append(vertices, leftTop, rightTop, leftBottom, rightBottom)
			indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
			start = -1
		}
		if visible && start < 0 {
			start, activeClip = x, clip
		}
	}
	return vertices, indices, true
}
