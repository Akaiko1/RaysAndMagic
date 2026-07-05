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
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawInventoryContent draws the inventory tab content
func (ui *UISystem) drawInventoryContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	mouseX, mouseY := ebiten.CursorPosition()

	const (
		paperW   = inventoryPaperdollSourceW
		paperH   = inventoryPaperdollSourceH
		gridSize = inventoryGridSourceSize
		panelGap = 52
	)
	// inventory panel is 700 wide; centre the paperdoll+grid block within it.
	const blockW = paperW + panelGap + gridSize
	paperX := panelX + (700-blockW)/2
	paperY := contentY + 48
	gridX := paperX + paperW + panelGap
	gridY := contentY + 84

	drawDebugTextColored(screen, fmt.Sprintf("%s's equipment", currentChar.Name), paperX, contentY+10, color.RGBA{232, 222, 190, 255})
	drawDebugText(screen, fmt.Sprintf("Gold: %d  Food: %d  Total Items: %d",
		ui.game.party.Gold, ui.game.party.Food, ui.game.party.GetTotalItems()),
		paperX, contentY+29)

	drawImageScaled(screen, ui.game.sprites.GetSprite("inventory_paperdoll_panel"), paperX, paperY, paperW, paperH)
	drawImageScaled(screen, ui.game.sprites.GetSprite("inventory_grid_panel"), gridX, gridY, gridSize, gridSize)

	var tooltip string
	var compareTooltip string
	var tooltipItem items.Item
	var tooltipHasItem bool
	var tooltipX, tooltipY int

	for _, slotInfo := range inventoryPaperdollSlots {
		x, y, w, h := scaleInventorySourceRect(paperX, paperY, paperW, paperH, inventoryPaperdollSourceW, inventoryPaperdollSourceH, slotInfo.rect)
		item, equipped := currentChar.Equipment[slotInfo.slot]
		isHovering := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
		// While dragging an equippable item, glow every slot it can go into — from
		// the bag, or from another paperdoll slot (a ring to its other finger). The
		// source slot itself never glows.
		if ui.game.dragActive && equipItemMatchesSlot(currentChar, ui.game.dragItem, slotInfo.slot) &&
			(ui.game.dragSrc == dragFromInventory ||
				(ui.game.dragSrc == dragFromEquip && ui.game.dragEquipChar == ui.game.selectedChar && ui.game.dragEquipSlot != slotInfo.slot)) {
			drawRectBorder(screen, x-3, y-3, w+6, h+6, 3, color.RGBA{90, 220, 100, 240})
		}
		if isHovering {
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 230})
		}
		ui.equipSlotDropZone(slotInfo.slot, x, y, w, h) // drop inventory item here to equip
		if equipped {
			// Hide the icon while its own slot (on this same character) is dragged off.
			dragging := ui.game.dragActive && ui.game.dragSrc == dragFromEquip &&
				ui.game.dragEquipChar == ui.game.selectedChar && ui.game.dragEquipSlot == slotInfo.slot
			if !dragging {
				ui.drawInventoryItemIcon(screen, item, x, y, w, h, 0, true)
			}
			ui.handleEquippedItemClick(slotInfo.slot, x-3, y-3, x+w+3, y+h+3)
			ui.equipSlotDragSource(ui.game.selectedChar, slotInfo.slot, item, x, y, w, h) // pick up equipped item
			if isHovering {
				tooltip = GetItemTooltip(item, currentChar, ui.game.combat, tooltipDetailHeld())
				tooltipItem = item
				tooltipHasItem = true
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
				if key := itemCardKey(item); key != "" {
					ui.fullArtCardKey = key
				}
			}
		}
	}

	drawCenteredDebugText(screen, "Inventory", gridX, gridY-22, gridSize, 18)
	pageSize := len(inventoryGridSlots)
	totalPages := pageCount(len(ui.game.party.Inventory), pageSize)
	// Clamp every frame so the page stays valid when the inventory shrinks
	// (equip/discard) out from under the current page.
	clampPage(&ui.inventoryPage, totalPages)
	pageStart := ui.inventoryPage * pageSize
	for slot := 0; slot < pageSize; slot++ {
		idx := pageStart + slot
		x, y, w, h := scaleInventorySourceRect(gridX, gridY, gridSize, gridSize, inventoryGridSourceSize, inventoryGridSourceSize, inventoryGridSlots[slot])
		// Empty cell (guard against the LIVE length — a double-click below can
		// equip/use mid-loop and shrink the bag). Dropping a dragged item on an
		// empty cell moves it to the end (bag is a packed slice).
		if idx >= len(ui.game.party.Inventory) {
			if !ui.inventoryContextOpen {
				if ui.game.dragActive && ui.game.dragSrc == dragFromInventory &&
					isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
					drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 230})
				}
				ui.inventoryEmptyDropZone(x, y, w, h)
			}
			continue
		}
		item := ui.game.party.Inventory[idx]
		canEquip := ui.canSelectedCharacterEquipInventoryItem(item)
		isHovering := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
		if !canEquip {
			drawFilledRect(screen, x, y, w, h, color.RGBA{120, 28, 28, 95})
		}
		if isHovering {
			border := color.RGBA{210, 170, 80, 230}
			if !canEquip {
				border = color.RGBA{190, 70, 60, 230}
			}
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, border)
		}
		// Hide the icon of the cell currently being dragged out of.
		dragging := ui.game.dragActive && ui.game.dragSrc == dragFromInventory && ui.game.dragInvIndex == idx
		if !dragging {
			ui.drawInventoryItemIcon(screen, item, x, y, w, h, 4, canEquip)
		}

		if !ui.inventoryContextOpen {
			ui.handleInventoryItemClick(idx, x-3, y-3, x+w+3, y+h+3)
			ui.quickInvSlotDragSource(idx, x, y, w, h)
			ui.inventoryCellDropZone(idx, x, y, w, h) // drop another bag item here to swap
		}
		if !ui.inventoryContextOpen && !ui.inventoryInputBlocked() && ui.game.consumeRightClickIn(x-3, y-3, x+w+3, y+h+3) {
			ui.inventoryContextOpen = true
			ui.inventoryContextX = ui.game.mouseRightClickX
			ui.inventoryContextY = ui.game.mouseRightClickY
			ui.inventoryContextIndex = idx
		}
		if isHovering {
			tooltip = GetItemTooltip(item, currentChar, ui.game.combat, tooltipDetailHeld())
			compareTooltip = GetItemComparisonTooltip(item, currentChar, ui.game.combat)
			tooltipItem = item
			tooltipHasItem = true
			tooltipX = mouseX + 16
			tooltipY = mouseY + 8
			if key := itemCardKey(item); key != "" {
				ui.fullArtCardKey = key
			}
		}
	}
	// Below the grid: pager, then the Camp button + its rest-result notice
	// vertically centred in the gap between the grid box and the quick-slot bar,
	// then the compact quick-slot bar (kept off the paperdoll on the left).
	gridBottom := gridY + gridSize
	barTop := gridBottom + 88
	const campBlockH = 26 + 6 + 14 // button + gap + notice line
	campY := gridBottom + (barTop-gridBottom-campBlockH)/2
	ui.drawInventoryPager(screen, gridX, gridBottom+6, gridSize, totalPages)
	ui.drawCampButton(screen, gridX, campY, gridSize)

	ui.quickInvDropZone(gridX, gridY, gridSize, gridSize)
	const qbW = 210
	ui.drawTabQuickSlotBar(screen, gridX+(gridSize-qbW)/2, barTop, qbW)

	if tooltip != "" && tooltipHasItem {
		lines := ui.appendCardArtHint(strings.Split(tooltip, "\n"), itemCardKey(tooltipItem))
		plate, titleText := ui.itemTitleColors(tooltipItem)
		var bodyColors []color.Color
		if titleText != nil { // gear keeps its rarity-metal body; spells/traps stay white
			bodyColors = ui.rarityBodyColors(tooltipItem, len(lines))
		}
		ui.queueTitledTooltipIcon(lines, bodyColors, plate, titleText, itemTooltipIconName(tooltipItem), tooltipX, tooltipY)
		if compareTooltip != "" {
			compareLines := strings.Split(compareTooltip, "\n")
			var compareBody []color.Color
			if titleText != nil {
				compareBody = ui.rarityBodyColors(tooltipItem, len(compareLines))
			}
			ui.queueTitledTooltipComparison(compareLines, compareBody, plate, titleText)
		}
	}

	ui.drawInventoryContextMenu(screen)

	instructionY := contentY + contentHeight - 35
	drawDebugText(screen, "Double-click inventory slots to equip/use, equipped slots to unequip", paperX, instructionY)
	drawDebugText(screen, "Right-click an inventory item to discard it. Use 1-4 to switch character.", paperX, instructionY+15)
}

