package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stats"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawInventoryContent draws the inventory tab content
func (ui *UISystem) drawInventoryContent(screen *ebiten.Image, content layoutRect) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	mouseX, mouseY := pointerPosition()

	layout := computeInventoryContentLayout(content)
	paperX, paperY, paperW, paperH := layout.paper.x, layout.paper.y, layout.paper.w, layout.paper.h

	drawLabel := func(label string, r layoutRect, tint color.RGBA) {
		text := clipUIText(label, (r.w-8)/layout.textScale-4)
		drawScaledMetalCenteredText(screen, text, r.x+r.w/2, r.y+r.h/2, float64(layout.textScale), tint)
		ui.offerClippedTextTooltip([]string{label}, text != label, r.x, r.y, r.w, r.h)
	}
	for i, title := range []string{currentChar.Name, "Personal bag", "Shared bag"} {
		drawLabel(title, layout.headings[i], rarityGold)
	}
	for i, label := range []string{fmt.Sprintf("Gold %d", ui.game.party.Gold), fmt.Sprintf("Food %d", ui.game.party.Food)} {
		drawLabel(label, layout.resources[i], color.RGBA{255, 255, 255, 255})
	}

	drawImageScaled(screen, ui.game.sprites.GetSprite("inventory_paperdoll_panel"), paperX, paperY, paperW, paperH)

	var tooltip character.CardRows
	var tooltipItem items.Item
	var tooltipHasItem bool
	var tooltipX, tooltipY int

	for _, slotInfo := range inventoryPaperdollSlots {
		x, y, size := scaleInventorySourceSquare(paperX, paperY, paperW, paperH, inventoryPaperdollSourceW, inventoryPaperdollSourceH, slotInfo.rect)
		w, h := size, size
		item, equipped := currentChar.Equipment[slotInfo.slot]
		isHovering := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)
		// While dragging an equippable item, glow every slot it can go into - from
		// the bag, or from another paperdoll slot (a ring to its other finger). The
		// source slot itself never glows.
		if ui.game.drag.active && equipItemMatchesSlot(currentChar, ui.game.dragItem, slotInfo.slot) &&
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
			dragging := ui.game.drag.active && ui.game.dragSrc == dragFromEquip &&
				ui.game.dragEquipChar == ui.game.selectedChar && ui.game.dragEquipSlot == slotInfo.slot
			if !dragging {
				// The complete authored icon square belongs on top of the paperdoll.
				// Do not crop, mask, or realign the artwork inside that square.
				ui.drawInventoryItemIcon(screen, item, x, y, w, h, 0, true)
			}
			ui.handleEquippedItemClick(slotInfo.slot, x-3, y-3, x+w+3, y+h+3)
			ui.equipSlotDragSource(ui.game.selectedChar, slotInfo.slot, item, x, y, w, h) // pick up equipped item
			if isHovering {
				tooltip = GetItemTooltipRows(item, currentChar, ui.game.combat, tooltipDetailHeld())
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

	ui.drawInventoryTabs(screen, layout.categories.x, layout.categories.y, layout.categories.w)
	ui.drawInventoryTabs(screen, layout.personalCategories.x, layout.personalCategories.y, layout.personalCategories.w, currentChar)
	ui.drawInventoryBagGrid(screen, currentChar, layout.personalGrid, layout.personalPager, &ui.personalInventoryPage)
	ui.drawInventoryBagGrid(screen, nil, layout.grid, layout.pager, &ui.inventoryPage)
	drawCenteredUIText(screen, "Quick slots", layout.quickSlots.x, layout.quickSlots.y-quickSlotTabLabelSpace, layout.quickSlots.w, quickSlotTabLabelH)
	ui.drawQuickSlotBar(screen, ui.game.selectedChar, layout.quickSlots.x, layout.quickSlots.y, layout.quickSlots.w, !ui.modalLayerOwnsInput())

	if len(tooltip) > 0 && tooltipHasItem {
		lines := ui.appendCardArtHintRows(tooltip, itemCardKey(tooltipItem))
		ui.queueItemTooltip(lines, tooltipItem, currentChar, tooltipX, tooltipY)
	}

	ui.drawInventoryContextMenu(screen)

}

