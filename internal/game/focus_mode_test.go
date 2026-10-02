package game

import (
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func focusModeTestGame(t *testing.T) *MMGame {
	t.Helper()
	cfg := loadTestConfig(t)
	return newTestGame(cfg, newTestWorld(cfg))
}

func TestFocusModeShiftRightClickTogglesPartyMembers(t *testing.T) {
	g := focusModeTestGame(t)

	if !g.handlePartyPortraitClick(1, true) {
		t.Fatal("Shift+right-click did not select party member 1")
	}
	if !g.focusModeActive() || !g.partyMemberFocused(1) || g.selectedChar != 1 {
		t.Fatalf("first Shift+right-click = mask %04b selected %d", g.focusedPartyMask, g.selectedChar)
	}

	g.handlePartyPortraitClick(3, true)
	if !g.partyMemberFocused(1) || !g.partyMemberFocused(3) {
		t.Fatalf("two focused members = mask %04b", g.focusedPartyMask)
	}

	g.handlePartyPortraitClick(1, true)
	if g.partyMemberFocused(1) || !g.partyMemberFocused(3) {
		t.Fatalf("removing member 1 = mask %04b", g.focusedPartyMask)
	}

	g.handlePartyPortraitClick(3, true)
	if g.focusModeActive() {
		t.Fatalf("removing the final member left mask %04b", g.focusedPartyMask)
	}
}

// Portrait clicks through the real mouse consumer: Shift+right-click on a
// portrait toggles focus once (even queued behind a world click), a click that
// misses a portrait (empty slot, stat button) or meets the world-click gate
// stays queued, and a left click only selects. A repeated pass changes nothing.
func TestFocusModePortraitClickInput(t *testing.T) {
	portrait := func(g *MMGame, slot int) queuedClick {
		w, h, left, top := partyPortraitLayout(g)
		return queuedClick{x: left + slot*w + w/2, y: top + h/2}
	}
	statButton := func(g *MMGame) queuedClick {
		w, h, left, top := partyPortraitLayout(g)
		panelX, panelY, _, _ := partyCardPanelRect(left, top, w, h)
		badges := makePartyProgressionBadgeLayout(panelX+panelPortraitX, panelY+panelPortraitY, panelPortraitW, panelPortraitH, true, false)
		return queuedClick{x: badges.stat.x + badges.stat.w/2, y: badges.stat.y + badges.stat.h/2}
	}
	for _, tc := range []struct {
		name     string
		setup    func(g *MMGame)
		right    bool
		clicks   func(g *MMGame) []queuedClick
		keep     []int // indices of the clicks that must stay queued
		focused  int   // the only focused member, -1 for none
		selected int
	}{
		{"right_click_behind_world_click_toggles_once", func(*MMGame) {}, true,
			func(g *MMGame) []queuedClick { return []queuedClick{{x: 1, y: 1}, portrait(g, 1)} }, []int{0}, 1, 1},
		{"right_click_on_empty_slot_stays_queued", func(g *MMGame) { g.party.Members = g.party.Members[:2] }, true,
			func(g *MMGame) []queuedClick { return []queuedClick{portrait(g, 3)} }, []int{0}, -1, 0},
		{"right_click_on_stat_button_stays_queued", func(g *MMGame) { g.party.Members[0].FreeStatPoints = 1 }, true,
			func(g *MMGame) []queuedClick { return []queuedClick{statButton(g)} }, []int{0}, -1, 0},
		{"left_click_selects_without_focus", func(*MMGame) {}, false,
			func(g *MMGame) []queuedClick { return []queuedClick{portrait(g, 2)} }, nil, -1, 2},
		{"world_click_gate_keeps_right_click", func(g *MMGame) { g.menuOpen = true }, true,
			func(g *MMGame) []queuedClick { return []queuedClick{portrait(g, 0)} }, []int{0}, -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := focusModeTestGame(t)
			g.showPartyStats = true
			tc.setup(g)
			ih := NewInputHandler(g)
			clicks := tc.clicks(g)
			for i := range clicks {
				clicks[i].at = int64(i + 1)
			}
			want := []queuedClick{}
			for _, i := range tc.keep {
				want = append(want, clicks[i])
			}
			queue := &g.mouseLeftClicks
			if tc.right {
				queue = &g.mouseRightClicks
			}
			*queue = append([]queuedClick(nil), clicks...)
			for pass := 1; pass <= 2; pass++ {
				ih.handlePartyPortraitMouseInput(true)
				if !reflect.DeepEqual(append([]queuedClick{}, *queue...), want) {
					t.Fatalf("pass %d left queue %+v, want %+v", pass, *queue, want)
				}
				wantMask := 0
				if tc.focused >= 0 {
					wantMask = 1 << tc.focused
				}
				if int(g.focusedPartyMask) != wantMask {
					t.Fatalf("pass %d focus mask %04b, want %04b", pass, g.focusedPartyMask, wantMask)
				}
				if g.selectedChar != tc.selected {
					t.Fatalf("pass %d selected %d, want %d", pass, g.selectedChar, tc.selected)
				}
			}
		})
	}
}

