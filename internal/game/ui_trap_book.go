package game

import (
	"fmt"
	"image/color"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The thief's trap book — rendered in the spellbook tab slot for characters
// with the Trapper skill (they have no magic schools). Spell-like controls:
// click / Up-Down browse a selection, Enter/F or double-click equip it as the
// QuickTrap that Space arms in the world.

// drawTrapBookContent mirrors the spellbook layout on the trap_recipe book art.
func (ui *UISystem) drawTrapBookContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]

	bookX := panelX + 24
	bookY := contentY + 60
	bookW := 652
	bookH := bookW / 2
	if maxBookH := contentHeight - 94; bookH > maxBookH {
		bookH = maxBookH
		bookW = bookH * 2
		bookX = panelX + (700-bookW)/2
	}
	scaleX := float64(bookW) / 1024.0
	scaleY := float64(bookH) / 512.0
	srcX := func(v int) int { return bookX + int(float64(v)*scaleX) }
	srcY := func(v int) int { return bookY + int(float64(v)*scaleY) }
	srcW := func(v int) int { return int(float64(v) * scaleX) }
	srcH := func(v int) int { return int(float64(v) * scaleY) }

	drawImageScaled(screen, ui.game.sprites.GetSprite("trap_recipe_book_open"), bookX, bookY, bookW, bookH)
	leftTextX := srcX(92)
	leftTextW := srcW(350)
	drawCenteredDebugText(screen, fmt.Sprintf("%s's Trap Book", currentChar.Name), leftTextX, srcY(72), leftTextW, 20)

	keys := availableTraps(currentChar)
	if len(keys) == 0 {
		drawCenteredDebugText(screen, "No traps known", bookX+24, bookY+bookH/2-8, bookW-48, 20)
		return
	}

	// Same 2×2-per-page grid as the spellbook.
	gridY := srcY(118)
	cardW := srcW(180)
	cardH := srcH(150)
	cols := 2
	const cardsPerPage = 4
	iconSize := srcW(96)
	if maxIcon := cardH - 2*debugTextCharHeight - 12; iconSize > maxIcon {
		iconSize = maxIcon
	}
	if iconSize < 16 {
		iconSize = 16
	}
	cardGap := srcW(18)
	rowGap := srcH(14)
	gridW := cols*cardW + (cols-1)*cardGap
	pageOriginX := [2]int{srcX(278) - gridW/2, srcX(747) - gridW/2}
	mouseX, mouseY := ebiten.CursorPosition()

	var tooltip string
	var tooltipIcon string
	var tooltipX, tooltipY int

	if ui.game.selectedTrap >= len(keys) || ui.game.selectedTrap < 0 {
		ui.game.selectedTrap = 0
	}

	for i, key := range keys {
		if i >= 2*cardsPerPage {
			break
		}
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			continue
		}
		page := i / cardsPerPage
		local := i % cardsPerPage
		cardX := pageOriginX[page] + (local%cols)*(cardW+cardGap)
		cardY := gridY + (local/cols)*(cardH+rowGap)
		if cardY+cardH > srcY(460) {
			continue
		}

		// Spell-like mouse controls: click selects, double-click ARMS the
		// clicked trap in the world (spells cast on double-click; Enter/F
		// equip the quick slot). TB consumes an action like a book-cast spell.
		if ui.game.consumeLeftClickIn(cardX, cardY, cardX+cardW, cardY+cardH) {
			now := ui.game.mouseLeftClickAt
			if ui.game.lastClickedSpell == i && now-ui.game.lastSpellClickTime < doubleClickWindowMs {
				if _, placed := ui.game.combat.placeTrapByKey(currentChar, key, true); placed {
					ui.game.consumeSelectedCharAction()
				}
				ui.game.lastSpellClickTime = 0
				ui.game.lastClickedSpell = -1
			} else {
				ui.game.lastSpellClickTime = now
				ui.game.lastClickedSpell = i
			}
			ui.game.selectedTrap = i
		}
		ui.quickTrapCardDragSource(key, cardX, cardY, cardW, cardH)
		ui.drawTrapCard(screen, cardX, cardY, cardW, cardH, iconSize, key, def, currentChar, i == ui.game.selectedTrap)

		if mouseX >= cardX && mouseX < cardX+cardW && mouseY >= cardY && mouseY < cardY+cardH {
			tooltip = trapTooltip(key, def, currentChar, ui.game.combat)
			tooltipIcon = def.Icon
			tooltipX, tooltipY = mouseX+16, mouseY+8
		}
	}

	if tooltip != "" {
		ui.queueTooltipIcon(strings.Split(tooltip, "\n"), tooltipIcon, tooltipX, tooltipY)
	}
	drawCenteredDebugText(screen, "Up/Down: Navigate  Enter/F: Equip quick trap  Click: Select  Double-click: Arm trap", bookX+20, contentY+contentHeight-28, bookW-40, 20)

	// Quick-slot bar below the book, same as the spellbook (drag traps here).
	qbW := 360
	ui.drawTabQuickSlotBar(screen, bookX+(bookW-qbW)/2, bookY+bookH+16, qbW)
}

// drawTrapCard renders one trap entry: icon, name, SP/level row. The browse
// SELECTION gets a light outline; the EQUIPPED quick trap gets the gold one
// (both can sit on different cards, like spell selection vs the quick slot).
func (ui *UISystem) drawTrapCard(screen *ebiten.Image, x, y, w, h, iconSize int, key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, selected bool) {
	if selected {
		vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 2, color.RGBA{210, 205, 190, 255}, false)
	}
	if armed, ok := equippedTrapKey(char); ok && armed == key {
		vector.StrokeRect(screen, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 3, color.RGBA{170, 115, 30, 255}, false)
	}
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

	locked := char.Level < def.Level
	if locked {
		// Level lock: dark veil + red outline.
		drawFilledRect(screen, x, y, w, h, color.RGBA{0, 0, 0, 110})
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{120, 38, 28, 255})
	} else if char.SpellPoints < cost {
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{120, 38, 28, 255})
	}
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
