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
	drawNineSlice(screen, ui.game.sprites.GetSprite("menu_panel_frame"), portraitX-portraitFramePad, portraitY-portraitFramePad, portraitSize+portraitFramePad*2, portraitSize+portraitFramePad*2, 16)
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
		line := fmt.Sprintf("%s %d (%s)", st.String(), s.Level, s.Mastery.String())
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
			school.DisplayName(), ms.Level, ms.Mastery, ms.CastCount)
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
	schools := currentChar.GetAvailableSchools()

	// Validate and fix selected school index if it's out of bounds
	if ui.game.selectedSchool >= len(schools) {
		ui.game.selectedSchool = 0
		ui.game.selectedSpell = 0
	}

	// Title
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("=== %s's SPELLBOOK ===", currentChar.Name), panelX+20, contentY+10)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Spell Points: %d/%d",
		currentChar.SpellPoints, currentChar.MaxSpellPoints), panelX+20, contentY+30)

	if len(schools) == 0 {
		ebitenutil.DebugPrintAt(screen, "No magic schools available", panelX+30, contentY+60)
		return
	}

	// Draw schools and spells
	y := contentY + 60
	var spellTooltip string
	var spellCompareTooltip string
	var spellTooltipID spells.SpellID
	var tooltipX, tooltipY int

	for schoolIndex, school := range schools {
		schoolName := school.DisplayName()
		schoolSpells := currentChar.GetSpellsForSchool(school)

		// Handle mouse clicks on school names
		ui.handleSpellbookSchoolClick(panelX+30, y, 300, 20, schoolIndex, school)

		// Draw school name
		if schoolIndex == ui.game.selectedSchool {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("> %s School:", schoolName), panelX+30, y)
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  %s School:", schoolName), panelX+30, y)
		}
		y += 20

		// Validate and fix selected spell index if it's out of bounds
		if schoolIndex == ui.game.selectedSchool && ui.game.selectedSpell >= len(schoolSpells) {
			ui.game.selectedSpell = 0
		}

		// Draw spells for this school (unless collapsed)
		if ui.game.collapsedSpellSchools[school] {
			continue
		}

		for spellIndex, spellID := range schoolSpells {
			// Get spell definition from centralized system
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue // Skip invalid spells
			}

			canCast := "✓"
			if currentChar.SpellPoints < def.SpellPointsCost {
				canCast = "✗"
			}

			// Handle mouse interactions for spells
			spellY := y
			spellHeight := 15
			mouseX, mouseY := ebiten.CursorPosition()
			isHovering := mouseX >= panelX+50 && mouseX < panelX+350 && mouseY >= spellY && mouseY < spellY+spellHeight

			// Handle mouse clicks on spells
			ui.handleSpellbookSpellClick(panelX+50, spellY, 300, spellHeight, schoolIndex, spellIndex)

			// Generate tooltip for hovering spell
			if isHovering {
				// Draw hover background
				drawFilledRect(screen, panelX+50, spellY, 300, spellHeight, color.RGBA{100, 100, 150, 100})

				// Generate spell tooltip using SpellID
				spellTooltip = GetSpellTooltip(spellID, currentChar, ui.game.combat)
				spellCompareTooltip = GetSpellComparisonTooltip(spellID, currentChar, ui.game.combat)
				spellTooltipID = spellID
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}

			if schoolIndex == ui.game.selectedSchool && spellIndex == ui.game.selectedSpell {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  > %s %s (SP:%d)",
					canCast, def.Name, def.SpellPointsCost), panelX+50, y)
			} else {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("    %s %s (SP:%d)",
					canCast, def.Name, def.SpellPointsCost), panelX+50, y)
			}
			y += 15
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
	ebitenutil.DebugPrintAt(screen, "Up/Down: Navigate, Enter: Equip, Click: Select, Double-click: Cast", panelX+30, contentY+contentHeight-30)
}

// handleInventoryItemClick handles double-click to equip items from inventory
func (ui *UISystem) handleInventoryItemClick(itemIndex int, x1, y1, x2, y2 int) {
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
