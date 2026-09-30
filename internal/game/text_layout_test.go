package game

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Every line fits its width in every shipped font, including tokens wider
// than the line and scripts the fonts do not cover yet.
func TestWrapUITextFitsInEveryFont(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "words", text: "one two three four five"},
		{name: "oversized token", text: strings.Repeat("x", 31)},
		{name: "wide glyphs", text: strings.Repeat("W", 31) + " MMMM WWW"},
		{name: "cyrillic", text: "\u0434\u043B\u0438\u043D\u043D\u0430\u044F \u0441\u0442\u0440\u043E\u043A\u0430 \u0434\u043B\u044F \u043F\u0440\u043E\u0432\u0435\u0440\u043A\u0438 \u043F\u0435\u0440\u0435\u043D\u043E\u0441\u0430"},
		{name: "armenian", text: "\u0565\u0580\u056F\u0561\u0580 \u057F\u0578\u0572 \u0583\u0578\u0580\u0571\u0561\u0580\u056F\u0574\u0561\u0576 \u0570\u0561\u0574\u0561\u0580"},
		{name: "hebrew", text: "\u05E9\u05D5\u05E8\u05D4 \u05D0\u05E8\u05D5\u05DB\u05D4 \u05DC\u05D1\u05D3\u05D9\u05E7\u05EA \u05D4\u05D2\u05DC\u05D9\u05E9\u05D4"},
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				const width = 10 * uiTextCharWidth
				lines := wrapUIText(tt.text, width)
				if len(lines) == 0 {
					t.Fatal("wrapUIText returned no lines")
				}
				if got := strings.Join(lines, ""); strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(tt.text, " ", "") {
					t.Fatalf("wrapping lost text: %q", lines)
				}
				for _, line := range lines {
					if got := uiTextWidth(line); got > width {
						t.Errorf("line is %dpx, want <= %d: %q", got, width, line)
					}
				}
			})
		}
	})
}

// Multi-byte runes (Cyrillic, Armenian, Hebrew - written as escapes so the
// source stays ASCII) keep the text helpers safe for scripts added later.
func TestRuneSafeTextEditingAndTruncation(t *testing.T) {
	if got := appendRunesLimited("\u041C\u0438\u044F", []rune("\u0431\u0438XYZ"), 5); got != "\u041C\u0438\u044F\u0431\u0438" {
		t.Fatalf("appendRunesLimited = %q, want %q", got, "\u041C\u0438\u044F\u0431\u0438")
	}
	if got := removeLastRune("\u041C\u0438\u044F\u0431\u0438"); got != "\u041C\u0438\u044F\u0431" {
		t.Fatalf("removeLastRune = %q, want %q", got, "\u041C\u0438\u044F\u0431")
	}
	if got := truncateName("\u0410\u043B\u0435\u043A\u0441\u0430\u043D\u0434\u0440\u0430", 6); got != "\u0410\u043B\u0435\u043A.." {
		t.Fatalf("truncateName = %q, want %q", got, "\u0410\u043B\u0435\u043A..")
	}
	if !utf8.ValidString(truncateSaveName("\u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u0438\u0435", 7)) {
		t.Fatal("truncateSaveName produced invalid UTF-8")
	}
	if got := clipUIText("\u0434\u043B\u0438\u043D\u043D\u043E\u0435", 6*uiTextCharWidth); got != "\u0434\u043B\u0438\u043D.." {
		t.Fatalf("clipUIText = %q, want %q", got, "\u0434\u043B\u0438\u043D..")
	}
}
