package game

import "math"

// Geometry and palette are part of the procedural asset, like pixels in an icon.
// Combat values and effect selection remain in weapons.yaml.
const (
	meshMetal = iota
	meshGrip
	meshStone
	meshGlow
)

var (
	meshSteel  = [3]int{172, 187, 199}
	meshSilver = [3]int{217, 230, 238}
	meshGold   = [3]int{218, 159, 64}
	meshDark   = [3]int{54, 62, 69}
	meshWood   = [3]int{101, 54, 26}
	meshRed    = [3]int{127, 28, 25}
	meshBone   = [3]int{221, 205, 153}
	meshJade   = [3]int{35, 151, 116}
)

type weaponMeshBuilder struct{ faces []modelFace }
type bladeSection struct{ x, y, width, thickness float64 }

func (m *weaponMeshBuilder) triangle(a, b, c, o [3]float64, col [3]int, material int) {
	m.faces = append(m.faces, modelFace{a, b, c, o, col, material})
}
func (m *weaponMeshBuilder) box(c, h [3]float64, bevel float64, col [3]int, material int) {
	beveledBoxFaces(c, h, math.Min(bevel, math.Min(h[0], math.Min(h[1], h[2]))*.9), 0, func(a, b, d, o [3]float64) { m.triangle(a, b, d, o, col, material) })
}
func (m *weaponMeshBuilder) rod(a, b [3]float64, ra, rb float64, col [3]int, material int) {
	axis := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	length := math.Sqrt(axis[0]*axis[0] + axis[1]*axis[1] + axis[2]*axis[2])
	if length < 1e-8 {
		return
	}
	for i := range axis {
		axis[i] /= length
	}
	u := [3]float64{axis[1], -axis[0], 0}
	l := math.Hypot(u[0], u[1])
	if l < .01 {
		u = [3]float64{1, 0, 0}
		l = 1
	}
	for i := range u {
		u[i] /= l
	}
	v := [3]float64{axis[1]*u[2] - axis[2]*u[1], axis[2]*u[0] - axis[0]*u[2], axis[0]*u[1] - axis[1]*u[0]}
	point := func(c [3]float64, r, t float64) [3]float64 {
		return [3]float64{c[0] + r*(u[0]*math.Cos(t)+v[0]*math.Sin(t)), c[1] + r*(u[1]*math.Cos(t)+v[1]*math.Sin(t)), c[2] + r*(u[2]*math.Cos(t)+v[2]*math.Sin(t))}
	}
	o := [3]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2] + b[2]) / 2}
	const sides = 8
	for i := 0; i < sides; i++ {
		t, q := float64(i)*2*math.Pi/sides, float64(i+1)*2*math.Pi/sides
		p0, p1, p2, p3 := point(a, ra, t), point(a, ra, q), point(b, rb, t), point(b, rb, q)
		m.triangle(p0, p1, p2, o, col, material)
		m.triangle(p2, p1, p3, o, col, material)
		m.triangle(a, p1, p0, o, col, material)
		m.triangle(b, p2, p3, o, col, material)
	}
}
func (m *weaponMeshBuilder) curve(a, b, c [3]float64, r float64, col [3]int, material int) {
	prev := a
	for i := 1; i <= 8; i++ {
		t := float64(i) / 8
		q := [3]float64{}
		for j := range q {
			q[j] = (1-t)*(1-t)*a[j] + 2*(1-t)*t*b[j] + t*t*c[j]
		}
		m.rod(prev, q, r, r, col, material)
		prev = q
	}
}
func (m *weaponMeshBuilder) ring(c [3]float64, radius, wire float64, col [3]int) {
	for i := 0; i < 16; i++ {
		a, b := float64(i)*math.Pi/8, float64(i+1)*math.Pi/8
		m.rod([3]float64{c[0] + radius*math.Cos(a), c[1] + radius*math.Sin(a), c[2]}, [3]float64{c[0] + radius*math.Cos(b), c[1] + radius*math.Sin(b), c[2]}, wire, wire, col, meshMetal)
	}
}

