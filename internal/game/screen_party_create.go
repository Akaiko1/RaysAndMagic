package game

import (
	"fmt"
	"image/color"
	"sort"
	uitext "ugataima/assets/text"
	"ugataima/internal/graphics"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------------------------------------------------------------------------
// Party creation screen (AppScreenPartyCreate)
//
// The player drags 4 of the available heroes into the party slots; the rest are
// auto-assigned on confirm - the config-flagged captives go to the mountain
// prison (jail) and everyone else waits at the tavern. Dragging a hero onto an
// occupied slot swaps the two.
//
// Input (drag/drop, button clicks) is registered by Draw and dispatched during Update
// so inpututil press/release fire reliably; drawPartyCreateScreen only renders.
// Both share the same layout via partyCreateLayout.
//
// Graphics-ready: portraits use the in-game HUD sprite keys (preferring the
// large "_full" art); the background uses the "screen_party_create_bg" hook with
// a procedural fallback. Panels/cards are drawn procedurally and can be reskinned.
// ---------------------------------------------------------------------------

// pcHero pairs a built hero with its roster entry so the screen can show
// name/class/race and remember whether it was a config-flagged captive.
type pcHero struct {
	char        *character.MMCharacter
	entry       config.RosterEntry
	captiveFlag bool
}

type rect struct{ x, y, w, h int }

func (r rect) contains(px, py int) bool {
	return px >= r.x && px < r.x+r.w && py >= r.y && py < r.y+r.h
}

// partyCreateState holds the transient state of the party-creation screen.
type partyCreateState struct {
	pool             []*pcHero  // available heroes (drag source/target)
	slots            [4]*pcHero // chosen active party (nil = empty)
	poolScroll       int
	detailScroll     int
	detailMaxScroll  int
	detailScrollHero *pcHero
	detail           *pcHero // hero shown in the detail panel
	jailTarget       int     // how many leftovers go to jail (= configured captive count)

	// Drag state. drag is the hero following the cursor; its origin is recorded
	// so a drop can swap/return it. The hero is NOT removed from pool/slots
	// during the drag - it's skipped while drawing and a ghost follows the
	// cursor; the move is applied on release.
	drag         *pcHero
	dragFromSlot int // origin slot index, or -1
	dragFromPool int // origin pool index, or -1

	// Pending press: a left-press records the candidate hero + origin and selects
	// it immediately (detail panel). The drag only begins once the cursor moves
	// past pcDragThreshold while held - a press/release within the threshold is a
	// plain click (selection only, no move).
	pending     *pcHero
	pendingSlot int
	pendingPool int
	pressX      int
	pressY      int
}

// pcDragThreshold is the cursor travel (px) that turns a held press into a drag.
const pcDragThreshold = 6

// pcLayout is the computed geometry of the screen, shared by update and draw.
type pcLayout struct {
	slots            [4]rect
	pool             []rect
	poolArea         rect
	poolUp, poolDown rect
	poolMaxScroll    int
	detail           rect
	begin            rect
	back             rect
}

// newPartyCreateState builds the full hero pool from config and pre-fills the
// party slots with the default starting roster (the player can rearrange).
func newPartyCreateState(cfg *config.Config) *partyCreateState {
	active, captives, recruits := character.StartingRoster(cfg)
	pc := &partyCreateState{jailTarget: len(captives), dragFromSlot: -1, dragFromPool: -1}

	build := func(e config.RosterEntry, captive bool) *pcHero {
		c := character.CreateRosterCharacter(e, cfg)
		if c == nil {
			return nil
		}
		return &pcHero{char: c, entry: e, captiveFlag: captive}
	}

	for i, e := range active {
		if i >= 4 {
			break
		}
		if hero := build(e, false); hero != nil {
			pc.slots[i] = hero
		}
	}
	for _, e := range captives {
		if hero := build(e, true); hero != nil {
			pc.pool = append(pc.pool, hero)
		}
	}
	for _, e := range recruits {
		if hero := build(e, false); hero != nil {
			pc.pool = append(pc.pool, hero)
		}
	}
	pc.detail = pc.slots[0]
	return pc
}

func (pc *partyCreateState) clearDrag() {
	pc.drag = nil
	pc.dragFromSlot = -1
	pc.dragFromPool = -1
}

// beginPending records a candidate for a possible drag and selects it now.
func (pc *partyCreateState) beginPending(hero *pcHero, fromSlot, fromPool, x, y int) {
	pc.pending = hero
	pc.pendingSlot = fromSlot
	pc.pendingPool = fromPool
	pc.pressX, pc.pressY = x, y
	pc.detail = hero // click selects immediately
}

func (pc *partyCreateState) clearPending() {
	pc.pending = nil
	pc.pendingSlot = -1
	pc.pendingPool = -1
}

func (pc *partyCreateState) filledSlots() int {
	n := 0
	for _, s := range pc.slots {
		if s != nil {
			n++
		}
	}
	return n
}

// partyCreateLayout computes all hit/draw rectangles for the given screen size.
func partyCreateLayout(pc *partyCreateState, w, h int) pcLayout {
	const margin = 20
	detailW := min(320, max(180, w/4))
	detailX := margin
	detailY := 44

	slotH := min(210, h*28/100)
	slotW, slotGap := slotH*2/3, 16
	slotsTotalW := 4*slotW + 3*slotGap
	slotsX := (w - slotsTotalW) / 2
	slotsY := h - slotH - 78

	var lay pcLayout
	for i := 0; i < 4; i++ {
		lay.slots[i] = rect{slotsX + i*(slotW+slotGap), slotsY, slotW, slotH}
	}
	lay.detail = rect{detailX, detailY, detailW, slotsY - detailY - 10}

	poolX := detailX + detailW + 20
	poolY := detailY
	poolW := w - margin - poolX
	const cardGap = 12
	poolH := slotsY - poolY - 16
	// Keep cards readable; overflow uses complete rows instead of shrinking art.
	cols := max(1, (poolW+cardGap)/(144+cardGap))
	cardW := min(144, (poolW-(cols-1)*cardGap)/cols)
	cardH := cardW * 3 / 2
	visibleRows := max(1, (poolH+cardGap)/(cardH+cardGap))
	rows := (len(pc.pool) + cols - 1) / cols
	lay.poolMaxScroll = max(0, rows-visibleRows)
	start := min(max(0, pc.poolScroll), lay.poolMaxScroll)
	lay.poolArea = rect{poolX, poolY, poolW, poolH}
	lay.poolUp = rect{poolX + poolW - 64, poolY - 28, 28, 24}
	lay.poolDown = rect{poolX + poolW - 30, poolY - 28, 28, 24}
	lay.pool = make([]rect, len(pc.pool))
	for i := range pc.pool {
		row := i/cols - start
		if row >= 0 && row < visibleRows {
			lay.pool[i] = rect{poolX + (i%cols)*(cardW+cardGap), poolY + row*(cardH+cardGap), cardW, cardH}
		}
	}

	beginW, beginH := 220, 44
	lay.begin = rect{(w - beginW) / 2, h - beginH - 18, beginW, beginH}
	lay.back = rect{lay.begin.x - 150, lay.begin.y + 7, 110, 30}
	return lay
}

func (g *MMGame) leavePartyCreate() {
	g.partyCreate = nil
	g.appScreen = AppScreenMainMenu
	g.entryMenuMode = EntryMenuRoot
}

// updatePartyCreate handles keyboard cancellation; pointer gestures are
// registered by the displayed party-creation screen.
func (g *MMGame) updatePartyCreate() {
	pc := g.partyCreate
	if pc == nil {
		pc = newPartyCreateState(g.config)
		g.partyCreate = pc
	}

	mx, my := pointerPosition()
	lay := partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	if pc.drag == nil && pc.pending == nil {
		_, wheel := pointerWheel()
		if lay.detail.contains(mx, my) {
			pc.detailScroll = min(pc.detailMaxScroll, max(0, pc.detailScroll-int(wheel*32)))
		} else if lay.poolArea.contains(mx, my) {
			pc.poolScroll = rosterScrollAfterWheel(pc.poolScroll, lay.poolMaxScroll, wheel)
		}
	}
	if pointerCancelJustPress() {
		if pc.drag != nil {
			pc.clearDrag()
			return
		}
		if pc.pending != nil {
			pc.clearPending()
			return
		}
		g.leavePartyCreate()
		return
	}
}

func (g *MMGame) updatePartyCreatePointer() {
	pc := g.partyCreate
	if pc == nil {
		return
	}

	lay := partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	mouseX, mouseY := pointerPosition()

	// Resolve an in-progress drag on release.
	if pc.drag != nil {
		if pointerLeftJustRelease() {
			dropSlot := -1
			for i := 0; i < 4; i++ {
				if lay.slots[i].contains(mouseX, mouseY) {
					dropSlot = i
					break
				}
			}
			pc.applyDrop(dropSlot)
			pc.clearDrag()
		}
		return
	}

	// A press is pending: promote to a drag once the cursor moves past the
	// threshold; a release before then was just a click (already selected).
	if pc.pending != nil {
		if !pointerLeftPressed() {
			pc.clearPending()
			return
		}
		dx, dy := mouseX-pc.pressX, mouseY-pc.pressY
		if dx*dx+dy*dy >= pcDragThreshold*pcDragThreshold {
			pc.drag = pc.pending
			pc.dragFromSlot = pc.pendingSlot
			pc.dragFromPool = pc.pendingPool
			pc.clearPending()
		}
		return
	}

	if !pointerLeftJustPressed() {
		return
	}

	// Buttons take priority over selecting/dragging a hero.
	if pc.filledSlots() == 4 && lay.begin.contains(mouseX, mouseY) {
		g.beginAdventure(pc)
		return
	}
	if lay.back.contains(mouseX, mouseY) {
		g.leavePartyCreate()
		return
	}

	if lay.poolMaxScroll > 0 {
		if lay.poolUp.contains(mouseX, mouseY) {
			pc.poolScroll = max(0, min(pc.poolScroll, lay.poolMaxScroll)-1)
			return
		}
		if lay.poolDown.contains(mouseX, mouseY) {
			pc.poolScroll = min(lay.poolMaxScroll, max(0, pc.poolScroll)+1)
			return
		}
	}

	// Press on a slot/pool hero: select it and arm a possible drag.
	for i := 0; i < 4; i++ {
		if pc.slots[i] != nil && lay.slots[i].contains(mouseX, mouseY) {
			pc.beginPending(pc.slots[i], i, -1, mouseX, mouseY)
			return
		}
	}
	for i, hero := range pc.pool {
		if lay.pool[i].contains(mouseX, mouseY) {
			pc.beginPending(hero, -1, i, mouseX, mouseY)
			return
		}
	}
}

// applyDrop moves the dragged hero to dropSlot (or back to the pool if -1),
// swapping with any occupant.
func (pc *partyCreateState) applyDrop(dropSlot int) {
	drag := pc.drag
	if drag == nil {
		return
	}
	if dropSlot >= 0 {
		if pc.dragFromSlot >= 0 {
			// Slot->slot: clean swap (occupant may be nil).
			pc.slots[pc.dragFromSlot], pc.slots[dropSlot] = pc.slots[dropSlot], pc.slots[pc.dragFromSlot]
			return
		}
		// Pool->slot: occupant (if any) goes back to the pool.
		occ := pc.slots[dropSlot]
		pc.slots[dropSlot] = drag
		pc.removeFromPool(pc.dragFromPool)
		if occ != nil {
			pc.pool = append(pc.pool, occ)
		}
		return
	}
	// Dropped outside any slot -> return to the pool.
	if pc.dragFromSlot >= 0 {
		pc.slots[pc.dragFromSlot] = nil
		pc.pool = append(pc.pool, drag)
	}
	// Pool->pool drop is a no-op (hero stays where it was).
}

func (pc *partyCreateState) removeFromPool(idx int) {
	if idx < 0 || idx >= len(pc.pool) {
		return
	}
	pc.pool = append(pc.pool[:idx], pc.pool[idx+1:]...)
}

// beginAdventure assigns leftovers to jail/tavern per the config-captive rule
// and starts a new game with the chosen party.
func (g *MMGame) beginAdventure(pc *partyCreateState) {
	var active []*character.MMCharacter
	for _, s := range pc.slots {
		if s != nil {
			active = append(active, s.char)
		}
	}

	// The jail/reserve split (captives-first, fill to jailTarget) is a domain
	// rule - delegate it to the character package; the UI only maps its heroes.
	leftovers := make([]character.LeftoverHero, 0, len(pc.pool))
	for _, h := range pc.pool {
		leftovers = append(leftovers, character.LeftoverHero{Char: h.char, Captive: h.captiveFlag})
	}
	jail, reserve := character.PartitionLeftovers(leftovers, pc.jailTarget)

	party := character.NewPartyFromGroups(g.config, active, jail, reserve)
	g.partyCreate = nil
	g.startNewGameWithParty(party)
}

func (ui *UISystem) drawPartyCreateScreen(screen *ebiten.Image) {
	g := ui.game
	ui.onDisplayedInput(uiCommandPointer, layoutRect{}, g.updatePartyCreatePointer)
	pc := g.partyCreate
	if pc == nil {
		pc = newPartyCreateState(g.config)
		g.partyCreate = pc
	}
	w := g.config.GetScreenWidth()
	h := g.config.GetScreenHeight()
	lay := partyCreateLayout(pc, w, h)
	mouseX, mouseY := ebiten.CursorPosition()

	ui.drawScreenBackdrop(screen, w, h, "screen_party_create_bg")
	drawDebugText(screen, "To pick a hero, drag it into a party slot below", 24, 8)

	ui.drawHeroDetailPanel(screen, pc.detail, lay.detail)

	// Party slots.
	for i := 0; i < 4; i++ {
		r := lay.slots[i]
		hero := pc.slots[i]
		if hero != nil && hero != pc.drag {
			ui.drawHeroCard(screen, hero, r, hero == pc.detail) // card draws its own frame
		} else {
			drawPortraitFrame(screen, r.x, r.y, r.w, r.h)
			drawCenteredDebugText(screen, fmt.Sprintf("Slot %d", i+1), r.x, r.y+r.h/2-8, r.w, 16)
		}
		if pc.drag != nil && r.contains(mouseX, mouseY) {
			drawRectBorder(screen, r.x, r.y, r.w, r.h, 3, color.RGBA{180, 210, 250, 220}) // valid drop target
		}
	}

	// Pool cards.
	for i, hero := range pc.pool {
		if hero == pc.drag || lay.pool[i].w == 0 {
			continue // following the cursor or outside the visible rows
		}
		ui.drawHeroCard(screen, hero, lay.pool[i], hero == pc.detail)
	}
	drawDebugText(screen, "Available heroes", lay.detail.x+lay.detail.w+20, lay.detail.y-18)

	if lay.poolMaxScroll > 0 {
		ui.drawScrollArrowButton(screen, lay.poolUp.x, lay.poolUp.y, lay.poolUp.w, lay.poolUp.h, true, lay.poolUp.contains(mouseX, mouseY), pc.poolScroll > 0)
		ui.drawScrollArrowButton(screen, lay.poolDown.x, lay.poolDown.y, lay.poolDown.w, lay.poolDown.h, false, lay.poolDown.contains(mouseX, mouseY), pc.poolScroll < lay.poolMaxScroll)
	}

	// Begin / Back buttons.
	if pc.filledSlots() == 4 {
		hover := lay.begin.contains(mouseX, mouseY)
		ui.drawMenuButton(screen, "Begin Adventure", lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h, hover)
	} else {
		ui.drawButtonFrame(screen, lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h, false)
		drawFilledRect(screen, lay.begin.x+3, lay.begin.y+3, lay.begin.w-6, lay.begin.h-6, color.RGBA{0, 0, 0, 100})
		drawCenteredDebugText(screen, "Pick 4 heroes", lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h)
	}
	ui.drawMenuButton(screen, "Back (Esc)", lay.back.x, lay.back.y, lay.back.w, lay.back.h, lay.back.contains(mouseX, mouseY))

	// Dragged card on top, following the cursor.
	if pc.drag != nil {
		ui.drawHeroCard(screen, pc.drag, rect{mouseX - 60, mouseY - 90, 120, 180}, true)
	}
}

// bigPortraitName resolves the largest available portrait: the "_full" art when
// it exists, else the base/class sprite (which is already full-size cinematic
// art for class-fallback heroes). Avoids a missing-"_full" placeholder.
func (g *MMGame) bigPortraitName(c *character.MMCharacter) string {
	if full := g.fullPortraitSpriteName(c); g.sprites.HasSprite(full) {
		return full
	}
	return g.portraitSpriteName(c)
}

// drawPortraitCover draws a name's portrait cover-fit (filled, centered, no
// distortion) into the given box.
func (ui *UISystem) drawPortraitCover(screen *ebiten.Image, name string, x, y, w, h int) {
	img := ui.cardPortrait(name, w, h, false)
	if img == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, op)
}

