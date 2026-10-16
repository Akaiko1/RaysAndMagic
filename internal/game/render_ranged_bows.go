package game

import "math"

// Bows use X for limb height and Y for the shot axis. Crossbows use the same
// local axes, but their model-to-camera transform keeps X horizontal. Every
// builder returns the rest pose; rangedLimbs flexes it and adds the strings.

func rangedBowLaminate(m *weaponMeshBuilder, side float64, profile []rangedSection, col, trim [3]int, material int) {
	p := append([]rangedSection(nil), profile...)
	for i := range p {
		p[i].p[0] *= side
	}
	rangedTube(m, rangedSmooth(p), col, material)
	for _, face := range []float64{-1, 1} {
		line := append([]rangedSection(nil), p...)
		for i := range line {
			line[i].p[2] = face * line[i].depth
			line[i].depth = .004
			line[i].width = math.Min(.009, line[i].width*.3)
		}
		rangedTube(m, rangedSmooth(line), trim, material)
	}
}
func buildLongBowHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	wood, grain := [3]int{139, 76, 28}, [3]int{203, 131, 61}
	p := []rangedSection{{[3]float64{.18, 0, 0}, .060, .040}, {[3]float64{.42, .075, 0}, .052, .035}, {[3]float64{.81, .18, 0}, .044, .030}, {[3]float64{1.20, .17, 0}, .031, .024}, {[3]float64{1.58, .04, 0}, .020, .017}, {[3]float64{1.89, -.12, 0}, .010, .010}}
	for _, side := range []float64{-1, 1} {
		rangedBowLaminate(&m, side, p, wood, grain, meshGrip)
		// The narrow grain follows the stave's taper rather than reading as
		// an extra metal strip on a plain wooden bow.
		for _, face := range []float64{-1, 1} {
			line := append([]rangedSection(nil), p...)
			for i := range line {
				line[i].p[0] *= side
				line[i].p[1] += line[i].width * .32
				line[i].p[2] = face * (line[i].depth*.93 + .002)
				line[i].width, line[i].depth = .0025, .0015
			}
			rangedTube(&m, rangedSmooth(line), [3]int{110, 56, 24}, meshGrip)
		}
		// Fine sapwood edges and two silk nock bindings, with no recurved metal tips.
		for _, x := range []float64{1.72, 1.80} {
			y := -.04 - (x-1.72)*.6
			m.rod([3]float64{side * x, y, 0}, [3]float64{side * (x + .03), y - .018, 0}, .027, .024, [3]int{177, 144, 74}, meshGrip)
		}
	}
	rangedLathe(&m, [][2]float64{{-.25, .048}, {-.22, .064}, {-.16, .069}, {.16, .069}, {.22, .064}, {.25, .048}}, [3]int{63, 39, 25}, meshGrip)
	for _, x := range []float64{-.245, .215} {
		rangedLathe(&m, [][2]float64{{x, .051}, {x + .007, .069}, {x + .024, .069}, {x + .03, .051}}, [3]int{132, 112, 73}, meshMetal)
	}
	rangedHelix(&m, -.20, .20, .070, .008, 7, [3]int{92, 58, 31}, meshGrip)
	return m.faces
}
func buildElvenBowHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	wood, gold := [3]int{192, 137, 65}, [3]int{229, 180, 70}
	p := []rangedSection{{[3]float64{.22, 0, 0}, .085, .049}, {[3]float64{.48, .17, 0}, .082, .041}, {[3]float64{.80, .25, 0}, .072, .033}, {[3]float64{1.12, .21, 0}, .052, .027}, {[3]float64{1.43, .01, 0}, .030, .021}, {[3]float64{1.59, -.21, 0}, .014, .012}}
	for _, side := range []float64{-1, 1} {
		rangedBowLaminate(&m, side, p, wood, gold, meshGrip)
		for _, anchor := range [][2]float64{{.27, .06}, {1.56, -.19}} {
			x, y := anchor[0], anchor[1]
			var curl [][3]float64
			var widths []float64
			for i := 0; i <= 20; i++ {
				t := float64(i) / 20
				a := t * math.Pi * 2.1
				r := .11 * (1 - t*.8)
				curl = append(curl, [3]float64{side * (x + math.Sin(a)*r), y - .09 + math.Cos(a)*r, -.018})
				widths = append(widths, .020*(1-t*.7))
			}
			rangedMetalCurve(&m, curl, widths, gold, meshMetal)
		}
		// Small clusters sprout from the nock and limb root, with slender leaf veins.
		for _, v := range [][4]float64{{.42, .17, .56, .44}, {.47, .18, .74, .39}, {.54, .19, .85, .30}, {1.57, -.15, 1.87, -.08}, {1.59, -.17, 1.80, -.34}, {1.55, -.18, 1.56, -.44}} {
			rangedLeaf(&m, [3]float64{side * v[0], v[1], -.062}, [3]float64{side * v[2], v[3], -.065}, .049)
		}
		// Engraved gold roots join the wooden limbs to the grip. Their relief
		// sits above both faces; it cannot disappear inside the laminate.
		for _, face := range []float64{-1, 1} {
			rangedMetalCurve(&m, [][3]float64{{side * .23, .01, face * .053}, {side * .33, .12, face * .058}, {side * .49, .19, face * .053}, {side * .57, .22, face * .042}}, []float64{.018, .021, .014, .002}, gold, meshMetal)
			rangedMetalCurve(&m, [][3]float64{{side * .24, -.018, face * .054}, {side * .39, .08, face * .057}, {side * .49, .10, face * .049}}, []float64{.008, .010, .002}, [3]int{129, 86, 30}, meshMetal)
		}
	}
	m.grip(-.20, .20, 0, .069, [3]int{69, 89, 34}, gold)
	rangedHelix(&m, -.17, .17, .074, .008, 5, [3]int{148, 158, 70}, meshGrip)
	return m.faces
}
func buildHellfireHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	red, edge := [3]int{103, 22, 29}, [3]int{143, 55, 49}
	// One forged silhouette per limb: a long hooked blade with swept backward
	// barbs. Molten channels are part of the surface, never bright rods laid on it.
	outline := [][2]float64{{.16, -.055}, {.39, .015}, {.62, .10}, {.84, .155}, {1.12, .16}, {1.40, .045}, {1.62, -.15}, {1.82, -.22}, {1.70, -.035}, {1.61, .035}, {1.55, .17}, {1.43, .13}, {1.25, .30}, {1.29, .39}, {1.12, .31}, {1.01, .30}, {.91, .39}, {.91, .49}, {.72, .35}, {.60, .30}, {.50, .43}, {.45, .26}, {.27, .29}, {.30, .14}, {.16, .08}}
	for _, side := range []float64{-1, 1} {
		p := make([][2]float64, len(outline))
		for i, v := range outline {
			p[i] = [2]float64{side * v[0], v[1]}
		}
		rangedStockPlate(&m, p, 0, .041, .30, red, meshHellfireMetal)
		// Narrow forged edge follows the outer hook without rounding it into a tube.
		ridge := [][3]float64{{side * .30, .14, -.045}, {side * .60, .25, -.046}, {side * .94, .29, -.045}, {side * 1.24, .25, -.042}, {side * 1.51, .11, -.035}, {side * 1.74, -.12, -.018}, {side * 1.82, -.22, 0}}
		for _, face := range []float64{-1, 1} {
			q := append([][3]float64(nil), ridge...)
			for i := range q {
				q[i][2] *= face
			}
			rangedMetalCurve(&m, q, []float64{.009, .010, .010, .009, .008, .005, .001}, edge, meshMetal)
		}
	}
	m.grip(-.20, .20, 0, .056, [3]int{55, 13, 18}, red)
	rangedHelix(&m, -.18, .18, .058, .007, 6, [3]int{117, 32, 31}, meshGrip)
	return m.faces
}
func buildWyrmHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	shell, rim := [3]int{60, 66, 45}, [3]int{132, 119, 78}
	p := []rangedSection{{[3]float64{.19, 0, 0}, .083, .054}, {[3]float64{.48, .15, 0}, .083, .048}, {[3]float64{.89, .29, 0}, .069, .043}, {[3]float64{1.22, .22, 0}, .057, .035}, {[3]float64{1.51, -.03, 0}, .03, .02}, {[3]float64{1.69, -.09, 0}, .002, .002}}
	for _, side := range []float64{-1, 1} {
		rangedBowLaminate(&m, side, p, shell, rim, meshStone)
		// Dark overlapping chitin owns the silhouette. Green scales form only
		// the narrow belly, as on the icon, rather than covering the whole limb.
		for i := 0; i < 10; i++ {
			t := float64(i) / 9
			x := .31 + t*1.16
			section := 1
			for section < len(p)-1 && p[section].p[0] < x {
				section++
			}
			a, b := p[section-1], p[section]
			u := (x - a.p[0]) / (b.p[0] - a.p[0])
			y, w, z := a.p[1]+u*(b.p[1]-a.p[1]), a.width+u*(b.width-a.width), a.depth+u*(b.depth-a.depth)
			for _, face := range []float64{-1, 1} {
				outer := [][2]float64{{side * (x - .07), y - w*.30}, {side * (x + .02), y - w*.58}, {side * (x + .09), y - w*.08}, {side * (x + .10), y + w*.45}, {side * (x - .005), y + w*.96}, {side * (x - .065), y + w*.52}}
				rangedStockPlate(&m, outer, face*z, .014, .32, [3]int{83 + i%3*8, 78 + i%3*7, 59}, meshStone)
				// A low ridge folds each overlapping shell instead of leaving a
				// flat tile on the limb. The green belly scales have pointed ends.
				rangedMetalCurve(&m, [][3]float64{{side * (x - .05), y + w*.42, face * (z + .014)}, {side * x, y + w*.20, face * (z + .032)}, {side * (x + .075), y + w*.12, face * (z + .014)}}, []float64{.003, .009, .002}, rim, meshStone)
				belly := [][2]float64{{side * (x - .05), y - w*.50}, {side * (x + .01), y - w*.49}, {side * (x + .055), y - w*.80}, {side * (x + .01), y - w*1.02}, {side * (x - .045), y - w*.92}}
				rangedStockPlate(&m, belly, face*(z+.016), .009, .22, [3]int{93 + i%3*12, 137 + i%2*13, 30}, meshStone)
				m.rod([3]float64{side * (x - .059), y - w*.35, face * (z + .03)}, [3]float64{side * (x - .066), y + w*.55, face * (z + .03)}, .004, .003, rim, meshMetal)
			}
			if i%2 == 0 {
				rangedMetalCurve(&m, [][3]float64{{side * x, y + .04, 0}, {side * (x - .015), y + .15, 0}, {side * (x + .09), y + .22, 0}}, []float64{.032, .020, .001}, meshBone, meshStone)
			}
		}
		rangedMetalCurve(&m, [][3]float64{{side * 1.37, .10, 0}, {side * 1.55, -.02, 0}, {side * 1.74, .04, 0}, {side * 1.82, .16, 0}}, []float64{.047, .041, .026, .001}, meshGold, meshMetal)
	}
	m.grip(-.21, .21, 0, .071, [3]int{112, 34, 24}, rim)
	rangedHelix(&m, -.19, .19, .075, .009, 7, [3]int{173, 65, 34}, meshGrip)
	return m.faces
}
func buildCompoundHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	metal, edge, cyan := [3]int{49, 59, 68}, [3]int{120, 138, 145}, [3]int{46, 184, 197}
	for _, side := range []float64{-1, 1} {
		// Flat machined webs surround three open windows in each riser. The
		// web shoulders and bolts remain separate from the split carbon limbs.
		for _, z := range []float64{-.055, .055} {
			webs := [][][2]float64{
				{{.17, -.035}, {.23, -.025}, {.39, .13}, {.58, .135}, {.79, .035}, {.85, .08}, {.60, .195}, {.35, .19}},
				{{.20, .065}, {.25, .055}, {.44, .25}, {.63, .24}, {.83, .10}, {.89, .145}, {.66, .30}, {.41, .305}},
				{{.34, .16}, {.38, .14}, {.46, .27}, {.42, .29}},
				{{.53, .16}, {.57, .16}, {.59, .27}, {.55, .28}},
				{{.70, .12}, {.74, .105}, {.80, .20}, {.76, .23}},
			}
			for _, web := range webs {
				for i := range web {
					web[i][0] *= side
				}
				rangedStockPlate(&m, web, z, .023, .32, edge, meshMetal)
			}
			rangedTube(&m, rangedSmooth([]rangedSection{{[3]float64{side * .80, .11, z}, .043, .022}, {[3]float64{side * 1.12, .21, z}, .035, .021}, {[3]float64{side * 1.48, .10, z}, .028, .018}}), meshDark, meshMetal)
			face := math.Copysign(1, z)
			for _, v := range [][2]float64{{.25, .055}, {.80, .13}} {
				m.rod([3]float64{side * v[0], v[1], z}, [3]float64{side * v[0], v[1], z + face*.034}, .031, .028, metal, meshMetal)
				m.rod([3]float64{side * (v[0] - .012), v[1], z + face*.036}, [3]float64{side * (v[0] + .012), v[1], z + face*.036}, .003, .003, meshDark, meshGrip)
			}
			rangedMetalCurve(&m, [][3]float64{{side * .28, .09, z + face*.026}, {side * .37, .17, z + face*.026}, {side * .49, .17, z + face*.026}}, []float64{.004, .006, .002}, cyan, meshGlow)
		}
		c := [3]float64{side * 1.49, .065, 0}
		for _, z := range []float64{-.026, .026} {
			m.ring([3]float64{c[0], c[1], z}, .135, .014, metal)
			m.ring([3]float64{c[0], c[1], z}, .108, .009, edge)
		}
		for i := 0; i < 4; i++ {
			a := float64(i)*math.Pi/2 + .2
			m.rod(c, [3]float64{c[0] + .11*math.Cos(a), c[1] + .11*math.Sin(a), 0}, .011, .011, edge, meshMetal)
		}
		m.rod([3]float64{c[0], c[1], -.08}, [3]float64{c[0], c[1], .08}, .026, .026, meshGold, meshMetal)
		for _, v := range [][2]float64{{.35, .18}, {.74, .13}} {
			m.rod([3]float64{side * v[0], v[1], -.069}, [3]float64{side * (v[0] + .075), v[1] + .025, -.069}, .008, .008, cyan, meshGlow)
		}
		m.rod([3]float64{c[0], c[1] + .09, -.043}, [3]float64{-c[0], c[1] + .09, -.043}, .0035, .0035, meshSteel, meshGrip)
	}
	m.grip(-.19, .19, 0, .063, [3]int{24, 28, 31}, metal)
	// A short stabilizer and a side-mounted sight, with a real open aperture.
	m.rod([3]float64{-.16, .04, 0}, [3]float64{-.16, .58, 0}, .020, .026, metal, meshMetal)
	m.rod([3]float64{.21, .04, 0}, [3]float64{.21, .04, -.27}, .016, .016, metal, meshMetal)
	m.ring([3]float64{.21, .09, -.28}, .092, .013, metal)
	m.rod([3]float64{.21, .01, -.28}, [3]float64{.21, .09, -.28}, .003, .003, cyan, meshGlow)
	return m.faces
}