// drawInventoryPager draws the inventory grid's pager. It's a no-op when the
// whole inventory fits on a single page (nothing to flip through). Flipping the
// page breaks any in-flight double-click chain so navigating away and clicking
// the same absolute index doesn't read as a double-click equip/use.
func (ui *UISystem) drawInventoryPager(screen *ebiten.Image, gridX, y, gridW, totalPages int) {
	clickable := !ui.inventoryContextOpen && !ui.inventoryInputBlocked()
	if ui.drawPager(screen, gridX, y, gridW, &ui.inventoryPage, totalPages, clickable) {
		ui.lastClickedItem = -1
		ui.lastClickTime = time.Time{}
	}
}

// drawPager renders a "< Page x/y >" strip with prev/next buttons spanning width
// w at (x,y), flipping *page on click. Shared by the inventory and merchant
// grids. No-op for a single page. Click handling lives here (Draw phase) like
// the rest of the icon-grid widgets. Returns true if the page changed this frame
// so callers can break any double-click chain that a page flip interrupted.
func (ui *UISystem) drawPager(screen *ebiten.Image, x, y, w int, page *int, totalPages int, clickable bool) bool {
	if totalPages <= 1 {
		return false
	}
	const btnW, btnH = 30, 18
	mouseX, mouseY := ebiten.CursorPosition()

	drawBtn := func(bx int, label string, enabled bool) bool {
		bg := color.RGBA{70, 50, 30, 210}
		switch {
		case !enabled:
			bg = color.RGBA{45, 40, 38, 160}
		case isMouseHoveringBox(mouseX, mouseY, bx, y, bx+btnW, y+btnH):
			bg = color.RGBA{120, 90, 50, 230}
		}
		drawFilledRect(screen, bx, y, btnW, btnH, bg)
		drawRectBorder(screen, bx, y, btnW, btnH, 1, color.RGBA{150, 110, 52, 220})
		drawCenteredDebugText(screen, label, bx, y+2, btnW, btnH-2)
		return enabled && clickable && ui.game.consumeLeftClickIn(bx, y, bx+btnW, y+btnH)
	}

	changed := false
	if drawBtn(x, "<", *page > 0) {
		*page--
		changed = true
	}
	if drawBtn(x+w-btnW, ">", *page < totalPages-1) {
		*page++
		changed = true
	}
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/%d", *page+1, totalPages), x, y+2, w, btnH-2)
	return changed
}