func TestPartyProgressionControlsDoNotBecomePortraitSelectionClicks(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	g.party.Members[0].FreeStatPoints = 2
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0}}
	ih := NewInputHandler(g)

	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	panelX, panelY, panelW, _ := partyCardPanelRect(baseLeft, startY, portraitWidth, portraitHeight)
	badges := makePartyProgressionBadgeLayout(
		panelX+panelPortraitX,
		panelY+panelPortraitY,
		panelPortraitW,
		panelPortraitH,
		true,
		true,
	)
	auto := makePartyAutoButtonLayout(makePartyCardContentLayout(panelX, panelY, panelW))

	controls := map[string]layoutRect{
		"stat":  badges.stat,
		"skill": badges.skill,
		"auto":  auto,
	}
	for name, control := range controls {
		t.Run(name, func(t *testing.T) {
			x := control.x + control.w/2
			y := control.y + control.h/2
			if got := ih.getPartyMemberUnderMouse(x, y); got != -1 {
				t.Fatalf("control click routed to party member %d", got)
			}

			g.selectedChar = 1
			g.mouseLeftClicks = []queuedClick{{x: x, y: y, at: 1}}
			ih.handlePartyPortraitMouseInput(false)
			if g.selectedChar != 1 {
				t.Fatalf("control click changed selection to %d", g.selectedChar)
			}
			if len(g.mouseLeftClicks) != 1 {
				t.Fatal("control click was consumed by portrait selection")
			}
		})
	}
}

func TestFocusModeLeavesOrdinarySelectionIndependent(t *testing.T) {
	g := focusModeTestGame(t)
	g.handlePartyPortraitClick(1, true)
	g.handlePartyPortraitClick(2, false)

	if g.selectedChar != 2 {
		t.Fatalf("ordinary portrait click selected %d, want 2", g.selectedChar)
	}
	if !g.partyMemberFocused(1) || g.partyMemberFocused(2) {
		t.Fatalf("ordinary click changed focus mask to %04b", g.focusedPartyMask)
	}

	g.clearFocusMode()
	g.menuOpen = true
	g.handlePartyPortraitClick(3, true)
	if g.focusModeActive() {
		t.Fatal("Shift+right-click inside a menu enabled combat focus")
	}
}

func TestFocusModeRestrictsRealTimeActorCycle(t *testing.T) {
	g := focusModeTestGame(t)
	g.togglePartyFocus(1)
	g.togglePartyFocus(3)

	g.selectedChar = 0
	if g.rtActionCapable(0, rtActSmart) {
		t.Fatal("unfocused member remained capable of a keyboard combat action")
	}
	if got := g.nextReadyRTActor(rtActSmart); got != 1 {
		t.Fatalf("next focused actor = %d, want 1", got)
	}

	g.selectedChar = 1
	g.advanceRTActor(rtActSmart)
	if g.selectedChar != 3 {
		t.Fatalf("focus cycle advanced to %d, want 3", g.selectedChar)
	}
	g.advanceRTActor(rtActSmart)
	if g.selectedChar != 1 {
		t.Fatalf("focus cycle wrapped to %d, want 1", g.selectedChar)
	}
}

func TestFocusModeDoesNotFallBackOutsideFocus(t *testing.T) {
	g := focusModeTestGame(t)
	g.party.Members[0].Equipment[items.SlotSpell] = items.Item{
		Name:      "Test spell",
		SpellCost: 0,
	}
	delete(g.party.Members[1].Equipment, items.SlotSpell)
	g.togglePartyFocus(1)
	g.selectedChar = 0

	if got := g.nextReadyRTActor(rtActCast); got != -1 {
		t.Fatalf("cast escaped focus to actor %d", got)
	}
	g.advanceRTActor(rtActCast)
	if g.selectedChar != 0 {
		t.Fatalf("failed focused cast moved selection to %d", g.selectedChar)
	}
}

func TestFocusModeClearsOnTurnBasedEntryAndRosterSwap(t *testing.T) {
	g := focusModeTestGame(t)
	g.togglePartyFocus(0)
	g.togglePartyFocus(2)

	g.ToggleTurnBasedMode()
	if g.focusModeActive() {
		t.Fatalf("TB entry retained focus mask %04b", g.focusedPartyMask)
	}
	g.ToggleTurnBasedMode()
	if g.focusModeActive() {
		t.Fatal("return to RT restored stale focus")
	}

	bench := character.CreateCharacter("Focus Reset", character.ClassPaladin, g.config)
	g.party.Recruit(bench)
	g.togglePartyFocus(1)
	if !g.swapRosterMember(1, len(g.party.Reserve)-1) {
		t.Fatal("roster swap failed")
	}
	if g.focusModeActive() {
		t.Fatalf("roster swap retained focus mask %04b", g.focusedPartyMask)
	}
}

func TestFocusModeClearsWhenLeavingGameplay(t *testing.T) {
	g := focusModeTestGame(t)
	g.togglePartyFocus(2)
	g.returnToMainMenu()

	if g.focusModeActive() {
		t.Fatalf("main-menu transition retained focus mask %04b", g.focusedPartyMask)
	}
}

func TestFocusModeIndicatorUsesMetallicBlueRamp(t *testing.T) {
	top := metalShade(focusModeMetal, 0)
	bottom := metalShade(focusModeMetal, 1)
	if top.B <= focusModeMetal.B || bottom.B >= focusModeMetal.B {
		t.Fatalf("focus metal ramp = top %#v base %#v bottom %#v", top, focusModeMetal, bottom)
	}
}
