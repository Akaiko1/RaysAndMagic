package game

import (
	"reflect"
	"testing"

	"slices"
	"ugataima/internal/character"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
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
		click := firstPartyStatChipClick(g, 0)
		return queuedClick{x: click.x, y: click.y}
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

	chips := g.partyLevelUpLayoutFor(0)
	rail, _ := inGameActionBarLayout(g)
	if chips.stat.w == 0 || chips.skill.w == 0 || !rail.hasAuto {
		t.Fatal("fixture lost a progression control (positive control)")
	}
	controls := map[string]layoutRect{
		"stat":  chips.stat,
		"skill": chips.skill,
		"auto":  rail.auto,
	}
	for name, control := range controls {
		t.Run(name, func(t *testing.T) {
			x := control.x + control.w/2
			y := control.y + control.h/2
			if got := ih.partyPortraitClickTarget(x, y); got != -1 {
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
	g.party.Members[0].Equipment[items.SlotQuickAction] = items.Item{
		Name:      "Test spell",
		SpellCost: 0,
	}
	delete(g.party.Members[1].Equipment, items.SlotQuickAction)
	g.togglePartyFocus(1)
	g.selectedChar = 0

	if got := g.nextReadyRTActor(rtActSlotted); got != -1 {
		t.Fatalf("cast escaped focus to actor %d", got)
	}
	g.advanceRTActor(rtActSlotted)
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

// Pointing at a hero's card means that hero everywhere a card is a target -
// C/F heals, Heal Other, the ally-spell target - even over its level-up
// chip; only a selection click yields the chip to its own control. Off the
// cards the targets fall back to the selected hero or the caster. An ally
// spell's paused picker takes the same whole card.
func TestPointingAtACardMeansItsWholeCard(t *testing.T) {
	g := focusModeTestGame(t)
	g.showPartyStats = true
	g.selectedChar = 0
	g.party.Members[1].FreeStatPoints = 5
	g.combat = NewCombatSystem(g)
	ih := NewInputHandler(g)
	g.gameLoop = &GameLoop{game: g, inputHandler: ih}
	prev := rawCursorPosition
	t.Cleanup(func() { rawCursorPosition = prev })

	chip := g.partyLevelUpLayoutFor(1).stat
	if chip.w == 0 {
		t.Fatal("hero 1 shows no stat chip (positive control)")
	}
	pw, ph, left, top := partyPortraitLayout(g)
	healOther, _ := spells.GetSpellDefinitionByID("heal_other")
	for _, tc := range []struct {
		name         string
		x, y         int
		target, pick int // pick: the selection click's hero, -1 for none
	}{
		{"chip", chip.x + chip.w/2, chip.y + chip.h/2, 1, -1},
		{"card body", left + pw + pw/4, top + ph/3, 1, 1},
		{"off the cards", left + pw, top - 40, -1, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawCursorPosition = func() (int, int) { return tc.x, tc.y }
			if got := g.combat.pointedHealTarget(healOther, g.heroUnderCursor()); got != tc.target {
				t.Errorf("a hand heal (F, C, quick bar, book) points at %d, want %d", got, tc.target)
			}
			if got := g.partyMemberAt(tc.x, tc.y); got != tc.target {
				t.Errorf("the ally-spell picker takes %d, want %d", got, tc.target)
			}
			if got := ih.partyPortraitClickTarget(tc.x, tc.y); got != tc.pick {
				t.Errorf("a selection click picks %d, want %d", got, tc.pick)
			}
		})
	}
}

// A level-up chip rings and explains itself, and a shortened status list names
// the whole list, only while the card's clicks are live: in play and under the
// character hub, not under a dialog, the skill choice or High Scores.
func TestLevelUpChipHoverFollowsItsClick(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(g *MMGame)
		live  bool
	}{
		{"in play", func(*MMGame) {}, true},
		{"character hub", func(g *MMGame) { g.menuOpen = true }, true},
		{"npc dialog", func(g *MMGame) { g.dialogActive, g.dialogNPC = true, &character.NPC{Name: "Trader"} }, false},
		{"skill choice", func(g *MMGame) {
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 2, options: []levelUpChoiceOption{{}}}}
			g.levelUpChoiceOpen = true
		}, false},
		{"high scores", func(g *MMGame) { g.showHighScores = true }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := focusModeTestGame(t)
			g.showPartyStats = true
			g.sprites = graphics.NewSpriteManager()
			g.party.Members[0].FreeStatPoints = 5
			ui := NewUISystem(g)
			tc.setup(g)
			if ui.partyCardClicksBlocked() == tc.live {
				t.Fatalf("party-card clicks blocked = %v (fixture)", !tc.live)
			}
			chip := g.partyLevelUpLayoutFor(0).stat
			prev := rawCursorPosition
			t.Cleanup(func() { rawCursorPosition = prev })
			rawCursorPosition = func() (int, int) { return chip.x + chip.w/2, chip.y + chip.h/2 }
			ui.clearQueuedTooltips()
			ui.drawPartyUI(ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight()))
			shown := slices.Contains(ui.tooltipLines, "Click to assign")
			if shown != tc.live {
				t.Fatalf("chip tooltip shown = %v, want %v", shown, tc.live)
			}

			// A shortened status list names the whole list on hover, under the same gate.
			hero := g.party.Members[0]
			hero.Conditions = []character.Condition{character.ConditionPoisoned, character.ConditionBurning, character.ConditionStunned,
				character.ConditionCursed, character.ConditionDiseased, character.ConditionParalyzed}
			pw, ph, left, top := partyPortraitLayout(g)
			x, y, w, _ := partyCardPanelRect(left, top, pw, ph)
			content := makePartyCardContentLayout(x, y, w)
			full, short := g.partyConditionLabels(hero)
			line := partyStatusLineLayout(content, y+67, full, short, g.partyLevelUpLayoutFor(0).statusRight(content))
			if line.full == "" {
				t.Fatalf("status %q is not shortened (fixture)", full)
			}
			rawCursorPosition = func() (int, int) { return line.box.x + 2, line.box.y + line.box.h/2 }
			beginUIFrame(ebiten.NewImage(1, 1), 1)
			ui.clearQueuedTooltips()
			ui.drawPartyUI(ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight()))
			ui.offerClippedLabelTooltip()
			if named := slices.Contains(ui.tooltipLines, full); named != tc.live {
				t.Fatalf("the whole status list on hover = %v, want %v", named, tc.live)
			}
		})
	}
}
