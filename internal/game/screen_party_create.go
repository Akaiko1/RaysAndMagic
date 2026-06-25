package game

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ---------------------------------------------------------------------------
// Party creation screen (AppScreenPartyCreate)
//
// The player drags 4 of the available heroes into the party slots; the rest are
// auto-assigned on confirm — the config-flagged captives go to the mountain
// prison (jail) and everyone else waits at the tavern. Dragging a hero onto an
// occupied slot swaps the two.
//
// Input (drag/drop, button clicks) is handled in updatePartyCreate (Update tick)
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
	pool       []*pcHero  // available heroes (drag source/target)
	slots      [4]*pcHero // chosen active party (nil = empty)
	detail     *pcHero    // hero shown in the detail panel
	jailTarget int        // how many leftovers go to jail (= configured captive count)

	// Drag state. drag is the hero following the cursor; its origin is recorded
	// so a drop can swap/return it. The hero is NOT removed from pool/slots
	// during the drag — it's skipped while drawing and a ghost follows the
	// cursor; the move is applied on release.
	drag         *pcHero
	dragFromSlot int // origin slot index, or -1
	dragFromPool int // origin pool index, or -1
}

// pcLayout is the computed geometry of the screen, shared by update and draw.
type pcLayout struct {
	slots  [4]rect
	pool   []rect
	detail rect
	begin  rect
	back   rect
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
	detailW := 320
	detailX := margin
	detailY := 44

	slotW, slotH, slotGap := 160, 190, 16
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
	cardW, cardH, cardGap := 120, 140, 14
	cols := poolW / (cardW + cardGap)
	if cols < 1 {
		cols = 1
	}
	lay.pool = make([]rect, len(pc.pool))
	for i := range pc.pool {
		col := i % cols
		row := i / cols
		lay.pool[i] = rect{poolX + col*(cardW+cardGap), poolY + row*(cardH+cardGap), cardW, cardH}
	}

	beginW, beginH := 220, 44
	lay.begin = rect{(w - beginW) / 2, h - beginH - 18, beginW, beginH}
	lay.back = rect{lay.begin.x - 150, lay.begin.y + 7, 110, 30}
	return lay
}

// updatePartyCreate handles all mouse + keyboard interaction (Update tick).
func (g *MMGame) updatePartyCreate() {
	pc := g.partyCreate
	if pc == nil {
		pc = newPartyCreateState(g.config)
		g.partyCreate = pc
	}

	backOut := func() {
		g.partyCreate = nil
		g.appScreen = AppScreenMainMenu
		g.entryMenuMode = EntryMenuRoot
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if pc.drag != nil {
			pc.clearDrag()
			return
		}
		backOut()
		return
	}

	lay := partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	mouseX, mouseY := ebiten.CursorPosition()

	// Resolve an in-progress drag on release.
	if pc.drag != nil {
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
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

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}

	// Buttons take priority over starting a drag.
	if pc.filledSlots() == 4 && lay.begin.contains(mouseX, mouseY) {
		g.beginAdventure(pc)
		return
	}
	if lay.back.contains(mouseX, mouseY) {
		backOut()
		return
	}

	// Grab from a slot.
	for i := 0; i < 4; i++ {
		if pc.slots[i] != nil && lay.slots[i].contains(mouseX, mouseY) {
			pc.drag = pc.slots[i]
			pc.dragFromSlot = i
			pc.dragFromPool = -1
			pc.detail = pc.drag
			return
		}
	}
	// Grab from the pool.
	for i, hero := range pc.pool {
		if lay.pool[i].contains(mouseX, mouseY) {
			pc.drag = hero
			pc.dragFromSlot = -1
			pc.dragFromPool = i
			pc.detail = hero
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
			// Slot→slot: clean swap (occupant may be nil).
			pc.slots[pc.dragFromSlot], pc.slots[dropSlot] = pc.slots[dropSlot], pc.slots[pc.dragFromSlot]
			return
		}
		// Pool→slot: occupant (if any) goes back to the pool.
		occ := pc.slots[dropSlot]
		pc.slots[dropSlot] = drag
		pc.removeFromPool(pc.dragFromPool)
		if occ != nil {
			pc.pool = append(pc.pool, occ)
		}
		return
	}
	// Dropped outside any slot → return to the pool.
	if pc.dragFromSlot >= 0 {
		pc.slots[pc.dragFromSlot] = nil
		pc.pool = append(pc.pool, drag)
	}
	// Pool→pool drop is a no-op (hero stays where it was).
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
	var active, jail, reserve []*character.MMCharacter
	for _, s := range pc.slots {
		if s != nil {
			active = append(active, s.char)
		}
	}

	leftover := make([]*pcHero, len(pc.pool))
	copy(leftover, pc.pool)
	used := make([]bool, len(leftover))

	// Prefer config-flagged captives for the jail, then fill to jailTarget from
	// the front of the remaining leftovers so the prison always holds the
	// configured number (keeps the rescue narrative intact).
	for i, hero := range leftover {
		if len(jail) >= pc.jailTarget {
			break
		}
		if hero.captiveFlag {
			jail = append(jail, hero.char)
			used[i] = true
		}
	}
	for i, hero := range leftover {
		if len(jail) >= pc.jailTarget {
			break
		}
		if !used[i] {
			jail = append(jail, hero.char)
			used[i] = true
		}
	}
	for i, hero := range leftover {
		if !used[i] {
			reserve = append(reserve, hero.char)
		}
	}

	party := character.NewPartyFromGroups(g.config, active, jail, reserve)
	g.partyCreate = nil
	g.startNewGameWithParty(party)
}

