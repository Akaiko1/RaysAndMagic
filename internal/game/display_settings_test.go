package game

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/storage"
)

// The interface size shrinks the logical frame by its scale, never below the
// UI minimum, and keeps the window's aspect. Scale 1 is the native layout.
func TestLogicalScreenSizeInterfaceScale(t *testing.T) {
	type size struct{ w, h int }
	cases := []struct {
		name   string
		window size
		want   [3]size // scale 1, 1.25, 1.5
	}{
		{"1080p", size{1920, 1080}, [3]size{{1920, 1080}, {1536, 864}, {1280, 720}}},
		{"1440p", size{2560, 1440}, [3]size{{1920, 1080}, {1536, 864}, {1280, 720}}},
		{"4k", size{3840, 2160}, [3]size{{1920, 1080}, {1536, 864}, {1280, 720}}},
		{"ultrawide", size{3440, 1440}, [3]size{{2580, 1080}, {2064, 864}, {1720, 720}}},
		{"macbook", size{1512, 982}, [3]size{{1512, 982}, {1210, 786}, {1047, 680}}},
		{"768p capped", size{1366, 768}, [3]size{{1366, 768}, {1209, 680}, {1209, 680}}},
		{"720p capped", size{1280, 720}, [3]size{{1280, 720}, {1209, 680}, {1209, 680}}},
		{"small window", size{640, 480}, [3]size{{907, 680}, {907, 680}, {907, 680}}},
		{"portrait", size{1080, 1920}, [3]size{{800, 1422}, {800, 1422}, {800, 1422}}},
	}
	minW, minH := MinimumWindowSize()
	for _, tc := range cases {
		for i, scale := range []float64{1, 1.25, 1.5} {
			t.Run(fmt.Sprintf("%s/x%.2f", tc.name, scale), func(t *testing.T) {
				w, h := logicalScreenSize(tc.window.w, tc.window.h, scale)
				if want := tc.want[i]; w != want.w || h != want.h {
					t.Fatalf("logical %dx%d, want %dx%d", w, h, want.w, want.h)
				}
				if w < minW || h < minH {
					t.Fatalf("logical %dx%d below the UI minimum %dx%d", w, h, minW, minH)
				}
				if drift := math.Abs(float64(w)/float64(h) - float64(tc.window.w)/float64(tc.window.h)); drift > 0.01 {
					t.Fatalf("logical %dx%d changes the window aspect by %.3f", w, h, drift)
				}
			})
		}
	}
}

// Layout is the real entry. On every test display each saved preset lays the
// interface out in its frame's UI units, draws it at the screen's native
// resolution (a whole number of pixels per unit wherever the frame is sharp)
// and leaves the 3D view at Normal's frame, whatever the preset.
func TestInterfaceSizeReachesLayout(t *testing.T) {
	for _, d := range interfaceTestDisplays {
		t.Run(d.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			prev := displayDeviceScale
			t.Cleanup(func() { displayDeviceScale = prev })
			displayDeviceScale = func() float64 { return d.scale }
			nativeW, nativeH := int(math.Round(float64(d.outW)*d.scale)), int(math.Round(float64(d.outH)*d.scale))
			for i, size := range h.g.config.Display.InterfaceSizes {
				h.g.config.PlayerInterfaceSize = size.Key
				w, hh := h.loop.Layout(d.outW, d.outH)
				want := d.want[i]
				if h.g.config.GetScreenWidth() != want.w || h.g.config.GetScreenHeight() != want.h {
					t.Fatalf("%s: interface frame %dx%d, want %dx%d", size.Key,
						h.g.config.GetScreenWidth(), h.g.config.GetScreenHeight(), want.w, want.h)
				}
				if ww, wh := h.g.worldWidth(), h.g.worldHeight(); ww != d.want[0].w || wh != d.want[0].h {
					t.Fatalf("%s: world %dx%d, want Normal's %dx%d", size.Key, ww, wh, d.want[0].w, d.want[0].h)
				}
				scale := h.g.uiPixelScale()
				if want.sharp && (scale != math.Round(scale) || w != want.w*int(scale) || hh != want.h*int(scale)) {
					t.Fatalf("%s: sharp frame drawn %dx%d at x%.4f, want whole pixels per unit", size.Key, w, hh, scale)
				}
				// The screen shows the whole interface and leaves less than one
				// unit of the window unused.
				if w > nativeW || hh > nativeH || float64(nativeW-w) >= scale+1 || float64(nativeH-hh) >= scale+1 {
					t.Fatalf("%s: Layout %dx%d at x%.4f on a %dx%d screen", size.Key, w, hh, scale, nativeW, nativeH)
				}
				if len(h.g.interfaceFrames) != len(d.want) || h.g.interfaceFrames[i].same != want.same {
					t.Fatalf("%s: Settings sees frames %+v", size.Key, h.g.interfaceFrames)
				}
			}
		})
	}
}

