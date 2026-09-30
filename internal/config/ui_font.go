package config

import (
	"fmt"
	"path/filepath"
)

// UIFont is one Settings > Display font choice. File is a pixel font drawn at
// its design size SizePx, resolved against the config file's folder; an
// empty File is the built-in classic face.
type UIFont struct {
	Key    string `yaml:"key"`
	Label  string `yaml:"label"`
	File   string `yaml:"file"`
	SizePx int    `yaml:"size_px"`
	Credit string `yaml:"credit"`

	// Path is File resolved at load time ("" for the built-in face).
	Path string `yaml:"-"`
}

const (
	minUIFontPx = 6
	maxUIFontPx = 16 // one interface text line
)

// UIFont resolves a font by its saved key.
func (d DisplayConfig) UIFont(key string) (UIFont, bool) {
	for _, f := range d.Fonts {
		if f.Key == key {
			return f, true
		}
	}
	return UIFont{}, false
}

// resolveFonts checks the font list and resolves every file against dir.
func (d *DisplayConfig) resolveFonts(dir string) error {
	if len(d.Fonts) == 0 {
		return fmt.Errorf("display.fonts is required")
	}
	seen := make(map[string]bool, len(d.Fonts))
	for i := range d.Fonts {
		f := &d.Fonts[i]
		where := fmt.Sprintf("display.fonts[%d]", i)
		switch {
		case f.Key == "":
			return fmt.Errorf("%s: key is required", where)
		case seen[f.Key]:
			return fmt.Errorf("%s: duplicate key %q", where, f.Key)
		case f.Label == "":
			return fmt.Errorf("%s: label is required", where)
		case !isASCII(f.Label) || !isASCII(f.Credit):
			return fmt.Errorf("%s: label and credit must be ASCII", where)
		case f.File == "" && f.SizePx != 0:
			return fmt.Errorf("%s: size_px applies to a font file only", where)
		case f.File != "" && (f.SizePx < minUIFontPx || f.SizePx > maxUIFontPx):
			return fmt.Errorf("%s: size_px %d must be within %d..%d", where, f.SizePx, minUIFontPx, maxUIFontPx)
		}
		if f.File != "" {
			// The game opens and checks the file itself when it builds the
			// font at boot.
			f.Path = filepath.Join(dir, f.File)
		}
		seen[f.Key] = true
	}
	if _, ok := d.UIFont(d.DefaultFont); !ok {
		return fmt.Errorf("display.default_font %q is not one of display.fonts", d.DefaultFont)
	}
	return nil
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
