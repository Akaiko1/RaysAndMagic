package game

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

// Both bags are always visible. Each has its own page and wheel target; every
// displayed cell captures its physical owner as well as its absolute item index.
func (ui *UISystem) drawInventoryBagGrid(screen *ebiten.Image, owner *character.MMCharacter, grid, pager layoutRect, page *int) {
	g := ui.game
	hero := g.party.Members[g.selectedChar]
	drawImageScaled(screen, g.sprites.GetSprite("inventory_grid_panel"), grid.x, grid.y, grid.w, grid.h)
	selected, _ := ui.inventoryFilterState(owner)
	view := g.inventoryViewIndices(*selected, owner)
	totalPages := pageCount(len(view), len(inventoryGridSlots))
	clampPage(page, totalPages)
	bag := g.party.Bag(owner)
	mx, my := pointerPosition()
	for slot, source := range inventoryGridSlots {
		idx := inventoryCellIndex(view, *page, len(inventoryGridSlots), slot)
		x, y, w, h := scaleInventorySourceRect(grid.x, grid.y, grid.w, grid.h, inventoryGridLayoutSize, inventoryGridLayoutSize, source)
		hover := isMouseHoveringBox(mx, my, x, y, x+w, y+h)
		if idx < 0 || idx >= len(bag.Items()) {
			if !ui.inventoryContextOpen {
				if g.dragActive && hover {
					drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, color.RGBA{210, 170, 80, 230})
				}
				ui.inventoryEmptyDropZone(x, y, w, h, owner)
			}
			continue
		}
		item := bag.Items()[idx]
		canEquip := ui.canSelectedCharacterEquipInventoryItem(item)
		if !canEquip {
			drawFilledRect(screen, x, y, w, h, color.RGBA{120, 28, 28, 95})
		}
		if hover {
			border := color.RGBA{210, 170, 80, 230}
			if !canEquip {
				border = color.RGBA{190, 70, 60, 230}
			}
			drawRectBorder(screen, x-2, y-2, w+4, h+4, 2, border)
		}
		dragging := g.dragActive && g.dragSrc == dragFromInventory && g.dragInvOwner == owner && g.dragInvIndex == idx && g.dragSplitQuantity == 0
		if !dragging {
			ui.drawInventoryItemIcon(screen, item, x, y, w, h, max(1, w/16), canEquip)
		}
		if !ui.inventoryContextOpen {
			ui.handleInventoryItemClick(idx, x-3, y-3, x+w+3, y+h+3, owner)
			ui.quickInvSlotDragSource(idx, x, y, w, h, owner)
			ui.inventoryCellDropZone(idx, x, y, w, h, owner)
		}
		ui.onDisplayedInput(uiCommandClick, layoutRect{x - 3, y - 3, w + 6, h + 6}, func() {
			if !ui.inventoryContextOpen && !ui.inventoryInputBlocked() && g.consumeRightClickIn(x-3, y-3, x+w+3, y+h+3) {
				ui.inventoryContextOpen = true
				ui.inventoryContextX, ui.inventoryContextY = g.mouseRightClickX, g.mouseRightClickY
				ui.inventoryContextIndex, ui.inventoryContextOwner = idx, owner
			}
		})
		if hover {
			key := itemCardKey(item)
			ui.fullArtCardKey = key
			lines := ui.appendCardArtHintRows(GetItemTooltipRows(item, hero, g.combat, tooltipDetailHeld()), key)
			ui.queueItemTooltip(lines, item, hero, mx+16, my+8)
			if compare := GetItemComparisonTooltipRows(item, hero, g.combat); len(compare) > 0 {
				lines := compare
				var body []color.Color
				if item.Type == items.ItemWeapon || item.Type == items.ItemArmor || item.Type == items.ItemAccessory {
					body = equipmentComparisonColors(lines.Lines(), body)
				}
				plate, title := ui.itemTitleColors(item)
				ui.queueCardComparison(lines, body, plate, title)
			}
		}
	}
	ui.quickInvDropZone(grid.x, grid.y, grid.w, grid.h, owner)
	clickable := !ui.inventoryContextOpen && !ui.inventoryInputBlocked()
	resetClick := func() { ui.lastClickedItem = -1; ui.lastClickTime = time.Time{} }
	ui.drawPager(screen, pager.x, pager.y, pager.w, page, totalPages, clickable, resetClick)
	ui.onDisplayedInput(uiCommandPointer, grid, func() {
		mx, my := pointerPosition()
		_, wheel := pointerWheel()
		if !clickable || wheel == 0 || !isMouseHoveringBox(mx, my, grid.x, grid.y, grid.right(), grid.bottom()) {
			return
		}
		step := 1
		if wheel > 0 {
			step = -1
		}
		next := max(0, min(totalPages-1, *page+step))
		if next != *page {
			*page = next
			resetClick()
		}
	})
}

func (ui *UISystem) transferInventoryDrag(owner *character.MMCharacter) {
	g := ui.game
	switch g.dragSrc {
	case dragFromInventory:
		if owner == g.dragInvOwner {
			return
		}
		if it, ok := g.takeInventoryDragItem(); ok {
			g.party.Bag(owner).Add(it)
		}
	case dragFromQuickSlot:
		ch := g.party.Members[g.dragQuickChar]
		if it := ch.QuickSlots[g.dragQuickSlot]; it != nil {
			g.returnQuickItemToInventory(*it, owner)
			ch.QuickSlots[g.dragQuickSlot] = nil
		}
	case dragFromEquip:
		g.unequipPartyItemToInventory(g.dragEquipSlot, g.dragEquipChar, owner)
	default:
		return
	}
	g.clearDrag()
}

func (ui *UISystem) inventoryMoveTarget(owner *character.MMCharacter) (*character.MMCharacter, string) {
	if owner != nil {
		return nil, "Move to shared bag"
	}
	return ui.game.party.Members[ui.game.selectedChar], "Move to personal bag"
}

// Context actions retain their source even when the selected hero changes.
func (ui *UISystem) inventoryContextBag() character.InventoryBag {
	return ui.game.party.Bag(ui.inventoryContextOwner)
}
func inventoryBagUnits(b character.InventoryBag) int {
	n := 0
	for _, it := range b.Items() {
		n += it.Count()
	}
	return n
}