const (
	inventoryPaperdollSourceW = 300
	inventoryPaperdollSourceH = 450
	inventoryGridSourceSize   = 300
)

type inventorySourceRect struct {
	x int
	y int
	w int
	h int
}

type inventoryPaperdollSlot struct {
	slot items.EquipSlot
	rect inventorySourceRect
}

// Paper-doll slots: uniform 39×39 so every equipped icon renders at the same
// size. Each rect is centered on the original (variable-sized) slot's center so
// it still lines up with the drawn slot boxes on inventory_paperdoll_panel.
var inventoryPaperdollSlots = []inventoryPaperdollSlot{
	{items.SlotAmulet, inventorySourceRect{44, 40, 39, 39}},
	{items.SlotSpell, inventorySourceRect{218, 40, 39, 39}},
	{items.SlotHelmet, inventorySourceRect{131, 57, 39, 39}},
	{items.SlotMainHand, inventorySourceRect{54, 166, 39, 39}},
	{items.SlotArmor, inventorySourceRect{131, 167, 39, 39}},
	{items.SlotOffHand, inventorySourceRect{207, 166, 39, 39}},
	{items.SlotGauntlets, inventorySourceRect{54, 237, 39, 39}},
	{items.SlotBelt, inventorySourceRect{131, 238, 39, 39}},
	{items.SlotCloak, inventorySourceRect{207, 238, 39, 39}},
	{items.SlotRing1, inventorySourceRect{57, 309, 39, 39}},
	{items.SlotRing2, inventorySourceRect{205, 309, 39, 39}},
	{items.SlotBoots, inventorySourceRect{131, 358, 39, 39}},
}

