package game

import "math"

// rangedCurve is a smooth tube through authored points; depth is the section's
// thickness as a share of its radius.
func rangedCurve(m *weaponMeshBuilder, p [][3]float64, radii []float64, depth float64, col [3]int, mat int) {
	s := make([]rangedSection, len(p))
	for i := range p {
		s[i] = rangedSection{p[i], radii[i], radii[i] * depth}
	}
	rangedTube(m, rangedSmooth(s), col, mat)
}

// Metal fittings are slightly flattened curves.
func rangedMetalCurve(m *weaponMeshBuilder, p [][3]float64, radii []float64, col [3]int, mat int) {
	rangedCurve(m, p, radii, .75, col, mat)
}

// Crystal rings make real facets and pointed ends, rather than a beveled box.
func rangedCrystal(m *weaponMeshBuilder, center [3]float64, length, width float64, palette [][3]int, mat int) {
	rings := [][2]float64{{-.55, .01}, {-.28, .78}, {.25, 1}, {.52, .56}, {.83, 0}}
	for i := 1; i < len(rings); i++ {
		for j := 0; j < 6; j++ {
			point := func(k, n int) [3]float64 {
				a := float64(n) * math.Pi / 3
				return [3]float64{center[0] + rings[k][0]*length, center[1] + math.Cos(a)*rings[k][1]*width, center[2] + math.Sin(a)*rings[k][1]*width*.76}
			}
			a, b, c, d := point(i-1, j), point(i-1, j+1), point(i, j), point(i, j+1)
			color := palette[(j+i)%len(palette)]
			m.triangle(a, b, c, center, color, mat)
			m.triangle(c, b, d, center, color, mat)
		}
	}
}

func buildArchmageHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	gold := [3]int{222, 158, 44}
	light := [3]int{255, 214, 102}
	purple := [3]int{90, 39, 139}
	m.rod([3]float64{-2.2, 0, 0}, [3]float64{.54, 0, 0}, .035, .044, purple, meshStone)
	// Gold spiral on the purple shaft, with a pointed butt and narrow collars.
	var spiral []rangedSection
	for i := 0; i <= 90; i++ {
		t := float64(i) / 90
		a := t * math.Pi * 14
		spiral = append(spiral, rangedSection{[3]float64{-2.12 + t*2.57, .062 * math.Cos(a), .062 * math.Sin(a)}, .012, .012})
	}
	rangedTube(&m, spiral, gold, meshMetal)
	for _, x := range []float64{-2.10, -1.92, -.45, .28, .43} {
		m.rod([3]float64{x, 0, 0}, [3]float64{x + .052, 0, 0}, .063, .059, gold, meshMetal)
	}
	m.rod([3]float64{-2.40, 0, 0}, [3]float64{-2.12, 0, 0}, .006, .095, gold, meshMetal)
	// Open gold horns cradle the tall purple crystal; curls remain separated.
	for _, sg := range []float64{-1, 1} {
		rangedMetalCurve(&m, [][3]float64{{.35, 0, 0}, {.59, sg * .19, .015}, {.89, sg * .36, .025}, {1.21, sg * .38, .03}, {1.47, sg * .28, .025}, {1.68, sg * .15, .02}}, []float64{.062, .06, .053, .047, .034, .002}, gold, meshMetal)
		rangedMetalCurve(&m, [][3]float64{{.35, sg * .04, -.065}, {.48, sg * .24, -.075}, {.39, sg * .31, -.075}, {.27, sg * .24, -.06}, {.32, sg * .18, -.04}}, []float64{.031, .029, .023, .018, .004}, light, meshMetal)
		rangedMetalCurve(&m, [][3]float64{{.65, sg * .19, -.04}, {.78, sg * .35, -.06}, {.68, sg * .43, -.045}, {.57, sg * .37, -.04}}, []float64{.025, .027, .022, .004}, gold, meshMetal)
		rangedMetalCurve(&m, [][3]float64{{.88, sg * .30, -.06}, {1.06, sg * .34, -.06}, {1.25, sg * .29, -.04}, {1.38, sg * .21, -.03}}, []float64{.014, .016, .012, .002}, light, meshMetal)
	}
	rangedCrystal(&m, [3]float64{1.10, 0, 0}, .75, .215, [][3]int{{117, 47, 181}, {182, 96, 231}, {233, 166, 255}, {92, 30, 142}, {150, 68, 214}, {215, 137, 255}}, meshStone)
	// A few short pale seams describe the crystal's real planes.
	m.rod([3]float64{.91, -.11, -.142}, [3]float64{1.28, -.18, -.14}, .008, .005, [3]int{240, 190, 255}, meshGlow)
	m.rod([3]float64{1.28, -.18, -.14}, [3]float64{1.70, 0, 0}, .006, .002, [3]int{240, 190, 255}, meshGlow)
	return m.faces
}

func buildSunHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	bronze, gold, dark := [3]int{175, 91, 29}, [3]int{244, 176, 57}, [3]int{37, 47, 61}
	rangedLathe(&m, [][2]float64{{-2.22, .04}, {-.4, .05}, {.47, .062}}, dark, meshMetal)
	for _, x := range []float64{-2.15, -1.55, -.94, -.34, .32, .43} {
		rangedLathe(&m, [][2]float64{{x, .051}, {x + .02, .071}, {x + .07, .071}, {x + .09, .051}}, bronze, meshMetal)
		rangedLathe(&m, [][2]float64{{x + .025, .073}, {x + .04, .073}}, gold, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{-2.51, 0}, {-2.29, .105}, {-2.17, .064}}, gold, meshMetal)
	// The long open prongs turn around the lava core, with bright inner
	// bevels and a dark outer rib. The basket is open above and between ribs.
	for i := 0; i < 3; i++ {
		a := float64(i)*2*math.Pi/3 + .43
		cy, cz := math.Cos(a), math.Sin(a)
		var p [][3]float64
		for _, v := range [][2]float64{{.40, .025}, {.57, .15}, {.80, .30}, {1.09, .36}, {1.32, .28}, {1.54, .18}, {1.67, .24}} {
			p = append(p, [3]float64{v[0], v[1] * cy, v[1] * cz})
		}
		rangedMetalCurve(&m, p, []float64{.06, .065, .066, .055, .038, .019, .001}, bronze, meshMetal)
		for j := range p {
			p[j][1] -= cy * .025
			p[j][2] -= cz * .025
		}
		rangedMetalCurve(&m, p, []float64{.018, .023, .022, .018, .013, .006, .001}, gold, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{.44, .066}, {.52, .116}, {.59, .108}, {.66, .061}}, bronze, meshMetal)
	// A small irregular ember remains in the basket before charging. Its crust
	// and hot seams share the weapon's depth, so the front prongs occlude it.
	rangedSpheroid(&m, [3]float64{.99, 0, 0}, [3]float64{.25, .205, .19}, 8, 12, func(lat, lon float64) float64 {
		return 1 + .09*math.Sin(lat*4+lon*3)*math.Sin(lat)
	}, false, [3]int{255, 96, 12}, meshHeldLava)
	return m.faces
}

