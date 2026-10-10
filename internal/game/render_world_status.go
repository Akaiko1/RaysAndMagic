package game

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/shaders"
)

// World status motifs use native-pixel shader coverage, not UI coordinates or
// vector stencil/supersampling targets. One actor submits each motif as a batch.
type worldStatusBatch struct {
	vertices []ebiten.Vertex
	indices  []uint32
	shader   *ebiten.Shader
	opts     ebiten.DrawTrianglesShaderOptions
}

func (b *worldStatusBatch) reset() { b.vertices, b.indices = b.vertices[:0], b.indices[:0] }

func (b *worldStatusBatch) ensureShader() error {
	if b.shader != nil {
		return nil
	}
	var err error
	b.shader, err = ebiten.NewShader([]byte(shaders.Source("world_status.kage")))
	return err
}

func (b *worldStatusBatch) flush(dst *ebiten.Image) {
	if len(b.indices) == 0 || b.ensureShader() != nil {
		return
	}
	worldDrawTrianglesShader32(dst, b.vertices, b.indices, b.shader, &b.opts)
}

func (b *worldStatusBatch) vertex(x, y, u, v, a, c, kind float64, col color.RGBA) {
	scale := statusColorScale(col)
	b.vertices = append(b.vertices, ebiten.Vertex{
		DstX: float32(x), DstY: float32(y), SrcX: float32(u), SrcY: float32(v),
		ColorR: scale.R(), ColorG: scale.G(), ColorB: scale.B(), ColorA: scale.A(),
		Custom0: float32(a), Custom1: float32(c), Custom3: float32(kind) + .25,
	})
}

func (b *worldStatusBatch) quad(x, y, rx, ry, co, si, kind float64, col color.RGBA) {
	base := uint32(len(b.vertices))
	// Two extra source pixels cover the final one-pixel antialias fringe in
	// either projection. Panini's viewport-normalized magnification is >= 1.
	for _, q := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		u, v := q[0]*(rx+2), q[1]*(ry+2)
		b.vertex(x+u*co-v*si, y+u*si+v*co, u, v, rx, ry, kind, col)
	}
	for _, i := range weaponQuadIndices {
		b.indices = append(b.indices, base+uint32(i))
	}
}

func (b *worldStatusBatch) circle(x, y, radius float64, col color.RGBA) {
	if radius > 0 {
		b.quad(x, y, radius, radius, 1, 0, 0, col)
	}
}

func (b *worldStatusBatch) line(x0, y0, x1, y1, width float64, col color.RGBA) {
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	if length > 0 && width > 0 {
		b.quad((x0+x1)/2, (y0+y1)/2, length/2, width/2, dx/length, dy/length, 1, col)
	}
}

func (b *worldStatusBatch) diamond(x, y, rx, ry float64, col color.RGBA) {
	if rx > 0 && ry > 0 {
		b.quad(x, y, rx, ry, 1, 0, 2, col)
	}
}

func (b *worldStatusBatch) triangle(p [3][2]float64, col color.RGBA) {
	ax, ay := p[1][0]-p[0][0], p[1][1]-p[0][1]
	cx, cy := p[2][0]-p[0][0], p[2][1]-p[0][1]
	det := ax*cy - ay*cx
	if math.Abs(det) < 1e-8 {
		return
	}
	// A padded bounding quad lets the shader cover every triangle edge once;
	// no stencil, overlapping triangle fan or internal antialias seam is needed.
	x0, x1 := min(p[0][0], p[1][0], p[2][0])-2, max(p[0][0], p[1][0], p[2][0])+2
	y0, y1 := min(p[0][1], p[1][1], p[2][1])-2, max(p[0][1], p[1][1], p[2][1])+2
	base := uint32(len(b.vertices))
	for _, q := range [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		x, y := q[0]-p[0][0], q[1]-p[0][1]
		b.vertex(q[0], q[1], (x*cy-y*cx)/det, (ax*y-ay*x)/det, 0, 0, 3, col)
	}
	for _, i := range weaponQuadIndices {
		b.indices = append(b.indices, base+uint32(i))
	}
}
