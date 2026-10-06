package game

import (
	"log"

	"ugataima/internal/storage"
)

func displayPreferencesPath() string { return storage.AppSavePath("display_settings.json") }

type displayPreferences struct {
	InterfaceSize string `json:"interface_size"`
	Font          string `json:"font,omitempty"`
}

// loadDisplayPreferences restores the saved interface size and font. A
// missing, unreadable or unknown choice falls back to the configured default.
func (g *MMGame) loadDisplayPreferences() {
	g.config.PlayerInterfaceSize = g.config.Display.DefaultInterfaceSize
	g.config.PlayerFont = g.config.Display.DefaultFont
	prefs, ok := loadSettingsJSON[displayPreferences](displayPreferencesPath(), "display")
	if !ok {
		return
	}
	if _, ok := g.config.Display.InterfaceSize(prefs.InterfaceSize); ok {
		g.config.PlayerInterfaceSize = prefs.InterfaceSize
	} else {
		log.Printf("Unknown interface size %q in display settings; using %q", prefs.InterfaceSize, g.config.PlayerInterfaceSize)
	}
	if _, ok := g.config.Display.UIFont(prefs.Font); ok {
		g.config.PlayerFont = prefs.Font
	} else if prefs.Font != "" {
		log.Printf("Unknown font %q in display settings; using %q", prefs.Font, g.config.PlayerFont)
	}
}

func (g *MMGame) saveDisplayPreferences() {
	if !g.displaySettingsDirty || g.config == nil {
		return
	}
	g.saveSettingsJSON(displayPreferencesPath(), "display", displayPreferences{InterfaceSize: g.interfaceSizeKey(), Font: g.uiFontKey()}, &g.displaySettingsDirty)
}

// interfaceSizeKey is the active preset: the player's choice, else the default.
func (g *MMGame) interfaceSizeKey() string {
	if g == nil || g.config == nil {
		return ""
	}
	if _, ok := g.config.Display.InterfaceSize(g.config.PlayerInterfaceSize); ok {
		return g.config.PlayerInterfaceSize
	}
	return g.config.Display.DefaultInterfaceSize
}

// interfaceSizeIndex is the active preset's position in the configured list.
func (g *MMGame) interfaceSizeIndex() int {
	key := g.interfaceSizeKey()
	for i, size := range g.config.Display.InterfaceSizes {
		if size.Key == key {
			return i
		}
	}
	return 0
}

// interfaceSizeOffered reports whether preset i gives this screen a size of
// its own; one that only repeats the smaller preset is not offered.
func (g *MMGame) interfaceSizeOffered(i int) bool {
	return i >= len(g.interfaceFrames) || !g.interfaceFrames[i].same
}

// setInterfaceSize applies a preset and saves it at once; the next Layout
// call picks up the new logical resolution.
func (g *MMGame) setInterfaceSize(key string) {
	if _, ok := g.config.Display.InterfaceSize(key); !ok || key == g.interfaceSizeKey() {
		return
	}
	g.config.PlayerInterfaceSize = key
	g.displaySettingsDirty = true
	g.saveDisplayPreferences()
}

// stepInterfaceSize moves to the next smaller (dir < 0) or larger preset this
// screen offers.
func (g *MMGame) stepInterfaceSize(dir int) {
	sizes := g.config.Display.InterfaceSizes
	for next := g.interfaceSizeIndex() + dir; next >= 0 && next < len(sizes); next += dir {
		if g.interfaceSizeOffered(next) {
			g.setInterfaceSize(sizes[next].Key)
			return
		}
	}
}

// uiFontKey is the active interface font: the player's choice, else the
// default.
func (g *MMGame) uiFontKey() string {
	if g == nil || g.config == nil {
		return ""
	}
	if _, ok := g.config.Display.UIFont(g.config.PlayerFont); ok {
		return g.config.PlayerFont
	}
	return g.config.Display.DefaultFont
}

// uiFontIndex is the active font's position in the configured list.
func (g *MMGame) uiFontIndex() int {
	key := g.uiFontKey()
	for i, f := range g.config.Display.Fonts {
		if f.Key == key {
			return i
		}
	}
	return 0
}

// applyUIFont makes the chosen font draw every label. A font not built at
// boot (a hand-made test config) leaves the classic face.
func (g *MMGame) applyUIFont() {
	setActiveUIFont(uiFontAtlases[g.uiFontKey()])
}

// setUIFont applies a font and saves it at once.
func (g *MMGame) setUIFont(key string) {
	if _, ok := g.config.Display.UIFont(key); !ok || key == g.uiFontKey() {
		return
	}
	g.config.PlayerFont = key
	g.applyUIFont()
	g.displaySettingsDirty = true
	g.saveDisplayPreferences()
}

// stepUIFont moves to the previous (dir < 0) or next font, wrapping around.
func (g *MMGame) stepUIFont(dir int) {
	fonts := g.config.Display.Fonts
	if len(fonts) == 0 {
		return
	}
	next := (g.uiFontIndex() + dir + len(fonts)) % len(fonts)
	g.setUIFont(fonts[next].Key)
}