// drawPager renders a "< Page x/y >" strip with prev/next buttons spanning width
// w at (x,y), flipping *page on click. Shared by the inventory and merchant
// grids. No-op for a single page. It registers navigation commands against the
// displayed page; onChange runs in Update after a successful flip.
func (ui *UISystem) drawPager(screen *ebiten.Image, x, y, w int, page *int, totalPages int, clickable bool, onChange ...func()) {
	if totalPages <= 1 {
		return
	}
	mouseX, mouseY := uiCursorPosition()

	drawBtn := func(r layoutRect, label string, enabled bool, step int) {
		// Hover lights only on the live layer, as the click does.
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, enabled && clickable && isMouseHoveringBox(mouseX, mouseY, r.x, r.y, r.right(), r.bottom()))
		if !enabled {
			drawFilledRect(screen, r.x+2, r.y+2, r.w-4, r.h-4, color.RGBA{0, 0, 0, 100})
		}
		drawCenteredUIText(screen, label, r.x, r.y+2, r.w, r.h-2)
		ui.onClick(r, enabled && clickable, func() {
			*page = max(0, min(totalPages-1, *page+step))
			for _, change := range onChange {
				change()
			}
		})
	}

	prev, next := pagerButtonRects(x, y, w)
	drawBtn(prev, "<", *page > 0, -1)
	drawBtn(next, ">", *page < totalPages-1, 1)

	drawCenteredUIText(screen, fmt.Sprintf("Page %d/%d", *page+1, totalPages), x, y+2, w, pagerBtnH-2)
}

// pagerButtonRects are drawPager's Prev and Next buttons for a strip at (x,y)
// spanning w.
func pagerButtonRects(x, y, w int) (prev, next layoutRect) {
	return layoutRect{x, y, pagerBtnW, pagerBtnH}, layoutRect{x + w - pagerBtnW, y, pagerBtnW, pagerBtnH}
}

const (
	inventoryPaperdollSourceW = 683
	inventoryPaperdollSourceH = 1024
	inventoryPaperdollLayoutW = 300
	inventoryPaperdollLayoutH = 450
	// The high-resolution grid PNG is authored against this normalized logical
	// canvas. Keeping its children in layout pixels avoids replacing working
	// click geometry when the raster source is re-exported at a higher density.
	inventoryGridLayoutSize = 300
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

// Paperdoll item rectangles cover the complete repeated slot artwork. Their
// coordinates are measured independently from the production image; the source
// art deliberately uses one identical 118x118 master slot in every position.
var inventoryPaperdollSlots = []inventoryPaperdollSlot{
	{items.SlotAmulet, inventorySourceRect{112, 138, 118, 118}},
	{items.SlotQuickAction, inventorySourceRect{452, 138, 118, 118}},
	{items.SlotHelmet, inventorySourceRect{282, 136, 118, 118}},
	{items.SlotMainHand, inventorySourceRect{92, 402, 118, 118}},
	{items.SlotArmor, inventorySourceRect{282, 360, 118, 118}},
	{items.SlotOffHand, inventorySourceRect{470, 402, 118, 118}},
	{items.SlotGauntlets, inventorySourceRect{92, 572, 118, 118}},
	{items.SlotBelt, inventorySourceRect{282, 514, 118, 118}},
	{items.SlotCloak, inventorySourceRect{470, 572, 118, 118}},
	{items.SlotRing1, inventorySourceRect{110, 742, 118, 118}},
	{items.SlotRing2, inventorySourceRect{452, 742, 118, 118}},
	{items.SlotBoots, inventorySourceRect{282, 830, 118, 118}},
}

// Grid slots use the normalized 300x300 layout canvas above. The one-pixel
// offsets are measured from the current 1024px raster's dark recess centres;
// bag icons retain their separate 4px inset at draw time.
var inventoryGridSlots = []inventorySourceRect{
	{42, 42, 45, 45}, {99, 42, 45, 45}, {155, 42, 45, 45}, {212, 42, 45, 45},
	{42, 99, 45, 45}, {99, 99, 45, 45}, {155, 99, 45, 45}, {212, 99, 45, 45},
	{42, 156, 45, 45}, {99, 156, 45, 45}, {155, 156, 45, 45}, {212, 156, 45, 45},
	{42, 213, 45, 45}, {99, 213, 45, 45}, {155, 213, 45, 45}, {212, 213, 45, 45},
}

func scaleInventorySourceRect(dstX, dstY, dstW, dstH, srcW, srcH int, r inventorySourceRect) (int, int, int, int) {
	x := dstX + int(float64(r.x)*float64(dstW)/float64(srcW))
	y := dstY + int(float64(r.y)*float64(dstH)/float64(srcH))
	w := int(float64(r.w) * float64(dstW) / float64(srcW))
	h := int(float64(r.h) * float64(dstH) / float64(srcH))
	return x, y, w, h
}

// scaleInventorySourceSquare keeps an authored square centered after scaling.
// Rounding its center and size once avoids the drift caused by independently
// truncating the left edge, top edge, width, and height.
func scaleInventorySourceSquare(dstX, dstY, dstW, dstH, srcW, srcH int, r inventorySourceRect) (int, int, int) {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 || r.w <= 0 || r.h <= 0 {
		return dstX, dstY, 0
	}
	scaleX := float64(dstW) / float64(srcW)
	scaleY := float64(dstH) / float64(srcH)
	size := max(1, int(math.Round(float64(min(r.w, r.h))*min(scaleX, scaleY))))
	centerX := float64(dstX) + (float64(r.x)+float64(r.w)/2)*scaleX
	centerY := float64(dstY) + (float64(r.y)+float64(r.h)/2)*scaleY
	x := int(math.Round(centerX - float64(size)/2))
	y := int(math.Round(centerY - float64(size)/2))
	return x, y, size
}

// inventoryHardBlocked reports modal state that forbids even a picked-up split
// fragment. inventoryInputBlocked additionally blocks ordinary inventory clicks
// while that fragment is waiting for its destination.
func (ui *UISystem) inventoryHardBlocked() bool {
	return ui.modalLayerOwnsInput()
}

// inventoryInputBlocked reports whether a modal popup or picked-up fragment
// should swallow ordinary inventory clicks. The revival picker holds an
// inventory index across frames, so any click that mutates inventory (equip,
// use, discard) would invalidate it.
func (ui *UISystem) inventoryInputBlocked() bool {
	return ui.inventoryHardBlocked() || ui.game.dragPickedUp
}

func (ui *UISystem) canSelectedCharacterEquipInventoryItem(item items.Item) bool {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	if item.Type != items.ItemWeapon && item.Type != items.ItemArmor && item.Type != items.ItemAccessory {
		return true
	}
	slot, ok := currentChar.EquipDestination(item)
	return ok && currentChar.ItemFitsSlot(item, slot)
}

func (ui *UISystem) drawInventoryItemIcon(screen *ebiten.Image, item items.Item, x, y, w, h int, pad int, enabled bool) {
	var actor *character.MMCharacter
	if g := ui.game; g.party != nil && g.selectedChar >= 0 && g.selectedChar < len(g.party.Members) {
		actor = g.party.Members[g.selectedChar]
	}
	ui.drawItemIcon(screen, item, x, y, w, h, pad, enabled, false, actor)
}

func (ui *UISystem) drawItemIcon(screen *ebiten.Image, item items.Item, x, y, w, h int, pad int, enabled, showSingleCount bool, actor *character.MMCharacter) {
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
		drawCenteredUIText(screen, spellInitials(item.Name), iconX, iconY, iconSize, iconSize)
	}
	if item.Type == items.ItemBattleSpell || item.Type == items.ItemUtilitySpell {
		ui.drawSpellReuse(screen, actor, spells.SpellID(item.SpellEffect), iconX, iconY, iconSize)
	}
	if item.Type == items.ItemDevice {
		drawDeviceCooldown(screen, item, iconX, iconY, iconSize)
	}
	if !enabled {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{60, 0, 0, 90})
	}
	// Stack count, bottom-right. Every icon surface (bag, quick slots,
	// merchant grids, stash) shares this renderer, so stacks read the same
	// everywhere.
	n := item.Count()
	if item.Type == items.ItemThrowable {
		n = ui.game.flaskStock(actor, item.Flask)
	}
	if n > 1 || item.Type == items.ItemThrowable || showSingleCount && item.Stackable() {
		// Use the existing metal text at native size, independent of the icon
		// scale. Its shadow follows the digits; there is no backing rectangle.
		glyph := outlinedLabelImage(fmt.Sprint(n), itemCountMetal)
		gw, gh := glyph.Bounds().Dx(), glyph.Bounds().Dy()
		drawImageScaled(screen, glyph, iconX+iconSize-gw, iconY+iconSize-gh, gw, gh)
	}
}

