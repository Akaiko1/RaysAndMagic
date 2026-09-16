package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Both books keep selection on the icon, quick-slot ownership as a small gold
// marker, and availability as a red icon edge. None frames the whole entry.
func drawBookEntryState(screen *ebiten.Image, icon layoutRect, selected, equipped, locked, affordable bool, tint color.Color) {
	if locked {
		drawFilledRect(screen, icon.x, icon.y, icon.w, icon.h, color.RGBA{0, 0, 0, 110})
	}
	if selected {
		glow := color.RGBAModel.Convert(tint).(color.RGBA)
		outer, middle := glow, glow
		outer.A, middle.A = 80, 150
		drawRectBorder(screen, icon.x-4, icon.y-4, icon.w+8, icon.h+8, 1, outer)
		drawRectBorder(screen, icon.x-2, icon.y-2, icon.w+4, icon.h+4, 1, middle)
		drawRectBorder(screen, icon.x-1, icon.y-1, icon.w+2, icon.h+2, 1, glow)
	}
	if equipped {
		// This permanent cue remains visible when browsing another entry.
		drawFilledRect(screen, icon.right()+3, icon.y+icon.h/2-5, 2, 10, rarityGold)
	}
	if locked || !affordable {
		drawRectBorder(screen, icon.x, icon.y, icon.w, icon.h, 1, color.RGBA{120, 38, 28, 255})
	}
}