// The narrow edge and broad central ridge are actual faces, not a painted quad.
func (m *weaponMeshBuilder) blade(s []bladeSection, col [3]int, material int) {
	vertex := func(p bladeSection, k int) [3]float64 {
		y, z := 0.0, 0.0
		switch k {
		case 0:
			y = -p.width
		case 1:
			y = -p.width * .36
			z = -p.thickness
		case 2:
			y = p.width * .36
			z = -p.thickness
		case 3:
			y = p.width
		case 4:
			y = p.width * .36
			z = p.thickness
		case 5:
			y = -p.width * .36
			z = p.thickness
		}
		return [3]float64{p.x, p.y + y, z}
	}
	for i := 1; i < len(s); i++ {
		a, b := s[i-1], s[i]
		o := [3]float64{(a.x + b.x) / 2, (a.y + b.y) / 2, 0}
		for k := 0; k < 6; k++ {
			j := (k + 1) % 6
			v0, v1, v2, v3 := vertex(a, k), vertex(a, j), vertex(b, k), vertex(b, j)
			m.triangle(v0, v1, v2, o, col, material)
			m.triangle(v2, v1, v3, o, col, material)
		}
	}
	for _, i := range []int{0, len(s) - 1} {
		a := s[i]
		o := [3]float64{s[len(s)/2].x, s[len(s)/2].y, 0}
		for k := 0; k < 6; k++ {
			m.triangle([3]float64{a.x, a.y, 0}, vertex(a, k), vertex(a, (k+1)%6), o, col, material)
		}
	}
}
func (m *weaponMeshBuilder) grip(from, to, y, r float64, col, fitting [3]int) {
	m.rod([3]float64{from, y, 0}, [3]float64{to, y, 0}, r, r, col, meshGrip)
	for x := from + .07; x < to; x += .12 {
		m.rod([3]float64{x - .018, y, 0}, [3]float64{x + .018, y, 0}, r*1.09, r*1.09, mixColor(col, fitting, .23), meshGrip)
	}
	for _, x := range []float64{from, to} {
		m.rod([3]float64{x - .035, y, 0}, [3]float64{x + .035, y, 0}, r*1.3, r*1.3, fitting, meshMetal)
	}
}
func (m *weaponMeshBuilder) gem(c, h [3]float64, col [3]int) {
	m.box(c, h, math.Min(h[0], math.Min(h[1], h[2]))*.75, col, meshStone)
}

var meleeWeaponMeshes volumeMeshCache

func buildMeleeWeaponMesh(id int) []modelFace {
	var m weaponMeshBuilder
	switch {
	case id < 20:
		m.sword(id)
	case id < 40:
		m.pole(id)
	case id < 50:
		m.heavy(id)
	case id == 53:
		m.claws()
	case id == 54:
		m.fan()
	case id < 70:
		m.axe(id)
	default:
		m.clock(id)
	}
	// Register the actual leading edge at zero, not the head's socket.
	// The same point is used by the slash ribbon and contact debris.
	contact := [3]float64{}
	switch {
	case id >= 40 && id <= 49:
		contact = [3]float64{-.45, .70, 0}
		if id == 43 {
			contact = [3]float64{-.18, .27, 0}
		}
		if id == 45 {
			contact[1] = .80
		}
		if id >= 46 {
			contact[1] = .92
		}
		if id == 47 {
			contact[1] = .88
		}
		if id == 49 {
			contact[1] = 1.08
		}
	case id >= 60 && id <= 64:
		contact = [3]float64{-.60, .98, 0}
		if id == 60 {
			contact[1] *= .87
		}
	case id == 71:
		contact = [3]float64{-.5, .98, 0}
	case id == 72:
		contact = [3]float64{-.55, .64, 0}
	}
	for i := range m.faces {
		f := &m.faces[i]
		for _, p := range []*[3]float64{&f.a, &f.b, &f.c, &f.origin} {
			for j := range p {
				p[j] -= contact[j]
			}
			// Drakefang curves the other way in its source construction.
			// Register their convex cutting side at +Y like the other blades.
			if id == 15 {
				p[1] = -p[1]
			}
		}
	}
	return m.faces
}