const (
	partyHeroDetailPortraitInset = 22
)

// drawHeroCard draws a compact portrait card (used for pool + slots + drag ghost).
func (ui *UISystem) drawHeroCard(screen *ebiten.Image, hero *pcHero, r rect, selected bool) {
	if hero == nil {
		return
	}
	if selected {
		drawHeroCardSelectionGlow(screen, r)
	}
	rarity := hero.cardRarity(ui.game.config)
	tint := rarityRGBA(rarity)
	// Hero cards use bronze for common; higher tiers share the item palette.
	if rarity == "common" {
		tint = color.RGBA{190, 130, 72, 255}
	}
	// Source: 320x496. Keep the picture inside the clipped corners;
	// the lower quarter is the name/class area. Tint only the card stock.
	if ui.game.sprites.HasSprite("theme_hero_card") {
		op := &ebiten.DrawImageOptions{}
		op.ColorScale.Scale(float32(tint.R)/255, float32(tint.G)/255, float32(tint.B)/255, 1)
		graphics.DrawImageScaled(screen, ui.game.sprites.GetSprite("theme_hero_card"), float64(r.x), float64(r.y), float64(r.w), float64(r.h), op)
	} else {
		drawPortraitFrame(screen, r.x, r.y, r.w, r.h)
	}
	// Repaint the existing outer rail with the item rarity color. Multiplying
	// the shaded silver source alone turns gold olive and legendary red brown.
	cut := max(3, r.w*17/320)
	points := [][2]int{{r.x + cut, r.y + 1}, {r.x + r.w - cut, r.y + 1},
		{r.x + r.w - 2, r.y + cut}, {r.x + r.w - 2, r.y + r.h - cut},
		{r.x + r.w - cut, r.y + r.h - 2}, {r.x + cut, r.y + r.h - 2},
		{r.x + 1, r.y + r.h - cut}, {r.x + 1, r.y + cut}}
	for i, p := range points {
		next := points[(i+1)%len(points)]
		vector.StrokeLine(screen, float32(p[0]), float32(p[1]), float32(next[0]), float32(next[1]), 2, tint, false)
	}
	picture := heroCardPortraitRect(r)
	ui.drawPortraitCover(screen, ui.game.bigPortraitName(hero.char), picture.x, picture.y, picture.w, picture.h)
	textY := r.y + r.h*75/100 + 1
	drawCenteredTextWithShadow(screen, clipDebugText(hero.char.Name, r.w-8), r.x+4, textY, r.w-8, 12, color.RGBA{242, 232, 210, 255})
	drawCenteredTextWithShadow(screen, clipDebugText(hero.char.Class.String(), r.w-8), r.x+4, textY+12, r.w-8, 12, color.RGBA{201, 199, 194, 255})
	symbol := "theme_rarity_" + rarity
	if rarity == "legendary" {
		symbol = "theme_rarity_gem"
	}
	if ui.game.sprites.HasSprite(symbol) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(symbol), r.x+r.w-28, r.y+6, 24, 36)
	}

}

