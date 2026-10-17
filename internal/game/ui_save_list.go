package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
)

// saveListLayout is the save/load slot list: one geometry for the ESC Save and
// Load menus and the title Load page, shared by drawing, hover and clicks.
type saveListLayout struct {
	panel, title     layoutRect
	rows             [saveRowsPerPage]layoutRect
	prev, next, page layoutRect
	hintY            int
	back             layoutRect // the title page's Back; zero in the ESC menus
}

// makeSaveListLayout fills panel inside inset: a heading, the page's rows, the
// pager, then the key hint (and Back on the title page).
func makeSaveListLayout(panel layoutRect, inset int, withBack bool) saveListLayout {
	c := layoutRect{panel.x + inset, panel.y + inset, panel.w - 2*inset, panel.h - 2*inset}
	l := saveListLayout{panel: panel, title: layoutRect{c.x, c.y, c.w, saveListTitleH}}
	y := l.title.bottom() + saveListGap
	for i := range l.rows {
		l.rows[i] = layoutRect{c.x, y + i*saveListPitch, c.w, saveListRowH}
	}
	y = l.rows[saveRowsPerPage-1].bottom() + saveListGap
	l.prev = layoutRect{c.x, y, saveListPagerW, saveListPagerH}
	l.next = layoutRect{c.right() - saveListPagerW, y, saveListPagerW, saveListPagerH}
	l.page = layoutRect{l.prev.right(), y, l.next.x - l.prev.right(), saveListPagerH}
	y = l.prev.bottom() + saveListGap
	if withBack {
		l.back = layoutRect{c.x, y, menuBackButtonW, menuBackButtonH}
		l.hintY = y + (menuBackButtonH-uiTextCharHeight)/2
	} else {
		l.hintY = y
	}
	return l
}

func escSaveListLayout(screenW, screenH int) saveListLayout {
	return makeSaveListLayout(mainMenuPanelRect(screenW, screenH, MenuSaveSelect), saveMenuInset, false)
}

func entrySaveListLayout(screenW, screenH int) saveListLayout {
	return makeSaveListLayout(entryLoadPanelRect(screenW, screenH), menuFrameInset, true)
}

// rowAt is the page row under (x, y), -1 for none.
func (l saveListLayout) rowAt(x, y int) int {
	for i, r := range l.rows {
		if isMouseHoveringBox(x, y, r.x, r.y, r.right(), r.bottom()) {
			return i
		}
	}
	return -1
}

// The save lists' key hints.
const (
	saveMenuHint  = "Enter: Save | R or right-click: Rename | Left/Right: Page"
	loadMenuHint  = "Enter: Load | Left/Right: Page"
	entryLoadHint = "Click a save to load | Left/Right: Page"
)

// drawSaveList draws the heading, the current page's rows (focused marks the
// cursor row, -1 for none) and the pager, lit under the pointer only while
// live (a dialog over the list owns the pointer). saving greys out the
// load-only rows.
func (ui *UISystem) drawSaveList(screen *ebiten.Image, l saveListLayout, heading string, focused int, saving, live bool) {
	g := ui.game
	drawScaledMetalCenteredText(screen, heading, l.title.x+l.title.w/2, l.title.y+l.title.h/2, 2, rarityGold)
	for i, r := range l.rows {
		row := g.savePage*saveRowsPerPage + i
		ui.drawSaveListRow(screen, r, row, GetSaveRowSummary(row), i == focused, saving)
	}
	mx, my := uiCursorPosition()
	for _, b := range []struct {
		r     layoutRect
		label string
	}{{l.prev, "< Prev"}, {l.next, "Next >"}} {
		ui.drawMenuButton(screen, b.label, b.r.x, b.r.y, b.r.w, b.r.h, live && isMouseHoveringBox(mx, my, b.r.x, b.r.y, b.r.right(), b.r.bottom()))
	}
	drawCenteredTextWithShadow(screen, fmt.Sprintf("Page %d of %d", g.savePage+1, savePageCount), l.page.x, l.page.y, l.page.w, l.page.h, gameplayInk.muted)
}

