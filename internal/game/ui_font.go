package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sync"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Interface text sits on lines uiTextCharHeight tall. Glyph widths come from
// the active font; uiTextCharWidth is the classic face's fixed advance and the
// unit in which layouts give text a budget ("n columns" = n*uiTextCharWidth
// pixels).
const (
	uiTextCharWidth  = 6
	uiTextCharHeight = 16
)

// uiGlyphAtlas is one interface font: every accepted rune of each shipped
// script, drawn in the style of Ebitengine's debug sheet (the look the game
// shipped with) - a white glyph over a 50% black copy one pixel down-right. A
// face advancing uiTextCharWidth for every rune (the classic face) keeps that
// sheet's exact layout, runes 0-255 at its own positions, so it matches it
// pixel for pixel. A proportional face gets one cell per glyph with a pixel of
// room on the left for glyphs that start before their pen (serifs,
// underscores) and on the right and bottom for the shadow.
type uiGlyphAtlas struct {
	sheet   *image.RGBA // the drawn cells, kept for tests and tooling
	cells   map[rune]image.Rectangle
	advance map[rune]int
	ascii   [128]int // advance of the common runes, looked up per character
	pad     int      // cell columns left of the pen
	column  float64  // average advance over English prose
	image   *ebiten.Image
	glyphs  map[rune]*ebiten.Image
	missing rune // drawn for a rune no script covers
}

// uiGlyphs is the built-in classic font.
var uiGlyphs = sync.OnceValue(func() *uiGlyphAtlas {
	atlas, err := buildUIGlyphAtlas(uiScripts)
	if err != nil {
		panic(fmt.Sprintf("UI font: %v", err))
	}
	return atlas
})

var (
	// uiActiveFont draws every label; nil is the classic font.
	uiActiveFont *uiGlyphAtlas
	// uiFontGeneration changes with the active font, so caches of measured
	// text can tell they were wrapped for other glyph widths.
	uiFontGeneration int
)

func activeUIFont() *uiGlyphAtlas {
	if uiActiveFont != nil {
		return uiActiveFont
	}
	return uiGlyphs()
}

func setActiveUIFont(a *uiGlyphAtlas) {
	if a == uiGlyphs() {
		a = nil
	}
	if a != uiActiveFont {
		uiActiveFont = a
		uiFontGeneration++
	}
}

const uiGlyphSheetCols = 32

// buildUIGlyphAtlas draws the atlas. A face that lacks an accepted rune,
// advances a fraction of a pixel or draws taller than a text line fails here
// instead of silently misplacing every label.
func buildUIGlyphAtlas(scripts []uiScript) (*uiGlyphAtlas, error) {
	if len(scripts) == 0 {
		return nil, fmt.Errorf("no script ships a face")
	}
	advance := map[rune]int{}
	fixedCell := true
	for _, script := range scripts {
		for r := rune(0); r <= 0xFFFF; r++ {
			if !script.accepts(r) {
				continue
			}
			adv, ok := script.face.GlyphAdvance(r)
			if !ok {
				return nil, fmt.Errorf("%s: no glyph for %q", script.name, r)
			}
			if adv != fixed.I(adv.Round()) || adv <= 0 {
				return nil, fmt.Errorf("%s: %q advances %.2fpx, not a whole pixel", script.name, r, float64(adv)/64)
			}
			if _, dup := advance[r]; !dup {
				advance[r] = adv.Round()
				fixedCell = fixedCell && adv.Round() == uiTextCharWidth
			}
		}
	}
	var atlas *uiGlyphAtlas
	var err error
	if fixedCell {
		atlas, err = buildFixedCellAtlas(scripts)
	} else {
		atlas, err = buildProportionalAtlas(scripts, advance)
	}
	if err != nil {
		return nil, err
	}
	atlas.advance = advance
	for r := range atlas.ascii {
		atlas.ascii[r] = atlas.advanceOf(rune(r))
	}
	atlas.column = float64(atlas.width(uiColumnSample)) / float64(len(uiColumnSample))
	if _, ok := atlas.cells[atlas.missing]; !ok {
		return nil, fmt.Errorf("no script covers the fallback glyph %q", atlas.missing)
	}
	return atlas, nil
}

