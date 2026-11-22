package game

import "math"

// Small construction primitives shared by the ranged assets. Silhouettes and
// fittings remain authored per item; these only build their continuous surfaces.
// An extruded plate with a bevel and explicit front/back for small fittings.
// The bevel is a share of each corner's distance from the mean, which suits a
// compact star-shaped polygon; a long stock needs rangedStockPlate's physical
// bevel instead. Slots are made from separate plates.
func rangedPlate(m *weaponMeshBuilder, outline [][2]float64, z, depth, bevel float64, col [3]int, material int) {
	var c [3]float64
	for _, p := range outline {
		c[0] += p[0]
		c[1] += p[1]
	}
	c[0] /= float64(len(outline))
	c[1] /= float64(len(outline))
	c[2] = z
	for _, side := range []float64{-1, 1} {
		center := c
		center[2] += side * depth
		for i, p := range outline {
			q := outline[(i+1)%len(outline)]
			a, b := [3]float64{p[0], p[1], z}, [3]float64{q[0], q[1], z}
			u, v := [3]float64{c[0] + (p[0]-c[0])*(1-bevel), c[1] + (p[1]-c[1])*(1-bevel), center[2]}, [3]float64{c[0] + (q[0]-c[0])*(1-bevel), c[1] + (q[1]-c[1])*(1-bevel), center[2]}
			m.triangle(a, b, u, c, col, material)
			m.triangle(u, b, v, c, col, material)
			m.triangle(u, v, center, c, col, material)
		}
	}
}

func rangedHelix(m *weaponMeshBuilder, from, to, radius, wire, turns float64, col [3]int, material int) {
	steps := max(12, int(turns*12))
	s := make([]rangedSection, steps+1)
	for i := range s {
		t := float64(i) / float64(steps)
		a := t * turns * 2 * math.Pi
		s[i] = rangedSection{[3]float64{from + (to-from)*t, radius * math.Cos(a), radius * math.Sin(a)}, wire, wire}
	}
	rangedTube(m, s, col, material)
}

// The one sphere builder: an ellipsoid about c whose radius relief(lat, lon)
// may vary (nil keeps it round). A smooth spheroid has continuous normals and
// object-local UVs, so shader currents move across it instead of a faceted
// solid; a faceted one shows its planes.
func rangedSpheroid(m *weaponMeshBuilder, c, radii [3]float64, rings, sides int, relief func(lat, lon float64) float64, smooth bool, col [3]int, material int) {
	point := func(i, j int) [3]float64 {
		lat, lon := math.Pi*float64(i)/float64(rings), 2*math.Pi*float64(j)/float64(sides)
		r := 1.0
		if relief != nil {
			r = relief(lat, lon)
		}
		return [3]float64{c[0] + radii[0]*math.Cos(lat)*r, c[1] + radii[1]*math.Sin(lat)*math.Cos(lon)*r, c[2] + radii[2]*math.Sin(lat)*math.Sin(lon)*r}
	}
	face := func(a, b, d [3]float64) {
		m.triangle(a, b, d, c, col, material)
		if !smooth {
			return
		}
		s := &modelSurface{}
		for k, p := range [3][3]float64{a, b, d} {
			v := [3]float64{(p[0] - c[0]) / (radii[0] * radii[0]), (p[1] - c[1]) / (radii[1] * radii[1]), (p[2] - c[2]) / (radii[2] * radii[2])}
			l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
			for j := range v {
				v[j] /= l
			}
			s.normals[k] = v
			s.uv[k] = [2]float64{(p[0] - c[0]) / radii[0], (p[1] - c[1]) / radii[1]}
		}
		m.faces[len(m.faces)-1].surface = s
	}
	for i := 0; i < rings; i++ {
		for j := 0; j < sides; j++ {
			a, b, d, e := point(i, j), point(i, j+1), point(i+1, j), point(i+1, j+1)
			face(a, b, d)
			face(d, b, e)
		}
	}
}

// Small faceted fittings: rivets, pommels and lens bosses.
func rangedOrb(m *weaponMeshBuilder, c, radii [3]float64, col [3]int, material int) {
	rangedSpheroid(m, c, radii, 8, 12, nil, false, col, material)
}

// A smooth reservoir for flowing shader materials.
func rangedFluidOrb(m *weaponMeshBuilder, c, radii [3]float64, col [3]int, material int) {
	rangedSpheroid(m, c, radii, 12, 20, nil, true, col, material)
}

// Ribbons have two hems and a folded center, not a row of round beads.
func rangedCloth(m *weaponMeshBuilder, points [][3]float64, widths []float64, col [3]int) {
	start := len(m.faces)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		for _, s := range []float64{-1, 1} {
			p, q := a, b
			p[1] += s * widths[i-1]
			q[1] += s * widths[i]
			p[2] += .015
			q[2] += .015
			o := [3]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2, (a[2]+b[2])/2 + .04}
			m.triangle(a, p, b, o, mixColor(col, meshRed, .2), meshGrip)
			m.triangle(b, p, q, o, col, meshGrip)
		}
	}
	end := len(m.faces)
	for i := start; i < end; i++ {
		f := m.faces[i]
		// The same fabric is visible from either side, with one culled face each.
		center := [3]float64{(f.a[0] + f.b[0] + f.c[0]) / 3, (f.a[1] + f.b[1] + f.c[1]) / 3, (f.a[2] + f.b[2] + f.c[2]) / 3}
		for axis := range f.origin {
			f.origin[axis] = 2*center[axis] - f.origin[axis]
		}
		m.faces = append(m.faces, f)
	}

}

// Gun/crossbow side profiles are drawn in the Y/Z plane with thickness in X.
func rangedSidePlate(m *weaponMeshBuilder, outline [][2]float64, x, depth, bevel float64, col [3]int, material int) {
	part := weaponMeshBuilder{}
	rangedStockPlate(&part, outline, x, depth, bevel, col, material)
	m.appendMapped(part.faces, func(p [3]float64) [3]float64 { return [3]float64{p[2], p[0], p[1]} })
}
