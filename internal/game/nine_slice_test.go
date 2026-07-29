package game

import (
	"testing"
)

// Every generated 512px frame has 32px caps around a 128px periodic core in
// each edge and both centre axes.
func TestPatternFrame_MeasuresTheRealArt(t *testing.T) {
	t.Chdir("../..")
	want := stripPattern{capA: 32, capB: 32, period: 128}
	for _, name := range []string{
		"menu_panel_frame",
		"menu_panel_slot",
		"menu_panel_tall",
		"menu_panel_wide",
		"menu_panel_slatted",
		"menu_panel_parchment",
		"character_scroll_panel",
	} {
		pf := analyzePatternFrame(name, generatedPatternFrameSlice)
		if pf == nil {
			t.Errorf("%s did not analyze as a pattern frame", name)
			continue
		}
		for stripName, got := range map[string]stripPattern{
			"top": pf.top, "bottom": pf.bottom, "left": pf.left, "right": pf.right,
			"centreH": pf.centreH, "centreV": pf.centreV,
		} {
			if got != want {
				t.Errorf("%s %s = %+v, want %+v", name, stripName, got, want)
			}
		}
	}
}

// Authored classification of every UI frame sprite: painted art must keep
// falling back to stretch. If new or re-authored art becomes pixel-periodic,
// move it to the tiling list here - it starts tiling automatically in game.
func TestPatternFrame_UIFrameInventory(t *testing.T) {
	t.Chdir("../..")
	tiling := map[string]int{
		"menu_panel_frame":       generatedPatternFrameSlice,
		"menu_panel_slot":        generatedPatternFrameSlice,
		"menu_panel_tall":        generatedPatternFrameSlice,
		"menu_panel_wide":        generatedPatternFrameSlice,
		"menu_panel_slatted":     generatedPatternFrameSlice,
		"menu_panel_parchment":   generatedPatternFrameSlice,
		"character_scroll_panel": generatedPatternFrameSlice,
	}
	painted := map[string]int{
		"menu_btn":             menuFrameSlice,
		"inventory_grid_panel": 16,
		"party_member_panel":   16,
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
	pf := analyzePatternFrame("menu_panel_frame", generatedPatternFrameSlice)
	if pf == nil {
		t.Fatal("menu_panel_frame did not analyze")
	}
	for _, size := range [][2]int{{520, 360}, {800, 120}, {130, 130}, {521, 363}, {97, 97}} {
		assertPlanCoverage(t, pf, size[0], size[1], generatedCompactFrameSourceScale, true)
	}
}

func TestGeneratedFrameInsetsClearUIContent(t *testing.T) {
	compactCorner := scaledFrameLength(generatedPatternFrameSlice, generatedCompactFrameSourceScale)
	slotCorner := scaledFrameLength(generatedPatternFrameSlice, generatedSlotFrameSourceScale)
	tallCorner := scaledFrameLength(generatedPatternFrameSlice, generatedTallPanelFrameSourceScale)

	if compactCorner != 16 || slotCorner != 12 || tallCorner > partyHeroDetailPortraitInset {
		t.Fatalf("frame geometry = compact:%d slot:%d tall:%d", compactCorner, slotCorner, tallCorner)
	}

	menu := computeTabbedMenuLayout(1280, 720)
	for i, tab := range menu.tabs {
		if tab.y-menu.panel.y < compactCorner {
			t.Errorf("tab %d starts inside the %dpx frame", i, compactCorner)
		}
	}
	if menu.close.y-menu.panel.y < compactCorner {
		t.Errorf("close button starts inside the %dpx frame", compactCorner)
	}

	character := computeCharacterContentLayout(menu.content)
	if character.portrait.x-character.portraitFrame.x < compactCorner ||
		character.portrait.y-character.portraitFrame.y < compactCorner {
		t.Errorf("character portrait does not clear the %dpx frame", compactCorner)
	}
	if character.portraitFrame.y != character.scroll.y {
		t.Errorf("character portrait frame top %d does not align with stats top %d",
			character.portraitFrame.y, character.scroll.y)
	}
	if partyHeroCardPortraitInset < slotCorner {
		t.Errorf("party card portrait inset %d is smaller than its %dpx frame", partyHeroCardPortraitInset, slotCorner)
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
						assertPlanCoverage(t, pf, size[0], size[1], 1, false)
					}
				}
			}
		}
	}
}

// assertPlanCoverage checks the exactly-once contract; mustPlan fails the test
// when the plan refuses (a refusal is a legal stretch fallback otherwise).
func assertPlanCoverage(t *testing.T, pf *patternFrame, w, h, sourceScale int, mustPlan bool) {
	t.Helper()
	ops, ok := planPatternFrame(pf, w, h, sourceScale)
	if !ok {
		if mustPlan {
			t.Errorf("%dx%d: plan refused", w, h)
		}
		return
	}
	cover := make([]int, w*h)
	for _, op := range ops {
		if op.sw <= 0 || op.sh <= 0 || op.dw <= 0 || op.dh <= 0 {
			t.Fatalf("%dx%d: empty op %+v", w, h, op)
		}
		if op.sx < 0 || op.sy < 0 || op.sx+op.sw > pf.w || op.sy+op.sh > pf.h {
			t.Fatalf("%dx%d: op reads outside the sprite: %+v", w, h, op)
		}
		for y := op.dy; y < op.dy+op.dh; y++ {
			for x := op.dx; x < op.dx+op.dw; x++ {
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
