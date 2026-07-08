package main

// Saves page ("Save Stashes"): browse the game's save slots and the shared
// stash, inspect a save's party equipment / inventory / card collection with
// the game's rarity tints, and park saves in an archive folder the game never
// lists (Add to Archive frees the slot; Restore Save moves one back into a
// FREE slot - occupied slots are refused).

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/items"
	"ugataima/internal/stash"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	saveListW      = 320
	saveListRowH   = 18
	saveListPadY   = 8
	saveDetailRowH = 14
	saveButtonH    = 24
)

type saveEntryKind int

const (
	saveEntryHeader saveEntryKind = iota
	saveEntryStash
	saveEntrySlot
	saveEntryArchive
)

// saveListEntry is one row of the left-hand list; slot rows carry their save
// row index, archive rows their index into savesPage.archived.
type saveListEntry struct {
	kind  saveEntryKind
	row   int
	arch  int
	label string
	dim   bool
}

// saveDetailLine is one row of a detail panel; lines that show an item carry
// it so hovering can raise the same tooltip the Items page uses. Header lines
// render on a filled band (the editor/game section-header convention).
type saveDetailLine struct {
	text   string
	col    color.Color
	item   *items.Item
	header bool
}

var savesPage struct {
	inited      bool
	stale       bool // set while another page is active; re-enter reloads from disk
	rows        []game.SaveSummary
	archived    []game.ArchivedSave
	entries     []saveListEntry
	selIdx      int // index into entries; -1 = none
	listScroll  int
	partyLines  []saveDetailLine // left detail panel: header + party equipment
	lootLines   []saveDetailLine // right detail panel: inventory + cards (or stash slots)
	partyScroll int
	lootScroll  int
	restoreArm  bool // armed by Restore Save: next click on a free slot restores
	status      string
	statusIsErr bool
}

type saveButton struct {
	id    string
	label string
	r     rect
}

func (v *viewer) updateSavesPage() {
	if !savesPage.inited || savesPage.stale {
		v.refreshSavesPage()
		savesPage.inited = true
		savesPage.stale = false
	}

	moved := 0
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		moved = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		moved = -1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		moved = 5
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		moved = -5
	}
	if moved != 0 {
		v.moveSaveSelection(moved)
	}

	mx, my := ebiten.CursorPosition()
	if _, wheelY := ebiten.Wheel(); wheelY != 0 {
		delta := -int(wheelY * 30)
		partyPanel, lootPanel := saveDetailPanels()
		switch {
		case mx < saveListW:
			savesPage.listScroll = clampScrollValue(savesPage.listScroll+delta, maxSaveListScroll())
		case pointInRect(mx, my, partyPanel.x, partyPanel.y, partyPanel.w, partyPanel.h):
			savesPage.partyScroll = clampScrollValue(savesPage.partyScroll+delta, maxDetailScroll(savesPage.partyLines, partyPanel))
		case pointInRect(mx, my, lootPanel.x, lootPanel.y, lootPanel.w, lootPanel.h):
			savesPage.lootScroll = clampScrollValue(savesPage.lootScroll+delta, maxDetailScroll(savesPage.lootLines, lootPanel))
		}
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		v.handleSavesClick(mx, my)
	}
}

func (v *viewer) refreshSavesPage() {
	rows := make([]game.SaveSummary, game.SaveRowsTotal())
	for i := range rows {
		rows[i] = game.GetSaveRowSummary(i)
	}
	savesPage.rows = rows
	savesPage.archived = game.ListArchivedSaves()
	savesPage.entries = buildSaveListEntries(rows, savesPage.archived)
	if savesPage.selIdx < 0 || savesPage.selIdx >= len(savesPage.entries) ||
		savesPage.entries[savesPage.selIdx].kind == saveEntryHeader {
		savesPage.selIdx = firstSelectableEntry(savesPage.entries)
	}
	v.rebuildSaveDetail()
}

