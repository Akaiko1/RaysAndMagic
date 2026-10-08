package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

func isFacetedBolt(kind int) bool {
	return kind == spellIce || kind == spellVoidNeedle || kind == spellShadow || kind == spellFireDart
}

// All directional volumes share this projection: their visible faces turn with
// the flight axis and hand convergence. Each triangle receives shader lighting.
func (r *Renderer) drawLitVolume(dst *ebiten.Image, x, y, size float64, axis [3]float64, rgb [3]int, alpha float64, kind int, handedness float64) {
	if size <= 0 || alpha <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	length := math.Sqrt(axis[0]*axis[0] + axis[1]*axis[1] + axis[2]*axis[2])
	if length < .001 {
		axis = [3]float64{0, 0, 1}
		length = 1
	}
	ax, ay, az := axis[0]/length, axis[1]/length, axis[2]/length
	bx, by, bz := az, 0.0, -ax
	bl := math.Hypot(bx, bz)
	if bl < .001 {
		bx, by, bz = 1, 0, 0
		bl = 1
	}
	bx, by, bz = bx/bl, by/bl, bz/bl
	cx, cy, cz := ay*bz-az*by, az*bx-ax*bz, ax*by-ay*bx
	transform := volumeTransform{rotation: [3][3]float64{
		{ax, bx * handedness, cx},
		{ay, by * handedness, cy},
		{az, bz * handedness, cz},
	}, uvY: handedness}
	if r.spellBoltOpts.Uniforms == nil {
		r.spellBoltOpts.Uniforms = map[string]any{"Time": r.spellBoltTime[:]}
	}
	r.spellBoltTime[0] = float32(r.weaponMaterialClock())
	var mesh *volumeMesh
	isGauntlet := isGauntletModel(kind)
	if isGauntlet {
		rgb = [3]int{255, 255, 255}
		mesh = gauntletModels.get(kind, buildGauntlet)
	} else {
		mesh = directionalVolumeMeshes.get(directionalVolumeModel(kind), buildDirectionalVolumeMesh)
	}
	// Equipment is an opaque solid; spell volumes retain their authored
	// per-face energy transparency rather than adopting a metal body's fade.
	r.drawCameraVolume(dst, x, y, size, alpha, transform, r.spellBoltShader, &r.spellBoltOpts, mesh, rgb, kind, isGauntlet)
}

// Existing spell solids are immutable. Only their orientation and tint change.
var directionalVolumeMeshes volumeMeshCache

// Unknown bolt kinds keep the original generic bipyramid instead of vanishing.
func directionalVolumeModel(kind int) int {
	if kind == spellHarm || isFacetedBolt(kind) {
		return kind
	}
	return 0
}
func buildDirectionalVolumeMesh(kind int) []modelFace {
	var mesh []modelFace
	face := func(a, b, c, o [3]float64) { mesh = append(mesh, modelFace{a, b, c, o, [3]int{255, 255, 255}, 0}) }
	if kind == spellHarm {
		// Chamfered solids form a clenched hand: palm, four folded fingers, a thumb
		// crossing their lower joints, and a short wrist behind the knuckles.
		addBox := func(center, half [3]float64, bevel, roll float64) {
			beveledBoxFaces(center, half, bevel, roll, face)
		}
		addBox([3]float64{-.60, 0, .02}, [3]float64{.42, .61, .35}, .13, 0)
		addBox([3]float64{-1.10, 0, .04}, [3]float64{.23, .40, .27}, .10, 0)
		for k := 0; k < 4; k++ {
			f := float64(k)
			u := .02 - .045*math.Abs(f-1.4)
			across := -.46 + f*.30
			addBox([3]float64{u, across, -.12}, [3]float64{.26, .137, .22}, .075, 0)
			addBox([3]float64{u - .055, across, .18}, [3]float64{.18, .13, .115}, .05, 0)
		}
		addBox([3]float64{-.31, .25, .35}, [3]float64{.19, .36, .14}, .095, -.23)
	} else {
		radius, tail, tip := .17, -1.45, 1.65
		if kind == spellVoidNeedle {
			radius = .14
			tail = -1.7
			tip = 1.9
		}
		if kind == spellShadow {
			radius = .22
			tail = -1.8
			tip = 1.4
		}
		if kind == spellFireDart {
			radius = .20
			tail = -1.8
			tip = 1.25
		}
		rear, nose := [3]float64{tail, 0, 0}, [3]float64{tip, 0, 0}
		for k := 0; k < 6; k++ {
			a0, a1 := float64(k)*math.Pi/3+.22, float64(k+1)*math.Pi/3+.22
			a, b := [3]float64{-.85, math.Cos(a0) * radius, math.Sin(a0) * radius}, [3]float64{-.85, math.Cos(a1) * radius, math.Sin(a1) * radius}
			face(rear, b, a, [3]float64{})
			face(a, b, nose, [3]float64{})
		}
	}
	return mesh
}
