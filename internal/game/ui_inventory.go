package game

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawInventoryContent draws the inventory tab content
func (ui *UISystem) drawInventoryContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== PARTY INVENTORY & EQUIPMENT ===", panelX+20, contentY+10)

	// Party resources
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Gold: %d  Food: %d  Total Items: %d",
		ui.game.party.Gold, ui.game.party.Food, ui.game.party.GetTotalItems()),
		panelX+20, contentY+30)

	// Split into two sections: Equipment (left) and General Inventory (right)

	// Equipment section (left side)
	equipX := panelX + 20
	equipY := contentY + 60
	ebitenutil.DebugPrintAt(screen, "=== CHARACTER EQUIPMENT ===", equipX, equipY)

	currentChar := ui.game.party.Members[ui.game.selectedChar]
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s's Equipment:", currentChar.Name), equipX, equipY+20)

	// Show equipped items for selected character
	equipSlots := []struct {
		slot items.EquipSlot
		name string
	}{
		{items.SlotMainHand, "Main Hand"},
		{items.SlotOffHand, "Off Hand"},
		{items.SlotSpell, "Spell"},
		{items.SlotArmor, "Armor"},
		{items.SlotHelmet, "Helmet"},
		{items.SlotBoots, "Boots"},
		{items.SlotGauntlets, "Gauntlets"},
		{items.SlotBelt, "Belt"},
		{items.SlotCloak, "Cloak"},
		{items.SlotAmulet, "Amulet"},
		{items.SlotRing1, "Ring 1"},
		{items.SlotRing2, "Ring 2"},
	}

	equipmentY := equipY + 40
	mouseX, mouseY := ebiten.CursorPosition()
	var equipTooltip string
	var equipTooltipItem items.Item
	var equipTooltipHasItem bool
	var equipTooltipX, equipTooltipY int
	for i, slotInfo := range equipSlots {
		y := equipmentY + (i * 15)
		if item, equipped := currentChar.Equipment[slotInfo.slot]; equipped {
			// Create colored background for equipped items
			isHovering := isMouseHoveringBox(mouseX, mouseY, equipX, y, equipX+220, y+15)

			var bgColor color.RGBA
			if isHovering {
				bgColor = color.RGBA{60, 80, 40, 80} // Green tint when hovering over equipped items
			} else {
				bgColor = color.RGBA{30, 40, 20, 40} // Subtle green background for equipped items
			}

			drawFilledRect(screen, equipX, y, 220, 15, bgColor)

			label := fmt.Sprintf("%-8s: ", slotInfo.name)
			drawColoredTextSegments(screen, equipX, y, []coloredTextSegment{
				{text: label, color: color.White},
				{text: item.Name, color: ui.itemRarityColor(item)},
			})

			// Handle double-click to unequip
			ui.handleEquippedItemClick(slotInfo.slot, equipX, y, equipX+220, y+15)

			// Handle hover tooltip
			if isHovering {
				equipTooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				equipTooltipItem = item
				equipTooltipHasItem = true
				equipTooltipX = mouseX + 16
				equipTooltipY = mouseY + 8
			}
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-8s: (empty)", slotInfo.name), equipX, y)
		}
	}
	// Draw tooltip for equipped item if needed
	if equipTooltip != "" && equipTooltipHasItem {
		lines := strings.Split(equipTooltip, "\n")
		ui.queueTooltipColoredIcon(lines, ui.itemTooltipColors(equipTooltipItem, lines), itemTooltipIconName(equipTooltipItem), equipTooltipX, equipTooltipY)
	}

	// General inventory section (right side)
	invX := panelX + 350 // Moved further right due to larger panel
	invY := contentY + 60
	ebitenutil.DebugPrintAt(screen, "=== GENERAL INVENTORY ===", invX, invY)

	// Inventory items
	itemsY := invY + 20
	if len(ui.game.party.Inventory) == 0 {
		ebitenutil.DebugPrintAt(screen, "No items in inventory", invX, itemsY)
	} else {
		mouseX, mouseY := ebiten.CursorPosition()
		var tooltip string
		var compareTooltip string
		var tooltipItem items.Item
		var tooltipHasItem bool
		var tooltipX, tooltipY int
		for i, item := range ui.game.party.Inventory {
			if i >= 15 {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("... and %d more items",
					len(ui.game.party.Inventory)-15), invX, itemsY+(i*15))
				break
			}
			y := itemsY + (i * 15)
			currentChar := ui.game.party.Members[ui.game.selectedChar]

			// Check if item can be equipped by current character
			canEquip := true
			if item.Type == items.ItemWeapon {
				canEquip = currentChar.CanEquipWeaponByName(item.Name)
			} else if item.Type == items.ItemArmor {
				canEquip = currentChar.CanEquipArmor(item)
			}

			// Create colored background for the item
			var bgColor color.RGBA
			isHovering := isMouseHoveringBox(mouseX, mouseY, invX, y, invX+200, y+15)

			if !canEquip {
				// Red background for unusable items
				if isHovering {
					bgColor = color.RGBA{120, 40, 40, 100}
				} else {
					bgColor = color.RGBA{80, 20, 20, 60}
				}
			} else {
				// Normal background
				if isHovering {
					bgColor = color.RGBA{40, 40, 80, 80}
				} else {
					bgColor = color.RGBA{20, 20, 40, 40}
				}
			}

			drawFilledRect(screen, invX, y, 200, 15, bgColor)

			// Draw item name
			prefix := fmt.Sprintf("%d. ", i+1)
			suffix := ""
			if !canEquip {
				suffix = " (can't equip)"
			}
			drawColoredTextSegments(screen, invX, y, []coloredTextSegment{
				{text: prefix, color: color.White},
				{text: item.Name, color: ui.itemRarityColor(item)},
				{text: suffix, color: color.White},
			})

			// Handle double-click to equip
			if !ui.inventoryContextOpen {
				ui.handleInventoryItemClick(i, invX, y, invX+200, y+15)
			}

			// Handle right-click to open context menu
			if !ui.inventoryContextOpen && ui.game.consumeRightClickIn(invX, y, invX+200, y+15) {
				ui.inventoryContextOpen = true
				ui.inventoryContextX = ui.game.mouseRightClickX
				ui.inventoryContextY = ui.game.mouseRightClickY
				ui.inventoryContextIndex = i
			}

			// Handle hover tooltip
			if isHovering {
				// Show tooltip for this item
				tooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				compareTooltip = GetItemComparisonTooltip(item, currentChar, ui.game.combat)
				tooltipItem = item
				tooltipHasItem = true
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}
		}
		// Draw tooltip if needed
		if tooltip != "" && tooltipHasItem {
			lines := strings.Split(tooltip, "\n")
			ui.queueTooltipColoredIcon(lines, ui.itemTooltipColors(tooltipItem, lines), itemTooltipIconName(tooltipItem), tooltipX, tooltipY)
			if compareTooltip != "" {
				compareLines := strings.Split(compareTooltip, "\n")
				ui.queueTooltipComparison(compareLines, ui.itemTooltipColors(tooltipItem, compareLines))
			}
		}

		// Draw inventory context menu if open
		if ui.inventoryContextOpen {
			menuW := 140
			menuH := 24
			x := ui.inventoryContextX
			y := ui.inventoryContextY
			// Background
			drawFilledRect(screen, x, y, menuW, menuH, color.RGBA{40, 40, 60, 230})
			// Border
			drawRectBorder(screen, x, y, menuW, menuH, 2, color.RGBA{120, 120, 160, 255})
			// Entry text
			drawCenteredDebugText(screen, "Discard", x, y, menuW, menuH)

			// Handle clicks on context menu
			if ui.game.consumeLeftClickIn(x, y, x+menuW, y+menuH) {
				// Discard clicked
				idx := ui.inventoryContextIndex
				if idx >= 0 && idx < len(ui.game.party.Inventory) {
					item := ui.game.party.Inventory[idx]
					if item.Type == items.ItemQuest {
						ui.game.AddCombatMessage(fmt.Sprintf("Cannot discard %s.", item.Name))
					} else {
						ui.game.party.RemoveItem(idx)
						ui.game.AddCombatMessage(fmt.Sprintf("Discarded %s.", item.Name))
					}
				}
				ui.inventoryContextOpen = false
			} else if ui.game.consumeLeftClick() {
				// Close the context menu on any left click
				ui.inventoryContextOpen = false
			}

			// Mouse state is updated once per frame in updateMouseState().
		}
	}

	// Instructions
	instructionY := contentY + contentHeight - 55
	ebitenutil.DebugPrintAt(screen, "Use 1-4 keys to select different characters and view their equipment", panelX+20, instructionY)
	ebitenutil.DebugPrintAt(screen, "Double-click items in inventory to equip them (red items can't be equipped)", panelX+20, instructionY+15)
	ebitenutil.DebugPrintAt(screen, "Double-click equipped items to unequip them back to inventory", panelX+20, instructionY+30)
	ebitenutil.DebugPrintAt(screen, "Right-click an inventory item to discard it", panelX+20, instructionY+45)
}