// itemDiscardable: quest items are undiscardable unless the item itself opts
// out via `discardable: true` (e.g. the Lich Phylactery - a choice the player
// may refuse and toss).
func itemDiscardable(item items.Item) bool {
	return item.Type != items.ItemQuest || item.Attributes["discardable"] > 0
}

func (ui *UISystem) drawInventoryContextMenu(screen *ebiten.Image) {
	if !ui.inventoryContextOpen {
		return
	}

	bag := ui.inventoryContextBag()
	idx := ui.inventoryContextIndex
	if idx < 0 || idx >= len(bag.Items()) {
		ui.inventoryContextOpen = false
		return
	}
	item := bag.Items()[idx]
	canSplit := item.Stackable() && item.Count() > 1
	target, moveLabel := ui.inventoryMoveTarget(ui.inventoryContextOwner)
	labels := []string{"Discard"}
	if canSplit {
		labels = append(labels, "Split...")
	}
	labels = append(labels, moveLabel)
	menuW, menuH := 166, 24*len(labels)
	x := min(ui.inventoryContextX, uiBounds(screen).Dx()-menuW-4)
	y := min(ui.inventoryContextY, uiBounds(screen).Dy()-menuH-4)
	ui.drawThemeFrame(screen, frameSilver, x, y, menuW, menuH)
	for i, label := range labels {
		drawCenteredUIText(screen, label, x, y+i*24, menuW, 24)
	}
	if ui.inventoryInputBlocked() {
		return
	}
	ui.onDisplayedInput(uiCommandClick, layoutRect{}, func() {
		switch {
		case ui.game.consumeLeftClickIn(x, y, x+menuW, y+24):
			if !itemDiscardable(item) {
				ui.game.AddCombatMessage(uitext.Text("combat.cannot_discard", item.Name))
			} else {
				bag.Consume(idx, 1)
				ui.game.AddCombatMessage(uitext.Text("combat.discarded", item.Name))
			}
			ui.inventoryContextOpen = false
		case ui.game.consumeLeftClickIn(x, y+(len(labels)-1)*24, x+menuW, y+len(labels)*24):
			bag.MoveTo(ui.game.party.Bag(target), idx, item.Count())
			ui.inventoryContextOpen = false
		case canSplit && ui.game.consumeLeftClickIn(x, y+24, x+menuW, y+48):
			ui.openStackSplitPicker(stackSplitPickerInventory, idx, item, ui.inventoryContextOwner)
		case ui.game.consumeLeftClick():
			ui.inventoryContextOpen = false
		}
	})
}

