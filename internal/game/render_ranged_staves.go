package game

import "math"

// The lower bulb, narrow central ferrule and stepped upper socket are distinct
// pieces. Their axial profiles follow the three metal regions of the icon.
func buildBattleHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	wood, iron, edge := [3]int{145, 83, 38}, [3]int{129, 135, 139}, [3]int{213, 218, 218}
	rangedLathe(&m, [][2]float64{{-1.70, .081}, {-.5, .085}, {.75, .086}}, wood, meshGrip)
	rangedLathe(&m, [][2]float64{{-2.43, .025}, {-2.41, .084}, {-2.36, .124}, {-2.29, .139}, {-2.23, .130}, {-2.17, .090}, {-2.07, .087}, {-1.92, .094}, {-1.77, .095}, {-1.72, .112}, {-1.66, .112}, {-1.63, .083}}, iron, meshMetal)
	rangedLathe(&m, [][2]float64{{.70, .087}, {.74, .109}, {.79, .109}, {.82, .096}, {1.01, .105}, {1.07, .141}, {1.12, .143}, {1.16, .120}, {1.37, .140}, {1.41, .174}, {1.49, .181}, {1.55, .163}, {1.60, .113}, {1.62, .034}}, iron, meshMetal)
	for _, v := range [][2]float64{{-2.29, .142}, {-1.72, .115}, {.74, .113}, {1.07, .147}, {1.43, .183}} {
		rangedLathe(&m, [][2]float64{{v[0], v[1]}, {v[0] + .019, v[1] + .006}, {v[0] + .034, v[1]}}, edge, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{-.35, .084}, {-.32, .108}, {-.09, .108}, {-.06, .085}}, iron, meshMetal)
	for _, x := range []float64{-.32, -.12} {
		rangedLathe(&m, [][2]float64{{x, .109}, {x + .025, .113}}, edge, meshMetal)
	}
	// Short diagonal tooling belongs to the central sleeve; the exposed wood
	// keeps a few long grain lines rather than a uniform metal tube.
	for j := 0; j < 8; j++ {
		a := float64(j) * math.Pi / 4
		m.rod([3]float64{-.28, .111 * math.Cos(a), .111 * math.Sin(a)}, [3]float64{-.15, .111 * math.Cos(a+.25), .111 * math.Sin(a+.25)}, .006, .006, edge, meshMetal)
	}
	for j := 0; j < 7; j++ {
		a := float64(j) * 2 * math.Pi / 7
		var p [][3]float64
		for k := 0; k < 7; k++ {
			x := -1.64 + float64(k)*.397
			r := .086
			p = append(p, [3]float64{x, r * math.Cos(a+.12*math.Sin(x*4)), r * math.Sin(a+.12*math.Sin(x*4))})
		}
		rangedMetalCurve(&m, p, []float64{.003, .005, .003, .004, .004, .003, .001}, [3]int{196, 132, 65}, meshGrip)
	}
	return m.faces
}

func buildFlowHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	bronze, edge := [3]int{148, 100, 50}, [3]int{215, 171, 93}
	rangedLathe(&m, [][2]float64{{-2.23, .048}, {-.6, .055}, {.52, .067}}, [3]int{66, 47, 46}, meshGrip)
	for _, span := range [][2]float64{{-2.02, -1.64}, {-1.34, -.81}, {-.49, -.12}} {
		rangedLathe(&m, [][2]float64{{span[0], .061}, {span[1], .065}}, [3]int{26, 64, 103}, meshGrip)
		rangedHelix(&m, span[0], span[1], .066, .006, 4, [3]int{63, 100, 133}, meshGrip)
	}
	for _, x := range []float64{-2.17, -1.61, -1.05, -.49, .11, .40} {
		rangedLathe(&m, [][2]float64{{x, .049}, {x + .025, .071}, {x + .075, .071}, {x + .1, .048}}, bronze, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{-2.48, 0}, {-2.30, .09}, {-2.20, .06}}, edge, meshMetal)
	rangedFluidOrb(&m, [3]float64{-2.32, 0, -.078}, [3]float64{.042, .032, .018}, [3]int{35, 161, 206}, meshStone)
	rangedFluidOrb(&m, [3]float64{1.04, 0, 0}, [3]float64{.34, .30, .29}, [3]int{22, 135, 199}, meshHeldWater)
	// Four thin cage ribs leave the moving water visible through open windows.
	for i := 0; i < 4; i++ {
		a := float64(i)*math.Pi/2 + .40
		cy, cz := math.Cos(a), math.Sin(a)
		var p [][3]float64
		for _, v := range [][2]float64{{.48, .02}, {.66, .23}, {.99, .35}, {1.27, .30}, {1.45, .13}, {1.53, 0}} {
			p = append(p, [3]float64{v[0], cy * v[1], cz * v[1]})
		}
		rangedMetalCurve(&m, p, []float64{.037, .031, .025, .025, .022, .016}, bronze, meshMetal)
		for j := range p {
			p[j][1] += cy * .014
			p[j][2] += cz * .014
		}
		rangedMetalCurve(&m, p, []float64{.009, .008, .007, .007, .003, .001}, edge, meshMetal)
		for _, v := range [][2]float64{{.69, .25}, {1.30, .29}} {
			rangedOrb(&m, [3]float64{v[0], cy * v[1], cz * v[1]}, [3]float64{.044, .047, .047}, edge, meshMetal)
		}
	}
	for _, x := range []float64{.50, .59} {
		rangedLathe(&m, [][2]float64{{x, .083}, {x + .036, .083}}, edge, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{1.40, .047}, {1.49, .08}, {1.58, .045}, {1.72, 0}}, bronze, meshMetal)
	return m.faces
}

func buildLanistaHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	gold, light, wood := [3]int{206, 145, 42}, [3]int{255, 210, 92}, [3]int{135, 69, 29}
	rangedLathe(&m, [][2]float64{{-2.16, .042}, {-.90, .048}, {.51, .066}}, wood, meshGrip)
	// Paired rings articulate the heel, middle grip and crown socket.
	rangedLathe(&m, [][2]float64{{-2.30, .036}, {-2.28, .071}, {-2.24, .091}, {-2.20, .09}, {-2.18, .062}, {-2.14, .061}, {-2.12, .087}, {-2.08, .087}, {-2.06, .05}}, gold, meshMetal)
	for _, x := range []float64{-2.24, -2.12, -.79, -.71, .35, .46} {
		rangedLathe(&m, [][2]float64{{x, .059}, {x + .012, .079}, {x + .036, .079}, {x + .048, .058}}, light, meshMetal)
	}
	for _, a := range []float64{-.7, 1.4, 3.5} {
		m.rod([3]float64{-2.10, .046 * math.Cos(a), .046 * math.Sin(a)}, [3]float64{.32, .064 * math.Cos(a), .064 * math.Sin(a)}, .004, .005, [3]int{183, 112, 47}, meshGrip)
	}
	// The crescent's flattened silver blades curl around, not across, the orb.
	for _, side := range []float64{-1, 1} {
		var s []rangedSection
		for _, v := range [][3]float64{{.49, .03, .085}, {.68, .26, .09}, {.98, .39, .074}, {1.25, .34, .049}, {1.47, .18, .014}, {1.53, .10, .002}} {
			s = append(s, rangedSection{[3]float64{v[0], side * v[1], .01}, v[2], v[2] * .43})
		}
		rangedTube(&m, rangedSmooth(s), meshSilver, meshMetal)
		for _, face := range []float64{-1, 1} {
			seam := append([]rangedSection(nil), s...)
			for i := range seam {
				seam[i].p[2] += face * (seam[i].depth + .005)
				seam[i].width *= .16
				seam[i].depth = .004
			}
			rangedTube(&m, rangedSmooth(seam), [3]int{245, 245, 220}, meshMetal)
		}
	}
	// The faceted gold orb sits on a visible silver pin inside the crescent.
	rangedLathe(&m, [][2]float64{{.52, .047}, {.82, .047}, {.90, .025}}, meshSilver, meshMetal)
	rangedOrb(&m, [3]float64{1.10, 0, .025}, [3]float64{.235, .23, .225}, gold, meshMetal)
	rangedLathe(&m, [][2]float64{{.40, .064}, {.43, .098}, {.46, .135}, {.50, .145}, {.54, .111}, {.61, .065}}, gold, meshMetal)
	for _, x := range []float64{.46, .51} {
		rangedLathe(&m, [][2]float64{{x, .141}, {x + .016, .146}}, light, meshMetal)
	}
	// A knot with two folded, tapered silk tails hangs from the crown socket.
	rangedHelix(&m, .27, .40, .079, .018, 2, [3]int{160, 22, 17}, meshGrip)
	rangedOrb(&m, [3]float64{.35, -.08, -.08}, [3]float64{.066, .045, .048}, [3]int{202, 33, 21}, meshGrip)
	rangedCloth(&m, [][3]float64{{.35, -.09, -.09}, {.16, -.25, -.12}, {-.10, -.35, -.10}, {-.35, -.48, -.16}, {-.45, -.51, -.13}}, []float64{.03, .057, .068, .031, 0}, [3]int{212, 29, 20})
	rangedCloth(&m, [][3]float64{{.35, -.09, -.09}, {.25, -.26, -.04}, {.22, -.49, -.10}, {.31, -.60, -.11}, {.25, -.65, -.06}}, []float64{.03, .048, .056, .035, 0}, [3]int{173, 18, 12})
	return m.faces
}

func buildEyeHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	bronze, dark, gold := [3]int{142, 123, 82}, [3]int{58, 56, 46}, [3]int{229, 168, 52}
	// Burgundy leather and a short pointed pommel; the ornate head occupies half
	// the scepter, unlike the much longer plain staff shafts.
	rangedLathe(&m, [][2]float64{{-1.78, .048}, {-.40, .069}, {.04, .075}}, [3]int{100, 29, 28}, meshGrip)
	rangedHelix(&m, -1.70, -.43, .071, .011, 10, [3]int{179, 64, 49}, meshGrip)
	for _, x := range []float64{-1.80, -.38, -.28} {
		rangedLathe(&m, [][2]float64{{x, .068}, {x + .01, .086}, {x + .018, .102}, {x + .027, .105}, {x + .032, .091}, {x + .065, .091}, {x + .071, .105}, {x + .080, .102}, {x + .09, .069}}, gold, meshMetal)
		rangedLathe(&m, [][2]float64{{x + .038, .093}, {x + .058, .093}}, bronze, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{-2.04, 0}, {-1.98, .028}, {-1.88, .065}, {-1.85, .094}, {-1.82, .096}, {-1.81, .078}, {-1.79, .080}, {-1.76, .06}}, gold, meshMetal)
	// A forged pierced plate carries the lens. Faceted bands make the dark
	// setting substantial; the lighter inlay follows its actual raised surface.
	cx := .88
	const sections = 32
	for i := 0; i < sections; i++ {
		a, b := float64(i)*2*math.Pi/sections, float64(i+1)*2*math.Pi/sections
		outer := func(t float64) [2]float64 {
			r := 1 + .035*math.Cos(t*8)
			return [2]float64{cx + .54*r*math.Cos(t), .445 * r * math.Sin(t)}
		}
		inner := func(t float64) [2]float64 { return [2]float64{cx + .343*math.Cos(t), .307 * math.Sin(t)} }
		p, q, u, v := outer(a), outer(b), inner(a), inner(b)
		for _, side := range []float64{-1, 1} {
			// Adjacent sectors share their bevel vertices. Separately beveled
			// wedges would leave radial seams resembling a clock dial.
			profile := [][2]float64{{0, 0}, {.14, .049}, {.84, .055}, {1, 0}}
			for j := 1; j < len(profile); j++ {
				point := func(o, in [2]float64, k int) [3]float64 {
					t := profile[k][0]
					return [3]float64{o[0]*(1-t) + in[0]*t, o[1]*(1-t) + in[1]*t, side * profile[k][1]}
				}
				p0, p1, p2, p3 := point(p, u, j-1), point(q, v, j-1), point(p, u, j), point(q, v, j)
				o := [3]float64{(p[0] + q[0] + u[0] + v[0]) * .25, (p[1] + q[1] + u[1] + v[1]) * .25, 0}
				m.triangle(p0, p1, p2, o, dark, meshMetal)
				m.triangle(p2, p1, p3, o, dark, meshMetal)
			}
			for _, inset := range []float64{.22, .74} {
				p0 := [3]float64{p[0]*(1-inset) + u[0]*inset, p[1]*(1-inset) + u[1]*inset, side * .062}
				p1 := [3]float64{q[0]*(1-inset) + v[0]*inset, q[1]*(1-inset) + v[1]*inset, side * .062}
				m.rod(p0, p1, .009, .009, bronze, meshMetal)
			}
		}
	}
	// Interlaced fine metal ribs fill the setting, as on the icon. Their raised
	// crossings stay above the band and never span the green lens.
	for _, face := range []float64{-1, 1} {
		for strand := 0; strand < 2; strand++ {
			var rib []rangedSection
			for i := 0; i <= 64; i++ {
				a := float64(i) * 2 * math.Pi / 64
				wave := 8*a + float64(strand)*math.Pi
				r := .48 + .032*math.Sin(wave)
				rib = append(rib, rangedSection{[3]float64{cx + r*math.Cos(a), r * .81 * math.Sin(a), face * (.071 + .011*math.Cos(wave))}, .010, .007})
			}
			rangedTube(&m, rib, bronze, meshMetal)
		}
	}
	// Tapered socket with branching metal ribs under the circular head.
	for _, side := range []float64{-1, 1} {
		rangedMetalCurve(&m, [][3]float64{{-.30, 0, 0}, {-.08, side * .08, 0}, {.13, side * .16, 0}, {.37, side * .27, 0}, {.55, side * .37, 0}}, []float64{.065, .071, .065, .055, .043}, dark, meshMetal)
		for _, face := range []float64{-1, 1} {
			rangedMetalCurve(&m, [][3]float64{{-.23, side * .025, face * .052}, {-.02, side * .09, face * .061}, {.22, side * .19, face * .060}, {.42, side * .30, face * .059}}, []float64{.012, .016, .014, .008}, bronze, meshMetal)
			rangedMetalCurve(&m, [][3]float64{{-.12, 0, face * .057}, {.09, side * .07, face * .063}, {.21, 0, face * .072}, {.38, side * .11, face * .070}}, []float64{.012, .016, .012, .005}, bronze, meshMetal)
		}
		// Three broad upper teeth and a curled heel claw, with dark sockets.
		for _, v := range [][3]float64{{.67, .42, .16}, {1.01, .43, .19}, {1.32, .29, .19}} {
			// Flattened leaf claws have a raised spine, honed edges and a
			// narrow recessed groove, instead of a conical rod at each setting.
			x, y := v[0], side*v[1]
			outline := [][2]float64{{x - .065, y - side*.035}, {x + .02, y - side*.060}, {x + .10, y + side*.014}, {x + .16, y + side*v[2]}, {x + .055, y + side*.080}, {x - .02, y + side*.065}}
			rangedStockPlate(&m, outline, 0, .035, .36, gold, meshMetal)
			for _, face := range []float64{-1, 1} {
				rangedMetalCurve(&m, [][3]float64{{x - .015, y, face * .037}, {x + .056, y + side*.04, face * .043}, {x + .13, y + side*(v[2]-.04), face * .017}}, []float64{.005, .007, .001}, bronze, meshMetal)
			}
		}
		// The heel is a chased crescent plate, with a broad root and a flat
		// tapering hook. A round tube here reads as a bent plastic horn.
		heel := [][2]float64{{.43, .28}, {.39, .39}, {.33, .47}, {.24, .51}, {.15, .50}, {.08, .45}, {.055, .40}, {.14, .445}, {.22, .445}, {.28, .40}, {.31, .30}}
		for i := range heel {
			heel[i][1] *= side
		}
		rangedStockPlate(&m, heel, 0, .031, .32, gold, meshMetal)
		for _, face := range []float64{-1, 1} {
			rangedMetalCurve(&m, [][3]float64{{.38, side * .32, face * .033}, {.33, side * .42, face * .036}, {.23, side * .47, face * .035}, {.14, side * .46, face * .028}, {.07, side * .415, 0}}, []float64{.006, .008, .007, .004, .001}, bronze, meshMetal)
		}
	}
	// The orb and its four claws remain open in front. No branches cross it.
	rangedFluidOrb(&m, [3]float64{cx, 0, 0}, [3]float64{.33, .294, .185}, [3]int{32, 175, 41}, meshHeldEmerald)
	for i := 0; i < 4; i++ {
		a := math.Pi/4 + float64(i)*math.Pi/2
		for _, face := range []float64{-1, 1} {
			rangedMetalCurve(&m, [][3]float64{{cx + .37*math.Cos(a), .33 * math.Sin(a), face * .04}, {cx + .31*math.Cos(a), .275 * math.Sin(a), face * .11}, {cx + .28*math.Cos(a), .24 * math.Sin(a), face * .13}}, []float64{.027, .022, .007}, bronze, meshMetal)
		}
	}
	return m.faces
}

