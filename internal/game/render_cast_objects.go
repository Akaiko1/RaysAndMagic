package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/shaders"
)

// Camera-local spell objects reuse the weapon mesh renderer: real curved faces,
// stable perspective and self-occlusion, not a flat icon enlarged over the view.
const (
	castNoObject = iota
	castCuirass
	castHourglass
	castWing
	castWingTips
	castWingBack // the embracing wing behind the candle: its tips recede
	castWingTipsBack
)

// Material ids select the shading branch; cast_object.kage declares the same ids.
const (
	castMatPlain  = iota
	castMatForged // engraved silver revealed by the forge front
	castMatGlass
	castMatSand
	castMatStream
	castMatFeather
	castMatJewel
	castMatPolished // forged reflections without the reveal
	castMatCloth    // matte lining under the plates
	castMatPlume    // white feather with cool tips (Preservation keeps castMatFeather)
	castMatGilt     // warm gold trim of the celestial cuirass
)

var castObjectMeshes volumeMeshCache

type castObjectRenderer struct {
	shader  *ebiten.Shader
	options ebiten.DrawTrianglesShaderOptions
	phase   [1]float32
}

// drawCastObject paints a camera-local object. inside, when set, is painted
// within the object's depth: Preservation's ghost light between its wings.
func (r *Renderer) drawCastObject(dst *ebiten.Image, a buffFxAnim, model int, inside func()) {
	o := &r.castOverlay.object
	if o.shader == nil {
		var err error
		o.shader, err = ebiten.NewShader([]byte(shaders.Source("cast_object.kage")))
		if err != nil {
			panic(err)
		}
		o.options.Uniforms = map[string]any{"Phase": o.phase[:]}
	}
	t, alpha := a.objectPhase()
	o.phase[0] = float32(t)
	yaw, roll := -.28+math.Sin(t*math.Pi)*.36, 0.0
	if model == castHourglass {
		roll = math.Pi * (1 - smoothStep(t/.35))
		yaw = .23
	}
	ox, oy, w, h := r.castOverlayViewport(a)
	if model == castWing {
		// Shoulder and wrist close in sequence. Feather lengths stay fixed.
		closed := smoothStep((t - .06) / .48)
		angle := -.78 * (1 - closed)
		cr, sr := math.Cos(angle), math.Sin(angle)
		cy, sy := math.Cos(.50*(1-closed)), math.Sin(.50*(1-closed))
		// Both wings hang from shoulders at the top and wrap down around the
		// candle, as on the icon; their primaries cross below the flame. The
		// embrace spirals in depth: the right wing sits a little lower and in
		// front (tips toward the viewer), the left one behind (tips receding;
		// feathers are one-sided, so it keeps facing the viewer). Separate
		// draws share no depth buffer: back wing, ghost light, front wing.
		scale := h * .24
		wx, wy := preservationArm(preservationWristU)
		drawWing := func(side float64) {
			pose := volumeTransform{rotation: [3][3]float64{{side * cy * cr, -side * cy * sr, side * sy}, {sr, cr, 0}, {-sy * cr, sy * sr, cy}}, uvY: 1}
			arm, hand, drop := castWingBack, castWingTipsBack, 0.0
			if side > 0 {
				arm, hand, drop = castWing, castWingTips, .16
			}
			// Each shoulder sits so the closed arcs share the almond's centre.
			sx, sy0 := ox+w*.5-side*.02*scale, oy+h*.55+(drop-.80)*scale
			r.drawCameraVolume(dst, sx, sy0, scale, alpha, pose, o.shader, &o.options, castObjectMeshes.get(arm, buildCastObject), [3]int{255, 255, 255}, 0, true)
			wrist := -.55 * (1 - smoothStep((t-.16)/.44))
			cw, sw := math.Cos(wrist), math.Sin(wrist)
			fold := pose
			for axis := 0; axis < 3; axis++ {
				fold.rotation[axis][0] = pose.rotation[axis][0]*cw + pose.rotation[axis][1]*sw
				fold.rotation[axis][1] = -pose.rotation[axis][0]*sw + pose.rotation[axis][1]*cw
			}
			// Both cached pieces share the wrist; transform that pivot by the
			// same shoulder pose instead of independently centering each piece.
			ex := pose.rotation[0][0]*wx + pose.rotation[0][1]*wy
			ey := pose.rotation[1][0]*wx + pose.rotation[1][1]*wy
			r.drawCameraVolume(dst, sx+ex*scale, sy0+ey*scale, scale, alpha, fold, o.shader, &o.options, castObjectMeshes.get(hand, buildCastObject), [3]int{255, 255, 255}, 0, true)
		}
		drawWing(-1)
		if inside != nil {
			inside()
		}
		drawWing(1)
		return
	}
	cy, sy, cr, sr := math.Cos(yaw), math.Sin(yaw), math.Cos(roll), math.Sin(roll)
	pose := volumeTransform{rotation: [3][3]float64{{cy * cr, -cy * sr, sy}, {sr, cr, 0}, {-sy * cr, sy * sr, cy}}, uvY: 1}
	r.drawCameraVolume(dst, ox+w*.5, oy+h*.43, h*.25, alpha, pose, o.shader, &o.options, castObjectMeshes.get(model, buildCastObject), [3]int{255, 255, 255}, 0, true)
}