// Duskneedle's outer limbs are scalloped chitin carrying pale moth scales.
// The membrane is part of its icon; its flying moth wake remains the shipped FX.
func buildDuskHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	purple := [3]int{81, 61, 93}
	edge := [3]int{145, 116, 153}
	ivory := [3]int{203, 189, 175}
	for _, sg := range []float64{-1, 1} {
		base := [][2]float64{{.12, 0}, {.34, .04}, {.52, .10}, {.64, .04}, {.82, .16}, {.94, .13}, {1.02, .26}, {1.20, .35}, {1.43, .31}, {1.60, .12}, {1.68, -.04}, {1.77, -.06}}
		p := make([][3]float64, len(base))
		for i, v := range base {
			p[i] = [3]float64{sg * v[0], v[1], 0}
		}
		radii := []float64{.065, .05, .049, .032, .04, .034, .06, .075, .07, .048, .03, .002}
		rangedMetalCurve(&m, p, radii, purple, meshStone)
		ridge := append([][3]float64(nil), p...)
		for i := range ridge {
			ridge[i][2] = -.045
		}
		thin := make([]float64, len(ridge))
		for i := range thin {
			thin[i] = .012
		}
		thin[len(thin)-1] = .002
		rangedMetalCurve(&m, ridge, thin, edge, meshMetal)
		// Broad upper wing membrane with scalloped edge and pale individual scale fans.
		outline := [][2]float64{{1.04, .25}, {1.15, .18}, {1.20, .11}, {1.29, .13}, {1.33, .07}, {1.41, .12}, {1.45, .10}, {1.52, .17}, {1.59, .12}, {1.43, .31}, {1.20, .35}}
		center := [3]float64{sg * 1.32, .22, 0}
		points := make([][3]float64, len(outline))
		for i, v := range outline {
			points[i] = [3]float64{sg * v[0], v[1], 0}
		}
		for _, side := range []float64{-1, 1} {
			c := center
			c[2] = side * .026
			for i, a := range points {
				b := points[(i+1)%len(points)]
				m.triangle(a, b, c, center, mixColor(purple, edge, float64(i%3)*.13), meshStone)
			}
		}
		for i := 0; i < 6; i++ {
			x := 1.13 + float64(i)*.065
			y := .31 - .03*float64(i)/6
			a := [3]float64{sg * x, y, -.035}
			b := [3]float64{sg * (x + .055), y - .11 + float64(i%2)*.025, -.036}
			m.rod(a, b, .009, .003, ivory, meshStone)
			a[2] = .035
			b[2] = .036
			m.rod(a, b, .009, .003, ivory, meshStone)
		}
		// Small inward hooks, visible even when the wing is nearly edge-on.
		for _, v := range [][2]float64{{.55, .10}, {.85, .16}} {
			rangedMetalCurve(&m, [][3]float64{{sg * v[0], v[1], 0}, {sg * (v[0] + .06), v[1] - .09, .01}, {sg * (v[0] + .13), v[1] - .12, .015}}, []float64{.03, .018, .002}, edge, meshStone)
		}
	}
	m.grip(-.16, .16, 0, .063, [3]int{43, 33, 49}, edge)

	return m.faces
}

func buildArenaHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	horn, gold, light := [3]int{175, 110, 31}, [3]int{236, 173, 49}, [3]int{255, 214, 107}
	profile := []rangedSection{{[3]float64{.22, .005, 0}, .082, .052}, {[3]float64{.44, .12, 0}, .078, .045}, {[3]float64{.73, .21, 0}, .062, .038}, {[3]float64{1.02, .18, 0}, .044, .03}, {[3]float64{1.31, -.015, 0}, .025, .019}, {[3]float64{1.48, -.18, 0}, .016, .014}}
	for _, side := range []float64{-1, 1} {
		rangedBowLaminate(&m, side, profile, horn, light, meshMetal)
		rangedMetalCurve(&m, [][3]float64{{side * 1.27, .02, 0}, {side * 1.45, -.14, 0}, {side * 1.53, -.16, 0}, {side * 1.56, -.08, 0}, {side * 1.50, -.045, 0}}, []float64{.03, .028, .026, .020, .008}, gold, meshMetal)
		for _, x := range []float64{.23, .28} {
			rangedLathe(&m, [][2]float64{{side * x, .09}, {side * (x + .025), .10}, {side * (x + .05), .09}}, gold, meshMetal)
		}
	}
	m.grip(-.22, .22, 0, .077, [3]int{151, 26, 15}, gold)
	rangedHelix(&m, -.20, .20, .083, .012, 7, [3]int{217, 53, 23}, meshGrip)
	return m.faces
}