// buildFixedCellAtlas lays a uiTextCharWidth-cell font out like the debug
// sheet: rows of 32 cells, runes 0-255 of the primary face at the sheet's own
// positions, other accepted runes in later rows.
func buildFixedCellAtlas(scripts []uiScript) (*uiGlyphAtlas, error) {
	type row struct {
		face  font.Face
		runes []rune
	}
	var rows []row
	extra := map[font.Face][]rune{}
	var extraOrder []font.Face
	for i, script := range scripts {
		m := script.face.Metrics()
		if line := m.Ascent.Ceil() + m.Descent.Ceil(); line > uiTextCharHeight {
			return nil, fmt.Errorf("%s: line box %dpx exceeds the %dpx line", script.name, line, uiTextCharHeight)
		}
		if i == 0 {
			for r := 0; r < 256; r += uiGlyphSheetCols {
				runes := make([]rune, uiGlyphSheetCols)
				for k := range runes {
					runes[k] = rune(r + k)
				}
				rows = append(rows, row{script.face, runes})
			}
		}
		for r := rune(0); r <= 0xFFFF; r++ {
			if script.accepts(r) && (i > 0 || r > 255) {
				if extra[script.face] == nil {
					extraOrder = append(extraOrder, script.face)
				}
				extra[script.face] = append(extra[script.face], r)
			}
		}
	}
	for _, face := range extraOrder {
		runes := extra[face]
		for len(runes) > 0 {
			n := min(uiGlyphSheetCols, len(runes))
			rows = append(rows, row{face, runes[:n]})
			runes = runes[n:]
		}
	}
	sheet := image.NewRGBA(image.Rect(0, 0, uiGlyphSheetCols*uiTextCharWidth, len(rows)*uiTextCharHeight))
	atlas := &uiGlyphAtlas{sheet: sheet, cells: map[rune]image.Rectangle{}, glyphs: map[rune]*ebiten.Image{}, missing: '?'}
	// Shadow first, then the glyph, one row of cells per line - the same two
	// passes as Ebitengine's sheet generator, so shadows spill into the next
	// cell just as they always did.
	for pass, ink := range []color.Color{color.RGBA{0, 0, 0, 0x80}, color.White} {
		offset := 1 - pass
		for y, r := range rows {
			d := font.Drawer{Dst: sheet, Src: image.NewUniform(ink), Face: r.face,
				Dot: fixed.Point26_6{X: fixed.I(offset), Y: r.face.Metrics().Ascent + fixed.I(offset+y*uiTextCharHeight)}}
			d.DrawString(string(r.runes))
		}
	}
	for y, r := range rows {
		for x, ch := range r.runes {
			if _, dup := atlas.cells[ch]; !dup {
				atlas.cells[ch] = image.Rect(x*uiTextCharWidth, y*uiTextCharHeight, (x+1)*uiTextCharWidth, (y+1)*uiTextCharHeight)
			}
		}
	}
	return atlas, nil
}

// buildProportionalAtlas gives each glyph its own cell - one line plus its
// shadow row - with the face's ink centred in the line and a spare column and
// row between cells, so no glyph or shadow ever reaches a neighbour.
func buildProportionalAtlas(scripts []uiScript, advance map[rune]int) (*uiGlyphAtlas, error) {
	const pad, sheetW = 1, 512
	type glyph struct {
		face     font.Face
		r        rune
		baseline int
	}
	var glyphs []glyph
	placed := map[rune]bool{}
	for _, script := range scripts {
		top, bottom := 0, 0
		var runes []rune
		for r := rune(0); r <= 0xFFFF; r++ {
			if !script.accepts(r) || placed[r] {
				continue
			}
			placed[r] = true
			runes = append(runes, r)
			b, _, _ := script.face.GlyphBounds(r)
			if !b.Empty() {
				top, bottom = min(top, b.Min.Y.Floor()), max(bottom, b.Max.Y.Ceil())
			}
		}
		// The ink must fit the line; its shadow row too where there is room,
		// or it falls just below, as the classic sheet's does.
		if ink := bottom - top; ink > uiTextCharHeight {
			return nil, fmt.Errorf("%s: glyphs span %dpx, the line is %dpx", script.name, ink, uiTextCharHeight)
		}
		baseline := max(0, (uiTextCharHeight-(bottom-top+1))/2) - top
		for _, r := range runes {
			glyphs = append(glyphs, glyph{script.face, r, baseline})
		}
	}
	cells := map[rune]image.Rectangle{}
	x, y := 0, 0
	for _, g := range glyphs {
		w := pad + advance[g.r] + 1
		if x+w > sheetW {
			x, y = 0, y+uiTextCharHeight+2
		}
		cells[g.r] = image.Rect(x, y, x+w, y+uiTextCharHeight+1)
		x += w + 1
	}
	sheet := image.NewRGBA(image.Rect(0, 0, sheetW, y+uiTextCharHeight+1))
	for pass, ink := range []color.Color{color.RGBA{0, 0, 0, 0x80}, color.White} {
		offset := 1 - pass
		for _, g := range glyphs {
			cell := cells[g.r]
			d := font.Drawer{Dst: sheet, Src: image.NewUniform(ink), Face: g.face,
				Dot: fixed.P(cell.Min.X+pad+offset, cell.Min.Y+g.baseline+offset)}
			d.DrawString(string(g.r))
		}
	}
	// Only whole pixels: a face drawn off its design size, or an outline font,
	// leaves partial coverage that the metallic ramp turns into grey fringes.
	for i := 3; i < len(sheet.Pix); i += 4 {
		if a := sheet.Pix[i]; a != 0 && a != 0x80 && a != 0xff {
			return nil, fmt.Errorf("%s: partial pixels - not a pixel font at this size", scripts[0].name)
		}
	}
	return &uiGlyphAtlas{sheet: sheet, cells: cells, pad: pad, glyphs: map[rune]*ebiten.Image{}, missing: '?'}, nil
}

