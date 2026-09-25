package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
)

func npcDialogCloseRect(dlg npcDialogRect) layoutRect {
	return layoutRect{dlg.x + dlg.w - 42, dlg.y + 10, 28, 28}
}

// Navigation belongs to the displayed UI dispatcher, just like other modal
// controls. It cannot depend on quest-filtered or authored dialogue choices.
func (ui *UISystem) drawDialogueNavigation(screen *ebiten.Image, layout dialogueContentLayout, x, y int) {
	interactive := ui.topModalLayer() == modalLayerDialog
	draw := func(rect layoutRect, label string, action func()) {
		rect.x += x
		rect.y += y
		mx, my := pointerPosition()
		hover := isMouseHoveringBox(mx, my, rect.x, rect.y, rect.right(), rect.bottom())
		ui.drawMenuButton(screen, label, rect.x, rect.y, rect.w, rect.h, hover)
		ui.onDisplayedInput(uiCommandNavigation, rect, func() {
			if interactive && ui.game.consumeLeftClickIn(rect.x, rect.y, rect.right(), rect.bottom()) {
				action()
			}
		})
	}
	if ui.game.currentDialogNode() != nil {
		draw(layout.backButton, uitext.Text("dialog.back"), ui.game.backConversation)
	}
	draw(layout.leaveButton, uitext.Text("dialog.leave"), ui.game.closeConversation)
}