// Per-corner attributes are baked with the geometry, not reconstructed from
// screen coordinates. Faceted models continue using the ordinary triangle path.
func castSurfaceTriangle(m *weaponMeshBuilder, points, normals [3][3]float64, uv [3][2]float64, origin [3]float64, tint [3]int, material int) {
	for i, n := range normals {
		length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		for axis := range n {
			normals[i][axis] /= length
		}
	}
	m.faces = append(m.faces, modelFace{a: points[0], b: points[1], c: points[2], origin: origin, rgb: tint, material: material, surface: &modelSurface{normals: normals, uv: uv}})
}

// A hammered plate has an authored outline, a narrow bevel and a domed face.
// Unlike a lathed profile it can preserve neck, arm and feather-tip cutouts.
func castPlate(m *weaponMeshBuilder, outline [][2]float64, center [3]float64, bulge float64, tint [3]int, material int) {
	rx, ry := .001, .001
	for _, p := range outline {
		rx, ry = math.Max(rx, math.Abs(p[0]-center[0])), math.Max(ry, math.Abs(p[1]-center[1]))
	}
	triangle := func(a, b, c [3]float64) {
		points := [3][3]float64{a, b, c}
		var normals [3][3]float64
		var uv [3][2]float64
		for i, p := range points {
			normals[i] = [3]float64{2 * bulge * (p[0] - center[0]) / (rx * rx), 2 * bulge * (p[1] - center[1]) / (ry * ry), -1}
			uv[i] = [2]float64{p[0], p[1]}
		}
		castSurfaceTriangle(m, points, normals, uv, [3]float64{center[0], center[1], center[2] + .2}, tint, material)
	}
	point := func(p [2]float64, scale float64, z float64) [3]float64 {
		return [3]float64{center[0] + (p[0]-center[0])*scale, center[1] + (p[1]-center[1])*scale, z}
	}
	scales := []float64{1, .98, .91, .82, .72, .61, .49, .36, .23, .11, 0}
	if material == castMatFeather {
		scales = []float64{1, .7, .3, 0}
	}
	for ring := 1; ring < len(scales); ring++ {
		outer, inner := scales[ring-1], scales[ring]
		z0, z1 := center[2]-bulge*(1-outer*outer), center[2]-bulge*(1-inner*inner)
		for i, p := range outline {
			q := outline[(i+1)%len(outline)]
			a, b, c, d := point(p, outer, z0), point(q, outer, z0), point(p, inner, z1), point(q, inner, z1)
			triangle(a, b, c)
			if inner > 0 {
				triangle(c, b, d)
			}
		}
	}
}

