package game

import (
	"fmt"
	"image"
	"image/color"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Quick slots: a per-character 5-cell pocket for mouse-driven quick use. While a
// character tab (inventory/spellbook) is open the bar is shown below the panel
// and accepts drag-and-drop (items from the bag, spells from the book). In game
// the bar floats above the party cards and double-click uses the slot:
//   weapon -> equip/swap, potion -> drink one, spell -> cast.
// Independent of the Space/SmartAttack quick-spell (see character.QuickSlots doc).

const (
	quickSlotBarSprite = "quick_slots_bar"
	quickSlotBarAspect = 2027.0 / 458.0 // source frame w/h
	quickDragThreshold = 5              // px of movement before a press becomes a drag
)

// quickSlotCellFrac is each cell as a CENTER + square side (fractions of the
// frame sprite). The side is sized to cover the gold window frame so an opaque
// item icon drawn here REPLACES that window's frame (no double border); empty
// slots simply show the frame sprite's ornate window. Measured from
// quick_slots_bar.png (cx evenly spaced, cy centred, side ~ gold-window width).
var quickSlotCellFrac = [character.QuickSlotCount]struct{ cx, cy, side float64 }{
	{0.1357, 0.499, 0.164},
	{0.3172, 0.499, 0.164},
	{0.4988, 0.499, 0.164},
	{0.6803, 0.499, 0.164},
	{0.8619, 0.499, 0.164},
}

type dragSource int

const (
	dragNone dragSource = iota
	dragFromInventory
	dragFromQuickSlot
	dragFromSpell
	dragFromTrap
	dragFromEquip // an equipped item dragged off the paperdoll
)

// quickSlotRects returns the bar height and the 5 cell rects for a bar drawn at
// (barX, barY) with width barW.
func quickSlotRects(barX, barY, barW int) (barH int, slots [character.QuickSlotCount]image.Rectangle) {
	barH = int(float64(barW) / quickSlotBarAspect)
	side := int(quickSlotCellFrac[0].side * float64(barW))
	for i, f := range quickSlotCellFrac {
		cx := barX + int(f.cx*float64(barW))
		cy := barY + int(f.cy*float64(barH))
		slots[i] = image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2)
	}
	return
}

func ptInRect(x, y int, r image.Rectangle) bool {
	return x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y
}

// drawQuickSlotBar renders the frame + the character's slot icons at (barX,barY)
// width barW, and (while the menu is open) wires each cell as a drag source /
// drop target. Returns the slot rects so callers can add their own click logic.
func (ui *UISystem) drawQuickSlotBar(screen *ebiten.Image, charIdx, barX, barY, barW int) [character.QuickSlotCount]image.Rectangle {
	_, slots := quickSlotRects(barX, barY, barW)
	barH := int(float64(barW) / quickSlotBarAspect)
	drawImageScaled(screen, ui.game.sprites.GetSprite(quickSlotBarSprite), barX, barY, barW, barH)

	ch := ui.game.party.Members[charIdx]
	mouseX, mouseY := ebiten.CursorPosition()
	for i := 0; i < character.QuickSlotCount; i++ {
		r := slots[i]
		ui.quickSlotCellInteract(charIdx, i, r)
		item := ch.QuickSlots[i]
		// Right-click a spell/trap in the bar to bind it as the Space quick-spell
		// (the single quick slot); the item stays in the bar.
		if item != nil && (item.Type == items.ItemBattleSpell || item.Type == items.ItemUtilitySpell || item.Type == items.ItemTrap) &&
			ui.game.consumeRightClickIn(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) {
			ui.game.bindQuickSpellFromPanel(charIdx, *item)
		}
		// Hide the icon of the cell the cursor is currently carrying out of.
		dragging := ui.game.dragActive && ui.game.dragSrc == dragFromQuickSlot &&
			ui.game.dragQuickChar == charIdx && ui.game.dragQuickSlot == i
		if item != nil && !dragging {
			// pad 0: the opaque icon fills the cell and covers the window's gold
			// frame, so a filled slot shows the item's own frame, not two.
			ui.drawInventoryItemIcon(screen, *item, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 0, true)
		}
		if ptInRect(mouseX, mouseY, r) {
			drawRectBorder(screen, r.Min.X-2, r.Min.Y-2, r.Dx()+4, r.Dy()+4, 2, color.RGBA{210, 170, 80, 230})
		}
	}
	return slots
}

