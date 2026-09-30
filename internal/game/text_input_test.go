package game

import (
	"strings"
	"testing"
)

// Every text field keeps exactly the typed runes a shipped script accepts -
// English only for now - through the real handlers, and still honours its
// length cap after filtering.
func TestTextFieldsKeepOnlyShippedScripts(t *testing.T) {
	cases := []struct {
		name, typed, want string
	}{
		{"english", "Gareth the Bold", "Gareth the Bold"},
		{"punctuation and tilde", "A-1 (x)_~!", "A-1 (x)_~!"},
		{"cyrillic", "\u0413\u0430\u0440\u0435\u0442", ""},
		{"armenian mixed", "Ari\u0561\u0580n", "Arin"},
		{"hebrew", "\u05E9\u05DC\u05D5\u05DD", ""},
		{"latin accents", "Jos\u00E9 M\u00FCller", "Jos Mller"},
		{"control runes", "a\tb\nc\rd\x00", "abcd"},
		{"emoji", "Hero\U0001F409", "Hero"},
	}
	fields := []struct {
		name  string
		limit int
		input func(ih *InputHandler) string
	}{
		{"victory name", 20, func(ih *InputHandler) string {
			ih.handleVictoryNameInput()
			return ih.game.victoryNameInput
		}},
		{"save rename", 24, func(ih *InputHandler) string {
			ih.handleSaveRenameInput()
			return ih.game.saveRenameInput
		}},
	}
	for _, field := range fields {
		for _, tc := range cases {
			t.Run(field.name+"/"+tc.name, func(t *testing.T) {
				g := newTestGame(loadTestConfig(t), nil)
				ih := NewInputHandler(g)
				prev := typedInputChars
				t.Cleanup(func() { typedInputChars = prev })
				typedInputChars = func() []rune { return []rune(tc.typed) }
				if got := field.input(ih); got != tc.want {
					t.Fatalf("typed %q kept %q, want %q", tc.typed, got, tc.want)
				}
			})
		}
		t.Run(field.name+"/cap after filtering", func(t *testing.T) {
			g := newTestGame(loadTestConfig(t), nil)
			ih := NewInputHandler(g)
			prev := typedInputChars
			t.Cleanup(func() { typedInputChars = prev })
			typedInputChars = func() []rune { return []rune(strings.Repeat("\u0414x", 40)) }
			if got := field.input(ih); got != strings.Repeat("x", field.limit) {
				t.Fatalf("kept %q, want %d ASCII runes", got, field.limit)
			}
		})
	}
}
