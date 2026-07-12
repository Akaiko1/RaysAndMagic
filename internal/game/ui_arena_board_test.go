package game

import (
	"strings"
	"testing"
)

func TestWrapArenaBoardLinePreservesDetailIndentAndWidth(t *testing.T) {
	const maxWidth = 180
	lines := wrapArenaBoardLine("   party: Miyabi (Archer L12), Kaede (Warrior L11), Riku (Cleric L10)", maxWidth)
	if len(lines) < 2 {
		t.Fatalf("long detail line = %v, want wrapping", lines)
	}
	for _, line := range lines {
		if got := debugTextWidth(line); got > maxWidth {
			t.Errorf("line width = %d, want <= %d: %q", got, maxWidth, line)
		}
		if !strings.HasPrefix(line, "   ") {
			t.Errorf("detail continuation lost indentation: %q", line)
		}
	}
}