func (m *weaponMeshBuilder) sword(id int) {
	if id == 13 {
		m.hairpin()
		return
	}
	length, width, curve := 3.4, .15, 0.0
	steel, guard, grip := meshSteel, meshSteel, meshWood
	switch id {
	case 1:
		length = 3.6
		steel = meshSilver
		grip = [3]int{32, 55, 101}
	case 2:
		steel = meshGold
		guard = meshGold
		grip = meshRed
	case 3:
		length = 2.65
		width = .22
		guard = meshGold
		grip = meshRed
	case 4, 5, 6:
		length = 3.9
		width = .095
		curve = -.38
		guard = meshGold
		grip = meshDark
		if id == 5 {
			length = 2.75
			curve = -.23
		}
		if id == 6 {
			steel = [3]int{137, 25, 30}
			guard = [3]int{128, 59, 31}
		}
	case 7:
		length = 2.65
		width = .27
		curve = .11
		steel = [3]int{103, 98, 82}
		guard = meshGold
		grip = [3]int{71, 83, 34}
	case 8, 9:
		length = 1.7
		width = .17
		if id == 8 {
			grip = meshDark
		} else {
			grip = meshRed
			guard = meshGold
		}
	case 11:
		length = 1.35
		width = .24
		grip = meshDark
		steel = [3]int{88, 88, 93}
	case 12:
		length = 2.15
		width = .27
		curve = .62
		steel = meshBone
		guard = meshGold
		grip = [3]int{25, 64, 44}
	case 14, 15:
		length = 1.9
		width = .29
		curve = .35
		steel = meshBone
		guard = meshGold
		grip = meshRed
		if id == 15 {
			length = 3.4
		}
	case 16:
		length = 2.6
		width = .20
		steel = [3]int{30, 70, 84}
		guard = [3]int{119, 112, 71}
		grip = meshDark
	case 17:
		length = 3.2
		width = .15
		curve = -.40
		steel = [3]int{91, 171, 207}
		guard = meshGold
		grip = [3]int{32, 75, 111}
	}
	sections := make([]bladeSection, 0, 14)
	for i := 0; i <= 12; i++ {
		t := float64(i) / 12
		w := width
		if id == 11 {
			w *= math.Sin(math.Pi * t)
		} else {
			w *= math.Min(1, (1-t)*5)
		}
		if id == 7 {
			w *= .5 + .75*math.Sin(math.Pi*t*.8)
		}
		if id == 14 || id == 15 {
			w *= .78 + .2*math.Sin(t*math.Pi)
			if i%2 == 0 && i < 11 {
				w += .07
			}
		}
		sections = append(sections, bladeSection{-length + length*t, curve * (t*t - 1), w, .045 * math.Min(1, (1-t)*6)})
	}
	m.blade(sections, steel, meshMetal)
	rootY := -curve
	if id == 8 || id == 12 || id == 14 || id == 15 || id == 16 || id == 17 {
		inlay := [3]int{64, 164, 64}
		if id == 15 {
			inlay = meshRed
		}
		if id == 16 {
			inlay = [3]int{30, 237, 244}
		}
		if id == 17 {
			inlay = [3]int{109, 224, 250}
		}
		for _, side := range []float64{-1, 1} {
			for i := 1; i < len(sections)-1; i++ {
				a, b := sections[i-1], sections[i]
				m.rod([3]float64{a.x, a.y, .048 * side}, [3]float64{b.x, b.y, .048 * side}, .032, .025, inlay, meshGlow)
			}
		}
	}
	m.grip(-length-.78, -length-.1, rootY, .09, grip, guard)
	if id == 11 {
		m.ring([3]float64{-length - 1.0, rootY, 0}, .15, .032, steel)
		return
	}
	if id == 4 || id == 5 || id == 6 {
		m.rod([3]float64{-length - .055, rootY, 0}, [3]float64{-length + .015, rootY, 0}, .22, .22, guard, meshMetal)
	} else {
		for _, side := range []float64{-1, 1} {
			m.curve([3]float64{-length - .04, rootY, 0}, [3]float64{-length - .15, rootY + side*.34, 0}, [3]float64{-length + .12, rootY + side*.40, 0}, .045, guard, meshMetal)
		}
	}
	m.gem([3]float64{-length - .9, rootY, 0}, [3]float64{.12, .12, .12}, guard)
	if id == 12 || id == 14 || id == 15 {
		m.curve([3]float64{-length - .86, rootY, 0}, [3]float64{-length - 1.10, rootY - .5, 0}, [3]float64{-length - .51, rootY - .32, 0}, .057, guard, meshMetal)
		m.rod([3]float64{-length - .6, rootY - .33, 0}, [3]float64{-length - .39, rootY - .13, 0}, .11, 0, meshBone, meshMetal)
	}
	if id == 12 {
		m.curve([3]float64{-length - .88, rootY + .03, 0}, [3]float64{-length - .51, rootY + .55, 0}, [3]float64{-length - .08, rootY + .15, 0}, .044, meshGold, meshMetal)
		m.rod([3]float64{-length - .92, rootY + .1, 0}, [3]float64{-length - .9, rootY + .61, 0}, .018, .018, meshGold, meshMetal)
		m.gem([3]float64{-length - .9, rootY + .68, 0}, [3]float64{.055, .13, .065}, meshJade)
	}
	if id == 2 || id == 8 || id == 17 {
		m.gem([3]float64{-length - .055, rootY, -.09}, [3]float64{.095, .10, .04}, meshJade)
	}
	if id == 16 {
		m.box([3]float64{-length - .08, rootY, 0}, [3]float64{.16, .26, .15}, .06, guard, meshMetal)
		m.ring([3]float64{-length - .08, rootY, -.16}, .13, .028, [3]int{39, 225, 248})
	}
}

