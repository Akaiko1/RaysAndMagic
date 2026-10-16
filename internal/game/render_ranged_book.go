package game

import "math"

func rangedBookSkull(m *weaponMeshBuilder, c [3]float64) {
	bone, rim, shade := [3]int{190, 159, 108}, [3]int{205, 176, 123}, [3]int{81, 60, 36}
	// The sockets are recessed openings bounded by the brow and cheek bones.
	// Their shared surface replaces the old spheres laid over a rounded head.
	outer := [][3]float64{{0, -.065, .090}, {0, -.265, .047}, {.100, -.248, .034}, {.171, -.183, .012}, {.190, -.046, .002}, {.178, .060, .012}, {.150, .109, .038}, {.074, .127, .071}, {0, .048, .085}}
	socket := [][3]float64{{.035, -.016, .088}, {.046, -.045, .085}, {.087, -.052, .079}, {.129, -.035, .062}, {.143, .003, .056}, {.131, .040, .061}, {.105, .055, .076}, {.068, .047, .084}, {.043, .027, .089}}
	for _, side := range []float64{-1, 1} {
		point := func(p [3]float64) [3]float64 { return [3]float64{c[0] + side*p[0], c[1] + p[1], c[2] + p[2]} }
		origin := point([3]float64{.08, -.02, -.20})
		face := func(a, b, d [3]float64, col [3]int) { m.triangle(point(a), point(b), point(d), origin, col, meshStone) }
		for i, a := range outer {
			j := (i + 1) % len(outer)
			b, u, v := outer[j], socket[i], socket[j]
			midA, midB := a, b
			for axis := range a {
				midA[axis] = (a[axis] + u[axis]) * .5
				midB[axis] = (b[axis] + v[axis]) * .5
			}
			midA[2] += .012
			midB[2] += .012
			face(a, b, midA, bone)
			face(midA, b, midB, bone)
			face(midA, midB, u, rim)
			face(u, midB, v, rim)
			// Inset the cavity walls and their dark floor. No black geometry sits
			// above the brow, and nothing spans across an eye opening.
			lowU, lowV := u, v
			lowU[0] = .085 + (u[0]-.085)*.86
			lowU[1] = .006 + (u[1]-.006)*.86
			lowU[2] = .018
			lowV[0] = .085 + (v[0]-.085)*.86
			lowV[1] = .006 + (v[1]-.006)*.86
			lowV[2] = .018
			face(u, v, lowU, shade)
			face(lowU, v, lowV, shade)
			face(lowU, lowV, [3]float64{.085, .006, .018}, [3]int{24, 22, 18})
			backA, backB := a, b
			backA[2] = -.005
			backB[2] = -.005
			face(a, backA, b, shade)
			face(b, backA, backB, shade)
		}
		// Upper jaw flares beneath the cheek; the nasal aperture narrows upward.
		bridge, cheek, jaw, lip := [3]float64{0, .048, .085}, [3]float64{.074, .127, .071}, [3]float64{.077, .161, .067}, [3]float64{0, .169, .083}
		noseTop, noseSide, noseBase := [3]float64{.009, .070, .089}, [3]float64{.023, .120, .085}, [3]float64{0, .127, .086}
		face(bridge, cheek, noseTop, bone)
		face(noseTop, cheek, noseSide, bone)
		face(noseSide, cheek, jaw, rim)
		face(noseSide, jaw, lip, bone)
		face(noseSide, lip, noseBase, bone)
		face(noseTop, noseSide, [3]float64{0, .099, .025}, shade)
		face(noseSide, noseBase, [3]float64{0, .099, .025}, [3]int{28, 23, 17})
		face(bridge, noseTop, [3]float64{0, .099, .025}, shade)
		// Angular cheek arch and lower jaw, tapering to a small rounded chin.
		rangedMetalCurve(m, [][3]float64{point([3]float64{.150, .096, .021}), point([3]float64{.111, .191, .027}), point([3]float64{.065, .222, .046}), point([3]float64{0, .229, .057})}, []float64{.012, .016, .019, .019}, bone, meshStone)
	}
	// Teeth are short individual facets in a narrow mouth, not one large grin.
	for i := 0; i < 8; i++ {
		x := -.061 + float64(i)*.0174
		y := .176 - .008*math.Abs(x)/.061
		m.box([3]float64{c[0] + x, c[1] + y, c[2] + .087}, [3]float64{.007, .011, .007}, .004, rim, meshStone)
		m.box([3]float64{c[0] + x*.92, c[1] + y + .024, c[2] + .080}, [3]float64{.0065, .009, .006}, .003, bone, meshStone)
	}
}

