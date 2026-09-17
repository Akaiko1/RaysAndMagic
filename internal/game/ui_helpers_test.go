package game

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
)

func TestWrapTooltipLinesAccountsForIconOffset(t *testing.T) {
	lines := []string{"one two three four five"}
	colors := []color.Color{color.White}
	x := 700
	screenW := 900

	withoutIcon, withoutColors := wrapTooltipLines(lines, colors, x, screenW, tooltipTextOffset(false))
	if len(withoutIcon) != 1 {
		t.Fatalf("without icon: got %d wrapped lines, want 1", len(withoutIcon))
	}
	if len(withoutColors) != len(withoutIcon) {
		t.Fatalf("without icon: got %d colors for %d lines", len(withoutColors), len(withoutIcon))
	}

	withIcon, withColors := wrapTooltipLines(lines, colors, x, screenW, tooltipTextOffset(true))
	if len(withIcon) <= 1 {
		t.Fatalf("with icon: got %d wrapped lines, want more than 1", len(withIcon))
	}
	if len(withColors) != len(withIcon) {
		t.Fatalf("with icon: got %d colors for %d lines", len(withColors), len(withIcon))
	}
}

func TestSingleTooltipStaysInViewport(t *testing.T) {
	for _, res := range campHUDResolutions {
		for _, icon := range []bool{false, true} {
			for _, text := range []string{"Camp", "Restores living party members' HP and SP.", strings.Repeat("Long tooltip words ", 50), strings.Repeat("A", 300)} {
				for _, point := range [][2]int{{-10, -10}, {0, 0}, {res[0] / 2, res[1] / 2}, {res[0] + 12, 0}, {0, res[1] + 8}, {res[0] + 12, res[1] + 8}} {
					t.Run(fmt.Sprintf("%dx%d/icon=%v/len=%d/%d,%d", res[0], res[1], icon, len(text), point[0], point[1]), func(t *testing.T) {
						lines := []string{text}
						r := singleTooltipLayout(lines, nil, icon, point[0], point[1], res[0], res[1])
						if r.x < tooltipScreenMargin || r.y < tooltipScreenMargin || r.right() > res[0]-tooltipScreenMargin || r.bottom() > res[1]-tooltipScreenMargin {
							t.Fatalf("tooltip outside viewport: %+v", r)
						}
						w, h := tooltipBoxSizeForScreen(lines, nil, icon, r.x, r.right())
						if w != r.w || h != r.h {
							t.Fatalf("draw wrapping differs from placement: %dx%d vs %+v", w, h, r)
						}
					})
				}
			}
		}
	}
}

func TestWrapTooltipLinesSplitsOversizedToken(t *testing.T) {
	lines, _ := wrapTooltipLines([]string{"averylongunbrokentooltiptoken"}, nil, 0, 140, 0)
	for _, line := range lines {
		if width := debugTextWidth(line); width > 128 {
			t.Errorf("tooltip line width = %d, want <= 128: %q", width, line)
		}
	}
}
