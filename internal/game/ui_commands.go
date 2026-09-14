package game

import (
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// UI commands retain the geometry and content of a completed display. Their
// model adapters run during Update, before keyboard handling and world pause.
// The adapters reuse the existing transaction rules; Draw never invokes them.
type uiCommandKind uint8

const (
	uiCommandClick uiCommandKind = iota
	uiCommandDrag
	uiCommandHold
	uiCommandNavigation
)

type uiInputCommand struct {
	kind   uiCommandKind
	bounds layoutRect
	apply  func()
}

type uiDisplayIdentity struct {
	world  *world.World3D
	party  *character.Party
	modal  modalLayerSnapshot
	screen uiScreenIdentity
	state  [12]int
	items  uint64
}

// Screen ownership is shared by the display barrier and raw pointer gestures.
// The gesture owner survives Draw, unlike the displayed-command snapshot.
type uiScreenIdentity struct {
	app           AppScreen
	entry         EntryMenuMode
	width, height int
}

func (ui *UISystem) inputScreenIdentity() uiScreenIdentity {
	g := ui.game
	return uiScreenIdentity{g.appScreen, g.entryMenuMode, g.config.GetScreenWidth(), g.config.GetScreenHeight()}
}

type uiDisplayedInput struct {
	building, suspended  bool
	processingClick      bool
	quickDrag, stashDrag uiDragIdentity
	holdIdentity         uiDisplayIdentity
	holdActor            *character.MMCharacter
	commands             []uiInputCommand
	identity             uiDisplayIdentity
	actors               []*character.MMCharacter
	sequence             uint64
	ready                bool
	pointerScreen        uiScreenIdentity
	pointerScreenSet     bool
}

func (ui *UISystem) syncPointerScreen() {
	d := &ui.displayedInput
	screen := ui.inputScreenIdentity()
	if d.pointerScreenSet && d.pointerScreen != screen {
		ui.cancelScreenPointerGestures()
	}
	d.pointerScreen, d.pointerScreenSet = screen, true
}

func (ui *UISystem) cancelScreenPointerGestures() {
	g := ui.game
	g.entryMenuRootPressArmed = false
	if g.partyCreate != nil {
		g.partyCreate.clearPending()
		g.partyCreate.clearDrag()
	}
	if g.audioSliderDrag >= 0 {
		g.saveAudioSettings()
		g.audioSliderDrag = -1
	}
}

func (ui *UISystem) displayIdentity() uiDisplayIdentity {
	g := ui.game
	id := uiDisplayIdentity{world: g.world, party: g.party, modal: ui.topModalSnapshot(), screen: ui.inputScreenIdentity()}
	id.state = [12]int{g.savePage,
		boolInt(g.menuOpen), int(g.currentTab), g.selectedChar, ui.inventoryPage, ui.inventoryTab, ui.spellPage, ui.questPage,
		boolInt(ui.inventoryContextOpen), ui.inventoryContextIndex, g.selectedSchool, g.selectedSpell}
	hash := uint64(14695981039346656037)
	mix := func(v uint64) { hash ^= v; hash *= 1099511628211 }
	item := func(it items.Item) { mix(uiItemIdentity(it)) }
	if g.party != nil {
		mix(g.party.ContentRevision())
		mix(uint64(len(g.party.Inventory)))
		for _, it := range g.party.Inventory {
			item(it)
		}
		for _, group := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve} {
			mix(uint64(len(group)))
			for _, ch := range group {
				if ch == nil {
					mix(0)
					continue
				}
				for schoolIndex, school := range character.AllMagicSchools {
					mix(uint64(schoolIndex))
					if skill := ch.MagicSchools[school]; skill != nil {
						mix(uint64(skill.Mastery) + 1)
						for _, spell := range skill.KnownSpells {
							for i := 0; i < len(spell); i++ {
								mix(uint64(spell[i]))
							}
							mix(0)
						}
					} else {
						mix(0)
					}
				}
				for slot := items.EquipSlot(0); slot <= items.SlotSpell; slot++ {
					it, ok := ch.Equipment[slot]
					mix(uint64(slot))
					mix(uint64(boolInt(ok)))
					if ok {
						item(it)
					}
				}
				for _, it := range ch.QuickSlots {
					if it == nil {
						mix(0)
					} else {
						mix(1)
						item(*it)
					}
				}
			}
		}
	}
	if g.stash != nil {
		for _, it := range g.stash.Slots {
			item(it)
		}
		for _, it := range g.stash.CardSlots {
			item(it)
		}
	}
	if g.dialogActive && g.dialogNPC != nil {
		for _, entry := range g.merchantVisibleStock() {
			if entry == nil {
				mix(0)
				continue
			}
			item(entry.Item)
			mix(uint64(entry.Quantity))
			mix(uint64(entry.Cost))
			mix(uint64(entry.GoldCost))
		}
	}
	id.items = hash
	return id
}