const (
	heroCardSelectionGlowSpread = 10
	heroCardSelectionGlowPeak   = 92
)

// heroCardSelectionGlowTint is the cool highlight of a picked hero card.
var heroCardSelectionGlowTint = color.RGBA{145, 190, 255, 255}

// drawHeroCardSelectionGlow rings a picked card with the shared soft halo
// (ui_helpers.go) - the same generator the party HUD's progression badges use.
func drawHeroCardSelectionGlow(screen *ebiten.Image, r rect) {
	drawSoftGlowAround(screen, r.x, r.y, r.w, r.h,
		heroCardSelectionGlowSpread, heroCardSelectionGlowTint, heroCardSelectionGlowPeak, 1)
}

// drawHeroDetailPanel renders the full stat/skill/equipment sheet for one hero.
func (ui *UISystem) drawHeroDetailPanel(screen *ebiten.Image, hero *pcHero, panel rect) {
	ui.drawThemeFrame(screen, frameSilver, panel.x, panel.y, panel.w, panel.h)
	ui.drawCornerDecor(screen, frameSilver, panel.x-8, panel.y-8, panel.w+16, panel.h+16, decorAllCorners)
	ui.drawPanelInlay(screen, frameSilver, panel.x+panel.w/2, panel.y)
	if hero == nil {
		drawCenteredDebugText(screen, "Select a hero", panel.x, panel.y+panel.h/2-8, panel.w, 16)
		return
	}
	c := hero.char

	// Keep a readable portrait above the scrollable details at every size.
	portW := panel.w - 2*partyHeroDetailPortraitInset
	portH := min(portW*5/4, max(80, panel.h/3))
	ui.drawPortraitCover(screen, ui.game.bigPortraitName(c),
		panel.x+partyHeroDetailPortraitInset, panel.y+partyHeroDetailPortraitInset,
		portW, portH)

	pc := ui.game.partyCreate
	if pc.detailScrollHero != hero {
		pc.detailScrollHero = hero
		pc.detailScroll = 0
	}
	tx := panel.x + 16
	textTop := panel.y + portH + 30
	contentBottom := panel.y + panel.h - 24
	ty := textTop - pc.detailScroll
	line := func(s string, col color.Color) {
		for _, text := range wrapDebugText(s, panel.w-32) {
			if ty >= textTop && ty+debugTextCharHeight <= contentBottom {
				drawDebugTextColored(screen, text, tx, ty, col)
			}
			ty += 16
		}
	}
	// Text right edge mirrors the left inset. wrapTokens packs
	// tokens so the DRAWN string (prefix + content) fits, measuring the prefix the
	// caller adds - otherwise lists run past the frame's right border.
	maxLineW := panel.w - 32
	wrapTokens := func(prefix, cont string, tokens []string, col color.Color) {
		budget := maxLineW - debugTextWidth(cont)
		if pb := maxLineW - debugTextWidth(prefix); pb < budget {
			budget = pb
		}
		for i, ln := range wrapToWidth(tokens, budget) {
			if i == 0 {
				line(prefix+ln, col)
			} else {
				line(cont+ln, col)
			}
		}
	}
	white := color.RGBA{220, 220, 230, 255}
	gold := color.RGBA{220, 200, 140, 255}
	grey := color.RGBA{160, 160, 175, 255}

	race := "Human"
	if hero.entry.Race != "" {
		race = humanizeKey(hero.entry.Race)
	}
	line(fmt.Sprintf("%s - %s %s", c.Name, race, c.Class.String()), gold)
	line(fmt.Sprintf("Level %d   HP %d/%d   SP %d/%d", c.Level, c.HitPoints, c.MaxHitPoints, c.SpellPoints, c.MaxSpellPoints), white)
	ty += 4

	might, intellect, personality, endurance, accuracy, speed, luck := c.GetEffectiveStats()
	line(fmt.Sprintf("Might %2d   Intellect %2d", might, intellect), white)
	line(fmt.Sprintf("Person. %2d  Endurance %2d", personality, endurance), white)
	line(fmt.Sprintf("Accuracy %2d  Speed %2d  Luck %2d", accuracy, speed, luck), white)
	ty += 4

	line("Equipment:", gold)
	for _, slot := range items.DisplayEquipSlots {
		if it, ok := c.Equipment[slot]; ok {
			line("  "+it.Name, grey)
		}
	}
	ty += 4

	if len(c.Skills) > 0 {
		var names []string
		for st, sk := range c.Skills {
			names = append(names, fmt.Sprintf("%s (%s)", st.String(), sk.Mastery.String()))
		}
		sort.Strings(names)
		line("Skills:", gold)
		wrapTokens("  ", "  ", names, grey)
	}

	hasMagic := false
	for _, id := range character.AllMagicSchools {
		if _, ok := c.MagicSchools[id]; ok {
			hasMagic = true
			break
		}
	}
	if hasMagic {
		line("Magic:", gold)
		for _, id := range character.AllMagicSchools {
			ms, ok := c.MagicSchools[id]
			if !ok {
				continue
			}
			label := humanizeKey(string(id))
			if len(ms.KnownSpells) == 0 {
				line("  "+label, grey)
				continue
			}
			var spellNames []string
			for _, sid := range ms.KnownSpells {
				if def, err := spells.GetSpellDefinitionByID(sid); err == nil && def.Name != "" {
					spellNames = append(spellNames, def.Name)
				} else {
					spellNames = append(spellNames, humanizeKey(string(sid)))
				}
			}
			wrapTokens("  "+label+": ", "      ", spellNames, grey)
		}
	}
	pc.detailMaxScroll = max(0, ty+pc.detailScroll-contentBottom)
	pc.detailScroll = min(pc.detailScroll, pc.detailMaxScroll)
	if pc.detailMaxScroll > 0 {
		drawCenteredDebugText(screen, uitext.Text("ui.scroll_details"), panel.x+8, panel.y+panel.h-20, panel.w-16, 14)
	}

}

// wrapToWidth packs comma-joined tokens into lines that fit maxPx in the debug
// font, returning each line's text.
func wrapToWidth(tokens []string, maxPx int) []string {
	var lines []string
	cur := ""
	for _, t := range tokens {
		cand := t
		if cur != "" {
			cand = cur + ", " + t
		}
		if debugTextWidth(cand) > maxPx && cur != "" {
			lines = append(lines, cur)
			cur = t
		} else {
			cur = cand
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// Class overrides live beside the class definition. Standard classes derive
// their display rarity from race; gameplay progression never reads this value.
func (hero *pcHero) cardRarity(cfg *config.Config) string {
	if rarity := cfg.Characters.Classes[hero.entry.Class].CardRarity; rarity != "" {
		return rarity
	}
	if hero.entry.Race != "" && hero.entry.Race != "human" {
		return "uncommon"
	}
	return "common"
}

func heroCardPortraitRect(r rect) rect {
	pad := max(6, r.w*20/320)
	return rect{r.x + pad, r.y + pad, r.w - 2*pad, r.h*73/100 - pad}
}
