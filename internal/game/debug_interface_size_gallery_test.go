//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"

	"github.com/hajimehoshi/ebiten/v2"
)

// Every interface size preset on real displays, drawn through Layout and the
// presented frame: the interface at the screen's native resolution, the world
// at Normal's. Output: RAM_INTERFACE_QA_DIR (default: the temp dir).
func TestDebugSim_InterfaceSizeGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	g.camera.Angle = 45 * math.Pi / 180
	g.turnBasedMode = false
	g.showPartyStats = true
	for _, m := range []string{"Gareth hits the Goblin for 14 damage.", "The Goblin misses Lysander.", "Celestine casts Heal on Gareth.", "Silvelyn shoots the Goblin Archer for 9 damage.", "The Goblin Archer flees into the trees."} {
		g.AddCombatMessage(m)
	}
	fp := installFakePointer(t)
	fp.moveTo(0, 0)
	hero := g.party.Members[0]
	defer func() { g.party.Members[0] = hero }()

	out := os.Getenv("RAM_INTERFACE_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-interface-sizes")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func(prev func() float64) { displayDeviceScale = prev }(displayDeviceScale)
	for _, d := range []struct {
		name  string
		w, h  int
		scale float64
	}{
		{"4k_hidpi", 1920, 1080, 2},
		{"1440p", 2560, 1440, 1},
		{"1080p", 1920, 1080, 1},
		{"macbook14", 1512, 982, 2},
	} {
		displayDeviceScale = func() float64 { return d.scale }
		for _, size := range g.config.Display.InterfaceSizes {
			g.config.PlayerInterfaceSize = size.Key
			for _, view := range []struct {
				name  string
				tab   MenuTab
				open  bool
				class character.CharacterClass
			}{
				{"hud", 0, false, -1}, {"inventory", TabInventory, true, -1}, {"characters", TabCharacters, true, -1},
				{"spellbook", TabSpellbook, true, character.ClassSorcerer}, {"trap_book", TabSpellbook, true, character.ClassThief},
				{"alchemist", TabSpellbook, true, character.ClassAlchemist}, {"pilgrim", TabSpellbook, true, character.ClassWayfarer},
			} {
				g.menuOpen, g.currentTab = view.open, view.tab
				g.party.Members[0] = hero
				if view.class >= 0 {
					g.party.Members[0] = character.CreateCharacter("Lysander", view.class, g.config)
				}
				g.selectedChar, g.selectedSchool, g.selectedSpell = 0, 0, 0
				w, h := g.gameLoop.Layout(d.w, d.h)
				screen := ebiten.NewImage(w, h)
				var problem error
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen.Clear()
					drawLaidOutFrame(g, screen)
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("%s_%s_%s.png", d.name, size.Key, view.name)))
					if err != nil {
						problem = err
						return
					}
					defer f.Close()
					problem = png.Encode(f, snapshotUIImage(screen))
				})
				screen.Deallocate()
				if problem != nil {
					t.Fatal(problem)
				}
			}
		}
	}
	t.Logf("interface size gallery -> %s", out)
}

// Every shipped font on the screens that carry the most text, drawn through
// Layout and the presented frame at 1080p Normal and 4K HiDPI Large. Output:
// RAM_FONT_QA_DIR (default: the temp dir).
func TestDebugSim_FontGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	if err := loadUIFonts(g.config.Display.Fonts); err != nil {
		t.Fatal(err)
	}
	defer func(prev *uiGlyphAtlas) { setActiveUIFont(prev) }(uiActiveFont)
	g.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	g.camera.Angle = 45 * math.Pi / 180
	g.showPartyStats = true
	g.AddCombatMessage("The forest path bends toward Silverbough.")
	fp := installFakePointer(t)
	fp.moveTo(0, 0)
	hero := g.party.Members[0]
	defer func() { g.party.Members[0] = hero }()

	out := os.Getenv("RAM_FONT_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-fonts")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func(prev func() float64) { displayDeviceScale = prev }(displayDeviceScale)
	for _, f := range g.config.Display.Fonts {
		g.config.PlayerFont = f.Key
		g.applyUIFont()
		for _, d := range []struct {
			name  string
			w, h  int
			scale float64
			size  string
		}{{"1080p_normal", 1920, 1080, 1, "normal"}, {"4k_large", 1920, 1080, 2, "large"}} {
			displayDeviceScale = func() float64 { return d.scale }
			g.config.PlayerInterfaceSize = d.size
			for _, view := range []string{"hud", "inventory", "spellbook", "settings"} {
				g.appScreen, g.menuOpen, g.fontListOpen = AppScreenInGame, false, false
				g.party.Members[0] = hero
				switch view {
				case "inventory":
					g.menuOpen, g.currentTab = true, TabInventory
				case "spellbook":
					g.menuOpen, g.currentTab = true, TabSpellbook
					g.party.Members[0] = character.CreateCharacter("Lysander", character.ClassSorcerer, g.config)
					g.selectedChar, g.selectedSchool, g.selectedSpell = 0, 0, 0
				case "settings":
					g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
					g.beginAudioSettings()
					g.settingsTab, g.fontListOpen = settingsTabDisplay, true
				}
				w, h := g.gameLoop.Layout(d.w, d.h)
				screen := ebiten.NewImage(w, h)
				var problem error
				runOnDrawFrame(func(_ *ebiten.Image) {
					screen.Clear()
					if g.appScreen == AppScreenInGame {
						drawLaidOutFrame(g, screen)
					} else {
						g.gameLoop.Draw(screen)
					}
					file, err := os.Create(filepath.Join(out, fmt.Sprintf("%s_%s_%s.png", f.Key, d.name, view)))
					if err != nil {
						problem = err
						return
					}
					defer file.Close()
					problem = png.Encode(file, snapshotUIImage(screen))
				})
				screen.Deallocate()
				if problem != nil {
					t.Fatal(problem)
				}
			}
		}
	}
	t.Logf("font gallery -> %s", out)
}
