package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/sound"
)

func presentInputScreen(h *displayedModalHarness) {
	if h.g.appScreen == AppScreenInGame {
		h.ui.Draw(h.screen)
	} else {
		h.loop.Draw(h.screen)
	}
}

func updateInputScreen(h *displayedModalHarness) {
	h.t.Helper()
	if err := h.loop.Update(); err != nil {
		h.t.Fatal(err)
	}
}

func titleButtonPoint(g *MMGame, key string) (int, int) {
	layout := makeEntryMenuRootLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight())
	for i, b := range entryButtons() {
		if b.key == key {
			return layout.buttonX + layout.buttonW/2, layout.buttonStartY + i*(layout.buttonH+layout.buttonGap) + layout.buttonH/2
		}
	}
	panic("missing title button")
}

func TestMainMenuTransitionDoesNotReuseTheOpeningPress(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1280, 800}} {
		for _, drawBeforeRelease := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/draw=%v", size[0], size[1], drawBeforeRelease), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				fp := installFakePointer(t)
				g.mainMenuOpen = true
				presentInputScreen(h)
				w, height := menuPanelSize(MenuMain)
				for i, option := range mainMenuOptions {
					if option.key == "main_menu" {
						r, _, _ := menuRowRect((size[0]-w)/2, (size[1]-height)/2, w, mainMenuListTopY, mainMenuRowPitch, i)
						fp.moveTo((r.x1+r.x2)/2, (r.y1+r.y2)/2)
					}
				}
				fp.press()
				updateInputScreen(h)
				if g.appScreen != AppScreenMainMenu || g.entryMenuMode != EntryMenuRoot {
					t.Fatal("Main Menu press did not leave the game")
				}
				if drawBeforeRelease {
					presentInputScreen(h)
				}
				fp.hold()
				for i := 0; i < 3; i++ {
					updateInputScreen(h)
				}
				x, y := titleButtonPoint(g, "settings")
				fp.moveTo(x, y)
				fp.release()
				updateInputScreen(h)
				if g.entryMenuMode != EntryMenuRoot || g.entryMenuRootPressArmed {
					t.Fatal("release of the in-game Main Menu press opened title Settings")
				}
				// A fresh gesture on the displayed destination must still work.
				presentInputScreen(h)
				fp.press()
				updateInputScreen(h)
				fp.release()
				updateInputScreen(h)
				if g.entryMenuMode != EntryMenuSettings {
					t.Fatal("fresh title gesture did not open Settings")
				}
			})
		}
	}
}

func TestTitleBackDoesNotArmRootButtons(t *testing.T) {
	for _, mode := range []EntryMenuMode{EntryMenuLoad, EntryMenuScores, EntryMenuAchievements, EntryMenuSettings} {
		for _, draw := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%d/draw=%v", mode, draw), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, fp := h.g, installFakePointer(t)
				g.appScreen, g.entryMenuMode = AppScreenMainMenu, mode
				var x, y int
				switch mode {
				case EntryMenuLoad:
					x = (1024-entryLoadPanelW)/2 + menuFrameInset
					y = (768-entryLoadPanelH)/2 + menuFrameInset + 22 + saveRowsPerPage*entryLoadRowH + 6 + 26 + 12
				case EntryMenuScores:
					x, y = 20, 768-44
				case EntryMenuAchievements:
					x, y = (1024-640)/2+menuFrameInset, (768-480)/2+480-menuFrameInset-30
				case EntryMenuSettings:
					layout := makeAudioSettingsPanelLayout(1024, 768, true)
					back := audioBackRect(layout.px, layout.py, layout.panelH, layout.contentInset)
					x, y = back.x1, back.y1
				}
				presentInputScreen(h)
				fp.moveTo(x+3, y+3)
				fp.press()
				updateInputScreen(h)
				if g.entryMenuMode != EntryMenuRoot {
					t.Fatal("Back did not show the title root")
				}
				if draw {
					presentInputScreen(h)
				}
				fp.moveTo(titleButtonPoint(g, "settings"))
				fp.release()
				updateInputScreen(h)
				if g.entryMenuMode != EntryMenuRoot || g.entryMenuRootPressArmed {
					t.Fatal("Back release activated a newly exposed title button")
				}
			})
		}
	}
}