// drawCharactersContent shows the complete selected-character record in one
// fullscreen dashboard. No fixed row budget is allowed to hide authored skills.
func (ui *UISystem) drawCharactersContent(screen *ebiten.Image, content layoutRect) {
	layout := computeCharacterContentLayout(content)
	drawCenteredUIText(screen, "CHARACTER OVERVIEW", layout.title.x, layout.title.y, layout.title.w, layout.title.h)
	if len(ui.game.party.Members) == 0 {
		drawUIText(screen, "No party members.", layout.title.x, layout.title.y+24)
		return
	}

	charIndex := ui.game.selectedChar
	if charIndex < 0 || charIndex >= len(ui.game.party.Members) {
		charIndex = 0
	}
	member := ui.game.party.Members[charIndex]
	if member == nil {
		return
	}
	mouseX, mouseY := uiCursorPosition()
	var tooltip character.CardRows
	var tooltipX, tooltipY int
	textColor := color.RGBA{240, 240, 240, 255}
	headingColor := color.RGBA{235, 200, 120, 255}
	const (
		sectionTextInset = 18
		sectionTitleY    = 16
		sectionBodyY     = 37
		sectionRowH      = 16
	)

	drawSection := func(r layoutRect, title string) {
		ui.drawPatternFrame(screen, "menu_panel_slot", r.x, r.y, r.w, r.h, menuPanelFrameSlice)
		drawUITextColored(screen, title, r.x+sectionTextInset, r.y+sectionTitleY, headingColor)
	}
	drawSection(layout.profile, "PROFILE")
	drawSection(layout.attributes, "ATTRIBUTES")
	drawSection(layout.magic, "MAGIC SCHOOLS")
	drawSection(layout.skills, "SKILLS")
	drawSection(layout.combat, "COMBAT AND RESISTANCES")

	portraitName := ui.game.fullPortraitSpriteName(member)
	drawPortraitFrame(screen, layout.portraitFrame.x, layout.portraitFrame.y, layout.portraitFrame.w, layout.portraitFrame.h)
	drawImageScaled(screen, ui.game.sprites.GetSprite(portraitName), layout.portrait.x, layout.portrait.y, layout.portrait.w, layout.portrait.h)
	profileX := layout.profile.x + sectionTextInset
	profileW := layout.profile.w - 2*sectionTextInset
	profileY := layout.portraitFrame.bottom() + 10
	profileLines := []string{
		member.Name,
		member.ClassDisplayName(),
		fmt.Sprintf("Level: %d", member.Level),
		fmt.Sprintf("Health: %d/%d", member.HitPoints, member.MaxHitPoints),
		fmt.Sprintf("Spell Points: %d/%d", member.SpellPoints, member.MaxSpellPoints),
		// Experience is the remainder toward the next level; show its target.
		fmt.Sprintf("XP: %d / %d", member.Experience, xpStepCost(member.Level)),
	}
	for i, line := range profileLines {
		lineColor := textColor
		if i < 2 {
			lineColor = headingColor
		}
		drawUITextColored(screen, clipUIText(line, profileW), profileX, profileY+i*16, lineColor)
	}
	statusText := "Status: Normal"
	if full, _ := ui.game.partyConditionLabels(member); full != "OK" {
		statusText = "Status: " + full // the same list the party card shows
	}
	for i, line := range wrapUIText(statusText, profileW) {
		drawUITextColored(screen, line, profileX, profileY+(len(profileLines)+i)*16, textColor)
	}

	base, eff := member.BaseStats(), member.EffectiveStatBlock()
	attributeRowH := min(sectionRowH, (layout.attributes.h-sectionBodyY-8)/len(stats.Names))
	for i, name := range stats.Names {
		x := layout.attributes.x + sectionTextInset
		y := layout.attributes.y + sectionBodyY + i*attributeRowH
		line := fmt.Sprintf("%s: %d", stats.Title(name), *eff.At(i))
		drawUITextColored(screen, clipUIText(line, layout.attributes.w-2*sectionTextInset), x, y, textColor)
		if delta := *eff.At(i) - *base.At(i); delta != 0 {
			clr := color.RGBA{130, 210, 130, 255}
			if delta < 0 {
				clr = color.RGBA{215, 120, 110, 255}
			}
			drawUITextColored(screen, fmt.Sprintf(" (%+d)", delta), x+uiTextWidth(line), y, clr)
		}
		if len(tooltip) == 0 && isMouseHoveringBox(mouseX, mouseY, x, y, layout.attributes.right()-sectionTextInset, y+sectionRowH) {
			tooltip = statTooltipRows(name)
			tooltipX, tooltipY = mouseX+16, mouseY+8
		}
	}

	type schoolLine struct {
		id   character.MagicSchoolID
		text string
	}
	var schools []schoolLine
	for _, school := range character.AllMagicSchools {
		if ms := member.MagicSchools[school]; ms != nil {
			schools = append(schools, schoolLine{school, fmt.Sprintf("%s %d (%s)", school.DisplayName(), ms.Level(), ms.Mastery)})
		}
	}
	if len(schools) == 0 {
		drawUITextColored(screen, "None", layout.magic.x+sectionTextInset, layout.magic.y+sectionBodyY, textColor)
	} else {
		rows := max(1, (layout.magic.h-sectionBodyY-12)/sectionRowH)
		cols := max(1, (len(schools)+rows-1)/rows)
		colW := (layout.magic.w - 2*sectionTextInset) / cols
		for i, school := range schools {
			x := layout.magic.x + sectionTextInset + (i/rows)*colW
			y := layout.magic.y + sectionBodyY + (i%rows)*sectionRowH
			drawUITextColored(screen, clipUIText(school.text, colW-8), x, y, textColor)
			if len(tooltip) == 0 && isMouseHoveringBox(mouseX, mouseY, x, y, x+colW-8, y+16) {
				tooltip = magicMasteryTooltipRows(school.id)
				tooltipX, tooltipY = mouseX+16, mouseY+8
			}
		}
	}

	type skillLine struct {
		id   character.SkillType
		text string
	}
	var skillsList []skillLine
	for _, skillID := range character.AllSkills {
		if skill := member.Skills[skillID]; skill != nil {
			text := skillID.String()
			if skillID.UsesMastery() {
				text = fmt.Sprintf("%s %d (%s)", skillID.String(), skill.Level(), skill.Mastery.String())
			}
			skillsList = append(skillsList, skillLine{skillID, text})
		}
	}
	if len(skillsList) == 0 {
		drawUITextColored(screen, "None", layout.skills.x+sectionTextInset, layout.skills.y+sectionBodyY, textColor)
	} else {
		rows := max(1, (layout.skills.h-sectionBodyY-12)/sectionRowH)
		cols := max(1, (len(skillsList)+rows-1)/rows)
		colW := (layout.skills.w - 2*sectionTextInset) / cols
		for i, skill := range skillsList {
			x := layout.skills.x + sectionTextInset + (i/rows)*colW
			y := layout.skills.y + sectionBodyY + (i%rows)*sectionRowH
			drawUITextColored(screen, clipUIText(skill.text, colW-8), x, y, textColor)
			if len(tooltip) == 0 && isMouseHoveringBox(mouseX, mouseY, x, y, x+colW-8, y+16) {
				tooltip = masteryTooltipRowsForSkill(skill.id)
				tooltipX, tooltipY = mouseX+16, mouseY+8
			}
		}
	}

	m := ui.game.combat.PhysicalMitigationBreakdown(member)
	combatX := layout.combat.x + sectionTextInset
	combatW := layout.combat.w - 2*sectionTextInset
	combatGap := 14
	leftW := max(1, combatW*60/100)
	rightX := combatX + leftW + combatGap
	rightW := max(1, layout.combat.right()-sectionTextInset-rightX)

	weaponLine := func(label string, slot items.EquipSlot, cooldownFrames int) string {
		weapon, ok := member.Equipment[slot]
		if !ok {
			return label + ": None"
		}
		if weapon.Type != items.ItemWeapon {
			return label + ": " + weapon.Name // a shield or another non-weapon
		}
		preview := ui.game.combat.calculateWeaponDamagePreview(weapon, member)
		crit := ui.game.combat.CalculateWeaponCritChance(weapon, member)
		tps := max(1, ui.game.config.GetTPS())
		return fmt.Sprintf("%s: %d dmg, %d%% crit, %.1fs CD", label, preview.Total, crit, float64(cooldownFrames)/float64(tps))
	}

	leftLines := []struct {
		text    string
		heading bool
		details []string
		lines   []string
	}{
		{text: "OFFENSE", heading: true},
		{text: fmt.Sprintf("Physical attack bonus: +%d", ui.game.combatBuffOutBonusForDamageType(damagecalc.Physical.String()))},
		{text: fmt.Sprintf("Universal crit: %d%%", ui.game.combat.totalCriticalChance(0, member))},
		{text: weaponLine("Main hand", items.SlotMainHand, ui.game.combat.WeaponCooldownFrames(member))},
		{text: weaponLine("Off hand", items.SlotOffHand, ui.game.combat.OffHandWeaponCooldownFrames(member))},
		{text: "DEFENSE", heading: true},
		{text: fmt.Sprintf("Armor Class: %d", m.ArmorClass)},
		{text: fmt.Sprintf("Armor: -%d%% physical / -%d%% other", m.ArmorPct, ui.game.combat.armorMitigationPct(member, false))},
		{text: fmt.Sprintf("Perfect Dodge: %d%%", ui.game.combat.PerfectDodgeChance(member))},
		{text: fmt.Sprintf("Physical resistance: -%d%%", m.ResistPct)},
		{text: uitext.Text("damage.flat_reduction_summary", m.SkillFlat+m.FlatBuff), details: []string{
			uitext.Text("damage.flat_reduction_sources", m.SkillFlat, m.FlatBuff),
			uitext.Text("damage.flat_reduction_rules"),
		}},
		// The strongest reflector guards every hero, so the sheet shows the party value.
		{text: fmt.Sprintf("Projectile reflection (party): %d%%", ui.game.partyProjectileReflectPct())},
		{text: fmt.Sprintf("Melee thorns: %d%% phys / %d%% fire", ui.game.cardThornsPct()+weaponThornsPct(member), member.SetFieryRipostePct())},
		{text: fmt.Sprintf("Status duration: %d%% / stun %d%%", clampHostileStatusDurationPct(member.ItemStatusDurationPct()), clampHostileStatusDurationPct(member.ItemStatusDurationPct()+member.SetStunDurationPct()))},
	}
	combatRows := 0
	for i := range leftLines {
		line := &leftLines[i]
		line.lines = []string{clipUIText(line.text, leftW)}
		if len(line.details) > 0 {
			line.lines = wrapUIText(line.text, leftW)
		}
		combatRows += len(line.lines)
	}
	combatRowH := min(sectionRowH, (layout.combat.h-sectionBodyY-8)/combatRows)
	combatY := layout.combat.y + sectionBodyY
	for _, line := range leftLines {
		lineColor := textColor
		if line.heading {
			lineColor = headingColor
		}
		for i, text := range line.lines {
			drawUITextColored(screen, text, combatX, combatY+i*combatRowH, lineColor)
		}
		rowH := len(line.lines) * combatRowH
		if len(tooltip) == 0 && len(line.details) > 0 && isMouseHoveringBox(mouseX, mouseY, combatX, combatY, combatX+leftW, combatY+rowH) {
			tooltip = character.PlainCardRows(append([]string{line.text}, line.details...))
			tooltipX, tooltipY = mouseX+16, mouseY+8
		}
		combatY += rowH
	}

	resistY := layout.combat.y + sectionBodyY
	drawUITextColored(screen, "RESISTANCES", rightX, resistY, headingColor)
	resistY += sectionRowH
	resistTypes := damagecalc.Types()
	for i, school := range resistTypes {
		y := resistY + i*sectionRowH
		line := fmt.Sprintf("%s: %d%%", config.TitleWords(school.String()), ui.game.schoolResistPct(member, school.String()))
		drawUITextColored(screen, clipUIText(line, rightW), rightX, y, textColor)
	}
	partyBuffY := resistY + len(resistTypes)*sectionRowH + 5
	drawUITextColored(screen, clipUIText(fmt.Sprintf("Party resist buff: +%d%%", ui.game.combatBuffResistPct()), rightW), rightX, partyBuffY, headingColor)

	drawCenteredUIText(screen, "Use the party strip or keys 1-4 to switch character", layout.instructions.x, layout.instructions.y, layout.instructions.w, layout.instructions.h)
	if len(tooltip) > 0 {
		ui.queueCardTooltip(tooltip, nil, nil, nil, "", tooltipX, tooltipY)
	}
}

