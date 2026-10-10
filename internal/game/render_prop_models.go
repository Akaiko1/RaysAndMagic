package game

import (
	"math"
	"sync"

	"ugataima/internal/character"
)

const (
	worldEmber = 6
	worldWood  = 8
	worldStone = 9
	worldFlame = 10
)

type propPart struct {
	faces           []worldModelTriangle
	pivot           worldModelPoint
	yaw, size, side float64
	falling         bool
}
type propModel struct {
	charredWood bool        // wood transitions to charcoal when this model is spent
	staticPoses [2]sync.Map // unopened and spent poses, keyed by scale/yaw
	base        []worldModelTriangle
	flame       []worldModelTriangle
	parts       []propPart
	height      float64 // closed/resting authored silhouette, never animated bounds
}

var propModelCache sync.Map // immutable geometry keyed by the complete authored specification

func cachedPropModel(spec character.NPCPropModel) *propModel {
	if m, ok := propModelCache.Load(spec); ok {
		return m.(*propModel)
	}
	m := buildPropModel(spec)
	actual, _ := propModelCache.LoadOrStore(spec, m)
	return actual.(*propModel)
}

func (b *worldModelBuilder) cylinder(a, c worldModelPoint, radius float64, col [3]int, mat int, sides int) {
	dx, dy, dz := c.x-a.x, c.y-a.y, c.z-a.z
	length := math.Sqrt(dx*dx + dy*dy + dz*dz)
	dx, dy, dz = dx/length, dy/length, dz/length
	ux, uy, uz := -dy, dx, 0.0
	if math.Abs(dz) > .95 {
		ux, uy, uz = 1, 0, 0
	}
	n := math.Sqrt(ux*ux + uy*uy + uz*uz)
	ux, uy, uz = ux/n, uy/n, uz/n
	vx, vy, vz := dy*uz-dz*uy, dz*ux-dx*uz, dx*uy-dy*ux
	point := func(p worldModelPoint, angle float64) worldModelPoint {
		co, si := radius*math.Cos(angle), radius*math.Sin(angle)
		return worldModelPoint{p.x + ux*co + vx*si, p.y + uy*co + vy*si, p.z + uz*co + vz*si}
	}
	for i := 0; i < sides; i++ {
		t, u := float64(i)*2*math.Pi/float64(sides), float64(i+1)*2*math.Pi/float64(sides)
		p, q, r, s := point(a, t), point(a, u), point(c, u), point(c, t)
		b.quad(p, q, r, s, col, mat, 1)
		b.triangle(a, q, p, col, mat, 1)
		b.triangle(c, s, r, col, mat, 1)
	}
}

func buildPropModel(spec character.NPCPropModel) *propModel {
	b := &worldModelBuilder{}
	m := &propModel{}
	switch spec.Shape {
	case "thieves_guild":
		return buildGuildModel(spec)
	case "crates":
		m.height = 1
		for index, box := range [][5]float64{{-.28, 0, 0, .48, -.14}, {.26, .04, 0, .5, .12}, {-.11, .075, .50, .49, -.28}} {
			x, y, z, size, yaw := box[0], box[1], box[2], box[3], box[4]
			body := &worldModelBuilder{}
			body.crateShell(x, y, z, size, spec.Body, spec.Trim)
			lid := &worldModelBuilder{}
			lid.propBox(x, y, z+size-.035, size*.5, size*.43, .035, 0, spec.Body, worldWood)
			for _, side := range []float64{-1, 1} {
				lid.propBox(x+side*size*.40, y, z+size, size*.04, size*.43, .026, 0, spec.Trim, 0)
			}
			if index == 2 {
				faces := append(body.triangles, lid.triangles...)
				rotatePropGeometry(faces, x, y, yaw)
				m.parts = append(m.parts, propPart{faces: faces, pivot: worldModelPoint{x, y, z + size*.5}, yaw: yaw, size: size, falling: true})
			} else {
				rotatePropGeometry(body.triangles, x, y, yaw)
				m.base = append(m.base, body.triangles...)
				rotatePropGeometry(lid.triangles, x, y, yaw)
				side := -1.0
				if index == 1 {
					side = 1
				}
				m.parts = append(m.parts, propPart{faces: lid.triangles, pivot: worldModelPoint{x, y, z + size - .0175}, yaw: yaw, size: size, side: side})
			}
		}
	case "campfire":
		m.charredWood = true
		m.height = 1
		b.disc(0, 0, .43, .003, .018, [3]int{39, 34, 29}, worldStone)
		for i := 0; i < 13; i++ {
			a := float64(i) * math.Pi * 2 / 13
			r := .41 + .018*math.Sin(float64(i)*2.7)
			b.propRock(r*math.Cos(a), r*math.Sin(a), .085+.015*math.Sin(float64(i)), .11+.02*math.Cos(a*3), float64(i), spec.Trim)
		}
		b.disc(0, 0, .27, .024, .016, [3]int{210, 53, 12}, worldEmber)
		for i := 0; i < 5; i++ {
			a := float64(i)*math.Pi/2.5 + .24
			co, si := math.Cos(a), math.Sin(a)
			z := .09 + float64(i%2)*.08
			b.cylinder(worldModelPoint{-.29 * co, -.29 * si, z}, worldModelPoint{.29 * co, .29 * si, z + .035}, .070, spec.Body, worldWood, 10)
			b.cylinder(worldModelPoint{-.295 * co, -.295 * si, z}, worldModelPoint{-.303 * co, -.303 * si, z}, .064, [3]int{165, 102, 43}, worldWood, 10)
		}
		m.base = b.triangles
		f := &worldModelBuilder{}
		f.propBox(0, 0, .17, .33, .33, .83, 0, [3]int{}, worldFlame)
		m.flame = f.triangles
	}
	return m
}

