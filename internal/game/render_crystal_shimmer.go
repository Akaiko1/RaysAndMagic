package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"ugataima/internal/character"
)

// The choice is stable throughout a pulse and independent of draw count, FPS,
// and facade segment order. Hashing each interval gives a non-cycling sequence.
func crystalShimmerAt(s *character.CrystalShimmer, frame int64, tps int, seed uint64) (int, float32) {
	if s == nil || tps <= 0 || len(s.Crystals) == 0 || s.IntervalSeconds <= 0 || s.DurationSeconds <= 0 {
		return 0, 0
	}
	seconds := float64(frame) / float64(tps)
	interval := uint64(seconds / s.IntervalSeconds)
	if interval == 0 {
		return 0, 0
	}
	phase := math.Mod(seconds, s.IntervalSeconds) / s.DurationSeconds
	if phase >= 1 {
		return 0, 0
	}
	h := seed + interval*0x9e3779b97f4a7c15
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	h ^= h >> 31
	envelope := math.Sin(math.Pi * phase)
	flicker := .8 + .2*math.Sin(phase*math.Pi*14)
	return int(h % uint64(len(s.Crystals))), float32(envelope * envelope * flicker)
}

func (r *Renderer) crystalShimmerOptions(npc *character.NPC, sprite *ebiten.Image) (*ebiten.DrawTrianglesShaderOptions, *ebiten.Shader) {
	seed := uint64(npc.X)*73856093 ^ uint64(npc.Y)*19349663
	index, strength := crystalShimmerAt(npc.CrystalShimmer, r.game.frameCount, r.game.config.GetTPS(), seed)
	if strength <= 0 {
		return nil, nil
	}
	if r.crystalShimmerShader == nil {
		shader, err := ebiten.NewShader([]byte(crystalShimmerShaderSource))
		if err != nil {
			return nil, nil
		}
		r.crystalShimmerShader = shader
	}
	c := npc.CrystalShimmer.Crystals[index]
	opts := &ebiten.DrawTrianglesShaderOptions{Blend: additiveGlowBlend}
	opts.Images[0] = sprite
	opts.Uniforms = map[string]any{
		"Center":   []float32{float32(c.Center[0]), float32(c.Center[1])},
		"Radius":   []float32{float32(c.Radius[0]), float32(c.Radius[1])},
		"Tint":     []float32{float32(c.Color[0]) / 255, float32(c.Color[1]) / 255, float32(c.Color[2]) / 255},
		"Strength": strength,
	}
	return opts, r.crystalShimmerShader
}

// Draw in the same painter slot and on the same near surface as the facade.
// Per-column depth and wall-top clipping keep the light behind occluders.
func (r *Renderer) drawFacadeCrystalShimmer(screen *ebiten.Image, npc *character.NPC, slab standeeSlab, clipMin, clipMax int) {
	if npc.CrystalShimmer == nil || len(slab.surfaces) == 0 {
		return
	}
	sf := slab.surfaces[len(slab.surfaces)-1]
	opts, shader := r.crystalShimmerOptions(npc, sf.img)
	if shader == nil {
		return
	}
	lo, hi := max(slab.minX, clipMin), min(slab.maxX, clipMax)
	cam, basis := r.game.camera, r.cameraBasis()
	horizon := r.game.viewHorizon()
	bounds := sf.img.Bounds()
	tw, th := float32(bounds.Dx()), float32(bounds.Dy())
	verts, indices := r.standeeVerts[:0], r.standeeMaterialIdx[:0]
	for x := lo; x <= hi; x++ {
		rx, ry := standeeRayAtScreenX(float64(x)+.5, r.game.worldWidth(), basis.dirX, basis.dirY, basis.planeX, basis.planeY)
		depth, u, ok := standeeColumnHit(cam.X, cam.Y, rx, ry, sf.p0x, sf.p0y, sf.dx, sf.dy)
		if !ok {
			continue
		}
		height := slab.centerSize * slab.centerDepth / depth
		bottom := horizon + (slab.bottomY-horizon)*slab.centerDepth/depth
		top := bottom - height
		drawBottom := bottom
		if x < len(r.game.depthBuffer) && standeeColumnOccluded(depth, r.game.depthBuffer[x], 0) {
			drawBottom = math.Min(drawBottom, float64(r.game.wallTopBuffer[x]))
		}
		if drawBottom <= top {
			continue
		}
		if sf.mirrored {
			u = 1 - u
		}
		sx := float32(bounds.Min.X) + float32(u)*(tw-1) + .5
		sy := float32(bounds.Min.Y) + .5
		ey := sy + float32((drawBottom-top)/height)*(th-1)
		base := uint32(len(verts))
		verts = append(verts,
			ebiten.Vertex{DstX: float32(x), DstY: float32(top), SrcX: sx, SrcY: sy},
			ebiten.Vertex{DstX: float32(x + 1), DstY: float32(top), SrcX: sx, SrcY: sy},
			ebiten.Vertex{DstX: float32(x), DstY: float32(drawBottom), SrcX: sx, SrcY: ey},
			ebiten.Vertex{DstX: float32(x + 1), DstY: float32(drawBottom), SrcX: sx, SrcY: ey})
		indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
	}
	if len(indices) > 0 {
		worldDrawTrianglesShader32(screen, verts, indices, shader, opts)
	}
	r.standeeVerts, r.standeeMaterialIdx = verts[:0], indices[:0]
}

func (r *Renderer) drawBillboardCrystalShimmer(screen *ebiten.Image, npc *character.NPC, sprite *ebiten.Image, left, top, size float64) {
	opts, shader := r.crystalShimmerOptions(npc, sprite)
	if shader == nil {
		return
	}
	b := sprite.Bounds()
	vertices := []ebiten.Vertex{
		{DstX: float32(left), DstY: float32(top), SrcX: float32(b.Min.X), SrcY: float32(b.Min.Y)},
		{DstX: float32(left + size), DstY: float32(top), SrcX: float32(b.Max.X), SrcY: float32(b.Min.Y)},
		{DstX: float32(left), DstY: float32(top + size), SrcX: float32(b.Min.X), SrcY: float32(b.Max.Y)},
		{DstX: float32(left + size), DstY: float32(top + size), SrcX: float32(b.Max.X), SrcY: float32(b.Max.Y)}}
	worldDrawTrianglesShader(screen, vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
}