// quickSlotCellInteract captures a drag that began on this cell and resolves a
// drop landing on it. Only active while a tab is open (the in-game bar is
// double-click only).
func (ui *UISystem) quickSlotCellInteract(charIdx, slotIdx int, r image.Rectangle) {
	g := ui.game
	if !g.menuOpen {
		return
	}
	if g.dragArmed && g.dragSrc == dragNone &&
		ptInRect(g.dragStartX, g.dragStartY, r) && g.party.Members[charIdx].QuickSlots[slotIdx] != nil {
		g.dragSrc = dragFromQuickSlot
		g.dragItem = *g.party.Members[charIdx].QuickSlots[slotIdx]
		g.dragQuickChar = charIdx
		g.dragQuickSlot = slotIdx
	}
	if g.dragDropAt == 1 && g.dragSrc != dragNone && ptInRect(g.dragCurX, g.dragCurY, r) {
		g.resolveQuickSlotDrop(charIdx, slotIdx)
	}
}

// quickInvSlotDragSource captures an inventory grid cell as a drag source.
func (ui *UISystem) quickInvSlotDragSource(invIndex, x, y, w, h int) {
	g := ui.game
	if !g.menuOpen || !g.dragArmed || g.dragSrc != dragNone {
		return
	}
	if invIndex < 0 || invIndex >= len(g.party.Inventory) {
		return
	}
	if ptInRect(g.dragStartX, g.dragStartY, image.Rect(x, y, x+w, y+h)) {
		g.dragSrc = dragFromInventory
		g.dragInvIndex = invIndex
		g.dragItem = g.party.Inventory[invIndex]
	}
}

// quickSpellCardDragSource captures a spellbook card as a drag source.
func (ui *UISystem) quickSpellCardDragSource(spellID spells.SpellID, x, y, w, h int) {
	g := ui.game
	if !g.menuOpen || !g.dragArmed || g.dragSrc != dragNone {
		return
	}
	if ptInRect(g.dragStartX, g.dragStartY, image.Rect(x, y, x+w, y+h)) {
		if sp, err := spells.CreateSpellItem(spellID); err == nil {
			g.dragSrc = dragFromSpell
			g.dragSpellID = spellID
			g.dragItem = sp
		}
	}
}

// quickTrapCardDragSource captures a trap-book recipe card as a drag source
// (trapper parity with spells - a trap recipe is book-owned, like a spell).
func (ui *UISystem) quickTrapCardDragSource(key string, x, y, w, h int) {
	g := ui.game
	if !g.menuOpen || !g.dragArmed || g.dragSrc != dragNone {
		return
	}
	if ptInRect(g.dragStartX, g.dragStartY, image.Rect(x, y, x+w, y+h)) {
		if it, ok := config.TrapItem(key); ok {
			g.dragSrc = dragFromTrap
			g.dragTrapKey = key
			g.dragItem = it
		}
	}
}

// bindQuickSpellFromPanel binds a spell/trap sitting in the quick-slot BAR (the
// panel) as the character's single Space quick-spell (Equipment[SlotSpell]),
// leaving the item in the bar. Right-click entry point for the bar cells.
func (g *MMGame) bindQuickSpellFromPanel(charIdx int, it items.Item) {
	if charIdx < 0 || charIdx >= len(g.party.Members) {
		return
	}
	ch := g.party.Members[charIdx]
	switch it.Type {
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		ch.Equipment[items.SlotSpell] = it
		g.AddCombatMessage(fmt.Sprintf("%s set as %s's quick spell", it.Name, ch.Name))
	case items.ItemTrap:
		if equipTrap(ch, string(it.SpellEffect)) {
			g.AddCombatMessage(fmt.Sprintf("%s armed in %s's quick slot", it.Name, ch.Name))
		} else {
			g.AddCombatMessage(fmt.Sprintf("%s can't arm %s yet", ch.Name, it.Name))
		}
	}
}