func (b *worldModelBuilder) crateShell(x, y, z, size float64, wood, trim [3]int) {
	b.propBox(x, y, z, size*.5, size*.43, .027, 0, [3]int{45, 29, 17}, worldWood)
	for _, side := range []float64{-1, 1} {
		b.propBox(x, y+side*size*.408, z, size*.5, size*.022, size, 0, wood, worldWood)
		b.propBox(x+side*size*.48, y, z, size*.02, size*.408, size, 0, wood, worldWood)
		b.cylinder(worldModelPoint{x + side*size*.514, y - size*.33, z + .07}, worldModelPoint{x + side*size*.514, y + size*.33, z + size - .08}, size*.041, wood, worldWood, 4)
		for _, zz := range []float64{z + .025, z + size - .07} {
			b.propBox(x+side*size*.51, y, zz, size*.024, size*.43, size*.10, 0, wood, worldWood)
		}
		for _, zz := range []float64{z + .025, z + size - .07} {
			b.propBox(x, y+side*size*.443, zz, size*.52, size*.024, size*.10, 0, wood, worldWood)
		}
		for _, xx := range []float64{x - size*.42, x + size*.42} {
			b.propBox(xx, y+side*size*.456, z, size*.054, size*.016, size, 0, trim, 0)
		}
		b.cylinder(worldModelPoint{x - size*.36, y + side*size*.457, z + .07}, worldModelPoint{x + size*.36, y + side*size*.457, z + size - .08}, size*.041, wood, worldWood, 4)
	}
}

// Closed solids need undersides too: a raised lid exposes its bottom.
func (b *worldModelBuilder) propBox(x, y, z, rx, ry, h, angle float64, col [3]int, mat int) {
	p := func(u, v, w float64) worldModelPoint {
		return worldModelPoint{x + u*math.Cos(angle) - v*math.Sin(angle), y + u*math.Sin(angle) + v*math.Cos(angle), z + w}
	}
	v := [8]worldModelPoint{p(-rx, -ry, 0), p(rx, -ry, 0), p(rx, ry, 0), p(-rx, ry, 0), p(-rx, -ry, h), p(rx, -ry, h), p(rx, ry, h), p(-rx, ry, h)}
	for _, f := range [][4]int{{4, 5, 6, 7}, {3, 2, 1, 0}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}} {
		b.quad(v[f[0]], v[f[1]], v[f[2]], v[f[3]], col, mat, 1)
	}
}

func (b *worldModelBuilder) propRock(x, y, r, h, seed float64, col [3]int) {
	point := func(i, level int) worldModelPoint {
		a := float64(i)*math.Pi/4 + seed*.3
		radius := r * (.86 + .14*math.Sin(float64(i)*3+seed))
		z := .012
		if level == 1 {
			z += h * .55
		} else if level == 2 {
			radius *= .61
			z += h * (.91 + .09*math.Sin(float64(i)+seed))
		}
		return worldModelPoint{x + radius*math.Cos(a), y + radius*math.Sin(a)*.82, z}
	}
	for i := 0; i < 8; i++ {
		for level := 0; level < 2; level++ {
			b.quad(point(i, level), point(i+1, level), point(i+1, level+1), point(i, level+1), col, worldStone, 1)
		}
		b.triangle(worldModelPoint{x, y, h + .02}, point(i, 2), point(i+1, 2), col, worldStone, 1)
	}
}

func rotatePropGeometry(faces []worldModelTriangle, x, y, yaw float64) {
	co, si := math.Cos(yaw), math.Sin(yaw)
	point := func(p worldModelPoint) worldModelPoint {
		p.x, p.y = x+(p.x-x)*co-(p.y-y)*si, y+(p.x-x)*si+(p.y-y)*co
		return p
	}
	for i := range faces {
		f := &faces[i]
		for j, p := range f.points {
			f.points[j] = point(p)
		}
		f.center = point(f.center)
		f.normal.x, f.normal.y = f.normal.x*co-f.normal.y*si, f.normal.x*si+f.normal.y*co
	}
}
