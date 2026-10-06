package game

import "testing"

func TestPageCount(t *testing.T) {
	cases := []struct {
		n, size, want int
	}{
		{0, 12, 1}, // empty list still has a valid page 0
		{1, 12, 1},
		{12, 12, 1}, // exactly one full page
		{13, 12, 2}, // one over spills to a second page
		{25, 12, 3},
	}
	for _, c := range cases {
		if got := pageCount(c.n, c.size); got != c.want {
			t.Errorf("pageCount(%d,%d) = %d, want %d", c.n, c.size, got, c.want)
		}
	}
}

func TestClampPage(t *testing.T) {
	cases := []struct {
		page, total, want int
	}{
		{0, 1, 0},
		{5, 3, 2},  // page past the end snaps to the last page
		{-1, 3, 0}, // negative snaps to 0
		{1, 3, 1},  // already valid, unchanged
	}
	for _, c := range cases {
		p := c.page
		clampPage(&p, c.total)
		if p != c.want {
			t.Errorf("clampPage(%d, total=%d) = %d, want %d", c.page, c.total, p, c.want)
		}
	}
}

// merchantCellRect must map each slot to a unique, non-overlapping cell laid out
// in row-major order, so buy/sell clicks resolve to the right item index.
func TestMerchantCellRectLayout(t *testing.T) {
	const baseX, gridTop = 100, 200
	type cell struct{ x, y, w, h int }
	cells := make([]cell, 0, merchantPageSize)
	for slot := 0; slot < merchantPageSize; slot++ {
		x, y, w, h := merchantCellRect(baseX, gridTop, slot)
		if w != merchantIconSize || h != merchantIconSize {
			t.Fatalf("slot %d: size = %dx%d, want %d square", slot, w, h, merchantIconSize)
		}
		for other, c := range cells {
			if x < c.x+c.w && c.x < x+w && y < c.y+c.h && c.y < y+h {
				t.Fatalf("slot %d (%d,%d) overlaps slot %d (%d,%d)", slot, x, y, other, c.x, c.y)
			}
		}
		// Row-major: x grows with column, y with row; both anchored at the grid origin.
		col, row := slot%merchantGridCols, slot/merchantGridCols
		if wantX := baseX + col*(merchantIconSize+merchantIconGapX); x != wantX {
			t.Errorf("slot %d: x = %d, want %d", slot, x, wantX)
		}
		if row == 0 && y != gridTop {
			t.Errorf("slot %d: first-row y = %d, want the grid top %d", slot, y, gridTop)
		}
		if row > 0 && y <= cells[slot-merchantGridCols].y {
			t.Errorf("slot %d: y = %d not below the cell above (%d)", slot, y, cells[slot-merchantGridCols].y)
		}
		cells = append(cells, cell{x, y, w, h})
	}
}
