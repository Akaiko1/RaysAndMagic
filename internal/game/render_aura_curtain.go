package game

import (
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

func (r *Renderer) ensureAuraCurtainShader() (*ebiten.Shader, error) {
	if r.auraCurtainShader != nil {
		return r.auraCurtainShader, nil
	}
	shader, err := ebiten.NewShader([]byte(auraCurtainShaderSrc))
	if err != nil {
		return nil, err
	}
	r.auraCurtainShader = shader
	r.auraCurtainOpts.Blend = additiveGlowBlend
	r.auraCurtainOpts.Uniforms = map[string]any{"Phase": r.auraCurtainPhase[:]}
	return shader, nil
}

// auraCurtainEdge stores perspective-correct coordinates across one projected
// tile edge. Pieces retain these coordinates when split around scene depths.
type auraCurtainEdge struct {
	left, right, inv0, invStep, uq0, uqStep float64
	tileSize, alpha, maxDepth               float64
	density                                 float32
	rgb                                     [3]int
}

// collectAuraEdge projects and clips a tile edge without drawing over scenery.
func (r *Renderer) collectAuraEdge(tx, ty int, d [2]int, ts, folds, baseAlpha, maxDepth float64, rgb [3]int) {
	if maxDepth <= auraMinDepth || baseAlpha <= 0 {
		return
	}
	x0, y0 := float64(tx)*ts, float64(ty)*ts
	x1, y1 := x0, y0
	u0 := float64(tx + ty)
	if d[0] != 0 {
		if d[0] > 0 {
			x0 += ts
			u0++
		}
		x1, y1 = x0, y0+ts
	} else {
		if d[1] > 0 {
			y0 += ts
			u0++
		}
		x1, y1 = x0+ts, y0
	}
	u1 := u0 + 1
	h := r.game.renderHelper
	cx0, z0, ok0 := h.cameraSpaceXY(x0, y0)
	cx1, z1, ok1 := h.cameraSpaceXY(x1, y1)
	if !ok0 || !ok1 || (z0 < auraMinDepth && z1 < auraMinDepth) || (z0 > maxDepth && z1 > maxDepth) {
		return
	}
	// Clip in camera space first, including edges crossing the camera plane.
	clip := func(x, z, u, otherX, otherZ, otherU float64) (float64, float64, float64) {
		clipped := math.Max(auraMinDepth, math.Min(maxDepth, z))
		if clipped == z {
			return x, z, u
		}
		f := (clipped - z) / (otherZ - z)
		return x + f*(otherX-x), clipped, u + f*(otherU-u)
	}
	cx0, z0, u0 = clip(cx0, z0, u0, cx1, z1, u1)
	cx1, z1, u1 = clip(cx1, z1, u1, cx0, z0, u0)
	w := r.game.worldWidth()
	sx0, sx1 := float64(w)*0.5*(1+cx0/z0), float64(w)*0.5*(1+cx1/z1)
	if sx0 > sx1 {
		sx0, sx1, z0, z1, u0, u1 = sx1, sx0, z1, z0, u1, u0
	}
	if sx1-sx0 < 0.01 || sx1 <= 0 || sx0 >= float64(w) {
		return
	}
	inv0, uq0 := 1/z0, u0/z0
	invStep, uqStep := (1/z1-inv0)/(sx1-sx0), (u1/z1-uq0)/(sx1-sx0)
	// Keep broad folds rather than a fence of narrow vertical bands.
	density := float32(math.Max(2, math.Min(8, folds)))
	left := math.Max(0, sx0)
	r.auraCurtainEdges = append(r.auraCurtainEdges, auraCurtainEdge{
		left: left, right: math.Min(float64(w), sx1),
		inv0: inv0 + (left-sx0)*invStep, invStep: invStep,
		uq0: uq0 + (left-sx0)*uqStep, uqStep: uqStep,
		tileSize: ts, alpha: baseAlpha, maxDepth: maxDepth, density: density, rgb: rgb,
	})
}

// collectTileCurtains splits sloping edges at scene depth planes. Sorting an
// entire square (or even an entire edge) by its centre lets its far half paint
// over a nearer sprite. Reused depth and geometry buffers keep this CPU-only.
func (r *Renderer) collectTileCurtains(sprites []UnifiedSpriteRenderData) []UnifiedSpriteRenderData {
	r.auraCurtainEdges = r.auraCurtainEdges[:0]
	r.collectImpassableTileAura()
	sprites = r.collectTrapTileBorders(sprites)
	r.collectEnvironmentEffects()
	r.collectSpawnTileBorder()
	if len(r.auraCurtainEdges) == 0 {
		return sprites
	}
	depths := r.auraSceneDepths[:0]
	for _, s := range sprites {
		depths = append(depths, s.depthPerp)
	}
	slices.Sort(depths)
	depths = slices.Compact(depths)
	r.auraSceneDepths = depths
	for i, edge := range r.auraCurtainEdges {
		leftDepth := 1 / edge.inv0
		rightDepth := 1 / (edge.inv0 + (edge.right-edge.left)*edge.invStep)
		near, far := math.Min(leftDepth, rightDepth), math.Max(leftDepth, rightDepth)
		add := func(a, b float64) {
			left, right := math.Min(a, b), math.Max(a, b)
			if right-left <= 1e-7 {
				return
			}
			depth := 1 / (edge.inv0 + ((left+right)*.5-edge.left)*edge.invStep)
			sprites = append(sprites, UnifiedSpriteRenderData{
				spriteType: SpriteTypeTileCurtain, depthPerp: depth,
				tileX: i, screenXF: left, sizeF: right - left,
			})
		}
		start := edge.left
		if rightDepth < leftDepth {
			start = edge.right
		}
		first, _ := slices.BinarySearch(depths, near)
		for _, depth := range depths[first:] {
			if depth >= far {
				break
			}
			if depth <= near {
				continue
			}
			x := edge.left + (1/depth-edge.inv0)/edge.invStep
			add(start, x)
			start = x
		}
		end := edge.right
		if rightDepth < leftDepth {
			end = edge.left
		}
		add(start, end)
	}
	return sprites
}

// appendAuraCurtain clips against walls; scenery and actors occlude with their
// actual alpha silhouettes in the painter pass. Consecutive pieces batch until
// the next non-curtain draw, preserving order without one GPU call per edge.
func (r *Renderer) appendAuraCurtain(s UnifiedSpriteRenderData) {
	edge := r.auraCurtainEdges[s.tileX]
	sx0, sx1 := s.screenXF, s.screenXF+s.sizeF
	w, height := r.game.worldWidth(), r.game.worldHeight()
	lo, hi := max(0, int(math.Floor(sx0))), min(w-1, int(math.Ceil(sx1))-1)
	verts, indices := r.auraCurtainVerts, r.auraCurtainIndices
	vertex := func(x float64) (ebiten.Vertex, ebiten.Vertex) {
		inv := edge.inv0 + (x-edge.left)*edge.invStep
		floorRise := float64(height) * 0.5 * edge.tileSize * inv
		bottom, rise := float64(height)*0.5+floorRise, floorRise*auraRiseFraction
		v := ebiten.Vertex{DstX: float32(x), DstY: float32(bottom - rise),
			SrcX: float32(bottom), SrcY: float32(rise),
			ColorR: float32(edge.rgb[0]) / 255, ColorG: float32(edge.rgb[1]) / 255, ColorB: float32(edge.rgb[2]) / 255, ColorA: float32(edge.alpha),
			Custom0: float32(inv), Custom1: float32(edge.uq0 + (x-edge.left)*edge.uqStep), Custom2: edge.density, Custom3: float32(1 / edge.maxDepth)}
		b := v
		b.DstY = float32(bottom)
		return v, b
	}
	start := -1
	for x := lo; x <= hi+1; x++ {
		visible := false
		if x <= hi {
			inv := edge.inv0 + (math.Max(sx0, math.Min(sx1, float64(x)+0.5))-edge.left)*edge.invStep
			depth := 1 / inv
			visible = !(x < len(r.game.depthBuffer) && depth >= r.game.depthBuffer[x])
			floorRise := float64(height) * 0.5 * edge.tileSize * inv
			visible = visible && float64(height)*0.5+floorRise*(1-auraRiseFraction) < float64(height)
		}
		if start >= 0 && !visible {
			lt, lb := vertex(math.Max(float64(start), sx0))
			rt, rb := vertex(math.Min(float64(x), sx1))
			base := uint32(len(verts))
			verts = append(verts, lt, rt, lb, rb)
			indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
			start = -1
		}
		if start < 0 && visible {
			start = x
		}
	}
	r.auraCurtainVerts, r.auraCurtainIndices = verts, indices
}

func (r *Renderer) flushAuraCurtains(screen *ebiten.Image) {
	verts, indices := r.auraCurtainVerts, r.auraCurtainIndices
	if len(indices) > 0 {
		if shader, err := r.ensureAuraCurtainShader(); err == nil {
			// All temporal frequencies are integral multiples of this phase,
			// so wrapping every 20 seconds is continuous even in long sessions.
			period := int64(max(1, r.game.config.GetTPS())) * 20
			r.auraCurtainPhase[0] = float32(float64(r.game.frameCount%period) * (2 * math.Pi / float64(period)))
			screen.DrawTrianglesShader32(verts, indices, shader, &r.auraCurtainOpts)
		}
	}
	r.auraCurtainVerts, r.auraCurtainIndices = verts[:0], indices[:0]
}

func (r *Renderer) drawAuraCurtainShaderWarm(target *ebiten.Image) {
	if r.auraCurtainWarmed {
		return
	}
	shader, err := r.ensureAuraCurtainShader()
	if err != nil {
		return
	}
	// An invisible pixel exercises the shader and blend pipeline before the
	// first visible effect, without a render target or readback fence.
	vertices := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 1, SrcY: 1, Custom0: 1, Custom2: 3},
		{DstX: 1, DstY: 0, SrcX: 1, SrcY: 1, Custom0: 1, Custom2: 3},
		{DstX: 0, DstY: 1, SrcX: 1, SrcY: 1, Custom0: 1, Custom2: 3},
		{DstX: 1, DstY: 1, SrcX: 1, SrcY: 1, Custom0: 1, Custom2: 3},
	}
	target.DrawTrianglesShader32(vertices, []uint32{0, 1, 2, 1, 3, 2}, shader, &r.auraCurtainOpts)
	r.auraCurtainWarmed = true
}