// buildSaveListEntries lays out the list rows: the shared stash, every game
// slot (empty ones dimmed), then the archive. Pure so it is testable.
func buildSaveListEntries(rows []game.SaveSummary, archived []game.ArchivedSave) []saveListEntry {
	out := []saveListEntry{
		{kind: saveEntryHeader, label: "Shared"},
		{kind: saveEntryStash, label: "Shared Stash"},
		{kind: saveEntryHeader, label: "Game Slots"},
	}
	for row := range rows {
		sum := rows[row]
		label := game.SaveRowDisplayName(row)
		dim := false
		switch {
		case !sum.Exists:
			label += "  - empty -"
			dim = true
		case sum.Name != "":
			label += "  " + sum.Name
		case sum.MapKey != "":
			label += "  " + sum.MapKey
		}
		out = append(out, saveListEntry{kind: saveEntrySlot, row: row, label: label, dim: dim})
	}
	out = append(out, saveListEntry{kind: saveEntryHeader, label: fmt.Sprintf("Archive (%d)", len(archived))})
	for i := range archived {
		out = append(out, saveListEntry{kind: saveEntryArchive, arch: i, label: archiveDisplayName(archived[i])})
	}
	return out
}

func archiveDisplayName(a game.ArchivedSave) string {
	name := strings.TrimSuffix(filepath.Base(a.Path), ".json")
	if a.Summary.Exists && a.Summary.Name != "" {
		name = a.Summary.Name
	}
	if a.Summary.Exists && a.Summary.MapKey != "" {
		name += "  (" + a.Summary.MapKey + ")"
	}
	return name
}

func firstSelectableEntry(entries []saveListEntry) int {
	for i, e := range entries {
		if e.kind != saveEntryHeader {
			return i
		}
	}
	return -1
}

func selectedSaveEntry() (saveListEntry, bool) {
	if savesPage.selIdx < 0 || savesPage.selIdx >= len(savesPage.entries) {
		return saveListEntry{}, false
	}
	e := savesPage.entries[savesPage.selIdx]
	return e, e.kind != saveEntryHeader
}

func (v *viewer) handleSavesClick(mx, my int) {
	for _, btn := range savesButtons() {
		if pointInRect(mx, my, btn.r.x, btn.r.y, btn.r.w, btn.r.h) {
			v.handleSavesButton(btn.id)
			return
		}
	}

	if mx >= saveListW || my <= pageBarHeight {
		return
	}
	idx := (my - pageBarHeight - saveListPadY + savesPage.listScroll) / saveListRowH
	if idx < 0 || idx >= len(savesPage.entries) {
		return
	}
	entry := savesPage.entries[idx]
	if entry.kind == saveEntryHeader {
		return
	}

	if savesPage.restoreArm && entry.kind == saveEntrySlot {
		v.performRestore(entry.row)
		return
	}

	savesPage.restoreArm = false
	savesPage.selIdx = idx
	savesPage.status = ""
	v.rebuildSaveDetail()
}

func (v *viewer) handleSavesButton(id string) {
	entry, _ := selectedSaveEntry()
	switch id {
	case "refresh":
		v.refreshSavesPage()
		setSavesStatus("Reloaded saves from disk", false)
	case "archive":
		dst, err := game.ArchiveSaveRow(entry.row)
		if err != nil {
			setSavesStatus(err.Error(), true)
			return
		}
		v.refreshSavesPage()
		v.selectArchivedPath(dst)
		setSavesStatus("Archived to "+filepath.Base(dst)+" - the slot is free now", false)
	case "restore":
		savesPage.restoreArm = true
		setSavesStatus("Click an EMPTY slot in the list to restore into it", false)
	case "cancel":
		savesPage.restoreArm = false
		setSavesStatus("Restore cancelled", false)
	}
}

func (v *viewer) performRestore(row int) {
	entry, ok := selectedSaveEntry()
	if !ok || entry.kind != saveEntryArchive || entry.arch >= len(savesPage.archived) {
		savesPage.restoreArm = false
		return
	}
	if err := game.RestoreArchivedSave(savesPage.archived[entry.arch].Path, row); err != nil {
		setSavesStatus(err.Error(), true) // stay armed so another slot can be picked
		return
	}
	savesPage.restoreArm = false
	v.refreshSavesPage()
	v.selectSlotRow(row)
	setSavesStatus("Restored into "+game.SaveRowDisplayName(row), false)
}

func (v *viewer) selectArchivedPath(path string) {
	for i, e := range savesPage.entries {
		if e.kind == saveEntryArchive && savesPage.archived[e.arch].Path == path {
			savesPage.selIdx = i
			v.rebuildSaveDetail()
			return
		}
	}
}

