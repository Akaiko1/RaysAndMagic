package game

import (
	"fmt"
	"image/color"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
)

const campHUDSprite = "icon_camp"

type gameplayActionBarLayout struct {
	bounds, quick, camp, auto layoutRect
	quick2                    layoutRect // the second quick row, above quick; zero without it
	hasQuick, hasAuto         bool
}

// One layout owns the whole action rail: the party's AUTO button while any
// hero has stat points, the always-available camp button, and the selected
// hero's quick items. Other HUD blocks reserve bounds, while each control
// draws and consumes clicks in its own rectangle.
func inGameActionBarLayout(g *MMGame) (gameplayActionBarLayout, bool) {
	if g == nil || g.config == nil || g.party == nil || len(g.party.Members) == 0 || g.menuOpen {
		return gameplayActionBarLayout{}, false
	}
	l := gameplayActionBarLayout{}
	secondRow := false
	if g.selectedChar >= 0 && g.selectedChar < len(g.party.Members) {
		if ch := g.party.Members[g.selectedChar]; ch != nil {
			for _, it := range ch.QuickSlots {
				l.hasQuick = l.hasQuick || it != nil
			}
			secondRow = ch.QuickSecondRow
		}
	}
	for _, m := range g.party.Members {
		l.hasAuto = l.hasAuto || m != nil && m.FreeStatPoints > 0
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
		if secondRow {
			l.quick2 = layoutRect{l.quick.x, l.quick.y - quickSlotRowGap - barH, barW, barH}
		}
		right = l.quick.x - gap
	}
	l.camp = layoutRect{right - campSize, bottom - campSize, campSize, campSize}
	l.bounds = l.camp
	if l.hasAuto {
		l.auto = layoutRect{l.camp.x - gap - campSize, l.camp.y, campSize, campSize}
		l.bounds.x = l.auto.x
	}
	end := l.camp.right()
	if l.hasQuick {
		end = l.quick.right()
	}
	l.bounds.w = end - l.bounds.x
	if l.quick2.w > 0 {
		l.bounds = l.bounds.union(l.quick2)
	}
	return l, true
}

// hudIconWhite keeps an icon's alpha silhouette and paints it white.
var hudIconWhite = func() colorm.ColorM {
	var cm colorm.ColorM
	cm.Scale(0, 0, 0, 1)
	cm.Translate(1, 1, 1, 0)
	return cm
}()

// drawHUDIconHoverGlow rims an action-rail icon in white while preserving its
// alpha silhouette.
func (ui *UISystem) drawHUDIconHoverGlow(screen *ebiten.Image, key string, r layoutRect) {
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleAlpha(0.16)
	white := ui.hudIconSilhouettes.of(key, ui.game.sprites.GetSprite(key))
	uiDrawImageScaledEdgeGlow(screen, white, float64(r.x), float64(r.y), float64(r.w), float64(r.h), 2, op)
}