func (m *weaponMeshBuilder) hairpin() {
	m.rod([3]float64{-2.1, 0, 0}, [3]float64{-.40, 0, 0}, .047, .045, meshDark, meshMetal)
	m.rod([3]float64{-.4, 0, 0}, [3]float64{}, .046, 0, meshSilver, meshMetal)
	for _, x := range []float64{-.5, -1.5} {
		m.rod([3]float64{x - .04, 0, 0}, [3]float64{x + .04, 0, 0}, .058, .058, meshGold, meshMetal)
	}
	c := [3]float64{-2.16, 0, 0}
	m.gem(c, [3]float64{.19, .12, .1}, meshGold)
	for i := 0; i < 4; i++ {
		f := float64(i) - 1.5
		for _, sg := range []float64{-1, 1} {
			m.curve([3]float64{c[0] + f*.06, sg * .06, 0}, [3]float64{c[0] + f*.28, sg * .5, 0}, [3]float64{c[0] + f*.37 + .12, sg * .36, 0}, .022, meshGold, meshMetal)
		}
	}
	m.rod([3]float64{-2.35, .22, 0}, [3]float64{-2.35, .60, 0}, .014, .014, meshGold, meshMetal)
	m.gem([3]float64{-2.35, .70, 0}, [3]float64{.10, .14, .065}, meshJade)
}

func (m *weaponMeshBuilder) pole(id int) {
	shaft, steel := meshWood, meshSteel
	length, width := 1.25, .18
	switch id {
	case 21:
		steel = [3]int{106, 101, 86}
	case 22:
		shaft = meshRed
	case 23:
		length = 1.5
		width = .085
	case 24:
		length = 1.5
		width = .10
		shaft = [3]int{177, 139, 65}
	case 25:
		length = 1.85
		width = .26
		steel = [3]int{126, 157, 56}
	case 26:
		length = 1.35
		steel = meshDark
	case 27:
		length = 1.95
		width = .29
		steel = meshGold
		shaft = meshRed
	case 28, 29:
		shaft = meshRed
	case 30:
		length = 1.9
		width = .13
		shaft = meshDark
	case 31:
		length = 1.5
		width = .20
		steel = meshDark
		shaft = [3]int{58, 44, 36}
	}
	m.rod([3]float64{-5, 0, 0}, [3]float64{-length + .08, 0, 0}, .063, .065, shaft, meshGrip)
	for _, x := range []float64{-4.95, -length, -length - .16} {
		m.rod([3]float64{x - .04, 0, 0}, [3]float64{x + .04, 0, 0}, .086, .086, meshGold, meshMetal)
	}
	if id == 28 || id == 29 {
		col := meshSilver
		if id == 29 {
			col = meshGold
		}
		m.blade([]bladeSection{{-1.5, 0, .08, .04}, {-.55, 0, .15, .05}, {0, 0, 0, 0}}, col, meshMetal)
		for _, sg := range []float64{-1, 1} {
			m.curve([3]float64{-1.45, 0, 0}, [3]float64{-1.55, sg * .50, 0}, [3]float64{-.6, sg * .5, 0}, .053, col, meshMetal)
			m.blade([]bladeSection{{-.75, sg * .5, .055, .035}, {-.39, sg * .5, .105, .04}, {-.1, sg * .5, 0, 0}}, col, meshMetal)
		}
		if id == 29 {
			m.gem([3]float64{-1.05, 0, 0}, [3]float64{.22, .12, .08}, meshJade)
		}
	} else {
		sections := []bladeSection{}
		for i := 0; i <= 10; i++ {
			t := float64(i) / 10
			w := width * math.Sin(math.Pi*t)
			y := 0.0
			if id == 30 {
				y = .35 * (1 - t*t)
				w = width * math.Min(1, (1-t)*4)
			}
			if (id == 25 || id == 27) && i > 1 && i < 9 && i%2 == 0 {
				w += .08
			}
			sections = append(sections, bladeSection{-length + length*t, y, w, .055 * math.Sin(math.Pi*t)})
		}
		m.blade(sections, steel, meshMetal)
		if id == 30 {
			m.rod([3]float64{-length - .16, 0, 0}, [3]float64{-length, .35, 0}, .065, .07, meshGold, meshMetal)
		}
		if id == 27 || id == 31 {
			col := meshRed
			if id == 31 {
				col = [3]int{255, 120, 20}
			}
			for _, sg := range []float64{-1, 1} {
				m.rod([3]float64{-length + .2, 0, sg * .058}, [3]float64{-.25, 0, sg * .027}, .03, .008, col, meshGlow)
			}
		}
	}
	if id == 24 || id == 26 || id == 28 || id == 30 {
		col := meshJade
		if id == 26 {
			col = meshBone
		}
		if id == 28 {
			col = meshRed
		}
		if id == 30 {
			col = [3]int{92, 37, 129}
		}
		for k := 0; k < 3; k++ {
			y := float64(k-1) * .055
			m.curve([3]float64{-length - .15, y, 0}, [3]float64{-length - .3, y + .19, .05}, [3]float64{-length - .57, y + .23, .06}, .02, col, meshGrip)
		}
	}
	if id == 31 {
		for i := 0; i < 5; i++ {
			x := -2.0 - float64(i)*.47
			m.rod([3]float64{x - .035, 0, 0}, [3]float64{x + .035, 0, 0}, .084, .084, meshGold, meshMetal)
		}
	}
}

