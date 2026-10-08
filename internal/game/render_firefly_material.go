package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

func (r *Renderer) drawFirefly(dst *ebiten.Image, x, y, radius, alpha, phase float64, glow, core [3]int) {
	if alpha <= 0 || radius <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	angle := .3 * math.Sin(phase*.021)
	ca, sa := math.Cos(angle), math.Sin(angle)
	for i, p := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		v := weaponMaterialVertex(x+(p[0]*ca-p[1]*sa)*radius, y+(p[0]*sa+p[1]*ca)*radius, p[0], p[1], glow, alpha)
		v.Custom0, v.Custom1, v.Custom2, v.Custom3 = float32(core[0])/255, float32(core[1])/255, float32(core[2])/255, float32(phase)
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendLighter
	dst.DrawTrianglesShader(r.weaponMaterialQuad[:], weaponQuadIndices, r.fireflyShader, &r.weaponMaterialOpts)
}

const fireflyPulseFloor = .12

// Insects flash independently, with a quiet interval between rounded pulses.
func fireflyPulse(seconds, phase float64) float64 {
	wave := .5 + .5*math.Sin(seconds*(1.5+.24*math.Sin(phase))+phase)
	return fireflyPulseFloor + (1-fireflyPulseFloor)*wave*wave*wave
}