// Grid slots: uniform 45×45 (the dominant size; were sloppily 44 in column 2
// and the bottom row). Positions kept as authored so they stay on the panel art.
var inventoryGridSlots = []inventorySourceRect{
	{43, 42, 45, 45}, {100, 42, 45, 45}, {156, 42, 45, 45}, {212, 42, 45, 45},
	{43, 99, 45, 45}, {100, 99, 45, 45}, {156, 99, 45, 45}, {212, 99, 45, 45},
	{43, 155, 45, 45}, {100, 155, 45, 45}, {156, 155, 45, 45}, {212, 155, 45, 45},
	{43, 212, 45, 45}, {100, 212, 45, 45}, {156, 212, 45, 45}, {212, 212, 45, 45},
}

func scaleInventorySourceRect(dstX, dstY, dstW, dstH, srcW, srcH int, r inventorySourceRect) (int, int, int, int) {
	x := dstX + int(float64(r.x)*float64(dstW)/float64(srcW))
	y := dstY + int(float64(r.y)*float64(dstH)/float64(srcH))
	w := int(float64(r.w) * float64(dstW) / float64(srcW))
	h := int(float64(r.h) * float64(dstH) / float64(srcH))
	return x, y, w, h
}

// inventoryInputBlocked reports whether a modal popup should swallow inventory
// clicks. The revival picker holds an inventory index across frames, so any
// click that mutates inventory (equip, use, discard) would invalidate it.
func (ui *UISystem) inventoryInputBlocked() bool {
	return ui.game.revivalPickerOpen || ui.game.healPickerOpen || ui.game.statPopupOpen || ui.game.currentLevelUpChoice() != nil
}

func (ui *UISystem) canSelectedCharacterEquipInventoryItem(item items.Item) bool {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	switch item.Type {
	case items.ItemWeapon:
		return currentChar.CanEquipWeaponByName(item.Name)
	case items.ItemArmor:
		return currentChar.CanEquipArmor(item)
	default:
		return true
	}
}

func (ui *UISystem) drawInventoryItemIcon(screen *ebiten.Image, item items.Item, x, y, w, h int, pad int, enabled bool) {
	iconX := x + pad
	iconY := y + pad
	iconW := w - pad*2
	iconH := h - pad*2
	if iconW <= 0 || iconH <= 0 {
		return
	}
	iconSize := iconW
	if iconH < iconSize {
		iconSize = iconH
	}
	iconX += (iconW - iconSize) / 2
	iconY += (iconH - iconSize) / 2
	iconName := itemTooltipIconName(item)
	if iconName != "" && ui.game.sprites.HasSprite(iconName) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(iconName), iconX, iconY, iconSize, iconSize)
	} else {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{22, 18, 24, 210})
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{150, 110, 52, 220})
		drawCenteredDebugText(screen, spellInitials(item.Name), iconX, iconY, iconSize, iconSize)
	}
	if !enabled {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{60, 0, 0, 90})
	}
}

func (ui *UISystem) drawInventoryContextMenu(screen *ebiten.Image) {
	if !ui.inventoryContextOpen {
		return
	}
	menuW := 140
	menuH := 24
	x := ui.inventoryContextX
	y := ui.inventoryContextY
	drawFilledRect(screen, x, y, menuW, menuH, color.RGBA{40, 40, 60, 230})
	drawRectBorder(screen, x, y, menuW, menuH, 2, color.RGBA{120, 120, 160, 255})
	drawCenteredDebugText(screen, "Discard", x, y, menuW, menuH)

	if ui.game.consumeLeftClickIn(x, y, x+menuW, y+menuH) {
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
		ui.inventoryContextOpen = false
	}
}

