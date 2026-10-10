package game

import (
	"math"
	"ugataima/internal/character"
)

const guildPlasterMaterial = 11
const guildWindowMaterial = 12

// The same static model is used for scene triangles and picking. Dimensions
// are relative to the authored size class, like the other procedural props.
func buildGuildModel(spec character.NPCPropModel) *propModel {
	b := &worldModelBuilder{}
	stone, wood := spec.Body, spec.Trim
	roof := [3]int{43, 53, 70}
	black := [3]int{25, 25, 26}
	gold := [3]int{192, 136, 52}
	// The doorway faces -Y. A little depth keeps the building readable when
	// approached from either side; yaw_degrees rotates every part together.
	b.propBox(0, 0, 0, .48, .36, .09, 0, stone, worldStone)
	b.guildWallShell(.43, .30, .09, .84, stone, worldStone)
	b.guildWallShell(.45, .32, .84, 1.44, [3]int{183, 166, 132}, guildPlasterMaterial)
	for _, z := range []float64{.80, .87, 1.41} {
		for i := 0; i < 12; i++ {
			x := -.47 + (float64(i)+.5)*.94/12
			for _, y := range []float64{-.33, .33} {
				b.propBox(x, y, z, .47/12, .01, .055, 0, wood, worldWood)
			}
		}
	}
	for _, x := range []float64{-.43, 0, .43} {
		for _, y := range []float64{-.33, .33} {
			b.propBox(x, y, .87, .025, .01, .55, 0, wood, worldWood)
		}
	}
	// Bounded courses on all four walls. Chamfered face geometry creates real
	// mortar recesses and edge light, without noisy displacement in the shader.
	for face := 0; face < 4; face++ {
		wall := &worldModelBuilder{}
		half := .43
		if face%2 == 1 {
			half = .30
		}
		for row := 0; row < 6; row++ {
			z := .102 + float64(row)*.113
			for col := 0; col < 7; col++ {
				x0 := -half + float64(col)*.145 - float64(row%2)*.0725
				x1 := math.Min(half, x0+.137)
				x0 = math.Max(-half, x0)
				if x1-x0 < .025 {
					continue
				}
				c := stone
				for i := range c {
					c[i] += ((col+row*3)%5 - 2) * 5
				}
				for _, rect := range guildWallRects(face, x0, x1, z, z+.104) {
					wall.guildStone(rect[0], rect[1], -.308, rect[2], rect[3], c)
				}
			}
		}
		if face%2 == 1 {
			for i := range wall.triangles {
				for j := range wall.triangles[i].points {
					wall.triangles[i].points[j].y -= .13
				}
			}
		}
		// Rebuild normals/UVs after rotating the facade plane.
		angle := float64(face) * math.Pi / 2
		for _, f := range wall.triangles {
			p := f.points
			for i := range p {
				p[i].x, p[i].y = p[i].x*math.Cos(angle)-p[i].y*math.Sin(angle), p[i].x*math.Sin(angle)+p[i].y*math.Cos(angle)
			}
			b.triangle(p[0], p[1], p[2], f.rgb, f.material, 1)
		}
	}
	// Door, jambs, iron straps and paired lit windows.
	b.propBox(0, -.327, .10, .115, .02, .55, 0, wood, worldWood)
	for _, x := range []float64{-.14, .14} {
		b.propBox(x, -.338, .09, .022, .023, .60, 0, stone, worldStone)
	}
	b.propBox(0, -.34, .67, .164, .024, .05, 0, stone, worldStone)
	for _, z := range []float64{.19, .55} {
		b.propBox(0, -.352, z, .105, .009, .024, 0, black, 0)
	}
	b.propBox(.072, -.364, .36, .012, .012, .025, 0, gold, 0)
	// Recessed door planks, threshold steps, strap studs and iron ring handle.
	for i := 0; i < 7; i++ {
		b.propBox(-.095+float64(i)*.0315, -.350, .108, .0145, .004, .53, 0, wood, worldWood)
	}
	for _, z := range []float64{.19, .55} {
		for _, x := range []float64{-.088, -.05, .05, .088} {
			b.propBox(x, -.364, z+.005, .003, .004, .009, 0, gold, 0)
		}
	}
	b.guildRing(.048, -.378, .365, .025, .005, black)
	for i := 0; i < 3; i++ {
		b.propBox(0, -.365-float64(2-i)*.055, float64(i)*.029, .18-float64(i)*.012, .075, .030, 0, stone, worldStone)
	}
	for _, x := range []float64{-.30, .30} {
		// Corbels carry the projecting upper floor; braces stop at the windows.
		b.cylinder(worldModelPoint{x * 1.38, -.327, .70}, worldModelPoint{x * 1.38, -.369, .84}, .024, wood, worldWood, 4)
		b.cylinder(worldModelPoint{x * .37, -.35, .92}, worldModelPoint{x * .73, -.35, 1.33}, .018, wood, worldWood, 4)
	}
	for _, window := range guildWindows() {
		trim := wood
		if window.lattice {
			trim = stone
		}
		b.guildWindow(window.x, -.35, window.z, window.half, window.height, wood, trim, window.lattice)
	}
	// Main pitched roof, front gable, and a stone chimney.
	a := worldModelPoint{-.50, -.37, 1.44}
	c := worldModelPoint{.50, -.37, 1.44}
	d := worldModelPoint{-.50, 0, 1.92}
	e := worldModelPoint{.50, 0, 1.92}
	f := worldModelPoint{-.50, .37, 1.44}
	h := worldModelPoint{.50, .37, 1.44}
	// Close the underside of the overhang. The roof extends beyond the walls,
	// so its downward-facing soffit must remain visible from ground level.
	b.quad(a, f, h, c, wood, worldWood, 1)
	// Overlapping slate courses are shallow quads, not hundreds of closed
	// boxes. They remain cached with the model and readable from both sides.
	for _, side := range []float64{-1, 1} {
		for row := 0; row < 8; row++ {
			y0 := float64(row) * .04625
			y1 := y0 + .04625
			for col := 0; col < 12; col++ {
				x0 := math.Max(-.5, -.5+float64(col)*.091-float64(row%2)*.0455)
				x1 := math.Min(.5, -.5+float64(col+1)*.091-float64(row%2)*.0455)
				if x1 <= x0 {
					continue
				}
				shade := 12 + ((col*3+row)%5)*4
				c := [3]int{roof[0] + shade, roof[1] + shade, roof[2] + shade}
				v := [4]worldModelPoint{{x0, side * y0, 1.928 - y0*.48/.37}, {x1, side * y0, 1.928 - y0*.48/.37}, {x1, side * y1, 1.928 - y1*.48/.37}, {x0, side * y1, 1.928 - y1*.48/.37}}
				if side < 0 {
					v[1], v[3] = v[3], v[1]
				}
				b.quad(v[0], v[1], v[2], v[3], c, worldStone, 1)
			}
		}
	}
	b.triangle(a, d, f, wood, worldWood, 1)
	b.triangle(c, h, e, wood, worldWood, 1)
	p := worldModelPoint{0, -.39, 1.93}
	l := worldModelPoint{-.26, -.39, 1.45}
	r := worldModelPoint{.26, -.39, 1.45}
	b.triangle(l, r, p, [3]int{151, 131, 96}, worldWood, 1)
	b.quad(l, p, worldModelPoint{0, 0, 1.93}, worldModelPoint{-.26, 0, 1.45}, roof, worldStone, 1)
	b.quad(p, r, worldModelPoint{.26, 0, 1.45}, worldModelPoint{0, 0, 1.93}, roof, worldStone, 1)
	for _, side := range []float64{-1, 1} {
		for row := 0; row < 6; row++ {
			x0 := float64(row) * .0433
			x1 := x0 + .04
			for col := 0; col < 5; col++ {
				y0 := -.397 + float64(col)*.079
				y1 := y0 + .073
				c := [3]int{57 + row%3*5, 68 + col%3*5, 86 + row%3*5}
				v := [4]worldModelPoint{{side * x0, y0, 1.943 - x0*.48/.26}, {side * x1, y0, 1.943 - x1*.48/.26}, {side * x1, y1, 1.943 - x1*.48/.26}, {side * x0, y1, 1.943 - x0*.48/.26}}
				if side < 0 {
					v[1], v[3] = v[3], v[1]
				}
				b.quad(v[0], v[1], v[2], v[3], c, worldStone, 1)
			}
		}
	}
	for _, side := range []float64{-1, 1} {
		b.propBox(0, side*.373, 1.423, .51, .013, .034, 0, wood, worldWood)
		b.cylinder(worldModelPoint{side * .5, -.375, 1.44}, worldModelPoint{side * .5, 0, 1.93}, .022, wood, worldWood, 4)
	}
	b.cylinder(worldModelPoint{-.51, 0, 1.943}, worldModelPoint{.51, 0, 1.943}, .018, roof, worldStone, 6)
	b.propBox(0, -.405, 1.45, .014, .012, .45, 0, wood, worldWood)
	for _, side := range []float64{-1, 1} {
		b.cylinder(worldModelPoint{side * .22, -.407, 1.47}, worldModelPoint{side * .025, -.407, 1.78}, .012, wood, worldWood, 4)
	}
	for _, side := range []float64{-1, 1} {
		b.cylinder(worldModelPoint{side * .27, -.407, 1.44}, p, .015, wood, worldWood, 4)
	}
	b.propBox(.34, .13, 1.65, .065, .065, .32, 0, stone, worldStone)
	b.propBox(.34, .13, 1.96, .077, .077, .04, 0, stone, worldStone)
	b.propBox(.34, .13, 2, .05, .05, .002, 0, black, worldStone)
	for i := 0; i < 4; i++ {
		b.propBox(.34, .13, 1.66+float64(i)*.073, .067, .067, .008, 0, [3]int{71, 64, 54}, worldStone)
	}
	// Raised crossed-key sign: two rigid keys, separate from the facade.
	b.propBox(0, -.374, .76, .105, .016, .14, 0, black, worldWood)
	for _, x := range []float64{-.11, .11} {
		b.propBox(x, -.394, .75, .006, .008, .16, 0, gold, 0)
	}
	for _, z := range []float64{.75, .90} {
		b.propBox(0, -.394, z, .11, .008, .008, 0, gold, 0)
	}
	for _, side := range []float64{-1, 1} {
		b.guildRing(side*.057, -.417, .877, .020, .0045, gold)
		b.cylinder(worldModelPoint{side * .057, -.4, .875}, worldModelPoint{-side * .061, -.4, .785}, .009, gold, 0, 6)
		b.propBox(-side*.060, -.4, .779, .019, .011, .015, 0, gold, 0)
	}
	// Match the broad two-storey facade rather than a narrow tower. Rebuild
	// normals and material coordinates from the final architectural dimensions.
	wide := &worldModelBuilder{}
	for _, face := range b.triangles {
		p := face.points
		for i := range p {
			p[i].x *= 1.75
			p[i].y *= 1.35
		}
		wide.triangle(p[0], p[1], p[2], face.rgb, face.material, 1)
	}
	return &propModel{height: 2.002, base: wide.triangles}
}