// drawCharactersContent draws the characters tab content
func (ui *UISystem) drawCharactersContent(screen *ebiten.Image, panelX, contentY int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== CHARACTER INFO ===", panelX+20, contentY+10)

	if len(ui.game.party.Members) == 0 {
		ebitenutil.DebugPrintAt(screen, "No party members.", panelX+20, contentY+40)
		return
	}

	// Only show the selected character
	charIndex := ui.game.selectedChar
	if charIndex < 0 || charIndex >= len(ui.game.party.Members) {
		charIndex = 0
	}
	member := ui.game.party.Members[charIndex]
	mouseX, mouseY := ebiten.CursorPosition()
	var tooltip string
	var tooltipX, tooltipY int

	// Character layout
	cardX := panelX + 20
	cardY := contentY + 40
	portraitX := cardX
	portraitY := cardY + 8
	portraitSize := 180
	scrollX := cardX + portraitSize + 24
	scrollY := cardY
	scrollW := 420
	scrollH := 330
	drawNineSlice(screen, ui.game.sprites.GetSprite("character_scroll_panel"), scrollX, scrollY, scrollW, scrollH, 16)

	portraitName := strings.ToLower(member.Name) + "_full"
	portrait := ui.game.sprites.GetSprite(portraitName)
	portraitFramePad := 6
	drawNineSlice(screen, ui.game.sprites.GetSprite("menu_panel_frame"), portraitX-portraitFramePad, portraitY-portraitFramePad, portraitSize+portraitFramePad*2, portraitSize+portraitFramePad*2, menuPanelFrameSlice)
	drawImageScaled(screen, portrait, portraitX, portraitY, portraitSize, portraitSize)

	// Text colors tuned for the parchment scroll: near-black body for readable
	// values, deep maroon for section headers so they stand out from the body.
	textColor := color.RGBA{16, 8, 4, 255}
	mutedTextColor := color.RGBA{96, 32, 20, 255}
	scrollTextX := scrollX + 26
	scrollTextY := scrollY + 18

	// Header
	header := fmt.Sprintf("%d. %s (%s) Level %d", charIndex+1, member.Name, member.Class.String(), member.Level)
	drawDebugTextColored(screen, header, scrollTextX, scrollTextY, textColor)

	// Core info
	drawDebugTextColored(screen, fmt.Sprintf("Health: %d/%d", member.HitPoints, member.MaxHitPoints), scrollTextX, scrollTextY+22, textColor)
	drawDebugTextColored(screen, fmt.Sprintf("Spell Points: %d/%d", member.SpellPoints, member.MaxSpellPoints), scrollTextX+190, scrollTextY+22, textColor)
	drawDebugTextColored(screen, fmt.Sprintf("Experience: %d", member.Experience), scrollTextX, scrollTextY+38, textColor)

	statusText := "Status: Normal"
	if len(member.Conditions) > 0 {
		names := make([]string, 0, len(member.Conditions))
		for _, cond := range member.Conditions {
			names = append(names, cond.String())
		}
		statusText = fmt.Sprintf("Status: %s", strings.Join(names, ", "))
	}
	drawDebugTextColored(screen, statusText, scrollTextX+190, scrollTextY+38, textColor)

	// Column layout for the scroll body: two columns side-by-side fit within
	// scrollW=420 (minus padding). All sections below stay within scrollH=300.
	const (
		rowH      = 14
		colGap    = 190
		statRows  = 4 // 7 stats split 4 + 3 across two columns
		skillRows = 4 // up to 8 skills shown across two columns
		magicRows = 3 // up to 6 magic schools shown across two columns
	)
	col1X := scrollTextX
	col2X := scrollTextX + colGap

	// Stats — 2 columns
	drawDebugTextColored(screen, "STATS", scrollTextX, scrollTextY+60, mutedTextColor)
	statY := scrollTextY + 76
	statLines := []struct {
		name  string
		value int
	}{
		{"Might", member.Might},
		{"Intellect", member.Intellect},
		{"Personality", member.Personality},
		{"Endurance", member.Endurance},
		{"Accuracy", member.Accuracy},
		{"Speed", member.Speed},
		{"Luck", member.Luck},
	}
	for i, stat := range statLines {
		x := col1X
		if i >= statRows {
			x = col2X
		}
		y := statY + (i%statRows)*rowH
		drawDebugTextColored(screen, fmt.Sprintf("%s: %d", stat.name, stat.value), x, y, textColor)
		if tooltip == "" && isMouseHoveringBox(mouseX, mouseY, x, y, x+colGap-10, y+rowH) {
			tooltip = statTooltipText(stat.name)
			tooltipX = mouseX + 16
			tooltipY = mouseY + 8
		}
	}

	// Skills — 2 columns
	skillY := statY + statRows*rowH + 12
	drawDebugTextColored(screen, "SKILLS", col1X, skillY, mutedTextColor)
	skillY += rowH + 2
	skillOrder := []character.SkillType{
		character.SkillSword, character.SkillDagger, character.SkillAxe,
		character.SkillSpear, character.SkillBow, character.SkillMace,
		character.SkillStaff, character.SkillLeather, character.SkillChain,
		character.SkillPlate, character.SkillShield, character.SkillBodybuilding,
		character.SkillMeditation, character.SkillMerchant, character.SkillRepair,
		character.SkillIdentifyItem, character.SkillDisarmTrap, character.SkillLearning,
		character.SkillArmsMaster,
	}
	skillIdx := 0
	for _, st := range skillOrder {
		if skillIdx >= skillRows*2 {
			break // ran out of column space; rest is hidden
		}
		s, ok := member.Skills[st]
		if !ok || s == nil {
			continue
		}
		line := fmt.Sprintf("%s %d (%s)", st.String(), s.Level(), s.Mastery.String())
		x := col1X
		if skillIdx >= skillRows {
			x = col2X
		}
		y := skillY + (skillIdx%skillRows)*rowH
		drawDebugTextColored(screen, line, x, y, textColor)
		if tooltip == "" && isMouseHoveringBox(mouseX, mouseY, x, y, x+colGap-10, y+rowH) {
			tooltip = masteryTooltipTextForSkill(st)
			tooltipX = mouseX + 16
			tooltipY = mouseY + 8
		}
		skillIdx++
	}
	if skillIdx == 0 {
		drawDebugTextColored(screen, "None", col1X, skillY, textColor)
	}

	// Magic schools — 2 columns
	magicY := skillY + skillRows*rowH + 12
	drawDebugTextColored(screen, "MAGIC SCHOOLS", col1X, magicY, mutedTextColor)
	magicY += rowH + 2
	schoolIdx := 0
	for _, school := range character.AllMagicSchools {
		if schoolIdx >= magicRows*2 {
			break
		}
		ms, ok := member.MagicSchools[school]
		if !ok || ms == nil {
			continue
		}
		line := fmt.Sprintf("%s %d (%s) C:%d",
			school.DisplayName(), ms.Level(), ms.Mastery, ms.CastCount)
		x := col1X
		if schoolIdx >= magicRows {
			x = col2X
		}
		y := magicY + (schoolIdx%magicRows)*rowH
		drawDebugTextColored(screen, line, x, y, textColor)
		if tooltip == "" && isMouseHoveringBox(mouseX, mouseY, x, y, x+colGap-10, y+rowH) {
			tooltip = magicMasteryTooltipText()
			tooltipX = mouseX + 16
			tooltipY = mouseY + 8
		}
		schoolIdx++
	}
	if schoolIdx == 0 {
		drawDebugTextColored(screen, "None", col1X, magicY, textColor)
	}

	// Instructions
	ebitenutil.DebugPrintAt(screen, "Use 1-4 keys to switch character", panelX+20, scrollY+scrollH+18)

	if tooltip != "" {
		ui.queueTooltip(strings.Split(tooltip, "\n"), tooltipX, tooltipY)
	}
}

