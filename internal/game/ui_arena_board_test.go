package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/arena"
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

func TestArenaBoardDetailToggleKeepsCenteredEntry(t *testing.T) {
	board := &arena.Board{}
	for i := 0; i < 12; i++ {
		board.Entries = append(board.Entries, arena.Entry{
			RunID:       fmt.Sprintf("run-%02d", i),
			Members:     []arena.Member{{Name: fmt.Sprintf("Hero %02d", i), Class: "Archer", Level: 10}},
			Kills:       map[string]map[string]int{"Armsmaster": {"champion": 1}},
			TotalPoints: 100 - i,
		})
	}
	const visible = 5
	compact := buildArenaBoardLinesFrom(board, false, 500)
	oldScroll := 5
	anchor := arenaBoardAnchor(compact, oldScroll, visible)
	if anchor == "" {
		t.Fatal("compact board produced no anchor")
	}

	expanded := buildArenaBoardLinesFrom(board, true, 500)
	newScroll := arenaBoardScrollForAnchor(expanded, anchor, visible)
	if got := arenaBoardAnchor(expanded, newScroll, visible); got != anchor {
		t.Fatalf("detail toggle centered %q, want %q", got, anchor)
	}
}

func TestArenaBoardScrollAcceptsFractionalWheelAndReturnsToTop(t *testing.T) {
	if got := arenaBoardScrollAfterWheel(0, -0.2); got != 3 {
		t.Fatalf("fractional down-scroll = %d, want 3", got)
	}
	if got := arenaBoardScrollAfterWheel(1, 0.2); got != 0 {
		t.Fatalf("fractional up-scroll from line 2 = %d, want 0", got)
	}
	if got := arenaBoardScrollAfterWheel(0, 0.2); got != 0 {
		t.Fatalf("up-scroll at top = %d, want 0", got)
	}
}

func TestArenaBoardShiftWheelUsesHorizontalDeltaOnMac(t *testing.T) {
	if got := arenaBoardWheelDelta(0.25, 0, true); got != 0.25 {
		t.Fatalf("shift wheel delta = %v, want horizontal 0.25", got)
	}
	if got := arenaBoardWheelDelta(0.25, 0, false); got != 0 {
		t.Fatalf("plain horizontal scroll delta = %v, want 0", got)
	}
	if got := arenaBoardWheelDelta(0.25, -0.5, true); got != -0.5 {
		t.Fatalf("vertical delta must win over horizontal delta, got %v", got)
	}
}
