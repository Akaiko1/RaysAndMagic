package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The "..." suffix is 3 chars; both truncators must reserve room for it so a
// clipped string never exceeds its limit (the bug: an "..."-suffixed result
// that was 2 chars wider than the column and spilled past it).

func TestClipTextNeverExceedsLimit(t *testing.T) {
	const glyphW = 6
	long := strings.Repeat("W", 40)
	for availPx := 0; availPx <= 120; availPx += 3 {
		maxChars := availPx / glyphW
		got := clipText(long, availPx)
		if len(got) > maxChars {
			t.Errorf("clipText(availPx=%d): len=%d exceeds maxChars=%d (%q)", availPx, len(got), maxChars, got)
		}
	}
	// Sanity: a wide budget still gets an ellipsis when it actually truncates.
	if got := clipText(long, 120); !strings.HasSuffix(got, "...") {
		t.Errorf("expected an ellipsis on a truncated wide budget, got %q", got)
	}
}

func TestTruncateNeverExceedsLimit(t *testing.T) {
	long := strings.Repeat("W", 40)
	for n := 0; n <= 20; n++ {
		got := truncate(long, n)
		if utf8.RuneCountInString(got) > n {
			t.Errorf("truncate(maxRunes=%d): runes=%d exceeds limit (%q)", n, utf8.RuneCountInString(got), got)
		}
	}
	if got := truncate(long, 10); !strings.HasSuffix(got, "...") {
		t.Errorf("expected an ellipsis on a truncated string, got %q", got)
	}
}