func (v *viewer) selectSlotRow(row int) {
	for i, e := range savesPage.entries {
		if e.kind == saveEntrySlot && e.row == row {
			savesPage.selIdx = i
			v.rebuildSaveDetail()
			return
		}
	}
}

// moveSaveSelection steps the selection up/down the list, skipping header
// rows, and keeps the selected row visible.
func (v *viewer) moveSaveSelection(delta int) {
	idx := savesPage.selIdx
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	for n := 0; n < delta; n++ {
		j := idx + step
		for j >= 0 && j < len(savesPage.entries) && savesPage.entries[j].kind == saveEntryHeader {
			j += step
		}
		if j < 0 || j >= len(savesPage.entries) {
			break
		}
		idx = j
	}
	if idx == savesPage.selIdx {
		return
	}
	savesPage.selIdx = idx
	savesPage.restoreArm = false
	savesPage.status = ""
	v.rebuildSaveDetail()

	top := idx * saveListRowH
	if top-savesPage.listScroll < 0 {
		savesPage.listScroll = top
	}
	if bottom := top + saveListRowH; bottom-savesPage.listScroll > saveListViewportH() {
		savesPage.listScroll = bottom - saveListViewportH()
	}
	savesPage.listScroll = clampScrollValue(savesPage.listScroll, maxSaveListScroll())
}

func setSavesStatus(msg string, isErr bool) {
	savesPage.status = msg
	savesPage.statusIsErr = isErr
}

func (v *viewer) rebuildSaveDetail() {
	savesPage.partyScroll = 0
	savesPage.lootScroll = 0
	savesPage.partyLines = nil
	savesPage.lootLines = nil
	entry, ok := selectedSaveEntry()
	if !ok {
		return
	}
	switch entry.kind {
	case saveEntryStash:
		savesPage.partyLines, savesPage.lootLines = buildStashDetail()
	case saveEntrySlot:
		sum := savesPage.rows[entry.row]
		if !sum.Exists {
			savesPage.partyLines = []saveDetailLine{{text: "(empty slot)", col: mobStatHeader}}
			return
		}
		savesPage.partyLines, savesPage.lootLines = buildSaveDetail(game.SaveRowFilePath(entry.row), game.SaveRowDisplayName(entry.row), sum)
	case saveEntryArchive:
		a := savesPage.archived[entry.arch]
		savesPage.partyLines, savesPage.lootLines = buildSaveDetail(a.Path, filepath.Base(a.Path), a.Summary)
	}
}