// The escapement wheel stands in the shaft's plane, as on the icon: a toothed
// rim, a fine inner rim on five spokes (the shaft is the sixth) and a stepped
// hub with a steel arbor pin. The shaft runs on to the hub.
func buildEscapementHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	brass, light, steel := [3]int{204, 138, 38}, [3]int{246, 194, 82}, [3]int{168, 172, 176}
	dark := [3]int{86, 52, 18}
	collar := func(x0, x1, r float64, col [3]int) {
		rangedLathe(&m, [][2]float64{{x0, r - .016}, {x0 + .011, r}, {x1 - .011, r}, {x1, r - .016}}, col, meshMetal)
	}
	rangedLathe(&m, [][2]float64{{-2.20, .052}, {-.6, .056}, {.98, .05}}, [3]int{146, 64, 30}, meshGrip)
	// Round knob with a dark groove near its end, then the heel ferrule.
	rangedLathe(&m, [][2]float64{{-2.43, 0}, {-2.42, .045}, {-2.39, .08}, {-2.34, .102}, {-2.28, .106}, {-2.22, .096}, {-2.18, .07}, {-2.15, .06}}, brass, meshMetal)
	rangedLathe(&m, [][2]float64{{-2.385, .084}, {-2.37, .088}}, dark, meshMetal)
	collar(-2.15, -2.09, .074, light)
	rangedLathe(&m, [][2]float64{{-2.09, .066}, {-1.95, .066}}, steel, meshMetal)
	collar(-1.95, -1.89, .074, light)
	// Mid-shaft cluster and the long brass sleeve under the socket.
	collar(-.68, -.61, .077, light)
	rangedLathe(&m, [][2]float64{{-.61, .07}, {-.47, .07}}, steel, meshMetal)
	collar(-.47, -.40, .079, light)
	rangedLathe(&m, [][2]float64{{-.40, .066}, {.25, .066}}, brass, meshMetal)
	for _, x := range []float64{-.30, -.08, .14} {
		collar(x, x+.05, .075, light)
	}
	rangedLathe(&m, [][2]float64{{.25, .06}, {.30, .06}}, dark, meshMetal)
	// The socket clasps the rim where the shaft crosses it.
	rangedLathe(&m, [][2]float64{{.30, .072}, {.32, .088}, {.54, .088}, {.56, .072}}, light, meshMetal)
	rangedLathe(&m, [][2]float64{{.56, .066}, {.60, .066}}, steel, meshMetal)

	const hub, rootR, tipR, innerR, half, edge = 1.05, .60, .70, .50, .045, .03
	// One tooth period: root, flank, flat top, flank, root. Sectors share
	// their vertices so the rim has no radial seams.
	fractions := []float64{0, .1, .2, .32, .5, .68, .8, .9}
	outer := func(f float64) float64 {
		switch {
		case f < .2 || f > .8:
			return rootR
		case f < .32:
			return rootR + (tipR-rootR)*(f-.2)/.12
		case f > .68:
			return tipR - (tipR-rootR)*(f-.68)/.12
		}
		return tipR
	}
	const teeth = 16
	var angles, radii []float64
	for i := 0; i < teeth; i++ {
		for _, f := range fractions {
			angles = append(angles, (float64(i)+f)*2*math.Pi/teeth)
			radii = append(radii, outer(f))
		}
	}
	at := func(a, r, z float64) [3]float64 { return [3]float64{hub + r*math.Cos(a), r * math.Sin(a), z} }
	profile := [][2]float64{{0, edge}, {.08, half}, {.92, half}, {1, edge}}
	for i := range angles {
		j := (i + 1) % len(angles)
		a, b, ra, rb := angles[i], angles[j], radii[i], radii[j]
		if j == 0 {
			b += 2 * math.Pi
		}
		for _, side := range []float64{-1, 1} {
			for k := 1; k < len(profile); k++ {
				p := func(ang, r float64, n int) [3]float64 {
					t := profile[n][0]
					return at(ang, r*(1-t)+innerR*t, side*profile[n][1])
				}
				rangedSurfaceTriangle(&m, p(a, ra, k-1), p(b, rb, k-1), p(a, ra, k), [3]float64{0, 0, side}, brass, meshMetal)
				rangedSurfaceTriangle(&m, p(a, ra, k), p(b, rb, k-1), p(b, rb, k), [3]float64{0, 0, side}, brass, meshMetal)
			}
		}
		// Tooth faces and flanks, then the rim's inner wall.
		pa, pb := at(a, ra, 0), at(b, rb, 0)
		out := [3]float64{pb[1] - pa[1], -(pb[0] - pa[0]), 0}
		rangedSurfaceTriangle(&m, at(a, ra, -edge), at(b, rb, -edge), at(a, ra, edge), out, brass, meshMetal)
		rangedSurfaceTriangle(&m, at(a, ra, edge), at(b, rb, -edge), at(b, rb, edge), out, brass, meshMetal)
		in := [3]float64{-math.Cos((a + b) / 2), -math.Sin((a + b) / 2), 0}
		rangedSurfaceTriangle(&m, at(a, innerR, -edge), at(b, innerR, -edge), at(a, innerR, edge), in, dark, meshMetal)
		rangedSurfaceTriangle(&m, at(a, innerR, edge), at(b, innerR, -edge), at(b, innerR, edge), in, dark, meshMetal)
	}
	var fine []rangedSection
	for i := 0; i <= 48; i++ {
		a := float64(i) * 2 * math.Pi / 48
		fine = append(fine, rangedSection{at(a, .45, 0), .021, .032})
	}
	rangedTube(&m, fine, light, meshMetal)
	// Beveled bars, broader at the hub, read as cast spokes rather than wire.
	for _, deg := range []float64{0, 60, -60, 120, -120} {
		a := deg * math.Pi / 180
		c, s := math.Cos(a), math.Sin(a)
		var bar [][2]float64
		for _, v := range [][2]float64{{.17, -.042}, {.51, -.032}, {.51, .032}, {.17, .042}} {
			bar = append(bar, [2]float64{hub + v[0]*c - v[1]*s, v[0]*s + v[1]*c})
		}
		rangedStockPlate(&m, bar, 0, .03, .45, brass, meshMetal)
	}
	center := [3]float64{hub, 0, 0}
	m.ring(center, .20, .036, light)
	rangedOrb(&m, center, [3]float64{.17, .17, .066}, brass, meshMetal)
	for _, side := range []float64{-1, 1} {
		m.ring([3]float64{hub, 0, side * .05}, .115, .013, dark)
	}
	rangedOrb(&m, center, [3]float64{.062, .062, .10}, steel, meshMetal)
	return m.faces
}