func (ui *UISystem) drawPartyCreateScreen(screen *ebiten.Image) {
	g := ui.game
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
	ebitenutil.DebugPrintAt(screen, "Assemble Your Party — drag four heroes into the slots", 24, 16)

	ui.drawHeroDetailPanel(screen, pc.detail, lay.detail)

	// Party slots.
	for i := 0; i < 4; i++ {
		r := lay.slots[i]
		hero := pc.slots[i]
		if hero != nil && hero != pc.drag {
			ui.drawHeroCard(screen, hero, r, hero == pc.detail) // card draws its own frame
		} else {
			ui.drawPanel(screen, "menu_panel_slot", r.x, r.y, r.w, r.h)
			drawCenteredDebugText(screen, fmt.Sprintf("Slot %d", i+1), r.x, r.y+r.h/2-8, r.w, 16)
		}
		if pc.drag != nil && r.contains(mouseX, mouseY) {
			drawRectBorder(screen, r.x, r.y, r.w, r.h, 3, color.RGBA{180, 210, 250, 220}) // valid drop target
		}
	}

	// Pool cards.
	for i, hero := range pc.pool {
		if hero == pc.drag {
			continue // following the cursor
		}
		ui.drawHeroCard(screen, hero, lay.pool[i], hero == pc.detail)
	}
	ebitenutil.DebugPrintAt(screen, "Available heroes", lay.detail.x+lay.detail.w+20, lay.detail.y-18)

	// Begin / Back buttons.
	if pc.filledSlots() == 4 {
		hover := lay.begin.contains(mouseX, mouseY)
		ui.drawMenuButton(screen, "begin", "Begin Adventure", lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h, hover)
	} else {
		drawFilledRect(screen, lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h, color.RGBA{40, 40, 50, 200})
		drawRectBorder(screen, lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h, 2, color.RGBA{80, 80, 90, 255})
		drawCenteredDebugText(screen, "Pick 4 heroes", lay.begin.x, lay.begin.y, lay.begin.w, lay.begin.h)
	}
	ui.drawMenuButton(screen, "back", "Back (Esc)", lay.back.x, lay.back.y, lay.back.w, lay.back.h, lay.back.contains(mouseX, mouseY))

	// Dragged card on top, following the cursor.
	if pc.drag != nil {
		ui.drawHeroCard(screen, pc.drag, rect{mouseX - 60, mouseY - 70, 120, 140}, true)
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
	img := ui.cardPortrait(name, w, h, 0)
	if img == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(img, op)
}

// drawHeroCard draws a compact portrait card (used for pool + slots + drag ghost).
func (ui *UISystem) drawHeroCard(screen *ebiten.Image, hero *pcHero, r rect, selected bool) {
	if hero == nil {
		return
	}
	ui.drawPanel(screen, "menu_panel_slot", r.x, r.y, r.w, r.h)

	// Portrait inset so the ornate frame border stays visible around it.
	portH := r.h - 40
	ui.drawPortraitCover(screen, ui.game.bigPortraitName(hero.char), r.x+14, r.y+14, r.w-28, portH-10)

	drawCenteredDebugText(screen, hero.char.Name, r.x, r.y+portH-8, r.w, 14)
	sub := hero.char.Class.String()
	if hero.entry.Race != "" {
		sub = titleCase(hero.entry.Race) + " " + sub
	}
	drawCenteredDebugText(screen, sub, r.x, r.y+portH+6, r.w, 14)

	if selected {
		drawRectBorder(screen, r.x+3, r.y+3, r.w-6, r.h-6, 2, color.RGBA{220, 225, 255, 255})
	}
}

// drawHeroDetailPanel renders the full stat/skill/equipment sheet for one hero.
func (ui *UISystem) drawHeroDetailPanel(screen *ebiten.Image, hero *pcHero, panel rect) {
	ui.drawPanel(screen, "menu_panel_tall", panel.x, panel.y, panel.w, panel.h)
	if hero == nil {
		drawCenteredDebugText(screen, "Select a hero", panel.x, panel.y+panel.h/2-8, panel.w, 16)
		return
	}
	c := hero.char

	// Large portrait (cover-fit, no distortion). Portrait-shaped box (4:5) so the
	// tall hero art crops minimally; inset to clear the ornate frame border.
	portW := panel.w - 44
	portH := portW * 5 / 4
	if max := panel.h - 250; portH > max && max > 0 {
		portH = max
	}
	ui.drawPortraitCover(screen, ui.game.bigPortraitName(c), panel.x+22, panel.y+22, portW, portH)

	tx := panel.x + 24
	ty := panel.y + portH + 34
	line := func(s string, col color.Color) {
		drawDebugTextColored(screen, s, tx, ty, col)
		ty += 16
	}
	white := color.RGBA{220, 220, 230, 255}
	gold := color.RGBA{220, 200, 140, 255}
	grey := color.RGBA{160, 160, 175, 255}

	race := "Human"
	if hero.entry.Race != "" {
		race = titleCase(hero.entry.Race)
	}
	line(fmt.Sprintf("%s — %s %s", c.Name, race, c.Class.String()), gold)
	line(fmt.Sprintf("Level %d   HP %d/%d   SP %d/%d", c.Level, c.HitPoints, c.MaxHitPoints, c.SpellPoints, c.MaxSpellPoints), white)
	ty += 4

	might, intellect, personality, endurance, accuracy, speed, luck := c.GetEffectiveStats()
	line(fmt.Sprintf("Might %2d   Intellect %2d", might, intellect), white)
	line(fmt.Sprintf("Person. %2d  Endurance %2d", personality, endurance), white)
	line(fmt.Sprintf("Accuracy %2d  Speed %2d  Luck %2d", accuracy, speed, luck), white)
	ty += 4

	line("Equipment:", gold)
	if it, ok := c.Equipment[items.SlotMainHand]; ok {
		line("  "+it.Name, grey)
	}
	if it, ok := c.Equipment[items.SlotArmor]; ok {
		line("  "+it.Name, grey)
	}
	ty += 4

	if len(c.Skills) > 0 {
		var names []string
		for st, sk := range c.Skills {
			names = append(names, fmt.Sprintf("%s (%s)", st.String(), sk.Mastery.String()))
		}
		sort.Strings(names)
		line("Skills:", gold)
		for _, n := range wrapToWidth(names, panel.w-28) {
			line("  "+n, grey)
		}
	}

	var schools []string
	for _, id := range character.AllMagicSchools {
		if ms, ok := c.MagicSchools[id]; ok {
			schools = append(schools, fmt.Sprintf("%s:%d", titleCase(string(id)), len(ms.KnownSpells)))
		}
	}
	if len(schools) > 0 {
		line("Magic (school:spells):", gold)
		for _, n := range wrapToWidth(schools, panel.w-28) {
			line("  "+n, grey)
		}
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

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
