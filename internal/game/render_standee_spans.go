package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

type standeeProjection struct {
	horizon, heightScale, bottomScale float64
}

func (slab standeeSlab) projection(horizon float64) standeeProjection {
	return standeeProjection{horizon, slab.centerSize * slab.centerDepth, (slab.bottomY - horizon) * slab.centerDepth}
}

func (p standeeProjection) atInverseDepth(inv float64) (top, bottom, height float32) {
	height = float32(p.heightScale * inv)
	bottom = float32(p.horizon + p.bottomScale*inv)
	return bottom - height, bottom, height
}

// A wall's reciprocal depth and projected top are affine in screen X. Keep
// their double-precision pixel-centre samples until the final GPU vertices.
// States separate hidden surfaces, clear columns and foreground wall clips.
type standeeClipState uint8

const (
	standeeClipHidden standeeClipState = iota
	standeeClipClear
	standeeClipWall
)

type standeeClipSample struct {
	state             standeeClipState
	top, inverseDepth float64
}

// standeeClipRun fits pixel-centre samples without quantizing their heights.
// The caller samples columns directly so clear runs need no callback per pixel.
type standeeClipRun struct {
	start            int
	first            standeeClipSample
	stepTop, stepInv float64
}

func (run *standeeClipRun) extend(x int, next standeeClipSample) bool {
	if next.state != run.first.state {
		return false
	}
	if next.state != standeeClipWall {
		return true
	}
	if x == run.start+1 {
		run.stepTop, run.stepInv = next.top-run.first.top, next.inverseDepth-run.first.inverseDepth
		return true
	}
	offset := float64(x - run.start)
	topError := next.top - (run.first.top + run.stepTop*offset)
	invError := next.inverseDepth - (run.first.inverseDepth + run.stepInv*offset)
	// Tolerate double-precision roundoff, far below one screen pixel.
	return topError >= -1e-7 && topError <= 1e-7 && invError >= -1e-12 && invError <= 1e-12
}

func (run standeeClipRun) emit(end int, emit func(int, int, standeeClipSample, standeeClipSample) bool) bool {
	// Two unequal samples cannot distinguish a slope from a wall corner.
	// Keep those columns separate until at least a third confirms the line.
	if end-run.start == 2 && (run.stepTop != 0 || run.stepInv != 0) {
		second := run.first
		second.top += run.stepTop
		second.inverseDepth += run.stepInv
		return emit(run.start, run.start+1, run.first, run.first) && emit(run.start+1, end, second, second)
	}
	left, right := run.first, run.first
	left.top -= 0.5 * run.stepTop
	left.inverseDepth -= 0.5 * run.stepInv
	right.top += (float64(end-run.start) - 0.5) * run.stepTop
	right.inverseDepth += (float64(end-run.start) - 0.5) * run.stepInv
	return emit(run.start, end, left, right)
}

// appendStandeeSurfaceSpans keeps the material path's painter order, tint and
// filtering, but coalesces columns along a linear wall boundary into one quad.
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
	projection := slab.projection(r.game.viewHorizon())
	depths, tops := r.game.depthBuffer, r.game.wallTopBuffer
	occlusion := r.standeeWallOcclusion

	// A negative Custom2 distinguishes strips from the narrow-column fallback.
	// Integers encode the layer/filter flags, the fraction keeps the mip blend.
	// Keep the blend inside each flag interval, away from floor boundaries even
	// when interpolating constant attributes introduces a few ULPs of error.
	material.Custom2 = -(1 + 4*material.Custom1 + 2*material.Custom2 + 0.25 + 0.5*material.Custom0)
	column := func(x int) standeeClipSample {
		offset := float64(x-minX) + 0.5
		inv, uq := inv0+invStep*offset, uq0+uqStep*offset
		if uq < 0 || uq > inv {
			return standeeClipSample{}
		}
		top, bottom, _ := projection.atInverseDepth(inv)
		if bottom <= 0 || top >= float32(h) {
			return standeeClipSample{}
		}
		if x < len(depths) && standeeColumnOccluded(1/inv, depths[x], occlusion.depthAllowance) {
			rx, ry := standeeRayAtScreenX(float64(x)+0.5, w, basis.dirX, basis.dirY, basis.planeX, basis.planeY)
			if !occlusion.matchesBackingWall(cam.X, cam.Y, rx, ry, depths[x]) {
				wallTop := float32(tops[x])
				if wallTop <= top || wallTop <= 0 {
					return standeeClipSample{}
				}
				if wallTop < bottom {
					return standeeClipSample{state: standeeClipWall, top: tops[x]}
				}
			}
		}
		return standeeClipSample{state: standeeClipClear}
	}
	endpoint := func(x int, clip standeeClipSample) (ebiten.Vertex, ebiten.Vertex) {
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
		if clip.state == standeeClipWall {
			lo.DstY = float32(clip.top)
		}
		return v, lo
	}
	emit := func(start, end int, left, right standeeClipSample) bool {
		if left.state == standeeClipHidden {
			return true
		}
		leftTop, leftBottom := endpoint(start, left)
		rightTop, rightBottom := endpoint(end, right)
		base := uint32(len(vertices))
		vertices = append(vertices, leftTop, rightTop, leftBottom, rightBottom)
		indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
		return true
	}
	run := standeeClipRun{start: minX, first: column(minX)}
	for x := minX + 1; x <= maxX; x++ {
		clip := column(x)
		if clip.state != standeeClipWall && clip.state == run.first.state {
			continue
		}
		if !run.extend(x, clip) {
			run.emit(x, emit)
			run = standeeClipRun{start: x, first: clip}
		}
	}
	run.emit(maxX+1, emit)
	return vertices, indices, true
}
