package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

func spellAutocastRect(x, y, w, iconSize int) layoutRect {
	width := min(w-8, uiTextWidth(uitext.Text("advanced.auto_label"))+24)
	margin := max(4, (w-width-uiTextWidth("SP 99")-4)/2)
	return layoutRect{x + w - margin - width, y + iconSize + 8 + uiTextCharHeight + 2, width, uiTextCharHeight}
}

// Keep the toggle beside the SP cost, outside the spell art and name.
// Displayed-input registration precedes the whole-card click/drag handlers.
func (ui *UISystem) spellAutocastControl(screen *ebiten.Image, c *character.MMCharacter, d spells.SpellDefinition, x, y, w, iconSize int) {
	if !d.HasAutocast() {
		return
	}
	box := spellAutocastRect(x, y, w, iconSize)
	on := c.SpellState.Automatic[string(d.ID)]
	mx, my := uiCursorPosition()
	hover := !ui.modalLayerOwnsInput() && isMouseHoveringBox(mx, my, box.x, box.y, box.right(), box.bottom())
	ui.drawButtonFrame(screen, box.x, box.y, box.w, box.h, hover)
	ink := color.RGBA{164, 149, 123, 255}
	if on || hover {
		ink = color.RGBA{255, 232, 175, 255}
	}
	drawCenteredTextWithShadow(screen, uitext.Text("advanced.auto_label"), box.x+3, box.y, box.w-21, box.h, ink)
	// A tiny sliding brass seal is part of one labelled hit target, not a
	// second checkbox. The lit school-colour stone makes enabled state clear.
	cx, cy := float32(box.right()-11), float32(box.y)+float32(box.h)/2
	uiStrokeLine(screen, cx-4, cy, cx+4, cy, 7, color.RGBA{6, 8, 10, 255}, true)
	knobX := cx - 3
	var knob color.Color = color.RGBA{122, 111, 94, 255}
	if on {
		knobX = cx + 3
		knob = SchoolColor(d.School)
	}
	uiFillCircle(screen, knobX, cy, 3, knob, true)
	uiFillCircle(screen, knobX-.6, cy-.7, .8, color.RGBA{255, 235, 188, 230}, true)
	ui.rareBookClick(box, true, func() {
		if c.SpellState.Automatic == nil {
			c.SpellState.Automatic = make(map[string]bool)
		}
		c.SpellState.Automatic[string(d.ID)] = !c.SpellState.Automatic[string(d.ID)]
	})
	if hover {
		state := uitext.Text("advanced.autocast_off")
		if on {
			state = uitext.Text("advanced.autocast_on")
		}
		var trigger string
		switch d.Mechanic.Autocast {
		case "dispel_magic":
			trigger = uitext.Text("advanced.auto_dispel_magic", d.Mechanic.RadiusTiles)
		case "quickening":
			trigger = uitext.Text("advanced.auto_quickening")
		case "purify":
			trigger = uitext.Text("advanced.auto_purify")
		case "celestial_armor":
			trigger = uitext.Text("advanced.auto_celestial_armor")
		}
		ui.queueTooltip([]string{d.Name, state, trigger, ui.game.automaticCastCostText()}, mx+12, my+8)
	}
}

const (
	personalSpellBadgeIconSize = 12
	personalSpellBadgeCountW   = 6
	personalSpellBadgeWidth    = 18
	personalSpellBadgeHeight   = personalSpellBadgeIconSize
)

// Personal effects and Flowing Staff share the original portrait gutter.
// Counts fit an eight-pixel seal with any selected font.
func personalSpellBadgeRect(px, py, pw, row int) layoutRect {
	return layoutRect{px + pw, py - 8 + row*(personalSpellBadgeHeight+1), personalSpellBadgeWidth, personalSpellBadgeHeight}
}

func (ui *UISystem) drawPersonalSpellEffects(screen *ebiten.Image, c *character.MMCharacter, px, py, pw, mx, my int) {
	row := 0
	draw := func(icon, label, detail string, charges int, staff bool, accent color.Color) {
		box := personalSpellBadgeRect(px, py, pw, row)
		if charges <= 0 {
			box.w = personalSpellBadgeIconSize
		}
		row++
		iconBox := layoutRect{box.x, box.y, personalSpellBadgeIconSize, personalSpellBadgeIconSize}
		drawFilledRect(screen, box.x, box.y, box.w, box.h, color.RGBA{8, 12, 18, 255})
		drawRectBorder(screen, box.x, box.y, box.w, box.h, 1, color.RGBA{72, 78, 87, 255})
		drawFilledRect(screen, box.x, box.y+2, 1, box.h-4, accent)
		if staff {
			drawFlowingStaffCharge(screen, iconBox)
		} else if ui.game.sprites.HasSprite(icon) {
			drawImageScaled(screen, ui.game.sprites.GetSprite(icon), iconBox.x, iconBox.y, iconBox.w, iconBox.h)
		}
		if charges > 0 {
			text := fmt.Sprint(charges)
			scale := math.Min(float64(personalSpellBadgeCountW)/float64(max(1, uiTextWidth(text))), 8.0/float64(uiTextCharHeight))
			drawFilledRect(screen, box.right()-personalSpellBadgeCountW, box.bottom()-8, personalSpellBadgeCountW, 8, color.RGBA{8, 12, 18, 240})
			drawScaledCenteredText(screen, text, box.right()-personalSpellBadgeCountW/2, box.bottom()-4, scale, raritySilver)
		}
		if !ui.modalLayerOwnsInput() && isMouseHoveringBox(mx, my, box.x, box.y, box.right(), box.bottom()) {
			lines := []string{label, detail}
			if charges > 0 {
				lines = append(lines, uitext.Text("advanced.charges", charges))
			}
			ui.queueTooltip(lines, mx+12, my+8)
		}
	}
	if n := c.FlowingStaffCharges(); n > 0 {
		draw("", character.SkillFlowingStaff.String(), uitext.Text("advanced.flowing_staff_effect", character.FlowingStaffPct(c.SkillTier(character.SkillFlowingStaff))), n, true, color.RGBA{86, 238, 209, 255})
	}
	for _, e := range c.SpellState.Effects {
		d, ok := config.GetSpellDefinition(e.SpellID)
		if !ok || d.Mechanic == nil {
			continue
		}
		detail := d.Mechanic.SummaryText(e.Tier, d.DurationByMastery)
		if e.Frames > 0 {
			detail += " | " + uitext.Text("buff.remaining", ui.game.buffRemainingText(e.Frames))
		}
		// The authored targeting glyph is deliberately readable without a book
		// frame at this scale; reuse it instead of shrinking a detailed square.
		icon := spellTooltipIconName(spells.SpellID(e.SpellID))
		if d.Graphics != nil && d.Graphics.TargetCursor != "" {
			icon = d.Graphics.TargetCursor
		}
		draw(icon, d.Name, detail, e.Charges, false, SchoolColor(d.School))
	}
}

func (g *MMGame) automaticCastCostText() string {
	return uitext.Text("advanced.auto_cost", g.config.AutocastIntervalRounds, g.config.AutocastIntervalRounds*config.CombatRoundSeconds)
}
