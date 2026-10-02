package game

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"ugataima/internal/config"
)

// Every highlight kind the markup accepts has a color to draw it in.
func TestEveryKeywordKindHasAColor(t *testing.T) {
	for _, kind := range config.KeywordKinds {
		if _, ok := keywordColors[kind]; !ok {
			t.Errorf("keyword kind %q has no color", kind)
		}
	}
	if len(keywordColors) != len(config.KeywordKinds) {
		t.Errorf("%d colors for %d kinds", len(keywordColors), len(config.KeywordKinds))
	}
}

// Highlights read apart from each other and from the prose around them.
func TestKeywordColorsStandApart(t *testing.T) {
	dist := func(a, b color.RGBA) float64 {
		dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
		return math.Sqrt(dr*dr + dg*dg + db*db)
	}
	for kind, c := range keywordColors {
		if d := dist(c, keywordPlainText); d < 80 {
			t.Errorf("%s is %.0f from the prose color, want 80+", kind, d)
		}
		for other, o := range keywordColors {
			if kind < other && dist(c, o) < 60 {
				t.Errorf("%s and %s are %.0f apart, want 60+", kind, other, dist(c, o))
			}
		}
	}
}

// Wrapped class and race pitches keep every word and highlight, and every line fits
// the party-creation detail column in every font.
func TestClassDescriptionsWrapInEveryFont(t *testing.T) {
	cfg := loadTestConfig(t)
	// The narrowest detail column: the one the smallest window lays out.
	minW, minH := MinimumWindowSize()
	width := heroDetailTextWidth(partyCreateLayout(&partyCreateState{}, minW, minH).detail)
	forEachUIFont(t, func(t *testing.T) {
		var pitches []struct{ key, text string }
		for key, class := range cfg.Characters.Classes {
			for _, p := range class.Description {
				pitches = append(pitches, struct{ key, text string }{key, p.Text})
			}
		}
		for key, race := range cfg.Characters.Races {
			for _, p := range race.Description {
				pitches = append(pitches, struct{ key, text string }{key, p.Text})
			}
		}
		for _, pitch := range pitches {
			key := pitch.key
			spans, err := config.ParseKeywordMarkup(pitch.text)
			if err != nil {
				t.Fatal(err)
			}
			var joined []string
			for _, line := range wrapKeywordText(spans, width) {
				text := ""
				for _, r := range line {
					text += r.text
				}
				if w := uiTextWidth(text); w > width {
					t.Errorf("%s: line %q is %d px, over %d", key, text, w, width)
				}
				joined = append(joined, text)
			}
			if got, want := strings.Join(joined, " "), strings.Join(strings.Fields(config.PlainKeywordText(pitch.text)), " "); got != want {
				t.Errorf("%s: wrapped text\n%q\nwant\n%q", key, got, want)
			}
			// Highlights survive the wrap, word for word.
			var marked, kept []string
			for _, s := range spans {
				for _, word := range strings.Fields(s.Text) {
					if s.Kind != "" {
						marked = append(marked, s.Kind+":"+word)
					}
				}
			}
			for _, line := range wrapKeywordText(spans, width) {
				for _, r := range line {
					if r.kind != "" {
						kept = append(kept, r.kind+":"+r.text)
					}
				}
			}
			if got, want := strings.Join(kept, " "), strings.Join(marked, " "); got != want {
				t.Errorf("%s: highlights\n%q\nwant\n%q", key, got, want)
			}
		}
	})
}