type guildWindowSpec struct {
	x, z, half, height float64
	lattice            bool
}

// One geometry source for the openings and their matching window frames.
func guildWindows() [4]guildWindowSpec {
	return [4]guildWindowSpec{
		{-.30, .27, .046, .26, true}, {.30, .27, .046, .26, true},
		{-.30, .99, .072, .25, false}, {.30, .99, .072, .25, false},
	}
}

// Cut actual holes instead of hiding wall faces behind shallow decoration.
// Only the front facade has windows. Rectangles use x0,x1,z0,z1 order.
func guildWallRects(face int, x0, x1, z0, z1 float64) [][4]float64 {
	rects := [][4]float64{{x0, x1, z0, z1}}
	if face != 0 {
		return rects
	}
	for _, w := range guildWindows() {
		var next [][4]float64
		for _, r := range rects {
			left, right := math.Max(r[0], w.x-w.half), math.Min(r[1], w.x+w.half)
			bottom, top := math.Max(r[2], w.z+.018), math.Min(r[3], w.z+w.height)
			if left >= right || bottom >= top {
				next = append(next, r)
				continue
			}
			for _, s := range [][4]float64{
				{r[0], left, r[2], r[3]}, {right, r[1], r[2], r[3]},
				{left, right, r[2], bottom}, {left, right, top, r[3]},
			} {
				if s[0] < s[1] && s[2] < s[3] {
					next = append(next, s)
				}
			}
		}
		rects = next
	}
	return rects
}