// castFeather builds one feather: a slender quill, a narrow leading vane and a
// broad trailing vane. tipRound 1 gives a covert's rounded tip, 0 a flight
// feather's pointed one.
func castFeather(m *weaponMeshBuilder, root, tip [2]float64, width, z, tipRound float64, tint [3]int, material int) {
	dx, dy := tip[0]-root[0], tip[1]-root[1]
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l, dx/l
	vertex := func(u, side float64) ([3]float64, [3]float64) {
		vane := smoothStep(u/.14) * (1 - .18*u)
		if u > .68 {
			k := (u - .68) / .32
			vane *= tipRound*math.Sqrt(math.Max(0, 1-k*k)) + (1-tipRound)*(1-k)
		}
		w := width * vane
		if side < 0 {
			w *= .58
		}
		bend := width * math.Sin(u*math.Pi*.92) * .55
		p := [3]float64{root[0] + dx*u + nx*(bend+side*w), root[1] + dy*u + ny*(bend+side*w), z - .028*(1-side*side)*math.Sin(u*math.Pi)}
		return p, [3]float64{nx * side * .45, ny * side * .45, -1}
	}
	const rows = 16
	for row := 0; row < rows; row++ {
		for side := -1; side < 1; side++ {
			u, v := float64(row)/rows, float64(row+1)/rows
			a, na := vertex(u, float64(side))
			b, nb := vertex(u, float64(side+1))
			c, nc := vertex(v, float64(side))
			d, nd := vertex(v, float64(side+1))
			ua, ub, uc, ud := [2]float64{float64(side), u}, [2]float64{float64(side + 1), u}, [2]float64{float64(side), v}, [2]float64{float64(side + 1), v}
			o := [3]float64{a[0], a[1], z + 1}
			castSurfaceTriangle(m, [3][3]float64{a, b, c}, [3][3]float64{na, nb, nc}, [3][2]float64{ua, ub, uc}, o, tint, material)
			castSurfaceTriangle(m, [3][3]float64{c, b, d}, [3][3]float64{nc, nb, nd}, [3][2]float64{uc, ub, ud}, o, tint, material)
		}
	}
}

// gem() tags its faces with the weapon id meshStone; retag them in the cast set.
func castGem(m *weaponMeshBuilder, c, h [3]float64, tint [3]int, material int) {
	first := len(m.faces)
	m.gem(c, h, tint)
	for i := first; i < len(m.faces); i++ {
		m.faces[i].material = material
	}
}

type castProfileRing struct{ y, width, depth float64 }

func castProfile(m *weaponMeshBuilder, rings []castProfileRing, cx, cy float64, tint [3]int, material int) {
	const sides = 32
	point := func(r castProfileRing, i int) [3]float64 {
		a := float64(i) * 2 * math.Pi / sides
		return [3]float64{cx + math.Cos(a)*r.width, cy + r.y, math.Sin(a) * r.depth}
	}
	for row := 1; row < len(rings); row++ {
		lo, hi := rings[row-1], rings[row]
		origin := [3]float64{cx, cy + (lo.y+hi.y)*.5, 0}
		for i := 0; i < sides; i++ {
			a, b, c, d := point(lo, i), point(lo, i+1), point(hi, i), point(hi, i+1)
			triangle := func(points [3][3]float64, rows [3]int) {
				// Hourglass retains its accepted faceted material. Curved armour
				// gets the analytic normal of the authored elliptical shell.
				if material != castMatForged && material != castMatCloth {
					m.triangle(points[0], points[1], points[2], origin, tint, material)
					return
				}
				var normals [3][3]float64
				var uv [3][2]float64
				for j, p := range points {
					k := rows[j]
					ring := rings[k]
					prev, next := rings[max(0, k-1)], rings[min(len(rings)-1, k+1)]
					dy := math.Max(.001, next.y-prev.y)
					x, z := (p[0]-cx)/ring.width, p[2]/ring.depth
					normals[j] = [3]float64{x / ring.width, -(x*x*(next.width-prev.width)/ring.width + z*z*(next.depth-prev.depth)/ring.depth) / dy, z / ring.depth}
					uv[j] = [2]float64{p[0], p[1]}
				}
				castSurfaceTriangle(m, points, normals, uv, origin, tint, material)
			}
			triangle([3][3]float64{a, b, c}, [3]int{row - 1, row - 1, row})
			triangle([3][3]float64{c, b, d}, [3]int{row, row - 1, row})
		}
	}
}

// celestialIvory is the pearl enamel of the icon's plates; gold trims them.
var (
	celestialIvory = [3]int{245, 241, 232}
	celestialNavy  = [3]int{40, 56, 104}
	celestialBlue  = [3]int{150, 205, 250}
)

// castCabochon is a round domed jewel on a surface point.
func castCabochon(m *weaponMeshBuilder, x, y, z, rx, ry float64, tint [3]int) {
	outline := make([][2]float64, 14)
	for i := range outline {
		a := float64(i) * 2 * math.Pi / float64(len(outline))
		outline[i] = [2]float64{x + math.Cos(a)*rx, y + math.Sin(a)*ry}
	}
	castPlate(m, outline, [3]float64{x, y, z}, math.Min(rx, ry)*.9, tint, castMatJewel)
}

