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
		{1024, 768},
		{1280, 720},
		{1366, 768},
		{1920, 1080},
		{2560, 1440},
		{3440, 1440},
		{3840, 2160},
	}

	type menuCase struct {
		name  string
		build func(w, h int) []func() (string, uiBox, []uiBox)
	}
	staticMenu := func(name string, layout func(int, int) (uiBox, []uiBox)) menuCase {
		return menuCase{
			name: name,
			build: func(w, h int) []func() (string, uiBox, []uiBox) {
				return []func() (string, uiBox, []uiBox){func() (string, uiBox, []uiBox) {
					region, boxes := layout(w, h)
					return name, region, boxes
				}}
			},
		}
	}
	menus := []menuCase{
		staticMenu("main-menu", mainMenuLayoutBoxes),
		staticMenu("audio-settings-entry", func(w, h int) (uiBox, []uiBox) {
			return audioSettingsLayoutBoxes(w, h, true)
		}),
		staticMenu("audio-settings-esc", func(w, h int) (uiBox, []uiBox) {
			return audioSettingsLayoutBoxes(w, h, false)
		}),
		staticMenu("tabbed-menu", tabbedMenuLayoutBoxes),
		staticMenu("inventory", inventoryLayoutBoxes),
		staticMenu("characters", charactersLayoutBoxes),
		staticMenu("spell-and-trap-book", bookLayoutBoxes),
		staticMenu("cards", cardsLayoutBoxes),
		staticMenu("quests", questsLayoutBoxes),
		staticMenu("map-overlay", mapOverlayLayoutBoxes),
		staticMenu("spell-trader", spellTraderLayoutBoxes),
		staticMenu("trainer-dialog", trainerDialogLayoutBoxes),
		staticMenu("merchant-dialog", merchantDialogLayoutBoxes),
		staticMenu("card-collector", cardCollectorLayoutBoxes),
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
			name: "trainer-popup",
			build: func(w, h int) []func() (string, uiBox, []uiBox) {
				return []func() (string, uiBox, []uiBox){
					func() (string, uiBox, []uiBox) {
						r, b := skillTrainerPopupLayoutBoxes(w, h)
						return "trainer-popup", r, b
					},
				}
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
		logicalW, logicalH := logicalScreenSize(res.w, res.h)
		for _, m := range menus {
			for _, build := range m.build(logicalW, logicalH) {
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

// The spell trader's price line is a full text line under each icon. It shipped
// folded into a magic "icon + 14" cell and a five-digit price ("22000 g") put
// its descenders on the frame of the icon in the row below - so the cell model
// is pinned here, per cell, at the geometry the renderer actually uses.
func TestSpellTraderPriceLineStaysInItsCell(t *testing.T) {
	const dialogX, dialogY = 100, 50
	const frameMargin = 3 // widest selection frame drawn around an icon

	if w := debugTextWidth("22000 g"); w > spellTraderPriceBoxW {
		t.Errorf("widest price is %dpx, box is %dpx", w, spellTraderPriceBoxW)
	}
	for slot := 0; slot < spellTraderPerPage; slot++ {
		x, y, _, _ := spellTraderIconRect(dialogX, dialogY, slot)
		px, py, pw, ph := spellTraderPriceRect(x, y)
		if py < y+spellTraderIconSize+frameMargin {
			t.Errorf("slot %d: price starts at %d, inside the icon frame ending at %d",
				slot, py, y+spellTraderIconSize+frameMargin)
		}
		if below := slot + spellTraderGridCols; below < spellTraderPerPage {
			_, by, _, _ := spellTraderIconRect(dialogX, dialogY, below)
			if py+ph > by-frameMargin {
				t.Errorf("slot %d: price ends at %d, the icon below frames from %d", slot, py+ph, by-frameMargin)
			}
		}
		if slot%spellTraderGridCols < spellTraderGridCols-1 {
			nx, ny, _, _ := spellTraderIconRect(dialogX, dialogY, slot+1)
			npx, _, _, _ := spellTraderPriceRect(nx, ny)
			if px+pw > npx {
				t.Errorf("slot %d: price ends at %d, the next price starts at %d", slot, px+pw, npx)
			}
		}
	}
}
