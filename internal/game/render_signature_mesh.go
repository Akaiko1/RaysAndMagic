package game

import "math"

// relicBody names a relic drawn outside a hand.
type relicBody int

const (
	relicReturningGale relicBody = iota
	relicEverflowRetort
)

var relicMeshes volumeMeshCache[relicBody]

// buildRelicMesh builds the relic bodies drawn outside a hand: the thrown
// Returning Gale and the Everflow Retort. Stone Script is a melee body.
func buildRelicMesh(id relicBody) []modelFace {
	var m weaponMeshBuilder
	switch id {
	case relicReturningGale:
		m.returningGale()
		for i := range m.faces {
			f := &m.faces[i]
			f.a[0] += 1.60
			f.b[0] += 1.60
			f.c[0] += 1.60
			f.origin[0] += 1.60
		}
	case relicEverflowRetort:
		m.everflowRetort()
	}
	return m.faces
}

// A conventional double-edged blade has a shallow central fuller on each face.
// Three upright gold runes read from the tip toward the grip inside that recess.
func (m *weaponMeshBuilder) stoneScript() {
	jade, bronze := [3]int{44, 174, 123}, [3]int{213, 151, 66}
	// Gold Sword uses a .15 half-width. The extra face width leaves room for
	// readable runes without turning the cutting edges into raised rails.
	sections := make([]bladeSection, 13)
	for i := range sections {
		t := float64(i) / 12
		taper := math.Min(1, t*5)
		sections[i] = bladeSection{-3.60 * t, 0, .20 * taper, .06 * taper}
	}
	cross := func(p bladeSection, k int) [3]float64 {
		yz := [14][2]float64{{-1, 0}, {-.68, 1}, {-.40, 1}, {-.35, .75}, {.35, .75}, {.40, 1}, {.68, 1}, {1, 0}, {.68, -1}, {.40, -1}, {.35, -.75}, {-.35, -.75}, {-.40, -1}, {-.68, -1}}
		return [3]float64{p.x, yz[k][0] * p.width, yz[k][1] * p.thickness}
	}
	for i := 1; i < len(sections); i++ {
		for k := 0; k < 14; k++ {
			next := (k + 1) % 14
			a, b, c, d := cross(sections[i-1], k), cross(sections[i-1], next), cross(sections[i], k), cross(sections[i], next)
			col, mat := jade, meshMetal
			if k == 3 || k == 10 {
				col, mat = [3]int{38, 120, 85}, meshRuneInlay
			}
			m.triangle(a, b, c, [3]float64{(a[0] + c[0]) / 2, 0, 0}, col, mat)
			m.triangle(c, b, d, [3]float64{(a[0] + c[0]) / 2, 0, 0}, col, mat)
		}
	}
	// The inlaid glyphs are shaded directly on the recessed faces, so they
	// cannot sort behind a long blade triangle during a depth-rotated swing.
	for _, side := range []float64{-1, 1} {
		m.curve([3]float64{-3.70, 0, 0}, [3]float64{-3.82, side * .32, 0}, [3]float64{-3.62, side * .50, 0}, .07, bronze, meshMetal)
		m.curve([3]float64{-3.62, side * .50, 0}, [3]float64{-3.50, side * .50, 0}, [3]float64{-3.57, side * .62, 0}, .045, bronze, meshMetal)
	}
	m.gem([3]float64{-3.70, 0, 0}, [3]float64{.20, .19, .14}, bronze)
	for _, side := range []float64{-1, 1} {
		m.gem([3]float64{-3.69, 0, side * .15}, [3]float64{.13, .12, .035}, [3]int{16, 130, 91})
	}
	m.grip(-4.63, -3.89, 0, .10, meshDark, bronze)
	m.gem([3]float64{-4.83, 0, 0}, [3]float64{.17, .16, .13}, bronze)
}

