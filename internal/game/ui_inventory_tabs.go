package game

import (
	"image/color"

	"ugataima/internal/character"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

// Inventory category tabs: one independent filter strip per bag. A tab owns an
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
	{label: "Consumables", types: []items.ItemType{items.ItemConsumable, items.ItemTrap, items.ItemDevice}},
	{label: "Quest", types: []items.ItemType{items.ItemQuest}},
	{label: "Loot"},
}

const (
	inventoryTabAll = 0
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
// drag, context menu) addresses its physical bag directly, so a filtered cell
// must hand them the real index or it acts on the wrong item.
func (g *MMGame) inventoryViewIndices(tab int, owner ...*character.MMCharacter) []int {
	return inventoryTabView(tab, g.party.Bag(owner...).Items())
}

// inventoryTabShows is the one category filter: bags, shop shelves and the
// shop's view of the party bag all ask it.
func inventoryTabShows(tab int, item items.Item) bool {
	return tab == inventoryTabAll || inventoryTabOwning(item) == tab
}

// inventoryTabView lists the positions in bag that tab shows, in bag order.
func inventoryTabView(tab int, bag []items.Item) []int {
	view := make([]int, 0, len(bag))
	for i, item := range bag {
		if inventoryTabShows(tab, item) {
			view = append(view, i)
		}
	}
	return view
}

// selectCategory switches a surface's category filter and returns to its
// first page; false when tab is invalid or already selected.
func selectCategory(selected, page *int, tab int) bool {
	if tab < 0 || tab >= len(inventoryTabs) || tab == *selected {
		return false
	}
	*selected, *page = tab, 0
	return true
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

// inventoryFilterState addresses only the shared or selected personal view.
func (ui *UISystem) inventoryFilterState(owner ...*character.MMCharacter) (*int, *int) {
	if len(owner) > 0 && owner[0] != nil {
		return &ui.personalInventoryTab, &ui.personalInventoryPage
	}
	return &ui.inventoryTab, &ui.inventoryPage
}

// A filter change invalidates its own page and pending item clicks. The other
// bag keeps both its filter and page, including while it is a transfer target.
func (ui *UISystem) setInventoryTab(tab int, owner ...*character.MMCharacter) {
	selected, page := ui.inventoryFilterState(owner...)
	if !selectCategory(selected, page, tab) {
		return
	}
	ui.lastClickedItem = -1
	ui.inventoryContextOpen = false
	ui.inventoryContextIndex = -1
}

func inventoryFilterHeight(w int) int { return max(20, min(48, (w-20)/6)) }

// Six equal targets stay centered over their own bag at every window size.
func inventoryTabRects(x, y, w int) []layoutRect {
	side := inventoryFilterHeight(w)
	const gap = 4
	x += (w - len(inventoryTabs)*side - (len(inventoryTabs)-1)*gap) / 2
	rects := make([]layoutRect, len(inventoryTabs))
	for i := range rects {
		rects[i] = layoutRect{x + i*(side+gap), y, side, side}
	}
	return rects
}

var inventoryFilterIcons = [...]string{"", "icon_weapon_iron_sword", "icon_item_leather_armor", "icon_item_health_potion", "icon_item_world_map", "icon_item_wolf_pelt"}

func (ui *UISystem) drawInventoryTabs(screen *ebiten.Image, x, y, w int, owner ...*character.MMCharacter) {
	clickable := !ui.inventoryContextOpen && !ui.inventoryInputBlocked()
	selected, _ := ui.inventoryFilterState(owner...)
	ui.drawItemCategoryTabs(screen, x, y, w, *selected, clickable, func(tab int) { ui.setInventoryTab(tab, owner...) })
}

// drawItemCategoryTabs shares the inventory's category vocabulary, art and hit
// geometry with merchant shelves; each surface owns its selection and paging.
func (ui *UISystem) drawItemCategoryTabs(screen *ebiten.Image, x, y, w, selected int, clickable bool, selectTab func(int)) {
	mx, my := pointerPosition()
	for i, r := range inventoryTabRects(x, y, w) {
		active := i == selected
		// Hover belongs to the live surface: under a modal the strip is decoration.
		hover := clickable && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawDialogTab(screen, r, active)
		if i == inventoryTabAll {
			drawCenteredUIText(screen, "All", r.x, r.y, r.w, r.h)
		} else {
			icon := inventoryFilterIcons[i]
			pad := max(4, r.w/8)
			drawImageScaled(screen, ui.game.sprites.GetSprite(icon), r.x+pad, r.y+pad, r.w-2*pad, r.h-2*pad)
		}
		if active {
			drawFilledRect(screen, r.x+5, r.bottom()-3, r.w-10, 2, color.RGBA{231, 194, 112, 255})
		}
		if hover {
			drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, color.RGBA{213, 178, 110, 255})
			ui.queueTooltip([]string{inventoryTabs[i].label}, mx+12, my+12)
		}
		ui.onDisplayedInput(uiCommandNavigation, r, func() {
			if clickable && ui.game.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
				selectTab(i)
			}
		})
	}
}