// drawSpellbookContent draws the spellbook tab content
func (ui *UISystem) drawSpellbookContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	schools := spellbookSchoolsWithSpells(currentChar)

	// Validate and fix selected school index if it's out of bounds
	if ui.game.selectedSchool >= len(schools) {
		ui.game.selectedSchool = 0
		ui.game.selectedSpell = 0
	}

	bookX := panelX + 24
	// Push the book down so the bookmark flags have room between the menu tabs and the book.
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

	// Draw bookmarks first so the book sprite hides the inserted portion.
	if len(schools) > 0 {
		var selSchool character.MagicSchoolID
		if ui.game.selectedSchool < len(schools) {
			selSchool = schools[ui.game.selectedSchool]
		}
		ui.drawSpellbookSchoolTabs(screen, schools, selSchool, bookX, bookY, scaleX, scaleY)
	}

	drawImageScaled(screen, ui.game.sprites.GetSprite("spellbook_open"), bookX, bookY, bookW, bookH)

	leftTextX := srcX(92)
	leftTextW := srcW(350)

	drawCenteredDebugText(screen, fmt.Sprintf("%s's Spellbook", currentChar.Name), leftTextX, srcY(72), leftTextW, 20)
	// SP counter is shown in the party panel; no need to duplicate it in the book.
	// School name is shown by the active bookmark; no header needed on the right page.

	if len(schools) == 0 {
		drawCenteredDebugText(screen, "No magic schools available", bookX+24, bookY+bookH/2-8, bookW-48, 20)
		return
	}

	var spellTooltip string
	var spellCompareTooltip string
	var spellTooltipID spells.SpellID
	var tooltipX, tooltipY int

	if ui.game.selectedSchool >= len(schools) {
		return
	}

	selectedSchool := schools[ui.game.selectedSchool]
	// School tabs are drawn before the book so they appear inserted; click handling lives inside drawSpellbookSchoolTabs.

	schoolSpells := currentChar.GetSpellsForSchool(selectedSchool)
	if ui.game.selectedSpell >= len(schoolSpells) {
		ui.game.selectedSpell = 0
	}

	if len(schoolSpells) == 0 {
		drawCenteredDebugText(screen, "No learned spells", leftTextX, bookY+bookH/2-8, leftTextW, 20)
	} else {
		// 2×2 grid per page (left + right) = up to 8 spells visible at once.
		gridY := srcY(118)
		cardW := srcW(180)
		cardH := srcH(150)
		cols := 2
		const cardsPerPage = 4
		iconSize := srcW(96)
		// Clamp icon size so name + stats rows fit below it without overlap at small scales.
		if maxIcon := cardH - 2*debugTextCharHeight - 12; iconSize > maxIcon {
			iconSize = maxIcon
		}
		if iconSize < 16 {
			iconSize = 16
		}
		cardGap := srcW(18)
		rowGap := srcH(14)
		// Centre the 2×2 grid on the parchment area of each page. Source-coord
		// centres measured from the spellbook_open sprite: left page parchment
		// spans x=87..468 (centre 278), right page spans x=558..936 (centre 747).
		gridW := cols*cardW + (cols-1)*cardGap
		pageOriginX := [2]int{
			srcX(278) - gridW/2,
			srcX(747) - gridW/2,
		}
		mouseX, mouseY := ebiten.CursorPosition()

		for spellIndex, spellID := range schoolSpells {
			if spellIndex >= 2*cardsPerPage {
				break
			}
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue
			}

			page := spellIndex / cardsPerPage
			local := spellIndex % cardsPerPage
			col := local % cols
			row := local / cols
			cardX := pageOriginX[page] + col*(cardW+cardGap)
			cardY := gridY + row*(cardH+rowGap)
			if cardY+cardH > srcY(460) {
				continue
			}

			ui.handleSpellbookSpellClick(cardX, cardY, cardW, cardH, ui.game.selectedSchool, spellIndex)
			isSelected := spellIndex == ui.game.selectedSpell
			isHovering := mouseX >= cardX && mouseX < cardX+cardW && mouseY >= cardY && mouseY < cardY+cardH
			ui.drawSpellbookSpellCard(screen, cardX, cardY, cardW, cardH, iconSize, spellID, def, currentChar, isSelected)

			if isHovering {
				spellTooltip = GetSpellTooltip(spellID, currentChar, ui.game.combat)
				spellCompareTooltip = GetSpellComparisonTooltip(spellID, currentChar, ui.game.combat)
				spellTooltipID = spellID
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}
		}
	}

	// Draw spell tooltip if hovering over a spell
	if spellTooltip != "" {
		lines := strings.Split(spellTooltip, "\n")
		ui.queueTooltipIcon(lines, spellTooltipIconName(spellTooltipID), tooltipX, tooltipY)
		if spellCompareTooltip != "" {
			compareLines := strings.Split(spellCompareTooltip, "\n")
			ui.queueTooltipComparison(compareLines, nil)
		}
	}

	// Draw spellbook controls
	drawCenteredDebugText(screen, "Up/Down: Navigate  Enter/F: Equip fast spell  Click: Select  Double-click: Cast", bookX+20, contentY+contentHeight-28, bookW-40, 20)
}