// A single beveled face is enough for shallow masonry relief.
func (b *worldModelBuilder) guildStone(x0, x1, y, z0, z1 float64, col [3]int) {
	bevelX, bevelZ := math.Min(.007, (x1-x0)*.2), math.Min(.008, (z1-z0)*.2)
	outer := [4]worldModelPoint{{x0, y, z0}, {x1, y, z0}, {x1, y, z1}, {x0, y, z1}}
	inner := [4]worldModelPoint{{x0 + bevelX, y - .008, z0 + bevelZ}, {x1 - bevelX, y - .008, z0 + bevelZ}, {x1 - bevelX, y - .008, z1 - bevelZ}, {x0 + bevelX, y - .008, z1 - bevelZ}}
	b.quad(inner[0], inner[1], inner[2], inner[3], col, worldStone, 1)
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		b.quad(outer[i], outer[j], inner[j], inner[i], col, worldStone, 1)
	}
}

// Small panels preserve the renderer's painter ordering at oblique angles.
// Large backing triangles otherwise sort in front of near-side window trim.
func (b *worldModelBuilder) guildWallShell(rx, ry, z0, z1 float64, col [3]int, mat int) {
	for face := 0; face < 4; face++ {
		half, depth := rx, ry
		if face%2 == 1 {
			half, depth = ry, rx
		}
		a := float64(face) * math.Pi / 2
		point := func(x, z float64) worldModelPoint {
			return worldModelPoint{x*math.Cos(a) + depth*math.Sin(a), x*math.Sin(a) - depth*math.Cos(a), z}
		}
		for row := 0; row < 8; row++ {
			for c := 0; c < 12; c++ {
				x0, x1 := -half+2*half*float64(c)/12, -half+2*half*float64(c+1)/12
				lo, hi := z0+(z1-z0)*float64(row)/8, z0+(z1-z0)*float64(row+1)/8
				for _, rect := range guildWallRects(face, x0, x1, lo, hi) {
					b.quad(point(rect[0], rect[2]), point(rect[1], rect[2]), point(rect[1], rect[3]), point(rect[0], rect[3]), col, mat, 1)
				}
			}
		}
	}
}

