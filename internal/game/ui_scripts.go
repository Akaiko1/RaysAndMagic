package game

import (
	"github.com/hajimehoshi/bitmapfont/v4"
	"golang.org/x/image/font"
)

// uiScript is one writing system the interface can show and accept from the
// keyboard: the runes that belong to it and the face that draws them. Only
// English ships. Russian, Armenian or Hebrew join with one row and a pixel
// face at most one text line tall - their runes enter every font's glyph
// atlas and typed text follows automatically. Hebrew additionally needs
// right-to-left layout and width measuring that skips combining marks.
type uiScript struct {
	name    string
	accepts func(r rune) bool
	face    font.Face // a pixel face: whole-pixel advances, no partial pixels
}

var uiScripts = []uiScript{
	{
		name:    "English",
		accepts: func(r rune) bool { return r >= ' ' && r <= '~' },
		face:    bitmapfont.Face,
	},
}

// uiScriptsWithLatin is the shipped scripts with a chosen interface font
// drawing English; the other scripts keep their own faces.
func uiScriptsWithLatin(face font.Face) []uiScript {
	scripts := append([]uiScript(nil), uiScripts...)
	scripts[0].face = face
	return scripts
}

// uiTextAccepts reports whether r belongs to a shipped script, i.e. a player
// may type it and the interface can draw it.
func uiTextAccepts(r rune) bool {
	for _, script := range uiScripts {
		if script.accepts(r) {
			return true
		}
	}
	return false
}

// AcceptTypedRunes keeps, in order, the typed runes the interface can show.
// Every text field (game and map editor) filters keyboard input through it.
func AcceptTypedRunes(input []rune) []rune {
	kept := input[:0:0]
	for _, r := range input {
		if uiTextAccepts(r) {
			kept = append(kept, r)
		}
	}
	return kept
}