// smoothOutline subdivides a closed control outline with Catmull-Rom, so a
// plate's edge and its gilded rim follow the same smooth curve.
func smoothOutline(points [][2]float64, steps int) [][2]float64 {
	n := len(points)
	out := make([][2]float64, 0, n*steps)
	for i := 0; i < n; i++ {
		p0, p1, p2, p3 := points[(i-1+n)%n], points[i], points[(i+1)%n], points[(i+2)%n]
		for j := 0; j < steps; j++ {
			t := float64(j) / float64(steps)
			var p [2]float64
			for axis := range p {
				a, b, c, d := p0[axis], p1[axis], p2[axis], p3[axis]
				p[axis] = .5 * (2*b + (-a+c)*t + (2*a-5*b+4*c-d)*t*t + (-a+3*b-3*c+d)*t*t*t)
			}
			out = append(out, p)
		}
	}
	return out
}

// castShellPlate hammers one plate onto a body surface: rings of its smoothed
// outline shrink toward the centre, are projected onto the surface, lifted
// off it and gently domed, so each plate reads as its own piece with a seam.
func castShellPlate(m *weaponMeshBuilder, outline [][2]float64, surface func(x, y float64) [3]float64, lift, dome float64, tint [3]int, material int) {
	cx, cy := 0.0, 0.0
	for _, p := range outline {
		cx, cy = cx+p[0]/float64(len(outline)), cy+p[1]/float64(len(outline))
	}
	point := func(p [2]float64, k float64) [3]float64 {
		x, y := cx+(p[0]-cx)*k, cy+(p[1]-cy)*k
		q := surface(x, y)
		q[2] -= lift + dome*(1-k*k)
		return q
	}
	normal := func(a, b, c [3]float64) [3]float64 {
		u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		n := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		if n[2] > 0 { // plates face the viewer (-z)
			n = [3]float64{-n[0], -n[1], -n[2]}
		}
		return n
	}
	origin := surface(cx, cy)
	origin[2] += .2
	rings := []float64{1, .94, .84, .70, .54, .36, .18, 0}
	// Vertex normals average the faces of the radial strips on either side,
	// so the plate shades as one smooth dome.
	vnormal := func(i, r int) [3]float64 {
		n := len(outline)
		k0, k1 := rings[r], rings[min(len(rings)-1, r+1)]
		if r == len(rings)-1 {
			k0, k1 = rings[r-1], rings[r]
		}
		a, b := point(outline[i], k0), point(outline[(i+1)%n], k0)
		c := point(outline[i], k1)
		d, e := point(outline[(i-1+n)%n], k0), point(outline[i], k1)
		f1, f2 := normal(a, b, c), normal(d, a, e)
		return [3]float64{f1[0] + f2[0], f1[1] + f2[1], f1[2] + f2[2]}
	}
	for r := 1; r < len(rings); r++ {
		for i := range outline {
			j := (i + 1) % len(outline)
			a, b := point(outline[i], rings[r-1]), point(outline[j], rings[r-1])
			c, d := point(outline[i], rings[r]), point(outline[j], rings[r])
			na, nb, nc, nd := vnormal(i, r-1), vnormal(j, r-1), vnormal(i, r), vnormal(j, r)
			ua, ub := [2]float64{a[0], a[1]}, [2]float64{b[0], b[1]}
			uc, ud := [2]float64{c[0], c[1]}, [2]float64{d[0], d[1]}
			castSurfaceTriangle(m, [3][3]float64{a, b, c}, [3][3]float64{na, nb, nc}, [3][2]float64{ua, ub, uc}, origin, tint, material)
			if rings[r] > 0 {
				castSurfaceTriangle(m, [3][3]float64{c, b, d}, [3][3]float64{nc, nb, nd}, [3][2]float64{uc, ub, ud}, origin, tint, material)
			}
		}
	}
}