const pagerBtnW, pagerBtnH = 30, 18

// drawSpellbookContent draws the spellbook tab content. The parchment spread
// contains spell cards only; identity, navigation, controls and quick slots
// live in the surrounding fullscreen hub.
func (ui *UISystem) drawSpellbookContent(screen *ebiten.Image, content layoutRect) {
	if ui.drawRareClassBook(screen, content) {
		return
	}
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	// Trappers (thief) carry a trap book instead of a magic spellbook.
	if hasTrapBook(currentChar) {
		ui.drawTrapBookContent(screen, content)
		return
	}
	schools := spellbookSchoolsWithSpells(currentChar)

	// Validate and fix selected school index if it's out of bounds
	if ui.game.selectedSchool < 0 || ui.game.selectedSchool >= len(schools) {
		ui.game.selectedSchool = 0
		ui.game.selectedSpell = -1
	}

	bl := computeBookLayout(content)
	header := fmt.Sprintf("%s - Spellbook", currentChar.Name)
	if ui.game.selectedSchool >= 0 && ui.game.selectedSchool < len(schools) {
		header += " - " + schools[ui.game.selectedSchool].DisplayName() + " Magic"
	}
	drawCenteredUIText(screen, header, bl.header.x, bl.header.y, bl.header.w, bl.header.h)

	// Draw bookmarks first so the book sprite hides the inserted portion.
	if len(schools) > 0 {
		ui.drawSpellbookSchoolTabs(screen, schools, ui.game.selectedSchool, bl)
	}

	drawImageScaled(screen, ui.game.sprites.GetSprite("spellbook_open"), bl.bookX, bl.bookY, bl.bookW, bl.bookH)

	if len(schools) == 0 {
		drawCenteredUIText(screen, "No magic schools available", bl.bookX+24, bl.bookY+bl.bookH/2-8, bl.bookW-48, 20)
		return
	}

	var spellTooltip character.CardRows
	var spellCompareTooltip character.CardRows
	var spellTooltipID spells.SpellID
	var tooltipX, tooltipY int

	if ui.game.selectedSchool < 0 || ui.game.selectedSchool >= len(schools) {
		return
	}

	selectedSchool := schools[ui.game.selectedSchool]
	schoolSpells := currentChar.GetSpellsForSchool(selectedSchool)
	if ui.game.selectedSpell >= len(schoolSpells) {
		ui.game.selectedSpell = 0
	}
	perSpread := bl.cardsPerSpread()
	totalPages := pageCount(len(schoolSpells), perSpread)
	if ui.game.selectedSpell >= 0 {
		ui.spellPage = ui.game.selectedSpell / perSpread
	}
	clampPage(&ui.spellPage, totalPages)

	if len(schoolSpells) == 0 {
		drawCenteredUIText(screen, "No learned spells", bl.bookX+24, bl.bookY+bl.bookH/2-8, bl.bookW-48, 20)
	} else {
		mouseX, mouseY := uiCursorPosition()
		pageStart := ui.spellPage * perSpread
		pageEnd := min(len(schoolSpells), pageStart+perSpread)
		for spellIndex := pageStart; spellIndex < pageEnd; spellIndex++ {
			spellID := schoolSpells[spellIndex]
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue
			}

			cardX, cardY := bl.cardPos(spellIndex - pageStart)
			if cardY+bl.cardH > bl.gridMaxY {
				continue
			}

			ui.spellAutocastControl(screen, currentChar, def, cardX, cardY, bl.cardW, bl.iconSize)
			ui.handleSpellbookSpellClick(cardX, cardY, bl.cardW, bl.cardH, ui.game.selectedSchool, spellIndex)
			ui.onDisplayedInput(uiCommandDrag, layoutRect{cardX, cardY, bl.cardW, bl.cardH}, func() {
				// Checkbox ownership also excludes the press from the drag phase,
				// which runs before queued click handlers consume the same gesture.
				if def.HasAutocast() {
					box := spellAutocastRect(cardX, cardY, bl.cardW, bl.iconSize)
					if isMouseHoveringBox(ui.game.drag.startX, ui.game.drag.startY, box.x, box.y, box.right(), box.bottom()) {
						return
					}
				}
				ui.quickSpellCardDragSource(spellID, cardX, cardY, bl.cardW, bl.cardH)
			})
			isSelected := spellIndex == ui.game.selectedSpell
			isHovering := mouseX >= cardX && mouseX < cardX+bl.cardW && mouseY >= cardY && mouseY < cardY+bl.cardH
			if def.HasAutocast() {
				b := spellAutocastRect(cardX, cardY, bl.cardW, bl.iconSize)
				isHovering = isHovering && !isMouseHoveringBox(mouseX, mouseY, b.x, b.y, b.right(), b.bottom())
			}
			ui.drawSpellbookSpellCard(screen, cardX, cardY, bl.cardW, bl.cardH, bl.iconSize, spellID, def, currentChar, selectedSchool, isSelected)

			if isHovering {
				spellTooltip = GetSpellTooltipRows(spellID, currentChar, ui.game.combat, tooltipDetailHeld())
				spellCompareTooltip = GetSpellComparisonTooltipRows(spellID, currentChar, ui.game.combat)
				spellTooltipID = spellID
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}
		}
	}

	// Draw spell tooltip if hovering over a spell
	if len(spellTooltip) > 0 {
		lines := spellTooltip
		plate := color.Color(nil)
		if def, err := spells.GetSpellDefinitionByID(spellTooltipID); err == nil {
			plate = schoolPlateColor(def.School)
		}
		ui.queueCardTooltip(lines, nil, plate, nil, spellTooltipIconName(spellTooltipID), tooltipX, tooltipY)
		if len(spellCompareTooltip) > 0 {
			compareLines := spellCompareTooltip
			ui.queueCardComparison(compareLines, nil, plate, nil)
		}
	}

	ui.drawPager(screen, bl.pager.x, bl.pager.y, bl.pager.w, &ui.spellPage, totalPages, !ui.modalLayerOwnsInput(), func() {
		ui.game.selectedSpell = -1
	})
	ui.drawTabQuickSlotBar(screen, bl.quick.x, bl.quick.y, bl.quick.w)
	drawCenteredUIText(screen, bookControlsHint, bl.controls.x, bl.controls.y, bl.controls.w, bl.controls.h)
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