func (m *weaponMeshBuilder) heavy(id int) {
	head, grip := meshSteel, meshWood
	switch id {
	case 40:
		head = meshGold
		grip = [3]int{27, 59, 150}
	case 42:
		head = meshBone
	case 43:
		head = [3]int{82, 62, 40}
	case 44:
		head = meshDark
		grip = meshRed
	case 45:
		head = [3]int{55, 44, 38}
		grip = meshRed
	case 46:
		head = meshJade
	case 47:
		head = meshGold
	case 48:
		head = meshSteel
		grip = meshRed
	case 49:
		head = meshDark
		grip = meshRed
	}
	m.grip(-3.05, -1.10, 0, .10, grip, meshGold)
	m.rod([3]float64{-1.1, 0, 0}, [3]float64{-.3, 0, 0}, .115, .14, meshSteel, meshMetal)
	center := [3]float64{-.45, 0, 0}
	if id >= 46 {
		half := [3]float64{.42, .92, .34}
		if id == 47 {
			half = [3]float64{.53, .88, .39}
		}
		if id == 49 {
			half = [3]float64{.43, 1.08, .37}
		}
		material := meshMetal
		if id == 46 {
			material = meshStone
		}
		if id == 49 {
			m.box(center, [3]float64{.29, .79, .27}, .10, head, material)
			for i := 0; i < 5; i++ {
				y := float64(i-2) * .37
				width := .42 - .055*math.Abs(float64(i-2))
				m.box([3]float64{-.44, y, 0}, [3]float64{width, .19, .34}, .08, head, material)
			}
		} else {
			m.box(center, half, .14, head, material)
		}
		for _, sg := range []float64{-1, 1} {
			m.box([3]float64{center[0], sg * (half[1] - .1), 0}, [3]float64{half[0] + .025, .075, half[2] + .025}, .045, meshGold, meshMetal)
		}
		for _, sg := range []float64{-1, 1} {
			z := sg * (half[2] + .03)
			if id == 46 {
				m.box([3]float64{-.45, 0, z}, [3]float64{.35, .35, .045}, .025, meshGold, meshMetal)
				m.gem([3]float64{-.45, 0, z + sg*.065}, [3]float64{.19, .2, .08}, [3]int{43, 92, 70})
			} else if id == 47 { // Stepped mask and cheek plates of the idol hammer.
				m.box([3]float64{-.43, 0, z}, [3]float64{.38, .41, .07}, .03, meshGold, meshMetal)
				for _, y := range []float64{-.22, .22} {
					m.box([3]float64{-.32, y, z + sg*.08}, [3]float64{.09, .075, .018}, .006, meshDark, meshGrip)
				}
				m.box([3]float64{-.55, 0, z + sg*.085}, [3]float64{.07, .24, .018}, .006, meshDark, meshGrip)
			} else if id == 48 { // Lion medallion: faceted mane, brow and projecting muzzle.
				m.gem([3]float64{-.45, 0, z}, [3]float64{.27, .33, .10}, meshGold)
				m.gem([3]float64{-.49, 0, z + sg*.13}, [3]float64{.13, .14, .10}, meshGold)
				for _, y := range []float64{-.13, .13} {
					m.gem([3]float64{-.34, y, z + sg*.10}, [3]float64{.045, .04, .02}, meshDark)
				}
			} else {
				for i := 0; i < 4; i++ {
					for j := 0; j < 3; j++ {
						m.gem([3]float64{-.68 + float64(j)*.20, -.66 + float64(i)*.43, z}, [3]float64{.105, .21, .04}, meshRed)
					}
				}
			}
		}
		if id == 49 {
			for _, sg := range []float64{-1, 1} {
				for i := 0; i < 3; i++ {
					y := sg * (.45 + float64(i)*.2)
					m.rod([3]float64{-.12, y, 0}, [3]float64{.24, y + sg*.22, 0}, .12, 0, meshGold, meshMetal)
				}
			}
		}
		return
	}
	if id == 43 {
		m.rod([3]float64{-2.1, 0, 0}, [3]float64{-.12, 0, 0}, .13, .26, head, meshGrip)
		for i := 0; i < 6; i++ {
			x := -.25 - float64(i)*.28
			for k := 0; k < 6; k++ {
				a := float64(k) * math.Pi / 3
				r := .26 - float64(i)*.016
				m.gem([3]float64{x, r * math.Cos(a), r * math.Sin(a)}, [3]float64{.06, .075, .075}, meshSteel)
			}
		}
		return
	}
	radius := .48
	if id == 45 {
		radius = .57
	}
	// Rings of facets form the ball/egg; spikes are independent solid cones.
	for i := 0; i < 6; i++ {
		a0, a1 := -math.Pi/2+float64(i)*math.Pi/6, -math.Pi/2+float64(i+1)*math.Pi/6
		for j := 0; j < 10; j++ {
			b0, b1 := float64(j)*math.Pi/5, float64(j+1)*math.Pi/5
			p := func(a, b float64) [3]float64 {
				return [3]float64{center[0] + radius*math.Sin(a), radius * math.Cos(a) * math.Cos(b), radius * math.Cos(a) * math.Sin(b)}
			}
			m.triangle(p(a0, b0), p(a1, b0), p(a0, b1), center, head, meshMetal)
			m.triangle(p(a0, b1), p(a1, b0), p(a1, b1), center, head, meshMetal)
		}
	}
	for k := 0; k < 10; k++ {
		a := float64(k) * math.Pi / 5
		col := meshGold
		if id == 41 {
			col = meshSilver
		}
		if id == 42 {
			col = meshBone
		}
		m.rod([3]float64{center[0], radius * .87 * math.Cos(a), radius * .87 * math.Sin(a)}, [3]float64{center[0] + .05, (radius + .26) * math.Cos(a), (radius + .26) * math.Sin(a)}, .13, 0, col, meshMetal)
	}
	if id == 40 {
		for _, sg := range []float64{-1, 1} {
			z := sg * .49
			m.gem([3]float64{-.45, 0, z}, [3]float64{.32, .32, .035}, [3]int{79, 149, 231})
			m.rod([3]float64{-.7, -.22, z + sg*.035}, [3]float64{-.2, .22, z + sg*.035}, .035, .035, meshGold, meshMetal)
			m.rod([3]float64{-.7, .22, z + sg*.035}, [3]float64{-.2, -.22, z + sg*.035}, .035, .035, meshGold, meshMetal)
		}
	}
	if id == 42 {
		for _, sg := range []float64{-1, 1} {
			for _, y := range []float64{-.18, .18} {
				m.gem([3]float64{-.32, y, sg * .44}, [3]float64{.105, .085, .027}, meshDark)
				m.box([3]float64{-.67, y, sg * .41}, [3]float64{.13, .085, .022}, .015, meshDark, meshGrip)
				for j := 0; j < 2; j++ {
					m.box([3]float64{-.64, y + float64(j)*.075 - .035, sg * .45}, [3]float64{.07, .027, .03}, .013, meshBone, meshMetal)
				}
			}
		}
	}
	if id == 45 {
		for _, sg := range []float64{-1, 1} {
			for i := 0; i < 5; i++ {
				a := float64(i) * 1.25
				m.gem([3]float64{-.45 + math.Cos(a)*.25, math.Sin(a) * .32, sg * .48}, [3]float64{.15, .075, .055}, [3]int{250, 73, 12})
			}
		}
	}
}