// castShoulderLame is one pauldron lame across the shoulder: the upper half
// of an ellipsoid whose long axis runs out from the neck, pitched toward the
// viewer and rolled down at its outer end, as the icon shows the dome's face.
// It returns the gilded lower rim and a point on the dome's front.
// side mirrors the lame so its narrow end always points away from the neck.
func castShoulderLame(m *weaponMeshBuilder, c, r [3]float64, pitch, roll, taper, side float64, tint [3]int, material int) (rim [][3]float64, front [3]float64) {
	const around, over = 22, 12
	lo, hi := -.35, math.Pi*.92
	cp, sp, cr, sr := math.Cos(pitch), math.Sin(pitch), math.Cos(roll), math.Sin(roll)
	turn := func(v [3]float64) [3]float64 {
		y, z := v[1]*cp-v[2]*sp, v[1]*sp+v[2]*cp
		return [3]float64{v[0]*cr - y*sr, v[0]*sr + y*cr, z}
	}
	point := func(th, ph float64) ([3]float64, [3]float64) {
		// taper narrows the section toward the outer end (th=0): a wedge.
		s := math.Sin(th) * (taper + (1-taper)*(1-math.Cos(th))*.5)
		d := [3]float64{side * r[0] * math.Cos(th), -r[1] * s * math.Sin(ph), -r[2] * s * math.Cos(ph)}
		n := turn([3]float64{d[0] / (r[0] * r[0]), d[1] / (r[1] * r[1]), d[2] / (r[2] * r[2])})
		d = turn(d)
		return [3]float64{c[0] + d[0], c[1] + d[1], c[2] + d[2]}, n
	}
	for i := 0; i < around; i++ {
		t0, t1 := float64(i)/around*math.Pi, float64(i+1)/around*math.Pi
		for j := 0; j < over; j++ {
			p0, p1 := lo+(hi-lo)*float64(j)/over, lo+(hi-lo)*float64(j+1)/over
			a, na := point(t0, p0)
			b, nb := point(t1, p0)
			d, nd := point(t0, p1)
			e, ne := point(t1, p1)
			castSurfaceTriangle(m, [3][3]float64{a, b, d}, [3][3]float64{na, nb, nd}, [3][2]float64{{a[0], a[1]}, {b[0], b[1]}, {d[0], d[1]}}, c, tint, material)
			castSurfaceTriangle(m, [3][3]float64{d, b, e}, [3][3]float64{nd, nb, ne}, [3][2]float64{{d[0], d[1]}, {b[0], b[1]}, {e[0], e[1]}}, c, tint, material)
		}
		p, _ := point(t0, lo)
		rim = append(rim, p)
	}
	p, _ := point(math.Pi, lo)
	front, _ = point(math.Pi/2, .45)
	return append(rim, p), front
}

