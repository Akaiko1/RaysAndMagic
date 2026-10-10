package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
)

// The slim rim, header display and two recessed counters are decoration
// around the existing shared trade rectangles, never separate input surfaces.
func computeMerchantDialogSectionLayout(panel layoutRect) npcDialogSectionLayout {
	l := computeNPCDialogSectionLayout(panel, true)
	l.title = layoutRect{panel.x + 28, panel.y + 28, 332, uiTextCharHeight}
	l.greeting = layoutRect{panel.x + 28, panel.y + 72, panel.w - 56, 3 * dialogueLineHeight}
	l.balance = layoutRect{panel.x + 28, panel.y + 120, 332, uiTextCharHeight}
	for i := range l.footer {
		l.footer[i] = layoutRect{panel.x + 28, panel.bottom() - 54 + i*uiTextCharHeight, panel.w - 56, uiTextCharHeight}
	}
	return l
}

func merchantDisplayRect(panel layoutRect) layoutRect {
	// The transparent display straddles the top rail, leaving the greeting
	// full-width below it. It ends at the top-right corner ornament's inner
	// edge, which also clears the close button.
	x := panel.right() - 220
	return layoutRect{x, panel.y - 56, panel.right() + merchantCornerOutset - merchantCornerSize - x, 112}
}

func merchantFolderTabRect(dialogX, dialogY, index, count int) layoutRect {
	display := merchantDisplayRect(layoutRect{dialogX, dialogY, npcDialogWidth, merchantDialogHeight})
	x, y, w, h := dialogFolderTabRect(dialogX, dialogY, 0)
	// Authored stock tabs share the rail with the floating display. Fit the
	// complete tab group to its left; drawing and input use this same rect.
	w = min(w, (display.x-12-x-(count-1)*dialogFolderTabGap)/count)
	return layoutRect{x + index*(w+dialogFolderTabGap), y, w, h}
}

const (
	merchantCornerSprite = "merchant_corner"
	// The ornament is drawn at a fixed size, outset past the frame corner.
	merchantCornerSize   = 64
	merchantCornerOutset = 8
)

func (ui *UISystem) drawMerchantFrame(screen *ebiten.Image, panel layoutRect) {
	ui.drawThemeFrame(screen, frameBronze, panel.x, panel.y, panel.w, panel.h)
	if ui.game.sprites == nil {
		return
	}
	sprite := ui.game.sprites.GetSprite(merchantCornerSprite)
	if sprite == nil {
		return
	}
	// The source's slim rails lie about 12% into its canvas. At 64px,
	// an 8px outset aligns them with the modal rim. Mirror one transparent
	// ornament, never stretch its geometry with the panel.
	const size, outset = merchantCornerSize, merchantCornerOutset
	for i := 0; i < 4; i++ {
		x, y, w, h := panel.x-outset, panel.y-outset, size, size
		if i%2 != 0 {
			x, w = panel.right()+outset, -size
		}
		if i/2 != 0 {
			y, h = panel.bottom()+outset, -size
		}
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(float64(w)/float64(sprite.Bounds().Dx()), float64(h)/float64(sprite.Bounds().Dy()))
		op.GeoM.Translate(float64(x), float64(y))
		uiDrawImage(screen, sprite, op)
	}
}

func (ui *UISystem) drawMerchantDisplay(screen *ebiten.Image, panel layoutRect) {
	// Read presentation from the authored definition, not the saved NPC or its
	// depleted/filtered stock. Selling the last sword must not change the shop.
	if npc := ui.game.dialogNPC; npc != nil && character.NPCConfigInstance != nil && ui.game.sprites != nil {
		if def, ok := character.NPCConfigInstance.GetNPCData(npc.Key); ok && def != nil && def.ShopDisplay != "" {
			if sprite := ui.game.sprites.GetSprite(def.ShopDisplay); sprite != nil {
				r := merchantDisplayRect(panel)
				b := sprite.Bounds()
				s := min(float64(r.w)/float64(b.Dx()), float64(r.h)/float64(b.Dy()))
				w, h := int(float64(b.Dx())*s), int(float64(b.Dy())*s)
				drawImageScaled(screen, sprite, r.x+(r.w-w)/2, r.y+(r.h-h)/2, w, h)
			}
		}
	}
	left, right, top, pager := merchantGridLayout(panel.x, panel.y)
	for _, x := range []int{left, right} {
		// Shared warm recessed surfaces keep prices and categories above the
		// decoration, with ten pixels of air on either side of the item grid.
		y, bottom := top-84, pager+pagerBtnH+8
		drawFilledRect(screen, x-10, y, merchantGridW+20, bottom-y, color.RGBA{27, 24, 20, 255})
		drawRectBorder(screen, x-10, y, merchantGridW+20, bottom-y, 1, color.RGBA{74, 61, 44, 255})
	}
}