// dragOver reports whether an in-flight drag is being released over the given
// rect - the shared guard for every drop zone.
func (g *MMGame) dragOver(x, y, w, h int) bool {
	return g.menuOpen && g.dragDropAt == 1 && g.dragSrc != dragNone &&
		ptInRect(g.dragCurX, g.dragCurY, image.Rect(x, y, x+w, y+h))
}

// quickInvDropZone resolves a quick-slot item dropped back onto the inventory grid.
func (ui *UISystem) quickInvDropZone(x, y, w, h int) {
	g := ui.game
	if !g.dragOver(x, y, w, h) {
		return
	}
	if g.dragSrc == dragFromQuickSlot {
		sch := g.party.Members[g.dragQuickChar]
		if it := sch.QuickSlots[g.dragQuickSlot]; it != nil {
			g.returnQuickItemToInventory(*it)
			sch.QuickSlots[g.dragQuickSlot] = nil
		}
	}
	// Equipped item dropped anywhere on the grid -> unequip its OWNER (the char it
	// was dragged from, not the possibly-switched selectedChar) back to the bag.
	if g.dragSrc == dragFromEquip {
		g.party.UnequipItemToInventory(g.dragEquipSlot, g.dragEquipChar)
	}
	// inventory->inventory (handled per-cell) and spell->inventory are no-ops here.
	g.clearDrag()
}

// equipItemMatchesSlot reports whether item would equip into paperdoll slot for c
// - used for both drag-drop validation and the drag-time compatible-slot
// highlight. Thin alias over the model's ItemFitsSlot (the SSoT; EquipItemToSlot
// enforces the same check, so the UI gate is a preview, not the guard).
func equipItemMatchesSlot(c *character.MMCharacter, item items.Item, slot items.EquipSlot) bool {
	return c.ItemFitsSlot(item, slot)
}

// equipSlotDragSource arms a drag that begins on charIdx's equipped paperdoll
// slot. charIdx is remembered so switching character mid-drag can't unequip the
// wrong hero on drop.
func (ui *UISystem) equipSlotDragSource(charIdx int, slot items.EquipSlot, item items.Item, x, y, w, h int) {
	g := ui.game
	if !g.menuOpen || !g.dragArmed || g.dragSrc != dragNone {
		return
	}
	if ptInRect(g.dragStartX, g.dragStartY, image.Rect(x, y, x+w, y+h)) {
		g.dragSrc = dragFromEquip
		g.dragEquipSlot = slot
		g.dragEquipChar = charIdx
		g.dragItem = item
	}
}

// equipSlotDropZone equips a dragged inventory item onto a paperdoll slot it fits.
func (ui *UISystem) equipSlotDropZone(slot items.EquipSlot, x, y, w, h int) {
	g := ui.game
	if !g.dragOver(x, y, w, h) {
		return
	}
	if g.dragSrc == dragFromInventory && g.dragInvIndex >= 0 && g.dragInvIndex < len(g.party.Inventory) {
		ch := g.party.Members[g.selectedChar]
		if equipItemMatchesSlot(ch, g.party.Inventory[g.dragInvIndex], slot) {
			// Equip into the EXACT slot dropped on (so a ring lands on the finger
			// under the cursor, not whichever one EquipItem would auto-pick).
			g.party.EquipItemFromInventoryToSlot(g.dragInvIndex, g.selectedChar, slot)
		}
	}
	// Equipped item dragged onto ANOTHER compatible slot (e.g. a ring between the
	// two fingers): move it there rather than only allowing a return to the bag.
	// Gated on the drag owner still being displayed (1-4 mid-drag switch), same
	// as the slot glow - otherwise the move would fire on the wrong character.
	if g.dragSrc == dragFromEquip && g.dragEquipChar == g.selectedChar && g.dragEquipSlot != slot {
		ch := g.party.Members[g.dragEquipChar]
		if equipItemMatchesSlot(ch, g.dragItem, slot) {
			g.party.MoveEquippedSlot(g.dragEquipSlot, slot, g.dragEquipChar)
		}
	}
	g.clearDrag()
}

