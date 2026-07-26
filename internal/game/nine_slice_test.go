package game

import (
	"testing"
)

// The analysis must measure the REAL art: menu_panel_frame edges are 5px caps
// around a 24px brick period, its centre 6px margins around an 8x8 weave.
// character_scroll_panel is not periodic (centre nail ornament) and must fall
// back to stretch.
func TestPatternFrame_MeasuresTheRealArt(t *testing.T) {
	t.Chdir("../..")
	pf := analyzePatternFrame("menu_panel_frame", 16)
	if pf == nil {
		t.Fatal("menu_panel_frame did not analyze as a pattern frame")
	}
	wantEdge := stripPattern{capA: 5, capB: 5, period: 24}
	for name, got := range map[string]stripPattern{
		"top": pf.top, "bottom": pf.bottom, "left": pf.left, "right": pf.right,
	} {
		if got != wantEdge {
			t.Errorf("%s edge = %+v, want %+v", name, got, wantEdge)
		}
	}
	wantCentre := stripPattern{capA: 6, capB: 6, period: 8}
	if pf.centreH != wantCentre || pf.centreV != wantCentre {
		t.Errorf("centre = %+v / %+v, want %+v", pf.centreH, pf.centreV, wantCentre)
	}

	if got := analyzePatternFrame("character_scroll_panel", 16); got != nil {
		t.Errorf("character_scroll_panel analyzed as periodic %+v, must stretch instead", got)
	}
}

// Authored classification of every UI frame sprite: painted art must keep
// falling back to stretch. If new or re-authored art becomes pixel-periodic,
// move it to the tiling list here - it starts tiling automatically in game.
func TestPatternFrame_UIFrameInventory(t *testing.T) {
	t.Chdir("../..")
	tiling := map[string]int{"menu_panel_frame": menuPanelFrameSlice}
	painted := map[string]int{
		"character_scroll_panel": 16,
		"menu_btn":               menuFrameSlice,
		"menu_panel_wide":        menuFrameSlice,
		"menu_panel_slot":        menuFrameSlice,
		"menu_panel_tall":        menuFrameSlice,
		"inventory_grid_panel":   16,
		"party_member_panel":     16,
	}
	for name, slice := range tiling {
		if analyzePatternFrame(name, slice) == nil {
			t.Errorf("%s no longer analyzes as a pattern frame - its art lost the exact period", name)
		}
	}
	for name, slice := range painted {
		if got := analyzePatternFrame(name, slice); got != nil {
			t.Errorf("%s now analyzes as periodic %+v - move it to the tiling list", name, got)
		}
	}
}

// Every plan is pure 1:1 blits that cover the panel exactly once - no gaps,
// no double-drawn pixels, no source reads outside the sprite.
func TestPatternFramePlan_CoversEveryPixelExactlyOnce(t *testing.T) {
	t.Chdir("../..")
	pf := analyzePatternFrame("menu_panel_frame", 16)
	if pf == nil {
		t.Fatal("menu_panel_frame did not analyze")
	}
	for _, size := range [][2]int{{520, 360}, {800, 120}, {130, 130}, {521, 363}, {97, 97}} {
		assertPlanCoverage(t, pf, size[0], size[1], true)
	}
}

// The plan geometry is art-agnostic: any caps/periods/sprite size must yield
// exact coverage at any panel size, or refuse cleanly (fallback to stretch).
func TestPatternFramePlan_GeometryHoldsForArbitraryPatterns(t *testing.T) {
	for _, slice := range []int{4, 16, 23} {
		for _, spriteRun := range []int{2*slice + 9, 2*slice + 64} {
			for _, capA := range []int{0, 3} {
				for _, period := range []int{2, 7} {
					srcLen := spriteRun - 2*slice
					if capA*2+period > srcLen {
						continue
					}
					sp := stripPattern{capA: capA, capB: capA, period: period}
					m := min(capA+1, srcLen/3)
					pf := &patternFrame{
						w: spriteRun, h: spriteRun, slice: slice,
						top: sp, bottom: sp, left: sp, right: sp,
						centreH: stripPattern{capA: m, capB: m, period: period},
						centreV: stripPattern{capA: m, capB: m, period: period},
					}
					for _, size := range [][2]int{
						{spriteRun, spriteRun}, {spriteRun + 1, spriteRun + 13},
						{3 * spriteRun, spriteRun}, {257, 121}, {2*slice + capA*2 + 2, 2*slice + capA*2 + 2},
					} {
						assertPlanCoverage(t, pf, size[0], size[1], false)
					}
				}
			}
		}
	}
}

// assertPlanCoverage checks the exactly-once contract; mustPlan fails the test
// when the plan refuses (a refusal is a legal stretch fallback otherwise).
func assertPlanCoverage(t *testing.T, pf *patternFrame, w, h int, mustPlan bool) {
	t.Helper()
	ops, ok := planPatternFrame(pf, w, h)
	if !ok {
		if mustPlan {
			t.Errorf("%dx%d: plan refused", w, h)
		}
		return
	}
	cover := make([]int, w*h)
	for _, op := range ops {
		if op.sw <= 0 || op.sh <= 0 {
			t.Fatalf("%dx%d: empty op %+v", w, h, op)
		}
		if op.sx < 0 || op.sy < 0 || op.sx+op.sw > pf.w || op.sy+op.sh > pf.h {
			t.Fatalf("%dx%d: op reads outside the sprite: %+v", w, h, op)
		}
		for y := op.dy; y < op.dy+op.sh; y++ {
			for x := op.dx; x < op.dx+op.sw; x++ {
				if x < 0 || y < 0 || x >= w || y >= h {
					t.Fatalf("%dx%d: op draws outside the panel: %+v", w, h, op)
				}
				cover[y*w+x]++
			}
		}
	}
	for i, c := range cover {
		if c != 1 {
			t.Fatalf("%dx%d: pixel (%d,%d) covered %d times, want exactly once", w, h, i%w, i/w, c)
		}
	}
}
