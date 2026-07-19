package game

import (
	"fmt"
	"image"
	"image/color"

	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
)

type stackSplitPickerSource uint8

const (
	stackSplitPickerInventory stackSplitPickerSource = iota + 1
	stackSplitPickerStash
)

// stackSplitModifierHeld is the quick one-unit transfer shortcut shared by the
// inventory quick bar and the stash. Exact quantities use the picker below.
func stackSplitModifierHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// stackSplitPickerState holds only UI-transient selection state. The actual
// item mutation remains in Party.TakeStackUnits / Item.SplitOff at drop time.
type stackSplitPickerState struct {
	open     bool
	source   stackSplitPickerSource
	from     int // inventory index or encoded stash source
	quantity int
	name     string
	itemType items.ItemType
	id       uint64
}

const (
	stackSplitPickerW = 280
	stackSplitPickerH = 144
)

func stackSplitPickerRect(screenW, screenH int) image.Rectangle {
	x := (screenW - stackSplitPickerW) / 2
	y := (screenH - stackSplitPickerH) / 2
	return image.Rect(x, y, x+stackSplitPickerW, y+stackSplitPickerH)
}

func (ui *UISystem) openStackSplitPicker(source stackSplitPickerSource, from int, item items.Item) {
	if !item.Stackable() || item.Count() < 2 {
		return
	}
	quantity := item.Count() / 2
	if quantity < 1 {
		quantity = 1
	}
	ui.stackSplitPicker = stackSplitPickerState{
		open: true, source: source, from: from, quantity: quantity,
		name: item.Name, itemType: item.Type, id: item.InstanceID,
	}
	ui.inventoryContextOpen = false
}

func (ui *UISystem) closeStackSplitPicker() {
	ui.stackSplitPicker = stackSplitPickerState{}
}

// stackSplitInteractionActive exposes the UI-owned transient state to input
// handling. Keyboard navigation must not switch tabs, close the panel or move
// the party while a modal picker or its carried fragment owns the next click.
func (g *MMGame) stackSplitInteractionActive() bool {
	if g == nil || g.gameLoop == nil || g.gameLoop.ui == nil {
		return false
	}
	return g.gameLoop.ui.stackSplitPicker.open || g.dragPickedUp || g.stashDragPickedUp
}

func (g *MMGame) cancelStackSplitInteraction() {
	if g == nil {
		return
	}
	if g.gameLoop != nil && g.gameLoop.ui != nil {
		g.gameLoop.ui.closeStackSplitPicker()
	}
	if g.dragPickedUp {
		g.clearDrag()
	}
	if g.stashDragPickedUp {
		g.clearStashDrag()
	}
}

// stackSplitPickerItem resolves and validates the source held by the picker.
// No other bag mutation is allowed while the picker is open, but validating the
// item identity still makes stale UI state harmless.
func (ui *UISystem) stackSplitPickerItem() (items.Item, bool) {
	s := ui.stackSplitPicker
	if !s.open {
		return items.Item{}, false
	}
	g := ui.game
	var item items.Item
	switch s.source {
	case stackSplitPickerInventory:
		if s.from < 0 || s.from >= len(g.party.Inventory) {
			return items.Item{}, false
		}
		item = g.party.Inventory[s.from]
	case stackSplitPickerStash:
		src, ok := decodeStashFrom(s.from)
		if !ok {
			return items.Item{}, false
		}
		if src.kind == stashKindBag {
			if src.idx < 0 || src.idx >= len(g.party.Inventory) {
				return items.Item{}, false
			}
			item = g.party.Inventory[src.idx]
		} else {
			cell := g.stashCellPtr(src)
			if cell == nil || stash.IsEmpty(*cell) {
				return items.Item{}, false
			}
			item = *cell
		}
	default:
		return items.Item{}, false
	}
	if !item.Stackable() || item.Count() < 2 || item.Name != s.name || item.Type != s.itemType ||
		(s.id != 0 && item.InstanceID != s.id) {
		return items.Item{}, false
	}
	return item, true
}

