package game

import (
	"math"
	"sync"
)

// Rest geometry of one held model, compiled once per asset. parts lists the
// first point of every part after the first: a bow's strings, a book's covers.
// Parts compile separately, so a moving part never shares a point with another.
// flipped marks faces whose outward normal opposes their winding at rest.
type rangedHeldModel struct {
	mesh    *volumeMesh
	parts   []int
	flipped []bool
}

// A bow or crossbow draws its rest mesh once its farthest moving point (the
// string centre) sits within this many pixels of rest.
const rangedPoseSettledPixels = .25

var rangedHeldModels = struct {
	mu     sync.Mutex
	models map[string]*rangedHeldModel
}{models: map[string]*rangedHeldModel{}}

// rangedHeldRest compiles a held model on first use (prewarm or first shot).
// Memory is one rest mesh per asset; poses are never cached.
func rangedHeldRest(key string, a rangedHeldAsset) *rangedHeldModel {
	rangedHeldModels.mu.Lock()
	defer rangedHeldModels.mu.Unlock()
	if model := rangedHeldModels.models[key]; model != nil {
		return model
	}
	parts := a.build()
	if a.limbs != nil {
		parts = append(parts, a.limbs.restStrings()...)
	}
	model := &rangedHeldModel{mesh: &volumeMesh{}}
	for i, raw := range parts {
		if i > 0 {
			model.parts = append(model.parts, len(model.mesh.points))
		}
		part := compileVolumeMesh(raw)
		base := len(model.mesh.points)
		model.mesh.points = append(model.mesh.points, part.points...)
		for _, f := range part.faces {
			for k := range f.vertices {
				f.vertices[k] += base
			}
			model.mesh.faces = append(model.mesh.faces, f)
		}
	}
	for _, f := range model.mesh.faces {
		v := f.vertices
		n := triangleNormal(model.mesh.points[v[0]], model.mesh.points[v[1]], model.mesh.points[v[2]])
		model.flipped = append(model.flipped, n[0]*f.normal[0]+n[1]*f.normal[1]+n[2]*f.normal[2] < 0)
	}
	rangedHeldModels.models[key] = model
	return model
}

func onePart(build func() []modelFace) func() [][]modelFace {
	return func() [][]modelFace { return [][]modelFace{build()} }
}

// Per-frame scratch: one posed copy of a rest model, rebuilt in place. Draws
// are sequential, so every hero's model shares it.
type rangedPoser struct {
	mesh     volumeMesh
	surfaces []modelSurface
}

// pose moves every rest point (place receives its part), recomputes each face
// normal from the moved triangle with its rest winding, and keeps rest UVs, so
// an inlay never slides along a bending limb. Posed faces are flat: an
// authored surface keeps its UVs, its normal follows the moved face.
func (s *rangedPoser) pose(model *rangedHeldModel, place func(part int, p [3]float64) [3]float64) *volumeMesh {
	rest, m := model.mesh, &s.mesh
	m.points, m.uv, m.faces = m.points[:0], m.uv[:0], m.faces[:0]
	part := 0
	for i, p := range rest.points {
		for part < len(model.parts) && i >= model.parts[part] {
			part++
		}
		m.points = append(m.points, place(part, p))
		m.uv = append(m.uv, [2]float64{p[0], p[1]})
	}
	surfaces := 0
	for _, f := range rest.faces {
		if f.surface != nil {
			surfaces++
		}
	}
	if cap(s.surfaces) < surfaces {
		s.surfaces = make([]modelSurface, 0, surfaces) // pointers below must not move
	}
	s.surfaces = s.surfaces[:0]
	for i, f := range rest.faces {
		v := f.vertices
		n := triangleNormal(m.points[v[0]], m.points[v[1]], m.points[v[2]])
		if length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2]); length > 1e-12 {
			if model.flipped[i] {
				length = -length
			}
			f.normal = [3]float64{n[0] / length, n[1] / length, n[2] / length}
		}
		if f.surface != nil {
			s.surfaces = append(s.surfaces, modelSurface{normals: [3][3]float64{f.normal, f.normal, f.normal}, uv: f.surface.uv})
			f.surface = &s.surfaces[len(s.surfaces)-1]
		}
		m.faces = append(m.faces, f)
	}
	return m
}