func (m *weaponMeshBuilder) axe(id int) {
	m.grip(-3.5, -1.45, 0, .093, meshRed, meshGold)
	m.rod([3]float64{-1.45, 0, 0}, [3]float64{.0, 0, 0}, .115, .13, meshWood, meshGrip)
	m.box([3]float64{-.62, 0, 0}, [3]float64{.43, .17, .18}, .04, meshGold, meshMetal)
	sides := []float64{1}
	if id != 60 {
		sides = []float64{-1, 1}
	}
	for _, sg := range sides {
		// Each section runs from the socket into the flared crescent; blade width
		// varies along the haft and the sharpened outer bevel has real thickness.
		col := meshSteel
		if id == 61 {
			col = meshGold
		}
		if id >= 63 {
			col = [3]int{86, 35, 30}
		}
		for i := 0; i < 10; i++ {
			a, b := float64(i)/10, float64(i+1)/10
			point := func(t, edge, z float64) [3]float64 {
				x := -1.35 + 1.5*t
				y := .18 + (.55+.40*math.Sin(math.Pi*t))*edge
				if id == 60 {
					y *= .87
				}
				return [3]float64{x, sg * y, z}
			}
			mid := [3]float64{(-1.35 + 1.5*(a+b)/2), sg * .45, 0}
			for _, face := range []float64{-1, 1} {
				v0, v1 := point(a, 0, face*.13), point(b, 0, face*.13)
				v2, v3 := point(a, .78, face*.10), point(b, .78, face*.10)
				v4, v5 := point(a, 1, 0), point(b, 1, 0)
				m.triangle(v0, v1, v2, mid, col, meshMetal)
				m.triangle(v2, v1, v3, mid, col, meshMetal)
				edge := meshSilver
				if id == 61 {
					edge = [3]int{240, 192, 99}
				}
				if id >= 63 {
					edge = meshBone
				}
				m.triangle(v2, v3, v4, mid, edge, meshMetal)
				m.triangle(v4, v3, v5, mid, edge, meshMetal)
				if id >= 63 {
					m.triangle(point(a, .12, face*.135), point(b, .12, face*.135), point((a+b)/2, .71, face*.115), mid, meshRed, meshMetal)
				}
			}
		}
		if id >= 63 {
			for _, z := range []float64{-.17, .17} {
				for i := 0; i < 5; i++ {
					for j := 0; j < 2; j++ {
						m.gem([3]float64{-1.16 + float64(i)*.25, sg * (.29 + float64(j)*.22), z}, [3]float64{.095, .10, .025}, meshRed)
					}
				}
			}
			for i := 0; i < 3; i++ {
				x := -1.15 + float64(i)*.5
				m.curve([3]float64{x, sg * .35, 0}, [3]float64{x + .2, sg * 1.0, 0}, [3]float64{x + .55, sg * .94, 0}, .065, meshGold, meshMetal)
			}
		}
		if id == 61 {
			for _, z := range []float64{-.135, .135} {
				m.curve([3]float64{-.95, sg * .3, z}, [3]float64{-.48, sg * .95, z}, [3]float64{-.04, sg * .35, z}, .022, meshBone, meshMetal)
			}
		}
	}
	if id == 63 {
		m.rod([3]float64{-.4, -.2, 0}, [3]float64{.45, -.67, 0}, .16, 0, meshBone, meshMetal)
	}
}

