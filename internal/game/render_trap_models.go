package game

import "math"

var trapIron = [3]int{95, 105, 118}
var trapSteel = [3]int{195, 211, 225}
var trapBrass = [3]int{180, 120, 53}
var trapCyan = [3]int{59, 209, 247}

func polarTrap(r, a, z float64) trapPoint { return trapPoint{r * math.Cos(a), r * math.Sin(a), z} }
func (b *trapModelBuilder) ring(radius, width, z, height, start, span float64, n int, col [3]int, mat int, transform func(trapPoint) trapPoint) {
	point := func(r, a, h float64) trapPoint {
		p := polarTrap(r, a, h)
		if transform != nil {
			return transform(p)
		}
		return p
	}
	for i := 0; i < n; i++ {
		a, c := start+span*float64(i)/float64(n), start+span*float64(i+1)/float64(n)
		o0, o1, i0, i1 := point(radius, a, z+height), point(radius, c, z+height), point(radius-width, a, z+height), point(radius-width, c, z+height)
		b.quad(o0, o1, i1, i0, col, mat, 1)
		lo0, lo1, li0, li1 := point(radius, a, z), point(radius, c, z), point(radius-width, a, z), point(radius-width, c, z)
		b.quad(lo1, lo0, li0, li1, col, mat, 1)
		b.quad(lo0, lo1, o1, o0, col, mat, 1)
		b.quad(li1, li0, i0, i1, col, mat, 1)
	}
	if span < 2*math.Pi-.001 {
		for i, a := range []float64{start, start + span} {
			o, in := point(radius, a, z), point(radius-width, a, z)
			ot, it := point(radius, a, z+height), point(radius-width, a, z+height)
			if i == 0 {
				b.quad(o, ot, it, in, col, mat, 1)
			} else {
				b.quad(in, it, ot, o, col, mat, 1)
			}
		}
	}
}
func (b *trapModelBuilder) disc(x, y, r, z, h float64, col [3]int, mat int) {
	// Small rivets need only a few facets, not the hub's full tessellation.
	n := 8
	if r > .14 {
		n = 24
	} else if r > .055 {
		n = 16
	} else if r > .025 {
		n = 10
	}
	point := func(a, height float64) trapPoint { return trapPoint{x + r*math.Cos(a), y + r*math.Sin(a), height} }
	center := trapPoint{x, y, z + h}
	for i := 0; i < n; i++ {
		a, c := float64(i)*2*math.Pi/float64(n), float64(i+1)*2*math.Pi/float64(n)
		aa, cc := point(a, z+h), point(c, z+h)
		b.triangle(center, aa, cc, col, mat, 1)
		b.quad(point(a, z), point(c, z), cc, aa, col, mat, 1)
	}
}
func (b *trapModelBuilder) box(x, y, z, rx, ry, h, angle float64, col [3]int, mat int) {
	p := func(a, c, d float64) trapPoint {
		return trapPoint{x + a*math.Cos(angle) - c*math.Sin(angle), y + a*math.Sin(angle) + c*math.Cos(angle), z + d}
	}
	a, c, d, e := p(-rx, -ry, 0), p(rx, -ry, 0), p(rx, ry, 0), p(-rx, ry, 0)
	aa, cc, dd, ee := p(-rx, -ry, h), p(rx, -ry, h), p(rx, ry, h), p(-rx, ry, h)
	b.quad(aa, cc, dd, ee, col, mat, 1)
	b.quad(c, a, aa, cc, col, mat, 1)
	b.quad(d, c, cc, dd, col, mat, 1)
	b.quad(e, d, dd, ee, col, mat, 1)
	b.quad(a, e, ee, aa, col, mat, 1)
}
func (b *trapModelBuilder) cone(x, y, r0, r1, z0, z1 float64, col [3]int, mat int) {
	for i := 0; i < 20; i++ {
		a, c := float64(i)*math.Pi/10, float64(i+1)*math.Pi/10
		p := func(r, t, z float64) trapPoint { return trapPoint{x + r*math.Cos(t), y + r*math.Sin(t), z} }
		b.quad(p(r0, a, z0), p(r0, c, z0), p(r1, c, z1), p(r1, a, z1), col, mat, 1)
	}
}

