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

type spellProjection struct {
	dx, dy, head, axial float64
	axis                [3]float64
}

func (r *Renderer) drawSpellMaterialFade(dst *ebiten.Image, x, y, size, dx, dy float64, rgb [3]int, crit float64, seed, kind int, alpha float64) {
	projection := spellProjection{dx: dx, dy: dy, axial: 1, axis: [3]float64{dx, dy, 0}}
	if math.Hypot(dx, dy) < .01 {
		projection.dx, projection.dy, projection.head, projection.axis = 1, 0, 1, [3]float64{0, 0, 1}
	}
	r.drawSpellMaterialProjected(dst, x, y, size, rgb, crit, seed, kind, alpha, projection)
}

func (r *Renderer) drawSpellMaterialProjected(dst *ebiten.Image, x, y, size float64, rgb [3]int, crit float64, seed, kind int, alpha float64, projection spellProjection) {
	if alpha <= 0 || size <= 0 || crit <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	if isFacetedBolt(kind) || kind == spellHarm {
		r.drawLitVolume(dst, x, y, size*math.Sqrt(crit), projection.axis, rgb, alpha, kind, 1)
		return
	}
	dx, dy, head := projection.dx, projection.dy, projection.head
	size *= math.Sqrt(crit)
	left, right, top, bottom := -3.3, 1.7, -1.7, 1.7
	if kind == spellLight {
		left = -7
	}
	if kind == spellLash {
		right = 2.8
	}
	left += (-1.7 - left) * head
	phase := r.weaponMaterialClock()
	axial := 1.0
	if directionalSpellKind(kind) {
		axial = projection.axial
	}
	for i, p := range [4][2]float64{{left, top}, {right, top}, {left, bottom}, {right, bottom}} {
		v := weaponMaterialVertex(x+(p[0]*axial*dx-p[1]*dy)*size, y+(p[0]*axial*dy+p[1]*dx)*size, p[0], p[1], rgb, alpha)
		v.Custom0, v.Custom1 = float32(phase), float32(uint(seed)%997)*.031
		v.Custom2, v.Custom3 = float32(head), float32(kind)+.25
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, r.spellBodyShader, &r.weaponMaterialOpts)
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
	worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, r.impactMaterialShader, &r.weaponMaterialOpts)
}