func TestTitleReleaseRequiresTheDisplayedScreen(t *testing.T) {
	for _, change := range []string{"none", "not drawn", "resize", "subscreen"} {
		for _, key := range []string{"start", "load", "scores", "achievements", "settings", "quit"} {
			t.Run(change+"/"+key, func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, fp := h.g, installFakePointer(t)
				g.appScreen = AppScreenMainMenu
				if change != "not drawn" {
					presentInputScreen(h)
				}
				// Release-position behavior intentionally permits a stale press
				// coordinate while focus/fullscreen settles on the same screen.
				fp.moveTo(0, 0)
				fp.press()
				updateInputScreen(h)
				switch change {
				case "resize":
					g.config.Display.ScreenWidth += 80
					presentInputScreen(h)
				case "subscreen":
					g.entryMenuMode = EntryMenuLoad
					presentInputScreen(h)
					fp.hold()
					updateInputScreen(h)
					g.entryMenuMode = EntryMenuRoot
					presentInputScreen(h)
				}
				fp.moveTo(titleButtonPoint(g, key))
				fp.release()
				updateInputScreen(h)
				if change != "none" {
					if g.appScreen != AppScreenMainMenu || g.entryMenuMode != EntryMenuRoot || g.exitRequested {
						t.Fatal("old or undisplayed press activated a title action")
					}
					return
				}
				want := map[string]EntryMenuMode{"load": EntryMenuLoad, "scores": EntryMenuScores, "achievements": EntryMenuAchievements, "settings": EntryMenuSettings}[key]
				if key == "start" {
					if g.appScreen != AppScreenPartyCreate || g.partyCreate == nil {
						t.Fatal("Start release did not create party screen")
					}
				} else if key == "quit" {
					if !g.exitRequested {
						t.Fatal("Quit release was lost")
					}
				} else if g.entryMenuMode != want {
					t.Fatal("fresh release did not activate the intended title action")
				}
			})
		}
	}
}

func TestPartyCreationGestureKeepsItsScreen(t *testing.T) {
	for _, change := range []string{"none", "resize pending", "resize drag", "screen change"} {
		t.Run(change, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g, fp := h.g, installFakePointer(t)
			g.enterPartyCreate()
			pc := g.partyCreate
			hero, other := pc.slots[0], pc.slots[1]
			layout := partyCreateLayout(pc, 1024, 768)
			presentInputScreen(h)
			fp.moveTo(layout.slots[0].x+3, layout.slots[0].y+3)
			fp.press()
			updateInputScreen(h)
			if pc.pending != hero {
				t.Fatal("creation press did not select and arm its hero")
			}
			fp.hold()
			if change != "resize pending" {
				fp.moveTo(layout.slots[1].x+3, layout.slots[1].y+3)
				updateInputScreen(h)
				if pc.drag != hero {
					t.Fatal("creation movement did not start its drag")
				}
			}
			if change == "screen change" {
				g.returnToMainMenu()
			} else if change != "none" {
				g.config.Display.ScreenWidth += 80
			}
			presentInputScreen(h)
			layout = partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
			fp.moveTo(layout.slots[1].x+3, layout.slots[1].y+3)
			fp.release()
			updateInputScreen(h)
			if pc.drag != nil || pc.pending != nil {
				t.Fatal("creation gesture survived release or loss of screen")
			}
			if change == "none" {
				if pc.slots[0] != other || pc.slots[1] != hero {
					t.Fatal("fresh creation drag did not swap heroes")
				}
			} else if pc.slots[0] != hero || pc.slots[1] != other {
				t.Fatal("stale creation drag moved a hero")
			}
		})
	}
}

