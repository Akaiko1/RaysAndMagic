package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"slices"
)

func isFacetedBolt(kind int) bool {
	return kind == spellIce || kind == spellVoidNeedle || kind == spellShadow || kind == spellFireDart
}

type litVolumePoint struct{ x, y, z, u, v float64 }
type litVolumeFace struct {
	p      [3]litVolumePoint
	normal [3]float64
	depth  float64
	rgb    [3]int
}

// All directional volumes share this projection: their visible faces turn with
// the flight axis and hand convergence. Each triangle receives shader lighting.
func (r *Renderer) drawLitVolume(dst *ebiten.Image, x, y, size float64, axis [3]float64, rgb [3]int, alpha float64, kind int, handedness float64) {
	if size <= 0 || alpha <= 0 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	length := math.Sqrt(axis[0]*axis[0] + axis[1]*axis[1] + axis[2]*axis[2])
	if length < .001 {
		axis = [3]float64{0, 0, 1}
		length = 1
	}
	ax, ay, az := axis[0]/length, axis[1]/length, axis[2]/length
	bx, by, bz := az, 0.0, -ax
	bl := math.Hypot(bx, bz)
	if bl < .001 {
		bx, by, bz = 1, 0, 0
		bl = 1
	}
	bx, by, bz = bx/bl, by/bl, bz/bl
	cx, cy, cz := ay*bz-az*by, az*bx-ax*bz, ax*by-ay*bx
	point := func(p [3]float64) litVolumePoint {
		p[1] *= handedness
		return litVolumePoint{ax*p[0] + bx*p[1] + cx*p[2], ay*p[0] + by*p[1] + cy*p[2], az*p[0] + bz*p[1] + cz*p[2], p[0], p[1]}
	}
	halfW, halfH := float64(dst.Bounds().Dx())*.5, float64(dst.Bounds().Dy())*.5
	focal := halfW
	if r.game != nil && r.game.camera != nil {
		focal = r.game.viewFocal()
		halfH = r.game.viewHorizon()
	}
	faces := r.litVolumeFaces[:0]
	face := func(va, vb, vc, origin [3]float64) {
		a, b, c, o := point(va), point(vb), point(vc), point(origin)
		ux, uy, uz := b.x-a.x, b.y-a.y, b.z-a.z
		vx, vy, vz := c.x-a.x, c.y-a.y, c.z-a.z
		nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
		mx, my, mz := (a.x+b.x+c.x)/3, (a.y+b.y+c.y)/3, (a.z+b.z+c.z)/3
		if nx*(mx-o.x)+ny*(my-o.y)+nz*(mz-o.z) < 0 {
			nx, ny, nz = -nx, -ny, -nz
		}
		nl := math.Sqrt(nx*nx + ny*ny + nz*nz)
		if nl < 1e-8 {
			return
		}
		nx, ny, nz = nx/nl, ny/nl, nz/nl
		if nx*((x-halfW)/focal+mx*size/focal)+ny*((y-halfH)/focal+my*size/focal)+nz*(1+mz*size/focal) >= 0 {
			return
		}
		if 1+math.Min(a.z, math.Min(b.z, c.z))*size/focal <= .02 {
			return
		}
		faces = append(faces, litVolumeFace{[3]litVolumePoint{a, b, c}, [3]float64{nx, ny, nz}, mz, rgb})
	}
	if mesh, ok := gauntletModels[kind]; ok {
		for _, f := range mesh {
			rgb = f.rgb
			face(f.a, f.b, f.c, f.origin)
		}
	} else if kind == spellHarm {
		// Chamfered solids form a clenched hand: palm, four folded fingers, a thumb
		// crossing their lower joints, and a short wrist behind the knuckles.
		addBox := func(center, half [3]float64, bevel, roll float64) {
			beveledBoxFaces(center, half, bevel, roll, face)
		}
		addBox([3]float64{-.60, 0, .02}, [3]float64{.42, .61, .35}, .13, 0)
		addBox([3]float64{-1.10, 0, .04}, [3]float64{.23, .40, .27}, .10, 0)
		for k := 0; k < 4; k++ {
			f := float64(k)
			u := .02 - .045*math.Abs(f-1.4)
			across := -.46 + f*.30
			addBox([3]float64{u, across, -.12}, [3]float64{.26, .137, .22}, .075, 0)
			addBox([3]float64{u - .055, across, .18}, [3]float64{.18, .13, .115}, .05, 0)
		}
		addBox([3]float64{-.31, .25, .35}, [3]float64{.19, .36, .14}, .095, -.23)
	} else {
		radius, tail, tip := .17, -1.45, 1.65
		if kind == spellVoidNeedle {
			radius = .14
			tail = -1.7
			tip = 1.9
		}
		if kind == spellShadow {
			radius = .22
			tail = -1.8
			tip = 1.4
		}
		if kind == spellFireDart {
			radius = .20
			tail = -1.8
			tip = 1.25
		}
		rear, nose := [3]float64{tail, 0, 0}, [3]float64{tip, 0, 0}
		for k := 0; k < 6; k++ {
			a0, a1 := float64(k)*math.Pi/3+.22, float64(k+1)*math.Pi/3+.22
			a, b := [3]float64{-.85, math.Cos(a0) * radius, math.Sin(a0) * radius}, [3]float64{-.85, math.Cos(a1) * radius, math.Sin(a1) * radius}
			face(rear, b, a, [3]float64{})
			face(a, b, nose, [3]float64{})
		}
	}
	// Ebitengine's 2D target has no depth buffer. Sorting the visible triangles
	// keeps the thumb and finger joints in front of the palm at every angle.
	slices.SortFunc(faces, func(a, b litVolumeFace) int {
		if a.depth > b.depth {
			return -1
		}
		if a.depth < b.depth {
			return 1
		}
		return 0
	})
	verts, indices := r.standeeVerts[:0], r.standeeIdx[:0]
	for _, f := range faces {
		base := uint16(len(verts))
		for _, p := range f.p {
			perspective := 1 / (1 + p.z*size/focal)
			v := weaponMaterialVertex(halfW+(x-halfW+p.x*size)*perspective, halfH+(y-halfH+p.y*size)*perspective, p.u, p.v, f.rgb, alpha)
			v.Custom0, v.Custom1, v.Custom2, v.Custom3 = float32(f.normal[0]), float32(f.normal[1]), float32(f.normal[2]), float32(kind)+.25
			verts = append(verts, v)
		}
		indices = append(indices, base, base+1, base+2)
	}
	if len(indices) > 0 {
		if r.spellBoltOpts.Uniforms == nil {
			r.spellBoltOpts.Uniforms = map[string]any{"Time": r.spellBoltTime[:]}
		}
		r.spellBoltTime[0] = float32(r.weaponMaterialClock())
		r.spellBoltOpts.Blend = ebiten.BlendSourceOver
		dst.DrawTrianglesShader(verts, indices, r.spellBoltShader, &r.spellBoltOpts)
	}
	r.standeeVerts, r.standeeIdx, r.litVolumeFaces = verts[:0], indices[:0], faces[:0]
}