// buildSaveDetail reads a full save and renders it into the two detail panels:
// party + equipment on the left, inventory + card collection on the right.
// Item lines carry a refreshed copy of the item for the hover tooltip.
func buildSaveDetail(path, label string, sum game.SaveSummary) (party, loot []saveDetailLine) {
	gs, err := game.ReadGameSave(path)
	if err != nil {
		return []saveDetailLine{{text: "failed to read save: " + err.Error(), col: mobStatDamage}}, nil
	}

	addp := func(col color.Color, format string, args ...any) {
		party = append(party, saveDetailLine{text: fmt.Sprintf(format, args...), col: col})
	}
	addpHeader := func(format string, args ...any) {
		party = append(party, saveDetailLine{text: fmt.Sprintf(format, args...), col: color.White, header: true})
	}
	addpItem := func(it items.Item, format string, args ...any) {
		party = append(party, saveDetailLine{text: fmt.Sprintf(format, args...), col: game.ItemRarityColor(it), item: &it})
	}
	addp(color.White, "%s", label)
	if sum.Name != "" {
		addp(color.White, "Name: %s", sum.Name)
	}
	addp(mobStatDefault, "Map: %s   Saved: %s", gs.MapKey, prettySavedAt(gs.SavedAt))
	if sum.PlayTime != "" {
		addp(mobStatDefault, "Playtime: %s", sum.PlayTime)
	}
	addp(mobStatGold, "Gold %d   Food %d", gs.Party.Gold, gs.Party.Food)

	appendRoster := func(title string, members []game.CharacterSave) {
		if len(members) == 0 {
			return
		}
		addp(mobStatHeader, "")
		addpHeader("%s", title)
		for _, cs := range members {
			addp(color.White, "%s  Lv.%d %s", cs.Name, cs.Level, character.CharacterClass(cs.Class).String())
			addp(mobStatHP, "  HP %d/%d   SP %d/%d", cs.HitPoints, cs.MaxHitPoints, cs.SpellPoints, cs.MaxSpellPoints)
			equipped := make(map[items.EquipSlot]items.Item, len(cs.Equipment))
			for _, eq := range cs.Equipment {
				equipped[items.EquipSlot(eq.Slot)] = eq.Item
			}
			for _, slot := range items.DisplayEquipSlots {
				it, ok := equipped[slot]
				if !ok || it.Name == "" {
					continue
				}
				game.RefreshItemFromConfig(&it)
				addpItem(it, "  %-9s %s", slot.DisplayName()+":", it.Name)
			}
			for _, qs := range cs.QuickSlots {
				it := qs.Item
				if it.Name == "" {
					continue
				}
				game.RefreshItemFromConfig(&it)
				addpItem(it, "  Quick %d:   %s", qs.Slot+1, it.Name)
			}
		}
	}
	appendRoster("Party", gs.Party.Members)
	appendRoster("Reserve", gs.Party.Reserve)
	appendRoster("Captive", gs.Party.Captive)

	addl := func(col color.Color, format string, args ...any) {
		loot = append(loot, saveDetailLine{text: fmt.Sprintf(format, args...), col: col})
	}
	addlHeader := func(format string, args ...any) {
		loot = append(loot, saveDetailLine{text: fmt.Sprintf(format, args...), col: color.White, header: true})
	}
	addlItem := func(it items.Item, format string, args ...any) {
		loot = append(loot, saveDetailLine{text: fmt.Sprintf(format, args...), col: game.ItemRarityColor(it), item: &it})
	}
	addlHeader("Inventory (%d)", len(gs.Party.Inventory))
	if len(gs.Party.Inventory) == 0 {
		addl(mobStatHeader, "(empty)")
	}
	for _, it := range gs.Party.Inventory {
		game.RefreshItemFromConfig(&it)
		addlItem(it, "  %s", it.Name)
	}

	addl(mobStatHeader, "")
	addlHeader("Card Collection")
	cards := 0
	for _, it := range gs.Party.CardCollectionItems {
		if it.Name == "" {
			continue
		}
		game.RefreshItemFromConfig(&it)
		addlItem(it, "  %s", it.Name)
		cards++
	}
	if cards == 0 {
		// Legacy saves carry only the key list.
		for _, key := range gs.Party.CardCollection {
			if key == "" {
				continue
			}
			addl(mobStatDefault, "  %s", key)
			cards++
		}
	}
	if cards == 0 {
		addl(mobStatHeader, "(no cards)")
	}
	return party, loot
}

// buildStashDetail renders the cross-save stash.json chest.
func buildStashDetail() (party, loot []saveDetailLine) {
	party = []saveDetailLine{
		{text: "Shared Stash", col: color.White},
		{text: "stash.json - one chest shared by every save", col: mobStatDefault},
		{text: "Managed in-game at the tavern (Manage your stash)", col: mobStatDefault},
	}
	s, err := stash.Load()
	if err != nil {
		loot = []saveDetailLine{{text: "failed to read stash: " + err.Error(), col: mobStatDamage}}
		return party, loot
	}
	addl := func(col color.Color, format string, args ...any) {
		loot = append(loot, saveDetailLine{text: fmt.Sprintf(format, args...), col: col})
	}
	appendBank := func(title string, slots []items.Item) {
		used := 0
		for _, it := range slots {
			if !stash.IsEmpty(it) {
				used++
			}
		}
		loot = append(loot, saveDetailLine{text: fmt.Sprintf("%s (%d/%d)", title, used, len(slots)), col: color.White, header: true})
		for i, it := range slots {
			if stash.IsEmpty(it) {
				addl(mobStatHeader, "  %d: -", i+1)
				continue
			}
			game.RefreshItemFromConfig(&it)
			loot = append(loot, saveDetailLine{text: fmt.Sprintf("  %d: %s", i+1, it.Name), col: game.ItemRarityColor(it), item: &it})
		}
	}
	appendBank("Item Slots", s.Slots[:])
	addl(mobStatHeader, "")
	appendBank("Card Slots", s.CardSlots[:])
	return party, loot
}

