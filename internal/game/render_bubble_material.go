package game

import "github.com/hajimehoshi/ebiten/v2"

func (r *Renderer) drawShaderBubble(dst *ebiten.Image, x, y, size float64, rgb [3]int, alpha, age, seed float64) {
	if size <= 0 || alpha <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	for i, p := range [4][2]float64{{-1.1, -1.1}, {1.1, -1.1}, {-1.1, 1.1}, {1.1, 1.1}} {
		v := weaponMaterialVertex(x+p[0]*size*.5, y+p[1]*size*.5, p[0], p[1], rgb, alpha*1.5)
		v.Custom0, v.Custom1, v.Custom2 = float32(r.weaponMaterialClock()), float32(seed*35), float32(age)
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	dst.DrawTrianglesShader(r.weaponMaterialQuad[:], weaponQuadIndices, r.bubbleShader, &r.weaponMaterialOpts)
}
