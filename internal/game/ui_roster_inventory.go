package game

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

func rosterWarningButtons(area layoutRect) [2]layoutRect {
	w := min(186, (area.w-16)/2)
	x := area.x + (area.w-2*w-16)/2
	return [2]layoutRect{{x, area.bottom() - 36, w, 32}, {x + w + 16, area.bottom() - 36, w, 32}}
}

func (ui *UISystem) drawRosterInventoryWarning(screen *ebiten.Image, area layoutRect, interactive bool) {
	g := ui.game
	r := g.pendingRosterSwap
	drawDebugTextColored(screen, clipDebugText("Send "+r.outgoing.Name+" to reserve?", area.w), area.x, area.y, rarityGold)
	y := area.y + 30
	for _, line := range wrapDebugText("This hero carries important items. These stay with the hero and will be unavailable until they return to the party.", area.w) {
		drawDebugText(screen, line, area.x, y)
		y += debugTextCharHeight
	}
	y += 12
	buttons := rosterWarningButtons(area)
	visible := max(0, (buttons[0].y-12-y)/debugTextCharHeight)
	for i, name := range r.items {
		if i >= visible {
			break
		}
		line := "- " + name
		if i == visible-1 && len(r.items) > visible {
			line = fmt.Sprintf("... and %d more", len(r.items)-i)
		}
		drawDebugText(screen, clipDebugText(line, area.w), area.x, y+i*debugTextCharHeight)
	}
	mx, my := pointerPosition()
	for i, b := range buttons {
		label := "Send to reserve"
		if i == 1 {
			label = "Keep in party"
		}
		ui.drawMenuButton(screen, label, b.x, b.y, b.w, b.h, isMouseHoveringBox(mx, my, b.x, b.y, b.right(), b.bottom()))
		ui.onDisplayedInput(uiCommandClick, b, func() {
			if interactive && g.consumeLeftClickIn(b.x, b.y, b.right(), b.bottom()) {
				g.resolveRosterSwap(i == 0)
			}
		})
	}
}