// inventoryCellDropZone swaps two bag items when one is dragged onto another
// (reorder within the inventory).
func (ui *UISystem) inventoryCellDropZone(dstIndex, x, y, w, h int) {
	g := ui.game
	if !g.dragOver(x, y, w, h) || g.dragSrc != dragFromInventory {
		return
	}
	src := g.dragInvIndex
	inv := g.party.Inventory
	if src >= 0 && src < len(inv) && dstIndex >= 0 && dstIndex < len(inv) && src != dstIndex {
		inv[src], inv[dstIndex] = inv[dstIndex], inv[src]
	}
	g.clearDrag()
}

// inventoryEmptyDropZone moves a dragged bag item to the end of the inventory when
// dropped onto an empty grid cell (the bag is a packed slice, so empty cells are
// the tail - "put it in a free slot" = append at the end).
func (ui *UISystem) inventoryEmptyDropZone(x, y, w, h int) {
	g := ui.game
	if !g.dragOver(x, y, w, h) || g.dragSrc != dragFromInventory {
		return
	}
	src := g.dragInvIndex
	inv := g.party.Inventory
	if src >= 0 && src < len(inv) {
		it := inv[src]
		rest := make([]items.Item, 0, len(inv))
		rest = append(rest, inv[:src]...)
		rest = append(rest, inv[src+1:]...)
		g.party.Inventory = append(rest, it)
	}
	g.clearDrag()
}

// resolveQuickSlotDrop moves the carried thing into (targetChar, targetSlot).
func (g *MMGame) resolveQuickSlotDrop(targetChar, targetSlot int) {
	tch := g.party.Members[targetChar]
	switch g.dragSrc {
	case dragFromInventory:
		if g.dragInvIndex >= 0 && g.dragInvIndex < len(g.party.Inventory) {
			item := g.party.Inventory[g.dragInvIndex]
			g.party.RemoveItem(g.dragInvIndex)
			occ := tch.QuickSlots[targetSlot]
			// Same-stack items pile up in the slot instead of displacing.
			if occ != nil && items.SameStack(*occ, item) {
				occ.Quantity = occ.Count() + item.Count()
				break
			}
			cp := item
			tch.QuickSlots[targetSlot] = &cp
			if occ != nil {
				g.returnQuickItemToInventory(*occ)
			}
		}
	case dragFromSpell:
		if sp, err := spells.CreateSpellItem(g.dragSpellID); err == nil {
			occ := tch.QuickSlots[targetSlot]
			cp := sp
			tch.QuickSlots[targetSlot] = &cp
			if occ != nil {
				g.returnQuickItemToInventory(*occ)
			}
		}
	case dragFromTrap:
		if it, ok := config.TrapItem(g.dragTrapKey); ok {
			occ := tch.QuickSlots[targetSlot]
			cp := it
			tch.QuickSlots[targetSlot] = &cp
			if occ != nil {
				g.returnQuickItemToInventory(*occ)
			}
		}
	case dragFromQuickSlot:
		if !(g.dragQuickChar == targetChar && g.dragQuickSlot == targetSlot) {
			sch := g.party.Members[g.dragQuickChar]
			src, dst := sch.QuickSlots[g.dragQuickSlot], tch.QuickSlots[targetSlot]
			if src != nil && dst != nil && items.SameStack(*src, *dst) {
				dst.Quantity = dst.Count() + src.Count()
				sch.QuickSlots[g.dragQuickSlot] = nil
			} else {
				sch.QuickSlots[g.dragQuickSlot], tch.QuickSlots[targetSlot] = dst, src
			}
		}
	}
	g.clearDrag()
}