func (m *weaponMeshBuilder) returningGale() {
	steel, blue, bronze := [3]int{183, 213, 239}, [3]int{37, 69, 122}, [3]int{211, 148, 64}
	sections := make([]bladeSection, 0, 17)
	for i := 0; i <= 16; i++ {
		t := float64(i) / 16
		sections = append(sections, bladeSection{-2.1 * t, -.18 * math.Sin(t*math.Pi*.85), .19 * math.Sin(math.Pi*t*.87), .065 * math.Min(1, t*5)})
	}
	m.blade(sections, steel, meshMetal)
	for _, side := range []float64{-1, 1} {
		for _, offset := range []float64{-.026, .026} {
			for i := 3; i < 15; i++ {
				a, b := sections[i], sections[i+1]
				m.rod([3]float64{a.x, a.y + offset, side * (a.thickness + .002)}, [3]float64{b.x, b.y + offset, side * (b.thickness + .002)}, .009, .009, blue, meshGrip)
			}
		}
	}
	m.curve([3]float64{-2.16, -.10, 0}, [3]float64{-2.22, .28, 0}, [3]float64{-1.94, .35, 0}, .05, bronze, meshMetal)
	m.curve([3]float64{-2.16, -.10, 0}, [3]float64{-2.18, -.27, 0}, [3]float64{-2.32, -.30, 0}, .04, bronze, meshMetal)
	m.grip(-2.94, -2.21, -.10, .085, blue, bronze)
	m.ring([3]float64{-3.14, -.10, 0}, .18, .035, bronze)
}

// The complete revolved bottle has a narrow neck and rounded shoulders. The
// two bands and loop handle remain separate rigid metal geometry.
func (m *weaponMeshBuilder) everflowRetort() {
	bronze, cyan := [3]int{225, 157, 65}, [3]int{93, 220, 237}
	profile := [][2]float64{{-1.12, .14}, {-.75, .14}, {-.58, .21}, {-.43, .38}, {-.20, .60}, {.05, .73}, {.37, .77}, {.64, .68}, {.85, .50}, {.99, .25}, {1.02, 0}}
	m.retortLathe(profile, cyan, meshRelicGlass)
	m.retortLathe([][2]float64{{-.02, 0}, {-.02, .67}, {.34, .70}, {.60, .62}, {.80, .45}, {.94, .22}, {.96, 0}}, [3]int{24, 162, 193}, meshRelicWater)
	m.rod([3]float64{0, -1.17, 0}, [3]float64{0, -1.07, 0}, .20, .20, bronze, meshMetal)
	m.rod([3]float64{0, -1.25, 0}, [3]float64{0, -1.17, 0}, .06, .20, bronze, meshMetal)
	for _, band := range [][2]float64{{.12, .76}, {.60, .70}} {
		for i := 0; i < 32; i++ {
			a, b := float64(i)*math.Pi/16, float64(i+1)*math.Pi/16
			m.rod([3]float64{band[1] * math.Cos(a), band[0], band[1] * math.Sin(a)}, [3]float64{band[1] * math.Cos(b), band[0], band[1] * math.Sin(b)}, .037, .037, bronze, meshMetal)
		}
	}
	m.curve([3]float64{.14, -.87, 0}, [3]float64{.55, -1.13, 0}, [3]float64{.82, -.86, 0}, .047, bronze, meshMetal)
	m.curve([3]float64{.82, -.86, 0}, [3]float64{1.04, -.42, 0}, [3]float64{.69, -.10, 0}, .047, bronze, meshMetal)
}

func (m *weaponMeshBuilder) retortLathe(profile [][2]float64, col [3]int, material int) {
	const sectors = 24
	point := func(i, j int) [3]float64 {
		a := float64(j) * 2 * math.Pi / sectors
		return [3]float64{profile[i][1] * math.Cos(a), profile[i][0], profile[i][1] * math.Sin(a)}
	}
	normal := func(i, j int) [3]float64 {
		lo, hi := max(0, i-1), min(len(profile)-1, i+1)
		dy, dr := profile[hi][0]-profile[lo][0], profile[hi][1]-profile[lo][1]
		n := math.Hypot(dy, dr)
		a := float64(j) * 2 * math.Pi / sectors
		return [3]float64{dy / n * math.Cos(a), -dr / n, dy / n * math.Sin(a)}
	}
	for i := 0; i < len(profile)-1; i++ {
		for j := 0; j < sectors; j++ {
			verts := [4][3]float64{point(i, j), point(i+1, j), point(i, j+1), point(i+1, j+1)}
			normals := [4][3]float64{normal(i, j), normal(i+1, j), normal(i, j+1), normal(i+1, j+1)}
			for _, tri := range [][3]int{{0, 1, 2}, {2, 1, 3}} {
				m.triangle(verts[tri[0]], verts[tri[1]], verts[tri[2]], [3]float64{0, (profile[i][0] + profile[i+1][0]) / 2, 0}, col, material)
				f := &m.faces[len(m.faces)-1]
				f.surface = &modelSurface{}
				for k, v := range tri {
					f.surface.normals[k] = normals[v]
					f.surface.uv[k] = [2]float64{verts[v][0], verts[v][1]}
				}
			}
		}
	}
}
