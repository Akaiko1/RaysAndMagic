package game

import (
	"image/color"

	"ugataima/internal/character"
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

// wrapKeywordText wraps marked-up prose to maxW, every word keeping its
// highlight.
func wrapKeywordText(spans []config.KeywordSpan, maxW int) [][]keywordRun {
	runs := make([]styledRun[string], len(spans))
	for i, s := range spans {
		runs[i] = styledRun[string]{s.Text, s.Kind}
	}
	var lines [][]keywordRun
	for _, line := range wrapStyled(runs, maxW) {
		row := make([]keywordRun, len(line))
		for i, r := range line {
			row[i] = keywordRun{r.style, r.text}
		}
		lines = append(lines, row)
	}
	return lines
}

// drawKeywordLine draws one wrapped line, plain runs in plain.
func drawKeywordLine(dst *ebiten.Image, line []keywordRun, x, y int, plain color.Color) {
	segs := make([]coloredTextSegment, len(line))
	for i, r := range line {
		segs[i] = coloredTextSegment{r.text, plain}
		if r.kind != "" {
			segs[i].color = keywordColors[r.kind]
		}
	}
	drawColoredTextSegments(dst, x, y, segs)
}

// styledRun is a piece of text in one style: a keyword kind, a color.
type styledRun[S any] struct {
	text  string
	style S
}

// wrapStyled wraps styled text with exactly wrapUIText's breaks: the plain
// text is wrapped, then every byte of each line takes its piece's style.
func wrapStyled[S any](runs []styledRun[S], maxW int) [][]styledRun[S] {
	return styledLines(runs, func(plain string) []string { return wrapUIText(plain, maxW) })
}

// styledLines gives every byte of the lines cut cuts from the runs' text -
// whitespace collapsed, as wrapUIText and the tooltip layout cut it - the
// style of the run it came from.
func styledLines[S any](runs []styledRun[S], cut func(plain string) []string) [][]styledRun[S] {
	if len(runs) == 0 {
		return nil
	}
	plain := ""
	var owner []int // run index of each byte of plain
	for i, r := range runs {
		plain += r.text
		for range len(r.text) {
			owner = append(owner, i)
		}
	}
	space := character.IsCardSpace
	var out [][]styledRun[S]
	at, last := 0, 0
	for _, text := range cut(plain) {
		var line []styledRun[S]
		start := 0 // where the line's last piece begins in text
		emit := func(k, run int) {
			if n := len(line); n > 0 && last == run {
				line[n-1].text = text[start : k+1]
				return
			}
			line = append(line, styledRun[S]{text[k : k+1], runs[run].style})
			start, last = k, run
		}
		for k := 0; k < len(text); k++ {
			for at < len(plain) && plain[at] != text[k] && space(plain[at]) {
				at++
			}
			if at >= len(plain) || plain[at] != text[k] {
				emit(k, last) // unreachable for lines cut from plain
				continue
			}
			emit(k, owner[at])
			at++
			// A run of spaces the cut collapsed belongs to this one.
			for text[k] == ' ' && at < len(plain) && space(plain[at]) && (k+1 >= len(text) || text[k+1] != ' ') {
				at++
			}
		}
		out = append(out, line)
	}
	return out
}