// decrementQuickSlot takes one unit off a quick-slot stack, emptying the slot
// when the last unit goes.
func (g *MMGame) decrementQuickSlot(ch *character.MMCharacter, slotIdx int) {
	it := ch.QuickSlots[slotIdx]
	if it == nil {
		return
	}
	if it.Count() > 1 {
		it.Quantity = it.Count() - 1
		return
	}
	ch.QuickSlots[slotIdx] = nil
}

// returnQuickItemToInventory puts a displaced quick-slot item back into the bag,
// except spells, which are spellbook-owned and simply vanish from the slot.
func (g *MMGame) returnQuickItemToInventory(item items.Item) {
	// Spells and trap recipes are book-owned - they never belong in the bag.
	switch item.Type {
	case items.ItemBattleSpell, items.ItemUtilitySpell, items.ItemTrap:
		return
	}
	g.party.AddItem(item)
}

// clearDrag resets all drag state.
func (g *MMGame) clearDrag() {
	g.dragArmed = false
	g.dragActive = false
	g.dragDropAt = 0
	g.dragSrc = dragNone
	g.dragItem = items.Item{}
}

// updateQuickDrag samples the raw mouse each frame to drive the drag lifecycle.
// Drag is only armed while a tab is open; rect-based source/drop resolution
// happens during Draw (where layouts are known), per the project's input model.
func (ui *UISystem) updateQuickDrag() {
	g := ui.game
	// Don't arm/process a drag while a modal owns the inventory: the revival
	// picker stores an inventory index across frames, and a drag's RemoveItem/
	// AddItem would shift it (wrong item revived). Mirrors inventoryInputBlocked,
	// which gates every other inventory mutation.
	if !g.menuOpen || ui.inventoryInputBlocked() {
		if g.dragArmed || g.dragActive {
			g.clearDrag()
		}
		return
	}
	x, y := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.dragArmed = true
		g.dragActive = false
		g.dragDropAt = 0
		g.dragSrc = dragNone
		g.dragStartX, g.dragStartY = x, y
		g.dragCurX, g.dragCurY = x, y
		g.dragItem = items.Item{}
	}
	if g.dragArmed && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.dragCurX, g.dragCurY = x, y
		if !g.dragActive && (absInt(x-g.dragStartX) > quickDragThreshold || absInt(y-g.dragStartY) > quickDragThreshold) {
			g.dragActive = true
			// A real drag began: break any in-flight inventory double-click chain
			// so the press that started the drag can't later read as a use.
			ui.lastClickedItem = -1
		}
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		g.dragCurX, g.dragCurY = x, y
		if g.dragActive && g.dragSrc != dragNone {
			g.dragDropAt = 1 // Draw resolves against the target under the cursor
		} else {
			g.clearDrag() // plain click -> leave it to the click queue
		}
		g.dragArmed = false
	}
}

// drawDragCarried renders the carried icon under the cursor and cancels an
// unresolved drop (cursor released over nothing). Called at the end of the menu
// draw, after every drop target has had its chance.
func (ui *UISystem) drawDragCarried(screen *ebiten.Image) {
	g := ui.game
	if g.dragActive && g.dragSrc != dragNone {
		const sz = 48
		ui.drawInventoryItemIcon(screen, g.dragItem, g.dragCurX-sz/2, g.dragCurY-sz/2, sz, sz, 0, true)
	}
	if g.dragDropAt == 1 {
		g.clearDrag()
	}
}

