package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func settingsTabRect(px, py, panelW, index int) layoutRect {
	w := (panelW - 64 - 8) / 2
	return layoutRect{px + 32 + index*(w+8), py + 58, w, 30}
}
func (g *MMGame) switchSettingsTab(tab int) {
	if tab < 0 || tab > 1 || tab == g.settingsTab {
		return
	}
	g.saveAudioSettings()
	g.audioSliderDrag = -1
	g.audioSettingsSelection = 0
	g.settingsTab = tab
	g.mouseLeftClicks = nil
}
func (ui *UISystem) settingsClick(r layoutRect, action func()) {
	if !ui.audioSettingsOwnsInput() {
		return
	}
	ui.onDisplayedInput(uiCommandClick, r, func() {
		if ui.game.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			action()
		}
	})
}
func (ui *UISystem) drawSettingsHeader(screen *ebiten.Image, px, py, panelW int) {
	drawScaledMetalCenteredText(screen, "Settings", px+panelW/2, py+22, 2, rarityGold)
	for tab, label := range []string{"Sound", "Auto-potions"} {
		r := settingsTabRect(px, py, panelW, tab)
		ui.drawMenuButton(screen, label, r.x, r.y, r.w, r.h, ui.game.settingsTab == tab)
		if ui.game.settingsTab == tab {
			drawFilledRect(screen, r.x+4, r.bottom()-4, r.w-8, 2, color.RGBA{202, 174, 104, 255})
		}
		ui.settingsClick(r, func() { ui.game.switchSettingsTab(tab) })
	}
}

// One measured slider is shared by volume and automatic-potion thresholds.
func drawSettingsSlider(screen *ebiten.Image, r pagerRect, value float64, tint color.RGBA, selected bool) {
	value = max(0, min(1, value))
	y := (r.y1 + r.y2) / 2
	w := r.x2 - r.x1
	drawFilledRect(screen, r.x1-2, y-6, w+4, 12, color.RGBA{10, 12, 15, 255})
	drawRectBorder(screen, r.x1-2, y-6, w+4, 12, 1, color.RGBA{87, 77, 53, 255})
	for tick := 0; tick <= 4; tick++ {
		x := r.x1 + w*tick/4
		drawFilledRect(screen, x, y+9, 1, 4, color.RGBA{105, 96, 74, 255})
	}
	fill := int(math.Round(float64(w) * value))
	drawFilledRect(screen, r.x1, y-3, fill, 6, tint)
	drawFilledRect(screen, r.x1, y-3, fill, 1, color.RGBA{225, 213, 162, 210})
	thumb := layoutRect{r.x1 + fill - audioSliderThumbW/2, y - audioSliderThumbH/2, audioSliderThumbW, audioSliderThumbH}
	drawFilledRect(screen, thumb.x+1, thumb.y+2, thumb.w, thumb.h, color.RGBA{0, 0, 0, 130})
	uiTint := color.RGBA{143, 125, 81, 255}
	if selected {
		uiTint = color.RGBA{226, 199, 132, 255}
	}
	drawFilledRect(screen, thumb.x, thumb.y, thumb.w, thumb.h, uiTint)
	drawRectBorder(screen, thumb.x, thumb.y, thumb.w, thumb.h, 1, color.RGBA{58, 48, 32, 255})
	drawRectBorder(screen, thumb.x+2, thumb.y+2, thumb.w-4, thumb.h-4, 1, color.RGBA{248, 231, 177, 150})
	for dx := -2; dx <= 2; dx += 2 {
		drawFilledRect(screen, thumb.x+thumb.w/2+dx, thumb.y+6, 1, thumb.h-12, color.RGBA{94, 77, 43, 255})
	}
}

type potionSettingsLayout struct {
	panel, types layoutRect
	slider       pagerRect
	cardW        int
}

