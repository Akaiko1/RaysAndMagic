package game

import (
	"fmt"
	"testing"
	"time"
)

func partyPointerPosition(g *MMGame, member int) (int, int) {
	w, h, left, top := partyPortraitLayout(g)
	return left + member*w + w/2, top + h/2
}

func TestDisplayedPartySelectionAcrossHubTabs(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tab := MenuTab(-1); tab <= TabCards; tab++ {
			t.Run(fmt.Sprintf("tb=%v/tab=%d", tb, tab), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				g.turnBasedMode = tb
				g.showPartyStats = true
				g.menuOpen = tab >= 0
				if g.menuOpen {
					g.currentTab = tab
				}
				g.world.Monsters = nil
				// Keep the gameplay fixture inside a party turn; an automatic
				// new round deliberately resets manual selection independently.
				for _, member := range g.party.Members {
					member.ActionsRemaining = 3
				}
				fp := installFakePointer(t)
				for _, member := range []int{1, 2, 3, 0} {
					x, y := partyPointerPosition(g, member)
					fp.moveTo(x, y)
					fp.press()
					h.pointerStep()
					if g.selectedChar != member || !g.parkSelection {
						t.Fatalf("physical portrait click selected %d, want %d", g.selectedChar, member)
					}
					fp.hold()
					for i := 0; i < 3; i++ {
						if err := h.loop.Update(); err != nil {
							t.Fatal(err)
						}
					}
					fp.release()
					h.pointerStep()
					fp.idle()
					h.pointerStep()
					if g.selectedChar != member || g.focusModeActive() {
						t.Fatal("hold/release changed selection or focus")
					}
					if len(g.mouseLeftClicks) != 0 {
						t.Fatal("portrait click survived its displayed owner")
					}
				}
			})
		}
	}
}

func TestDisplayedPartySelectionRejectsOldAndObscuredOwners(t *testing.T) {
	for _, owner := range []string{"hidden", "modal", "stale_selection", "stale_roster", "split_fragment", "stash_fragment", "same_batch"} {
		t.Run(owner, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab, g.showPartyStats = true, TabInventory, true
			if owner == "hidden" {
				g.showPartyStats = false
			}
			if owner == "modal" {
				g.mainMenuOpen = true
			}
			if owner == "split_fragment" {
				g.dragPickedUp = true
			}
			if owner == "stash_fragment" {
				g.stashDragPickedUp = true
			}
			h.ui.Draw(h.screen)
			x, y := partyPointerPosition(g, 1)
			g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: time.Now().UnixMilli()}}
			want := 0
			switch owner {
			case "stale_selection":
				g.selectedChar = 2
				want = 2
			case "stale_roster":
				g.party.Members[0], g.party.Members[1] = g.party.Members[1], g.party.Members[0]
			case "same_batch":
				x, y = partyPointerPosition(g, 2)
				g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: x, y: y, at: time.Now().UnixMilli() + 1})
				want = 1
			}
			h.ui.dispatchDisplayedInput()
			if g.selectedChar != want {
				t.Fatalf("selection=%d, want %d for %s", g.selectedChar, want, owner)
			}
		})
	}
}

func TestDisplayedPartySelectionBlocksEveryRenderedModal(t *testing.T) {
	for layer := modalLayerNone + 1; layer < modalLayerCount; layer++ {
		t.Run(fmt.Sprint(layer), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab, g.showPartyStats = true, TabSpellbook, true
			h.ui.Draw(h.screen)
			// A closed modal still owns the displayed pixels until the next
			// Draw. Every modal uses this same protection for the party strip.
			h.ui.renderedModalSnapshot.layer = layer
			x, y := partyPointerPosition(g, 1)
			g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: time.Now().UnixMilli()}}
			h.ui.dispatchDisplayedInput()
			if g.selectedChar != 0 {
				t.Fatal("portrait acted beneath the last displayed modal")
			}
		})
	}
}

func TestDisplayedPartyControlsRetainTheirOwnActions(t *testing.T) {
	for _, action := range []string{"stat_badge", "auto_stats", "plain_right", "incapacitated", "exhausted"} {
		t.Run(action, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.menuOpen, g.currentTab, g.showPartyStats = true, TabInventory, true
			g.turnBasedMode = true
			g.selectedChar = 2
			x, y := partyPointerPosition(g, 0)
			want := 2
			switch action {
			case "stat_badge":
				g.party.Members[0].FreeStatPoints = 5
				click := firstPartyStatBadgeClick(g, time.Now().UnixMilli())
				x, y = click.x, click.y
			case "auto_stats":
				g.party.Members[0].FreeStatPoints = 5
				w, h, left, top := partyPortraitLayout(g)
				px, py, pw, _ := partyCardPanelRect(left, top, w, h)
				auto := makePartyAutoButtonLayout(makePartyCardContentLayout(px, py, pw))
				x, y = auto.x+auto.w/2, auto.y+auto.h/2
			case "incapacitated":
				g.party.Members[0].HitPoints = 0
				want = 0
			case "exhausted":
				g.party.Members[0].ActionsRemaining = 0
				want = 0
			}
			h.clicks(action == "plain_right", x, y, 1)
			if g.selectedChar != want || g.focusModeActive() {
				t.Fatal("party control changed the wrong selection/focus state")
			}
			if action == "stat_badge" && (!g.statPopupOpen || g.statPopupCharIdx != 0) {
				t.Fatal("stat badge lost priority over portrait selection")
			}
			if action == "auto_stats" && g.party.Members[0].FreeStatPoints != 0 {
				t.Fatal("auto-assignment lost priority over portrait selection")
			}
		})
	}
}