// The page block hinges with the cover. Cover hardware stays on its outside;
// ink is a material on the exposed page faces, not geometry floating above them.
// Parts are the left cover, the right cover and the spine; bookHinge opens them.
func buildBookHeldModel() [][]modelFace {
	var parts [][]modelFace
	m := weaponMeshBuilder{}
	leather, paper, iron, edge := [3]int{37, 32, 34}, [3]int{205, 180, 130}, [3]int{103, 102, 94}, [3]int{176, 168, 143}
	for _, side := range []float64{-1, 1} {
		half := weaponMeshBuilder{}
		half.box([3]float64{side * .49, 0, .09}, [3]float64{.49, .69, .055}, .025, leather, meshGrip)
		// Recessed inset panel and double metal edging follow the cover silhouette.
		outline := [][2]float64{{side * .12, -.56}, {side * .75, -.56}, {side * .88, -.43}, {side * .88, .43}, {side * .75, .56}, {side * .12, .56}}
		rangedPlate(&half, outline, .147, .012, .035, [3]int{48, 43, 42}, meshGrip)
		for i, a := range outline {
			b := outline[(i+1)%len(outline)]
			half.rod([3]float64{a[0], a[1], .164}, [3]float64{b[0], b[1], .164}, .008, .008, iron, meshMetal)
			// A second inset border follows the same chamfered leather panel.
			half.rod([3]float64{side*.49 + (a[0]-side*.49)*.91, a[1] * .94, .163}, [3]float64{side*.49 + (b[0]-side*.49)*.91, b[1] * .94, .163}, .0035, .0035, [3]int{114, 104, 83}, meshMetal)
		}
		for _, x := range []float64{.13, .86} {
			for _, y := range []float64{-.58, .58} {
				rangedPlate(&half, [][2]float64{{side * (x - .08), y - .06}, {side * (x + .08), y - .06}, {side * (x + .08), y + .06}, {side * x, y + .10}, {side * (x - .08), y + .06}}, .156, .02, .15, iron, meshMetal)
				// Recessed angular tooling and two small rivets replace the plain
				// diagonal slash. All fittings stand clear of the outside cover.
				corner := [][2]float64{{-.053, .031}, {-.053, -.036}, {.047, -.036}, {.047, .031}, {0, .070}, {-.053, .031}}
				for i := 1; i < len(corner); i++ {
					a, b := corner[i-1], corner[i]
					half.rod([3]float64{side * (x + a[0]), y + a[1], .180}, [3]float64{side * (x + b[0]), y + b[1], .180}, .005, .005, [3]int{48, 46, 39}, meshMetal)
				}
				for _, dx := range []float64{-.035, .031} {
					rangedOrb(&half, [3]float64{side * (x + dx), y - .017, .185}, [3]float64{.010, .010, .006}, edge, meshMetal)
				}
			}
		}
		// Hinged clasps on the fore-edge and raised bands on the binding.
		for _, y := range []float64{-.36, .36} {
			half.box([3]float64{side * .96, y, .035}, [3]float64{.045, .045, .12}, .012, iron, meshMetal)
			half.box([3]float64{side * .89, y, .16}, [3]float64{.075, .035, .015}, .008, edge, meshMetal)
			half.box([3]float64{side * .91, y, .178}, [3]float64{.028, .017, .003}, .002, [3]int{73, 70, 58}, meshMetal)
		}
		if side > 0 {
			// The icon has a single bronze sunburst mount: a raised oval rim
			// joins broad, beveled points. It is not eight detached ivory spikes.
			ringCenter := [3]float64{.49, -.06, .179}
			bronze, bevel := [3]int{118, 100, 73}, [3]int{175, 152, 111}
			for i := 0; i < 32; i++ {
				a, b := float64(i)*math.Pi/16, float64(i+1)*math.Pi/16
				p := [3]float64{ringCenter[0] + .214*math.Cos(a), ringCenter[1] + .270*math.Sin(a), ringCenter[2]}
				q := [3]float64{ringCenter[0] + .214*math.Cos(b), ringCenter[1] + .270*math.Sin(b), ringCenter[2]}
				half.rod(p, q, .021, .021, bronze, meshMetal)
			}
			for i := 0; i < 8; i++ {
				a := float64(i)*math.Pi/4 - math.Pi/2
				ca, sa := math.Cos(a), math.Sin(a)
				length := .32
				if i == 0 || i == 4 {
					length = .39
				}
				if i%2 == 1 {
					length = .305
				}
				root := [2]float64{.49 + .204*ca, -.06 + .262*sa}
				outline := [][2]float64{{root[0] - .050*sa, root[1] + .050*ca}, {.49 + length*ca, -.06 + length*sa}, {root[0] + .050*sa, root[1] - .050*ca}, {.49 + .184*ca, -.06 + .235*sa}}
				rangedPlate(&half, outline, .180, .021, .26, bronze, meshMetal)
				// The thin light ridge belongs to the upper bevel, with a darker
				// flank so the emblem reads as forged hardware on the leather.
				half.rod([3]float64{root[0] - .032*sa, root[1] + .032*ca, .202}, [3]float64{.49 + (length-.019)*ca, -.06 + (length-.019)*sa, .185}, .006, .001, bevel, meshMetal)
			}

			rangedBookSkull(&half, [3]float64{.49, -.06, .19})
			// Alternating links cross the lower cover and hang beyond the fore-edge.
			for i := 0; i < 20; i++ {
				t := float64(i) / 19
				h := weaponMeshBuilder{}
				h.ring([3]float64{}, .036, .008, edge)
				a := float64(i%2) * 1.10
				ca, sa := math.Cos(a), math.Sin(a)
				direction := math.Atan2(.55, .99)
				cd, sd := math.Cos(direction), math.Sin(direction)
				cx, cy, cz := .02+.99*t, .05+.55*t, .218
				half.appendMapped(h.faces, func(p [3]float64) [3]float64 {
					x, y, z := p[0]*1.17, p[1]*.70, p[2]
					y, z = y*ca-z*sa, y*sa+z*ca
					return [3]float64{cx + x*cd - y*sd, cy + x*sd + y*cd, cz + z}
				})
			}
		}
		block := weaponMeshBuilder{}
		block.box([3]float64{side * .48, 0, -.026}, [3]float64{.446, .632, .055}, .014, paper, meshGrip)
		for _, f := range block.faces {
			// The hidden page back is glued to the cover; it is not an
			// exposed face when the book is reflected toward the target.
			if f.a[2] > .014 && f.b[2] > .014 && f.c[2] > .014 {
				continue
			}
			if math.Abs(f.a[2]+.081) < 1e-6 && math.Abs(f.b[2]+.081) < 1e-6 && math.Abs(f.c[2]+.081) < 1e-6 {
				// Ink runs from the spine on both pages.
				n := [3]float64{0, 0, -1}
				f.material = meshBookPage
				f.surface = &modelSurface{normals: [3][3]float64{n, n, n}, uv: [3][2]float64{{math.Abs(f.a[0]), f.a[1]}, {math.Abs(f.b[0]), f.b[1]}, {math.Abs(f.c[0]), f.c[1]}}}
			}
			half.faces = append(half.faces, f)
		}
		// Fine stacked leaf edges only along exposed edges, with no coplanar noise.
		for i := 0; i < 5; i++ {
			z := -.065 + float64(i)*.019
			half.rod([3]float64{side * .924, -.59, z}, [3]float64{side * .924, .59, z}, .003, .003, [3]int{122, 95, 62}, meshGrip)
		}
		parts = append(parts, half.faces)
	}
	m.box([3]float64{0, 0, .12}, [3]float64{.061, .69, .08}, .025, leather, meshGrip)
	for _, y := range []float64{-.53, -.28, .28, .53} {
		m.box([3]float64{0, y, .18}, [3]float64{.068, .026, .027}, .013, iron, meshMetal)
	}
	return append(parts, m.faces)
}
