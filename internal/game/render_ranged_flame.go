package game

import "github.com/hajimehoshi/ebiten/v2"

type rangedSunFlameRenderer struct {
	opts     ebiten.DrawTrianglesShaderOptions
	vertices [4]ebiten.Vertex
	time     [1]float32
}

// The charged crown uses Preservation's flame, anchored by its round base to
// the lava core. One tight quad shares the cached noise and remains behind the
// depth-resolved basket. Uncharged shots draw only the core in the solid mesh.
func (r *Renderer) drawRangedSunFlame(dst *ebiten.Image, x, y, scale, alpha float64) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	f := &r.rangedSunFlame
	if f.opts.Uniforms == nil {
		f.opts.Uniforms = map[string]any{"Time": f.time[:]}
	}
	f.time[0] = float32(r.weaponMaterialClock()) + .3
	f.opts.Images[0] = r.ensureFireNoise()
	// These are the bounds of preservationFlame; its base is at y=.218.
	for i, p := range [4][2]float64{{-.13, -.16}, {.13, -.16}, {-.13, .32}, {.13, .32}} {
		f.vertices[i] = ebiten.Vertex{
			DstX: float32(x + p[0]*scale), DstY: float32(y + (p[1]-.218)*scale),
			SrcX: float32(p[0]), SrcY: float32(p[1]),
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: float32(alpha),
		}
	}
	dst.DrawTrianglesShader(f.vertices[:], weaponQuadIndices, r.rangedFlameShader, &f.opts)
}
