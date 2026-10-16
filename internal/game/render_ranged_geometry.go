package game

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type rangedSection struct {
	p            [3]float64
	width, depth float64
}

// A tapered continuous tube with elliptical sections. Unlike capped rods,
// adjacent bends share their rims and leave no internal faces.
func rangedTube(m *weaponMeshBuilder, s []rangedSection, col [3]int, material int) {
	const sides = 8
	ring := func(i, k int) [3]float64 {
		a, b := s[max(0, i-1)].p, s[min(len(s)-1, i+1)].p
		dx, dy := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(dx, dy)
		if l < 1e-8 {
			dx, dy, l = 1, 0, 1
		}
		t := float64(k) * 2 * math.Pi / sides
		return [3]float64{s[i].p[0] - dy/l*s[i].width*math.Cos(t), s[i].p[1] + dx/l*s[i].width*math.Cos(t), s[i].p[2] + s[i].depth*math.Sin(t)}
	}
	for i := 1; i < len(s); i++ {
		o := [3]float64{(s[i-1].p[0] + s[i].p[0]) / 2, (s[i-1].p[1] + s[i].p[1]) / 2, (s[i-1].p[2] + s[i].p[2]) / 2}
		for j := 0; j < sides; j++ {
			a, b, c, d := ring(i-1, j), ring(i-1, j+1), ring(i, j), ring(i, j+1)
			tint := col
			if material == meshGrip {
				tint = mixColor(col, [3]int{51, 26, 13}, .07*float64(j%3))
			}
			m.triangle(a, b, c, o, tint, material)
			m.triangle(c, b, d, o, tint, material)
		}
	}
	for _, i := range []int{0, len(s) - 1} {
		for j := 0; j < sides; j++ {
			m.triangle(s[i].p, ring(i, j), ring(i, j+1), s[len(s)/2].p, col, material)
		}
	}
}

// Interpolate construction sections before compiling, so a branch or bow
// curves continuously while its tiny cross-section remains faceted.
func rangedSmooth(s []rangedSection) []rangedSection {
	out := make([]rangedSection, 0, (len(s)-1)*4+1)
	for i := 0; i < len(s)-1; i++ {
		a, b, c, d := s[max(0, i-1)], s[i], s[i+1], s[min(len(s)-1, i+2)]
		for j := 0; j < 4; j++ {
			t := float64(j) / 4
			curve := func(a, b, c, d float64) float64 {
				return .5 * ((2 * b) + (-a+c)*t + (2*a-5*b+4*c-d)*t*t + (-a+3*b-3*c+d)*t*t*t)
			}
			q := rangedSection{width: max(.002, curve(a.width, b.width, c.width, d.width)), depth: max(.002, curve(a.depth, b.depth, c.depth, d.depth))}
			for k := range q.p {
				q.p[k] = curve(a.p[k], b.p[k], c.p[k], d.p[k])
			}
			out = append(out, q)
		}
	}
	return append(out, s[len(s)-1])
}

func rangedLeaf(m *weaponMeshBuilder, root, tip [3]float64, width float64) {
	d := [3]float64{tip[0] - root[0], tip[1] - root[1], tip[2] - root[2]}
	l := math.Hypot(d[0], d[1])
	if l == 0 {
		return
	}
	mid := [3]float64{root[0] + d[0]*.48, root[1] + d[1]*.48, root[2] + d[2]*.48}
	a, b := mid, mid
	a[0] -= d[1] / l * width
	a[1] += d[0] / l * width
	b[0] += d[1] / l * width
	b[1] -= d[0] / l * width
	ridge := mid
	ridge[2] -= .024
	for _, sg := range []float64{-1, 1} {
		ridge[2] = mid[2] + sg*.024
		m.triangle(root, a, ridge, mid, [3]int{66, 108, 20}, meshGrip)
		m.triangle(a, tip, ridge, mid, [3]int{126, 166, 34}, meshGrip)
		m.triangle(tip, b, ridge, mid, [3]int{95, 133, 24}, meshGrip)
		m.triangle(b, root, ridge, mid, [3]int{164, 179, 53}, meshGrip)
	}
	m.rod(root, tip, .006, .003, [3]int{180, 192, 66}, meshGrip)
}

func buildOakHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	wood := [3]int{170, 101, 44}
	light := [3]int{213, 147, 71}
	dark := [3]int{92, 49, 22}
	// The gnarled shaft, with two real helical bark ridges.
	var stem []rangedSection
	for i := 0; i <= 24; i++ {
		t := float64(i) / 24
		x := -2.15 + t*2.62
		stem = append(stem, rangedSection{[3]float64{x, .032 * math.Sin(t*10), .018 * math.Sin(t*7)}, .074 + .026*t, .07 + .017*t})
	}
	rangedTube(&m, stem, wood, meshGrip)
	for strand := 0; strand < 2; strand++ {
		var s []rangedSection
		for i := 0; i <= 54; i++ {
			t := float64(i) / 54
			x := -2.15 + t*2.63
			a := t*math.Pi*6 + float64(strand)*math.Pi
			s = append(s, rangedSection{[3]float64{x, .032*math.Sin(t*10) + .077*math.Cos(a), .018*math.Sin(t*7) + .075*math.Sin(a)}, .022, .022})
		}
		rangedTube(&m, s, light, meshGrip)
	}
	// Moss-green crossed bindings, without the metal bands of unrelated staves.
	for i := 0; i < 7; i++ {
		x := -1.50 + float64(i)*.125
		m.rod([3]float64{x, 0, 0}, [3]float64{x + .038, 0, 0}, .099, .099, [3]int{68, 91, 39}, meshGrip)
		m.rod([3]float64{x + .039, 0, 0}, [3]float64{x + .057, 0, 0}, .102, .102, [3]int{193, 153, 74}, meshGrip)
	}
	// Open crown: distinct forked limbs weave in front of and behind the gem.
	rangedCurve(&m, [][3]float64{{.22, 0, 0}, {.47, -.17, .05}, {.72, -.31, .04}, {1.04, -.37, 0}, {1.30, -.30, -.01}, {1.48, -.13, .02}, {1.63, -.18, .03}, {1.78, -.38, .04}}, []float64{.11, .105, .10, .085, .078, .065, .045, .008}, .86, wood, meshGrip)
	rangedCurve(&m, [][3]float64{{.36, .02, .03}, {.61, .22, .07}, {.85, .34, .06}, {1.13, .30, .05}, {1.37, .15, .07}, {1.51, .21, .09}, {1.61, .42, .10}}, []float64{.10, .10, .088, .075, .065, .045, .007}, .86, light, meshGrip)
	rangedCurve(&m, [][3]float64{{.33, 0, -.02}, {.62, -.15, -.18}, {.87, -.14, -.22}, {1.12, .11, -.20}, {1.32, .19, -.11}, {1.41, .31, -.06}}, []float64{.065, .068, .055, .045, .032, .006}, .86, wood, meshGrip)
	rangedCurve(&m, [][3]float64{{.57, .10, .10}, {.85, .20, .19}, {1.12, .15, .20}, {1.31, -.04, .14}, {1.46, -.06, .07}}, []float64{.06, .065, .055, .035, .006}, .86, dark, meshGrip)
	// Warm highlights follow the bark, rather than changing the wood into metal.
	rangedCurve(&m, [][3]float64{{.49, -.22, -.045}, {.73, -.355, -.04}, {1.03, -.405, -.04}, {1.27, -.335, -.04}, {1.44, -.17, -.03}}, []float64{.015, .014, .013, .009, .003}, .86, light, meshGrip)
	rangedCurve(&m, [][3]float64{{.63, .27, -.02}, {.88, .38, -.025}, {1.14, .34, -.015}, {1.36, .19, 0}}, []float64{.016, .014, .010, .003}, .86, light, meshGrip)
	// An elongated, asymmetrically faceted emerald nested inside the empty cage.
	c := [3]float64{.94, -.015, 0}
	rings := []struct{ x, r float64 }{{-.23, .025}, {-.13, .13}, {.10, .135}, {.22, .02}}
	for i := 1; i < len(rings); i++ {
		for j := 0; j < 7; j++ {
			point := func(k, l int) [3]float64 {
				a := float64(l) * 2 * math.Pi / 7
				return [3]float64{c[0] + rings[k].x, c[1] + rings[k].r*math.Cos(a), c[2] + rings[k].r*math.Sin(a)}
			}
			a, b, d, e := point(i-1, j), point(i-1, j+1), point(i, j), point(i, j+1)
			col := [][3]int{{34, 158, 60}, {71, 203, 75}, {152, 233, 107}, {22, 114, 49}, {38, 182, 87}, {116, 217, 102}, {34, 137, 53}}[j]
			m.triangle(a, b, d, c, col, meshStone)
			m.triangle(d, b, e, c, col, meshStone)
		}
	}
	for _, leaf := range [][3][3]float64{
		{{.61, .27, .06}, {.47, .56, .06}, {.065, 0, 0}}, {{1.13, .32, .04}, {1.08, .61, .02}, {.07, 0, 0}},
		{{1.32, -.29, .02}, {1.27, -.55, .01}, {.065, 0, 0}}, {{.91, -.34, .03}, {.80, -.57, .01}, {.06, 0, 0}},
		{{.53, -.19, .05}, {.34, -.36, .04}, {.055, 0, 0}},
	} {
		rangedLeaf(&m, leaf[0], leaf[1], leaf[2][0])
	}
	return m.faces
}

