package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Display tab reuses the Sound tab's rows: a framed box with a label, and
// across the slider track one button per interface size preset (row 0) or
// the font list (row 1).

const (
	displayRowSize = iota
	displayRowFont
	displayRows
)

// displayFontItemH is one entry of the open font list.
const displayFontItemH = 26

func displaySizeButtonRect(px, py, panelW, index, count int) layoutRect {
	track := audioSliderRect(px, py, panelW, displayRowSize)
	const gap = 8
	w := (track.x2 - track.x1 - gap*(count-1)) / max(1, count)
	return layoutRect{track.x1 + index*(w+gap), track.y1 - 6, w, track.y2 - track.y1 + 12}
}

// displayFontFieldRect is the closed font list: the chosen font's name.
func displayFontFieldRect(px, py, panelW int) layoutRect {
	track := audioSliderRect(px, py, panelW, displayRowFont)
	return layoutRect{track.x1, track.y1 - 6, track.x2 - track.x1, track.y2 - track.y1 + 12}
}

// displayFontItemRect is entry i of the open list, just under the field.
func displayFontItemRect(px, py, panelW, i int) layoutRect {
	field := displayFontFieldRect(px, py, panelW)
	return layoutRect{field.x, field.bottom() + 2 + i*displayFontItemH, field.w, displayFontItemH}
}

func displayInfoY(py int) int {
	return py + audioSettingsRowTop + displayRowFont*audioSettingsRowPitch + 56
}

func (ui *UISystem) drawDisplaySettings(screen *ebiten.Image, px, py, panelW, contentInset int) {
	g := ui.game
	drawCenteredUIText(screen, "Make the text and panels larger", px+32, py+100, panelW-64, 18)
	for row := 0; row < displayRows; row++ {
		box := audioSelectionRect(px, py, panelW, contentInset, row)
		border := color.RGBA{68, 65, 55, 255}
		if row == g.audioSettingsSelection {
			border = color.RGBA{177, 149, 89, 255}
		}
		drawFilledRect(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, color.RGBA{24, 27, 30, 255})
		drawRectBorder(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, 1, border)
	}
	box := audioSelectionRect(px, py, panelW, contentInset, displayRowSize)
	r := audioSliderRect(px, py, panelW, displayRowSize)

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

	fontBox := audioSelectionRect(px, py, panelW, contentInset, displayRowFont)
	fr := audioSliderRect(px, py, panelW, displayRowFont)
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

	y := displayInfoY(py)
	for i, line := range displaySizeStatus(g) {
		drawCenteredUIText(screen, line, px+32, y+i*22, panelW-64, 18)
	}
	credit := color.RGBA{138, 134, 120, 255}
	for i, line := range wrapUIText(displayFontCredits(g), panelW-64) {
		w := uiTextWidth(line)
		drawUITextColored(screen, line, px+(panelW-w)/2, y+4*22+i*18, credit)
	}
	if g.fontListOpen {
		ui.drawFontList(screen, px, py, panelW)
	}
}

// drawFontList draws the open list over the tab, every name in its own font.
// A click on an entry chooses it; a click anywhere else only closes the list.
func (ui *UISystem) drawFontList(screen *ebiten.Image, px, py, panelW int) {
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

// displayFontCredits names the authors of every shipped font.
func displayFontCredits(g *MMGame) string {
	var credits []string
	for _, f := range g.config.Display.Fonts {
		if f.Credit != "" {
			credits = append(credits, f.Credit)
		}
	}
	if len(credits) == 0 {
		return ""
	}
	return "Fonts: " + strings.Join(credits, ", ")
}

// displaySizeStatus explains what the active preset does on this screen.
func displaySizeStatus(g *MMGame) []string {
	lines := []string{fmt.Sprintf("Interface %d x %d, 3D view %d x %d",
		g.config.GetScreenWidth(), g.config.GetScreenHeight(), g.worldWidth(), g.worldHeight())}
	i := g.interfaceSizeIndex()
	if i >= len(g.interfaceFrames) {
		return lines
	}
	frame := g.interfaceFrames[i]
	switch {
	case frame.sharp:
		lines = append(lines, fmt.Sprintf("Sharp: text pixels scale x%d", int(math.Round(frame.scale))))
	default:
		lines = append(lines, "Text slightly soft: this screen has no sharp step here")
	}
	if frame.same {
		lines = append(lines, "This screen has no larger size than the one before")
	} else if i+1 < len(g.interfaceFrames) && g.interfaceFrames[i+1].same && i > 0 {
		lines = append(lines, "The largest size this screen allows")
	}
	return lines
}
