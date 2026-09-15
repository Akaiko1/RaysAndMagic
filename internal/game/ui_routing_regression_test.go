package game

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game/keytracker"
)

func TestDisplayedCombatLogCapturesOnlyItsInputFrame(t *testing.T) {
	for _, kind := range []string{"single", "double", "expired", "covered"} {
		t.Run(kind, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			g.world.Monsters = nil
			g.maxMessages = 4
			g.AddCombatMessage("A visible message")
			fp := installFakePointer(t)
			if kind == "covered" {
				g.menuOpen, g.currentTab = true, TabInventory
			}
			x, y, _, _ := combatMessageArea(g)
			fp.moveTo(x+10, y+10)
			fp.press()
			h.pointerStep()
			if g.combatLogOpen {
				t.Fatal("single log click opened the overlay")
			}
			if h.ui.displayedInput.capturedGameplay != (kind != "covered") {
				t.Fatal("visible/covered log captured the wrong input owner")
			}
			fp.release()
			h.pointerStep()
			fp.idle()
			if h.ui.displayedInput.capturedGameplay {
				t.Fatal("UI capture survived its Update")
			}
			if kind == "double" || kind == "expired" {
				if kind == "expired" {
					g.lastCombatLogClick = time.Now().UnixMilli() - doubleClickWindowMs - 1
				}
				fp.press()
				h.pointerStep()
				if g.combatLogOpen != (kind == "double") {
					t.Fatal("log double-click timing changed")
				}
			}
		})
	}
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.world.Monsters = nil
	g.AddCombatMessage("Capture gameplay input")
	fp := installFakePointer(t)
	h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyI })
	x, y, _, _ := combatMessageArea(g)
	fp.moveTo(x+10, y+10)
	fp.press()
	h.pointerStep()
	if g.menuOpen {
		t.Fatal("same-frame gameplay key acted after log capture")
	}
	fp.hold()
	h.pointerStep()
	if !g.menuOpen {
		t.Fatal("log capture blocked the next frame's keyboard input")
	}
}

func TestDisplayedGestureCancellationPrecedesPointerRelease(t *testing.T) {
	for _, screen := range []string{"title", "creation"} {
		t.Run(screen, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			fp := installFakePointer(t)
			cancel := false
			previous := pointerCancelJustPress
			pointerCancelJustPress = func() bool { return cancel }
			t.Cleanup(func() { pointerCancelJustPress = previous })
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return cancel && k == ebiten.KeyEscape })
			if screen == "title" {
				g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuRoot
				x, y := titleButtonPoint(g, "settings")
				fp.moveTo(x, y)
			} else {
				g.enterPartyCreate()
				lay := partyCreateLayout(g.partyCreate, 1024, 768)
				fp.moveTo(lay.slots[0].x+3, lay.slots[0].y+3)
			}
			presentInputScreen(h)
			fp.press()
			updateInputScreen(h)
			if screen == "creation" {
				lay := partyCreateLayout(g.partyCreate, 1024, 768)
				fp.moveTo(lay.slots[1].x+3, lay.slots[1].y+3)
				fp.hold()
				updateInputScreen(h)
			}
			before := g.partyCreate
			var firstHero *pcHero
			if before != nil {
				firstHero = before.slots[0]
			}
			cancel = true
			fp.release()
			updateInputScreen(h)
			if screen == "title" && (g.entryMenuMode != EntryMenuRoot || g.entryMenuRootPressArmed) {
				t.Fatal("release activated a title button before Escape cancellation")
			}
			if screen == "creation" && (g.partyCreate != before || before.drag != nil || before.pending != nil || before.slots[0] != firstHero) {
				t.Fatal("Escape failed to cancel the creation drop")
			}
		})
	}
}

func TestTitlePointerCannotArmCoveredInventoryDrag(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuRoot
	g.menuOpen, g.currentTab = true, TabInventory
	menu := computeTabbedMenuLayout(1024, gameplayViewportBottom(g))
	layout := computeInventoryContentLayout(menu.content)
	x, y, w, height := scaleInventorySourceRect(layout.grid.x, layout.grid.y, layout.grid.w, layout.grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
	fp := installFakePointer(t)
	fp.moveTo(x+w/2, y+height/2)
	presentInputScreen(h)
	fp.press()
	updateInputScreen(h)
	if g.dragArmed || g.dragActive || g.stashDragArmed || g.stashDragActive {
		t.Fatal("title input reached hidden gameplay drag recognizers")
	}
}