// buildCelestialCuirass follows the icon: a navy under-layer covered by
// separate pearl plates whose gilded rims are the plate edges - domed
// pectorals with a gold sternum ridge and star, side plates, two chevron
// abdominal lames and a flared fauld with the blue diamond - under a wide
// gorget, with three-lame pauldrons lying across the shoulders.
func buildCelestialCuirass(m *weaponMeshBuilder) {
	// Proportions measured on the icon: chest .43, waist .30, fauld .42 and
	// pauldrons reaching .78, under a wide gorget; the fauld ends in a point.
	profile := []castProfileRing{{-.82, .30, .22}, {-.70, .40, .28}, {-.50, .44, .31}, {-.25, .43, .30}, {-.02, .36, .26}, {.18, .31, .23}, {.32, .33, .22}, {.50, .43, .24}, {.66, .38, .21}, {.98, .03, .06}}
	castProfile(m, profile, 0, 0, celestialNavy, castMatCloth)
	shell := func(x, y float64) [3]float64 {
		row := 1
		for row < len(profile)-1 && y > profile[row].y {
			row++
		}
		a, b := profile[row-1], profile[row]
		u := math.Max(0, math.Min(1, (y-a.y)/(b.y-a.y)))
		width, depth := a.width+(b.width-a.width)*u, a.depth+(b.depth-a.depth)*u
		return [3]float64{x, y, -depth * math.Sqrt(math.Max(0, 1-x*x/(width*width)))}
	}
	gild := func(points [][3]float64, radius float64) {
		for i := 1; i < len(points); i++ {
			m.rod(points[i-1], points[i], radius, radius, meshGold, castMatGilt)
		}
	}
	// plate lays one pearl plate; its gilded rim is the plate's own edge.
	plate := func(control [][2]float64, dome float64) {
		outline := smoothOutline(control, 6)
		castShellPlate(m, outline, shell, .018, dome, celestialIvory, castMatForged)
		edge := make([][3]float64, 0, len(outline)+1)
		for _, p := range append(outline, outline[0]) {
			q := shell(p[0], p[1])
			q[2] -= .024
			edge = append(edge, q)
		}
		gild(edge, .024)
	}
	mirror := func(outline [][2]float64) [][2]float64 {
		out := make([][2]float64, len(outline))
		for i, p := range outline {
			out[len(outline)-1-i] = [2]float64{-p[0], p[1]}
		}
		return out
	}
	for _, side := range []float64{1, -1} {
		half := func(outline [][2]float64) [][2]float64 {
			if side > 0 {
				return outline
			}
			return mirror(outline)
		}
		// Abdominal plate slanting to the centre: the pair forms the V of gold
		// that leads down to the diamond.
		plate(half([][2]float64{{.04, .10}, {.20, .00}, {.32, .04}, {.31, .20}, {.22, .29}, {.04, .40}}), .03)
		// Flared side plate of the fauld.
		plate(half([][2]float64{{.17, .34}, {.31, .31}, {.42, .49}, {.39, .65}, {.25, .76}, {.19, .62}}), .02)
	}
	// One domed breastplate; its lower edge sweeps into the heart-shaped point.
	plate([][2]float64{{0, -.70}, {.18, -.74}, {.33, -.69}, {.42, -.54}, {.43, -.36}, {.37, -.18}, {.24, -.04}, {.10, .05}, {0, .10}, {-.10, .05}, {-.24, -.04}, {-.37, -.18}, {-.43, -.36}, {-.42, -.54}, {-.33, -.69}, {-.18, -.74}}, .10)
	// Fauld centre plate carrying the blue diamond, pointed at the bottom.
	plate([][2]float64{{0, .44}, {.14, .38}, {.17, .62}, {0, .97}, {-.17, .62}, {-.14, .38}}, .025)
	gz := shell(0, .55)[2] - .05
	castPlate(m, [][2]float64{{0, .46}, {.06, .55}, {0, .64}, {-.06, .55}}, [3]float64{0, .55, gz}, .035, [3]int{120, 185, 245}, castMatJewel)
	// Gold sternum ridge between the pectorals, with the four-point star.
	ridge := make([][3]float64, 0, 12)
	for i := 0; i <= 11; i++ {
		y := -.68 + .76*float64(i)/11
		q := shell(0, y)
		q[2] -= .02 + .10*(1-math.Pow((y+.29)/.39, 2)) // rides the breastplate's dome
		ridge = append(ridge, q)
	}
	gild(ridge, .026)
	star := [][2]float64{{0, -.60}, {.035, -.445}, {.15, -.405}, {.035, -.365}, {0, -.20}, {-.035, -.365}, {-.15, -.405}, {-.035, -.445}}
	sz := shell(0, -.405)[2] - .125
	castPlate(m, star, [3]float64{0, -.405, sz}, .025, meshGold, castMatGilt)
	castCabochon(m, 0, -.405, sz-.03, .028, .028, [3]int{235, 244, 255})
	// Wide gorget: a gilded oval rim around the navy lining.
	gorget := []castProfileRing{{-.95, .36, .25}, {-.90, .40, .28}, {-.78, .38, .27}, {-.70, .41, .29}}
	castProfile(m, gorget, 0, 0, celestialIvory, castMatForged)
	castProfile(m, []castProfileRing{{-.948, .001, .001}, {-.948, .35, .24}}, 0, 0, celestialNavy, castMatCloth)
	for _, g := range []castProfileRing{gorget[0], gorget[3]} {
		castProfile(m, []castProfileRing{{g.y, g.width + .005, g.depth + .005}, {g.y + .03, g.width + .012, g.depth + .012}}, 0, 0, meshGold, castMatGilt)
	}
	for _, side := range []float64{-1, 1} {
		// Two curved wedge lames lie across each shoulder, the lower one
		// stepping down the arm beneath the large upper one.
		for _, l := range []struct {
			c, r        [3]float64
			pitch, roll float64
		}{
			{[3]float64{.53, -.32, .01}, [3]float64{.27, .19, .24}, .62, .85},
			{[3]float64{.47, -.55, -.06}, [3]float64{.36, .28, .29}, .72, .62},
		} {
			c := [3]float64{side * l.c[0], l.c[1], l.c[2]}
			rim, front := castShoulderLame(m, c, [3]float64{l.r[0], l.r[1], l.r[2]}, l.pitch, side*l.roll, .30, side, celestialIvory, castMatForged)
			gild(rim, .024)
			if l.c[1] < -.5 { // the jewel sits on the large top lame
				castCabochon(m, front[0], front[1], front[2]-.025, .048, .048, celestialBlue)
			}
		}
		// Small shoulder wings rise up and out behind the pauldrons, white with
		// pale blue tips, unlike Preservation's embracing golden wings.
		for i := 0; i < 5; i++ {
			f := float64(i) / 4
			a := -1.18 + f*1.02 // from up-and-out to nearly level
			length := .66 - .10*f
			root := [2]float64{side * .68, -.74 + f*.07}
			tip := [2]float64{root[0] + side*length*math.Cos(a), root[1] + length*math.Sin(a)}
			tint := [3]int{244 - int(28*f), 248 - int(16*f), 255}
			castFeather(m, root, tip, .085-.012*f, .04-.012*float64(i), 1, tint, castMatPlume)
		}
	}
}

