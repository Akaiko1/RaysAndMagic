package monster

import (
	"math"
	"sort"
	"testing"
)

const fanTestTile, fanTestTPS = 64.0, 120

func fanTestMonster(id string, tx, ty int, hp int) *Monster3D {
	return &Monster3D{ID: id, HitPoints: hp, X: (float64(tx) + .5) * fanTestTile, Y: (float64(ty) + .5) * fanTestTile}
}

// settleFans runs one second of calm fan ticks seen from the camera tile (cx, cy).
func settleFans(monsters []*Monster3D, cx, cy float64) {
	settleFansFighting(monsters, cx, cy, false)
}

func settleFansFighting(monsters []*Monster3D, cx, cy float64, fighting bool) {
	frame := TileFanFrame{Tile: fanTestTile, TPS: fanTestTPS, CamX: cx * fanTestTile, CamY: cy * fanTestTile,
		Fighting: func(*Monster3D) bool { return fighting }}
	var scratch []*Monster3D
	for i := 0; i < fanTestTPS; i++ {
		scratch = UpdateTileFans(monsters, frame, scratch)
	}
}

// Any tile two live monsters stand on fans them apart in the picture, calm
// ones included; a lone monster gets no fan, a dead one keeps its last offset,
// and a monster that walks off settles back on its true position.
func TestTileFanSpreadsEveryTileShare(t *testing.T) {
	a, b, c, lone := fanTestMonster("a", 3, 0, 10), fanTestMonster("b", 3, 0, 10), fanTestMonster("c", 3, 0, 10), fanTestMonster("d", 5, 0, 10)
	monsters := []*Monster3D{b, c, lone, a}
	settleFans(monsters, 0, 0)
	if a.Fan.Count != 3 || b.Fan.Count != 3 || a.Fan.Index == b.Fan.Index || !a.Fan.Active || !b.Fan.Active {
		t.Fatalf("calm monsters on one tile are not fanned: %+v / %+v", a.Fan, b.Fan)
	}
	ax, ay := FanVisualOffset(a, a.X, a.Y, 0, 0, fanTestTile)
	bx, by := FanVisualOffset(b, b.X, b.Y, 0, 0, fanTestTile)
	if math.Hypot(ax-bx, ay-by) < 1 {
		t.Fatal("the fanned pair is still drawn pixel on pixel")
	}
	if lone.Fan != (TileFan{}) {
		t.Fatalf("a lone monster got a fan: %+v", lone.Fan)
	}
	c.HitPoints = 0
	dead := c.Fan
	b.X += fanTestTile // walks off to the next tile
	settleFans(monsters, 0, 0)
	if c.Fan != dead {
		t.Fatalf("a dead monster's fan moved: %+v, was %+v", c.Fan, dead)
	}
	for _, m := range []*Monster3D{a, b} {
		if x, y := FanVisualOffset(m, m.X, m.Y, 0, 0, fanTestTile); m.Fan.Active || x != 0 || y != 0 {
			t.Fatalf("%s did not settle on its true position: %+v", m.ID, m.Fan)
		}
	}
}

