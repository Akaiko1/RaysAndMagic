package game

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/sound"

	"github.com/hajimehoshi/ebiten/v2"
)

// scrollbarCase is one list with a scrollbar, posed on its real screen.
type scrollbarCase struct {
	name         string
	setup        func(h *displayedModalHarness)
	track        func(h *displayedModalHarness) (track layoutRect, offset, contentH int)
	contentRight func(h *displayedModalHarness) int        // right edge of the list's clickable content
	visible      func(h *displayedModalHarness) layoutRect // where the track shows, when a viewport cuts it
	unchanged    func(h *displayedModalHarness) any        // model state a scrollbar press must not touch
}

func scrollbarCases() []scrollbarCase {
	alchemy := func(h *displayedModalHarness) (alchemyMaterialLayout, *config.AlchemyRecipe) {
		r := &config.GlobalAlchemy.Recipes[0]
		content := computeTabbedMenuLayout(h.g.config.GetScreenWidth(), gameplayViewportBottom(h.g)).content
		return makeAlchemyMaterialLayout(computeRareBookLayout(content, true), r), r
	}
	potion := func(h *displayedModalHarness, row int) potionSettingsLayout {
		p := makeAudioSettingsPanelLayout(h.g.config.GetScreenWidth(), h.g.config.GetScreenHeight(), true)
		return makePotionSettingsLayout(p.px, p.py, p.panelW, row)
	}
	stats := func(h *displayedModalHarness) profileStatsLayout {
		return makeProfileStatsLayout(h.g.config.GetScreenWidth(), h.g.config.GetScreenHeight(), profilePages[h.g.statisticsTab])
	}
	achievements := func(h *displayedModalHarness) profileAchievementsLayout {
		return makeProfileAchievementsLayout(h.g.config.GetScreenWidth(), h.g.config.GetScreenHeight(), len(config.GetAchievements()))
	}
	profile := func(mode EntryMenuMode) func(h *displayedModalHarness) {
		return func(h *displayedModalHarness) {
			g := h.g
			attachTestProfile(h.t, g)
			for i := 0; i < 14; i++ {
				g.playerProfile.Data.Rank("classes", fmt.Sprint(i), "Entry", "", int64(i+1))
			}
			g.appScreen, g.entryMenuMode = AppScreenMainMenu, mode
		}
	}
	cases := []scrollbarCase{{
		name: "alchemy materials",
		setup: func(h *displayedModalHarness) {
			h.g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, h.g.config)
			h.g.menuOpen, h.g.currentTab, h.g.selectedChar = true, TabSpellbook, 0
		},
		track: func(h *displayedModalHarness) (layoutRect, int, int) {
			a, r := alchemy(h)
			return a.track, h.ui.alchemyScroll[r.Key], a.height
		},
		contentRight: func(h *displayedModalHarness) int {
			a, _ := alchemy(h)
			right := 0
			for _, c := range a.cells {
				right = max(right, c.rect.right())
			}
			return right
		},
		unchanged: func(h *displayedModalHarness) any {
			_, r := alchemy(h)
			return h.g.alchemySelection(r).Clone()
		},
	}}
	for row, name := range []string{"health", "mana"} {
		cases = append(cases, scrollbarCase{
			name: "auto-potion " + name + " types",
			setup: func(h *displayedModalHarness) {
				// The shipped potions fit one screen of cards; pose enough
				// copies that the list scrolls.
				poseExtraAutomaticPotions(h.t, row == 1, 8)
				h.g.soundManager = &sound.Manager{}
				h.g.beginAudioSettings()
				h.g.appScreen, h.g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
				h.g.switchSettingsTab(settingsTabPotions)
			},
			track: func(h *displayedModalHarness) (layoutRect, int, int) {
				l := potion(h, row)
				return l.track, h.g.potionSettingsScroll[row], l.typesHeight(len(config.AutomaticPotionChoices(row == 1)))
			},
			contentRight: func(h *displayedModalHarness) int { l := potion(h, row); return l.card(1, 0).right() },
			unchanged:    func(h *displayedModalHarness) any { return fmt.Sprintf("%+v", *h.g.config.EnsurePotionPreferences()) },
		})
	}
	cases = append(cases,
		scrollbarCase{
			name:  "profile achievements",
			setup: profile(EntryMenuAchievements),
			track: func(h *displayedModalHarness) (layoutRect, int, int) {
				l := achievements(h)
				return l.track, h.g.achievementsScroll, l.contentH
			},
			contentRight: func(h *displayedModalHarness) int {
				l := achievements(h)
				return l.card(l.columns-1, 0).right()
			},
			unchanged: func(h *displayedModalHarness) any { return h.g.entryMenuMode },
		},
		scrollbarCase{
			name:  "profile statistics",
			setup: profile(EntryMenuStatistics),
			track: func(h *displayedModalHarness) (layoutRect, int, int) {
				l := stats(h)
				return l.track, h.g.statisticsScroll, l.contentH
			},
			contentRight: func(h *displayedModalHarness) int {
				l := stats(h)
				return max(l.rankingRect(l.columns-1, 0).right(), l.body.x+l.counterRect(l.counterColumns-1).right())
			},
			unchanged: func(h *displayedModalHarness) any { return h.g.statisticsTab },
		},
		scrollbarCase{
			name:  "profile ranking",
			setup: profile(EntryMenuStatistics),
			track: func(h *displayedModalHarness) (layoutRect, int, int) {
				l := stats(h)
				body := profileRankingBody(l.rankingRect(0, h.g.statisticsScroll))
				return profileRankingTrack(body), h.g.statisticsRankingScroll["classes"], 14 * profileRankingRowH
			},
			contentRight: func(h *displayedModalHarness) int {
				return profileRankingBody(stats(h).rankingRect(0, h.g.statisticsScroll)).right() - 8
			},
			visible:   func(h *displayedModalHarness) layoutRect { return stats(h).body },
			unchanged: func(h *displayedModalHarness) any { return h.g.statisticsScroll },
		},
	)
	return cases
}