// uiColumnSample is the prose a font's average advance is taken over.
const uiColumnSample = "Seek the Lich beyond the gate of Silverbough, then bring the lamp back to Yusra at the shrine."

// uiColumnsWidth is a reading measure of n characters: the classic width, or
// n characters of prose in a wider active font, so a wider font keeps the same
// words per line and a narrower one the same column.
func uiColumnsWidth(n int) int {
	return max(n*uiTextCharWidth, int(math.Round(float64(n)*activeUIFont().column)))
}

func (a *uiGlyphAtlas) advanceOf(r rune) int {
	if r >= 0 && r < 128 && a.ascii[r] > 0 {
		return a.ascii[r]
	}
	if adv, ok := a.advance[r]; ok {
		return adv
	}
	return a.advance[a.missing]
}

// width is the pen advance of one line of text.
func (a *uiGlyphAtlas) width(text string) int {
	w := 0
	for _, r := range text {
		w += a.advanceOf(r)
	}
	return w
}

// glyph returns the drawable cell for r, the fallback glyph for an uncovered
// rune. The GPU image is built on first use.
func (a *uiGlyphAtlas) glyph(r rune) *ebiten.Image {
	if _, ok := a.cells[r]; !ok {
		r = a.missing
	}
	if img, ok := a.glyphs[r]; ok {
		return img
	}
	if a.image == nil {
		a.image = ebiten.NewImageFromImage(a.sheet)
	}
	img := a.image.SubImage(a.cells[r]).(*ebiten.Image)
	a.glyphs[r] = img
	return img
}

// raster draws one line of plain text with the pen starting at (x, y), the
// top of the line; glyph cells reach a.pad columns left of the pen.
func (a *uiGlyphAtlas) raster(dst *ebiten.Image, s string, x, y int, colorScale ebiten.ColorScale) {
	op := &ebiten.DrawImageOptions{ColorScale: colorScale}
	pen := x
	for _, r := range s {
		if r != ' ' {
			op.GeoM.Reset()
			op.GeoM.Translate(float64(pen-a.pad), float64(y))
			dst.DrawImage(a.glyph(r), op)
		}
		pen += a.advanceOf(r)
	}
}

// rasterUIText draws one line of plain text in the active font with the pen
// starting at (x, y).
func rasterUIText(dst *ebiten.Image, s string, x, y int, colorScale ebiten.ColorScale) {
	activeUIFont().raster(dst, s, x, y, colorScale)
}

// uiFontAtlases are the configured fonts by key, built once at boot.
var uiFontAtlases = map[string]*uiGlyphAtlas{}

// loadUIFonts builds every configured font, failing on the first one that
// cannot be read or drawn cleanly.
func loadUIFonts(fonts []config.UIFont) error {
	for _, f := range fonts {
		if _, ok := uiFontAtlases[f.Key]; ok {
			continue
		}
		atlas, err := buildConfiguredUIFont(f)
		if err != nil {
			return fmt.Errorf("display.fonts %q: %w", f.Key, err)
		}
		uiFontAtlases[f.Key] = atlas
	}
	return nil
}

func buildConfiguredUIFont(f config.UIFont) (*uiGlyphAtlas, error) {
	if f.Path == "" {
		return uiGlyphs(), nil
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: float64(f.SizePx), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	return buildUIGlyphAtlas(uiScriptsWithLatin(face))
}
