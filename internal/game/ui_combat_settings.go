package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	gameplayRowView = iota
	gameplayRowAurora
)

// The same warm ink and bronze used by the other Settings tabs. Gold marks
// the active choice; the quiet section edge identifies keyboard focus.
var gameplayInk = struct {
	heading, text, muted, edge, focus, selected color.RGBA
}{
	color.RGBA{230, 213, 172, 255},
	color.RGBA{210, 209, 197, 255},
	color.RGBA{156, 158, 158, 255},
	color.RGBA{68, 65, 55, 255},
	color.RGBA{177, 149, 89, 255},
	color.RGBA{43, 39, 29, 255},
}

type combatSettingsText struct {
	label string
	box   layoutRect
	tint  color.RGBA
}

func combatSettingsTextLayout(px, py, panelW int) []combatSettingsText {
	texts := []combatSettingsText{
		{"Field of view", layoutRect{px + 44, py + 116, 0, 0}, gameplayInk.heading},
		{"Battlefield aurora", layoutRect{px + 44, py + 312, 0, 0}, gameplayInk.heading},
		{"Show enemy reach in turn-based combat.", layoutRect{px + 44, py + 340, 0, 0}, gameplayInk.muted},
	}
	labels := [...]string{"Battlefield", "Attack reach", "Movement reach"}
	for i, label := range labels {
		texts = append(texts, combatSettingsText{label, layoutRect{px + 80 + i*((panelW-88)/3), py + 390, 0, 0}, gameplayInk.text})
	}
	return texts
}

func viewChoiceRect(px, py, panelW int, wide bool) layoutRect {
	const gap = 16
	w := (panelW - 88 - gap) / 2
	x := px + 44
	if wide {
		x += w + gap
	}
	return layoutRect{x, py + 148, w, 128}
}

func viewChoiceTextLayout(r layoutRect, wide bool) []combatSettingsText {
	labels := [4]string{"Classic", "Original perspective", "Small creatures stay above", "the party panel."}
	if wide {
		labels = [4]string{"Wide", "Broader perspective", "Nearby large creatures fit", "inside the view."}
	}
	return []combatSettingsText{
		{labels[0], layoutRect{r.x + 16, r.y + 16, 0, 0}, gameplayInk.heading},
		{labels[1], layoutRect{r.x + 16, r.y + 48, 0, 0}, gameplayInk.text},
		{labels[2], layoutRect{r.x + 16, r.y + 76, 0, 0}, gameplayInk.muted},
		{labels[3], layoutRect{r.x + 16, r.y + 96, 0, 0}, gameplayInk.muted},
	}
}

func combatOverlayChoiceRect(px, py, panelW int, enabled bool) layoutRect {
	x := px + panelW - 204
	if enabled {
		x += 80
	}
	return layoutRect{x, py + 304, 72, 32}
}

func (ui *UISystem) drawCombatSettings(screen *ebiten.Image, px, py, panelW int) {
	g := ui.game
	// A thin section edge follows keyboard focus without competing with the
	// selected option's bronze frame and filled radio mark.
	for row, box := range []layoutRect{
		{px + 28, py + 104, panelW - 56, 184},
		{px + 28, py + 296, panelW - 56, 136},
	} {
		edge := gameplayInk.edge
		if g.audioSettingsSelection == row {
			edge = gameplayInk.focus
		}
		drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, edge)
	}
	for _, text := range combatSettingsTextLayout(px, py, panelW) {
		drawUITextColored(screen, text.label, text.box.x, text.box.y, text.tint)
	}
	mx, my := uiCursorPosition()
	for _, wide := range []bool{false, true} {
		r := viewChoiceRect(px, py, panelW, wide)
		chosen := g.combatPreferences.WideView == wide
		hover := ui.audioSettingsOwnsInput() && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, chosen || hover)
		if chosen {
			drawFilledRect(screen, r.x+4, r.y+4, r.w-8, r.h-8, gameplayInk.selected)
		}
		for _, text := range viewChoiceTextLayout(r, wide) {
			drawUITextColored(screen, text.label, text.box.x, text.box.y, text.tint)
		}
		cx, cy := float32(r.right()-24), float32(r.y+24)
		ink := gameplayInk.edge
		if chosen || hover {
			ink = gameplayInk.focus
		}
		uiStrokeCircle(screen, cx, cy, 6, 1, ink, true)
		if chosen {
			uiFillCircle(screen, cx, cy, 3, gameplayInk.heading, true)
		}
		ui.settingsClick(r, func() {
			g.audioSettingsSelection = gameplayRowView
			g.setWideView(wide)
		})
	}
	for _, enabled := range []bool{false, true} {
		label := "Off"
		if enabled {
			label = "On"
		}
		r := combatOverlayChoiceRect(px, py, panelW, enabled)
		ui.drawSettingsChoice(screen, label, r, g.combatPreferences.TurnBasedOverlay == enabled)
		ui.settingsClick(r, func() {
			g.audioSettingsSelection = gameplayRowAurora
			g.setCombatOverlay(enabled)
		})
	}
	// The legend uses floor-grid and perimeter glyphs, matching what each color
	// means in play instead of suggesting that every threatened tile is filled.
	for i, rgb := range [][3]int{combatAuraBlue, combatAuraRed, combatAuraGreen} {
		x, y := px+44+i*((panelW-88)/3), py+386
		ink := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		drawRectBorder(screen, x, y, 24, 24, 1, ink)
		if i == 0 {
			drawFilledRect(screen, x+8, y, 1, 24, ink)
			drawFilledRect(screen, x+16, y, 1, 24, ink)
			drawFilledRect(screen, x, y+8, 24, 1, ink)
			drawFilledRect(screen, x, y+16, 24, 1, ink)
		} else {
			drawFilledRect(screen, x+11, y+11, 2, 2, gameplayInk.muted)
		}
	}
}
