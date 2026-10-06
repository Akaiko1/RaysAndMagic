package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// One bounded GPU plume replaces a stack of independent glowing particles.
// Visible spans retain wall clipping. The unified painter pass supplies
// per-pixel occlusion against scenery and actor silhouettes.
func (r *Renderer) drawZonePlume(dst *ebiten.Image, wx, wy, width, height, maxDepth, alpha float64, rgb [3]int) {
	r.drawFirePlume(dst, wx, wy, width, height, maxDepth, alpha, rgb, int(wx), int(wy))
}

// World fields seed from their position; attached flames use stable actor IDs
// so moving a burning monster does not replace its noise pattern every step.
func (r *Renderer) drawFirePlume(dst *ebiten.Image, wx, wy, width, height, maxDepth, alpha float64, rgb [3]int, seedX, seedY int) {
	sx, depth, ok := r.game.renderHelper.projectToScreenXF(wx, wy)
	if !ok || depth < auraMinDepth || depth > maxDepth || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	floor := float64(r.game.renderHelper.calculateFloorScreenY(depth))
	unit := floor - float64(r.game.worldHeight())*.5
	half := unit * width
	rise := unit * height * (.87 + .26*auraHash(seedX, seedY, 912, 0))
	if rise <= 0 || half <= 0 {
		return
	}
	lo, hi := max(0, int(math.Floor(sx-half))), min(r.game.worldWidth()-1, int(math.Ceil(sx+half)))
	phase := r.weaponMaterialClock() * (.82 + .36*auraHash(seedX, seedY, 913, 0))
	seed := auraHash(seedX, seedY, 911, 0) * 47
	alpha *= math.Min(1, (maxDepth-depth)/(maxDepth*.3))
	verts, indices := r.standeeVerts[:0], r.standeeIdx[:0]
	appendSpan := func(a, b int) {
		base := uint16(len(verts))
		for _, p := range [4][2]float64{{float64(a), 1}, {float64(b), 1}, {float64(a), 0}, {float64(b), 0}} {
			v := weaponMaterialVertex(p[0], floor-p[1]*rise, (p[0]-sx)/half, p[1], rgb, alpha)
			v.Custom0, v.Custom1 = float32(phase), float32(seed)
			verts = append(verts, v)
		}
		indices = append(indices, base, base+1, base+2, base+1, base+3, base+2)
	}
	start := -1
	for x := lo; x <= hi+1; x++ {
		visible := x <= hi && !(x < len(r.game.depthBuffer) && depth >= r.game.depthBuffer[x])
		if start >= 0 && !visible {
			appendSpan(start, x)
			start = -1
		}
		if start < 0 && visible {
			start = x
		}
	}
	if len(indices) > 0 {
		r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
		r.weaponMaterialOpts.Images[0] = r.ensureFireNoise()
		dst.DrawTrianglesShader(verts, indices, r.zonePlumeShader, &r.weaponMaterialOpts)
		r.weaponMaterialOpts.Images[0] = nil
	}
	r.standeeVerts, r.standeeIdx = verts[:0], indices[:0]
}