func (ui *UISystem) beginPickedUpStackSplit(item items.Item) {
	s := ui.stackSplitPicker
	quantity := s.quantity
	if quantity < 1 || quantity >= item.Count() {
		ui.closeStackSplitPicker()
		return
	}
	item.Quantity = quantity
	ui.closeStackSplitPicker()
	g := ui.game
	x, y := ebiten.CursorPosition()
	switch s.source {
	case stackSplitPickerInventory:
		g.clearDrag()
		g.dragActive = true
		g.dragPickedUp = true
		g.dragSrc = dragFromInventory
		g.dragInvIndex = s.from
		g.dragSplitQuantity = quantity
		g.dragItem = item
		g.dragCurX, g.dragCurY = x, y
	case stackSplitPickerStash:
		g.clearStashDrag()
		g.stashDragActive = true
		g.stashDragPickedUp = true
		g.stashDragFrom = s.from
		g.stashDragSplitQuantity = quantity
		g.stashDragItem = item
		g.stashDragCurX, g.stashDragCurY = x, y
	}
}

func drawStackSplitButton(screen *ebiten.Image, r image.Rectangle, label string, hovered bool) {
	bg := color.RGBA{70, 50, 30, 230}
	if hovered {
		bg = color.RGBA{120, 90, 50, 240}
	}
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), bg)
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 1, color.RGBA{170, 130, 70, 235})
	drawCenteredDebugText(screen, label, r.Min.X, r.Min.Y+2, r.Dx(), r.Dy()-2)
}

func (ui *UISystem) drawStackSplitPicker(screen *ebiten.Image) {
	item, ok := ui.stackSplitPickerItem()
	if !ok {
		ui.closeStackSplitPicker()
		return
	}
	g := ui.game
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	r := stackSplitPickerRect(screenW, screenH)
	drawFilledRect(screen, 0, 0, screenW, screenH, color.RGBA{0, 0, 0, 125})
	drawFilledRect(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{30, 30, 60, 248})
	drawRectBorder(screen, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 2, color.RGBA{170, 130, 70, 235})
	drawCenteredDebugText(screen, "Split "+truncateRunes(item.Name, 28, "..."), r.Min.X+12, r.Min.Y+12, r.Dx()-24, 16)

	minus := image.Rect(r.Min.X+24, r.Min.Y+48, r.Min.X+56, r.Min.Y+72)
	plus := image.Rect(r.Min.X+224, r.Min.Y+48, r.Min.X+256, r.Min.Y+72)
	half := image.Rect(r.Min.X+178, r.Min.Y+48, r.Min.X+216, r.Min.Y+72)
	take := image.Rect(r.Min.X+24, r.Min.Y+102, r.Min.X+136, r.Min.Y+128)
	cancel := image.Rect(r.Min.X+144, r.Min.Y+102, r.Min.X+256, r.Min.Y+128)
	mouseX, mouseY := ebiten.CursorPosition()
	drawStackSplitButton(screen, minus, "-", ptInRect(mouseX, mouseY, minus))
	drawStackSplitButton(screen, half, "1/2", ptInRect(mouseX, mouseY, half))
	drawStackSplitButton(screen, plus, "+", ptInRect(mouseX, mouseY, plus))
	drawStackSplitButton(screen, take, "Pick up", ptInRect(mouseX, mouseY, take))
	drawStackSplitButton(screen, cancel, "Cancel", ptInRect(mouseX, mouseY, cancel))
	drawCenteredDebugText(screen, fmt.Sprintf("x%d of x%d", ui.stackSplitPicker.quantity, item.Count()), r.Min.X+64, r.Min.Y+53, 104, 18)

	if g.consumeLeftClickIn(minus.Min.X, minus.Min.Y, minus.Max.X, minus.Max.Y) {
		if ui.stackSplitPicker.quantity > 1 {
			ui.stackSplitPicker.quantity--
		}
		return
	}
	if g.consumeLeftClickIn(plus.Min.X, plus.Min.Y, plus.Max.X, plus.Max.Y) {
		if ui.stackSplitPicker.quantity < item.Count()-1 {
			ui.stackSplitPicker.quantity++
		}
		return
	}
	if g.consumeLeftClickIn(half.Min.X, half.Min.Y, half.Max.X, half.Max.Y) {
		ui.stackSplitPicker.quantity = item.Count() / 2
		if ui.stackSplitPicker.quantity < 1 {
			ui.stackSplitPicker.quantity = 1
		}
		return
	}
	if g.consumeLeftClickIn(take.Min.X, take.Min.Y, take.Max.X, take.Max.Y) {
		ui.beginPickedUpStackSplit(item)
		return
	}
	if g.consumeLeftClickIn(cancel.Min.X, cancel.Min.Y, cancel.Max.X, cancel.Max.Y) || g.consumeLeftClick() {
		ui.closeStackSplitPicker()
	}
}