// Open jaws share two fixed hinge pins. Closing rotates each jaw as one rigid
// piece, including its teeth; the pan and springs remain planted on the tile.
func (b *trapModelBuilder) bear() {
	b.ring(.245, .026, .018, .025, 0, 2*math.Pi, 24, trapIron, 0, nil)
	b.box(0, 0, .023, .24, .035, .024, 0, trapIron, 0)
	b.disc(0, 0, .11, .047, .014, b.accent, 1)
	b.ring(.108, .01, .061, .004, 0, 2*math.Pi, 20, trapSteel, 2, nil)
	for _, side := range []float64{-1, 1} {
		b.box(side*.32, 0, .016, .065, .22, .027, 0, trapIron, 0)
		b.box(side*.32, 0, .043, .044, .17, .014, 0, trapSteel, 2)
		b.disc(side*.255, 0, .043, .06, .055, b.accent, 1)
		b.box(side*.255, 0, .116, .026, .005, .002, 0, trapIron, 0)
		close := .12
		if b.progress >= 0 {
			close += 1.3 * math.Min(1, b.progress*8)
		}
		transform := func(p trapPoint) trapPoint {
			return trapPoint{p.x, p.y*math.Cos(close) - side*p.z*math.Sin(close), .04 + side*p.y*math.Sin(close) + p.z*math.Cos(close)}
		}
		start := 0.0
		if side < 0 {
			start = math.Pi
		}
		b.ring(.29, .045, 0, .032, start, math.Pi, 18, trapIron, 0, transform)
		b.ring(.287, .008, .032, .006, start, math.Pi, 18, trapSteel, 2, transform)
		for i := 0; i < 9; i++ {
			angle := start + (float64(i)+.5)*math.Pi/9
			a, c := polarTrap(.25, angle-.065, .025), polarTrap(.25, angle+.065, .025)
			tip := polarTrap(.19, angle, .102)
			back := polarTrap(.274, angle, .038)
			a, c, tip, back = transform(a), transform(c), transform(tip), transform(back)
			b.triangle(a, c, tip, trapSteel, 2, 1)
			b.triangle(c, back, tip, trapIron, 0, 1)
			b.triangle(back, a, tip, trapSteel, 2, 1)
		}
	}
	// Short, solid oval chain links lie beside the frame, not floating motes.
	for i := 0; i < 3; i++ {
		off := .30 + float64(i)*.055
		b.ring(.036, .009, .012, .014, 0, math.Pi*2, 10, trapIron, 0, func(p trapPoint) trapPoint { p.y = p.y*.62 + off; p.x += .09; return p })
	}
}

// Three hooked scythes are bolted to one rotor. Their raised spine, flat face
// and sharpened bevel remain distinct even at the floor's grazing angle.
func (b *trapModelBuilder) cleave() {
	b.disc(0, 0, .18, .014, .032, trapIron, 0)
	b.ring(.173, .023, .046, .01, 0, 2*math.Pi, 24, trapBrass, 1, nil)
	rotation := 0.0
	if b.progress >= 0 {
		rotation = 7 * (1 - math.Pow(1-b.progress, 3))
	}
	for i := 0; i < 12; i++ {
		a := float64(i) * math.Pi / 6
		b.box(.175*math.Cos(a), .175*math.Sin(a), .025, .027, .016, .027, a, trapBrass, 1)
	}
	for i := 0; i < 3; i++ {
		root := rotation + float64(i)*math.Pi*2/3
		for j := 0; j < 9; j++ {
			point := func(k int, edge float64, z float64) trapPoint {
				t := float64(k) / 9
				r := .105 + .285*t
				// A tapering hooked ribbon, not a straight radial spoke.
				a := root + .95*t*t + edge*.19*math.Sin(math.Pi*(.18+.82*t))
				return polarTrap(r, a, z)
			}
			a, c := point(j, -1, .076), point(j+1, -1, .076)
			d, e := point(j+1, 1, .076), point(j, 1, .076)
			b.quad(a, c, d, e, b.accent, 2, 1)
			b.quad(point(j, -1, .059), point(j+1, -1, .059), c, a, trapIron, 0, 1)
			b.quad(point(j, .6, .077), point(j+1, .6, .077), point(j+1, 1, .061), point(j, 1, .061), trapSteel, 2, 1)
			b.quad(point(j, 1, .061), point(j+1, 1, .061), point(j+1, -1, .059), point(j, -1, .059), trapIron, 0, 1)
		}
		a := root + .04
		b.disc(.13*math.Cos(a), .13*math.Sin(a), .019, .077, .016, trapBrass, 1)
	}
	b.disc(0, 0, .087, .057, .035, trapBrass, 1)
	b.disc(0, 0, .034, .092, .014, trapSteel, 2)
	b.box(0, 0, .107, .025, .005, .001, rotation, trapIron, 0)
}