func readDisplayPreference(t *testing.T) string {
	t.Helper()
	g := &MMGame{config: loadTestConfig(t)}
	g.config.PlayerInterfaceSize = ""
	g.loadDisplayPreferences()
	return g.config.PlayerInterfaceSize
}

// Settings > Display drives the preset through the displayed buttons and the
// arrow keys, saves every change at once and leaves the other tabs intact.
func TestDisplaySettingsControls(t *testing.T) {
	for _, entry := range []bool{false, true} {
		t.Run(fmt.Sprintf("entry=%v", entry), func(t *testing.T) {
			h, _ := audioGestureHarness(t, entry, 0)
			g := h.g
			g.config.PlayerInterfaceSize = g.config.Display.DefaultInterfaceSize
			fp := installFakePointer(t)
			layout := makeAudioSettingsPanelLayout(1024, 768, entry)
			click := func(r layoutRect) {
				presentInputScreen(h)
				fp.moveTo(r.x+r.w/2, r.y+r.h/2)
				fp.press()
				updateInputScreen(h)
				fp.release()
				updateInputScreen(h)
			}
			key := func(k ebiten.Key) {
				presentInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(p ebiten.Key) bool { return p == k })
				updateInputScreen(h)
			}
			click(settingsTabRect(layout.px, layout.py, layout.panelW, settingsTabDisplay))
			if g.settingsTab != settingsTabDisplay {
				t.Fatal("Display tab did not open")
			}
			sizes := g.config.Display.InterfaceSizes
			for i := len(sizes) - 1; i >= 0; i-- {
				click(displaySizeButtonRect(layout.px, layout.py, layout.panelW, i, len(sizes)))
				if g.interfaceSizeKey() != sizes[i].Key || g.interfaceSizeIndex() != i {
					t.Fatalf("button %d chose %q", i, g.interfaceSizeKey())
				}
				if got := readDisplayPreference(t); got != sizes[i].Key || g.displaySettingsDirty {
					t.Fatalf("button %d saved %q (dirty=%v)", i, got, g.displaySettingsDirty)
				}
			}
			want := []string{}
			for _, size := range sizes[1:] {
				want = append(want, size.Key)
			}
			want = append(want, sizes[len(sizes)-1].Key) // Right at the largest stays put
			for step, w := range want {
				key(ebiten.KeyRight)
				if g.interfaceSizeKey() != w {
					t.Fatalf("Right #%d chose %q, want %q", step+1, g.interfaceSizeKey(), w)
				}
			}
			key(ebiten.KeyLeft)
			if got := g.interfaceSizeKey(); got != sizes[len(sizes)-2].Key || readDisplayPreference(t) != got {
				t.Fatalf("Left chose %q", got)
			}
			// The neighbour tabs keep their own rows and controls.
			click(settingsTabRect(layout.px, layout.py, layout.panelW, settingsTabSound))
			master := audioSettingDefinitions[0].channel
			g.soundManager.SetVolume(master, 0.5)
			key(ebiten.KeyLeft)
			if g.settingsTab != settingsTabSound || math.Abs(g.soundManager.Volume(master)-0.45) > 1e-9 {
				t.Fatalf("Sound tab keyboard: tab=%d master=%.2f, want 0.45", g.settingsTab, g.soundManager.Volume(master))
			}
			if g.interfaceSizeKey() != sizes[len(sizes)-2].Key {
				t.Fatal("Sound tab keys changed the interface size")
			}
		})
	}
}