func (m *weaponMeshBuilder) claws() {
	m.grip(-2.45, -1.5, 0, .10, [3]int{100, 47, 133}, meshSilver)
	m.ring([3]float64{-2.66, 0, 0}, .13, .035, meshSilver)
	m.box([3]float64{-1.4, 0, 0}, [3]float64{.13, .39, .13}, .05, meshGold, meshMetal)
	for i := 0; i < 3; i++ {
		offset := float64(i-1) * .27
		s := []bladeSection{}
		for k := 0; k <= 8; k++ {
			t := float64(k) / 8
			s = append(s, bladeSection{-1.4 + 1.4*t, offset*(1-t*.22) + .28*(t*t-1), .066 * (1 - t), .032 * (1 - t)})
		}
		m.blade(s, meshSilver, meshMetal)
	}
}
func (m *weaponMeshBuilder) fan() {
	// The pivot lies behind the scalloped leading rim, registered at the tip.
	pivot := [3]float64{-1.7, 0, 0}
	const n = 12
	for i := 0; i < n; i++ {
		a, b := -1.02+float64(i)*2.04/n, -1.02+float64(i+1)*2.04/n
		p := func(t, r, z float64) [3]float64 { return [3]float64{pivot[0] + r*math.Cos(t), r * math.Sin(t), z} }
		a0, b0 := p(a, 1.7, 0), p(b, 1.7, 0)
		mid := p((a+b)/2, 1.75, -.075)
		for _, sg := range []float64{-1, 1} {
			o := [3]float64{-1, 0, sg * .2}
			ridge := mid
			ridge[2] *= sg
			inner := [3]float64{pivot[0] + (ridge[0]-pivot[0])*.80, ridge[1] * .80, ridge[2] * .80}
			ai, bi := p(a, 1.36, 0), p(b, 1.36, 0)
			m.triangle(pivot, ai, inner, o, [3]int{74, 85, 100}, meshGrip)
			m.triangle(pivot, inner, bi, o, [3]int{74, 85, 100}, meshGrip)
			m.triangle(ai, a0, ridge, o, meshRed, meshGrip)
			m.triangle(ai, ridge, inner, o, meshRed, meshGrip)
			m.triangle(bi, inner, ridge, o, meshRed, meshGrip)
			m.triangle(bi, ridge, b0, o, meshRed, meshGrip)
		}
		m.rod(pivot, a0, .018, .012, meshSilver, meshMetal)
	}
	m.gem(pivot, [3]float64{.09, .09, .07}, meshGold)
	for _, sg := range []float64{-1, 1} {
		prev := [3]float64{-.60, 0, sg * .075}
		for k := 1; k <= 24; k++ {
			t := float64(k) / 24
			a := t * math.Pi * 4
			radius := t * .26
			q := [3]float64{-.60 + math.Cos(a)*radius, math.Sin(a) * radius, sg * .075}
			m.rod(prev, q, .012, .012, [3]int{132, 218, 245}, meshGlow)
			prev = q
		}
	}
}

