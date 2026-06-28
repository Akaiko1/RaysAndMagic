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
//   weapon → equip/swap, potion → drink one, spell → cast.
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
// quick_slots_bar.png (cx evenly spaced, cy centred, side ≈ gold-window width).
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
// (trapper parity with spells — a trap recipe is book-owned, like a spell).
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

// quickInvDropZone resolves a quick-slot item dropped back onto the inventory grid.
func (ui *UISystem) quickInvDropZone(x, y, w, h int) {
	g := ui.game
	if !g.menuOpen || g.dragDropAt != 1 || g.dragSrc == dragNone {
		return
	}
	if !ptInRect(g.dragCurX, g.dragCurY, image.Rect(x, y, x+w, y+h)) {
		return
	}
	if g.dragSrc == dragFromQuickSlot {
		sch := g.party.Members[g.dragQuickChar]
		if it := sch.QuickSlots[g.dragQuickSlot]; it != nil {
			g.returnQuickItemToInventory(*it)
			sch.QuickSlots[g.dragQuickSlot] = nil
		}
	}
	// inventory→inventory and spell→inventory are no-ops (spells are book-owned).
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
			sch.QuickSlots[g.dragQuickSlot], tch.QuickSlots[targetSlot] =
				tch.QuickSlots[targetSlot], sch.QuickSlots[g.dragQuickSlot]
		}
	}
	g.clearDrag()
}

// returnQuickItemToInventory puts a displaced quick-slot item back into the bag,
// except spells, which are spellbook-owned and simply vanish from the slot.
func (g *MMGame) returnQuickItemToInventory(item items.Item) {
	// Spells and trap recipes are book-owned — they never belong in the bag.
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
			g.clearDrag() // plain click → leave it to the click queue
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
	drawCenteredDebugText(screen, "Quick Slots — drag items / spells here", barX, barY-15, barW, 14)
	ui.drawQuickSlotBar(screen, ui.game.selectedChar, barX, barY, barW)
}

// drawInGameQuickSlots floats the bar above the party cards, right-aligned to the
// card row (clear of the top-left spell-status icons), and double-click uses a
// slot for the selected character. Hidden when the selected character has no
// quick items.
func (ui *UISystem) drawInGameQuickSlots(screen *ebiten.Image) {
	g := ui.game
	if g.menuOpen { // the open tab already shows the bar
		return
	}
	if g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
		return
	}
	ch := g.party.Members[g.selectedChar]
	any := false
	for _, it := range ch.QuickSlots {
		if it != nil {
			any = true
			break
		}
	}
	if !any {
		return
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

	slots := ui.drawQuickSlotBar(screen, g.selectedChar, barX, barY, barW)
	for i := 0; i < character.QuickSlotCount; i++ {
		if ch.QuickSlots[i] == nil {
			continue
		}
		r := slots[i]
		if g.consumeLeftClickIn(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y) {
			now := g.mouseLeftClickAt
			if g.lastQuickClickedCh == g.selectedChar && g.lastQuickClickedSl == i && now-g.lastQuickClickTime < doubleClickWindowMs {
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
// right now — alive/conscious AND off cooldown (RT) or holding an action (TB).
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
// slots respect the same combat cadence as the keyboard — and are inert while
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

	if !g.quickSlotCharReady(charIdx) {
		return
	}
	acted := false
	cdFrames := 0
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
		acted, cdFrames = true, g.combat.WeaponCooldownFrames(ch)
	case items.ItemConsumable:
		// Route through the shared inventory consumable path: drop into the bag,
		// use by index, then reconcile so the slot empties iff it was consumed.
		drink := *item
		g.party.AddItem(drink)
		idx := len(g.party.Inventory) - 1
		used := g.UseConsumableFromInventory(idx, charIdx)
		if used || g.revivalPickerOpen {
			ch.QuickSlots[slotIdx] = nil // consumed (or the revive picker now owns it)
			acted, cdFrames = true, g.combat.WeaponCooldownFrames(ch)
		} else {
			g.party.RemoveItem(idx) // refused (full HP etc.): keep it, spend nothing
		}
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		spellID := spells.SpellID(item.SpellEffect)
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err != nil {
			return
		}
		if g.combat.castResolvedSpell(spellID, def, ch, g.combat.effectiveSpellCost(ch, def.SpellPointsCost), true) {
			acted, cdFrames = true, g.combat.SpellCooldownFrames(ch, spellID)
		}
	case items.ItemTrap:
		// Arm the trap recipe in the world (same path as the trap-book double-click).
		// The recipe is book-owned, so the slot keeps the trap for reuse.
		if _, placed := g.combat.placeTrapByKey(ch, string(item.SpellEffect), true); placed {
			acted, cdFrames = true, g.combat.WeaponCooldownFrames(ch)
		}
	}
	if acted {
		if g.turnBasedMode {
			g.consumeSelectedCharAction()
		} else {
			ch.RTCooldown = cdFrames
		}
	}
}