func (ui *UISystem) beginDisplayedInput() {
	d := &ui.displayedInput
	d.building = true
	d.suspended = false
	// Release captured references from the previous layout before reuse.
	clear(d.commands)
	d.commands = d.commands[:0]
	d.actors = d.actors[:0]
	if ui.game.party != nil {
		d.actors = append(d.actors, ui.game.party.Members...)
		d.actors = append(d.actors, ui.game.party.Reserve...)
	}
}

func (ui *UISystem) endDisplayedInput() {
	d := &ui.displayedInput
	d.building = false
	d.ready = true
	// Presentation may clamp a page; identity describes the layout just drawn.
	d.identity = ui.displayIdentity()
}

func (ui *UISystem) displayedInputCurrent() bool {
	d := &ui.displayedInput
	if !d.ready || d.identity != ui.displayIdentity() {
		return false
	}
	g := ui.game
	if g.party == nil {
		return len(d.actors) == 0
	}
	if len(d.actors) != len(g.party.Members)+len(g.party.Reserve) {
		return false
	}
	i := 0
	for _, group := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve} {
		for _, ch := range group {
			if d.actors[i] != ch {
				return false
			}
			i++
		}
	}
	return true
}

// onDisplayedInput is also usable from direct Update/model adapters: only a
// Draw registration defers execution. Closures capture displayed slot indices,
// item values and layout coordinates, protected by the frame identity check.
func (ui *UISystem) onDisplayedInput(kind uiCommandKind, bounds layoutRect, apply func()) {
	if !ui.displayedInput.building {
		apply()
		return
	}
	if !ui.displayedInput.suspended {
		ui.displayedInput.commands = append(ui.displayedInput.commands, uiInputCommand{kind: kind, bounds: bounds, apply: apply})
	}
}

// Register existing Update mouse adapters only for the modal being displayed.
// Keyboard, wheel and hover polling still run once in InputHandler.HandleInput.
func (ui *UISystem) registerDisplayedModalMouse(handle func(*InputHandler), layers ...modalLayerID) {
	if !ui.displayedInput.building {
		return
	}
	for _, layer := range layers {
		if ui.topModalLayer() == layer {
			// A trainer popup is drawn inside the otherwise suspended overlay
			// group. Its explicit top-layer match still owns this adapter.
			ui.displayedInput.commands = append(ui.displayedInput.commands, uiInputCommand{apply: func() {
				handler := InputHandler{game: ui.game}
				handle(&handler)
			}})
			return
		}
	}
}

func (ui *UISystem) displayedDragPending() bool {
	g := ui.game
	return g.dragArmed || g.dragActive || g.dragPickedUp || g.dragDropAt == 1 ||
		g.stashDragArmed || g.stashDragActive || g.stashDragPickedUp || g.stashDragDrop
}

