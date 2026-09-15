package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/sound"
)

func audioGestureHarness(t *testing.T, entry bool, row int) (*displayedModalHarness, pagerRect) {
	t.Helper()
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.soundManager = &sound.Manager{}
	g.beginAudioSettings()
	if entry {
		g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
	} else {
		g.mainMenuOpen, g.mainMenuMode = true, MenuSettings
	}
	layout := makeAudioSettingsPanelLayout(1024, 768, entry)
	return h, audioSliderRect(layout.px, layout.py, layout.panelW, row)
}

func TestAudioDragKeepsPressedChannel(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for row := range audioSettingDefinitions {
			for selected := range audioSettingDefinitions {
				for _, redraw := range []bool{false, true} {
					t.Run(fmt.Sprintf("entry=%v/row=%d/selected=%d/redraw=%v", entry, row, selected, redraw), func(t *testing.T) {
						h, r := audioGestureHarness(t, entry, row)
						g, fp := h.g, installFakePointer(t)
						g.audioSettingsSelection = selected
						presentInputScreen(h)
						fp.moveTo(r.x1+(r.x2-r.x1)/4, (r.y1+r.y2)/2)
						fp.press()
						updateInputScreen(h)
						if g.audioSliderDrag != row || g.audioSettingsSelection != row {
							t.Fatal("press failed to select and retain the channel")
						}
						before := g.soundManager.Volume(audioSettingDefinitions[row].channel)
						if redraw {
							presentInputScreen(h)
						}
						fp.hold()
						fp.moveTo(r.x1+3*(r.x2-r.x1)/4, (r.y1+r.y2)/2)
						for range 3 {
							updateInputScreen(h)
						}
						if g.soundManager.Volume(audioSettingDefinitions[row].channel) <= before {
							t.Fatal("hold stopped following the pressed slider")
						}
						for other, def := range audioSettingDefinitions {
							if other != row && g.soundManager.Volume(def.channel) != 0 {
								t.Fatal("drag adjusted another channel")
							}
						}
						fp.moveTo(0, 0)
						fp.release()
						updateInputScreen(h)
						if g.audioSliderDrag != -1 || g.audioSettingsDirty {
							t.Fatal("release outside did not commit and finish the drag")
						}
					})
				}
			}
		}
	}
}

func TestAudioDragCancelsOnOwnerLoss(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for row := range audioSettingDefinitions {
			for _, loss := range []string{"escape", "back", "resize", "screen", "modal", "load"} {
				if entry && loss == "modal" {
					continue
				} // Title settings have no in-game modal stack.
				t.Run(fmt.Sprintf("entry=%v/row=%d/loss=%s", entry, row, loss), func(t *testing.T) {
					h, r := audioGestureHarness(t, entry, row)
					g, fp := h.g, installFakePointer(t)
					if loss == "load" {
						if err := g.SaveGameToFile(saveRowPath(1)); err != nil {
							t.Fatal(err)
						}
					}
					presentInputScreen(h)
					fp.moveTo(r.x1+(r.x2-r.x1)/4, (r.y1+r.y2)/2)
					fp.press()
					updateInputScreen(h)
					before := g.soundManager.Volume(audioSettingDefinitions[row].channel)
					fp.hold()
					fp.moveTo(r.x2, (r.y1+r.y2)/2)
					switch loss {
					case "escape":
						previous := pointerCancelJustPress
						pointerCancelJustPress = func() bool { return true }
						t.Cleanup(func() { pointerCancelJustPress = previous })
						h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyEscape })
						updateInputScreen(h)
						pointerCancelJustPress = previous
						h.loop.inputHandler.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
					case "back":
						presentInputScreen(h)
						layout := makeAudioSettingsPanelLayout(1024, 768, entry)
						b := audioBackRect(layout.px, layout.py, layout.panelH, layout.contentInset)
						fp.release()
						updateInputScreen(h)
						fp.moveTo(b.x1+3, b.y1+3)
						fp.press()
						updateInputScreen(h)
					case "resize":
						g.config.Display.ScreenWidth += 80
						presentInputScreen(h)
						updateInputScreen(h)
					case "screen":
						if entry {
							g.entryMenuMode = EntryMenuRoot
						} else {
							g.mainMenuMode = MenuMain
						}
						presentInputScreen(h)
						updateInputScreen(h)
					case "modal":
						g.mapOverlayOpen = true
						presentInputScreen(h)
						updateInputScreen(h)
						g.mapOverlayOpen = false
					case "load":
						if err := g.LoadGameFromFile(saveRowPath(1)); err != nil {
							t.Fatal(err)
						}
						presentInputScreen(h)
						updateInputScreen(h)
					}
					if g.audioSliderDrag != -1 || g.audioSettingsDirty {
						t.Fatal("owner loss did not retire and commit the drag")
					}
					if g.soundManager.Volume(audioSettingDefinitions[row].channel) != before {
						t.Fatal("owner loss allowed an extra volume adjustment")
					}
					// Returning to the same settings with the button held cannot
					// resurrect a gesture, even when Draw precedes the next Update.
					if entry {
						g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
					} else {
						g.appScreen, g.mainMenuOpen, g.mainMenuMode = AppScreenInGame, true, MenuSettings
					}
					presentInputScreen(h)
					fp.hold()
					updateInputScreen(h)
					if g.audioSliderDrag != -1 || g.soundManager.Volume(audioSettingDefinitions[row].channel) != before {
						t.Fatal("returning to settings revived the cancelled drag")
					}
				})
			}
		}
	}
}

func TestAudioKeyboardWaitsForPresentedSelection(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for _, direction := range []ebiten.Key{ebiten.KeyDown, ebiten.KeyUp} {
			t.Run(fmt.Sprintf("entry=%v/key=%d", entry, direction), func(t *testing.T) {
				h, _ := audioGestureHarness(t, entry, 0)
				g := h.g
				if direction == ebiten.KeyUp {
					g.audioSettingsSelection = 2
				}
				keys := map[ebiten.Key]bool{direction: true, ebiten.KeyRight: true}
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return keys[k] })
				presentInputScreen(h)
				updateInputScreen(h)
				channel := audioSettingDefinitions[g.audioSettingsSelection].channel
				if g.audioSettingsSelection != 1 || g.soundManager.Volume(channel) != 0 {
					t.Fatal("navigation adjusted an unpresented channel in the same Update")
				}
				delete(keys, direction)
				updateInputScreen(h)
				if g.soundManager.Volume(channel) != 0 {
					t.Fatal("catch-up Update adjusted the unpresented channel")
				}
				presentInputScreen(h)
				updateInputScreen(h)
				if g.soundManager.Volume(channel) <= 0 {
					t.Fatal("presented channel did not receive keyboard adjustment")
				}
			})
		}
	}
}