// prettySavedAt turns an RFC3339 stamp into "YYYY-MM-DD HH:MM".
func prettySavedAt(s string) string {
	s = strings.Replace(s, "T", " ", 1)
	if len(s) > 16 {
		s = s[:16]
	}
	return s
}

// savesButtons is the contextual action row above the detail panels; one
// geometry shared by draw and click handling.
func savesButtons() []saveButton {
	x := saveListW + contentPad
	y := pageBarHeight + 10
	var btns []saveButton
	add := func(id, label string) {
		w := len(label)*7 + 22
		btns = append(btns, saveButton{id: id, label: label, r: rect{x: x, y: y, w: w, h: saveButtonH}})
		x += w + 8
	}
	add("refresh", "Refresh")
	if entry, ok := selectedSaveEntry(); ok {
		switch entry.kind {
		case saveEntrySlot:
			if !game.IsAutosaveRow(entry.row) && entry.row < len(savesPage.rows) && savesPage.rows[entry.row].Exists {
				add("archive", "Add to Archive")
			}
		case saveEntryArchive:
			if savesPage.restoreArm {
				add("cancel", "Cancel Restore")
			} else {
				add("restore", "Restore Save")
			}
		}
	}
	return btns
}

// saveDetailPanels returns the two detail panel rects (party | loot).
func saveDetailPanels() (partyPanel, lootPanel rect) {
	top := pageBarHeight + 10 + saveButtonH + 24 // buttons row + status line
	w := (windowWidth - saveListW - 3*contentPad) / 2
	h := windowHeight - top - contentPad
	partyPanel = rect{x: saveListW + contentPad, y: top, w: w, h: h}
	lootPanel = rect{x: partyPanel.x + w + contentPad, y: top, w: w, h: h}
	return partyPanel, lootPanel
}

func saveListViewportH() int { return windowHeight - pageBarHeight - saveListPadY*2 }

func maxSaveListScroll() int {
	return len(savesPage.entries)*saveListRowH - saveListViewportH()
}

func maxDetailScroll(lines []saveDetailLine, panel rect) int {
	return len(lines)*saveDetailRowH - (panel.h - 16)
}

func clampScrollValue(v, max int) int {
	if max < 0 {
		max = 0
	}
	if v > max {
		return max
	}
	if v < 0 {
		return 0
	}
	return v
}

func (v *viewer) drawSavesPage(screen *ebiten.Image) {
	// Left: the slot + archive list.
	drawFilledRect(screen, 0, pageBarHeight, saveListW, windowHeight-pageBarHeight, color.RGBA{22, 22, 32, 255})
	list := screen.SubImage(image.Rect(0, pageBarHeight, saveListW, windowHeight)).(*ebiten.Image)
	y0 := pageBarHeight + saveListPadY - savesPage.listScroll
	for i, entry := range savesPage.entries {
		ry := y0 + i*saveListRowH
		if ry < pageBarHeight-saveListRowH || ry > windowHeight {
			continue
		}
		if i == savesPage.selIdx && entry.kind != saveEntryHeader {
			drawFilledRect(list, 0, ry-2, saveListW, saveListRowH, color.RGBA{60, 90, 140, 200})
		}
		col := mobStatDefault
		switch {
		case entry.kind == saveEntryHeader:
			// Section headers render on a filled band (editor/game convention).
			drawFilledRect(list, 4, ry-2, saveListW-8, saveListRowH, color.RGBA{40, 40, 60, 255})
			drawRectBorder(list, 4, ry-2, saveListW-8, saveListRowH, 1, color.RGBA{70, 70, 100, 255})
			col = color.RGBA{255, 255, 255, 255}
		case entry.dim:
			col = color.RGBA{110, 110, 125, 255}
		}
		// While restoring, spotlight the valid targets: free manual slots.
		if savesPage.restoreArm && entry.kind == saveEntrySlot && entry.dim && !game.IsAutosaveRow(entry.row) {
			col = mobStatHP
		}
		game.DrawShadedText(list, clipText(entry.label, saveListW-16), 8, ry, col)
	}

	// Action buttons + status line.
	for _, btn := range savesButtons() {
		drawFilledRect(screen, btn.r.x, btn.r.y, btn.r.w, btn.r.h, color.RGBA{40, 40, 55, 255})
		drawRectBorder(screen, btn.r.x, btn.r.y, btn.r.w, btn.r.h, 1, color.RGBA{90, 90, 115, 255})
		drawCenteredLabel(screen, btn.label, btn.r)
	}
	if savesPage.status != "" {
		col := mobStatHP
		if savesPage.statusIsErr {
			col = mobStatDamage
		}
		game.DrawShadedText(screen, savesPage.status, saveListW+contentPad, pageBarHeight+10+saveButtonH+6, col)
	}

	partyPanel, lootPanel := saveDetailPanels()
	hovered := drawSaveDetailPanel(screen, partyPanel, savesPage.partyLines, savesPage.partyScroll)
	if h := drawSaveDetailPanel(screen, lootPanel, savesPage.lootLines, savesPage.lootScroll); h != nil {
		hovered = h
	}
	if hovered != nil {
		mx, my := ebiten.CursorPosition()
		card := cardForSavedItem(*hovered)
		drawCardTooltip(screen, &card, mx, my, 0, windowWidth)
	}
}

