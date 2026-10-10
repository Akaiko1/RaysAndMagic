package game

import (
	"image"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// Only final world submissions are projected. Resource uploads, the panorama,
// HUD and previews keep their own coordinates. Draw runs on the engine thread;
// the scoped binding is restored even if a loading preflight aborts the pass.
var activeWorldRaster *paniniRasterizer
var zeroWorldPanini [4]float32

type paniniRasterizer struct {
	target        *ebiten.Image
	projection    paniniProjection
	bounds        image.Rectangle
	knots         []float64
	columnScale   []float64
	vertices      []ebiten.Vertex
	indices       []uint32
	uniform       [4]float32
	centerUniform [1]float32
	emptyUniforms map[string]any
}

func (gl *GameLoop) renderProjectedWorld(dst *ebiten.Image) {
	if gl.worldProjection.distance == 0 {
		gl.renderer.RenderFirstPersonView(dst)
		return
	}
	p := &gl.paniniRaster
	p.configure(dst, gl.worldProjection)
	previous := activeWorldRaster
	activeWorldRaster = p
	defer func() { activeWorldRaster = previous; p.target = nil }()
	gl.renderer.RenderFirstPersonView(dst)
}

func (p *paniniRasterizer) configure(dst *ebiten.Image, projection paniniProjection) {
	p.target = dst
	if p.projection == projection && p.bounds == dst.Bounds() {
		return
	}
	p.projection, p.bounds = projection, dst.Bounds()
	p.centerUniform[0] = float32(projection.center)
	p.uniform = [4]float32{float32(projection.distance), float32(projection.scale), float32(projection.focal), float32(projection.horizon)}
	count := 2*p.bounds.Dx() + 1
	if cap(p.columnScale) < count {
		p.columnScale = make([]float64, count)
	} else {
		p.columnScale = p.columnScale[:count]
	}
	for i := range p.columnScale {
		x := float64(p.bounds.Min.X) + float64(i)*.5
		p.columnScale[i] = projection.columnMagnification(x)
	}
	p.knots = p.knots[:0]
	p.knots = append(p.knots, float64(p.bounds.Min.X))
	// Neighbouring faces share X cuts. Bound curvature against
	// both vertical viewport extremes, including quarter points of symmetric arcs.
	var divide func(float64, float64)
	divide = func(a, b float64) {
		error := 0.0
		for _, y := range []float64{float64(p.bounds.Min.Y), float64(p.bounds.Max.Y)} {
			ax, ay := projection.presentedPoint(a, y)
			bx, by := projection.presentedPoint(b, y)
			for _, t := range []float64{.25, .5, .75} {
				x, v := projection.presentedPoint(a+(b-a)*t, y)
				error = max(error, math.Abs(x-(ax+(bx-ax)*t)), math.Abs(v-(ay+(by-ay)*t)))
			}
		}
		if error > .08 && b-a > 1 {
			m := (a + b) * .5
			divide(a, m)
			divide(m, b)
			return
		}
		p.knots = append(p.knots, b)
	}
	divide(float64(p.bounds.Min.X), float64(p.bounds.Max.X))
}

// Material helpers inherit their caller's pass. Camera-local effects can reuse
// worldDraw helpers: without this target binding they submit ordinary pixels.
func worldRaster(dst *ebiten.Image) *paniniRasterizer {
	if p := activeWorldRaster; p != nil && p.target == dst {
		return p
	}
	return nil
}

// worldProjectionOf is the projection a world draw onto dst goes through.
func worldProjectionOf(dst *ebiten.Image) paniniProjection {
	if p := worldRaster(dst); p != nil {
		return p.projection
	}
	return paniniProjection{}
}

func vertexBetween(a, b ebiten.Vertex, t float32) ebiten.Vertex {
	return ebiten.Vertex{
		DstX: a.DstX + (b.DstX-a.DstX)*t, DstY: a.DstY + (b.DstY-a.DstY)*t,
		SrcX: a.SrcX + (b.SrcX-a.SrcX)*t, SrcY: a.SrcY + (b.SrcY-a.SrcY)*t,
		ColorR: a.ColorR + (b.ColorR-a.ColorR)*t, ColorG: a.ColorG + (b.ColorG-a.ColorG)*t,
		ColorB: a.ColorB + (b.ColorB-a.ColorB)*t, ColorA: a.ColorA + (b.ColorA-a.ColorA)*t,
		Custom0: a.Custom0 + (b.Custom0-a.Custom0)*t, Custom1: a.Custom1 + (b.Custom1-a.Custom1)*t,
		Custom2: a.Custom2 + (b.Custom2-a.Custom2)*t, Custom3: a.Custom3 + (b.Custom3-a.Custom3)*t,
	}
}

func clipPaniniPolygon(out, in []ebiten.Vertex, axis int, edge float32, greater bool) []ebiten.Vertex {
	out = out[:0]
	if len(in) == 0 {
		return out
	}
	coordinate := func(v ebiten.Vertex) float32 {
		if axis == 0 {
			return v.DstX
		}
		return v.DstY
	}
	a := in[len(in)-1]
	av := coordinate(a)
	for _, b := range in {
		bv := coordinate(b)
		ai, bi := av >= edge, bv >= edge
		if !greater {
			ai, bi = av <= edge, bv <= edge
		}
		if ai != bi {
			out = append(out, vertexBetween(a, b, (edge-av)/(bv-av)))
		}
		if bi {
			out = append(out, b)
		}
		a, av = b, bv
	}
	return out
}

// Keep perspective coordinates for visibility and painter order, but project
// each surface before the GPU samples its material. This never resamples a
// rendered scene. Subdivision bounds nonlinear attribute interpolation error.
func projectWorldTriangles[I ~uint16 | ~uint32](p *paniniRasterizer, vs []ebiten.Vertex, is []I, columnMaterial bool) ([]ebiten.Vertex, []uint32) {
	p.vertices, p.indices = p.vertices[:0], p.indices[:0]
	for i := 0; i+2 < len(is); i += 3 {
		tri := [3]ebiten.Vertex{vs[is[i]], vs[is[i+1]], vs[is[i+2]]}
		analyticColumn := false
		if columnMaterial {
			for j := 0; j < 3; j++ {
				a, b := tri[j], tri[(j+1)%3]
				if a.DstX == b.DstX && a.DstY != b.DstY {
					a.DstY = b.DstY
					analyticColumn = a == b
					break
				}
			}
		}
		lo := max(float64(p.bounds.Min.X), float64(min(tri[0].DstX, tri[1].DstX, tri[2].DstX)))
		hi := min(float64(p.bounds.Max.X), float64(max(tri[0].DstX, tri[1].DstX, tri[2].DstX)))
		if hi <= lo {
			continue
		}
		start := sort.SearchFloat64s(p.knots, lo)
		if start > 0 && p.knots[start] > lo {
			start--
		}
		if hi <= p.knots[start+1] && min(tri[0].DstX, tri[1].DstX, tri[2].DstX) >= float32(p.bounds.Min.X) && max(tri[0].DstX, tri[1].DstX, tri[2].DstX) <= float32(p.bounds.Max.X) && min(tri[0].DstY, tri[1].DstY, tri[2].DstY) >= float32(p.bounds.Min.Y) && max(tri[0].DstY, tri[1].DstY, tri[2].DstY) <= float32(p.bounds.Max.Y) {
			p.appendSurfaceTriangle(tri[0], tri[1], tri[2], 0, analyticColumn)
			continue
		}
		for k := start; k+1 < len(p.knots) && p.knots[k] < hi; k++ {
			var a, b [12]ebiten.Vertex
			poly := clipPaniniPolygon(a[:0], tri[:], 0, float32(p.knots[k]), true)
			poly = clipPaniniPolygon(b[:0], poly, 0, float32(p.knots[k+1]), false)
			poly = clipPaniniPolygon(a[:0], poly, 1, float32(p.bounds.Min.Y), true)
			poly = clipPaniniPolygon(b[:0], poly, 1, float32(p.bounds.Max.Y), false)
			if len(poly) < 3 {
				continue
			}
			for j := 1; j+1 < len(poly); j++ {
				p.appendSurfaceTriangle(poly[0], poly[j], poly[j+1], 0, analyticColumn)
			}
		}
	}
	return p.vertices, p.indices
}

func worldDrawImage(dst, src *ebiten.Image, op *ebiten.DrawImageOptions) {
	p := worldRaster(dst)
	if p == nil {
		dst.DrawImage(src, op)
		return
	}
	var o ebiten.DrawImageOptions
	if op != nil {
		o = *op
	}
	b := src.Bounds()
	var v [4]ebiten.Vertex
	for i, q := range [4][2]float64{{0, 0}, {float64(b.Dx()), 0}, {0, float64(b.Dy())}, {float64(b.Dx()), float64(b.Dy())}} {
		x, y := o.GeoM.Apply(q[0], q[1])
		v[i] = ebiten.Vertex{DstX: float32(x), DstY: float32(y), SrcX: float32(q[0] + float64(b.Min.X)), SrcY: float32(q[1] + float64(b.Min.Y)), ColorR: o.ColorScale.R(), ColorG: o.ColorScale.G(), ColorB: o.ColorScale.B(), ColorA: o.ColorScale.A()}
	}
	opts := ebiten.DrawTrianglesOptions{ColorM: o.ColorM, ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha, CompositeMode: o.CompositeMode, Blend: o.Blend, Filter: o.Filter, DisableMipmaps: o.DisableMipmaps}
	vertices, indices := projectWorldTriangles(p, v[:], weaponQuadIndices, false)
	dst.DrawTriangles32(vertices, indices, src, &opts)
}

func worldDrawTriangles(dst *ebiten.Image, v []ebiten.Vertex, i []uint16, src *ebiten.Image, op *ebiten.DrawTrianglesOptions) {
	if p := worldRaster(dst); p != nil {
		v, j := projectWorldTriangles(p, v, i, false)
		dst.DrawTriangles32(v, j, src, op)
		return
	}
	dst.DrawTriangles(v, i, src, op)
}
func worldShaderOptions(p *paniniRasterizer, op *ebiten.DrawTrianglesShaderOptions) ebiten.DrawTrianglesShaderOptions {
	var o ebiten.DrawTrianglesShaderOptions
	if op != nil {
		o = *op
	}
	if p == nil {
		if o.Uniforms != nil {
			o.Uniforms["WorldPanini"] = zeroWorldPanini[:]
		}
		return o
	}
	if o.Uniforms == nil {
		if op != nil {
			op.Uniforms = make(map[string]any)
			o.Uniforms = op.Uniforms
		} else {
			if p.emptyUniforms == nil {
				p.emptyUniforms = make(map[string]any)
			}
			o.Uniforms = p.emptyUniforms
		}
	}
	o.Uniforms["WorldPanini"] = p.uniform[:]
	o.Uniforms["WorldCenter"] = p.centerUniform[:]
	return o
}
func worldDrawTrianglesShader(dst *ebiten.Image, v []ebiten.Vertex, i []uint16, shader *ebiten.Shader, op *ebiten.DrawTrianglesShaderOptions) {
	p := worldRaster(dst)
	o := worldShaderOptions(p, op)
	if p != nil {
		v, j := projectWorldTriangles(p, v, i, false)
		dst.DrawTrianglesShader32(v, j, shader, &o)
		return
	}
	dst.DrawTrianglesShader(v, i, shader, &o)
}
func worldDrawTrianglesShader32(dst *ebiten.Image, v []ebiten.Vertex, i []uint32, shader *ebiten.Shader, op *ebiten.DrawTrianglesShaderOptions) {
	p := worldRaster(dst)
	o := worldShaderOptions(p, op)
	if p != nil {
		v, i = projectWorldTriangles(p, v, i, false)
	}
	dst.DrawTrianglesShader32(v, i, shader, &o)
}

// Mip selection measures the final pixel footprint, not the temporary
// perspective coordinates used by visibility and primitive construction.
func worldPixelScale(dst *ebiten.Image, x, y float64) (float64, float64) {
	p := worldRaster(dst)
	if p == nil {
		return 1, 1
	}
	ax, ay := p.presentedPoint(x-.5, y)
	bx, by := p.presentedPoint(x+.5, y)
	_, cy := p.presentedPoint(x, y+.5)
	_, dy := p.presentedPoint(x, y-.5)
	return max(.001, math.Hypot(bx-ax, by-ay)), max(.001, cy-dy)
}

// The horizon remains horizontal in Panini. A single full-width floor quad
// generates the final pixel rays directly in Kage; tessellating it adds no detail.
func worldDrawFloorShader(dst *ebiten.Image, v []ebiten.Vertex, i []uint16, shader *ebiten.Shader, op *ebiten.DrawTrianglesShaderOptions) {
	o := worldShaderOptions(worldRaster(dst), op)
	dst.DrawTrianglesShader(v, i, shader, &o)
}

// Horizontal strips alone do not bound UV error on a tall oblique triangle:
// Panini's vertical scale also changes across X. Split its curved diagonals
// until interpolated material coordinates agree to a fraction of one pixel.
func (p *paniniRasterizer) appendSurfaceTriangle(a, b, c ebiten.Vertex, level int, columnMaterial bool) {
	// Clipping can repeat a boundary vertex. Such fans cover no pixels.
	if (float64(b.DstX)-float64(a.DstX))*(float64(c.DstY)-float64(a.DstY)) == (float64(b.DstY)-float64(a.DstY))*(float64(c.DstX)-float64(a.DstX)) {
		return
	}
	raw := [3]ebiten.Vertex{a, b, c}
	var projected [3]ebiten.Vertex
	for i, v := range raw {
		x, y := p.presentedPoint(float64(v.DstX), float64(v.DstY))
		v.DstX, v.DstY = float32(x), float32(y)
		projected[i] = v
	}
	error, worst := 0.0, 0
	for i := 0; !columnMaterial && i < 3; i++ {
		j := (i + 1) % 3
		x, y := p.presentedPoint((float64(raw[i].DstX)+float64(raw[j].DstX))*.5, (float64(raw[i].DstY)+float64(raw[j].DstY))*.5)
		e := max(math.Abs(x-(float64(projected[i].DstX)+float64(projected[j].DstX))*.5), math.Abs(y-(float64(projected[i].DstY)+float64(projected[j].DstY))*.5))
		if e > error {
			error, worst = e, i
		}
	}
	// Inputs are clipped to the viewport. Sixteen bisections bound work even
	// for degenerate imported geometry without depending on texture resolution.
	if error > .1 && level < 16 {
		i, j, k := worst, (worst+1)%3, (worst+2)%3
		mid := vertexBetween(raw[i], raw[j], .5)
		p.appendSurfaceTriangle(raw[i], mid, raw[k], level+1, false)
		p.appendSurfaceTriangle(mid, raw[j], raw[k], level+1, false)
		return
	}
	base := uint32(len(p.vertices))
	p.vertices = append(p.vertices, projected[:]...)
	p.indices = append(p.indices, base, base+1, base+2)
}

// Column materials reconstruct their vertical sample analytically from dstPos.
// Their remaining attributes vary only with X: vertical tessellation would add
// geometry without improving their sampling or their projected outer edges.
func worldDrawColumnShader32(dst *ebiten.Image, v []ebiten.Vertex, i []uint32, shader *ebiten.Shader, op *ebiten.DrawTrianglesShaderOptions) {
	p := worldRaster(dst)
	o := worldShaderOptions(p, op)
	if p != nil {
		v, i = projectWorldTriangles(p, v, i, true)
	}
	dst.DrawTrianglesShader32(v, i, shader, &o)
}

// Columns and their half-pixel midpoints recur across every wall and slab.
// Cache their scale when the viewport changes, keeping arbitrary clipped
// vertices exact instead of quantizing them onto the column grid.
func (p *paniniRasterizer) presentedPoint(x, y float64) (float64, float64) {
	fx := 2 * (x - float64(p.bounds.Min.X))
	i := int(fx)
	scale := 0.0
	if i >= 0 && i < len(p.columnScale) && fx == float64(i) {
		scale = p.columnScale[i]
	} else {
		scale = p.projection.columnMagnification(x)
	}
	return p.projection.center + (x-p.projection.center)*scale, p.projection.horizon + (y-p.projection.horizon)*scale
}
