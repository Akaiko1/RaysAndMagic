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

func TestFocusModeShiftClickTogglesPartyMembers(t *testing.T) {
	g := focusModeTestGame(t)

	if !g.handlePartyPortraitClick(1, true) {
		t.Fatal("Shift-click did not select party member 1")
	}
	if !g.focusModeActive() || !g.partyMemberFocused(1) || g.selectedChar != 1 {
		t.Fatalf("first Shift-click = mask %04b selected %d", g.focusedPartyMask, g.selectedChar)
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
		t.Fatal("Shift-click inside a menu enabled combat focus")
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
