package game

import (
	"image/color"
	"strings"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// keywordColors draws each config.KeywordKinds highlight, one hue apiece so
// kinds stay apart from each other and from the plain prose.
var keywordColors = map[string]color.RGBA{
	"damage":  {240, 90, 100, 255},
	"defense": {245, 145, 60, 255},
	"auto":    {240, 215, 90, 255},
	"heal":    {120, 220, 120, 255},
	"utility": {95, 215, 215, 255},
	"buff":    {120, 165, 255, 255},
	"control": {200, 140, 250, 255},
}

// keywordPlainText is the prose around the highlights.
var keywordPlainText = color.RGBA{220, 220, 230, 255}

// keywordRun is one colored piece of a wrapped line; kind "" is plain prose.
type keywordRun struct {
	kind string
	text string
}

// wrapKeywordText wraps marked-up prose to maxW, keeping each word whole and
// its highlight intact.
func wrapKeywordText(spans []config.KeywordSpan, maxW int) [][]keywordRun {
	// A word is the runs between two spaces: "{damage:Harm}," is one word.
	var words [][]keywordRun
	var word []keywordRun
	flush := func() {
		if len(word) > 0 {
			words = append(words, word)
			word = nil
		}
	}
	for _, span := range spans {
		for i, piece := range strings.Split(span.Text, " ") {
			if i > 0 {
				flush()
			}
			if piece != "" {
				word = append(word, keywordRun{span.Kind, piece})
			}
		}
	}
	flush()

	var lines [][]keywordRun
	var line []keywordRun
	lineText := ""
	for _, w := range words {
		text := ""
		for _, r := range w {
			text += r.text
		}
		next := text
		if lineText != "" {
			next = lineText + " " + text
		}
		if lineText != "" && uiTextWidth(next) > maxW {
			lines = append(lines, line)
			line, lineText, next = nil, "", text
		} else if lineText != "" {
			line = append(line, keywordRun{"", " "})
		}
		line = append(line, w...)
		lineText = next
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}

// drawKeywordLine draws one wrapped line, plain runs in plain.
func drawKeywordLine(dst *ebiten.Image, line []keywordRun, x, y int, plain color.Color) {
	drawn := ""
	for _, r := range line {
		col := plain
		if r.kind != "" {
			col = keywordColors[r.kind]
		}
		if strings.TrimSpace(r.text) != "" {
			drawUITextColored(dst, r.text, x+uiTextWidth(drawn), y, col)
		}
		drawn += r.text
	}
}
