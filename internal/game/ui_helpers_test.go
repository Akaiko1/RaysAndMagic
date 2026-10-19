package game

import (
	"fmt"
	"strings"
	"testing"
	"ugataima/internal/character"
)

// A line that just fits a tooltip column wraps once the icon takes its share
// of the width.
func TestTooltipLayoutAccountsForIconOffset(t *testing.T) {
	line := "one two three four five"
	rows := character.PlainCardRows([]string{line})
	width := uiTextWidth(line) + 12
	if got := len(tooltipLayoutRows(rows, false, width, 16).rows); got != 1 {
		t.Fatalf("without icon: %d rows, want 1", got)
	}
	if got := len(tooltipLayoutRows(rows, true, width, 16).rows); got <= 1 {
		t.Fatalf("with icon: %d rows, want the line wrapped", got)
	}
}

func TestSingleTooltipStaysInViewport(t *testing.T) {
	for _, res := range withInterfaceFrames(t, campHUDResolutions) {
		for _, icon := range []bool{false, true} {
			for _, text := range []string{"Camp", "Restores living party members' HP and SP.", strings.Repeat("Long tooltip words ", 50), strings.Repeat("A", 300)} {
				for _, point := range [][2]int{{-10, -10}, {0, 0}, {res[0] / 2, res[1] / 2}, {res[0] + 12, 0}, {0, res[1] + 8}, {res[0] + 12, res[1] + 8}} {
					t.Run(fmt.Sprintf("%dx%d/icon=%v/len=%d/%d,%d", res[0], res[1], icon, len(text), point[0], point[1]), func(t *testing.T) {
						lines := []string{text}
						r := singleTooltipLayout(lines, nil, icon, point[0], point[1], res[0], res[1])
						if r.x < tooltipScreenMargin || r.y < tooltipScreenMargin || r.right() > res[0]-tooltipScreenMargin || r.bottom() > res[1]-tooltipScreenMargin {
							t.Fatalf("tooltip outside viewport: %+v", r)
						}
						w, h := tooltipBoxSizeForScreen(lines, nil, icon, r.x, r.right(), res[1])
						if w != r.w || h != r.h {
							t.Fatalf("draw wrapping differs from placement: %dx%d vs %+v", w, h, r)
						}
					})
				}
			}
		}
	}
}

func TestTooltipLayoutSplitsOversizedToken(t *testing.T) {
	layout := tooltipLayoutRows(character.PlainCardRows([]string{"averylongunbrokentooltiptoken"}), false, 140, 16)
	for _, row := range layout.rows {
		if width := uiTextWidth(row.text); width > 128 {
			t.Errorf("tooltip line width = %d, want <= 128: %q", width, row.text)
		}
	}
}