func (ui *UISystem) dispatchDisplayedInput() {
	d := &ui.displayedInput
	g := ui.game
	if ui.topModalLayer() != modalLayerStat || ui.statHoldStat != "" &&
		(d.holdIdentity != ui.displayIdentity() || d.holdActor != ui.statHoldMember()) {
		ui.statHoldStat = ""
		ui.statHoldFrames = 0
	}
	defer func() {
		if ui.statHoldStat != "" {
			d.holdIdentity, d.holdActor = ui.displayIdentity(), ui.statHoldMember()
		}
	}()
	if !d.ready {
		return
	}
	ui.validateDisplayedDrags()
	dragPending := ui.displayedDragPending()
	holdPending := ui.topModalLayer() == modalLayerStat && (pointerLeftPressed() || ui.statHoldStat != "")
	if len(g.mouseLeftClicks) == 0 && len(g.mouseRightClicks) == 0 && !dragPending && !holdPending {
		return
	}
	if !ui.displayedInputCurrent() {
		ui.dropQueuedClicks()
		ui.game.clearDrag()
		ui.game.clearStashDrag()
		return
	}
	ui.syncCharacterHubClickContext()
	// Hold timers and drag edges tick once per Update, regardless of queued
	// clicks. Click adapters then see one event at a time in timestamp order.
	left, right := g.mouseLeftClicks, g.mouseRightClicks
	g.mouseLeftClicks, g.mouseRightClicks = nil, nil
	for _, cmd := range d.commands {
		if !(cmd.kind == uiCommandDrag && dragPending || cmd.kind == uiCommandHold && holdPending) {
			continue
		}
		if cmd.kind == uiCommandDrag && cmd.bounds.w > 0 && cmd.bounds.h > 0 {
			hit := func(x, y int) bool {
				return isMouseHoveringBox(x, y, cmd.bounds.x, cmd.bounds.y, cmd.bounds.right(), cmd.bounds.bottom())
			}
			if !(g.dragArmed && hit(g.dragStartX, g.dragStartY) || g.dragDropAt == 1 && hit(g.dragCurX, g.dragCurY) ||
				g.stashDragArmed && hit(g.stashDragStartX, g.stashDragStartY) || g.stashDragDrop && hit(g.stashDragCurX, g.stashDragCurY)) {
				continue
			}
		}
		if !ui.displayedInputCurrent() {
			break
		}
		cmd.apply()
	}
	ui.captureDisplayedDrags()
	remainingLeft, remainingRight := []queuedClick(nil), []queuedClick(nil)
	d.processingClick = true
	for len(left) > 0 || len(right) > 0 {
		if !ui.displayedInputCurrent() {
			break
		}
		isLeft := len(left) > 0 && (len(right) == 0 || left[0].at <= right[0].at)
		d.sequence++
		if isLeft {
			g.mouseLeftClicks = []queuedClick{left[0]}
			left = left[1:]
		} else {
			g.mouseRightClicks = []queuedClick{right[0]}
			right = right[1:]
		}
		ui.handleModalLayerInput()
		for _, cmd := range d.commands {
			if len(g.mouseLeftClicks) == 0 && len(g.mouseRightClicks) == 0 {
				break
			}
			if cmd.kind == uiCommandDrag {
				continue
			}
			if cmd.bounds.w > 0 && cmd.bounds.h > 0 {
				queue := g.mouseRightClicks
				if isLeft {
					queue = g.mouseLeftClicks
				}
				if len(queue) == 0 || !isMouseHoveringBox(queue[0].x, queue[0].y, cmd.bounds.x, cmd.bounds.y, cmd.bounds.right(), cmd.bounds.bottom()) {
					continue
				}
			}
			if !ui.displayedInputCurrent() {
				break
			}
			cmd.apply()
		}
		if ui.topModalLayer() == modalLayerNone && ui.displayedInputCurrent() {
			remainingLeft = append(remainingLeft, g.mouseLeftClicks...)
			remainingRight = append(remainingRight, g.mouseRightClicks...)
		}
		g.mouseLeftClicks, g.mouseRightClicks = nil, nil
	}
	d.processingClick = false
	g.mouseLeftClicks, g.mouseRightClicks = remainingLeft, remainingRight
	if g.dragDropAt == 1 {
		g.clearDrag()
	}
	if g.stashDragDrop {
		g.clearStashDrag()
	}
	if !ui.displayedInputCurrent() {
		ui.dropQueuedClicks()
	}
}

func (ui *UISystem) statHoldMember() *character.MMCharacter {
	g := ui.game
	if g.party == nil || g.statPopupCharIdx < 0 || g.statPopupCharIdx >= len(g.party.Members) {
		return nil
	}
	return g.party.Members[g.statPopupCharIdx]
}

