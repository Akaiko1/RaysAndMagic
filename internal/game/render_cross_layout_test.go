package game

import (
	"math"
	"testing"
)

// Each row is a 3x3 neighbourhood around the tile under test (the centre).
// 'H' is a linked panel, anything else is not.
func TestLinkedPanelArmsFollowNeighbours(t *testing.T) {
	const (
		rowEW = panelArmE | panelArmW
		rowNS = panelArmN | panelArmS
	)
	tests := []struct {
		name string
		grid [3]string
		coin bool
		want panelArms
	}{
		{name: "alone stands along one side", grid: [3]string{"...", ".H.", "..."}, want: rowEW},
		{name: "alone stands along the other side", grid: [3]string{"...", ".H.", "..."}, coin: true, want: rowNS},
		{name: "one neighbour makes a whole panel", grid: [3]string{"...", ".HH", "..."}, coin: true, want: rowEW},
		{name: "end of a column", grid: [3]string{".H.", ".H.", "..."}, want: rowNS},
		{name: "middle of a row", grid: [3]string{"...", "HHH", "..."}, coin: true, want: rowEW},
		{name: "corner folds", grid: [3]string{"...", ".HH", ".H."}, want: panelArmE | panelArmS},
		{name: "tee", grid: [3]string{"...", "HHH", ".H."}, want: panelArmE | panelArmW | panelArmS},
		{name: "one diagonal neighbour", grid: [3]string{"..H", ".H.", "..."}, want: panelArmNE | panelArmSW},
		{name: "diagonal run", grid: [3]string{"..H", ".H.", "H.."}, want: panelArmNE | panelArmSW},
		{name: "vee over a stem", grid: [3]string{"H.H", ".H.", ".H."}, want: panelArmNW | panelArmNE | panelArmS},
		{name: "diagonal already joined by a side", grid: [3]string{".HH", ".H.", "..."}, want: rowNS},
		{name: "filled block joins by sides only", grid: [3]string{"HHH", "HHH", "HHH"}, want: rowEW | rowNS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkedPanelArms(func(dx, dy int) bool { return tt.grid[1+dy][1+dx] == 'H' }, tt.coin)
			if got != tt.want {
				t.Fatalf("arms %08b, want %08b", got, tt.want)
			}
		})
	}
}

// Linked arms must end where the neighbour's begin: a side arm on the middle
// of the shared edge, a diagonal arm on the shared corner.
func TestLinkedPanelLayoutMeetsNeighbours(t *testing.T) {
	const tile = 64.0
	ends := map[panelArms][2]float64{
		panelArmE: {tile / 2, 0}, panelArmW: {-tile / 2, 0},
		panelArmS: {0, tile / 2}, panelArmN: {0, -tile / 2},
		panelArmSE: {tile / 2, tile / 2}, panelArmNW: {-tile / 2, -tile / 2},
		panelArmSW: {-tile / 2, tile / 2}, panelArmNE: {tile / 2, -tile / 2},
	}
	for arm, want := range ends {
		layout := linkedPanelLayout(arm, tile)
		if layout.count != 1 {
			t.Fatalf("arm %08b: %d planes, want one", arm, layout.count)
		}
		plane := layout.planes[0]
		if plane.arms[0] == plane.arms[1] {
			t.Fatalf("arm %08b: raised %v, want exactly one half", arm, plane.arms)
		}
		side := 1.0
		if plane.arms[1] {
			side = -1
		}
		x := math.Cos(plane.yaw) * side * plane.footprint / 2
		y := math.Sin(plane.yaw) * side * plane.footprint / 2
		if math.Abs(x-want[0]) > 1e-9 || math.Abs(y-want[1]) > 1e-9 {
			t.Fatalf("arm %08b ends at (%.3f, %.3f), want (%.3f, %.3f)", arm, x, y, want[0], want[1])
		}
	}
}