// drawSaveListRow is one slot as a two-line entry: slot, save name (or its
// place) and when; then where, the party's levels, play time and the mode.
// A save from this playthrough carries a gold edge; the cursor row its frame.
func (ui *UISystem) drawSaveListRow(screen *ebiten.Image, r layoutRect, row int, sum SaveSummary, focused, saving bool) {
	if focused {
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, true)
	}
	if sum.Exists && sum.RunID != "" && sum.RunID == ui.game.playthroughID {
		drawFilledRect(screen, r.x, r.y+3, 3, r.h-6, buildsBarTones.accent)
	}
	slotW := uiTextWidth(saveRowLabel(quicksaveRow)) + 14
	x, right := r.x+10, r.right()-10
	line1, line2 := r.y+(r.h-2*uiTextCharHeight-2)/2, r.y+(r.h-2*uiTextCharHeight-2)/2+uiTextCharHeight+2
	loadOnly := saving && saveRowIsLoadOnly(row)
	nameX := x + slotW

	if !sum.Exists {
		// One line, centred: an empty slot has nothing for the second.
		y := r.y + (r.h-uiTextCharHeight)/2
		empty := "Empty"
		if saveRowIsLoadOnly(row) {
			empty = "Not written yet"
		}
		drawUITextColored(screen, saveRowLabel(row), x, y, gameplayInk.muted)
		drawUITextColored(screen, empty, nameX, y, gameplayInk.muted)
		if loadOnly {
			drawRightUIText(screen, "Load only", right, y, gameplayInk.muted)
		}
		return
	}
	drawUITextColored(screen, saveRowLabel(row), x, line1, gameplayInk.muted)
	when := saveTimeLabel(sum.SavedAt)
	nameInk := gameplayInk.text
	if focused {
		nameInk = gameplayInk.heading
	}
	if loadOnly {
		nameInk, when = gameplayInk.muted, "Load only"
	}
	place := mapDisplayName(sum.MapKey)
	title := sum.Name
	if title == "" || saveRowIsLoadOnly(row) {
		title = place
	}
	drawRightUIText(screen, when, right, line1, gameplayInk.text)
	drawUITextColored(screen, clipUIText(title, right-uiTextWidth(when)-12-nameX), nameX, line1, nameInk)

	var details []string
	if title != place {
		details = append(details, place)
	}
	if levels := saveLevelLabel(sum.Party); levels != "" {
		details = append(details, levels)
	}
	if sum.PlayTime != "" {
		details = append(details, sum.PlayTime)
	}
	mode := "Real-time"
	if sum.TurnBased {
		mode = "Turn-based"
	}
	drawRightUIText(screen, mode, right, line2, gameplayInk.muted)
	drawUITextColored(screen, clipUIText(strings.Join(details, " | "), right-uiTextWidth(mode)-12-nameX), nameX, line2, gameplayInk.muted)
}

// saveTimeLabel turns a save's RFC 3339 stamp into "17 Oct 11:49" (the year
// replaces the time for older years); anything else shows as written.
func saveTimeLabel(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return stamp
	}
	t = t.Local()
	if t.Year() != time.Now().Year() {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan 15:04")
}

// saveLevelLabel is the saved party's level span: "Lv 31" or "Lv 31-33".
func saveLevelLabel(party []SavePartyBrief) string {
	if len(party) == 0 {
		return ""
	}
	lo, hi := party[0].Level, party[0].Level
	for _, m := range party[1:] {
		lo, hi = min(lo, m.Level), max(hi, m.Level)
	}
	if lo == hi {
		return fmt.Sprintf("Lv %d", lo)
	}
	return fmt.Sprintf("Lv %d-%d", lo, hi)
}

// drawSaveRowHoverTooltip names the saved party under the cursor, and whether
// the save belongs to this playthrough.
func (ui *UISystem) drawSaveRowHoverTooltip(screen *ebiten.Image, l saveListLayout) {
	mx, my := uiCursorPosition()
	i := l.rowAt(mx, my)
	if i < 0 {
		return
	}
	sum := GetSaveRowSummary(ui.game.savePage*saveRowsPerPage + i)
	if !sum.Exists {
		return
	}
	var lines []string
	for _, m := range sum.Party {
		lines = append(lines, fmt.Sprintf("%s  Lv.%d %s", m.Name, m.Level, character.CharacterClass(m.Class).String()))
	}
	if sum.RunID != "" && sum.RunID == ui.game.playthroughID {
		lines = append(lines, "From this playthrough")
	}
	if len(lines) > 0 {
		ui.drawTooltipLines(screen, mx+16, my+16, lines)
	}
}

// drawTooltipLines renders a small bordered tooltip box of text lines, clamped to
// the screen so it never spills off the edge.
func (ui *UISystem) drawTooltipLines(screen *ebiten.Image, x, y int, lines []string) {
	sw, sh := uiBounds(screen).Dx(), uiBounds(screen).Dy()
	r := singleTooltipLayout(lines, nil, false, x, y, sw, sh)
	drawTooltip(screen, lines, nil, nil, nil, "", r.x, r.y, r.right(), ui.game.sprites)
}