// drawTabQuickSlotBar places the quick-slot bar at (barX,barY) width barW, with a
// label above it. Callers position it in the free space of the open tab so it
// clears the panel art.
func (ui *UISystem) drawTabQuickSlotBar(screen *ebiten.Image, barX, barY, barW int) {
	drawCenteredDebugText(screen, "Quick Slots - drag items / spells here", barX, barY-15, barW, 14)
	ui.drawQuickSlotBar(screen, ui.game.selectedChar, barX, barY, barW)
}

// inGameQuickSlotBarLayout returns the gameplay quick-bar rectangle exactly
// when it is visible. The HUD chat shares this layout to reserve the bar's
// space instead of drawing over it at narrow standard resolutions.
func inGameQuickSlotBarLayout(g *MMGame) (layoutRect, bool) {
	if g == nil || g.config == nil || g.menuOpen || g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
		return layoutRect{}, false
	}
	ch := g.party.Members[g.selectedChar]
	if ch == nil {
		return layoutRect{}, false
	}
	any := false
	for _, it := range ch.QuickSlots {
		if it != nil {
			any = true
			break
		}
	}
	if !any {
		return layoutRect{}, false
	}

	pw, _, baseLeft, startY := partyPortraitLayout(g)
	barW := pw * 2
	if barW > 240 {
		barW = 240
	}
	barH := int(float64(barW) / quickSlotBarAspect)
	barX := baseLeft + pw*4 - barW // right edge aligned to the rightmost card
	barY := startY - barH - 4
	if barY < 0 {
		barY = 0
	}
	return layoutRect{x: barX, y: barY, w: barW, h: barH}, true
}

// drawInGameQuickSlots floats the bar above the party cards, right-aligned to the
// card row (clear of the top-left spell-status icons), and double-click uses a
// slot for the selected character. Hidden when the selected character has no
// quick items.
func (ui *UISystem) drawInGameQuickSlots(screen *ebiten.Image) {
	g := ui.game
	bar, visible := inGameQuickSlotBarLayout(g)
	if !visible {
		return
	}
	ch := g.party.Members[g.selectedChar]

	slots := ui.drawQuickSlotBar(screen, g.selectedChar, bar.x, bar.y, bar.w)
	for i := 0; i < character.QuickSlotCount; i++ {
		if ch.QuickSlots[i] == nil {
			continue
		}
		r := slots[i]
		if g.consumeLeftClickIn(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) {
			now := g.mouseLeftClickAt
			if g.lastQuickClickedCh == g.selectedChar && g.lastQuickClickedSl == i &&
				withinDoubleClickWindow(now, g.lastQuickClickTime) {
				g.useQuickSlot(g.selectedChar, i)
				g.lastQuickClickTime = 0
				g.lastQuickClickedSl = -1
			} else {
				g.lastQuickClickTime = now
				g.lastQuickClickedCh = g.selectedChar
				g.lastQuickClickedSl = i
			}
		}
	}
}

// quickSlotCharReady reports whether a character may act through a quick slot
// right now - alive/conscious AND off cooldown (RT) or holding an action (TB).
// Mirrors the F/Space gating so quick slots can't bypass the combat cadence.
func (g *MMGame) quickSlotCharReady(idx int) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	if !m.CanAct() {
		return false
	}
	if g.turnBasedMode {
		return m.ActionsRemaining > 0
	}
	return m.RTCooldown == 0
}

