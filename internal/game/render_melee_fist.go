package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type gauntletFace struct {
	a, b, c, origin [3]float64
	rgb             [3]int
}

// Build each rigid model once. Animation transforms whole solids; fingers and
// fittings never change shape between poses. Local +X is the striking axis.
var gauntletModels = map[int][]gauntletFace{50: buildGauntlet(50), 52: buildGauntlet(52)}

func buildGauntlet(model int) []gauntletFace {
	var mesh []gauntletFace
	metal, gold, leather, seam := [3]int{120, 133, 144}, [3]int{196, 141, 57}, [3]int{116, 28, 23}, [3]int{49, 43, 36}
	if model == 50 {
		metal = [3]int{180, 112, 42}
	}
	box := func(c, h [3]float64, bevel, roll float64, rgb [3]int) {
		beveledBoxFaces(c, h, bevel, roll, func(a, b, d, o [3]float64) { mesh = append(mesh, gauntletFace{a, b, d, o, rgb}) })
	}

	// Elliptical sections form rounded finger joints and an open flared cuff.
	type section struct{ x, w, h float64 }
	tube := func(sections []section, cy, cz float64, rgb [3]int) {
		const sides = 12
		emit := func(a, b, c [3]float64) {
			mesh = append(mesh, gauntletFace{a, b, c, [3]float64{(a[0] + b[0] + c[0]) / 3, cy, cz}, rgb})
		}
		for i := 0; i < len(sections)-1; i++ {
			p, q := sections[i], sections[i+1]
			for j := 0; j < sides; j++ {
				a, b := 2*math.Pi*float64(j)/sides, 2*math.Pi*float64(j+1)/sides
				v0 := [3]float64{p.x, cy + p.w*math.Cos(a), cz + p.h*math.Sin(a)}
				v1 := [3]float64{p.x, cy + p.w*math.Cos(b), cz + p.h*math.Sin(b)}
				v2 := [3]float64{q.x, cy + q.w*math.Cos(a), cz + q.h*math.Sin(a)}
				v3 := [3]float64{q.x, cy + q.w*math.Cos(b), cz + q.h*math.Sin(b)}
				emit(v0, v1, v2)
				emit(v2, v1, v3)
			}
		}
	}
	tube([]section{{-.96, .35, .22}, {-.78, .49, .30}, {-.35, .55, .32}, {-.04, .44, .22}}, 0, 0, seam)
	tube([]section{{-.9, .34, .235}, {-.72, .49, .31}, {-.36, .535, .33}, {-.22, .50, .29}}, 0, -.015, metal)
	for i := 0; i < 4; i++ {
		y := -.435 + float64(i)*.29
		x := -.045 * math.Abs(float64(i)-1.4)
		tube([]section{{x - .30, .11, .13}, {x - .21, .142, .195}, {x + .08, .145, .20}, {x + .23, .10, .145}, {x + .27, 0, 0}}, y, -.06, metal)
		// Dark joint seam and a short overlapping fingertip curled into the palm.
		tube([]section{{x - .27, .125, .10}, {x - .16, .125, .12}, {x - .03, .08, .09}, {x + .01, 0, 0}}, y, .20, metal)
		if model == 52 {
			tube([]section{{x + .02, .147, .202}, {x + .055, .148, .205}}, y, -.06, gold)
		}
	}
	box([3]float64{-.46, .47, .16}, [3]float64{.25, .14, .16}, .095, -.3, metal)
	box([3]float64{-.25, .35, .27}, [3]float64{.17, .21, .125}, .085, -.3, metal)
	// Open back: the inside has an inward-facing tube and a dark recess.
	tube([]section{{-1.70, .43, .32}, {-1.62, .44, .33}, {-1.18, .34, .25}, {-.96, .35, .25}}, 0, .02, leather)
	// Inner cuff uses an outside origin to retain its inward-facing normals.
	begin := len(mesh)
	tube([]section{{-1.70, .35, .245}, {-1.20, .27, .18}}, 0, .02, [3]int{42, 23, 18})
	for i := begin; i < len(mesh); i++ {
		f := &mesh[i]
		f.origin = [3]float64{f.a[0], f.a[1] * 3, f.a[2] * 3}
	}
	// A narrow annular rim, not a solid cap over the wrist opening.
	for _, x := range []float64{-1.70, -1.14} {
		w, h := .44, .33
		if x > -1.5 {
			w, h = .36, .27
		}
		tube([]section{{x - .02, w - .06, h - .06}, {x - .04, w, h}, {x + .055, w, h}, {x + .08, w - .02, h - .02}}, 0, .02, gold)
	}
	for _, x := range []float64{-1.49, -1.38, -1.27} {
		w := .34 + (-1.18-x)*.21
		h := .25 + (-1.18-x)*.17
		tube([]section{{x - .014, w + .009, h + .009}, {x + .014, w + .009, h + .009}}, 0, .02, [3]int{160, 40, 27})
	}
	for _, y := range []float64{-.30, .30} {
		box([3]float64{-1.14, y, -.16}, [3]float64{.052, .044, .052}, .029, 0, gold)
	}
	if model == 52 {
		tube([]section{{-1.62, .432, .322}, {-1.45, .397, .294}}, 0, .02, metal)
		box([3]float64{-.60, 0, -.335}, [3]float64{.145, .16, .035}, .028, 0, gold)
		box([3]float64{-.60, 0, -.375}, [3]float64{.075, .085, .025}, .02, 0, metal)
	}

	return mesh
}

func (r *Renderer) drawGauntletStrike(dst *ebiten.Image, s SlashEffect, cx, cy, height float64) {
	model, ok := weaponBodyModels[s.WeaponKey]
	if !ok || gauntletModels[model] == nil {
		return
	}
	p, _, _, _ := meleeFxTiming(s)
	scale := height * meleeSizeScale
	if s.Crit {
		scale *= 1.2
	}
	for hand := 0; hand < 2; hand++ {
		t := (p - float64(hand)*.14) / .72
		if t < 0 || t >= 1 {
			continue
		}
		extension := math.Sin(math.Pi * math.Min(1, t/.62))
		if t >= .62 {
			extension = 0
		}
		side := 1.0
		if hand == 1 {
			side = -1
		}
		// The fist extends away from the eye, rotates knuckles down, then retracts.
		x := cx + side*scale*(.22-.095*extension)
		y := cy + scale*(.15-.29*extension)
		size := scale * (.115 - .028*extension)
		alpha := math.Min(1, t/.07) * math.Min(1, (1-t)/.18)
		axis := [3]float64{-side * (.40 - .21*extension), -.36 + .18*extension, .86}
		r.drawLitVolume(dst, x, y, size, axis, [3]int{}, alpha, model, side)
	}
}
