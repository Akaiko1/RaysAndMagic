package monster

import (
	"math"
	"sort"
)

// Monsters walk through each other freely; the tile fan is the picture-only
// answer to two or more of them sharing a tile, in RT and TB alike: a calm
// band, a combat transit, or monsters that simply stopped there. The occupants
// stand on a short line through the tile along world X, fixed in the world:
// the first slot keeps the centre, the second and third sit
// TileFanRadiusTiles to either side, and a fourth or later occupant shares a
// slot, so a crowd reads as three. Only when the camera sees them one behind
// another (a file walking toward or away from it) are they shifted across the
// line of sight: the nearest left, the middle one where it is, the farthest
// right. Movement, collision and combat never read it.

// TileFanRadiusTiles is how far the outer slots sit from the tile's centre,
// and how far a hidden occupant is shifted sideways.
const TileFanRadiusTiles = 0.16

// tileFanSlots is how many distinct places one shared tile shows.
const tileFanSlots = 3

// tileFanHideGapTiles is the sideways spacing below which the camera sees two
// occupants as hiding each other; the sideways shift fades out as their own
// spacing opens to it.
const tileFanHideGapTiles = 0.1

// TileFanFrame is what one tick of the fan needs from the game.
type TileFanFrame struct {
	Tile float64
	TPS  int
	// CamX, CamY is the logical camera the occupants are seen from.
	CamX, CamY float64
	// Fighting marks a combat occupant; a tile holding one has a Combat fan.
	Fighting func(*Monster3D) bool
}

func fanTile(m *Monster3D, tile float64) [2]int {
	return [2]int{int(math.Floor(m.X / tile)), int(math.Floor(m.Y / tile))}
}

// fanRank orders a tile's occupants so their slots stay put while others come
// and go: an attack post holder keeps the centre, a band keeps its own order
// (leader first), everyone else follows by ID.
func fanRank(m *Monster3D) int {
	switch {
	case m.AttackPost:
		return 0
	case m.BandStackCount > 1:
		return 1
	}
	return 2
}

func fanBefore(a, b *Monster3D) bool {
	if ra, rb := fanRank(a), fanRank(b); ra != rb {
		return ra < rb
	}
	if a.BandID != b.BandID {
		return a.BandID < b.BandID
	}
	if a.BandStackIndex != b.BandStackIndex {
		return a.BandStackIndex < b.BandStackIndex
	}
	return a.ID < b.ID
}

// TileFanSlot is the world offset of slot index of count: the centre, then
// one side of the line, then the other.
func TileFanSlot(index, count int, tile float64) (float64, float64) {
	if count <= 1 {
		return 0, 0
	}
	switch index % tileFanSlots {
	case 1:
		return TileFanRadiusTiles * tile, 0
	case 2:
		return -TileFanRadiusTiles * tile, 0
	}
	return 0, 0
}

// fanSight is how the camera sees a shared tile's group: the sight unit
// (ux, uy) toward the group and the camera's left (lx, ly); w is how fully
// its occupants hide one another (1 = seen end on) and shifts how far each
// slot moves left so they stand apart - the nearest left, the farthest right,
// a middle one not at all. A pair splits the shift.
type fanSight struct {
	ux, uy, lx, ly, w float64
	shifts            [tileFanSlots]float64
}

func (f TileFanFrame) sight(group []*Monster3D) (v fanSight) {
	anchor := group[0]
	ux, uy := anchor.X-f.CamX, anchor.Y-f.CamY
	length := math.Hypot(ux, uy)
	if length == 0 {
		return
	}
	v.ux, v.uy = ux/length, uy/length
	v.lx, v.ly = v.uy, -v.ux
	type seen struct {
		slot           int
		depth, lateral float64
	}
	var byDepth [tileFanSlots]seen
	n := min(len(group), tileFanSlots)
	for k := 0; k < n; k++ {
		ox, oy := TileFanSlot(k, len(group), f.Tile)
		x, y := group[k].X+ox-f.CamX, group[k].Y+oy-f.CamY
		byDepth[k] = seen{k, x*v.ux + y*v.uy, x*v.lx + y*v.ly}
		for j := k; j > 0 && byDepth[j].depth < byDepth[j-1].depth; j-- {
			byDepth[j], byDepth[j-1] = byDepth[j-1], byDepth[j]
		}
	}
	// Every pair: the nearest and farthest can hide each other while the
	// middle one stands aside.
	gap := math.Inf(1)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			gap = min(gap, math.Abs(byDepth[i].lateral-byDepth[j].lateral))
		}
	}
	v.w = max(0, 1-gap/(tileFanHideGapTiles*f.Tile))
	step := TileFanRadiusTiles * f.Tile * v.w
	if n == 2 {
		step /= 2
	}
	v.shifts[byDepth[0].slot], v.shifts[byDepth[n-1].slot] = step, -step
	return
}