// useQuickSlot applies a quick slot for a character: equip/swap a weapon or
// armour, drink one potion, or cast a spell. Reuses the existing equip/consume/
// cast paths so behaviour matches the inventory and spellbook exactly. A
// successful use spends the character's action (one cooldown per use), so quick
// slots respect the same combat cadence as the keyboard - and are inert while
// the character is on cooldown / out of actions.
func (g *MMGame) useQuickSlot(charIdx, slotIdx int) {
	if charIdx < 0 || charIdx >= len(g.party.Members) {
		return
	}
	ch := g.party.Members[charIdx]
	item := ch.QuickSlots[slotIdx]
	if item == nil {
		return
	}

	// Quest items (map, lich phylactery) are not combat actions: they work
	// regardless of cooldown/turn budget, exactly as they do in the inventory.
	if item.Type == items.ItemQuest {
		switch {
		case item.Attributes["opens_map"] > 0:
			g.mapOverlayOpen = true
		case item.Attributes["promotes_lich"] > 0:
			// useLichPhylactery consumes by inventory index: route through the bag.
			q := *item
			g.party.AddItem(q)
			g.useLichPhylactery(len(g.party.Inventory) - 1)
			ch.QuickSlots[slotIdx] = nil
		}
		return
	}

	// Potions are passive: usable regardless of cooldown / turn budget /
	// consciousness, and never spend an action (matches inventory use). A heal
	// used by an unconscious owner routes to a target picker (see
	// UseConsumableFromInventory); a revive opens the revival picker.
	if item.Type == items.ItemConsumable {
		// ONE unit goes to the bag as a temp entry - raw append, not AddItem: a
		// merge into an existing bag stack would break the "temp copy at idx"
		// contract below and consume from the wrong pile.
		drink := *item
		drink.Quantity = 1
		g.party.Inventory = append(g.party.Inventory, drink)
		idx := len(g.party.Inventory) - 1
		used := g.UseConsumableFromInventory(idx, charIdx)
		switch {
		case used:
			g.decrementQuickSlot(ch, slotIdx) // one unit drunk; stack lives on
		case g.revivalPickerOpen || g.healPickerOpen:
			// A picker owns the temp bag copy at idx; keep the slot filled until it
			// resolves (confirm clears it, cancel drops the temp copy & keeps it).
			g.pickerQuickChar, g.pickerQuickSlot = charIdx, slotIdx
		default:
			g.party.RemoveItem(idx) // refused (full HP etc.): keep it, spend nothing
		}
		return
	}

	// Equipping a weapon/armor/accessory is a gear SWAP, not a combat action: it
	// works regardless of cooldown / turn budget and never spends one (matches the
	// inventory double-click). Handled BEFORE the readiness gate.
	switch item.Type {
	case items.ItemWeapon, items.ItemArmor, items.ItemAccessory:
		prev, had, ok := ch.EquipItem(*item)
		if !ok {
			g.AddCombatMessage(fmt.Sprintf("%s cannot use %s!", ch.Name, item.Name))
			return
		}
		g.AddCombatMessage(fmt.Sprintf("%s equips %s!", ch.Name, item.Name))
		// Swap: the displaced gear takes the freed slot (spells never leak here).
		if had && prev.Type != items.ItemBattleSpell && prev.Type != items.ItemUtilitySpell {
			cp := prev
			ch.QuickSlots[slotIdx] = &cp
		} else {
			ch.QuickSlots[slotIdx] = nil
		}
		return
	}

	// Spells and traps ARE combat actions: gated by readiness, and a successful one
	// spends the action (TB) / sets the cooldown (RT).
	if !g.quickSlotCharReady(charIdx) {
		return
	}
	acted := false
	cdFrames := 0
	switch item.Type {
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		spellID := spells.SpellID(item.SpellEffect)
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err != nil {
			return
		}
		if g.combat.castResolvedSpell(spellID, def, ch, g.combat.effectiveSpellCost(ch, def.SpellPointsCost), true, true) {
			acted, cdFrames = true, g.combat.SpellCooldownFrames(ch, spellID)
		}
	case items.ItemTrap:
		// Arm the trap recipe in the world (same path as the trap-book double-click).
		// The recipe is book-owned, so the slot keeps the trap for reuse.
		trapKey := string(item.SpellEffect)
		if _, placed := g.combat.placeTrapByKey(ch, trapKey, true); placed {
			acted, cdFrames = true, g.combat.TrapCooldownFrames(ch, trapKey)
		}
	}
	if acted {
		if g.turnBasedMode {
			g.consumeSelectedCharActionWithRTCooldown(cdFrames)
		} else {
			ch.RTCooldown = cdFrames
		}
	}
}