// Elastic limbs shared by every bow and crossbow. Bows use X for limb height
// and Y for the shot axis; a crossbow's prod lies across its stock in the same
// axes. Displacement grows with the square of the distance past root and peaks
// at extent; every fitting follows its limb. The strings are parts 1 (left)
// and 2 (right): their limb end follows the bend, their centre the draw.
type rangedLimbs struct {
	root, extent float64    // |x| where the bend starts and where it peaks
	inward, back float64    // limb-end travel at full draw: toward the grip, toward the archer
	nock         [3]float64 // right string end at rest; the left mirrors it
	center       [3]float64 // string centre at rest
	draw         float64    // string centre travel toward the archer at full draw
	string       [3]int
}

func bowLimbs(extent float64, nock [3]float64, string [3]int) *rangedLimbs {
	return &rangedLimbs{extent: extent, inward: .075, back: .14, nock: nock, center: [3]float64{0, -.27, 0}, draw: .90, string: string}
}

// The prod's arms flex straight back; the stock between them stays rigid.
func crossbowLimbs(string [3]int) *rangedLimbs {
	return &rangedLimbs{root: .12, extent: 1.06, back: .14, nock: [3]float64{1.17, .12, .02}, center: [3]float64{0, .12, .13}, draw: .85, string: string}
}

// reach is the farthest any point moves at full draw, in model units: the
// string centre, or a limb end.
func (l *rangedLimbs) reach() float64 { return max(l.draw, math.Hypot(l.inward, l.back)) }

func (l *rangedLimbs) bend(p [3]float64, pull float64) [3]float64 {
	t := math.Min(1, math.Max(0, (math.Abs(p[0])-l.root)/(l.extent-l.root)))
	p[0] -= math.Copysign(l.inward*pull*t*t, p[0])
	p[1] -= l.back * pull * t * t
	return p
}

func (l *rangedLimbs) restStrings() [][]modelFace {
	var out [][]modelFace
	for _, side := range []float64{-1, 1} {
		m := weaponMeshBuilder{}
		nock := l.nock
		nock[0] *= side
		m.rod(nock, l.center, .005, .005, l.string, meshGrip)
		out = append(out, m.faces)
	}
	return out
}

// placer poses the limbs and both strings for one draw weight.
func (l *rangedLimbs) placer(pull float64) func(part int, p [3]float64) [3]float64 {
	return func(part int, p [3]float64) [3]float64 {
		if part == 0 {
			return l.bend(p, pull)
		}
		nock := l.nock
		nock[0] *= float64(2*part - 3)
		bent := l.bend(nock, pull)
		span := [3]float64{l.center[0] - nock[0], l.center[1] - nock[1], l.center[2] - nock[2]}
		s := ((p[0]-nock[0])*span[0] + (p[1]-nock[1])*span[1] + (p[2]-nock[2])*span[2]) / (span[0]*span[0] + span[1]*span[1] + span[2]*span[2])
		s = math.Min(1, math.Max(0, s))
		for k := range p {
			p[k] += (1 - s) * (bent[k] - nock[k])
		}
		p[1] -= s * l.draw * pull
		return p
	}
}

// The book's two covers (parts 0 and 1, left and right) hinge about the spine
// (part 2, local Y) by the angle whose cosine and sine are given.
func bookHinge(part int, p [3]float64, ca, sa float64) [3]float64 {
	if part == 2 {
		return p
	}
	if part == 0 {
		sa = -sa
	}
	return [3]float64{p[0]*ca + p[2]*sa, p[1], -p[0]*sa + p[2]*ca}
}

// heldMesh is this frame's geometry for a model drawn size pixels per unit:
// the cached rest mesh while nothing visibly moves, otherwise the shared
// scratch posed by the limbs or covers.
func (r *Renderer) heldMesh(key string, a rangedHeldAsset, pose, size float64) *volumeMesh {
	model := rangedHeldRest(key, a)
	switch {
	case a.limbs != nil && pose*a.limbs.reach()*size >= rangedPoseSettledPixels:
		return r.rangedPoser.pose(model, a.limbs.placer(pose))
	case a.kind == heldBook:
		// open 1 lies nearly flat; 0 nearly closes the covers.
		angle := 1.48 - pose*1.35
		ca, sa := math.Cos(angle), math.Sin(angle)
		return r.rangedPoser.pose(model, func(part int, p [3]float64) [3]float64 { return bookHinge(part, p, ca, sa) })
	}
	return model.mesh
}