// preservationWristU splits the embracing wing into its shoulder (forearm)
// and wrist (hand) pieces; the draw pivots the hand about preservationArm.
const preservationWristU = .55

// preservationArm is the closed right wing's leading edge, shoulder at the
// origin: half of the almond around the candle flame on the icon. The left
// wing is the same wing turned half a circle about the almond's centre.
func preservationArm(u float64) (x, y float64) {
	a := (-88 + u*170) * math.Pi / 180
	return .02 + .70*math.Cos(a), .80 + .86*math.Sin(a)
}

// preservationJitter is a fixed per-feather variation, so the ranks read as
// grown feathers rather than a stamped pattern while staying identical every cast.
func preservationJitter(i, rank int) float64 {
	v := math.Sin(float64(i)*12.9898+float64(rank)*78.233) * 43758.5453
	return v - math.Floor(v) - .5
}

// buildPreservationWing follows the anatomy of a folded wing: pointed
// primaries on the hand and rounded secondaries on the forearm (bottom
// layer), greater coverts over their quills, two rows of small lesser coverts
// along the leading edge and the alula at the wrist. Feathers trail down the
// almond and tuck inward, so the primaries cross beyond the flame.
func buildPreservationWing(m *weaponMeshBuilder, tips, back bool) {
	// The front wing wraps toward the viewer along its arm, the back one away.
	wrap := -.30
	if back {
		wrap = .30
	}
	wx, wy := preservationArm(preservationWristU)
	frame := func(u float64) (x, y, tx, ty, nx, ny float64) {
		x, y = preservationArm(u)
		x1, y1 := preservationArm(math.Min(1, u+.02))
		x0, y0 := preservationArm(math.Max(0, u-.02))
		tx, ty = x1-x0, y1-y0
		l := math.Hypot(tx, ty)
		tx, ty = tx/l, ty/l
		// The inward normal of the right wing's arc points toward its axis.
		nx, ny = -ty, tx
		if nx > 0 {
			nx, ny = -nx, -ny
		}
		return
	}
	feather := func(u, inset, inward, length, width, z, tipRound float64, tint [3]int) {
		if (u > preservationWristU) != tips {
			return
		}
		x, y, _, _, nx, ny := frame(u)
		// The tip follows the almond onward (the arc runs about 2.2 units per
		// unit of u), so long feathers continue the spiral instead of jutting.
		ex, ey := preservationArm(u + length/2.2)
		root := [2]float64{x + nx*inset, y + ny*inset}
		tip := [2]float64{ex + nx*(inset+inward*length*.5), ey + ny*(inset+inward*length*.5)}
		if tips {
			root[0], root[1], tip[0], tip[1] = root[0]-wx, root[1]-wy, tip[0]-wx, tip[1]-wy
		}
		castFeather(m, root, tip, width, z-.05*math.Sin(u*math.Pi)+wrap*u, tipRound, tint, castMatFeather)
	}
	gold := func(g float64) [3]int { return [3]int{255, int(238 - 50*g), int(200 - 110*g)} }
	// Flight feathers, outermost first so the inner ones overlap them on top.
	for i := 9; i >= 0; i-- {
		f := float64(i) / 9
		j := preservationJitter(i, 0)
		u := .57 + .43*math.Pow(f, .85) + .012*j
		feather(u, .04, .80-.30*f, (.62+.36*f)*(1+.08*j), .100*(1+.10*j), -.04, 0, gold(.55+.45*f))
	}
	for i := 7; i >= 0; i-- {
		f := float64(i) / 7
		j := preservationJitter(i, 1)
		feather(.10+.44*f+.015*j, .05, .85-.15*f, (.50+.08*f)*(1+.10*j), .110*(1+.12*j), -.06, 1, gold(.20+.30*f))
	}
	// Greater coverts sit over the flight-feather quills, shorter and rounded.
	for i := 13; i >= 0; i-- {
		f := float64(i) / 13
		j := preservationJitter(i, 2)
		feather(.06+.80*f+.01*j, .03, .85-.25*f, (.30+.12*f)*(1+.12*j), .095*(1+.10*j), -.10, 1, [3]int{255, int(243 - 18*f), int(218 - 40*f)})
	}
	// Two rows of small lesser coverts scale the leading edge.
	for row := 0; row < 2; row++ {
		for i := 12; i >= 0; i-- {
			f := (float64(i) + .5*float64(row)) / 12.5
			j := preservationJitter(i, 3+row)
			feather(.02+.66*f+.012*j, .005+.035*float64(row), .45, (.17+.05*f)*(1+.20*j)*(1-.15*float64(row)), .075*(1+.15*j), -.14-.03*float64(row), 1, [3]int{255, 246, 226})
		}
	}
	// The alula: three short stiff feathers on the thumb at the wrist.
	for i := 0; i < 3; i++ {
		feather(preservationWristU-.005-.012*float64(i), 0, .18, .20-.03*float64(i), .045, -.19, .4, [3]int{255, 247, 228})
	}
}

