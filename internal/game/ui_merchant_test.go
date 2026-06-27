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
	seen := map[[2]int]int{}
	for slot := 0; slot < merchantPageSize; slot++ {
		x, y, w, h := merchantCellRect(baseX, gridTop, slot)
		if w != merchantIconSize || h != merchantIconSize {
			t.Fatalf("slot %d: size = %dx%d, want %d square", slot, w, h, merchantIconSize)
		}
		col := slot % merchantGridCols
		row := slot / merchantGridCols
		if key := ([2]int{col, row}); seen[key] != 0 || (col == 0 && row == 0 && slot != 0) {
			t.Fatalf("slot %d collides at col=%d row=%d", slot, col, row)
		}
		seen[[2]int{col, row}] = slot + 1
		// x grows with column, y with row; both anchored at the grid origin.
		wantX := baseX + col*(merchantIconSize+merchantIconGapX)
		if x != wantX {
			t.Errorf("slot %d: x = %d, want %d", slot, x, wantX)
		}
		if y < gridTop || (row > 0 && y <= gridTop) {
			t.Errorf("slot %d: y = %d not below grid top %d for row %d", slot, y, gridTop, row)
		}
	}
	if len(seen) != merchantPageSize {
		t.Errorf("expected %d distinct cells, got %d", merchantPageSize, len(seen))
	}
}
