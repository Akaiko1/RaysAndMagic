package game

import (
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Display tab reuses the Sound tab's rows: a framed box with a label, and
// across the slider track one button per interface size preset (row 0) or
// the font list (row 1), followed by the 3D props toggle (row 2).

const (
	displayRowSize = iota
	displayRowFont
	displayRowProps
	displayRows
)

// displayFontItemH is one entry of the open font list.
const displayFontItemH = 26
const displayRowPitch = 76

func displaySliderRect(px, py, panelW, row int) pagerRect {
	r := audioSliderRect(px, py, panelW, row)
	dy := row * (audioSettingsRowPitch - displayRowPitch)
	r.y1 -= dy
	r.y2 -= dy
	return r
}

func displaySelectionRect(px, py, panelW, inset, row int) pagerRect {
	r := displaySliderRect(px, py, panelW, row)
	return pagerRect{px + inset, r.y1 - 24, px + panelW - inset, r.y2 + 24}
}

func displaySizeButtonRect(px, py, panelW, index, count int) layoutRect {
	track := displaySliderRect(px, py, panelW, displayRowSize)
	const gap = 8
	w := (track.x2 - track.x1 - gap*(count-1)) / max(1, count)
	return layoutRect{track.x1 + index*(w+gap), track.y1 - 6, w, track.y2 - track.y1 + 12}
}

// displayFontFieldRect is the closed font list: the chosen font's name.
func displayFontFieldRect(px, py, panelW int) layoutRect {
	track := displaySliderRect(px, py, panelW, displayRowFont)
	return layoutRect{track.x1, track.y1 - 6, track.x2 - track.x1, track.y2 - track.y1 + 12}
}

// displayFontItemRect is entry i of the open list, just under the field.
func displayFontItemRect(px, py, panelW, i int) layoutRect {
	field := displayFontFieldRect(px, py, panelW)
	return layoutRect{field.x, field.bottom() + 2 + i*displayFontItemH, field.w, displayFontItemH}
}

func displayPropsToggleRect(px, py, panelW int) layoutRect {
	r := displaySliderRect(px, py, panelW, displayRowProps)
	return layoutRect{r.x2 - 104, r.y1 - 10, 104, 40}
}

func (ui *UISystem) drawDisplaySettings(screen *ebiten.Image, px, py, panelW, contentInset int) {
	g := ui.game
	drawCenteredUIText(screen, "Interface and world appearance", px+32, py+100, panelW-64, 18)
	for row := 0; row < displayRows; row++ {
		box := displaySelectionRect(px, py, panelW, contentInset, row)
		border := color.RGBA{68, 65, 55, 255}
		if row == g.audioSettingsSelection {
			border = color.RGBA{177, 149, 89, 255}
		}
		drawFilledRect(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, color.RGBA{24, 27, 30, 255})
		drawRectBorder(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, 1, border)
	}
	box := displaySelectionRect(px, py, panelW, contentInset, displayRowSize)
	r := displaySliderRect(px, py, panelW, displayRowSize)

	sizes := g.config.Display.InterfaceSizes
	active := g.interfaceSizeKey()
	activeIndex := 0
	for i, size := range sizes {
		if size.Key == active {
			activeIndex = i
		}
	}
	// Growing squares echo the Sound tab's volume bars.
	for i := range sizes {
		side := 6 + i*4
		ink := color.RGBA{65, 61, 50, 255}
		if i <= activeIndex {
			ink = color.RGBA{209, 174, 93, 255}
		}
		drawFilledRect(screen, box.x1+12+i*11, box.y1+46-side, side-2, side, ink)
	}
	drawUITextColored(screen, "Interface size", box.x1+50, r.y1-6, color.RGBA{230, 213, 172, 255})
	drawUITextColored(screen, "Text and panels", box.x1+50, r.y1+14, color.RGBA{156, 158, 158, 255})
	for i, size := range sizes {
		b := displaySizeButtonRect(px, py, panelW, i, len(sizes))
		chosen := size.Key == active
		ui.drawSettingsChoice(screen, size.Label, b, chosen)
		if !g.interfaceSizeOffered(i) && !chosen {
			// Nothing larger fits this screen: dim the button and ignore it.
			drawFilledRect(screen, b.x, b.y, b.w, b.h, color.RGBA{12, 13, 15, 170})
			continue
		}
		key := size.Key
		ui.settingsClick(b, func() {
			g.audioSettingsSelection = displayRowSize
			g.fontListOpen = false
			g.setInterfaceSize(key)
		})
	}

	fontBox := displaySelectionRect(px, py, panelW, contentInset, displayRowFont)
	fr := displaySliderRect(px, py, panelW, displayRowFont)
	drawUITextColored(screen, "Aa", fontBox.x1+16, fr.y1+4, color.RGBA{209, 174, 93, 255})
	drawUITextColored(screen, "Font", fontBox.x1+50, fr.y1-6, color.RGBA{230, 213, 172, 255})
	drawUITextColored(screen, "Letters everywhere", fontBox.x1+50, fr.y1+14, color.RGBA{156, 158, 158, 255})
	field := displayFontFieldRect(px, py, panelW)
	fonts := g.config.Display.Fonts
	chosen := fonts[g.uiFontIndex()]
	ui.drawButtonFrame(screen, field.x, field.y, field.w, field.h, g.fontListOpen)
	drawUITextColored(screen, chosen.Label, field.x+12, field.y+(field.h-uiTextCharHeight)/2, color.RGBA{235, 221, 180, 255})
	drawUITextColored(screen, "v", field.right()-12-uiTextWidth("v"), field.y+(field.h-uiTextCharHeight)/2, color.RGBA{177, 149, 89, 255})
	ui.settingsClick(field, func() {
		g.audioSettingsSelection = displayRowFont
		g.fontListOpen = !g.fontListOpen
	})

	propBox := displaySelectionRect(px, py, panelW, contentInset, displayRowProps)
	pr := displaySliderRect(px, py, panelW, displayRowProps)
	drawUITextColored(screen, "3D", propBox.x1+12, pr.y1+4, gameplayInk.focus)
	drawUITextColored(screen, "3D props", propBox.x1+50, pr.y1-6, gameplayInk.heading)
	drawUITextColored(screen, "Off restores original sprites", propBox.x1+50, pr.y1+14, gameplayInk.muted)
	toggle := displayPropsToggleRect(px, py, panelW)
	enabled := g.props3DEnabled()
	mx, my := uiCursorPosition()
	hover := ui.audioSettingsOwnsInput() && isMouseHoveringBox(mx, my, toggle.x, toggle.y, toggle.right(), toggle.bottom())
	ui.drawButtonFrame(screen, toggle.x, toggle.y, toggle.w, toggle.h, enabled || hover)
	ink := gameplayInk.muted
	label := "OFF"
	if enabled {
		ink, label = gameplayInk.heading, "ON"
	}
	cx, cy := toggle.x+14, toggle.y+11
	drawRectBorder(screen, cx, cy, 18, 18, 1, ink)
	if enabled {
		uiStrokeLine(screen, float32(cx+4), float32(cy+9), float32(cx+8), float32(cy+13), 2, ink, true)
		uiStrokeLine(screen, float32(cx+8), float32(cy+13), float32(cx+15), float32(cy+5), 2, ink, true)
	}
	drawUITextColored(screen, label, toggle.x+46, toggle.y+(toggle.h-uiTextCharHeight)/2, ink)
	ui.settingsClick(toggle, func() {
		g.audioSettingsSelection = displayRowProps
		g.fontListOpen = false
		g.setProps3D(!g.props3DEnabled())
	})

	if g.fontListOpen {
		ui.drawFontList(screen, px, py, panelW)
	}
}

// drawFontList draws the open list over the tab, every name in its own font.
// A click on an entry chooses it; a click anywhere else only closes the list.
func (ui *UISystem) drawFontList(screen *ebiten.Image, px, py, panelW int) {
	// Popup hits precede every underlying control, including the header. Its
	// final outside-click handler consumes the gesture without click-through.
	firstCommand := len(ui.displayedInput.commands)
	defer func() {
		if ui.displayedInput.building {
			commands := ui.displayedInput.commands
			slices.Reverse(commands[:firstCommand])
			slices.Reverse(commands[firstCommand:])
			slices.Reverse(commands)
		}
	}()
	g := ui.game
	fonts := g.config.Display.Fonts
	first, last := displayFontItemRect(px, py, panelW, 0), displayFontItemRect(px, py, panelW, len(fonts)-1)
	frame := layoutRect{first.x - 2, first.y - 2, first.w + 4, last.bottom() - first.y + 4}
	drawFilledRect(screen, frame.x, frame.y, frame.w, frame.h, color.RGBA{16, 18, 21, 250})
	drawRectBorder(screen, frame.x, frame.y, frame.w, frame.h, 1, color.RGBA{177, 149, 89, 255})
	mx, my := pointerPosition()
	for i, f := range fonts {
		item := displayFontItemRect(px, py, panelW, i)
		if i == g.uiFontIndex() {
			drawFilledRect(screen, item.x, item.y, item.w, item.h, color.RGBA{43, 39, 29, 255})
		}
		if isMouseHoveringBox(mx, my, item.x, item.y, item.right(), item.bottom()) {
			drawRectBorder(screen, item.x, item.y, item.w, item.h, 1, color.RGBA{226, 199, 132, 255})
		}
		font := uiFontAtlases[f.Key]
		if font == nil {
			font = uiGlyphs()
		}
		drawUITextColoredIn(screen, font, f.Label, item.x+12, item.y+(item.h-uiTextCharHeight)/2, color.RGBA{235, 221, 180, 255})
		key := f.Key
		ui.settingsClick(item, func() {
			g.fontListOpen = false
			g.setUIFont(key)
		})
	}
	ui.settingsClick(layoutRect{0, 0, g.config.GetScreenWidth(), g.config.GetScreenHeight()}, func() {
		g.fontListOpen = false
	})
}