func (b *trapModelBuilder) stasis() {
	b.disc(0, 0, .245, .012, .024, trapIron, 0)
	b.ring(.238, .016, .036, .005, 0, 2*math.Pi, 32, trapBrass, 1, nil)
	b.ring(.20, .004, .038, .003, 0, 2*math.Pi, 32, b.accent, 5, nil)
	for i := 0; i < 12; i++ {
		a := float64(i) * math.Pi / 6
		b.box(.22*math.Cos(a), .22*math.Sin(a), .039, .009, .003, .003, a, b.accent, 5)
	}
	// Sand drains through an actual waist inside the two glass chambers.
	b.cone(0, 0, .079, .006, .073, .139, b.accent, 4)
	b.disc(0, 0, .004, .137, .113, b.accent, 4)
	b.cone(0, 0, .004, .067, .197, .271, b.accent, 4)
	b.disc(0, 0, .067, .269, .002, b.accent, 4)
	b.cone(0, 0, .096, .016, .071, .188, trapCyan, 3)
	b.cone(0, 0, .016, .096, .188, .305, trapCyan, 3)
	for _, z := range []float64{.049, .304} {
		b.disc(0, 0, .126, z, .015, trapBrass, 1)
		b.ring(.126, .012, z+.015, .005, 0, 2*math.Pi, 24, trapSteel, 2, nil)
	}
	for i := 0; i < 4; i++ {
		a := math.Pi/4 + float64(i)*math.Pi/2
		x, y := .11*math.Cos(a), .11*math.Sin(a)
		b.box(x, y, .062, .009, .009, .245, a, trapBrass, 1)
		b.disc(x, y, .019, .323, .013, trapSteel, 2)
	}
	if b.progress >= 0 {
		for i := 0; i < 3; i++ {
			p := math.Max(0, b.progress-float64(i)*.12)
			b.ring(.14+p*.32, .005, .065+p*.55, .012, 0, 2*math.Pi, 32, b.accent, 5, nil)
		}
	}
}

func (b *trapModelBuilder) blast() {
	b.disc(0, 0, .265, .012, .027, trapIron, 0)
	b.cone(0, 0, .26, .215, .039, .085, trapBrass, 1)
	b.disc(0, 0, .211, .071, .013, b.accent, 6)
	b.ring(.256, .012, .04, .008, 0, 2*math.Pi, 32, trapSteel, 2, nil)
	open := 0.0
	if b.progress >= 0 {
		open = math.Sin(math.Min(1, b.progress*4)*math.Pi/2) * .07
	}
	// Radial vents leave the hot chamber visible between solid lid sectors.
	for i := 0; i < 10; i++ {
		a := float64(i) * math.Pi / 5
		shift := func(p trapPoint) trapPoint {
			p.x += open * math.Cos(a+.2)
			p.y += open * math.Sin(a+.2)
			p.z += open
			return p
		}
		b.ring(.213, .10, .085, .012, a, .43, 3, trapIron, 0, shift)
		b.ring(.213, .012, .097, .004, a, .43, 3, trapBrass, 1, shift)
		b.disc(.238*math.Cos(a), .238*math.Sin(a), .012, .061, .014, trapSteel, 2)
	}
	b.disc(0, 0, .101, .084, .035, trapBrass, 1)
	b.disc(0, 0, .075, .119, .008, trapIron, 0)
	b.ring(.068, .006, .129, .003, 0, 2*math.Pi, 24, b.accent, 5, nil)
	if b.progress >= 0 {
		// Crossed, world-fixed plume planes inherit the same ground projection.
		height := .22 + .85*math.Sin(math.Pi*b.progress)
		for i := 0; i < 3; i++ {
			a := float64(i) * math.Pi / 3
			x, y := .33*math.Cos(a), .33*math.Sin(a)
			b.quad(trapPoint{-x, -y, .10}, trapPoint{x, y, .10}, trapPoint{x * .45, y * .45, height}, trapPoint{-x * .45, -y * .45, height}, b.accent, 7, .86)
		}
	}
}