func buildCastObject(model int) []modelFace {
	m := &weaponMeshBuilder{}
	if model == castCuirass {
		buildCelestialCuirass(m)
	} else if model == castWing || model == castWingTips || model == castWingBack || model == castWingTipsBack {
		buildPreservationWing(m, model == castWingTips || model == castWingTipsBack, model == castWingBack || model == castWingTipsBack)
	} else if model == castHourglass {
		// A brass hourglass with curved glass, two sand reservoirs and a thin
		// falling stream. The object turns rigidly; only the sand level changes.
		for _, y := range []float64{-.82, .82} {
			castProfile(m, []castProfileRing{{y - .035, .001, .001}, {y - .035, .57, .42}, {y + .035, .57, .42}, {y + .035, .001, .001}}, 0, 0, meshGold, castMatPlain)
		}
		for _, x := range []float64{-.47, .47} {
			for _, z := range []float64{-.27, .27} {
				m.rod([3]float64{x, -.78, z}, [3]float64{x, .78, z}, .035, .035, meshGold, castMatPlain)
				castGem(m, [3]float64{x, 0, z}, [3]float64{.06, .10, .06}, [3]int{132, 207, 246}, castMatGlass)
			}
		}
		glass := []castProfileRing{{-.75, .35, .30}, {-.57, .43, .35}, {-.32, .26, .23}, {-.04, .055, .05}, {.04, .055, .05}, {.32, .26, .23}, {.57, .43, .35}, {.75, .35, .30}}
		castProfile(m, glass, 0, 0, [3]int{155, 220, 235}, castMatGlass)
		castProfile(m, []castProfileRing{{-.61, .34, .27}, {-.38, .27, .23}, {-.08, .014, .014}}, 0, 0, [3]int{251, 192, 64}, castMatSand)
		castProfile(m, []castProfileRing{{.18, .012, .012}, {.69, .34, .27}, {.70, .01, .01}}, 0, 0, [3]int{251, 192, 64}, castMatSand)
		m.rod([3]float64{0, -.06, 0}, [3]float64{0, .53, 0}, .009, .012, [3]int{255, 226, 132}, castMatStream)
	}
	// The hourglass keeps its plain brass; every other object polishes its trim.
	if model != castHourglass {
		for i := range m.faces {
			if m.faces[i].material == castMatPlain {
				m.faces[i].material = castMatPolished
			}
		}
	}
	return m.faces
}
