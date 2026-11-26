package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

// The measured rows must hold every sample that can read colour, in the
// sticker chain and the core chain alike. The test walks the art the way the
// shaders do (standeeSourcePosition, then a bilinear fetch in the level,
// standee_sampling.kageinc) and checks each sample that touches a
// non-transparent texel against the rows of its band. The 40x24 art also
// reduces through odd terminal sizes.
func TestStandeeArtRowsHoldEverySampleThatReadsColour(t *testing.T) {
	art := image.NewRGBA(image.Rect(0, 0, 40, 24))
	for _, p := range []image.Point{{0, 0}, {39, 23}, {3, 9}, {20, 2}, {21, 2}, {31, 17}} {
		art.SetRGBA(p.X, p.Y, color.RGBA{200, 120, 40, 255})
	}
	for y := 6; y < 20; y++ {
		art.SetRGBA(12, y, color.RGBA{9, 9, 9, 9}) // a faint line still counts
	}
	art.SetRGBA(22, 2, color.RGBA{60, 40, 10, 100}) // a soft fringe: core, no wall
	prepared := prepareStandeePixels(art, 0, false, false)
	rows := prepared.rows
	if len(rows) != len(prepared.stickerMips) || len(rows) != len(prepared.coreMips) || len(rows) < 3 {
		t.Fatalf("rows for %d levels, chains have %d and %d", len(rows), len(prepared.stickerMips), len(prepared.coreMips))
	}
	w0, h0 := float64(art.Bounds().Dx()), float64(art.Bounds().Dy())
	for k := range rows {
		reads := 0
		for _, level := range []*image.RGBA{prepared.stickerMips[k], prepared.coreMips[k]} {
			w, h := level.Bounds().Dx(), level.Bounds().Dy()
			colour := func(x, y int) bool {
				c := level.RGBAAt(x, y)
				return c.R|c.G|c.B|c.A != 0
			}
			for ui := 0; ui <= 520; ui++ {
				for vi := 0; vi <= 530; vi++ {
					u, v := float64(ui)/520, float64(vi)/530
					px, py := math.Min(u*w0, w0-1)+0.5, v*(h0-1)+0.5
					qx := math.Min(math.Max(px*float64(w)/w0, 0.5), float64(w)-0.5)
					qy := math.Min(math.Max(py*float64(h)/h0, 0.5), float64(h)-0.5)
					x0, y0 := int(math.Floor(qx-0.5)), int(math.Floor(qy-0.5))
					// The far texel of each pair weighs nothing at a zero rate.
					x1, y1 := x0, y0
					if qx-0.5 > float64(x0) {
						x1 = min(x0+1, w-1)
					}
					if qy-0.5 > float64(y0) {
						y1 = min(y0+1, h-1)
					}
					if !colour(x0, y0) && !colour(x1, y0) && !colour(x0, y1) && !colour(x1, y1) {
						continue
					}
					reads++
					band := standeeRowBand(u)
					if top, bottom := float64(rows[k].top[band]), float64(rows[k].bottom[band]); v < top || v > bottom {
						t.Fatalf("level %d: the sample at u=%.4f v=%.4f reads colour outside band %d rows %.4f..%.4f", k, u, v, band, top, bottom)
					}
					group := band / standeeRowGroup
					if top, bottom := float64(rows[k].groupTop[group]), float64(rows[k].groupBottom[group]); v < top || v > bottom {
						t.Fatalf("level %d: the sample at u=%.4f v=%.4f lies outside group %d rows %.4f..%.4f", k, u, v, group, top, bottom)
					}
				}
			}
		}
		if reads == 0 {
			t.Fatalf("level %d: no sample read colour; the walk is broken", k)
		}
	}
	// The measurement stays tight: empty columns stay empty at full size.
	if band := standeeRowBand(27.5 / 40); rows[0].top[band] <= rows[0].bottom[band] {
		t.Errorf("band %d holds no art yet measures rows %.3f..%.3f", band, rows[0].top[band], rows[0].bottom[band])
	}
	// The end walls close each row's opaque art where the faces show it: half
	// a texel out from its first and last opaque texel, held to the art's
	// edge. Neither the faint line nor the soft fringe at 22,2 moves them.
	edge := func(texelEdge, n float64) float32 { return float32(math.Min(math.Max(texelEdge/n, 0), 1)) }
	var starts, ends []standeeEndWall
	for _, p := range []image.Point{{0, 0}, {20, 2}, {3, 9}, {31, 17}, {39, 23}} {
		last := float64(p.X)
		if p.Y == 2 {
			last = 21
		}
		y := float64(p.Y)
		v0, v1 := edge(y-0.5, h0-1), edge(y+0.5, h0-1)
		end := edge(last+0.5, w0)
		if last == w0-1 {
			end = 1
		}
		starts = append(starts, standeeEndWall{v0, v1, edge(float64(p.X)-0.5, w0)})
		ends = append(ends, standeeEndWall{v0, v1, end})
	}
	if got := rows[0].endWalls; fmt.Sprint(got) != fmt.Sprint([2][]standeeEndWall{starts, ends}) {
		t.Errorf("end walls %v, want starts %v and ends %v", got, starts, ends)
	}
	if rows[0].endTone[3] == 0 {
		t.Error("the end walls have no tone")
	}
}

