package game

import (
	"image/color"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

const campHUDSprite = "icon_camp"

type gameplayActionBarLayout struct {
	bounds, quick, camp layoutRect
	hasQuick            bool
}

// One layout owns the whole action rail, including the always-available camp
// button when the selected hero has no quick items. Other HUD blocks reserve
// bounds, while each control draws and consumes clicks in its own rectangle.
func inGameActionBarLayout(g *MMGame) (gameplayActionBarLayout, bool) {
	if g == nil || g.config == nil || g.party == nil || len(g.party.Members) == 0 || g.menuOpen {
		return gameplayActionBarLayout{}, false
	}
	l := gameplayActionBarLayout{}
	if g.selectedChar >= 0 && g.selectedChar < len(g.party.Members) {
		if ch := g.party.Members[g.selectedChar]; ch != nil {
			for _, it := range ch.QuickSlots {
				l.hasQuick = l.hasQuick || it != nil
			}
		}
	}
	pw, _, left, partyTop := partyPortraitLayout(g)
	barW := min(pw*2, 240)
	barH := int(float64(barW) / quickSlotBarAspect)
	const gap = 8
	campSize := max(48, barH)
	right := left + pw*4
	bottom := max(campSize, partyTop-18)
	if l.hasQuick {
		l.quick = layoutRect{right - barW, bottom - barH, barW, barH}
		right = l.quick.x - gap
	}
	l.camp = layoutRect{right - campSize, bottom - campSize, campSize, campSize}
	l.bounds = l.camp
	if l.hasQuick {
		l.bounds.w = l.quick.right() - l.camp.x
	}
	return l, true
}

func (ui *UISystem) drawCampHUD(screen *ebiten.Image) {
	g := ui.game
	layout, visible := inGameActionBarLayout(g)
	if !visible {
		return
	}
	r := layout.camp
	mouseX, mouseY := pointerPosition()
	hover := !ui.hudClicksBlocked() && isMouseHoveringBox(mouseX, mouseY, r.x, r.y, r.right(), r.bottom())
	sprite := g.sprites.GetSprite(campHUDSprite)
	if hover {
		op := &ebiten.DrawImageOptions{}
		// Replace RGB with white while preserving the tent's alpha silhouette.
		op.ColorM.Scale(0, 0, 0, 1)
		op.ColorM.Translate(1, 1, 1, 0)
		op.ColorScale.ScaleAlpha(0.16)
		graphics.DrawImageScaledEdgeGlow(screen, sprite, float64(r.x), float64(r.y), float64(r.w), float64(r.h), 2, op)
	}
	drawImageScaled(screen, sprite, r.x, r.y, r.w, r.h)
	// A native-size number on an opaque inset badge stays readable over the
	// transparent artwork and the world behind it.
	text := strconv.Itoa(max(0, g.party.Food))
	if debugTextWidth(text) > r.w-8 {
		text = "9999+"
	}
	w := debugTextWidth(text) + 6
	badge := layoutRect{r.right() - w - 3, r.bottom() - debugTextCharHeight - 4, w, debugTextCharHeight + 1}
	drawFilledRect(screen, badge.x, badge.y, badge.w, badge.h, color.RGBA{5, 6, 8, 245})
	ink := color.RGBA{245, 220, 157, 255}
	if g.party.Food < CampFoodCost {
		ink = color.RGBA{236, 111, 95, 255}
	}
	drawCenteredTextWithShadow(screen, text, badge.x, badge.y, badge.w, badge.h, ink)
	if hover {
		ui.queueTooltipIcon([]string{uitext.Text("ui.camp"), uitext.Text("ui.camp_food", g.party.Food),
			uitext.Text("ui.camp_use", CampFoodCost), uitext.Text("ui.camp_effect")}, campHUDSprite, mouseX+12, mouseY+8)
	}
	ui.onDisplayedInput(uiCommandClick, r, func() {
		if !ui.hudClicksBlocked() && g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			g.campConfirmOpen = true
		}
	})
}

type campConfirmationLayout struct {
	panel, title, icon, prompt, food, yes, no, close layoutRect
}

func layoutCampConfirmation(screenW, screenH int) campConfirmationLayout {
	const w, h = 420, 260
	x, y := (screenW-w)/2, (screenH-h)/2
	return campConfirmationLayout{
		panel:  layoutRect{x, y, w, h},
		title:  layoutRect{x + 40, y + 14, w - 80, 22},
		icon:   layoutRect{x + (w-80)/2, y + 44, 80, 80},
		prompt: layoutRect{x + 16, y + 136, w - 32, 20},
		food:   layoutRect{x + 16, y + 162, w - 32, 20},
		yes:    layoutRect{x + 66, y + 200, 136, 40},
		no:     layoutRect{x + 218, y + 200, 136, 40},
		close:  layoutRect{x + w - 36, y + 12, 24, 24},
	}
}

func (ui *UISystem) drawCampConfirmation(screen *ebiten.Image) {
	g := ui.game
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	l := layoutCampConfirmation(w, h)
	interactive := ui.topModalLayer() == modalLayerCamp
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 140})
	ui.drawThemeFrame(screen, frameGold, l.panel.x, l.panel.y, l.panel.w, l.panel.h)
	drawCenteredTextWithShadow(screen, uitext.Text("ui.camp"), l.title.x, l.title.y, l.title.w, l.title.h, rarityGold)
	drawImageScaled(screen, g.sprites.GetSprite(campHUDSprite), l.icon.x, l.icon.y, l.icon.w, l.icon.h)
	drawCenteredDebugText(screen, uitext.Text("ui.camp_confirm", CampFoodCost), l.prompt.x, l.prompt.y, l.prompt.w, l.prompt.h)
	drawCenteredTextWithShadow(screen, uitext.Text("ui.camp_food", g.party.Food), l.food.x, l.food.y, l.food.w, l.food.h, color.RGBA{193, 178, 148, 255})
	mouseX, mouseY := pointerPosition()
	for _, button := range []struct {
		r       layoutRect
		label   string
		confirm bool
	}{{l.yes, uitext.Text("ui.yes"), true}, {l.no, uitext.Text("ui.no"), false}} {
		r := button.r
		hover := interactive && isMouseHoveringBox(mouseX, mouseY, r.x, r.y, r.right(), r.bottom())
		ui.drawMenuButton(screen, button.label, r.x, r.y, r.w, r.h, hover)
		ui.onDisplayedInput(uiCommandClick, r, func() {
			if interactive && g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
				g.resolveCampConfirmation(button.confirm)
			}
		})
	}
	ui.drawPopupCloseButton(screen, l.close.x, l.close.y, l.close.w, interactive, func() {
		g.resolveCampConfirmation(false)
	})
}

func (g *MMGame) resolveCampConfirmation(confirmed bool) {
	if !g.campConfirmOpen {
		return
	}
	g.campConfirmOpen = false
	if confirmed {
		message, rested := g.TryCamp()
		g.AddCombatMessage(message)
		if rested {
			g.beginCampRest()
			// One cue per accepted rest, using the actual healing spell's
			// YAML school routing; never once per party member or draw frame.
			if heal, err := spells.GetSpellDefinitionByID(spells.SpellID(items.SpellEffectHealOther)); err == nil {
				g.playSpellSound(heal)
			}
		}
	}
}