// Every list scrollbar drags: press the thumb and move, or press the track to
// bring the thumb under the pointer. The drag follows the pointer past the
// track's ends and clamps, stops on release, and the press reaches nothing
// else. The grab strip never covers the list's own content.
func TestEveryListScrollbarDrags(t *testing.T) {
	for _, tc := range scrollbarCases() {
		scrolled := false
		for _, size := range [][2]int{{800, 600}, {1024, 768}, {1920, 1080}} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, size), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				tc.setup(h)
				fp := installFakePointer(t)
				fp.moveTo(0, 0)
				step := func() {
					presentInputScreen(h)
					updateInputScreen(h)
				}
				step()
				track, _, contentH := tc.track(h)
				if contentH <= track.h {
					return // the whole list is on screen; no scrollbar is drawn
				}
				scrolled = true
				if strip := scrollbarGrabStrip(track); tc.contentRight(h) > strip.x {
					t.Fatalf("list content reaches %d, the grab strip starts at %d", tc.contentRight(h), strip.x)
				}
				before := tc.unchanged(h)
				offset := func() int { _, o, _ := tc.track(h); return o }
				limit := contentH - track.h

				// Grab the thumb and drag past both ends.
				thumbY, thumbH, _ := scrollbarThumb(track, offset(), contentH)
				fp.moveTo(track.x+1, thumbY+thumbH/2)
				fp.press()
				step()
				fp.hold()
				fp.moveTo(track.x+30, track.bottom()+80)
				step()
				if offset() != limit {
					t.Fatalf("dragged past the bottom to %d, want %d", offset(), limit)
				}
				fp.moveTo(track.x-30, track.y-80)
				step()
				if offset() != 0 {
					t.Fatalf("dragged past the top to %d", offset())
				}
				fp.moveTo(track.x+1, track.y+track.h/2)
				step()
				mid := offset()
				if mid <= 0 || mid >= limit {
					t.Fatalf("thumb at mid-track gave offset %d of %d", mid, limit)
				}
				fp.release()
				step()
				fp.idle()
				fp.moveTo(track.x+1, track.bottom()+80)
				step()
				if offset() != mid {
					t.Fatalf("released thumb still followed the pointer: %d, want %d", offset(), mid)
				}
				// A press on the track below the thumb brings the thumb under it.
				foot := track.bottom() - 1
				if tc.visible != nil {
					foot = min(foot, tc.visible(h).bottom()-1)
				}
				fp.moveTo(track.x+track.w-1, foot)
				fp.press()
				step()
				fp.release()
				step()
				fp.idle()
				if y, th, _ := scrollbarThumb(track, offset(), contentH); foot < y || foot >= y+th {
					t.Fatalf("press at %d left the thumb at %d..%d (offset %d of %d)", foot, y, y+th, offset(), limit)
				}
				jumped := offset()
				if after := tc.unchanged(h); !reflect.DeepEqual(before, after) {
					t.Fatalf("scrollbar presses changed %v to %v", before, after)
				}
				// A press beside the strip belongs to the list, not the thumb.
				fp.moveTo(scrollbarGrabStrip(track).x-2, track.y+track.h/2)
				fp.press()
				step()
				fp.hold()
				fp.moveTo(scrollbarGrabStrip(track).x-2, track.y)
				step()
				fp.release()
				step()
				if offset() != jumped {
					t.Fatalf("a press beside the scrollbar dragged the list to %d", offset())
				}
			})
		}
		if !scrolled {
			t.Errorf("%s: fixture never overflows its track, so its scrollbar is untested", tc.name)
		}
	}
}

