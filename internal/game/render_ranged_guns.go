package game

import "math"

// Barrels and optics share axial machining, not a common weapon silhouette.
// Local +Y points downrange, +X crosses the weapon and +Z is its top.
func rangedGunLathe(m *weaponMeshBuilder, profile [][2]float64, z float64, col [3]int, material int) {
	part := weaponMeshBuilder{}
	rangedLathe(&part, profile, col, material)
	m.appendMapped(part.faces, func(p [3]float64) [3]float64 { return [3]float64{p[1], p[0], z + p[2]} })
}
func rangedBarrel(m *weaponMeshBuilder, rear, front, radius, z float64, col [3]int) {
	rangedGunLathe(m, [][2]float64{{rear, radius}, {front - .025, radius}, {front, radius * .94}, {front, radius * .69}, {front - .08, radius * .69}}, z, col, meshMetal)
	// Recessed dark end behind the inner wall; the muzzle lip remains open.
	m.rod([3]float64{0, front - .082, z}, [3]float64{0, front - .081, z}, radius*.67, radius*.67, [3]int{8, 13, 16}, meshGrip)
}
func rangedGunScrew(m *weaponMeshBuilder, c [3]float64, r float64, col [3]int) {
	m.rod([3]float64{c[0] - .008, c[1], c[2]}, [3]float64{c[0] + .008, c[1], c[2]}, r, r, col, meshMetal)
	m.rod([3]float64{c[0] + .009, c[1] - r*.55, c[2]}, [3]float64{c[0] + .009, c[1] + r*.55, c[2]}, .003, .003, meshDark, meshGrip)
	m.rod([3]float64{c[0] - .009, c[1] - r*.55, c[2]}, [3]float64{c[0] - .009, c[1] + r*.55, c[2]}, .003, .003, meshDark, meshGrip)
}
func buildBlowgunHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	bamboo, gold, dark := [3]int{138, 132, 48}, [3]int{205, 152, 62}, [3]int{62, 82, 29}
	// A continuous hollow tube includes both lips; neither mouth has a cap.
	rangedGunLathe(&m, [][2]float64{{-1.46, .070}, {-1.43, .057}, {1.43, .057}, {1.47, .070}, {1.47, .040}, {-1.46, .040}, {-1.46, .070}}, 0, bamboo, meshGrip)
	// Natural bamboo nodes have a darker seam and a pale raised lip.
	for _, y := range []float64{-.82, -.03, .81} {
		rangedGunLathe(&m, [][2]float64{{y - .018, .059}, {y, .073}, {y + .045, .073}, {y + .062, .059}}, 0, dark, meshGrip)
		rangedGunLathe(&m, [][2]float64{{y + .025, .074}, {y + .04, .074}}, 0, gold, meshGrip)
	}

	for j := 0; j < 5; j++ {
		a := float64(j) * 2 * math.Pi / 5
		m.rod([3]float64{.057 * math.Cos(a), -1.36, .057 * math.Sin(a)}, [3]float64{.057 * math.Cos(a), 1.38, .057 * math.Sin(a)}, .002, .002, [3]int{179, 169, 65}, meshGrip)
	}
	for _, y := range []float64{-1.02, .24} {
		for j := 0; j < 3; j++ {
			v := y + float64(j)*.036
			rangedGunLathe(&m, [][2]float64{{v, .067}, {v + .017, .067}}, 0, gold, meshGrip)
		}
	}
	// Two leaves at the mouth binding and the small feather charm at the center.
	for _, v := range [][2]float64{{-.20, -.73}, {.17, -.72}} {
		rangedLeaf(&m, [3]float64{0, -.94, 0}, [3]float64{v[0], v[1], -.03}, .036)
	}
	m.rod([3]float64{.04, .26, 0}, [3]float64{.065, .15, -.28}, .005, .005, [3]int{83, 60, 28}, meshGrip)
	rangedSidePlate(&m, [][2]float64{{.15, -.22}, {.21, -.32}, {.18, -.48}, {.10, -.41}, {.09, -.31}}, .065, .013, .25, gold, meshGrip)
	for i := 0; i < 5; i++ {
		z := -.27 - float64(i)*.032
		m.rod([3]float64{.081, .14, z}, [3]float64{.081, .20, z - .025}, .003, .001, [3]int{112, 75, 28}, meshGrip)
	}
	return m.faces
}

