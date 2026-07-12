package game

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrapTextGuaranteesRuneWidth(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "words", text: "one two three four five"},
		{name: "oversized token", text: strings.Repeat("x", 31)},
		{name: "unicode", text: "длинная строка для проверки переноса"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := wrapText(tt.text, 10)
			if len(lines) == 0 {
				t.Fatal("wrapText returned no lines")
			}
			for _, line := range lines {
				if got := utf8.RuneCountInString(line); got > 10 {
					t.Errorf("line has %d runes, want <= 10: %q", got, line)
				}
			}
		})
	}
}

func TestRuneSafeTextEditingAndTruncation(t *testing.T) {
	if got := appendRunesLimited("Мия", []rune("биXYZ"), 5); got != "Мияби" {
		t.Fatalf("appendRunesLimited = %q, want %q", got, "Мияби")
	}
	if got := removeLastRune("Мияби"); got != "Мияб" {
		t.Fatalf("removeLastRune = %q, want %q", got, "Мияб")
	}
	if got := truncateName("Александра", 6); got != "Алек.." {
		t.Fatalf("truncateName = %q, want %q", got, "Алек..")
	}
	if !utf8.ValidString(truncateSaveName("Сохранение", 7)) {
		t.Fatal("truncateSaveName produced invalid UTF-8")
	}
	if got := clipDebugText("длинное", 6*debugTextCharWidth); got != "длин.." {
		t.Fatalf("clipDebugText = %q, want %q", got, "длин..")
	}
}