func buildBowHeldModel() []modelFace {
	m := weaponMeshBuilder{}
	brown := [3]int{157, 79, 30}
	amber := [3]int{200, 121, 53}
	silver := [3]int{182, 193, 200}
	// Flattened stave with recurved tips, steel nocks, and a bound central grip.
	for _, sg := range []float64{-1, 1} {
		base := [][2]float64{{.15, 0}, {.35, .09}, {.60, .20}, {.86, .24}, {1.10, .17}, {1.29, .025}, {1.43, -.14}, {1.52, -.26}, {1.63, -.24}, {1.67, -.16}}
		var s []rangedSection
		for i, p := range base {
			t := float64(i) / float64(len(base)-1)
			s = append(s, rangedSection{[3]float64{sg * p[0], p[1], 0}, .072 - .044*t, .052 - .026*t})
		}
		rangedTube(&m, rangedSmooth(s), brown, meshGrip)
		// Two long light laminations catch the bow's curve on its broad face.
		for _, side := range []float64{-1, 1} {
			var line []rangedSection
			for i, p := range s {
				p.p[2] = side * p.depth
				p.width = .012 - float64(i)*.0008
				p.depth = .006
				line = append(line, p)
			}
			rangedTube(&m, rangedSmooth(line), amber, meshGrip)
		}
		ferrule := append([]rangedSection(nil), s[len(s)-3:]...)
		for i := range ferrule {
			ferrule[i].width *= 1.40
			ferrule[i].depth *= 1.45
		}
		rangedTube(&m, rangedSmooth(ferrule), silver, meshMetal)
		for _, x := range []float64{.22, .30} {
			m.rod([3]float64{sg * x, .025, 0}, [3]float64{sg * (x + .035), .04, 0}, .095, .095, silver, meshMetal)
		}
	}
	m.grip(-.22, .22, 0, .087, [3]int{73, 38, 23}, silver)
	for i := 0; i < 7; i++ {
		x := -.19 + float64(i)*.06
		m.rod([3]float64{x, 0, 0}, [3]float64{x + .016, 0, 0}, .096, .096, [3]int{174, 119, 65}, meshGrip)
	}

	return m.faces
}

// equipmentTransform turns a camera-local weapon model by yaw, pitch and roll;
// mirror -1 reflects it across its shaft (a single-edged blade in the other
// hand).
func equipmentTransform(angle, pitch, roll, mirror float64) volumeTransform {
	ca, sa, cp, sp, cr, sr := math.Cos(angle), math.Sin(angle), math.Cos(pitch), math.Sin(pitch), math.Cos(roll), math.Sin(roll)
	return volumeTransform{rotation: [3][3]float64{
		{cp * ca, -mirror*sr*sp*ca - mirror*cr*sa, -cr*sp*ca + sr*sa},
		{cp * sa, -mirror*sr*sp*sa + mirror*cr*ca, -cr*sp*sa - sr*ca},
		{sp, mirror * sr * cp, cr * cp},
	}, uvY: 1}
}
func rangedBowTransform(yaw, roll float64) volumeTransform {
	cy, sy, cr, sr := math.Cos(yaw), math.Sin(yaw), math.Cos(roll), math.Sin(roll)
	// Cant rotates the limbs around the flight axis; it must not tilt that axis.
	return volumeTransform{rotation: [3][3]float64{{sr * cy, -sy, -cr * cy}, {-cr, 0, -sr}, {sr * sy, cy, -cr * sy}}, uvY: 1}
}

// Ranged equipment uses the same object-space mesh, camera projection,
// lighting and resolve as the shipped melee equipment. view is the 3D view's
// size, which sizes the shared equipment layers.
func (r *Renderer) drawRangedHeldMesh(dst *ebiten.Image, view image.Point, x, y, size, alpha float64, tr volumeTransform, mesh *volumeMesh) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	if r.rangedMaterialOpts.Uniforms == nil {
		r.rangedMaterialOpts.Uniforms = map[string]any{"Time": r.rangedMaterialTime[:], "DepthStage": r.rangedDepthStage[:]}
	}
	r.rangedMaterialTime[0] = float32(r.weaponMaterialClock())
	tr.depth = true
	verts, indices, bounds := r.prepareCameraVolume(dst.Bounds(), x, y, size, alpha, tr, mesh, [3]int{255, 255, 255}, 0, true)
	if len(indices) == 0 || bounds.Empty() {
		return
	}
	r.drawRangedDepthResolved(dst, view, verts, indices, bounds, alpha)
}