// uiItemIdentity includes provenance and count, including legacy untracked
// items. It does not assign IDs or otherwise mutate items while rendering.
func uiItemIdentity(it items.Item) uint64 {
	hash := uint64(14695981039346656037)
	mix := func(v uint64) { hash ^= v; hash *= 1099511628211 }
	mix(it.InstanceID)
	mix(uint64(it.Type))
	mix(uint64(it.Count()))
	for _, text := range []string{it.Name, string(it.SpellEffect)} {
		for i := 0; i < len(text); i++ {
			mix(uint64(text[i]))
		}
		mix(0)
	}
	for _, part := range it.Lineages {
		mix(part.ID)
		mix(uint64(part.Quantity))
	}
	return hash
}

// Gesture identity survives Draws: a newly rendered bag must not silently
// retarget an already carried item after reorder, stack merge or roster swap.
type uiDragIdentity struct {
	active      bool
	world       *world.World3D
	party       *character.Party
	actor       *character.MMCharacter
	kind, index int
	item        uint64
}

func (ui *UISystem) currentDragIdentity(stashDrag bool) uiDragIdentity {
	g := ui.game
	id := uiDragIdentity{world: g.world, party: g.party}
	if g.party == nil {
		return id
	}
	var it items.Item
	if stashDrag {
		if !g.stashDragArmed && !g.stashDragActive && !g.stashDragPickedUp && !g.stashDragDrop {
			return id
		}
		addr, ok := decodeStashFrom(g.stashDragFrom)
		if !ok {
			return id
		}
		id.kind, id.index = int(addr.kind), addr.idx
		switch addr.kind {
		case stashKindBag:
			if addr.idx < 0 || addr.idx >= len(g.party.Inventory) {
				return id
			}
			it = g.party.Inventory[addr.idx]
		case stashKindShop:
			stock := g.merchantVisibleStock()
			if addr.idx < 0 || addr.idx >= len(stock) || stock[addr.idx] == nil {
				return id
			}
			it = stock[addr.idx].Item
		default:
			if g.stash == nil {
				return id
			}
			p := g.stashCellPtr(addr)
			if p == nil {
				return id
			}
			it = *p
		}
	} else {
		if g.dragSrc == dragNone {
			return id
		}
		id.kind = int(g.dragSrc)
		charIdx := g.selectedChar
		switch g.dragSrc {
		case dragFromInventory:
			id.index = g.dragInvIndex
			if id.index < 0 || id.index >= len(g.party.Inventory) {
				return id
			}
			it = g.party.Inventory[id.index]
		case dragFromQuickSlot:
			charIdx = g.dragQuickChar
			id.index = g.dragQuickSlot
		case dragFromEquip:
			charIdx = g.dragEquipChar
			id.index = int(g.dragEquipSlot)
		default:
			it = g.dragItem
		}
		if charIdx < 0 || charIdx >= len(g.party.Members) {
			return id
		}
		id.actor = g.party.Members[charIdx]
		if id.actor == nil {
			return id
		}
		if g.dragSrc == dragFromQuickSlot {
			if id.index < 0 || id.index >= len(id.actor.QuickSlots) || id.actor.QuickSlots[id.index] == nil {
				return id
			}
			it = *id.actor.QuickSlots[id.index]
		}
		if g.dragSrc == dragFromEquip {
			var ok bool
			it, ok = id.actor.Equipment[g.dragEquipSlot]
			if !ok {
				return id
			}
		}
	}
	id.item = uiItemIdentity(it)
	id.active = true
	return id
}

func (ui *UISystem) validateDisplayedDrags() {
	d := &ui.displayedInput
	if d.quickDrag.active && d.quickDrag != ui.currentDragIdentity(false) {
		ui.game.clearDrag()
		d.quickDrag = uiDragIdentity{}
	}
	if d.stashDrag.active && d.stashDrag != ui.currentDragIdentity(true) {
		ui.game.clearStashDrag()
		d.stashDrag = uiDragIdentity{}
	}
}

func (ui *UISystem) captureDisplayedDrags() {
	d := &ui.displayedInput
	if !d.quickDrag.active {
		d.quickDrag = ui.currentDragIdentity(false)
	}
	if !d.stashDrag.active {
		d.stashDrag = ui.currentDragIdentity(true)
	}
}