// On a screen where a preset cannot grow (4K: Largest repeats Large), the
// button is dimmed and ignored, the arrow keys skip it, and the status line
// says whether the active size is sharp.
func TestDisplaySettingsOfferedSizes(t *testing.T) {
	for _, tc := range []struct {
		display       string
		largestOffers bool
		largeStatus   string
	}{
		{"4K HiDPI", false, "Sharp: text pixels scale x3"},
		{"1920x1080", true, "Text slightly soft: this screen has no sharp step here"},
	} {
		t.Run(tc.display, func(t *testing.T) {
			h, _ := audioGestureHarness(t, false, 0)
			g := h.g
			for _, d := range interfaceTestDisplays {
				if d.name == tc.display {
					prev := displayDeviceScale
					t.Cleanup(func() { displayDeviceScale = prev })
					displayDeviceScale = func() float64 { return d.scale }
					g.config.PlayerInterfaceSize = "large"
					h.loop.Layout(d.outW, d.outH)
				}
			}
			if len(g.interfaceFrames) == 0 {
				t.Fatal("setup: display not found")
			}
			// The panel is measured against the harness screen; keep it there.
			g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = 1024, 768
			g.switchSettingsTab(settingsTabDisplay)
			if got := displaySizeStatus(g); len(got) < 2 || got[1] != tc.largeStatus {
				t.Fatalf("status %q, want %q", got, tc.largeStatus)
			}
			fp := installFakePointer(t)
			layout := makeAudioSettingsPanelLayout(1024, 768, false)
			presentInputScreen(h)
			b := displaySizeButtonRect(layout.px, layout.py, layout.panelW, 2, len(g.config.Display.InterfaceSizes))
			fp.moveTo(b.x+b.w/2, b.y+b.h/2)
			fp.press()
			updateInputScreen(h)
			fp.release()
			updateInputScreen(h)
			wantKey := "large"
			if tc.largestOffers {
				wantKey = "largest"
			}
			if g.interfaceSizeKey() != wantKey {
				t.Fatalf("clicking Largest chose %q, want %q", g.interfaceSizeKey(), wantKey)
			}
			g.config.PlayerInterfaceSize = "large"
			presentInputScreen(h)
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyRight })
			updateInputScreen(h)
			if g.interfaceSizeKey() != wantKey {
				t.Fatalf("Right from Large chose %q, want %q", g.interfaceSizeKey(), wantKey)
			}
		})
	}
}

