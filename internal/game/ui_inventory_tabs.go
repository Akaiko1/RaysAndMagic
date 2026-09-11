package game

import (
	"image/color"

	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

// Inventory category tabs: one filter strip over the party bag. A tab owns an
// item by its AUTHORED items.yaml type - never by name - and the last tab is the
// declared catch-all, so no item can end up invisible on every tab.

type inventoryTab struct {
	label string
	types []items.ItemType // empty on the All tab and on the catch-all
}

// inventoryTabs is the strip, left to right. All comes first, Loot last: it
// claims every type the tabs before it do not (trinkets, cards, and any type
// added later).
var inventoryTabs = []inventoryTab{
	{label: "All"},
	{label: "Weapons", types: []items.ItemType{items.ItemWeapon}},
	{label: "Armor", types: []items.ItemType{items.ItemArmor, items.ItemAccessory}},
	{label: "Consumables", types: []items.ItemType{items.ItemConsumable, items.ItemTrap}},
	{label: "Quest", types: []items.ItemType{items.ItemQuest}},
	{label: "Loot"},
}

const (
	inventoryTabAll = 0
	inventoryTabH   = 18
	inventoryTabPad = 8 // total horizontal padding inside one tab
)

// inventoryTabCatchAll is the last tab - it claims every type no earlier tab does.
var inventoryTabCatchAll = len(inventoryTabs) - 1

// inventoryTabOwning returns the tab that shows this item: the first tab whose
// type set claims it, else the catch-all.
func inventoryTabOwning(item items.Item) int {
	for i, tab := range inventoryTabs {
		for _, t := range tab.types {
			if item.Type == t {
				return i
			}
		}
	}
	return inventoryTabCatchAll
}

// inventoryViewIndices lists the ABSOLUTE bag indices the given tab shows, in
// bag order. Absolute is the contract: every grid handler (equip, use, discard,
// drag, context menu) addresses party.Inventory directly, so a filtered cell
// must hand them the real index or it acts on the wrong item.
func (g *MMGame) inventoryViewIndices(tab int) []int {
	bag := g.party.Inventory
	view := make([]int, 0, len(bag))
	for i, item := range bag {
		if tab == inventoryTabAll || inventoryTabOwning(item) == tab {
			view = append(view, i)
		}
	}
	return view
}

// inventoryCellIndex maps a grid cell on the current page to its ABSOLUTE bag
// index, or -1 for an empty cell. Shared by the draw loop and everything it
// wires up, so the icon drawn in a cell is the item its handlers act on.
func inventoryCellIndex(view []int, page, pageSize, slot int) int {
	pos := page*pageSize + slot
	if pos < 0 || pos >= len(view) {
		return -1
	}
	return view[pos]
}

// setInventoryTab switches the filter. The page, the double-click chain and the
// context menu all address the OLD view, so they are dropped with it.
func (ui *UISystem) setInventoryTab(tab int) {
	if tab == ui.inventoryTab {
		return
	}
	ui.inventoryTab = tab
	ui.inventoryPage = 0
	ui.lastClickedItem = -1
	ui.inventoryContextOpen = false
	ui.inventoryContextIndex = -1
}

// inventoryTabRects lays the strip across width w: each tab is as wide as its
// label needs, and the slack is shared out so the strip spans the grid.
func inventoryTabRects(x, y, w int) []layoutRect {
	widths := make([]int, len(inventoryTabs))
	total := 0
	for i, tab := range inventoryTabs {
		widths[i] = debugTextWidth(tab.label) + inventoryTabPad
		total += widths[i]
	}
	if slack := w - total; slack > 0 {
		share := slack / len(widths)
		for i := range widths {
			widths[i] += share
		}
		widths[len(widths)-1] += slack - share*len(widths)
	}
	rects := make([]layoutRect, len(widths))
	cursor := x
	for i, tw := range widths {
		rects[i] = layoutRect{cursor, y, tw, inventoryTabH}
		cursor += tw
	}
	return rects
}

// drawInventoryTabs renders the strip and owns its clicks, like the pager and
// the dialog folder tabs (Draw-phase widgets).
func (ui *UISystem) drawInventoryTabs(screen *ebiten.Image, x, y, w int) {
	clickable := !ui.inventoryContextOpen && !ui.inventoryInputBlocked()
	mouseX, mouseY := ebiten.CursorPosition()
	for i, r := range inventoryTabRects(x, y, w) {
		active := i == ui.inventoryTab
		bg := color.RGBA{45, 38, 30, 200}
		switch {
		case active:
			bg = color.RGBA{120, 90, 50, 235}
		case isMouseHoveringBox(mouseX, mouseY, r.x, r.y, r.right(), r.bottom()):
			bg = color.RGBA{80, 62, 38, 220}
		}
		drawFilledRect(screen, r.x, r.y, r.w, r.h, bg)
		drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, color.RGBA{150, 110, 52, 220})
		label := color.RGBA{232, 222, 190, 255}
		if !active {
			label = color.RGBA{170, 160, 140, 255}
		}
		text := clipDebugText(inventoryTabs[i].label, r.w-2)
		textX, textY := centeredTextPos(text, r.x, r.y, r.w, r.h)
		drawDebugTextColored(screen, text, textX, textY, label)
		ui.onDisplayedInput(uiCommandClick, layoutRect{r.x, r.y, (r.right()) - (r.x), (r.bottom()) - (r.y)}, func() {
			if clickable && ui.game.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
				ui.setInventoryTab(i)
			}
		})
	}
}
