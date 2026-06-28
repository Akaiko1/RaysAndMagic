package game

import (
	"strings"
	"testing"

	"ugataima/internal/items"
)

// Hovering kage_kunai while a Hunting Bow is equipped shows the item card + a
// comparison card. They must sit side by side without overlapping at ANY cursor
// X. Regression for the bug where a 200+ char compare line made the comparison
// card span the screen and bury the main card.
func TestTooltipCompare_NoHorizontalOverlap(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	screenW := cs.game.config.GetScreenWidth()

	var holder = cs.game.party.Members[0]
	for _, m := range cs.game.party.Members {
		if m != nil {
			holder = m
			break
		}
	}
	holder.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")

	kage := items.CreateWeaponFromYAML("kage_kunai")
	main := strings.Split(GetItemTooltip(kage, holder, cs, false), "\n")
	cmp := strings.Split(GetItemComparisonTooltip(kage, holder, cs), "\n")
	if len(cmp) == 0 || cmp[0] == "" {
		t.Fatalf("expected a comparison card for kage_kunai vs hunting_bow")
	}

	// Same sizing the renderer uses: each card capped to ~half the screen.
	gap := tooltipCompareGap
	cardCap := screenW/2 - gap
	mainW, _ := tooltipBoxSizeForScreen(main, nil, true, 0, cardCap)
	compareW, _ := tooltipBoxSizeForScreen(cmp, nil, false, 0, cardCap)
	if mainW > cardCap || compareW > cardCap {
		t.Errorf("a card exceeds the half-screen cap: mainW=%d compareW=%d cap=%d", mainW, compareW, cardCap)
	}
	t.Logf("screenW=%d cardCap=%d mainW=%d compareW=%d mainLines=%d cmpLines=%d",
		screenW, cardCap, mainW, compareW, len(main), len(cmp))

	for cursorX := 0; cursorX < screenW; cursorX += 8 {
		mainX, compareX := tooltipPairX(cursorX, mainW, compareW, gap, screenW)
		if compareX < mainX+mainW {
			t.Fatalf("overlap at cursorX=%d: main[%d,%d] compareX=%d", cursorX, mainX, mainX+mainW, compareX)
		}
		// The pair fits the screen (it's <= screenW by construction), so both cards
		// must land fully on screen.
		if mainX < 0 || compareX+compareW > screenW {
			t.Fatalf("pair off-screen at cursorX=%d: mainX=%d compareRight=%d screenW=%d",
				cursorX, mainX, compareX+compareW, screenW)
		}
	}

	// Vertical: the taller card drives the shared flip; with the cursor anywhere
	// (incl. the very bottom), flipTooltipY must keep the whole card on screen.
	screenH := cs.game.config.GetScreenHeight()
	_, mainH := tooltipBoxSizeForScreen(main, nil, true, 0, cardCap)
	_, compareH := tooltipBoxSizeForScreen(cmp, nil, false, 0, cardCap)
	h := mainH
	if compareH > h {
		h = compareH
	}
	t.Logf("screenH=%d mainH=%d compareH=%d", screenH, mainH, compareH)
	for _, cursorY := range []int{0, screenH / 2, screenH - 8} {
		y := flipTooltipY(cursorY+8, h, screenH)
		if y < 0 || y+h > screenH {
			t.Fatalf("card off-screen vertically: cursorY=%d -> y=%d h=%d screenH=%d", cursorY, y, h, screenH)
		}
	}
}
