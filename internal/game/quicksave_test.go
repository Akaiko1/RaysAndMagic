package game

import (
	"slices"
	"testing"

	"ugataima/internal/game/keytracker"

	"github.com/hajimehoshi/ebiten/v2"
)

// keysPressed is a key source with these keys pressed this frame.
func keysPressed(keys ...ebiten.Key) keytracker.Consumer {
	return keytracker.NewWithSource(func(k ebiten.Key) bool { return slices.Contains(keys, k) })
}

// pressKeys runs one input frame with the given keys pressed and held.
func pressKeys(ih *InputHandler, keys ...ebiten.Key) {
	ih.keys = keysPressed(keys...)
	ih.heldKeys = heldOnly(keys...)
	ih.HandleInput()
}

// F5 writes the Quicksave slot and Shift+F4 loads it back, wherever the ESC
// menu could open; F4 alone, a modal layer, or no quicksave do nothing. The
// Quicksave is its own file, loaded but never written by the menus.
func TestQuicksaveKeys(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g, ih := h.g, h.loop.inputHandler
	g.party.Gold = 100

	// No quicksave yet: Shift+F4 says so and changes nothing.
	pressKeys(ih, ebiten.KeyShiftLeft, ebiten.KeyF4)
	if g.party.Gold != 100 || GetSaveRowSummary(quicksaveRow).Exists {
		t.Fatal("a quickload without a quicksave changed the game")
	}

	for _, tc := range []struct {
		name string
		pose func()
	}{
		{"exploration", func() {}},
		{"hub open", func() { g.menuOpen = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.pose()
			g.party.Gold = 100
			pressKeys(ih, ebiten.KeyF5)
			if !GetSaveRowSummary(quicksaveRow).Exists {
				t.Fatal("F5 wrote no quicksave")
			}
			g.party.Gold = 5
			pressKeys(ih, ebiten.KeyF4)
			if g.party.Gold != 5 {
				t.Fatal("F4 without Shift loaded the quicksave")
			}
			pressKeys(ih, ebiten.KeyShiftRight, ebiten.KeyF4)
			if g.party.Gold != 100 {
				t.Fatalf("Shift+F4 left gold at %d, want the quicksaved 100", g.party.Gold)
			}
			if g.menuOpen {
				t.Fatal("the hub stayed open over the loaded game")
			}
		})
	}

	t.Run("modal layer", func(t *testing.T) {
		g.party.Gold = 100
		pressKeys(ih, ebiten.KeyF5)
		g.openMainMenu()
		g.party.Gold = 7
		pressKeys(ih, ebiten.KeyF5)
		pressKeys(ih, ebiten.KeyShiftLeft, ebiten.KeyF4)
		if g.party.Gold != 7 {
			t.Fatal("the quick keys acted under the ESC menu")
		}
		g.closeMainMenu()
		pressKeys(ih, ebiten.KeyShiftLeft, ebiten.KeyF4)
		if g.party.Gold != 100 {
			t.Fatal("the quicksave was overwritten under the ESC menu")
		}
	})

	t.Run("menus load it but never write it", func(t *testing.T) {
		g.party.Gold = 100
		pressKeys(ih, ebiten.KeyF5)
		g.party.Gold = 3
		g.Autosave()
		g.appScreen = AppScreenInGame
		if err := g.SaveGameToFile(saveRowPath(autosaveRow)); err != nil {
			t.Fatal(err)
		}
		g.openMainMenu()
		g.mainMenuMode = MenuSaveSelect
		g.savePage, g.slotSelection = 0, quicksaveRow
		ih.doSaveToSelectedRow()
		g.mainMenuMode = MenuLoadSelect
		ih.doLoadFromSelectedRow()
		if g.party.Gold != 100 {
			t.Fatalf("loading the Quicksave row gave gold %d, want 100 (an autosave or menu save overwrote it)", g.party.Gold)
		}
	})
}