func TestAudioGestureKeepsItsScreen(t *testing.T) {
	for _, entry := range []bool{false, true} {
		for _, resize := range []bool{false, true} {
			t.Run(fmt.Sprintf("entry=%v/resize=%v", entry, resize), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g, fp := h.g, installFakePointer(t)
				g.soundManager = &sound.Manager{}
				if entry {
					g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
				} else {
					g.mainMenuOpen, g.mainMenuMode = true, MenuSettings
				}
				g.beginAudioSettings()
				layout := makeAudioSettingsPanelLayout(1024, 768, entry)
				r := audioSliderRect(layout.px, layout.py, layout.panelW, 0)
				presentInputScreen(h)
				fp.moveTo(r.x1+(r.x2-r.x1)/4, (r.y1+r.y2)/2)
				fp.press()
				updateInputScreen(h)
				before := g.soundManager.Volume(sound.VolumeMaster)
				if g.audioSliderDrag != 0 || before <= 0 || !g.audioSettingsDirty {
					t.Fatal("fresh audio press did not start volume adjustment")
				}
				if resize {
					g.config.Display.ScreenWidth += 80
				}
				presentInputScreen(h)
				layout = makeAudioSettingsPanelLayout(g.config.GetScreenWidth(), 768, entry)
				r = audioSliderRect(layout.px, layout.py, layout.panelW, 0)
				fp.moveTo(r.x1+3*(r.x2-r.x1)/4, (r.y1+r.y2)/2)
				fp.hold()
				updateInputScreen(h)
				after := g.soundManager.Volume(sound.VolumeMaster)
				if resize && after != before || !resize && after <= before {
					t.Fatal("audio hold ignored its screen ownership")
				}
				fp.release()
				updateInputScreen(h)
				if g.audioSliderDrag != -1 || g.audioSettingsDirty {
					t.Fatal("audio release/cancellation did not commit and stop the adjustment")
				}
			})
		}
	}
}

func TestTopLevelActionsWaitForDestinationPresentation(t *testing.T) {
	for _, action := range []string{"load", "creation back", "creation begin"} {
		t.Run(action, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g, fp := h.g, installFakePointer(t)
			var x, y int
			want := AppScreenInGame
			if action == "load" {
				g.party.Gold = 321
				if err := g.SaveGameToFile(saveRowPath(1)); err != nil {
					t.Fatal(err)
				}
				g.party.Gold = 999
				g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuLoad
				x = (1024-entryLoadPanelW)/2 + menuFrameInset + 3
				y = (768-entryLoadPanelH)/2 + menuFrameInset + 22 + entryLoadRowH + 3
			} else {
				g.enterPartyCreate()
				layout := partyCreateLayout(g.partyCreate, 1024, 768)
				r := layout.begin
				if action == "creation back" {
					r, want = layout.back, AppScreenMainMenu
				}
				x, y = r.x+3, r.y+3
				// Restart the real fixture world without replacing it from disk.
				setTestWorldManager(t, nil)
			}
			presentInputScreen(h)
			fp.moveTo(x, y)
			fp.press()
			updateInputScreen(h)
			if g.appScreen != want {
				t.Fatal("displayed top-level action failed to navigate")
			}
			if action == "load" && g.party.Gold != 321 {
				t.Fatal("Load did not restore the selected save")
			}
			if !h.ui.modalRedrawBarrierActive() {
				t.Fatal("undisplayed destination has no input barrier")
			}
			before := g.frameCount
			fp.hold()
			for i := 0; i < 3; i++ {
				updateInputScreen(h)
			}
			if g.frameCount != before {
				t.Fatal("world advanced before the destination was shown")
			}
			// Another press while the old screen is still visible is stale too.
			fp.moveTo(titleButtonPoint(g, "settings"))
			fp.press()
			updateInputScreen(h)
			presentInputScreen(h)
			fp.release()
			updateInputScreen(h)
			if g.entryMenuMode != EntryMenuRoot || g.entryMenuRootPressArmed || g.dragArmed || g.stashDragArmed {
				t.Fatal("screen transition forwarded a raw edge to its destination")
			}
			if len(g.mouseLeftClicks)+len(g.mouseRightClicks) != 0 {
				t.Fatal("stale queued clicks survived transition")
			}
		})
	}
}

func TestTitleLoadKeyboardPagesOnlyInUpdate(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	h.g.appScreen, h.g.entryMenuMode = AppScreenMainMenu, EntryMenuLoad
	keys := map[ebiten.Key]bool{ebiten.KeyRight: true}
	h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return keys[k] })
	for i := 0; i < 3; i++ {
		presentInputScreen(h)
	}
	if h.g.savePage != 0 {
		t.Fatal("Draw consumed a page-navigation key")
	}
	updateInputScreen(h)
	if h.g.savePage != 1 {
		t.Fatal("Update did not consume the page-navigation key")
	}
	for i := 0; i < 3; i++ {
		presentInputScreen(h)
		updateInputScreen(h)
	}
	if h.g.savePage != 1 {
		t.Fatal("held page-navigation key fired more than once")
	}
	keys[ebiten.KeyRight] = false
	updateInputScreen(h)
	keys[ebiten.KeyLeft] = true
	updateInputScreen(h)
	if h.g.savePage != 0 {
		t.Fatal("fresh previous-page key was lost")
	}
}