// The fan is fixed in the world: walking the camera around a shared tile
// moves nobody and a fourth occupant moves nobody. Only occupants the camera
// sees one behind another are shifted: the nearest left, the middle one in
// place, the farthest right - ranked by depth, not by slot - and they return
// to their world places once seen from the side.
func TestTileFanHoldsStill(t *testing.T) {
	offsets := func(ms []*Monster3D) [][2]float64 {
		var out [][2]float64
		for _, m := range ms {
			out = append(out, [2]float64{m.Fan.X, m.Fan.Y})
		}
		return out
	}
	same := func(a, b [][2]float64) bool {
		for i := range a {
			if math.Hypot(a[i][0]-b[i][0], a[i][1]-b[i][1]) > 1e-9 {
				return false
			}
		}
		return true
	}
	band := func() []*Monster3D {
		var ms []*Monster3D
		for i, id := range []string{"m2", "m3", "m4"} {
			m := fanTestMonster(id, 5, 5, 10)
			m.BandID, m.BandStackIndex, m.BandStackCount = 1, i, 3
			ms = append(ms, m)
		}
		return ms
	}
	// Seen from the side: the world line (X) is left alone.
	t.Run("camera_walks_around", func(t *testing.T) {
		ms := band()
		settleFans(ms, 5.5, .5)
		before := offsets(ms)
		for _, cam := range [][2]float64{{3.5, 1.5}, {7.5, 1.5}, {5.5, 10.5}, {2.5, 9.5}} {
			settleFans(ms, cam[0], cam[1])
			if !same(before, offsets(ms)) {
				t.Fatalf("the band moved when the camera went to %v: %v -> %v", cam, before, offsets(ms))
			}
		}
	})
	t.Run("fourth_occupant", func(t *testing.T) {
		ms := band()
		settleFans(ms, 5.5, .5)
		before := offsets(ms)
		extra := fanTestMonster("m1", 5, 5, 10) // sorts first by ID
		settleFans(append(ms, extra), 5.5, .5)
		if !same(before, offsets(ms)) {
			t.Fatalf("the band reshuffled for a fourth occupant: %v -> %v", before, offsets(ms))
		}
		if shared := [2]float64{extra.Fan.X, extra.Fan.Y}; !same([][2]float64{shared}, before[:1]) &&
			!same([][2]float64{shared}, before[1:2]) && !same([][2]float64{shared}, before[2:]) {
			t.Fatalf("a fourth occupant got a fourth place %v; a crowd reads as three %v", shared, before)
		}
	})
	file := func() []*Monster3D { // a transit file on one tile, walking along X
		var ms []*Monster3D
		for i, id := range []string{"c", "a", "b"} { // slot order differs from depth order
			m := fanTestMonster(id, 5, 5, 10)
			m.X = (5.2 + .3*float64(i)) * fanTestTile
			ms = append(ms, m)
		}
		return ms
	}
	aside := func() []*Monster3D { // nearest and farthest in line, the middle one aside
		var ms []*Monster3D
		for _, p := range []struct {
			id   string
			x, y float64
		}{{"a", 5.1, 5.5}, {"b", 5.5, 5.85}, {"c", 5.95, 5.5}} {
			m := fanTestMonster(p.id, 5, 5, 10)
			m.X, m.Y = p.x*fanTestTile, p.y*fanTestTile
			ms = append(ms, m)
		}
		return ms
	}
	type view struct {
		id          string
		depth, left float64
	}
	// seenFrom ranks the occupants by the depth they are drawn at (the order
	// the renderer paints them in) with their sideways place (left positive),
	// as the camera at (camX, camY) looking along +X or -X sees them.
	seenFrom := func(ms []*Monster3D, camX, camY float64) []view {
		ux := math.Copysign(1, ms[0].X-camX)
		var seen []view
		for _, m := range ms {
			ox, oy := FanVisualOffset(m, m.X, m.Y, camX, camY, fanTestTile)
			x, y := m.X+ox-camX, m.Y+oy-camY
			seen = append(seen, view{m.ID, x * ux, -y * ux})
		}
		sort.Slice(seen, func(i, j int) bool { return seen[i].depth < seen[j].depth })
		return seen
	}
	for _, tc := range []struct {
		name     string
		group    func() []*Monster3D
		camX     float64
		ordered  bool // nearest left, middle, farthest right; otherwise only apart
		fighting bool
	}{{"band_from_west", band, .5, true, false}, {"band_from_east", band, 10.5, true, false},
		{"file_from_west", file, .5, true, false}, {"file_from_east", file, 10.5, true, false},
		{"combat_band_from_west", band, .5, true, true}, {"combat_file_from_east", file, 10.5, true, true},
		{"aside_from_west", aside, .5, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			ms := tc.group()
			camX, camY := tc.camX*fanTestTile, 5.5*fanTestTile
			settleFansFighting(ms, tc.camX, 5.5, tc.fighting)
			seen := seenFrom(ms, camX, camY)
			if tc.ordered && (seen[0].left-seen[1].left < .1*fanTestTile || seen[1].left-seen[2].left < .1*fanTestTile ||
				seen[1].depth-seen[0].depth < .05*fanTestTile || seen[2].depth-seen[1].depth < .05*fanTestTile) {
				t.Fatalf("seen end on, not drawn nearest-left / middle / farthest-right: %+v", seen)
			}
			if tc.fighting {
				for _, m := range ms {
					if ox, oy := FanVisualOffset(m, m.X, m.Y, camX, camY, fanTestTile); (ox*(m.X-camX) + oy*(m.Y-camY)) < -1e-9 {
						t.Fatalf("%s is drawn closer to the camera than it stands in a combat fan", m.ID)
					}
				}
			}
			for i := range seen {
				for j := i + 1; j < len(seen); j++ {
					if math.Abs(seen[i].left-seen[j].left) < .1*fanTestTile {
						t.Fatalf("seen end on, %s and %s still hide each other: %+v", seen[i].id, seen[j].id, seen)
					}
				}
			}
			settleFans(ms, 5.5, .5)
			for _, m := range ms {
				x, y := TileFanSlot(m.Fan.Index, len(ms), fanTestTile)
				if math.Hypot(m.Fan.X-x, m.Fan.Y-y) > 1e-9 {
					t.Fatalf("%s did not return to its world place once seen from the side: %+v", m.ID, m.Fan)
				}
			}
		})
	}
}

