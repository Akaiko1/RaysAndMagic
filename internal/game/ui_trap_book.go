package game

import (
	"fmt"
	"image/color"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// The thief's trap book - rendered in the spellbook tab slot for characters
// with the Trapper skill (they have no magic schools). Spell-like controls:
// click / Up-Down browse, double-click equips, and Enter/F uses the selection.

// drawTrapBookContent mirrors the spellbook layout on the trap_recipe book art.
func (ui *UISystem) drawTrapBookContent(screen *ebiten.Image, content layoutRect) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]

	bl := computeBookLayout(content)

	drawCenteredDebugText(screen, fmt.Sprintf("%s - Trap Book", currentChar.Name), bl.header.x, bl.header.y, bl.header.w, bl.header.h)
	drawImageScaled(screen, ui.game.sprites.GetSprite("trap_recipe_book_open"), bl.bookX, bl.bookY, bl.bookW, bl.bookH)

	keys := availableTraps(currentChar)
	if len(keys) == 0 {
		drawCenteredDebugText(screen, "No traps known", bl.bookX+24, bl.bookY+bl.bookH/2-8, bl.bookW-48, 20)
		return
	}

	mouseX, mouseY := ebiten.CursorPosition()

	var tooltip string
	var tooltipIcon string
	var tooltipX, tooltipY int

	if ui.game.selectedTrap >= len(keys) || ui.game.selectedTrap < 0 {
		ui.game.selectedTrap = 0
	}
	perSpread := bl.cardsPerSpread()
	totalPages := pageCount(len(keys), perSpread)
	ui.spellPage = ui.game.selectedTrap / perSpread
	clampPage(&ui.spellPage, totalPages)

	pageStart := ui.spellPage * perSpread
	pageEnd := min(len(keys), pageStart+perSpread)
	for i := pageStart; i < pageEnd; i++ {
		key := keys[i]
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			continue
		}
		cardX, cardY := bl.cardPos(i - pageStart)
		if cardY+bl.cardH > bl.gridMaxY {
			continue
		}

		ui.handleBookEntryClick(layoutRect{cardX, cardY, bl.cardW, bl.cardH}, -1, i, func() {
			ui.game.selectedTrap = i
		}, func() { equipTrap(currentChar, key) })
		ui.quickTrapCardDragSource(key, cardX, cardY, bl.cardW, bl.cardH)
		ui.drawTrapCard(screen, cardX, cardY, bl.cardW, bl.cardH, bl.iconSize, key, def, currentChar, i == ui.game.selectedTrap)

		if mouseX >= cardX && mouseX < cardX+bl.cardW && mouseY >= cardY && mouseY < cardY+bl.cardH {
			tooltip = trapTooltip(key, def, currentChar, ui.game.combat)
			tooltipIcon = def.Icon
			tooltipX, tooltipY = mouseX+16, mouseY+8
		}
	}

	if tooltip != "" {
		ui.queueTitledTooltipIcon(strings.Split(tooltip, "\n"), nil, woodPlateColor, nil, tooltipIcon, tooltipX, tooltipY)
	}
	ui.drawPager(screen, bl.pager.x, bl.pager.y, bl.pager.w, &ui.spellPage, totalPages, !ui.modalLayerOwnsInput(), func() {
		ui.game.selectedTrap = ui.spellPage * perSpread
	})
	ui.drawTabQuickSlotBar(screen, bl.quick.x, bl.quick.y, bl.quick.w)
	drawCenteredDebugText(screen, bookControlsHint, bl.controls.x, bl.controls.y, bl.controls.w, bl.controls.h)
}

// drawTrapCard renders one trap entry: icon, name, SP/level row. The browse
// selection and quick-slot marker use the same icon treatment as spells.
func (ui *UISystem) drawTrapCard(screen *ebiten.Image, x, y, w, h, iconSize int, key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, selected bool) {
	iconX := x + (w-iconSize)/2
	iconY := y + 6
	if ui.game.sprites.HasSprite(def.Icon) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(def.Icon), iconX, iconY, iconSize, iconSize)
	} else {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{42, 32, 45, 255})
		drawCenteredDebugText(screen, spellInitials(def.Name), iconX, iconY, iconSize, iconSize)
	}

	cost := def.SPCost
	if ui.game.combat != nil {
		cost = ui.game.combat.effectiveSpellCost(char, def.SPCost)
	}
	nameY := y + iconSize + 8
	drawCenteredDebugText(screen, truncateName(def.Name, 12), x+4, nameY, w-8, debugTextCharHeight)
	drawCenteredDebugText(screen, fmt.Sprintf("SP %d  Lv %d", cost, def.Level), x+4, nameY+debugTextCharHeight+2, w-8, debugTextCharHeight)

	armed, equipped := equippedTrapKey(char)
	drawBookEntryState(screen, layoutRect{iconX, iconY, iconSize, iconSize},
		selected, equipped && armed == key, char.Level < def.Level, char.SpellPoints >= cost, SchoolColor(def.Element))
}

// trapTooltip renders the unified template card for a trap (the same builder
// the quick-slot hover uses).
func trapTooltip(key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, cs *CombatSystem) string {
	out := buildTrapTooltipUnified(key, def, char, cs, tooltipDetailHeld())
	if def.Description != "" {
		out += "\n\n\"" + def.Description + "\""
	}
	return out
}