// UpdateTileFans runs once per tick after movement: it numbers the live
// occupants of each shared tile, shifts those the camera sees one behind
// another, and eases every monster's offset toward its place in about 0.15 s,
// so joining or leaving a tile glides instead of snapping. Dead monsters are
// left alone (the kill keeps where they were drawn). scratch is reused between
// calls and returned.
func UpdateTileFans(monsters []*Monster3D, f TileFanFrame, scratch []*Monster3D) []*Monster3D {
	if f.Tile <= 0 || f.TPS <= 0 {
		return scratch
	}
	occupants := scratch[:0]
	for _, m := range monsters {
		if m != nil && m.IsAlive() {
			m.Fan.Index, m.Fan.Count = 0, 0
			occupants = append(occupants, m)
		}
	}
	sort.Slice(occupants, func(i, j int) bool {
		a, b := fanTile(occupants[i], f.Tile), fanTile(occupants[j], f.Tile)
		if a != b {
			return a[1] < b[1] || a[1] == b[1] && a[0] < b[0]
		}
		return fanBefore(occupants[i], occupants[j])
	})
	// Exponential response, independent of TPS. Presentation only, not a move.
	blend := 1 - math.Exp(-20/float64(f.TPS))
	ease := func(m *Monster3D, x, y float64) {
		m.Fan.X += (x - m.Fan.X) * blend
		m.Fan.Y += (y - m.Fan.Y) * blend
		if math.Hypot(x-m.Fan.X, y-m.Fan.Y) < .01 {
			m.Fan.X, m.Fan.Y = x, y
			m.Fan.Active = m.Fan.Count > 1
		}
	}
	for first := 0; first < len(occupants); {
		last := first + 1
		for last < len(occupants) && fanTile(occupants[last], f.Tile) == fanTile(occupants[first], f.Tile) {
			last++
		}
		if group := occupants[first:last]; len(group) > 1 {
			v := f.sight(group)
			combat := false
			for _, m := range group {
				combat = combat || f.Fighting != nil && f.Fighting(m)
			}
			// A combat file seen end on steps back as a whole rather than let
			// its nearest occupant come closer than it stands, so the nearest
			// is still drawn nearest.
			var back float64
			if combat {
				for k := 0; k < min(len(group), tileFanSlots); k++ {
					x, y := TileFanSlot(k, len(group), f.Tile)
					back = max(back, -(x*v.ux + y*v.uy))
				}
				back *= v.w
			}
			for index, m := range group {
				m.Fan.Index, m.Fan.Count, m.Fan.Combat, m.Fan.Active = index, len(group), combat, true
				x, y := TileFanSlot(index, len(group), f.Tile)
				shift := v.shifts[index%tileFanSlots]
				ease(m, x+v.lx*shift+v.ux*back, y+v.ly*shift+v.uy*back)
			}
		}
		first = last
	}
	for _, m := range occupants {
		if m.Fan.Count <= 1 && m.Fan.Active {
			ease(m, 0, 0)
		}
	}
	return occupants
}

// FanVisualOffset is the drawn offset of m at the presented point (x, y) seen
// from (camX, camY). A combat fan never draws a fanned hostile closer to the
// camera than it stands (so never closer than its attack post) and keeps it
// inside its tile, so a monster walking at a tile edge cannot appear in the
// neighbouring one: it takes the nearest offset meeting both. A calm fan is
// drawn as it stands.
func FanVisualOffset(m *Monster3D, x, y, camX, camY, tile float64) (float64, float64) {
	if m == nil || m.Fan.X == 0 && m.Fan.Y == 0 {
		return 0, 0
	}
	ox, oy := m.Fan.X, m.Fan.Y
	if !m.Fan.Combat {
		return ox, oy
	}
	lo := func(p float64) float64 { return math.Floor(p/tile) * tile }
	hi := func(p float64) float64 { return math.Nextafter(lo(p)+tile, lo(p)) }
	ux, uy := x-camX, y-camY
	length := math.Hypot(ux, uy)
	if length > 0 {
		ux, uy = ux/length, uy/length
		if inward := ox*ux + oy*uy; inward < 0 {
			ox, oy = ox-inward*ux, oy-inward*uy
		}
	}
	// The nearest point of the tile; if that one points toward the camera,
	// the nearest allowed one lies on the sideways line through (x, y).
	cx, cy := max(lo(x), min(hi(x), x+ox))-x, max(lo(y), min(hi(y), y+oy))-y
	if length == 0 || cx*ux+cy*uy >= 0 {
		return cx, cy
	}
	lx, ly := -uy, ux
	t0, t1 := math.Inf(-1), math.Inf(1)
	for _, axis := range [2][2]float64{{x, lx}, {y, ly}} {
		if p, l := axis[0], axis[1]; l != 0 {
			a, b := (lo(p)-p)/l, (hi(p)-p)/l
			t0, t1 = max(t0, min(a, b)), min(t1, max(a, b))
		}
	}
	t := max(t0, min(t1, ox*lx+oy*ly))
	return t * lx, t * ly
}