func (ui *UISystem) drawSpellbookSchoolTabs(screen *ebiten.Image, schools []character.MagicSchoolID, selectedSchool int, bl bookLayout) {
	if len(schools) == 0 {
		return
	}
	for i, tab := range bl.schoolTabRects(len(schools), selectedSchool) {
		school := schools[i]
		ui.handleSpellbookSchoolClick(tab, i, school)
		drawImageScaled(screen, ui.game.sprites.GetSprite("spellbook_tab_"+school.String()), tab.x, tab.y, tab.w, tab.h)
	}
}

func (ui *UISystem) drawSpellbookSpellCard(screen *ebiten.Image, x, y, w, h, iconSize int, spellID spells.SpellID, def spells.SpellDefinition, currentChar *character.MMCharacter, school character.MagicSchoolID, selected bool) {
	iconX := x + (w-iconSize)/2
	iconY := y + 6
	iconName := spellTooltipIconName(spellID)
	if ui.game.sprites.HasSprite(iconName) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(iconName), iconX, iconY, iconSize, iconSize)
	} else {
		drawFilledRect(screen, iconX, iconY, iconSize, iconSize, color.RGBA{42, 32, 45, 255})
		drawRectBorder(screen, iconX, iconY, iconSize, iconSize, 1, color.RGBA{218, 170, 72, 255})
		drawCenteredUIText(screen, spellInitials(def.Name), iconX, iconY, iconSize, iconSize)
	}

	ui.drawSpellReuse(screen, currentChar, spellID, iconX, iconY, iconSize)
	name := def.Name
	nameY := y + iconSize + 8
	statsY := nameY + uiTextCharHeight + 2
	// Show the cost actually paid (Meditation GM discount applied) so the card
	// and its red "can't afford" outline match what casting will charge.
	cost := currentChar.MeditatedSPCost(def.SpellPointsCost)
	// The selection glow extends below the icon; labels draw over it.
	quick, equipped := currentChar.Equipment[items.SlotQuickAction]
	drawBookEntryState(screen, layoutRect{iconX, iconY, iconSize, iconSize},
		selected, equipped && string(quick.SpellEffect) == string(spellID), false, currentChar.SpellPoints >= cost, SchoolColor(school.String()))
	drawCenteredUIText(screen, name, x+4, nameY, w-8, uiTextCharHeight)
	costX, costW := x+4, w-8
	if def.HasAutocast() {
		box := spellAutocastRect(x, y, w, iconSize)
		costX = x + (x + w - box.right())
		costW = box.x - costX - 4
	}
	drawCenteredUIText(screen, fmt.Sprintf("SP %d", cost), costX, statsY, costW, uiTextCharHeight)
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
func (ui *UISystem) handleInventoryItemClick(itemIndex int, x1, y1, x2, y2 int, owner ...*character.MMCharacter) {
	ui.onGatedClick(layoutRect{x1, y1, x2 - x1, y2 - y1}, func() bool { return !ui.inventoryInputBlocked() }, func() {
		// Only a fast second click acts; a pause leaves this as selection. The
		// pair resets, or a burst of clicks would equip the whole cascade of
		// items that shifts into this index.
		bagOwner := ui.game.party.Bag(owner...).Owner
		if ui.bagClicks.click(bagClickKey{itemIndex, bagOwner}, ui.game.mouseLeftClickAt) {
			ui.game.useBagItem(itemIndex, bagOwner)
		}
	})
}

// handleEquippedItemClick handles double-click to unequip items from equipment slots
func (ui *UISystem) handleEquippedItemClick(slot items.EquipSlot, x1, y1, x2, y2 int) {
	ui.onGatedClick(layoutRect{x1, y1, x2 - x1, y2 - y1}, func() bool { return !ui.inventoryInputBlocked() }, func() {
		// Only a fast second click acts; a pause leaves this as selection.
		if !ui.equipClicks.click(slot, ui.game.mouseLeftClickAt) {
			return
		}
		currentChar := ui.game.party.Members[ui.game.selectedChar]
		if item, exists := currentChar.Equipment[slot]; exists {
			itemName := item.Name
			if ui.game.unequipPartyItemToInventory(slot, ui.game.selectedChar, currentChar) {
				ui.game.AddCombatMessage(uitext.Text("combat.unequipped", currentChar.Name, itemName))
			} else {
				ui.game.AddCombatMessage(uitext.Text("combat.cannot_unequip_this_item"))
			}
		}
	})
}