func (m *weaponMeshBuilder) gear(c [3]float64, radius, thickness float64) {
	// Annulus with spokes and separate rectangular teeth: gaps remain open.
	for i := 0; i < 16; i++ {
		a, b := float64(i)*math.Pi/8, float64(i+1)*math.Pi/8
		for _, sg := range []float64{-1, 1} {
			p := func(t, r float64) [3]float64 {
				return [3]float64{c[0] + math.Cos(t)*r, c[1] + math.Sin(t)*r, c[2] + sg*thickness}
			}
			o := c
			o[2] -= sg * .1
			m.triangle(p(a, radius*.65), p(b, radius*.65), p(a, radius), o, meshGold, meshMetal)
			m.triangle(p(a, radius), p(b, radius*.65), p(b, radius), o, meshGold, meshMetal)
		}
		x, y := math.Cos(a), math.Sin(a)
		m.rod([3]float64{c[0] + x*radius*.96, c[1] + y*radius*.96, c[2]}, [3]float64{c[0] + x*radius*1.15, c[1] + y*radius*1.15, c[2]}, radius*.09, radius*.09, meshGold, meshMetal)
		if i%4 == 0 {
			m.rod(c, [3]float64{c[0] + x*radius*.75, c[1] + y*radius*.75, c[2]}, .035, .035, meshGold, meshMetal)
		}
	}
	m.rod([3]float64{c[0], c[1], c[2] - thickness}, [3]float64{c[0], c[1], c[2] + thickness}, radius*.22, radius*.22, meshGold, meshMetal)
}
func (m *weaponMeshBuilder) clock(id int) {
	blue := [3]int{75, 152, 211}
	if id == 70 || id == 74 {
		length, width := 3.4, .14
		if id == 74 {
			length = 3.7
			width = .046
			blue = meshSilver
		}
		m.blade([]bladeSection{{-length, 0, width, .055}, {-.45, 0, width * .65, .038}, {0, 0, 0, 0}}, blue, meshMetal)
		m.grip(-length-.85, -length-.1, 0, .08, meshWood, meshGold)
		if id == 70 {
			m.gear([3]float64{-length + .03, 0, 0}, .34, .075)
		} else {
			for _, sg := range []float64{-1, 1} {
				m.curve([3]float64{-length + .03, 0, 0}, [3]float64{-length - .65, sg * .8, 0}, [3]float64{-length - .58, sg * .13, 0}, .028, meshGold, meshMetal)
				m.curve([3]float64{-length, 0, 0}, [3]float64{-length + .35, sg * .40, 0}, [3]float64{-length - .22, sg * .31, 0}, .025, meshGold, meshMetal)
			}
		}
	} else if id == 73 {
		m.rod([3]float64{-4.9, 0, 0}, [3]float64{-1.1, 0, 0}, .065, .065, meshDark, meshMetal)
		for i := 0; i < 10; i++ {
			x := -1.4 - float64(i)*.36
			m.rod([3]float64{x - .055, 0, 0}, [3]float64{x + .055, 0, 0}, .093, .093, meshGold, meshMetal)
		}
		m.blade([]bladeSection{{-1.25, 0, .08, .045}, {-.8, 0, .19, .06}, {0, 0, 0, 0}}, blue, meshMetal)
		for _, sg := range []float64{-1, 1} {
			m.rod([3]float64{-1.4, 0, 0}, [3]float64{-.85, sg * .3, 0}, .07, 0, meshGold, meshMetal)
		}
	} else {
		m.grip(-3.0, -.8, 0, .09, meshWood, meshGold)
		if id == 72 {
			m.gear([3]float64{-.55, 0, 0}, .56, .12)
		} else {
			m.box([3]float64{-.5, 0, 0}, [3]float64{.33, .67, .28}, .09, meshGold, meshMetal)
			for _, sg := range []float64{-1, 1} {
				m.rod([3]float64{-.5, sg * .55, 0}, [3]float64{-.5, sg * .98, 0}, .31, 0, meshGold, meshMetal)
			}
			for _, sg := range []float64{-1, 1} {
				z := sg * .31
				m.rod([3]float64{-.5, 0, z}, [3]float64{-.5, 0, z + sg*.04}, .44, .44, meshBone, meshGrip)
				m.ring([3]float64{-.5, 0, z + sg*.06}, .44, .035, meshGold)
				for k := 0; k < 12; k++ {
					a := float64(k) * math.Pi / 6
					m.rod([3]float64{-.5 + math.Cos(a)*.32, math.Sin(a) * .32, z + sg*.05}, [3]float64{-.5 + math.Cos(a)*.38, math.Sin(a) * .38, z + sg*.05}, .013, .013, meshDark, meshGrip)
				}
				m.rod([3]float64{-.5, 0, z + sg*.055}, [3]float64{-.56, -.29, z + sg*.055}, .015, .011, meshDark, meshGrip)
				m.rod([3]float64{-.5, 0, z + sg*.055}, [3]float64{-.29, .1, z + sg*.055}, .02, .01, meshDark, meshGrip)
			}
			m.rod([3]float64{-1.08, .15, 0}, [3]float64{-1.1, .72, 0}, .018, .018, meshGold, meshMetal)
			m.gem([3]float64{-1.1, .83, 0}, [3]float64{.13, .15, .07}, meshGold)
		}
	}
}
