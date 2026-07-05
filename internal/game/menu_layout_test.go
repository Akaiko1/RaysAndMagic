package game

import (
	"fmt"
	"testing"
)

// assertNoCollisions checks the two invariants every menu must hold: each section
// box stays inside the menu region, and no two section boxes overlap. A failure
// names the offending boxes so the layout bug is obvious.
func assertNoCollisions(t *testing.T, menu string, region uiBox, boxes []uiBox) {
	t.Helper()
	for _, b := range boxes {
		if !region.contains(b) {
			t.Errorf("[%s] %q (%d,%d %dx%d) spills outside region %q (%d,%d %dx%d)",
				menu, b.Name, b.X, b.Y, b.W, b.H, region.Name, region.X, region.Y, region.W, region.H)
		}
	}
	for i := 0; i < len(boxes); i++ {
		for j := i + 1; j < len(boxes); j++ {
			if boxes[i].overlaps(boxes[j]) {
				t.Errorf("[%s] %q (%d,%d %dx%d) overlaps %q (%d,%d %dx%d)",
					menu, boxes[i].Name, boxes[i].X, boxes[i].Y, boxes[i].W, boxes[i].H,
					boxes[j].Name, boxes[j].X, boxes[j].Y, boxes[j].W, boxes[j].H)
			}
		}
	}
}

// TestMenuLayout_NoCollisions iterates every collision-prone menu, across all of
// its pages, at several screen resolutions, asserting no section overlaps another
// or leaves the menu's bounds. Add a builder to the menus slice to cover a new one.
func TestMenuLayout_NoCollisions(t *testing.T) {
	resolutions := []struct{ w, h int }{
		{1280, 720},
		{1024, 768},
		{1920, 1080},
	}

	type menuCase struct {
		name  string
		build func(w, h int) []func() (string, uiBox, []uiBox)
	}
	menus := []menuCase{
		{
			name: "stash",
			build: func(w, h int) []func() (string, uiBox, []uiBox) {
				return []func() (string, uiBox, []uiBox){
					func() (string, uiBox, []uiBox) {
						r, b := stashLayoutBoxes(w, h)
						return "stash", r, b
					},
				}
			},
		},
		{
			name: "save-menu",
			build: func(w, h int) []func() (string, uiBox, []uiBox) {
				var cases []func() (string, uiBox, []uiBox)
				for page := 0; page < savePageCount; page++ {
					for _, load := range []bool{false, true} {
						page, load := page, load
						cases = append(cases, func() (string, uiBox, []uiBox) {
							r, b := saveMenuLayoutBoxes(w, h, page, load)
							mode := "save"
							if load {
								mode = "load"
							}
							return fmt.Sprintf("save-menu/%s/page%d", mode, page+1), r, b
						})
					}
				}
				return cases
			},
		},
		{
			name: "entry-load",
			build: func(w, h int) []func() (string, uiBox, []uiBox) {
				var cases []func() (string, uiBox, []uiBox)
				for page := 0; page < savePageCount; page++ {
					page := page
					cases = append(cases, func() (string, uiBox, []uiBox) {
						r, b := entryLoadLayoutBoxes(w, h, page)
						return fmt.Sprintf("entry-load/page%d", page+1), r, b
					})
				}
				return cases
			},
		},
	}

	for _, res := range resolutions {
		for _, m := range menus {
			for _, build := range m.build(res.w, res.h) {
				name, region, boxes := build()
				t.Run(fmt.Sprintf("%dx%d/%s", res.w, res.h, name), func(t *testing.T) {
					assertNoCollisions(t, name, region, boxes)
				})
			}
		}
	}
}

// TestUiBox_OverlapContains sanity-checks the geometry primitives the collision
// test relies on (touching edges must not count as an overlap).
func TestUiBox_OverlapContains(t *testing.T) {
	a := uiBox{"a", 0, 0, 10, 10}
	if a.overlaps(uiBox{"b", 10, 0, 5, 5}) {
		t.Error("edge-touching boxes must not overlap")
	}
	if !a.overlaps(uiBox{"c", 9, 9, 5, 5}) {
		t.Error("boxes sharing interior must overlap")
	}
	if !a.contains(uiBox{"d", 1, 1, 8, 8}) {
		t.Error("a should contain d")
	}
	if a.contains(uiBox{"e", 1, 1, 20, 20}) {
		t.Error("a must not contain a larger box")
	}
}