// The preference file is the only source across restarts; anything unusable
// falls back to the configured default, and a failed save stays retryable.
func TestDisplayPreferencesFile(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"missing", "", "normal"},
		{"saved large", `{"interface_size":"large"}`, "large"},
		{"unknown preset", `{"interface_size":"huge"}`, "normal"},
		{"corrupt", `{"interface_size":`, "normal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storage.SetDataRootForTesting(t.TempDir())
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			if tc.body != "" {
				path := displayPreferencesPath()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := readDisplayPreference(t); got != tc.want {
				t.Fatalf("loaded %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("failed save remains retryable", func(t *testing.T) {
		h := newDisplayedModalHarness(t, 1024, 768)
		g := h.g
		g.config.PlayerInterfaceSize = "normal"
		path := displayPreferencesPath()
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		g.setInterfaceSize("large")
		if !g.displaySettingsDirty || g.settingsSaveError == "" || g.interfaceSizeKey() != "large" {
			t.Fatal("save failure silently accepted or dropped the choice")
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		g.saveAudioSettings() // leaving or switching a tab retries every pending save
		if g.displaySettingsDirty || g.settingsSaveError != "" || readDisplayPreference(t) != "large" {
			t.Fatal("save did not recover")
		}
	})
}

// Real startup reads the file; campaign load and a new adventure never touch it.
func TestDisplaySettingsRealStartupAndCampaignLoad(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	path := displayPreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"interface_size":"largest"}`), 0600); err != nil {
		t.Fatal(err)
	}
	g, _, cfg := bootOpenWorldGame(t, false)
	if g.interfaceSizeKey() != "largest" {
		t.Fatalf("startup chose %q", g.interfaceSizeKey())
	}
	campaign := filepath.Join(t.TempDir(), "campaign.json")
	if err := g.SaveGameToFile(campaign); err != nil {
		t.Fatal(err)
	}
	if err := g.LoadGameFromFile(campaign); err != nil {
		t.Fatal(err)
	}
	g.beginAdventure(newPartyCreateState(cfg))
	if g.interfaceSizeKey() != "largest" || cfg.PlayerInterfaceSize != "largest" {
		t.Fatal("campaign load or new adventure reset the interface size")
	}
}

func readDisplayFontPreference(t *testing.T) (font, size string) {
	t.Helper()
	g := &MMGame{config: loadTestConfig(t)}
	g.config.PlayerInterfaceSize, g.config.PlayerFont = "", ""
	g.loadDisplayPreferences()
	return g.config.PlayerFont, g.config.PlayerInterfaceSize
}

// The font is saved beside the interface size and restored at start; an
// unknown or absent font falls back to the default without costing the saved
// size.
func TestDisplayFontPreference(t *testing.T) {
	def := loadTestConfig(t).Display.DefaultFont
	for _, tc := range []struct {
		name, body, font, size string
	}{
		{"missing file", "", def, "normal"},
		{"saved font", `{"interface_size":"large","font":"alagard"}`, "alagard", "large"},
		{"unknown font", `{"interface_size":"large","font":"papyrus"}`, def, "large"},
		{"file from before fonts", `{"interface_size":"large"}`, def, "large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage.SetDataRootForTesting(t.TempDir())
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			if tc.body != "" {
				path := displayPreferencesPath()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if font, size := readDisplayFontPreference(t); font != tc.font || size != tc.size {
				t.Fatalf("loaded font %q size %q, want %q %q", font, size, tc.font, tc.size)
			}
		})
	}
}

// Settings > Display: the font list opens from its field, chooses by click
// and closes on a click anywhere else; Left/Right on the font row step
// through the fonts and wrap. Every choice draws at once and is saved, and
// the interface size stays as it was.
func TestDisplaySettingsFontList(t *testing.T) {
	fonts := shippedUIFonts(t)
	prev := uiActiveFont
	t.Cleanup(func() { setActiveUIFont(prev) })
	for _, entry := range []bool{false, true} {
		t.Run(fmt.Sprintf("entry=%v", entry), func(t *testing.T) {
			h, _ := audioGestureHarness(t, entry, 0)
			g := h.g
			g.config.PlayerFont = g.config.Display.DefaultFont
			g.applyUIFont()
			size := g.interfaceSizeKey()
			fp := installFakePointer(t)
			layout := makeAudioSettingsPanelLayout(1024, 768, entry)
			click := func(r layoutRect) {
				presentInputScreen(h)
				fp.moveTo(r.x+r.w/2, r.y+r.h/2)
				fp.press()
				updateInputScreen(h)
				fp.release()
				updateInputScreen(h)
			}
			key := func(k ebiten.Key) {
				presentInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(p ebiten.Key) bool { return p == k })
				updateInputScreen(h)
			}
			click(settingsTabRect(layout.px, layout.py, layout.panelW, settingsTabDisplay))
			field := displayFontFieldRect(layout.px, layout.py, layout.panelW)
			chosen := func(want string) {
				t.Helper()
				saved, _ := readDisplayFontPreference(t)
				if g.uiFontKey() != want || activeUIFont() != uiFontAtlases[want] || saved != want || g.fontListOpen {
					t.Fatalf("font %q (saved %q, list open %v), want %q drawing", g.uiFontKey(), saved, g.fontListOpen, want)
				}
			}
			for i := len(fonts) - 1; i >= 0; i-- {
				click(field)
				if !g.fontListOpen {
					t.Fatal("the field did not open the list")
				}
				click(displayFontItemRect(layout.px, layout.py, layout.panelW, i))
				chosen(fonts[i].Key)
			}
			click(field)
			item := displayFontItemRect(layout.px, layout.py, layout.panelW, 1)
			click(layoutRect{layout.px + 40, item.y, 10, item.h}) // beside the list
			chosen(fonts[0].Key)

			key(ebiten.KeyDown)
			if g.audioSettingsSelection != displayRowFont {
				t.Fatalf("Down selected row %d", g.audioSettingsSelection)
			}
			key(ebiten.KeyRight)
			chosen(fonts[1].Key)
			key(ebiten.KeyLeft)
			key(ebiten.KeyLeft)
			chosen(fonts[len(fonts)-1].Key)
			if g.interfaceSizeKey() != size {
				t.Fatalf("font controls changed the interface size to %q", g.interfaceSizeKey())
			}
		})
	}
}