func buildMatchlockHeldModel(variant int) []modelFace {
	if variant == 2 {
		return buildInazumaHeldModel()
	}
	m := weaponMeshBuilder{}
	wood, gold, steel := [3]int{125, 60, 27}, [3]int{207, 151, 54}, [3]int{141, 157, 170}
	if variant == 0 {
		rangedSidePlate(&m, [][2]float64{{-1.49, -.39}, {-1.48, -.08}, {-.96, -.025}, {-.63, .035}, {-.42, .035}, {-.12, -.065}, {.62, -.04}, {.69, -.12}, {-.45, -.145}, {-.84, -.25}}, 0, .061, .12, wood, meshGrip)
		rangedBarrel(&m, -.62, 1.58, .039, .052, steel)
		for _, side := range []float64{-1, 1} {
			m.rod([3]float64{side * .067, -1.46, -.36}, [3]float64{side * .067, -1.46, -.10}, .014, .014, gold, meshMetal)
			rangedMetalCurve(&m, [][3]float64{{side * .063, -1.37, -.32}, {side * .063, -1.03, -.20}, {side * .064, -.77, -.14}, {side * .064, -.51, -.07}}, []float64{.003, .004, .004, .002}, [3]int{194, 112, 48}, meshGrip)
		}
		// Telescope with flared lens rings, raised mounts, and the receiver bolt.
		for _, y := range []float64{-.43, .10} {
			m.box([3]float64{0, y, .13}, [3]float64{.025, .035, .065}, .01, gold, meshMetal)
		}
		rangedGunLathe(&m, [][2]float64{{-.63, .061}, {-.52, .061}, {-.48, .037}, {.25, .037}, {.29, .061}, {.39, .061}}, .222, gold, meshMetal)
		rangedGunLathe(&m, [][2]float64{{.391, .048}, {.394, .048}}, .222, [3]int{44, 85, 97}, meshStone)
		// The ocular is recessed into its brass lip, not a solid gold cap.
		rangedGunLathe(&m, [][2]float64{{-.637, .047}, {-.635, .047}}, .222, [3]int{24, 55, 63}, meshStone)
		rangedGunLathe(&m, [][2]float64{{-.640, .019}, {-.638, .019}}, .228, [3]int{94, 139, 145}, meshMetal)
		m.rod([3]float64{.04, -.48, .07}, [3]float64{.15, -.54, .10}, .017, .017, steel, meshMetal)
		rangedOrb(&m, [3]float64{.15, -.54, .10}, [3]float64{.027, .027, .027}, steel, meshMetal)
	} else {
		// The matchlock has a continuous wooden fore-stock, a short angular butt,
		// an exposed side lock and serpentine. It has no scope or modern receiver.
		rangedSidePlate(&m, [][2]float64{{-1.31, -.28}, {-1.31, -.02}, {-.88, .02}, {-.65, .04}, {1.15, .018}, {1.20, -.068}, {-.61, -.105}, {-.91, -.20}}, 0, .058, .12, wood, meshGrip)
		rangedBarrel(&m, -.73, 1.44, .041, .058, steel)
		for _, side := range []float64{-1, 1} {
			rangedSidePlate(&m, [][2]float64{{-.80, -.04}, {-.70, .083}, {-.31, .083}, {-.24, -.025}, {-.48, -.10}}, side*.061, .012, .12, [3]int{105, 110, 111}, meshMetal)
			for _, y := range []float64{-.69, -.38} {
				rangedGunScrew(&m, [3]float64{side * .078, y, .015}, .018, gold)
			}
		}
		rangedMetalCurve(&m, [][3]float64{{.08, -.69, .03}, {.11, -.61, .20}, {.12, -.45, .23}, {.09, -.33, .13}}, []float64{.020, .024, .018, .012}, gold, meshMetal)
		m.rod([3]float64{.09, -.36, .14}, [3]float64{.09, -.21, .12}, .006, .006, [3]int{137, 106, 63}, meshGrip)
		rangedSidePlate(&m, [][2]float64{{-1.32, -.28}, {-1.32, -.02}, {-1.28, -.018}, {-1.28, -.265}}, 0, .065, .05, gold, meshMetal)
		// The fore-stock carries a slender ramrod under the barrel. Its two
		// ferrules, lock pivot and pan remain distinct at the firing angle.
		m.rod([3]float64{0, -.10, -.081}, [3]float64{0, 1.22, -.081}, .010, .010, steel, meshMetal)
		for _, y := range []float64{.34, .97} {
			rangedGunLathe(&m, [][2]float64{{y, .016}, {y + .05, .016}}, -.081, gold, meshMetal)
		}
		rangedGunScrew(&m, [3]float64{.102, -.67, .048}, .027, gold)
		m.box([3]float64{.081, -.26, .107}, [3]float64{.036, .052, .016}, .009, gold, meshMetal)
		for _, side := range []float64{-1, 1} {
			for _, dz := range []float64{-.045, .035} {
				rangedMetalCurve(&m, [][3]float64{{side * .060, -1.26, -.16 + dz}, {side * .061, -1.08, -.12 + dz}, {side * .060, -.90, -.074 + dz}}, []float64{.002, .004, .001}, [3]int{164, 91, 43}, meshGrip)
			}
			rangedMetalCurve(&m, [][3]float64{{side * .075, -.74, .054}, {side * .076, -.51, .061}, {side * .075, -.30, .043}}, []float64{.002, .003, .002}, [3]int{190, 174, 131}, meshMetal)
		}
	}
	for _, y := range []float64{-.21, .54} {
		rangedGunLathe(&m, [][2]float64{{y, .057}, {y + .035, .057}}, .032, gold, meshMetal)
	}
	m.curve([3]float64{0, -.65, -.09}, [3]float64{0, -.78, -.36}, [3]float64{0, -.97, -.15}, .012, gold, meshMetal)
	m.rod([3]float64{0, -.70, -.08}, [3]float64{0, -.77, -.23}, .009, .006, steel, meshMetal)
	m.box([3]float64{0, 1.32, .104}, [3]float64{.012, .028, .025}, .004, gold, meshMetal)
	return m.faces
}

func buildInazumaHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	gold, dark, blue := [3]int{220, 151, 42}, [3]int{88, 53, 25}, [3]int{42, 89, 189}
	rangedSidePlate(&m, [][2]float64{{-1.18, -.40}, {-1.20, -.28}, {-1.14, -.14}, {-.98, .03}, {-.79, .11}, {-.44, .07}, {.98, .045}, {1.02, -.035}, {-.34, -.10}, {-.65, -.18}, {-.82, -.30}, {-.99, -.43}}, 0, .079, .25, [3]int{138, 76, 28}, meshGrip)
	rangedBarrel(&m, -.48, 1.14, .068, .065, gold)
	for _, y := range []float64{-.43, .07, .86, 1.07} {
		rangedGunLathe(&m, [][2]float64{{y, .080}, {y + .055, .080}}, .065, [3]int{245, 190, 76}, meshMetal)
	}
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-.65, -.12}, {-.69, .075}, {-.47, .16}, {-.11, .16}, {.12, .07}, {.03, -.09}, {-.30, -.16}}, side*.079, .025, .13, gold, meshMetal)
		// Dark inlaid scrolls follow the brass lock plate, with two pivot screws.
		rangedMetalCurve(&m, [][3]float64{{side * .107, -.55, -.07}, {side * .109, -.40, .06}, {side * .109, -.16, .08}, {side * .108, -.06, .015}, {side * .108, -.19, -.065}, {side * .108, -.35, -.045}}, []float64{.008, .009, .009, .008, .006, .002}, dark, meshMetal)
		for _, y := range []float64{-.45, -.15} {
			rangedGunScrew(&m, [3]float64{side * .113, y, .013}, .028, [3]int{250, 207, 92})
		}
		// Curved grip with crossing blue silk bindings and a complete gilt
		// heel. The binding follows the grip, not a flat row of painted bars.
		grip := [][3]float64{{-1.145, -.378, -.185}, {-1.09, -.387, -.12}, {-1.035, -.396, -.06}, {-.98, -.387, .001}, {-.925, -.346, .024}, {-.87, -.304, .047}, {-.815, -.271, .07}, {-.76, -.238, .078}}
		for j := 1; j < len(grip); j++ {
			a, b := grip[j-1], grip[j]
			// Broad flat strips cross the full tapered side of the grip. The
			// small offset separates the crossing layers without sinking them.
			rangedSidePlate(&m, [][2]float64{{a[0] - .007, a[1]}, {a[0] + .015, a[1]}, {b[0] + .007, b[2]}, {b[0] - .015, b[2]}}, side*.084, .004, .3, blue, meshGrip)
			rangedSidePlate(&m, [][2]float64{{a[0] - .007, a[2]}, {a[0] + .015, a[2]}, {b[0] + .007, b[1]}, {b[0] - .015, b[1]}}, side*.093, .004, .3, [3]int{72, 116, 215}, meshGrip)
		}
		for _, y := range []float64{.22, .46, .70} {
			rangedMetalCurve(&m, [][3]float64{{side * .068, y, .065}, {side * .071, y + .05, .095}, {side * .071, y + .12, .087}, {side * .069, y + .16, .050}}, []float64{.004, .006, .005, .001}, dark, meshMetal)
		}
		for _, z := range []float64{.011, .115} {
			m.rod([3]float64{side * .043, .14, z}, [3]float64{side * .043, .85, z}, .004, .004, [3]int{250, 199, 78}, meshMetal)
		}
		rangedMetalCurve(&m, [][3]float64{{side * .108, -.62, -.07}, {side * .109, -.58, .065}, {side * .109, -.44, .125}, {side * .109, -.16, .125}, {side * .108, .035, .050}}, []float64{.004, .007, .007, .006, .003}, [3]int{250, 205, 91}, meshMetal)
	}
	rangedSidePlate(&m, [][2]float64{{-1.19, -.40}, {-1.20, -.28}, {-1.16, -.23}, {-1.13, -.30}, {-.98, -.40}, {-.99, -.435}}, 0, .086, .28, gold, meshMetal)
	m.curve([3]float64{0, -.42, -.11}, [3]float64{0, -.57, -.36}, [3]float64{0, -.75, -.18}, .015, gold, meshMetal)
	rangedMetalCurve(&m, [][3]float64{{.09, -.58, .10}, {.14, -.46, .23}, {.12, -.25, .23}, {.08, -.14, .15}}, []float64{.023, .025, .018, .008}, gold, meshMetal)
	return m.faces
}

func buildAlienHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	shell, edge, cyan := [3]int{61, 66, 88}, [3]int{114, 126, 151}, [3]int{33, 218, 249}
	rangedGunLathe(&m, [][2]float64{{-.67, .141}, {-.30, .141}}, .03, shell, meshMetal)
	rangedBarrel(&m, .39, 1.00, .141, .03, shell)
	// Exposed cylindrical power core inside four open rails.
	rangedGunLathe(&m, [][2]float64{{-.29, .126}, {.38, .126}}, .03, cyan, meshGlow)
	for j := 0; j < 5; j++ {
		y := -.25 + float64(j)*.14
		rangedGunLathe(&m, [][2]float64{{y, .133}, {y + .015, .133}}, .03, [3]int{24, 113, 159}, meshMetal)
	}
	for _, side := range []float64{-1, 1} {
		for _, z := range []float64{-.11, .18} {
			rangedSidePlate(&m, [][2]float64{{-.75, z - .035}, {-.58, z + .045}, {.49, z + .045}, {.80, z + .02}, {.83, z - .035}}, side*.16, .031, .18, shell, meshMetal)
			m.rod([3]float64{side * .178, -.55, z + .035}, [3]float64{side * .178, .46, z + .035}, .008, .008, edge, meshMetal)
		}
		// Separate rear and muzzle armor pods frame the luminous window.
		for _, v := range [][2]float64{{-.60, .20}, {.71, .17}} {
			y, l := v[0], v[1]
			rangedSidePlate(&m, [][2]float64{{y - l, -.10}, {y - l, .17}, {y + l*.6, .17}, {y + l, .07}, {y + l, -.085}}, side*.156, .054, .18, shell, meshMetal)
			m.box([3]float64{side * .214, y, .047}, [3]float64{.010, l * .54, .030}, .008, cyan, meshGlow)
			rangedGunScrew(&m, [3]float64{side * .215, y - l*.7, .12}, .012, edge)
		}
		for _, z := range []float64{.18, .31} {
			m.box([3]float64{side * .105, -.64, z}, [3]float64{.065, .17, .039}, .022, shell, meshMetal)
			m.box([3]float64{side * .108, -.49, z}, [3]float64{.032, .018, .018}, .009, cyan, meshGlow)
		}
	}
	rangedGunLathe(&m, [][2]float64{{.95, .156}, {1.04, .151}, {1.06, .11}}, .03, edge, meshMetal)
	rangedGunLathe(&m, [][2]float64{{1.061, .10}, {1.062, .10}}, .03, cyan, meshGlow)
	rangedSidePlate(&m, [][2]float64{{-.69, -.09}, {-.44, -.11}, {-.37, -.56}, {-.55, -.64}, {-.75, -.54}, {-.60, -.27}}, 0, .083, .16, shell, meshMetal)
	for _, side := range []float64{-1, 1} {
		for j := 0; j < 5; j++ {
			z := -.26 - float64(j)*.056
			m.rod([3]float64{side * .087, -.63, z}, [3]float64{side * .087, -.43, z - .035}, .007, .007, edge, meshMetal)
		}
	}
	m.curve([3]float64{0, -.38, -.09}, [3]float64{0, -.07, -.40}, [3]float64{0, -.45, -.39}, .014, shell, meshMetal)
	return m.faces
}

