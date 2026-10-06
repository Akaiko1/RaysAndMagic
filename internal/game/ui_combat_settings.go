package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

type combatSettingsText struct {
	label    string
	box      layoutRect
	centered bool
}

func combatSettingsTextLayout(px, py, panelW int) []combatSettingsText {
	return []combatSettingsText{
		{"Optional aids for turn-based combat", layoutRect{px + 28, py + 100, panelW - 56, 18}, true},
		{"Battlefield aurora", layoutRect{px + 44, py + 138, 0, 0}, false},
		{"Turn-based mode only", layoutRect{px + 44, py + 162, 0, 0}, false},
		{"Light blue: battlefield tiles", layoutRect{px + 76, py + 224, 0, 0}, false},
		{"Red: enemy attack and pounce reach", layoutRect{px + 76, py + 256, 0, 0}, false},
		{"Green: enemy movement boundary", layoutRect{px + 76, py + 288, 0, 0}, false},
		{"Low curtains keep the battlefield readable.", layoutRect{px + 28, py + 340, panelW - 56, 18}, true},
		{"Other auroras take priority. Flames and bubbles coexist.", layoutRect{px + 28, py + 364, panelW - 56, 18}, true},
	}
}

func combatOverlayChoiceRect(px, py, panelW int, enabled bool) layoutRect {
	x := px + panelW - 210
	if enabled {
		x += 80
	}
	return layoutRect{x, py + 140, 72, 32}
}

func (ui *UISystem) drawCombatSettings(screen *ebiten.Image, px, py, panelW int) {
	g := ui.game
	box := layoutRect{px + 28, py + 124, panelW - 56, 72}
	drawFilledRect(screen, box.x, box.y, box.w, box.h, color.RGBA{24, 27, 30, 255})
	drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, color.RGBA{177, 149, 89, 255})
	for i, text := range combatSettingsTextLayout(px, py, panelW) {
		b := text.box
		if text.centered {
			drawCenteredUIText(screen, text.label, b.x, b.y, b.w, b.h)
			continue
		}
		tint := color.RGBA{210, 209, 197, 255}
		if i == 1 {
			tint = color.RGBA{230, 213, 172, 255}
		}
		if i == 2 {
			tint = color.RGBA{156, 158, 158, 255}
		}
		drawUITextColored(screen, text.label, b.x, b.y, tint)
	}
	for _, enabled := range []bool{false, true} {
		label := "Off"
		if enabled {
			label = "On"
		}
		r := combatOverlayChoiceRect(px, py, panelW, enabled)
		ui.drawSettingsChoice(screen, label, r, g.combatPreferences.TurnBasedOverlay == enabled)
		ui.settingsClick(r, func() { g.setCombatOverlay(enabled) })
	}
	for i, rgb := range [][3]int{combatAuraBlue, combatAuraRed, combatAuraGreen} {
		y := py + 224 + i*32
		tint := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		drawFilledRect(screen, px+48, y+3, 16, 10, tint)
	}
}
