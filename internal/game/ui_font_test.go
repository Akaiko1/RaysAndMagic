package game

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/bitmapfont/v4"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// uiAcceptedRunes lists every BMP rune a shipped script accepts (the scripts
// planned next - Cyrillic, Armenian, Hebrew - all live in the BMP).
func uiAcceptedRunes() []rune {
	var runes []rune
	for r := rune(0); r <= 0xFFFF; r++ {
		if uiTextAccepts(r) {
			runes = append(runes, r)
		}
	}
	return runes
}

// The atlas reproduces Ebitengine's debug sheet (testdata copy of
// ebitenutil/text.png, Apache-2.0) pixel for pixel for runes 0-255: the text
// keeps exactly the look the game shipped with, now drawn by our own code.
func TestUIGlyphAtlasMatchesEbitengineSheet(t *testing.T) {
	f, err := os.Open("testdata/ebitenutil_text.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	golden, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := buildUIGlyphAtlas(uiScripts)
	if err != nil {
		t.Fatal(err)
	}
	b := golden.Bounds()
	if b.Dx() != uiGlyphSheetCols*uiTextCharWidth || b.Dy() != 8*uiTextCharHeight {
		t.Fatalf("golden sheet is %v, want 32x8 cells", b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			want := color.RGBAModel.Convert(golden.At(x, y))
			if got := atlas.sheet.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, sheet has %v", x, y, got, want)
			}
		}
	}
}

// Every accepted rune owns one 6x16 cell and every visible one draws white
// ink there; space draws none.
func TestUIGlyphAtlasIsComplete(t *testing.T) {
	atlas, err := buildUIGlyphAtlas(uiScripts)
	if err != nil {
		t.Fatal(err)
	}
	accepted := uiAcceptedRunes()
	if len(accepted) == 0 {
		t.Fatal("no script accepts any rune")
	}
	for _, r := range accepted {
		cell, ok := atlas.cells[r]
		if !ok || cell.Dx() != uiTextCharWidth || cell.Dy() != uiTextCharHeight {
			t.Fatalf("%q cell %v, want %dx%d", r, cell, uiTextCharWidth, uiTextCharHeight)
		}
		if ink := hasWhiteInk(atlas.sheet, cell); ink != (r != ' ') {
			t.Errorf("%q white ink = %v", r, ink)
		}
	}
}

// offCellFace breaks one property of the real face for the rejection rows.
type offCellFace struct {
	font.Face
	advance   fixed.Int26_6
	missing   rune
	tallLines bool
}

func (f offCellFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	if r == f.missing {
		return 0, false
	}
	if r == 'A' && f.advance != 0 {
		return f.advance, true
	}
	return f.Face.GlyphAdvance(r)
}

func (f offCellFace) Metrics() font.Metrics {
	m := f.Face.Metrics()
	if f.tallLines {
		m.Ascent = fixed.I(uiTextCharHeight)
	}
	return m
}

// A face that would misplace labels is refused when the atlas is built; a
// proportional one is a font of its own.
func TestUIGlyphAtlasRejectsOffGridFaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		face offCellFace
		ok   bool
	}{
		{"real face", offCellFace{Face: bitmapfont.Face}, true},
		{"proportional advance", offCellFace{Face: bitmapfont.Face, advance: fixed.I(8)}, true},
		{"fractional advance", offCellFace{Face: bitmapfont.Face, advance: fixed.I(6) + 32}, false},
		{"missing glyph", offCellFace{Face: bitmapfont.Face, missing: 'A'}, false},
		{"tall line box", offCellFace{Face: bitmapfont.Face, tallLines: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := uiScript{name: "test", accepts: func(r rune) bool { return r == 'A' || r == '?' }, face: tc.face}
			_, err := buildUIGlyphAtlas([]uiScript{script})
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func hasWhiteInk(sheet *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := sheet.RGBAAt(x, y); c.A == 0xff && c.R == 0xff {
				return true
			}
		}
	}
	return false
}

// shippedUIFonts builds every configured font, in config order.
func shippedUIFonts(t testing.TB) []config.UIFont {
	t.Helper()
	cfg := loadTestConfig(t)
	if err := loadUIFonts(cfg.Display.Fonts); err != nil {
		t.Fatal(err)
	}
	return cfg.Display.Fonts
}

// withUIFont makes key the active font for the rest of the test.
func withUIFont(t testing.TB, key string) {
	t.Helper()
	prev := uiActiveFont
	t.Cleanup(func() { setActiveUIFont(prev) })
	atlas := uiFontAtlases[key]
	if atlas == nil {
		t.Fatalf("font %q was not built", key)
	}
	setActiveUIFont(atlas)
}

// forEachUIFont runs fn as a subtest under every shipped font.
func forEachUIFont(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	for _, f := range shippedUIFonts(t) {
		t.Run("font="+f.Key, func(t *testing.T) {
			withUIFont(t, f.Key)
			fn(t)
		})
	}
}

// Every shipped font draws every accepted rune in a cell of its own: whole
// pixels only, visible ink for every visible rune, and nothing drawn outside
// the cells, so no glyph or shadow bleeds into a neighbour.
func TestShippedUIFontsDrawCleanCells(t *testing.T) {
	for _, f := range shippedUIFonts(t) {
		t.Run(f.Key, func(t *testing.T) {
			atlas := uiFontAtlases[f.Key]
			inCell := image.NewAlpha(atlas.sheet.Bounds())
			for _, r := range uiAcceptedRunes() {
				cell, ok := atlas.cells[r]
				if !ok || atlas.advanceOf(r) <= 0 {
					t.Fatalf("%q: cell %v ok=%v advance %d", r, cell, ok, atlas.advanceOf(r))
				}
				if ink := hasWhiteInk(atlas.sheet, cell); ink != (r != ' ') {
					t.Errorf("%q white ink = %v", r, ink)
				}
				draw.Draw(inCell, cell, image.Opaque, image.Point{}, draw.Src)
			}
			if atlas.pad == 0 {
				return // the classic sheet shares cell edges by design
			}
			b := atlas.sheet.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if inCell.AlphaAt(x, y).A == 0 && atlas.sheet.RGBAAt(x, y).A != 0 {
						t.Fatalf("ink at (%d,%d) outside every cell", x, y)
					}
				}
			}
		})
	}
}

// Labels are measured by the pen advance of the active font; clipping never
// overruns its box and keeps as much text as fits.
func TestUITextMetricsFollowTheActiveFont(t *testing.T) {
	forEachUIFont(t, func(t *testing.T) {
		atlas := activeUIFont()
		text := "Seek the Lich beyond the gate of Silverbough."
		sum := 0
		for _, r := range text {
			sum += atlas.advanceOf(r)
		}
		if uiTextWidth(text) != sum {
			t.Fatalf("width %d, advances sum to %d", uiTextWidth(text), sum)
		}
		for _, maxW := range []int{0, 5, 12, 40, 97, sum - 1, sum} {
			got := clipUIText(text, maxW)
			if uiTextWidth(got) > maxW || maxW >= sum && got != text {
				t.Fatalf("clip to %d gave %q (%dpx)", maxW, got, uiTextWidth(got))
			}
			if got != text && got != "" && strings.HasSuffix(got, "..") {
				kept := strings.TrimSuffix(got, "..")
				next := text[:len(kept)+1]
				if uiTextWidth(next+"..") <= maxW {
					t.Fatalf("clip to %d stopped early at %q", maxW, got)
				}
			}
		}
	})
}
