package game

import (
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

func TestFocusModePortraitInputConsumesMatchingQueuedRightClickOnce(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	ih := NewInputHandler(g)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	portraitX := baseLeft + portraitWidth + portraitWidth/2
	portraitY := startY + portraitHeight/2
	g.mouseRightClicks = []queuedClick{
		{x: 1, y: 1, at: 1},
		{x: portraitX, y: portraitY, at: 2},
	}

	ih.handlePartyPortraitMouseInput(true)
	if !g.partyMemberFocused(1) {
		t.Fatal("queued portrait right-click behind a world click did not toggle focus")
	}
	if len(g.mouseRightClicks) != 1 || g.mouseRightClicks[0].x != 1 {
		t.Fatalf("portrait consumer left queue %+v, want only the world click", g.mouseRightClicks)
	}

	ih.handlePartyPortraitMouseInput(true)
	if !g.partyMemberFocused(1) {
		t.Fatal("consumed portrait click fired a second time")
	}
}

func TestFocusModePortraitInputDoesNotConsumeNonPortraitRightClicks(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	g.party.Members = g.party.Members[:2]
	ih := NewInputHandler(g)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	emptySlotX := baseLeft + 3*portraitWidth + portraitWidth/2
	portraitY := startY + portraitHeight/2
	g.mouseRightClicks = []queuedClick{{x: emptySlotX, y: portraitY, at: 1}}

	ih.handlePartyPortraitMouseInput(true)
	if len(g.mouseRightClicks) != 1 {
		t.Fatal("right-click on an empty party slot was consumed")
	}
	if g.focusModeActive() {
		t.Fatal("right-click on an empty party slot enabled focus mode")
	}
}

func TestFocusModePortraitInputDoesNotConsumeStatButtonRightClick(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	g.party.Members[0].FreeStatPoints = 1
	ih := NewInputHandler(g)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	panelX, panelY, _, _ := partyCardPanelRect(baseLeft, startY, portraitWidth, portraitHeight)
	badges := makePartyProgressionBadgeLayout(
		panelX+panelPortraitX,
		panelY+panelPortraitY,
		panelPortraitW,
		panelPortraitH,
		true,
		false,
	)
	plusX := badges.stat.x + badges.stat.w/2
	plusY := badges.stat.y + badges.stat.h/2
	g.mouseRightClicks = []queuedClick{{x: plusX, y: plusY, at: 1}}

	ih.handlePartyPortraitMouseInput(true)
	if len(g.mouseRightClicks) != 1 {
		t.Fatal("right-click on the stat button was consumed as portrait focus")
	}
	if g.focusModeActive() {
		t.Fatal("right-click on the stat button enabled focus mode")
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

func TestFocusModePortraitInputKeepsLeftClickAsSelection(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	ih := NewInputHandler(g)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	g.mouseLeftClicks = []queuedClick{{
		x:  baseLeft + 2*portraitWidth + portraitWidth/2,
		y:  startY + portraitHeight/2,
		at: 1,
	}}

	ih.handlePartyPortraitMouseInput(true)
	if g.selectedChar != 2 {
		t.Fatalf("left portrait click selected %d, want 2", g.selectedChar)
	}
	if g.focusModeActive() {
		t.Fatalf("left portrait click enabled focus mask %04b", g.focusedPartyMask)
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatal("left portrait click was not consumed")
	}
}

func TestFocusModePortraitInputRespectsWorldClickGate(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	g.menuOpen = true
	ih := NewInputHandler(g)
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(g)
	g.mouseRightClicks = []queuedClick{{
		x:  baseLeft + portraitWidth/2,
		y:  startY + portraitHeight/2,
		at: 1,
	}}

	ih.handlePartyPortraitMouseInput(true)
	if g.focusModeActive() {
		t.Fatal("blocked portrait click enabled focus mode")
	}
	if len(g.mouseRightClicks) != 1 {
		t.Fatal("blocked portrait click was consumed")
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
	base, ok := asMetal(focusModeMetal)
	if !ok || base != focusModeMetal {
		t.Fatal("Focus mode label is not registered for metallic text rendering")
	}
	top := metalShade(focusModeMetal, 0)
	bottom := metalShade(focusModeMetal, 1)
	if top.B <= focusModeMetal.B || bottom.B >= focusModeMetal.B {
		t.Fatalf("focus metal ramp = top %#v base %#v bottom %#v", top, focusModeMetal, bottom)
	}
}