// A combat fan's drawn offset stays inside its tile and never points toward
// the camera, from any spot in the tile and any viewing angle; seen along X at
// the tile's X edge, its sideways part survives the clip.
func TestCombatFanStaysInTileAndNeverCloser(t *testing.T) {
	m := &Monster3D{Fan: TileFan{Active: true, Combat: true}}
	for _, off := range [][2]float64{{.3, .1}, {-.2, .25}, {.16, -.16}, {0, .16}, {-.32, 0}} {
		m.Fan.X, m.Fan.Y = off[0]*fanTestTile, off[1]*fanTestTile
		for px := .02; px < 1; px += .12 {
			for py := .02; py < 1; py += .12 {
				for deg := 0.0; deg < 360; deg += 15 {
					x, y := (5+px)*fanTestTile, (5+py)*fanTestTile
					a := deg * math.Pi / 180
					camX, camY := x-3*fanTestTile*math.Cos(a), y-3*fanTestTile*math.Sin(a)
					ox, oy := FanVisualOffset(m, x, y, camX, camY, fanTestTile)
					if x+ox < 5*fanTestTile || x+ox >= 6*fanTestTile || y+oy < 5*fanTestTile || y+oy >= 6*fanTestTile {
						t.Fatalf("offset %v at (%.2f,%.2f) seen at %.0f deg left the tile: (%.3f,%.3f)", off, px, py, deg, ox, oy)
					}
					if inward := ox*(x-camX) + oy*(y-camY); inward < -1e-6 {
						t.Fatalf("offset %v at (%.2f,%.2f) seen at %.0f deg points toward the camera: %.3g", off, px, py, deg, inward)
					}
				}
			}
		}
	}
	m.Fan.X, m.Fan.Y = .3*fanTestTile, .16*fanTestTile
	x, y := 5.95*fanTestTile, 5.5*fanTestTile
	if _, oy := FanVisualOffset(m, x, y, 0, y, fanTestTile); math.Abs(oy-.16*fanTestTile) > 1e-9 {
		t.Fatalf("the sideways part was clipped with the depth part: %.3f", oy/fanTestTile)
	}
}
