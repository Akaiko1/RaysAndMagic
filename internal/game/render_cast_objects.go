package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/shaders"
)

// Camera-local spell objects reuse the weapon mesh renderer: real curved faces,
// stable perspective and self-occlusion, not a flat icon enlarged over the view.
const (
	castCuirass = iota
	castHourglass
	castWing
	castCandle
	castWingTips
)

var castObjectMeshes volumeMeshCache

type castObjectRenderer struct {
	shader  *ebiten.Shader
	options ebiten.DrawTrianglesShaderOptions
	phase   [1]float32
}

func (r *Renderer) drawCastObject(dst *ebiten.Image, a buffFxAnim, model int) {
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
		// Shoulder and wrist close in sequence. Feather lengths stay fixed;
		// the lower flight feathers fold around the candle, never open upward.
		closed := smoothStep((t - .06) / .48)
		angle := -.58 * (1 - closed)
		cr, sr := math.Cos(angle), math.Sin(angle)
		cy, sy := math.Cos(.40*(1-closed)), math.Sin(.40*(1-closed))
		for _, side := range []float64{-1, 1} {
			pose := volumeTransform{rotation: [3][3]float64{{side * cy * cr, -side * cy * sr, side * sy}, {sr, cr, 0}, {-sy * cr, sy * sr, cy}}, uvY: 1}
			r.drawCameraVolume(dst, ox+w*.5+side*h*.034, oy+h*.33, h*.24, alpha, pose, o.shader, &o.options, castObjectMeshes.get(castWing, buildCastObject), [3]int{255, 255, 255}, 0, true)
			wrist := -.42 * (1 - smoothStep((t-.16)/.44))
			cw, sw := math.Cos(wrist), math.Sin(wrist)
			fold := pose
			for axis := 0; axis < 3; axis++ {
				fold.rotation[axis][0] = pose.rotation[axis][0]*cw + pose.rotation[axis][1]*sw
				fold.rotation[axis][1] = -pose.rotation[axis][0]*sw + pose.rotation[axis][1]*cw
			}
			// Both cached pieces share an elbow; transform the pivot by the
			// same shoulder pose instead of independently centering each piece.
			ex := pose.rotation[0][0]*.39 + pose.rotation[0][1]*.68
			ey := pose.rotation[1][0]*.39 + pose.rotation[1][1]*.68
			r.drawCameraVolume(dst, ox+w*.5+side*h*.034+ex*h*.24, oy+h*.33+ey*h*.24, h*.24, alpha, fold, o.shader, &o.options, castObjectMeshes.get(castWingTips, buildCastObject), [3]int{255, 255, 255}, 0, true)
		}
		pose := volumeTransform{rotation: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, uvY: 1}
		r.drawCameraVolume(dst, ox+w*.5, oy+h*.69, h*.24, alpha, pose, o.shader, &o.options, castObjectMeshes.get(castCandle, buildCastObject), [3]int{255, 255, 255}, 0, true)
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
	if material == 5 {
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

func castFeather(m *weaponMeshBuilder, root, tip [2]float64, width, z float64, tint [3]int) {
	dx, dy := tip[0]-root[0], tip[1]-root[1]
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l, dx/l
	vertex := func(u, side float64) ([3]float64, [3]float64) {
		w := width * math.Pow(math.Sin(u*math.Pi), .7) * (1.12 - .30*u)
		bend := width * math.Sin(u*math.Pi) * .65
		p := [3]float64{root[0] + dx*u + nx*(bend+side*w), root[1] + dy*u + ny*(bend+side*w), z - .028*(1-side*side)*math.Sin(u*math.Pi)}
		return p, [3]float64{nx * side * .45, ny * side * .45, -1}
	}
	for row := 0; row < 12; row++ {
		for side := -1; side < 1; side++ {
			u, v := float64(row)/12, float64(row+1)/12
			a, na := vertex(u, float64(side))
			b, nb := vertex(u, float64(side+1))
			c, nc := vertex(v, float64(side))
			d, nd := vertex(v, float64(side+1))
			ua, ub, uc, ud := [2]float64{float64(side), u}, [2]float64{float64(side + 1), u}, [2]float64{float64(side), v}, [2]float64{float64(side + 1), v}
			o := [3]float64{a[0], a[1], z + 1}
			castSurfaceTriangle(m, [3][3]float64{a, b, c}, [3][3]float64{na, nb, nc}, [3][2]float64{ua, ub, uc}, o, tint, 5)
			castSurfaceTriangle(m, [3][3]float64{c, b, d}, [3][3]float64{nc, nb, nd}, [3][2]float64{uc, ub, ud}, o, tint, 5)
		}
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
				// and wax get the analytic normal of the authored elliptical shell.
				if material != 1 && material != 6 {
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

func buildCelestialCuirass(m *weaponMeshBuilder) {
	// The icon's silhouette: raised gorget, broad rounded shoulders, a pinched
	// waist and a short pointed fauld. No hanging strips or tall collar straps.
	profile := []castProfileRing{{-.78, .22, .15}, {-.68, .35, .20}, {-.52, .46, .27}, {-.29, .44, .29}, {-.04, .36, .25}, {.19, .29, .20}, {.32, .28, .19}, {.47, .34, .21}, {.66, .35, .19}, {.83, .035, .07}}
	castProfile(m, profile, 0, 0, [3]int{221, 230, 239}, 1)
	front := func(x, y float64) [3]float64 {
		row := 1
		for row < len(profile)-1 && y > profile[row].y {
			row++
		}
		a, b := profile[row-1], profile[row]
		u := math.Max(0, math.Min(1, (y-a.y)/(b.y-a.y)))
		width, depth := a.width+(b.width-a.width)*u, a.depth+(b.depth-a.depth)*u
		z := -depth*math.Sqrt(math.Max(0, 1-x*x/(width*width))) - .022
		return [3]float64{x, y, z}
	}
	line := func(points [][2]float64, radius float64) {
		for i := 1; i < len(points); i++ {
			// Catmull-Rom follows the trim's control skeleton while its depth
			// is sampled from the same torso shell, so decoration cannot float.
			p0, p1, p2, p3 := points[max(0, i-2)], points[i-1], points[i], points[min(len(points)-1, i+1)]
			point := func(t float64) [3]float64 {
				var p [2]float64
				for axis := range p {
					a, b, c, d := p0[axis], p1[axis], p2[axis], p3[axis]
					p[axis] = .5 * (2*b + (-a+c)*t + (2*a-5*b+4*c-d)*t*t + (-a+3*b-3*c+d)*t*t*t)
				}
				return front(p[0], p[1])
			}
			for j := 0; j < 6; j++ {
				m.rod(point(float64(j)/6), point(float64(j+1)/6), radius, radius, meshGold, 0)
			}
		}
	}
	castProfile(m, []castProfileRing{{-.88, .205, .15}, {-.85, .23, .17}, {-.71, .25, .18}, {-.67, .30, .205}}, 0, 0, meshSilver, 1)
	for _, y := range []float64{-.87, -.70} {
		castProfile(m, []castProfileRing{{y, .23, .175}, {y + .018, .24, .185}}, 0, 0, meshGold, 0)
	}
	for _, side := range []float64{-1, 1} {
		// Overlapping articulated pauldrons sit on the actual shoulder line.
		for row := 1; row >= 0; row-- {
			cy := -.51 + float64(row)*.14
			cx := side * (.46 + float64(row)*.025)
			outline := [][2]float64{{-.20, -.08}, {-.14, -.23}, {-.03, -.26}, {.13, -.19}, {.23, -.07}, {.22, .07}, {.13, .17}, {-.01, .14}, {-.16, .04}}
			for i := range outline {
				outline[i] = [2]float64{cx + side*outline[i][0], cy + outline[i][1]}
			}
			castPlate(m, outline, [3]float64{cx, cy - .025, -.14}, .15, meshSilver, 1)
			for i, p := range outline {
				q := outline[(i+1)%len(outline)]
				m.rod([3]float64{p[0], p[1], -.155}, [3]float64{q[0], q[1], -.155}, .012, .012, meshGold, 0)
			}
		}
		// Small shoulder wings point up/out, unlike Preservation's downturned wings.
		for i := 0; i < 4; i++ {
			f := float64(i)
			castFeather(m, [2]float64{side * .50, -.59 + f*.06}, [2]float64{side * (.87 - f*.035), -1.03 + f*.16}, .075, -.025-f*.012, [3]int{231, 239, 251})
		}
		for _, points := range [][][2]float64{
			{{.17, -.70}, {.22, -.52}, {.36, -.42}, {.30, -.25}, {.16, -.18}, {.10, -.07}},
			{{.10, -.07}, {.19, .12}, {.18, .27}, {.27, .45}, {.17, .61}, {.08, .71}},
			{{.34, -.48}, {.29, -.40}, {.30, -.32}, {.24, -.30}, {.21, -.35}},
			{{.23, .35}, {.14, .43}, {.16, .54}, {.23, .53}},
		} {
			for i := range points {
				points[i][0] *= side
			}
			line(points, .012)
		}
		// Gold-edged waist lames follow the narrow shell, not an overlaid broad skirt.
		for _, y := range []float64{.22, .32, .45, .59, .70} {
			points := make([][2]float64, 9)
			for i := range points {
				x := float64(i) / 8 * .23
				points[i] = [2]float64{side * x, y - .09*x/.23}
			}
			line(points, .012)
		}
		m.gem([3]float64{side * .43, -.59, -.32}, [3]float64{.046, .050, .025}, meshGold)
	}
	for _, side := range []float64{-1, 1} {
		line([][2]float64{{0, -.63}, {side * .04, -.54}, {side * .13, -.50}, {side * .18, -.37}, {side * .13, -.24}, {0, -.13}}, .016)
		line([][2]float64{{side * .13, -.46}, {side * .09, -.43}, {side * .075, -.35}, {side * .12, -.32}, {side * .145, -.37}}, .010)
	}
	line([][2]float64{{0, -.13}, {0, .78}}, .012)
}

func buildCastObject(model int) []modelFace {
	m := &weaponMeshBuilder{}
	if model == castCuirass {
		buildCelestialCuirass(m)
		// Weapon gems use material 2; the hourglass reserves that for glass.
		// Cuirass jewels and rivets are solid, so assign their own finish here.
		for i := range m.faces {
			if m.faces[i].material == meshStone {
				m.faces[i].material = 7
			}
		}
	} else if model == castWing || model == castWingTips {
		// Each row follows the descending curved arm. The long tips sweep
		// inward below the candle, not upward from one fan-shaped root.
		for layer := 0; layer < 3; layer++ {
			for i := 13; i >= 0; i-- {
				u := float64(i) / 13
				if (u > .55) != (model == castWingTips) {
					continue
				}
				inset := float64(layer) * .09
				root := [2]float64{.035 + (.46-inset)*math.Sin(u*math.Pi*.90), .025 + u*1.10}
				tip := [2]float64{.15 + (.59-inset)*math.Sin(u*math.Pi) - .52*math.Pow(u, 3), .40 + u*1.25}
				// Coverts overlap the flight-feather roots, with shorter lengths.
				for axis := range tip {
					tip[axis] = root[axis] + (tip[axis]-root[axis])*(1-float64(layer)*.24)
				}
				if model == castWingTips {
					root[0] -= .39
					root[1] -= .68
					tip[0] -= .39
					tip[1] -= .68
				}
				castFeather(m, root, tip, .060+.019*math.Sin(u*math.Pi), -.08-float64(layer)*.055-.07*math.Sin(u*math.Pi), [3]int{255, 232 + layer*5, 171 + layer*20})
			}
		}
	} else if model == castCandle {
		castProfile(m, []castProfileRing{{.13, .001, .001}, {.13, .21, .16}, {.19, .23, .18}, {.24, .17, .12}, {.26, .001, .001}}, 0, 0, meshGold, 0)
		castProfile(m, []castProfileRing{{-.67, .001, .001}, {-.66, .09, .09}, {-.60, .115, .11}, {.15, .115, .11}}, 0, 0, [3]int{246, 223, 172}, 6)
		for i := 0; i < 7; i++ {
			a := float64(i) * 2 * math.Pi / 7
			x, z := math.Cos(a)*.108, math.Sin(a)*.108
			m.rod([3]float64{x, -.59, z}, [3]float64{x, -.35 + .09*math.Sin(a*3), z}, .018, .026, [3]int{253, 235, 193}, 6)
		}
		m.rod([3]float64{0, -.65, 0}, [3]float64{.008, -.74, 0}, .012, .009, [3]int{32, 20, 16}, 0)
	} else if model == castHourglass {
		// A brass hourglass with curved glass, two sand reservoirs and a thin
		// falling stream. The object turns rigidly; only the sand level changes.
		for _, y := range []float64{-.82, .82} {
			castProfile(m, []castProfileRing{{y - .035, .001, .001}, {y - .035, .57, .42}, {y + .035, .57, .42}, {y + .035, .001, .001}}, 0, 0, meshGold, 0)
		}
		for _, x := range []float64{-.47, .47} {
			for _, z := range []float64{-.27, .27} {
				m.rod([3]float64{x, -.78, z}, [3]float64{x, .78, z}, .035, .035, meshGold, 0)
				m.gem([3]float64{x, 0, z}, [3]float64{.06, .10, .06}, [3]int{132, 207, 246})
			}
		}
		glass := []castProfileRing{{-.75, .35, .30}, {-.57, .43, .35}, {-.32, .26, .23}, {-.04, .055, .05}, {.04, .055, .05}, {.32, .26, .23}, {.57, .43, .35}, {.75, .35, .30}}
		castProfile(m, glass, 0, 0, [3]int{155, 220, 235}, 2)
		castProfile(m, []castProfileRing{{-.61, .34, .27}, {-.38, .27, .23}, {-.08, .014, .014}}, 0, 0, [3]int{251, 192, 64}, 3)
		castProfile(m, []castProfileRing{{.18, .012, .012}, {.69, .34, .27}, {.70, .01, .01}}, 0, 0, [3]int{251, 192, 64}, 3)
		m.rod([3]float64{0, -.06, 0}, [3]float64{0, .53, 0}, .009, .012, [3]int{255, 226, 132}, 4)
	}
	if model != castHourglass {
		for i := range m.faces {
			if m.faces[i].material == 0 {
				m.faces[i].material = 8
			}
		}
	}
	return m.faces
}