func spellbookSchoolsWithSpells(currentChar *character.MMCharacter) []character.MagicSchoolID {
	available := currentChar.GetAvailableSchools()
	schools := make([]character.MagicSchoolID, 0, len(available))
	for _, school := range available {
		if len(currentChar.GetSpellsForSchool(school)) > 0 {
			schools = append(schools, school)
		}
	}
	return schools
}

func (ui *UISystem) drawSpellbookSchoolTabs(screen *ebiten.Image, schools []character.MagicSchoolID, selectedSchool character.MagicSchoolID, bookX, bookY int, scaleX, scaleY float64) {
	if len(schools) == 0 {
		return
	}
	tabW := int(72 * scaleX)
	tabH := int(112 * scaleY)
	if tabW < 44 {
		tabW = 44
	}
	if tabH < 68 {
		tabH = 68
	}
	gap := int(10 * scaleX)
	if gap < 4 {
		gap = 4
	}
	// Bookmarks start from the left side of the book. They are drawn before the
	// book sprite so the inserted lower portion is hidden behind the page.
	startX := bookX + int(40*scaleX)
	// Show roughly the upper half of the tab; the lower half is hidden behind the book.
	visibleFrac := 0.55
	hiddenH := int(float64(tabH) * (1 - visibleFrac))
	for i, school := range schools {
		tabX := startX + i*(tabW+gap)
		tabY := bookY - (tabH - hiddenH)
		if school == selectedSchool {
			// Pull the selected bookmark slightly further up.
			tabY -= int(10 * scaleY)
		}
		// Click region covers the entire bookmark sprite — the lower portion is
		// only visually hidden by the book, the bookmark itself is still the target.
		ui.handleSpellbookSchoolClick(tabX, tabY, tabW, tabH, i, school)
		drawImageScaled(screen, ui.game.sprites.GetSprite("spellbook_tab_"+school.String()), tabX, tabY, tabW, tabH)
	}
}