func rangedGear(m *weaponMeshBuilder, c [3]float64, r float64, col [3]int) {
	// Rectangular teeth, a recessed hub and spokes in a single extruded wheel.
	outline := make([][2]float64, 0, 48)
	for i := 0; i < 12; i++ {
		for j := 0; j < 4; j++ {
			a := (float64(i) + float64(j)/4) * math.Pi / 6
			rad := r * .81
			if j == 1 || j == 2 {
				rad = r
			}
			outline = append(outline, [2]float64{c[1] + rad*math.Cos(a), c[2] + rad*math.Sin(a)})
		}
	}
	rangedSidePlate(m, outline, c[0], .026, .04, col, meshMetal)
	for _, side := range []float64{-1, 1} {
		m.rod([3]float64{c[0] + side*.028, c[1], c[2]}, [3]float64{c[0] + side*.031, c[1], c[2]}, r*.57, r*.57, [3]int{108, 68, 23}, meshMetal)
		for i := 0; i < 6; i++ {
			a := float64(i) * math.Pi / 3
			m.rod([3]float64{c[0] + side*.034, c[1] + r*.15*math.Cos(a), c[2] + r*.15*math.Sin(a)}, [3]float64{c[0] + side*.034, c[1] + r*.57*math.Cos(a), c[2] + r*.57*math.Sin(a)}, .009, .009, col, meshMetal)
		}
		// The hub is a stepped steel bearing with a bright retaining ring.
		// Small root studs keep the gear face from reading as a flat cog icon.
		m.rod([3]float64{c[0] + side*.033, c[1], c[2]}, [3]float64{c[0] + side*.043, c[1], c[2]}, r*.34, r*.34, [3]int{151, 164, 173}, meshMetal)
		m.rod([3]float64{c[0] + side*.043, c[1], c[2]}, [3]float64{c[0] + side*.047, c[1], c[2]}, r*.26, r*.26, [3]int{66, 72, 74}, meshMetal)
		for j := 0; j < 6; j++ {
			a := float64(j)*math.Pi/3 + .2
			rangedGunScrew(m, [3]float64{c[0] + side*.031, c[1] + r*.65*math.Cos(a), c[2] + r*.65*math.Sin(a)}, .009, col)
		}
	}
	for _, side := range []float64{-1, 1} {
		rangedGunScrew(m, [3]float64{c[0] + side*.052, c[1], c[2]}, r*.17, meshSteel)
	}
}
func buildClockPistolHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	brass, copper, steel := [3]int{220, 155, 42}, [3]int{169, 85, 30}, [3]int{156, 167, 176}
	// Stepped brass barrel with an exposed steel muzzle and dark bottom tube.
	rangedBarrel(&m, -.28, 1.06, .102, .12, steel)
	rangedGunLathe(&m, [][2]float64{{-.08, .128}, {.54, .128}, {.61, .108}}, .12, brass, meshMetal)
	for _, y := range []float64{-.10, .48, .99} {
		rangedGunLathe(&m, [][2]float64{{y - .008, .128}, {y, .141}, {y + .009, .145}, {y + .036, .145}, {y + .045, .141}, {y + .053, .128}}, .12, brass, meshMetal)
	}
	rangedBarrel(&m, -.28, .67, .054, -.035, copper)
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-.51, -.045}, {-.51, .27}, {-.15, .27}, {-.10, -.045}}, side*.092, .025, .12, brass, meshMetal)
		rangedSidePlate(&m, [][2]float64{{-.45, .012}, {-.45, .21}, {-.21, .21}, {-.19, .012}}, side*.121, .012, .08, steel, meshMetal)
		for _, y := range []float64{-.43, -.22} {
			rangedGunScrew(&m, [3]float64{side * .14, y, .22}, .013, brass)
		}
		// Beveled receiver inset and a bright upper rail frame the silver lock.
		for _, z := range []float64{.015, .211} {
			m.rod([3]float64{side * .136, -.44, z}, [3]float64{side * .136, -.21, z}, .005, .005, [3]int{233, 189, 85}, meshMetal)
		}
		m.rod([3]float64{side * .102, -.06, .205}, [3]float64{side * .102, .47, .205}, .006, .006, [3]int{241, 191, 72}, meshMetal)
		rangedGear(&m, [3]float64{side * .13, -.67, .04}, .22, brass)
	}
	// Curved walnut pistol grip with a brass heel, not a modern straight handle.
	outline := [][2]float64{{-.77, .03}, {-.53, -.08}, {-.55, -.23}, {-.72, -.49}, {-.83, -.58}, {-1.01, -.58}, {-1.04, -.43}, {-.97, -.17}}
	rangedSidePlate(&m, outline, 0, .087, .19, brass, meshMetal)
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-.79, -.04}, {-.63, -.11}, {-.66, -.28}, {-.80, -.51}, {-.96, -.51}, {-.94, -.29}}, side*.09, .012, .14, [3]int{150, 68, 28}, meshGrip)
		rangedGunScrew(&m, [3]float64{side * .11, -.86, -.44}, .023, brass)
		for _, dy := range []float64{-.03, .025} {
			rangedMetalCurve(&m, [][3]float64{{side * .104, -.78 + dy, -.13}, {side * .105, -.79 + dy, -.25}, {side * .104, -.88 + dy, -.43}}, []float64{.002, .004, .001}, [3]int{185, 96, 45}, meshGrip)
		}
	}
	rangedMetalCurve(&m, [][3]float64{{0, -.45, -.03}, {0, -.23, -.15}, {0, -.29, -.32}, {0, -.48, -.35}, {0, -.66, -.21}}, []float64{.018, .019, .019, .018, .015}, brass, meshMetal)
	m.rod([3]float64{0, -.40, -.04}, [3]float64{0, -.47, -.22}, .012, .008, steel, meshMetal)
	rangedMetalCurve(&m, [][3]float64{{0, -.64, .19}, {0, -.71, .31}, {0, -.88, .40}, {0, -.94, .48}}, []float64{.025, .026, .021, .015}, steel, meshMetal)
	m.rod([3]float64{0, -.26, .25}, [3]float64{0, -.28, .36}, .017, .017, brass, meshMetal)
	return m.faces
}
func buildSuppressorHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	steel, edge, brass := [3]int{50, 53, 52}, [3]int{106, 108, 98}, [3]int{175, 143, 76}
	rangedSidePlate(&m, [][2]float64{{-.68, -.12}, {-.72, .13}, {-.53, .21}, {.52, .21}, {.65, .09}, {.60, -.13}}, 0, .137, .10, steel, meshMetal)
	// Layered receiver, top rail, rear sling loop, and a short perforated shroud.
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-.47, -.07}, {-.47, .14}, {.47, .14}, {.48, -.07}}, side*.14, .012, .07, edge, meshMetal)
		m.box([3]float64{side * .158, -.20, .04}, [3]float64{.006, .18, .026}, .004, [3]int{27, 28, 27}, meshGrip)
		for j := 0; j < 5; j++ {
			m.box([3]float64{side * .166, -.34 + float64(j)*.064, .045}, [3]float64{.005, .018, .012}, .002, [3]int{142, 31, 21}, meshGlow)
		}
		rangedGunScrew(&m, [3]float64{side * .158, .32, .035}, .037, brass)
		for _, y := range []float64{-.55, .51} {
			rangedGunScrew(&m, [3]float64{side * .162, y, .14}, .013, edge)
		}
		// Vent fins below the forward receiver and a raised rear selector.
		for j := 0; j < 5; j++ {
			y := .20 + float64(j)*.073
			rangedSidePlate(&m, [][2]float64{{y - .012, -.12}, {y + .025, -.12}, {y + .015, -.265}, {y - .008, -.245}}, side*.112, .010, .22, edge, meshMetal)
		}
		rangedMetalCurve(&m, [][3]float64{{side * .164, -.56, .055}, {side * .184, -.62, .10}, {side * .186, -.67, .15}}, []float64{.019, .017, .011}, edge, meshMetal)
	}
	for j := 0; j < 9; j++ {
		m.box([3]float64{0, -.47 + float64(j)*.102, .229}, [3]float64{.075, .024, .020}, .006, edge, meshMetal)
	}
	rangedBarrel(&m, .57, 1.06, .105, .055, brass)
	for j := 0; j < 3; j++ {
		for _, side := range []float64{-1, 1} {
			m.box([3]float64{side * .104, .69 + float64(j)*.12, .056}, [3]float64{.006, .031, .029}, .008, [3]int{15, 18, 18}, meshGrip)
		}
	}
	// Concentric steel drum; the visible ammunition forms one side belt.
	m.rod([3]float64{-.21, -.14, -.29}, [3]float64{.21, -.14, -.29}, .31, .31, steel, meshMetal)
	for _, side := range []float64{-1, 1} {
		for _, r := range []float64{.29, .25, .19, .08} {
			part := weaponMeshBuilder{}
			part.ring([3]float64{-.14, -.29, side * .225}, r, .012, edge)
			m.appendMapped(part.faces, func(p [3]float64) [3]float64 { return [3]float64{p[2], p[0], p[1]} })
		}
	}
	for j := 0; j < 13; j++ {
		a := -1.3 + float64(j)*.205
		y, z := -.14+.32*math.Cos(a), -.29+.32*math.Sin(a)
		m.rod([3]float64{-.25, y, z}, [3]float64{.25, y, z}, .032, .032, brass, meshMetal)
		for _, side := range []float64{-1, 1} {
			m.rod([3]float64{side * .25, y, z}, [3]float64{side * .28, y, z}, .032, .018, [3]int{219, 187, 111}, meshMetal)
		}
	}
	// Skeleton shoulder stock and the short grip behind the drum.
	for _, z := range []float64{.13, -.12} {
		m.rod([3]float64{0, -.64, z}, [3]float64{0, -1.13, z}, .032, .032, steel, meshMetal)
	}
	m.box([3]float64{0, -1.17, -.01}, [3]float64{.073, .047, .22}, .025, [3]int{29, 31, 30}, meshGrip)
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-1.15, .15}, {-.64, .15}, {-.62, .10}, {-1.11, .055}, {-1.15, -.04}}, side*.036, .017, .24, edge, meshMetal)
		rangedGunScrew(&m, [3]float64{side * .243, -.14, -.29}, .056, edge)
		m.rod([3]float64{side * .20, -.25, -.04}, [3]float64{side * .27, -.12, -.02}, .016, .016, steel, meshMetal)
		// The raised sight ears bracket the aiming rail, not the muzzle.
		rangedSidePlate(&m, [][2]float64{{.34, .23}, {.48, .23}, {.53, .33}, {.51, .37}, {.46, .29}, {.34, .27}}, side*.054, .013, .22, edge, meshMetal)
	}
	rangedSidePlate(&m, [][2]float64{{-.67, -.09}, {-.48, -.15}, {-.61, -.43}, {-.78, -.40}}, 0, .067, .16, steel, meshGrip)
	m.curve([3]float64{0, -.47, -.10}, [3]float64{0, -.35, -.34}, [3]float64{0, -.62, -.32}, .014, edge, meshMetal)
	rangedGunLathe(&m, [][2]float64{{-.44, .044}, {-.19, .044}}, .32, steel, meshMetal)
	rangedGunLathe(&m, [][2]float64{{-.185, .029}, {-.180, .029}}, .32, [3]int{238, 40, 25}, meshGlow)
	return m.faces
}
func buildLonglanceHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	steel, edge, cyan, gold := [3]int{44, 56, 65}, [3]int{124, 142, 152}, [3]int{61, 217, 225}, [3]int{187, 145, 61}
	rangedSidePlate(&m, [][2]float64{{-.71, -.10}, {-.75, .12}, {-.49, .18}, {.98, .18}, {1.27, .11}, {1.34, -.04}, {.95, -.13}, {-.47, -.13}}, 0, .102, .1, steel, meshMetal)
	rangedBarrel(&m, .86, 1.54, .069, .05, edge)
	// Long exposed accelerator window on both sides, with individual bright
	// coils and black ribs. Raised rails run uninterrupted above and below it.
	for _, side := range []float64{-1, 1} {
		m.box([3]float64{side * .11, .48, .034}, [3]float64{.009, .59, .067}, .014, [3]int{12, 29, 36}, meshGrip)
		for j := 0; j < 10; j++ {
			y := -.025 + float64(j)*.112
			rangedSidePlate(&m, [][2]float64{{y - .027, -.01}, {y - .034, .095}, {y + .025, .095}, {y + .04, -.008}}, side*.122, .010, .16, cyan, meshGlow)
			m.rod([3]float64{side * .139, y + .037, -.029}, [3]float64{side * .139, y + .012, .113}, .009, .009, edge, meshMetal)
		}
		for _, z := range []float64{-.075, .142} {
			m.rod([3]float64{side * .116, -.20, z}, [3]float64{side * .116, 1.21, z}, .014, .014, edge, meshMetal)
		}
		for _, y := range []float64{-.20, 1.17} {
			m.rod([3]float64{side * .12, y, -.09}, [3]float64{side * .12, y, .15}, .018, .018, edge, meshMetal)
		}
		rangedSidePlate(&m, [][2]float64{{-.68, -.08}, {-.68, .10}, {-.29, .10}, {-.28, -.08}}, side*.106, .012, .10, [3]int{65, 76, 80}, meshMetal)
		rangedGunScrew(&m, [3]float64{side * .125, -.51, .035}, .021, gold)
	}
	// The layered shoulder stock has a triangular cutout between flat webs;
	// the thin rails support it rather than substituting for the stock shell.
	for _, side := range []float64{-1, 1} {
		rangedSidePlate(&m, [][2]float64{{-1.27, .13}, {-.71, .13}, {-.71, .055}, {-1.22, .035}}, side*.052, .018, .20, steel, meshMetal)
		rangedSidePlate(&m, [][2]float64{{-1.27, -.22}, {-1.24, -.06}, {-.81, .055}, {-.77, -.02}, {-1.11, -.22}}, side*.052, .018, .20, steel, meshMetal)
		m.rod([3]float64{side * .073, -1.14, -.15}, [3]float64{side * .073, -.91, -.025}, .007, .007, cyan, meshGlow)
	}
	// Low angled pistol grip and box magazine.
	for _, z := range []float64{.10, -.12} {
		m.rod([3]float64{0, -.67, z}, [3]float64{0, -1.25, z - .02}, .030, .030, edge, meshMetal)
	}
	m.box([3]float64{0, -1.29, -.05}, [3]float64{.077, .044, .20}, .016, steel, meshMetal)
	rangedSidePlate(&m, [][2]float64{{-.72, -.09}, {-.50, -.10}, {-.62, -.41}, {-.81, -.39}}, 0, .065, .13, steel, meshGrip)
	for _, side := range []float64{-1, 1} {
		m.rod([3]float64{side * .070, -.70, -.16}, [3]float64{side * .070, -.75, -.34}, .007, .007, cyan, meshGlow)
	}
	rangedSidePlate(&m, [][2]float64{{-.28, -.11}, {-.05, -.11}, {-.04, -.39}, {-.29, -.40}}, 0, .065, .1, steel, meshMetal)
	for j := 0; j < 4; j++ {
		for _, side := range []float64{-1, 1} {
			z := -.18 - float64(j)*.054
			m.rod([3]float64{side * .071, -.26, z}, [3]float64{side * .071, -.07, z}, .006, .006, edge, meshMetal)
		}
	}
	m.curve([3]float64{0, -.48, -.10}, [3]float64{0, -.35, -.31}, [3]float64{0, -.64, -.31}, .012, gold, meshMetal)
	// Forward-facing cyan scope lens, narrow eyepiece, adjustment turret.
	for _, y := range []float64{-.48, -.01} {
		m.box([3]float64{0, y, .23}, [3]float64{.027, .032, .058}, .01, steel, meshMetal)
	}
	rangedGunLathe(&m, [][2]float64{{-.66, .052}, {-.55, .052}, {-.50, .038}, {.10, .038}, {.18, .069}, {.28, .069}}, .31, edge, meshMetal)
	rangedGunLathe(&m, [][2]float64{{.281, .053}, {.285, .053}}, .31, cyan, meshGlow)
	m.rod([3]float64{0, -.18, .31}, [3]float64{0, -.18, .40}, .029, .029, steel, meshMetal)
	return m.faces
}