// poseExtraAutomaticPotions adds n copies of a shipped automatic potion for
// the rest of the test.
func poseExtraAutomaticPotions(t *testing.T, mana bool, n int) {
	t.Helper()
	choices := config.AutomaticPotionChoices(mana)
	if len(choices) == 0 {
		t.Fatal("fixture: no shipped automatic potion to copy")
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("scrollbar_fixture_%v_%d", mana, i)
		def := *choices[0].Definition
		def.Name = fmt.Sprintf("%s %d", def.Name, i)
		config.GlobalItems.Items[key] = &def
		t.Cleanup(func() { delete(config.GlobalItems.Items, key) })
	}
}

// A held thumb belongs to its list on the presented frame. Once the list is
// gone - Escape closed the book, another tab took the hub, a modal covers it -
// the grab ends, and reopening the list with the button still down scrolls
// nothing until a new press. Two Updates between Draws (the drag's own scroll
// staling the display) keep it.
func TestScrollbarGrabEndsWithItsList(t *testing.T) {
	alchemy := scrollbarCases()[0]
	for _, tc := range []struct {
		name  string
		leave func(h *displayedModalHarness)
		back  func(h *displayedModalHarness)
		ended bool
	}{
		{"escape closes the book", func(h *displayedModalHarness) {
			key := true
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return key && k == ebiten.KeyEscape })
			updateInputScreen(h)
			key = false
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
			if h.g.menuOpen {
				h.t.Fatal("fixture: Escape left the book open")
			}
		}, func(h *displayedModalHarness) { h.g.menuOpen, h.g.currentTab = true, TabSpellbook }, true},
		{"another tab", func(h *displayedModalHarness) { h.g.currentTab = TabInventory },
			func(h *displayedModalHarness) { h.g.currentTab = TabSpellbook }, true},
		{"a modal on top", func(h *displayedModalHarness) { h.g.statPopupOpen, h.g.statPopupCharIdx = true, 0 },
			func(h *displayedModalHarness) { h.g.statPopupOpen = false }, true},
		{"updates between draws", func(h *displayedModalHarness) { updateInputScreen(h) },
			func(*displayedModalHarness) {}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			alchemy.setup(h)
			fp := installFakePointer(t)
			fp.moveTo(0, 0)
			step := func() {
				presentInputScreen(h)
				updateInputScreen(h)
			}
			step()
			track, _, contentH := alchemy.track(h)
			offset := func() int { _, o, _ := alchemy.track(h); return o }
			thumbY, thumbH, _ := scrollbarThumb(track, offset(), contentH)
			fp.moveTo(track.x+1, thumbY+thumbH/2)
			fp.press()
			step()
			fp.hold()
			fp.moveTo(track.x+1, track.y+track.h/3)
			step()
			held := offset()
			if held == 0 {
				t.Fatal("fixture: the drag did not scroll")
			}
			presentInputScreen(h)
			tc.leave(h)
			presentInputScreen(h)
			updateInputScreen(h)
			tc.back(h)
			fp.moveTo(track.x+1, track.bottom()+80)
			step()
			if moved := offset() != held; moved == tc.ended {
				t.Fatalf("offset %d after returning (held at %d); grab should have ended: %v", offset(), held, tc.ended)
			}
			if tc.ended && h.ui.scrollDrag.key != "" {
				t.Fatalf("grab %q survived its list", h.ui.scrollDrag.key)
			}
		})
	}
}