// drawSaveDetailPanel renders one panel and returns the item under the cursor
// (nil when none), so the page can raise a tooltip over everything else.
func drawSaveDetailPanel(screen *ebiten.Image, panel rect, lines []saveDetailLine, scroll int) *items.Item {
	drawFilledRect(screen, panel.x, panel.y, panel.w, panel.h, color.RGBA{20, 20, 30, 255})
	drawRectBorder(screen, panel.x, panel.y, panel.w, panel.h, 1, color.RGBA{70, 70, 90, 255})
	if len(lines) == 0 {
		ebitenutil.DebugPrintAt(screen, "(nothing selected)", panel.x+10, panel.y+8)
		return nil
	}
	clip := screen.SubImage(image.Rect(panel.x, panel.y, panel.x+panel.w, panel.y+panel.h)).(*ebiten.Image)
	maxTextW := panel.w - 20
	for i, line := range lines {
		ry := panel.y + 8 + i*saveDetailRowH - scroll
		if ry < panel.y-saveDetailRowH || ry > panel.y+panel.h {
			continue
		}
		// Section headers render on a filled band (editor/game convention).
		if line.header {
			drawFilledRect(clip, panel.x+4, ry-2, panel.w-8, saveDetailRowH+2, color.RGBA{40, 40, 60, 255})
			drawRectBorder(clip, panel.x+4, ry-2, panel.w-8, saveDetailRowH+2, 1, color.RGBA{70, 70, 100, 255})
		}
		game.DrawShadedText(clip, clipText(line.text, maxTextW), panel.x+10, ry, line.col)
	}

	mx, my := ebiten.CursorPosition()
	if !pointInRect(mx, my, panel.x, panel.y+8, panel.w, panel.h-8) {
		return nil
	}
	idx := (my - panel.y - 8 + scroll) / saveDetailRowH
	if idx >= 0 && idx < len(lines) {
		return lines[idx].item
	}
	return nil
}

// cardForSavedItem resolves a saved item back to its YAML definition and
// renders it through the SAME card builders the Items/Spells pages use, so the
// hover tooltip in the saves browser can't drift from the catalog pages.
func cardForSavedItem(it items.Item) contentCard {
	switch it.Type {
	case items.ItemWeapon:
		if def, key, ok := config.GetWeaponDefinitionByName(it.Name); ok && def != nil {
			return weaponCard(titleCase(def.Category), key, def)
		}
	case items.ItemTrap:
		if def, ok := config.GetTrapDefinition(string(it.SpellEffect)); ok {
			return trapCard("Trap", string(it.SpellEffect), def)
		}
	case items.ItemBattleSpell, items.ItemUtilitySpell:
		if config.GlobalSpells != nil {
			key := string(it.SpellEffect)
			if def, ok := config.GlobalSpells.Spells[key]; ok && def != nil {
				return spellCard(titleCase(def.School), key, def)
			}
		}
	default:
		if def, key, ok := config.GetItemDefinitionByName(it.Name); ok && def != nil {
			return itemCard(wearableKindLabel(def), key, def)
		}
	}
	// No definition found (renamed/removed content): show what the save carries.
	return contentCard{name: it.Name, rarity: it.Rarity, description: it.Description}
}
