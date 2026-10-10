package game

import "math"

const (
	meleeBodyRainfang     = 18
	meleeBodyShellknocker = 65
)

// The cyan cutting surface curves continuously into a gold serpent guard.
// These are rigid facets and fittings, not a warped icon or a tinted dagger.
func (m *weaponMeshBuilder) rainfang() {
	ice := [3]int{102, 220, 235}
	teal := [3]int{21, 104, 113}
	gold := [3]int{205, 155, 61}
	sections := []bladeSection{
		{-2.35, -.62, .15, .085}, {-2.10, -.47, .26, .11},
		{-1.78, -.31, .32, .12}, {-1.40, -.16, .29, .105},
		{-1.00, -.045, .22, .08}, {-.60, .02, .14, .055},
		{-.25, .035, .055, .025}, {0, 0, 0, 0},
	}
	m.blade(sections, ice, meshStone)
	for _, side := range []float64{-1, 1} {
		for i := 1; i < len(sections)-1; i++ {
			a, b := sections[i-1], sections[i]
			m.rod([3]float64{a.x, a.y + a.width*.24, side * (a.thickness + .005)},
				[3]float64{b.x, b.y + b.width*.24, side * (b.thickness + .005)}, .015, .009, [3]int{195, 249, 246}, meshGlow)
		}
		// Two hooked jaw curves surround the root, with a turquoise eye.
		z := side * .115
		m.curve([3]float64{-2.57, -.62, z}, [3]float64{-2.33, -.02, z}, [3]float64{-1.93, -.08, z}, .055, gold, meshMetal)
		m.curve([3]float64{-2.49, -.64, z}, [3]float64{-2.24, -1.09, z}, [3]float64{-1.94, -.82, z}, .045, gold, meshMetal)
		m.gem([3]float64{-2.38, -.39, side * .18}, [3]float64{.105, .07, .055}, teal)
	}
	m.grip(-3.45, -2.48, -.62, .105, teal, gold)
	for i := 0; i < 5; i++ {
		x := -3.35 + float64(i)*.18
		m.curve([3]float64{x, -.73, -.06}, [3]float64{x + .10, -.62, -.17}, [3]float64{x + .16, -.51, -.06}, .018, gold, meshMetal)
		m.curve([3]float64{x, -.51, .06}, [3]float64{x + .10, -.62, .17}, [3]float64{x + .16, -.73, .06}, .018, gold, meshMetal)
	}
	m.gem([3]float64{-3.58, -.62, 0}, [3]float64{.15, .14, .14}, gold)
	m.gem([3]float64{-3.65, -.62, -.10}, [3]float64{.08, .085, .055}, ice)
}

// A single bronze crescent, with a domed olive carapace over its socket.
// Local +Y is the cutting side; the haft extends along -X toward the hand.
func (m *weaponMeshBuilder) shellknocker() {
	bronze := [3]int{177, 117, 49}
	edge := [3]int{230, 176, 92}
	shell := [3]int{80, 91, 45}
	leather := [3]int{72, 42, 25}
	m.grip(-4.0, -1.45, 0, .095, leather, bronze)
	m.rod([3]float64{-1.45, 0, 0}, [3]float64{.15, 0, 0}, .115, .13, leather, meshGrip)
	m.gem([3]float64{-4.12, 0, 0}, [3]float64{.14, .13, .13}, bronze)
	// Strip construction preserves the concave inner edge and serrated bevel.
	for i := 0; i < 18; i++ {
		point := func(j int, band int, side float64) [3]float64 {
			t := float64(j) / 18
			x := -1.48 + 1.90*t
			inner := .23 + .12*math.Sin(t*math.Pi)
			outer := .27 + .91*math.Sin(t*math.Pi)
			if j%3 == 1 {
				outer += .09
			}
			y, z := inner, side*.11
			if band == 1 {
				y = inner + (outer-inner)*.79
				z = side * .07
			}
			if band == 2 {
				y = outer
				z = 0
			}
			return [3]float64{x, y, z}
		}
		for _, side := range []float64{-1, 1} {
			for band := 0; band < 2; band++ {
				a, b, c, d := point(i, band, side), point(i+1, band, side), point(i, band+1, side), point(i+1, band+1, side)
				col := bronze
				if band == 1 {
					col = edge
				}
				o := [3]float64{(a[0] + b[0]) / 2, (a[1] + c[1]) / 2, 0}
				m.triangle(a, b, c, o, col, meshMetal)
				m.triangle(c, b, d, o, col, meshMetal)
			}
		}
		// Close the socket-facing edge, which is visible as the axe rolls.
		a, b, c, d := point(i, 0, -1), point(i+1, 0, -1), point(i, 0, 1), point(i+1, 0, 1)
		m.triangle(a, c, b, [3]float64{a[0], a[1] + .05, 0}, bronze, meshMetal)
		m.triangle(b, c, d, [3]float64{b[0], b[1] + .05, 0}, bronze, meshMetal)
	}
	center := [3]float64{-.55, .045, 0}
	point := func(a, b float64) [3]float64 {
		return [3]float64{center[0] + .64*math.Sin(a), center[1] + .33*math.Cos(a)*math.Cos(b), .29 * math.Cos(a) * math.Sin(b)}
	}
	for i := 0; i < 8; i++ {
		for j := 0; j < 12; j++ {
			a, b := -math.Pi/2+float64(i)*math.Pi/8, -math.Pi/2+float64(i+1)*math.Pi/8
			c, d := float64(j)*math.Pi/6, float64(j+1)*math.Pi/6
			m.triangle(point(a, c), point(b, c), point(a, d), center, shell, meshStone)
			m.triangle(point(a, d), point(b, c), point(b, d), center, shell, meshStone)
		}
	}
	for _, side := range []float64{-1, 1} {
		// Raised longitudinal seam and three curved brass carapace ribs.
		m.curve([3]float64{-1.13, .045, side * .08}, [3]float64{-.55, .045, side * .47}, [3]float64{.04, .045, side * .08}, .025, edge, meshMetal)
		for i := 0; i < 3; i++ {
			x := -.92 + float64(i)*.34
			m.curve([3]float64{x, -.24, 0}, [3]float64{x + .12, .045, side * .52}, [3]float64{x, .33, 0}, .03, bronze, meshMetal)
		}
	}
	m.rod([3]float64{.02, 0, 0}, [3]float64{.35, -.12, 0}, .12, 0, bronze, meshMetal)
}