func buildCrossbowHeldModel(nest bool) []modelFace {
	m := weaponMeshBuilder{}
	wood, gold, steel := [3]int{124, 66, 29}, [3]int{225, 170, 57}, [3]int{183, 198, 210}
	// The stock tapers into a narrower flight rail, with a rounded shoulder heel.
	rangedSidePlate(&m, [][2]float64{{-1.24, -.22}, {-1.25, .12}, {-.88, .16}, {-.55, .08}, {.83, .08}, {.91, -.05}, {-.45, -.13}, {-.90, -.26}}, 0, .115, .16, wood, meshGrip)
	// The arena stock ends in an oval brass heel and central raised boss.
	// A complete end fitting joins the wood instead of two floating side strips.
	if !nest {
		heel := weaponMeshBuilder{}
		rangedGunLathe(&heel, [][2]float64{{-1.29, .12}, {-1.28, .145}, {-1.245, .145}, {-1.23, .12}}, 0, gold, meshMetal)
		rangedGunLathe(&heel, [][2]float64{{-1.298, .102}, {-1.291, .102}}, 0, [3]int{115, 69, 25}, meshGrip)
		rangedGunLathe(&heel, [][2]float64{{-1.318, .025}, {-1.310, .063}, {-1.299, .075}}, 0, gold, meshMetal)
		m.appendMapped(heel.faces, func(p [3]float64) [3]float64 { return [3]float64{p[0], p[1], p[2]*1.35 - .065} })
	} else {
		// The nest's spear-like gold nose extends the rail, with a recessed slot.
		for _, side := range []float64{-1, 1} {
			rangedSidePlate(&m, [][2]float64{{.82, .025}, {.99, .10}, {1.23, .06}, {1.42, .06}, {1.12, -.08}, {.87, -.065}}, side*.047, .018, .30, gold, meshMetal)
		}
	}
	for _, side := range []float64{-1, 1} {
		if nest {
			// Red scale-covered butt with a dark structural frame and brass rim.
			rangedSidePlate(&m, [][2]float64{{-1.24, -.20}, {-1.20, .10}, {-.83, .10}, {-.49, -.04}, {-.64, -.17}, {-.93, -.24}}, side*.114, .018, .10, [3]int{131, 31, 24}, meshStone)
			for row := 0; row < 4; row++ {
				for col := 0; col < 6; col++ {
					y := -1.17 + float64(col)*.085
					z := -.17 + float64(row)*.072
					xx := side * .138
					rangedSidePlate(&m, [][2]float64{{y - .04, z}, {y, z + .037}, {y + .04, z}, {y, z - .028}}, xx, .009, .25, [3]int{170 + row*9, 43 + row*6, 27}, meshStone)
				}
			}
			rangedMetalCurve(&m, [][3]float64{{side * .14, -1.26, -.18}, {side * .14, -1.20, .13}, {side * .12, -.80, .13}, {side * .10, -.43, -.03}}, []float64{.02, .022, .02, .016}, gold, meshMetal)
		} else {
			for j := 0; j < 3; j++ {
				z := -.17 + float64(j)*.10
				m.rod([3]float64{side * .115, -1.15, z}, [3]float64{side * .115, -.57, z + .08}, .004, .003, [3]int{183, 107, 46}, meshGrip)
			}
			rangedMetalCurve(&m, [][3]float64{{side * .117, -1.24, -.20}, {side * .117, -1.24, .12}, {side * .117, -1.09, .14}}, []float64{.027, .027, .015}, gold, meshMetal)
		}
		// Raised brass guide rails leave a dark central channel for the quarrel.
		m.rod([3]float64{side * .076, -.73, .115}, [3]float64{side * .076, .91, .115}, .024, .021, gold, meshMetal)
		m.rod([3]float64{side * .040, -.65, .13}, [3]float64{side * .040, .84, .13}, .009, .009, steel, meshMetal)
		profile := []rangedSection{{[3]float64{side * .10, .62, .02}, .073, .032}, {[3]float64{side * .40, .61, .02}, .078, .030}, {[3]float64{side * .76, .46, .02}, .061, .024}, {[3]float64{side * 1.06, .24, .02}, .034, .019}, {[3]float64{side * 1.17, .12, .02}, .016, .013}}
		limb := steel
		if nest {
			limb = [3]int{63, 71, 48}
		}
		rangedTube(&m, rangedSmooth(profile), limb, meshMetal)
		edge := append([]rangedSection(nil), profile...)
		for i := range edge {
			edge[i].p[2] += .032
			edge[i].width = .01
			edge[i].depth = .005
		}
		rangedTube(&m, rangedSmooth(edge), mixColor(steel, gold, .25), meshMetal)
		tip := profile[len(profile)-1].p
		if nest {
			for i := 0; i < 9; i++ {
				t := float64(i) / 8
				x := side * (.25 + .77*t)
				y := .61 - .36*t*t
				z := .055
				rangedPlate(&m, [][2]float64{{x - side*.06, y - .04}, {x + side*.04, y - .065}, {x + side*.095, y}, {x + side*.035, y + .065}, {x - side*.06, y + .03}}, z, .012, .25, [3]int{68 + int(25*t), 140 - int(23*t), 43}, meshStone)
			}
			rangedMetalCurve(&m, [][3]float64{tip, {tip[0] + side*.08, tip[1] + .03, .035}, {tip[0] + side*.16, tip[1] + .19, .045}}, []float64{.033, .026, .001}, gold, meshMetal)
		} else {
			rangedOrb(&m, tip, [3]float64{.042, .047, .039}, gold, meshMetal)
		}
		if !nest {
			rangedCloth(&m, [][3]float64{{side * .12, -.52, -.06}, {side * .20, -.60, -.20}, {side * .23, -.52, -.39}, {side * .30, -.67, -.48}}, []float64{.04, .05, .045, 0}, [3]int{192, 25, 18})
		}
	}
	for _, y := range []float64{-.72, .69, .86} {
		m.box([3]float64{0, y, .07}, [3]float64{.13, .023, .075}, .01, gold, meshMetal)
	}
	m.box([3]float64{0, .76, .14}, [3]float64{.035, .04, .02}, .008, steel, meshMetal)
	m.curve([3]float64{0, -.55, -.10}, [3]float64{0, -.72, -.40}, [3]float64{0, -.95, -.17}, .016, gold, meshMetal)
	m.rod([3]float64{0, -.64, -.11}, [3]float64{0, -.72, -.24}, .011, .008, steel, meshMetal)
	return m.faces
}