// drawCharactersContent draws the characters tab content
func (ui *UISystem) drawCharactersContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	// Title
	drawDebugText(screen, "CHARACTER INFO", panelX+20, contentY+10)

	if len(ui.game.party.Members) == 0 {
		drawDebugText(screen, "No party members.", panelX+20, contentY+40)
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

	// All character-sheet text gets a drop shadow so it lifts off the parchment.
	// A local closure shadows the package draw so every call below is covered.
	drawDebugTextColored := drawDebugTextShadowed

	// Character layout — centre the portrait+scroll block within the 700-wide panel.
	const (
		portraitSize = 180
		portraitGap  = 24
		scrollW      = 420
		scrollH      = 330
		blockW       = portraitSize + portraitGap + scrollW
	)
	cardX := panelX + (700-blockW)/2
	cardY := contentY + 40
	portraitX := cardX
	portraitY := cardY + 8
	scrollX := cardX + portraitSize + portraitGap
	scrollY := cardY
	drawNineSlice(screen, ui.game.sprites.GetSprite("character_scroll_panel"), scrollX, scrollY, scrollW, scrollH, 16)

	portraitName := ui.game.fullPortraitSpriteName(member)
	portrait := ui.game.sprites.GetSprite(portraitName)
	portraitFramePad := 6
	drawNineSlice(screen, ui.game.sprites.GetSprite("menu_panel_frame"), portraitX-portraitFramePad, portraitY-portraitFramePad, portraitSize+portraitFramePad*2, portraitSize+portraitFramePad*2, menuPanelFrameSlice)
	drawImageScaled(screen, portrait, portraitX, portraitY, portraitSize, portraitSize)

	// Light text over a dark outline (drawDebugTextShadowed): white body for
	// readable values, warm gold for section headers so they stand out.
	textColor := color.RGBA{240, 240, 240, 255}
	mutedTextColor := color.RGBA{235, 200, 120, 255}
	scrollTextX := scrollX + 26
	scrollTextY := scrollY + 18

	// Header
	header := fmt.Sprintf("%d. %s (%s) Level %d", charIndex+1, member.Name, member.ClassDisplayName(), member.Level)
	drawDebugTextColored(screen, header, scrollTextX, scrollTextY, textColor)

	if ui.characterPage == 1 {
		ui.drawCharacterCombatPage(screen, member, scrollTextX, scrollTextY, textColor, mutedTextColor)
		drawDebugText(screen, "Use 1-4 keys to switch character", cardX, contentY+contentHeight-42)
		ui.drawCharacterPager(screen, scrollX, contentY+contentHeight-22, scrollW)
		return
	}

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
	// Combat runs on EFFECTIVE stats — show them, with the gear/buff delta in
	// brackets so the player sees both ("Might: 18 (+3)").
	effMight, effInt, effPers, effEnd, effAcc, effSpeed, effLuck := member.GetEffectiveStats()
	statLines := []struct {
		name      string
		base, eff int
	}{
		{"Might", member.Might, effMight},
		{"Intellect", member.Intellect, effInt},
		{"Personality", member.Personality, effPers},
		{"Endurance", member.Endurance, effEnd},
		{"Accuracy", member.Accuracy, effAcc},
		{"Speed", member.Speed, effSpeed},
		{"Luck", member.Luck, effLuck},
	}
	for i, stat := range statLines {
		x := col1X
		if i >= statRows {
			x = col2X
		}
		y := statY + (i%statRows)*rowH
		line := fmt.Sprintf("%s: %d", stat.name, stat.eff)
		drawDebugTextColored(screen, line, x, y, textColor)
		if delta := stat.eff - stat.base; delta != 0 {
			clr := color.RGBA{130, 210, 130, 255} // bonus green
			if delta < 0 {
				clr = color.RGBA{215, 120, 110, 255} // debuff red
			}
			drawDebugTextColored(screen, fmt.Sprintf(" (%+d)", delta), x+debugTextWidth(line), y, clr)
		}
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
	skillIdx := 0
	for _, st := range character.AllSkills {
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
		line := fmt.Sprintf("%s %d (%s)",
			school.DisplayName(), ms.Level(), ms.Mastery)
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
	drawDebugText(screen, "Use 1-4 keys to switch character", cardX, contentY+contentHeight-42)
	ui.drawCharacterPager(screen, scrollX, contentY+contentHeight-22, scrollW)

	if tooltip != "" {
		ui.queueTooltip(strings.Split(tooltip, "\n"), tooltipX, tooltipY)
	}
}

func (ui *UISystem) drawCharacterCombatPage(screen *ebiten.Image, member *character.MMCharacter, x, y int, textColor, headingColor color.Color) {
	if member == nil {
		return
	}
	// Drop shadow on all sheet text (see drawCharactersContent).
	drawDebugTextColored := drawDebugTextShadowed

	// Physical mitigation is a PIPELINE, not a single number: combat applies armor %,
	// THEN resistance %, floors at 1, THEN the flat reductions (skill + buff, which
	// CAN finish a hit off to 0). Show the steps in that order; the breakdown comes
	// from CombatSystem so it can't drift from mitigateCharacterDamage.
	m := ui.game.combat.PhysicalMitigationBreakdown(member)

	drawDebugTextColored(screen, "COMBAT TOTALS", x, y+32, headingColor)
	lines := []string{
		fmt.Sprintf("Physical attack bonus: +%d damage", ui.game.combatBuffOutBonusForDamageType("physical")),
		fmt.Sprintf("Total defense (AC): %d", m.ArmorClass),
		fmt.Sprintf("1. Armor mitigation: -%d%% physical (-%d%% elemental)", m.ArmorPct, ui.game.combat.armorMitigationPct(member, false)),
		fmt.Sprintf("2. Physical resistance: -%d%%", m.ResistPct),
		fmt.Sprintf("3. Skill reduction: -%d flat", m.SkillFlat),
		fmt.Sprintf("4. Flat buff reduction: -%d", m.FlatBuff),
	}
	for i, line := range lines {
		drawDebugTextColored(screen, line, x, y+50+i*16, textColor)
	}

	drawDebugTextColored(screen, "RESISTANCES", x, y+158, headingColor)
	buffResist := ui.game.combatBuffResistPct()
	schools := []string{"physical", "fire", "water", "air", "earth", "spirit", "mind", "body", "light", "dark"}
	const colGap = 190
	for i, school := range schools {
		total := member.GearResistPct(school) + buffResist
		if total > 100 {
			total = 100
		}
		colX := x
		if i >= 5 {
			colX += colGap
		}
		rowY := y + 178 + (i%5)*18
		drawDebugTextColored(screen, fmt.Sprintf("%s: %d%%", strings.Title(school), total), colX, rowY, textColor)
	}
	drawDebugTextColored(screen, fmt.Sprintf("Party resist buff: +%d%%", buffResist), x, y+276, headingColor)
}

const pagerBtnW, pagerBtnH = 30, 18

// drawPagerButton draws one prev/next pager button at (bx, y) and reports whether
// it was clicked this frame (only when enabled). Shared by the quest and
// character list pagers.
func (ui *UISystem) drawPagerButton(screen *ebiten.Image, bx, y int, label string, enabled bool) bool {
	mouseX, mouseY := ebiten.CursorPosition()
	bg := color.RGBA{70, 50, 30, 210}
	switch {
	case !enabled:
		bg = color.RGBA{45, 40, 38, 160}
	case isMouseHoveringBox(mouseX, mouseY, bx, y, bx+pagerBtnW, y+pagerBtnH):
		bg = color.RGBA{120, 90, 50, 230}
	}
	drawFilledRect(screen, bx, y, pagerBtnW, pagerBtnH, bg)
	drawRectBorder(screen, bx, y, pagerBtnW, pagerBtnH, 1, color.RGBA{150, 110, 52, 220})
	drawCenteredDebugText(screen, label, bx, y+2, pagerBtnW, pagerBtnH-2)
	return enabled && ui.game.consumeLeftClickIn(bx, y, bx+pagerBtnW, y+pagerBtnH)
}

func (ui *UISystem) drawCharacterPager(screen *ebiten.Image, x, y, width int) {
	if ui.drawPagerButton(screen, x, y, "<", ui.characterPage > 0) {
		ui.characterPage--
	}
	if ui.drawPagerButton(screen, x+width-pagerBtnW, y, ">", ui.characterPage < 1) {
		ui.characterPage++
	}
	drawCenteredDebugText(screen, fmt.Sprintf("Page %d/2", ui.characterPage+1), x, y+2, width, pagerBtnH-2)
}

// drawSpellbookContent draws the spellbook tab content
func (ui *UISystem) drawSpellbookContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	// Trappers (thief) carry a trap book instead of a magic spellbook.
	if hasTrapBook(currentChar) {
		ui.drawTrapBookContent(screen, panelX, contentY, contentHeight)
		return
	}
	schools := spellbookSchoolsWithSpells(currentChar)

	// Validate and fix selected school index if it's out of bounds
	if ui.game.selectedSchool >= len(schools) {
		ui.game.selectedSchool = 0
		ui.game.selectedSpell = 0
	}

	bl := computeBookLayout(panelX, contentY, contentHeight)

	// Draw bookmarks first so the book sprite hides the inserted portion.
	if len(schools) > 0 {
		var selSchool character.MagicSchoolID
		if ui.game.selectedSchool < len(schools) {
			selSchool = schools[ui.game.selectedSchool]
		}
		ui.drawSpellbookSchoolTabs(screen, schools, selSchool, bl.bookX, bl.bookY, bl.scaleX, bl.scaleY)
	}

	drawImageScaled(screen, ui.game.sprites.GetSprite("spellbook_open"), bl.bookX, bl.bookY, bl.bookW, bl.bookH)

	leftTextX := bl.srcX(92)
	leftTextW := bl.srcW(350)

	drawCenteredDebugText(screen, fmt.Sprintf("%s's Spellbook", currentChar.Name), leftTextX, bl.srcY(72), leftTextW, 20)
	// SP counter is shown in the party panel; no need to duplicate it in the book.
	// School name is shown by the active bookmark; no header needed on the right page.

	if len(schools) == 0 {
		drawCenteredDebugText(screen, "No magic schools available", bl.bookX+24, bl.bookY+bl.bookH/2-8, bl.bookW-48, 20)
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
		drawCenteredDebugText(screen, "No learned spells", leftTextX, bl.bookY+bl.bookH/2-8, leftTextW, 20)
	} else {
		mouseX, mouseY := ebiten.CursorPosition()

		for spellIndex, spellID := range schoolSpells {
			if spellIndex >= 2*bl.cardsPerPage {
				break
			}
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue
			}

			cardX, cardY := bl.cardPos(spellIndex)
			if cardY+bl.cardH > bl.gridMaxY {
				continue
			}

			ui.handleSpellbookSpellClick(cardX, cardY, bl.cardW, bl.cardH, ui.game.selectedSchool, spellIndex)
			ui.quickSpellCardDragSource(spellID, cardX, cardY, bl.cardW, bl.cardH)
			isSelected := spellIndex == ui.game.selectedSpell
			isHovering := mouseX >= cardX && mouseX < cardX+bl.cardW && mouseY >= cardY && mouseY < cardY+bl.cardH
			ui.drawSpellbookSpellCard(screen, cardX, cardY, bl.cardW, bl.cardH, bl.iconSize, spellID, def, currentChar, isSelected)

			if isHovering {
				spellTooltip = GetSpellTooltip(spellID, currentChar, ui.game.combat, tooltipDetailHeld())
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
	drawCenteredDebugText(screen, "Up/Down: Navigate  Enter/F: Equip fast spell  Click: Select  Double-click: Cast", bl.bookX+20, contentY+contentHeight-28, bl.bookW-40, 20)

	// Quick-slot bar below the book, narrow + centred so cells stay compact.
	qbW := 360
	ui.drawTabQuickSlotBar(screen, bl.bookX+(bl.bookW-qbW)/2, bl.bookY+bl.bookH+16, qbW)
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
	// Show the cost actually paid (Meditation GM discount applied) so the card
	// and its red "can't afford" outline match what casting will charge.
	cost := def.SpellPointsCost
	if ui.game.combat != nil {
		cost = ui.game.combat.effectiveSpellCost(currentChar, def.SpellPointsCost)
	}
	drawCenteredDebugText(screen, name, x+4, nameY, w-8, debugTextCharHeight)
	drawCenteredDebugText(screen, fmt.Sprintf("SP %d  Lv %d", cost, def.Level), x+4, statsY, w-8, debugTextCharHeight)
	if currentChar.SpellPoints < cost {
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
	if ui.inventoryInputBlocked() {
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
				if item.Attributes["promotes_lich"] > 0 {
					ui.game.useLichPhylactery(itemIndex)
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
			}
			// Spells (ItemBattleSpell/ItemUtilitySpell) are spellbook-owned;
			// they shouldn't appear in inventory, so we don't equip from here.

			// Consume the double-click: reset to a sentinel so a third rapid
			// click starts a fresh pair instead of re-triggering. Equipping
			// shifts the next item into this index, so without this a burst of
			// clicks equips a whole cascade of items.
			ui.lastClickedItem = -1
			ui.lastClickTime = time.Time{}
			return
		}

		ui.lastClickedItem = itemIndex
		ui.lastClickTime = currentTime
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// handleEquippedItemClick handles double-click to unequip items from equipment slots
func (ui *UISystem) handleEquippedItemClick(slot items.EquipSlot, x1, y1, x2, y2 int) {
	if ui.inventoryInputBlocked() {
		return
	}
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
			// Consume the double-click so a third rapid click doesn't re-fire.
			// -1 is not a real slot, so the next click can't pair with it.
			ui.lastClickedSlot = items.EquipSlot(-1)
			ui.lastEquipClickTime = time.Time{}
			return
		}

		ui.lastClickedSlot = slot
		ui.lastEquipClickTime = currentTime
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// drawCampButton renders the Camp button under the inventory grid: spend
// CampFoodCost food to fully restore the party in the field — unless enemies
// are within CampEnemyRadiusTiles (TryCamp refuses). The result line stays
// visible under the button.
func (ui *UISystem) drawCampButton(screen *ebiten.Image, gridX, y, gridW int) {
	const btnW, btnH = 120, 26
	btnX := gridX + (gridW-btnW)/2
	mouseX, mouseY := ebiten.CursorPosition()
	hover := isMouseHoveringBox(mouseX, mouseY, btnX, y, btnX+btnW, y+btnH)

	fill := color.RGBA{30, 45, 30, 255}
	if hover {
		fill = color.RGBA{50, 75, 50, 255}
	}
	drawFilledRect(screen, btnX, y, btnW, btnH, fill)
	drawRectBorder(screen, btnX, y, btnW, btnH, 2, color.RGBA{100, 120, 100, 255})
	drawCenteredDebugText(screen, fmt.Sprintf("Camp (-%d food)", CampFoodCost), btnX, y, btnW, btnH)

	if !ui.inventoryContextOpen && !ui.inventoryInputBlocked() &&
		ui.game.consumeLeftClickIn(btnX, y, btnX+btnW, y+btnH) {
		ui.campNotice, ui.campNoticeOK = ui.game.TryCamp()
	}

	if ui.campNotice != "" {
		clr := color.RGBA{210, 90, 80, 255}
		if ui.campNoticeOK {
			clr = color.RGBA{120, 210, 120, 255}
		}
		noticeX := gridX + (gridW-debugTextWidth(ui.campNotice))/2
		drawDebugTextColored(screen, ui.campNotice, noticeX, y+btnH+6, clr)
	}
}