// drawAutoStatsHUD is the party's AUTO button beside the camp: every hero
// spends their stat points by the build chosen for their class.
func (ui *UISystem) drawAutoStatsHUD(screen *ebiten.Image) {
	g := ui.game
	layout, visible := inGameActionBarLayout(g)
	if !visible || !layout.hasAuto {
		return
	}
	r := layout.auto
	mouseX, mouseY := pointerPosition()
	hover := !ui.hudClicksBlocked() && isMouseHoveringBox(mouseX, mouseY, r.x, r.y, r.right(), r.bottom())
	// The badge art is already a framed tile: it is the button, drawn at a
	// whole-pixel scale like the camp beside it.
	sprite := g.sprites.GetSprite(statBadgeStyle.icon)
	side := r.w
	if art := sprite.Bounds().Dx(); art > 0 && side >= art {
		side = side / art * art
	}
	icon := layoutRect{r.x + (r.w-side)/2, r.bottom() - side, side, side}
	drawPartyProgressionBadgeAura(screen, icon.x, icon.y, icon.w, icon.h, statBadgeStyle.tint, g.badgeAuraPulse(ui.cardAnimClock()))
	if hover {
		ui.drawHUDIconHoverGlow(screen, statBadgeStyle.icon, icon)
	}
	drawImageScaled(screen, sprite, icon.x, icon.y, icon.w, icon.h)
	drawCenteredTextWithShadow(screen, "AUTO", icon.x, icon.bottom()-uiTextCharHeight-4, icon.w, uiTextCharHeight+1, statBadgeStyle.tint)
	if hover {
		points := 0
		for _, m := range g.party.Members {
			if m != nil {
				points += m.FreeStatPoints
			}
		}
		ui.queueTooltip([]string{"Auto-assign party stats", fmt.Sprintf("Spend %d points by each hero's build", points), "Builds: Settings > Gameplay"}, mouseX+12, mouseY+8)
	}
	ui.onGatedClick(r, func() bool { return !ui.hudClicksBlocked() }, func() { g.autoDistributePartyStatPoints() })
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
		ui.drawHUDIconHoverGlow(screen, campHUDSprite, r)
	}
	drawImageScaled(screen, sprite, r.x, r.y, r.w, r.h)
	// Keep the number directly on the artwork; only the glyph shadow supplies
	// contrast against the world behind the transparent tent icon.
	text := strconv.Itoa(max(0, g.party.Food))
	if uiTextWidth(text) > r.w-8 {
		text = "9999+"
	}
	w := uiTextWidth(text) + 6
	badge := layoutRect{r.right() - w - 3, r.bottom() - uiTextCharHeight - 4, w, uiTextCharHeight + 1}
	ink := color.RGBA{245, 220, 157, 255}
	if g.party.Food < CampFoodCost {
		ink = color.RGBA{236, 111, 95, 255}
	}
	drawCenteredTextWithShadow(screen, text, badge.x, badge.y, badge.w, badge.h, ink)
	if hover {
		lines, colors := g.campStatusCardRows()
		ui.queueCardTooltip(lines, colors, woodPlateColor, nil, campHUDSprite, mouseX+12, mouseY+8)
	}
	ui.onGatedClick(r, func() bool { return !ui.hudClicksBlocked() }, func() { g.campConfirmOpen = true })
}

type campConfirmationLayout struct {
	panel, title, icon, prompt, food, yes, no, close layoutRect
}

func layoutCampConfirmation(screenW, screenH int) campConfirmationLayout {
	const w, h = 420, 260
	panel := centeredRect(screenW, screenH, w, h)
	x, y := panel.x, panel.y
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
	w, h := uiBounds(screen).Dx(), uiBounds(screen).Dy()
	l := layoutCampConfirmation(w, h)
	interactive := ui.topModalLayer() == modalLayerCamp
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 140})
	ui.drawThemeFrame(screen, frameGold, l.panel.x, l.panel.y, l.panel.w, l.panel.h)
	drawCenteredTextWithShadow(screen, uitext.Text("ui.camp"), l.title.x, l.title.y, l.title.w, l.title.h, rarityGold)
	drawImageScaled(screen, g.sprites.GetSprite(campHUDSprite), l.icon.x, l.icon.y, l.icon.w, l.icon.h)
	drawCenteredUIText(screen, uitext.Text("ui.camp_confirm", CampFoodCost), l.prompt.x, l.prompt.y, l.prompt.w, l.prompt.h)
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
		ui.onClick(r, interactive, func() {
			g.resolveCampConfirmation(button.confirm)
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
		hurt := g.watcherPartyHurt() // judged as they lie down: the rest heals it
		message, rested := g.TryCamp()
		g.AddCombatMessage(message)
		if rested {
			g.beginCampRest(hurt)
			// One cue per accepted rest, using the actual healing spell's
			// YAML school routing; never once per party member or draw frame.
			if heal, err := spells.GetSpellDefinitionByID(spells.SpellID(items.SpellEffectHealOther)); err == nil {
				g.playSpellSound(heal)
			}
		}
	}
}