func makePotionSettingsLayout(px, py, panelW, row int) potionSettingsLayout {
	p := layoutRect{px + 28, py + 120 + row*156, panelW - 56, 144}
	types := layoutRect{p.x + 244, p.y + 12, p.w - 258, 120}
	return potionSettingsLayout{p, types, pagerRect{p.x + 20, p.y + 78, p.x + 214, p.y + 98}, (types.w - 6) / 2}
}
func (l potionSettingsLayout) card(index, offset int) layoutRect {
	return layoutRect{l.types.x + (index%2)*(l.cardW+6), l.types.y + (index/2)*62 - offset, l.cardW, 56}
}
func (g *MMGame) updatePotionSettingsPointer(px, py, panelW int) {
	mx, my := pointerPosition()
	if pointerLeftJustPressed() {
		g.audioSliderDrag = -1
		for row := 0; row < 2; row++ {
			r := makePotionSettingsLayout(px, py, panelW, row).slider
			if isMouseHoveringBox(mx, my, r.x1-8, r.y1, r.x2+8, r.y2) {
				g.beginAudioSliderDrag(row)
				break
			}
		}
	}
	if g.audioSliderDrag >= 0 && g.audioSliderDrag < 2 && pointerLeftPressed() {
		r := makePotionSettingsLayout(px, py, panelW, g.audioSliderDrag).slider
		value := int(math.Round(100 * float64(mx-r.x1) / float64(r.x2-r.x1)))
		g.setPotionThreshold(g.audioSliderDrag == 1, value)
	}
	if pointerLeftJustRelease() && g.audioSliderDrag >= 0 {
		g.savePotionPreferences()
		g.audioSliderDrag = -1
	}
}
func (ui *UISystem) drawPotionSettings(screen *ebiten.Image, px, py, panelW int) {
	g := ui.game
	prefs := g.config.EnsurePotionPreferences()
	drawCenteredDebugText(screen, "Potion types include brewed versions. Manual use remains available.", px+28, py+96, panelW-56, 18)
	for row := 0; row < 2; row++ {
		mana := row == 1
		p := prefs.Resource(mana)
		l := makePotionSettingsLayout(px, py, panelW, row)
		tint := color.RGBA{164, 65, 63, 255}
		title, resource, icon := "Health", "HP", "health_potion"
		if mana {
			tint = color.RGBA{65, 119, 191, 255}
			title, resource, icon = "Mana", "SP", "mana_potion"
		}
		drawFilledRect(screen, l.panel.x, l.panel.y, l.panel.w, l.panel.h, color.RGBA{22, 27, 29, 255})
		drawRectBorder(screen, l.panel.x, l.panel.y, l.panel.w, l.panel.h, 1, color.RGBA{89, 81, 61, 255})
		drawFilledRect(screen, l.panel.x, l.panel.y, 3, l.panel.h, tint)
		it, _ := items.TryCreateItemFromYAML(icon)
		ui.drawInventoryItemIcon(screen, it, l.panel.x+14, l.panel.y+12, 36, 36, 1, true)
		drawDebugTextColored(screen, title, l.panel.x+60, l.panel.y+12, color.RGBA{226, 214, 181, 255})
		label := fmt.Sprintf("Below %d%% %s", p.ThresholdPct, resource)
		if p.ThresholdPct == 0 {
			label = "Automatic use off"
		}
		drawDebugTextColored(screen, label, l.panel.x+60, l.panel.y+32, color.RGBA{180, 183, 179, 255})
		drawSettingsSlider(screen, l.slider, float64(p.ThresholdPct)/100, tint, g.audioSettingsSelection == row)
		drawDebugTextColored(screen, "Off", l.slider.x1-6, l.slider.y2+8, color.RGBA{134, 139, 140, 255})
		drawDebugTextColored(screen, "100%", l.slider.x2-22, l.slider.y2+8, color.RGBA{134, 139, 140, 255})
		choices := config.AutomaticPotionChoices(mana)
		total := max(0, ((len(choices)+1)/2)*62-6)
		limit := max(0, total-l.types.h)
		offset := max(0, min(g.potionSettingsScroll[row], limit))
		g.potionSettingsScroll[row] = offset
		if ui.audioSettingsOwnsInput() {
			ui.onDisplayedInput(uiCommandPointer, l.types, func() {
				mx, my := pointerPosition()
				_, wheel := pointerWheel()
				if isMouseHoveringBox(mx, my, l.types.x, l.types.y, l.types.right(), l.types.bottom()) && wheel != 0 {
					g.potionSettingsScroll[row] = max(0, min(g.potionSettingsScroll[row]-int(math.Copysign(max(1, math.Abs(wheel*36)), wheel)), limit))
				}
			})
		}
		clip := image.Rect(l.types.x, l.types.y, l.types.right(), l.types.bottom()).Intersect(screen.Bounds())
		if clip.Empty() {
			continue
		}
		dst := screen.SubImage(clip).(*ebiten.Image)
		for i, choice := range choices {
			r := l.card(i, offset)
			hit := image.Rect(r.x, r.y, r.right(), r.bottom()).Intersect(clip)
			if hit.Empty() {
				continue
			}
			on := p.Allowed[choice.Key]
			border := color.RGBA{72, 78, 77, 255}
			background := color.RGBA{29, 34, 35, 255}
			if on {
				border = color.RGBA{185, 161, 92, 255}
				background = color.RGBA{43, 48, 38, 255}
			}
			drawFilledRect(dst, r.x, r.y, r.w, r.h, background)
			drawRectBorder(dst, r.x, r.y, r.w, r.h, 1, border)
			item, _ := items.TryCreateItemFromYAML(choice.Key)
			ui.drawInventoryItemIcon(dst, item, r.x+5, r.y+10, 34, 34, 1, true)
			ui.rareBookText(dst, choice.Definition.Name, layoutRect{r.x + 44, r.y + 8, r.w - 65, 32})
			mark := layoutRect{r.right() - 17, r.y + 6, 11, 11}
			drawRectBorder(dst, mark.x, mark.y, mark.w, mark.h, 1, border)
			if on {
				ink := color.RGBA{233, 215, 141, 255}
				vector.StrokeLine(dst, float32(mark.x+2), float32(mark.y+5), float32(mark.x+4), float32(mark.y+8), 1.5, ink, true)
				vector.StrokeLine(dst, float32(mark.x+4), float32(mark.y+8), float32(mark.x+9), float32(mark.y+2), 1.5, ink, true)
			}
			caption := "Excluded"
			ink := color.RGBA{138, 141, 140, 255}
			if on {
				caption = "Allowed"
				ink = color.RGBA{162, 192, 134, 255}
			}
			drawDebugTextColored(dst, caption, r.x+44, r.y+38, ink)
			ui.settingsClick(layoutRect{hit.Min.X, hit.Min.Y, hit.Dx(), hit.Dy()}, func() { g.toggleAutomaticPotion(mana, choice.Key) })
			mx, my := pointerPosition()
			if image.Pt(mx, my).In(hit) {
				lines := strings.Split(GetItemTooltip(item, nil, g.combat, tooltipDetailHeld()), "\n")
				ui.queueItemTooltip(lines, item, nil, mx+16, my+8)
			}
		}
		drawProfileScrollbar(screen, layoutRect{l.types.right() + 3, l.types.y, 3, l.types.h}, offset, total)
	}
}
