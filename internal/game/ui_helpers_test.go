package game

import (
	"image/color"
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

func TestWrapTooltipLinesSplitsOversizedToken(t *testing.T) {
	lines, _ := wrapTooltipLines([]string{"averylongunbrokentooltiptoken"}, nil, 0, 140, 0)
	for _, line := range lines {
		if width := debugTextWidth(line); width > 128 {
			t.Errorf("tooltip line width = %d, want <= 128: %q", width, line)
		}
	}
}