func (b *worldModelBuilder) guildRing(x, y, z, r, thickness float64, col [3]int) {
	for i := 0; i < 12; i++ {
		a, c := float64(i)*math.Pi/6, float64(i+1)*math.Pi/6
		b.cylinder(worldModelPoint{x + r*math.Cos(a), y, z + r*math.Sin(a)}, worldModelPoint{x + r*math.Cos(c), y, z + r*math.Sin(c)}, thickness, col, 0, 4)
	}
}

func (b *worldModelBuilder) guildWindow(x, y, z, half, height float64, wood, trim [3]int, lattice bool) {
	// Real facade openings and separate panes avoid coplanar backing faces
	// painting over the glass at oblique angles. These are opaque emissive panes,
	// not transparent surfaces requiring a second sorting pass.
	for row := 0; row < 6; row++ {
		for col := 0; col < 4; col++ {
			x0, x1 := x-half+float64(col)*half/2, x-half+float64(col+1)*half/2
			z0, z1 := z+.018+float64(row)*(height-.018)/6, z+.018+float64(row+1)*(height-.018)/6
			b.quad(worldModelPoint{x0, y - .025, z0}, worldModelPoint{x1, y - .025, z0}, worldModelPoint{x1, y - .025, z1}, worldModelPoint{x0, y - .025, z1}, [3]int{226, 143, 38}, guildWindowMaterial, 1)
		}
	}
	for _, side := range []float64{-1, 1} {
		b.propBox(x+side*(half+.012), y-.030, z, .012, .012, height+.025, 0, trim, worldWood)
	}
	for _, zz := range []float64{z, z + height} {
		b.propBox(x, y-.036, zz, half+.026, .024, .024, 0, trim, worldWood)
	}
	if lattice {
		for i := 0; i < 3; i++ {
			zz := z + .025 + float64(i)*(height-.034)/3
			for _, s := range []float64{-1, 1} {
				b.cylinder(worldModelPoint{x - s*half, y - .031, zz}, worldModelPoint{x + s*half, y - .031, zz + (height-.034)/3}, .0035, wood, worldWood, 4)
			}
		}
	} else {
		b.propBox(x, y-.032, z+.024, .006, .007, height-.027, 0, wood, worldWood)
		b.propBox(x, y-.032, z+height*.54, half, .007, .014, 0, wood, worldWood)
	}
}
