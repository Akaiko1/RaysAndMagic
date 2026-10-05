package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

// These IDs match the silhouette branches in spell_body.kage.
const (
	spellFireball = iota
	spellLightning
	spellHarm
	spellPsyshock
	spellStarburst
	spellDisintegrate
	spellLight
	spellRock
	spellStoneBud
	spellSwarm
	spellIce
	spellFireDart
	spellSparks
	spellShadow
	spellVoidNeedle
	spellShackle
	spellCharm
	spellPsiLance
	spellLash
	spellEye
)

func spellMaterialRenderer(kind int) func(*Renderer, *ebiten.Image, float64, float64, float64, float64, float64, [3]int, projectileFxProfile, float64, int) {
	return func(r *Renderer, dst *ebiten.Image, x, y, size, dx, dy float64, rgb [3]int, _ projectileFxProfile, crit float64, seed int) {
		r.drawSpellMaterial(dst, x, y, size, dx, dy, rgb, crit, seed, kind)
	}
}

func (r *Renderer) drawSpellMaterial(dst *ebiten.Image, x, y, size, dx, dy float64, rgb [3]int, crit float64, seed, kind int) {
	r.drawSpellMaterialFade(dst, x, y, size, dx, dy, rgb, crit, seed, kind, 1)
}

func (r *Renderer) drawSpellMaterialFade(dst *ebiten.Image, x, y, size, dx, dy float64, rgb [3]int, crit float64, seed, kind int, alpha float64) {
	if alpha <= 0 || size <= 0 || crit <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	if isFacetedBolt(kind) || kind == spellHarm {
		axis := r.spellFlightAxis
		if axis == [3]float64{} {
			axis = [3]float64{dx, dy, 0}
			if math.Hypot(dx, dy) < .01 {
				axis = [3]float64{0, 0, 1}
			}
		}
		r.drawSpellVolume(dst, x, y, size*math.Sqrt(crit), axis, rgb, alpha, kind)
		return
	}
	head := 0.0
	if math.Hypot(dx, dy) < .01 {
		head = 1
		dx, dy = 1, 0
	}
	size *= math.Sqrt(crit)
	left, right, top, bottom := -3.3, 1.7, -1.7, 1.7
	if kind == spellLight {
		left = -7
	}
	if head > 0 {
		left = -1.7
	} else if kind == spellLash {
		right = 2.8
	}
	phase := r.weaponMaterialClock()
	axial := 1.0
	if head == 0 && r.spellAxialScale > 0 && directionalSpellKind(kind) {
		axial = r.spellAxialScale
	}
	for i, p := range [4][2]float64{{left, top}, {right, top}, {left, bottom}, {right, bottom}} {
		v := weaponMaterialVertex(x+(p[0]*axial*dx-p[1]*dy)*size, y+(p[0]*axial*dy+p[1]*dx)*size, p[0], p[1], rgb, alpha)
		v.Custom0, v.Custom1 = float32(phase), float32(uint(seed)%997)*.031
		v.Custom2, v.Custom3 = float32(head), float32(kind)+.25
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	dst.DrawTrianglesShader(r.weaponMaterialQuad[:], weaponQuadIndices, r.spellBodyShader, &r.weaponMaterialOpts)
}

// The broad impact face expands quickly, then rolls apart into smoke and grit.
// Width and height are already projected; this helper never invents a depth.
func (r *Renderer) drawImpactCloud(dst *ebiten.Image, x, y, rx, ry, age float64, rgb [3]int, alpha float64, seed int, dust bool) {
	if age <= 0 || age >= 1 || rx <= 0 || ry <= 0 || alpha <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	for i, p := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		v := weaponMaterialVertex(x+p[0]*rx, y+p[1]*ry, p[0], p[1], rgb, alpha)
		v.Custom0, v.Custom1, v.Custom3 = float32(age), float32(uint(seed)%997)*.041, -1
		if dust {
			v.Custom2 = 1
		}
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	dst.DrawTrianglesShader(r.weaponMaterialQuad[:], weaponQuadIndices, r.impactMaterialShader, &r.weaponMaterialOpts)
}