func TestStandeeTrimSpan(t *testing.T) {
	var level standeeLevelRows
	for band := range level.top {
		level.top[band], level.bottom[band] = 1, 0
	}
	// Art in bands 16..23 between v 0.25 and 0.5, and in band 40 at the foot.
	for band := 16; band < 24; band++ {
		level.top[band], level.bottom[band] = 0.25, 0.5
	}
	level.top[40], level.bottom[40] = 0.9, 1
	for group := range level.groupTop {
		level.groupTop[group], level.groupBottom[group] = 1, 0
		for band := group * standeeRowGroup; band < (group+1)*standeeRowGroup; band++ {
			level.groupTop[group] = min(level.groupTop[group], level.top[band])
			level.groupBottom[group] = max(level.groupBottom[group], level.bottom[band])
		}
	}
	trim := standeeTrim{levels: [3]*standeeLevelRows{&level}, count: 1}
	at := func(band float64) float64 { return (band + 0.5) / standeeRowBands }
	const height = 200.0
	slack := standeeTrimMargin / height
	for _, tc := range []struct {
		name        string
		trim        standeeTrim
		u0, u1      float64
		height      float64
		top, bottom float64
		ok          bool
	}{
		{"one band of art", trim, at(18), at(18), height, 0.25 - slack, 0.5 + slack, true},
		{"either order", trim, at(20), at(17), height, 0.25 - slack, 0.5 + slack, true},
		{"across a whole group and beyond", trim, at(10), at(45), height, 0.25 - slack, 1, true},
		{"between the two", trim, at(26), at(38), height, 0, 0, false},
		{"left of the art's frame", trim, -0.4, -0.1, height, 0, 0, false},
		{"right of the art's frame", trim, 1.1, 1.3, height, 0, 0, false},
		{"a face too short for the margin keeps every row", trim, at(18), at(18), standeeTrimMargin, 0, 1, true},
		{"no measured rows", standeeTrim{}, at(30), at(30), height, 0, 1, true},
		{"taps along U reach the art from an empty band", standeeTrim{levels: trim.levels, count: 1, reachU: 3.0 / standeeRowBands}, at(26), at(26), height, 0.25 - slack, 0.5 + slack, true},
		{"taps along V widen the rows", standeeTrim{levels: trim.levels, count: 1, reachV: 0.1}, at(18), at(18), height, 0.15 - slack, 0.6 + slack, true},
	} {
		top, bottom, ok := tc.trim.span(tc.u0, tc.u1, tc.height)
		if ok != tc.ok || math.Abs(top-tc.top) > 1e-6 || math.Abs(bottom-tc.bottom) > 1e-6 {
			t.Errorf("%s: span = %.4f..%.4f %v, want %.4f..%.4f %v", tc.name, top, bottom, ok, tc.top, tc.bottom, tc.ok)
		}
	}

	// reaching follows nearStickerAt: 0.375 of what the footprint exceeds a
	// texel and a half by, along each axis, since the shader picks the major
	// one per pixel.
	full, mip := image.Pt(512, 256), image.Pt(128, 64)
	if got := trim.reaching(40, 5, full, mip); math.Abs(got.reachU-0.375*(40-1.5*4)/512) > 1e-9 || got.reachV != 0 {
		t.Errorf("horizontal footprint 40: reach %.6f, %.6f", got.reachU, got.reachV)
	}
	if got := trim.reaching(5, 40, full, mip); math.Abs(got.reachV-0.375*(40-1.5*4)/255) > 1e-9 || got.reachU != 0 {
		t.Errorf("vertical footprint 40: reach %.6f, %.6f", got.reachU, got.reachV)
	}
	if got := trim.reaching(5, 5, full, mip); got.reachU != 0 || got.reachV != 0 {
		t.Errorf("a footprint under a texel and a half reaches %.6f, %.6f", got.reachU, got.reachV)
	}
	for _, tc := range []struct{ columns, want int }{{standeeTrimMinColumns - 1, 0}, {standeeTrimMinColumns, 1}, {standeeTrimPieceColumns * 3, 3}, {standeeTrimPieceColumns * 100, standeeTrimMaxPieces}} {
		if got := trim.pieces(tc.columns); got != tc.want {
			t.Errorf("a strip %d columns wide is cut into %d pieces, want %d", tc.columns, got, tc.want)
		}
	}
}