func (ui *UISystem) drawSpellbookSpellCard(screen *ebiten.Image, x, y, w, h, iconSize int, spellID spells.SpellID, def spells.SpellDefinition, currentChar *character.MMCharacter, selected bool) {
	if selected {
		// Selected spell: dark gold border, contrasts against the parchment pages.
		vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 3, color.RGBA{170, 115, 30, 255}, false)
	}

	iconX := x + (w-iconSize)/2
	iconY := y + 6
	iconName := spellTooltipIconName(spellID)
	if ui.game.sprites.HasSprite(iconName) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(iconName), iconX, iconY, iconSize, iconSize)
	} else {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{42, 32, 45, 255})
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{218, 170, 72, 255})
		drawCenteredDebugText(screen, spellInitials(def.Name), iconX, iconY, iconSize, iconSize)
	}

	name := truncateName(def.Name, 12)
	nameY := y + iconSize + 8
	statsY := nameY + debugTextCharHeight + 2
	drawCenteredDebugText(screen, name, x+4, nameY, w-8, debugTextCharHeight)
	drawCenteredDebugText(screen, fmt.Sprintf("SP %d  Lv %d", def.SpellPointsCost, def.Level), x+4, statsY, w-8, debugTextCharHeight)
	if currentChar.SpellPoints < def.SpellPointsCost {
		// Red icon outline signals "not enough SP".
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{120, 38, 28, 255})
	}
}

func spellInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	initials := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		initials += strings.ToUpper(part[:1])
		if len(initials) >= 2 {
			break
		}
	}
	return initials
}

// handleInventoryItemClick handles double-click to equip items from inventory
func (ui *UISystem) handleInventoryItemClick(itemIndex int, x1, y1, x2, y2 int) {
	// When a modal popup is up, suppress inventory clicks entirely. Without
	// this an open revival picker (whose item-index points into inventory)
	// would be invalidated if the user double-clicked another consumable —
	// applyReviveTo's re-validation would silently no-op the chosen potion.
	if ui.game.revivalPickerOpen || ui.game.statPopupOpen || ui.game.currentLevelUpChoice() != nil {
		return
	}
	// Check for mouse click
	if ui.game.consumeLeftClickIn(x1, y1, x2, y2) {
		currentTime := time.UnixMilli(ui.game.mouseLeftClickAt)

		// Check for double-click (same item clicked within 500ms)
		delta := currentTime.Sub(ui.lastClickTime)
		doubleClick := itemIndex == ui.lastClickedItem && delta < doubleClickWindow
		if doubleClick {
			// Double-click detected - try to equip or use the item
			item := ui.game.party.Inventory[itemIndex]
			currentChar := ui.game.party.Members[ui.game.selectedChar]

			if item.Type == items.ItemQuest {
				if item.Attributes["opens_map"] > 0 {
					ui.game.mapOverlayOpen = true
					return
				}
			}

			if item.Type == items.ItemConsumable {
				// Use consumable item
				ui.game.UseConsumableFromInventory(itemIndex, ui.game.selectedChar)
			} else if item.Type == items.ItemWeapon {
				if currentChar.CanEquipWeaponByName(item.Name) {
					if ui.game.party.EquipItemFromInventory(itemIndex, ui.game.selectedChar) {
						ui.game.AddCombatMessage(fmt.Sprintf("%s equipped %s!",
							currentChar.Name, item.Name))
					}
				} else {
					ui.game.AddCombatMessage(fmt.Sprintf("%s cannot use %s!",
						currentChar.Name, item.Name))
				}
			} else if item.Type == items.ItemArmor {
				if currentChar.CanEquipArmor(item) {
					if ui.game.party.EquipItemFromInventory(itemIndex, ui.game.selectedChar) {
						ui.game.AddCombatMessage(fmt.Sprintf("%s equipped %s!",
							currentChar.Name, item.Name))
					}
				} else {
					ui.game.AddCombatMessage(fmt.Sprintf("%s cannot wear %s!",
						currentChar.Name, item.Name))
				}
			} else if item.Type == items.ItemAccessory {
				if ui.game.party.EquipItemFromInventory(itemIndex, ui.game.selectedChar) {
					ui.game.AddCombatMessage(fmt.Sprintf("%s equipped %s!",
						currentChar.Name, item.Name))
				}
			} else {
				// Other item types (spells, etc.)
				if ui.game.party.EquipItemFromInventory(itemIndex, ui.game.selectedChar) {
					ui.game.AddCombatMessage(fmt.Sprintf("%s equipped %s!",
						currentChar.Name, item.Name))
				}
			}
		}

		ui.lastClickedItem = itemIndex
		ui.lastClickTime = currentTime
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// handleEquippedItemClick handles double-click to unequip items from equipment slots
func (ui *UISystem) handleEquippedItemClick(slot items.EquipSlot, x1, y1, x2, y2 int) {
	// Check for mouse click
	if ui.game.consumeLeftClickIn(x1, y1, x2, y2) {
		currentTime := time.UnixMilli(ui.game.mouseLeftClickAt)

		// Check for double-click (same slot clicked within 500ms)
		delta := currentTime.Sub(ui.lastEquipClickTime)
		doubleClick := slot == ui.lastClickedSlot && delta < doubleClickWindow
		if doubleClick {
			// Double-click detected - try to unequip the item
			currentChar := ui.game.party.Members[ui.game.selectedChar]
			if item, exists := currentChar.Equipment[slot]; exists {
				itemName := item.Name
				if ui.game.party.UnequipItemToInventory(slot, ui.game.selectedChar) {
					ui.game.AddCombatMessage(fmt.Sprintf("%s unequipped %s!",
						currentChar.Name, itemName))
				} else {
					ui.game.AddCombatMessage("Cannot unequip this item!")
				}
			}
		}

		ui.lastClickedSlot = slot
		ui.lastEquipClickTime = currentTime
	}

	// Mouse state is updated once per frame in updateMouseState().
}
