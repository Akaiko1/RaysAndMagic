package game

import "math"

// Face orientation follows the local surface, including concave shells. A
// single object center cannot orient an inner barrel wall or a recessed lip.
func rangedSurfaceTriangle(m *weaponMeshBuilder, a, b, c, outward [3]float64, col [3]int, material int) {
	n := triangleNormal(a, b, c)
	length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
	if length < 1e-12 {
		return
	}
	if n[0]*outward[0]+n[1]*outward[1]+n[2]*outward[2] < 0 {
		length = -length
	}
	var origin [3]float64
	for i := range origin {
		origin[i] = (a[i]+b[i]+c[i])/3 - n[i]/length
	}
	m.triangle(a, b, c, origin, col, material)
}

// The revolve for held equipment (the Retort's smooth glass, with its own
// shading normals, has retortLathe): an (axial, radius) profile turned about
// local X. A profile
// can return along its bore, so faces follow the profile normal for radial lips
// and inward walls rather than pointing away from the axis. An open profile is
// a solid with end caps; a closed one (first point repeated) is a hollow tube.
func rangedLathe(m *weaponMeshBuilder, profile [][2]float64, col [3]int, material int) {
	const sides = 12
	capped := profile[0] != profile[len(profile)-1]
	area := 0.0
	for i := 1; i < len(profile); i++ {
		area += profile[i-1][0]*profile[i][1] - profile[i][0]*profile[i-1][1]
	}
	if capped {
		// Close the section along the axis before choosing its winding. The
		// open profile's signed area alone depends on its axial translation.
		first, last := profile[0], profile[len(profile)-1]
		area += first[0]*first[1] - last[0]*last[1]
	} else {
		a, b := profile[len(profile)-1], profile[0]
		area += a[0]*b[1] - b[0]*a[1]
	}
	direction := 1.0
	if area > 0 {
		direction = -1
	}
	point := func(v [2]float64, j int) [3]float64 {
		a := float64(j) * 2 * math.Pi / sides
		return [3]float64{v[0], v[1] * math.Cos(a), v[1] * math.Sin(a)}
	}
	for i := 1; i < len(profile); i++ {
		a, b := profile[i-1], profile[i]
		for j := 0; j < sides; j++ {
			angle := (float64(j) + .5) * 2 * math.Pi / sides
			n := [3]float64{-(b[1] - a[1]) * direction, (b[0] - a[0]) * math.Cos(angle) * direction, (b[0] - a[0]) * math.Sin(angle) * direction}
			p, q, r, s := point(a, j), point(a, j+1), point(b, j), point(b, j+1)
			rangedSurfaceTriangle(m, p, q, r, n, col, material)
			rangedSurfaceTriangle(m, r, q, s, n, col, material)
		}
	}
	if capped {
		for _, i := range []int{0, len(profile) - 1} {
			n := [3]float64{direction, 0, 0}
			if i == 0 {
				n[0] = -n[0]
			}
			for j := 0; j < sides; j++ {
				rangedSurfaceTriangle(m, [3]float64{profile[i][0], 0, 0}, point(profile[i], j), point(profile[i], j+1), n, col, material)
			}
		}
	}
}

// Stock bevels use a physical edge width, not a percentage of the entire
// polygon. Shrinking a long stock about its center pulls its butt away from
// the metal heel and leaves the fittings hanging beyond the apparent cut.
func rangedStockPlate(m *weaponMeshBuilder, outline [][2]float64, z, depth, bevel float64, col [3]int, material int) {
	area := 0.0
	for i, p := range outline {
		q := outline[(i+1)%len(outline)]
		area += p[0]*q[1] - q[0]*p[1]
	}
	winding := 1.0
	if area < 0 {
		winding = -1
	}
	width := depth * bevel
	inner := make([][2]float64, len(outline))
	for i, p := range outline {
		prev, next := outline[(i+len(outline)-1)%len(outline)], outline[(i+1)%len(outline)]
		a, b := [2]float64{p[0] - prev[0], p[1] - prev[1]}, [2]float64{next[0] - p[0], next[1] - p[1]}
		la, lb := math.Hypot(a[0], a[1]), math.Hypot(b[0], b[1])
		na, nb := [2]float64{-a[1] / la * winding, a[0] / la * winding}, [2]float64{-b[1] / lb * winding, b[0] / lb * winding}
		denom := max(.25, 1+na[0]*nb[0]+na[1]*nb[1])
		inner[i] = [2]float64{p[0] + width*(na[0]+nb[0])/denom, p[1] + width*(na[1]+nb[1])/denom}
	}
	cap := rangedPolygonTriangles(inner, winding)
	for _, side := range []float64{-1, 1} {
		for i, p := range outline {
			j := (i + 1) % len(outline)
			q := outline[j]
			a, b := [3]float64{p[0], p[1], z}, [3]float64{q[0], q[1], z}
			u, v := [3]float64{inner[i][0], inner[i][1], z + side*depth}, [3]float64{inner[j][0], inner[j][1], z + side*depth}
			n := [3]float64{(q[1] - p[1]) * winding, -(q[0] - p[0]) * winding, side * width}
			rangedSurfaceTriangle(m, a, b, u, n, col, material)
			rangedSurfaceTriangle(m, u, b, v, n, col, material)
		}
		for _, t := range cap {
			var p [3][3]float64
			for k, index := range t {
				p[k] = [3]float64{inner[index][0], inner[index][1], z + side*depth}
			}
			rangedSurfaceTriangle(m, p[0], p[1], p[2], [3]float64{0, 0, side}, col, material)
		}
	}
}

// Ear clipping closes concave stock silhouettes without a center fan crossing
// a trigger recess. This runs only while a cached mesh is constructed.
func rangedPolygonTriangles(p [][2]float64, winding float64) [][3]int {
	indices := make([]int, len(p))
	for i := range indices {
		indices[i] = i
	}
	cross := func(a, b, c [2]float64) float64 { return ((b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])) * winding }
	var out [][3]int
	for len(indices) > 3 {
		found := false
		for i, b := range indices {
			a, c := indices[(i+len(indices)-1)%len(indices)], indices[(i+1)%len(indices)]
			if cross(p[a], p[b], p[c]) <= 1e-12 {
				continue
			}
			empty := true
			for _, q := range indices {
				if q != a && q != b && q != c && cross(p[a], p[b], p[q]) >= 0 && cross(p[b], p[c], p[q]) >= 0 && cross(p[c], p[a], p[q]) >= 0 {
					empty = false
					break
				}
			}
			if empty {
				out = append(out, [3]int{a, b, c})
				indices = append(indices[:i], indices[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			panic("ranged stock outline must be a simple polygon")
		}
	}
	return append(out, [3]int{indices[0], indices[1], indices[2]})
}