// Six inset faces, twelve bevel strips and eight corner triangles. The
// same topology is used for each joint; only its proportions and pose differ.
func beveledBoxFaces(center, half [3]float64, bevel, roll float64, face func([3]float64, [3]float64, [3]float64, [3]float64)) {
	transform := func(p [3]float64) [3]float64 {
		return [3]float64{center[0] + p[0], center[1] + p[1]*math.Cos(roll) - p[2]*math.Sin(roll), center[2] + p[1]*math.Sin(roll) + p[2]*math.Cos(roll)}
	}
	emit := func(a, b, c [3]float64) { face(transform(a), transform(b), transform(c), center) }
	for axis := 0; axis < 3; axis++ {
		u, v := (axis+1)%3, (axis+2)%3
		for side := -1; side <= 1; side += 2 {
			var pts [4][3]float64
			for k, p := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
				pts[k][axis] = float64(side) * half[axis]
				pts[k][u], pts[k][v] = p[0]*(half[u]-bevel), p[1]*(half[v]-bevel)
			}
			emit(pts[0], pts[1], pts[2])
			emit(pts[0], pts[2], pts[3])
		}
	}
	// Bevel strips join the inset faces. Corner gaps close with triangles.
	for edge := 0; edge < 3; edge++ {
		u, v := (edge+1)%3, (edge+2)%3
		for su := -1; su <= 1; su += 2 {
			for sv := -1; sv <= 1; sv += 2 {
				var q [4][3]float64
				for i := range q {
					se := 1.0
					if i == 0 || i == 3 {
						se = -1
					}
					q[i][edge] = se * (half[edge] - bevel)
					if i < 2 {
						q[i][u] = float64(su) * half[u]
						q[i][v] = float64(sv) * (half[v] - bevel)
					} else {
						q[i][u] = float64(su) * (half[u] - bevel)
						q[i][v] = float64(sv) * half[v]
					}
				}
				emit(q[0], q[1], q[2])
				emit(q[0], q[2], q[3])
			}
		}
	}
	for sx := -1; sx <= 1; sx += 2 {
		for sy := -1; sy <= 1; sy += 2 {
			for sz := -1; sz <= 1; sz += 2 {
				var q [3][3]float64
				signs := [3]float64{float64(sx), float64(sy), float64(sz)}
				for i := range q {
					for j := 0; j < 3; j++ {
						h := half[j] - bevel
						if i == j {
							h = half[j]
						}
						q[i][j] = signs[j] * h
					}
				}
				emit(q[0], q[1], q[2])
			}
		}
	}
}
