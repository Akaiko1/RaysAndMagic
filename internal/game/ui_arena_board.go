package game

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/arena"
)

// sortedKeys returns a map's keys in stable sorted order (deterministic UI rows).
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// drawArenaGladiatorDialog is the tabbed gladiator dialog (gatekeeper outside,
// duel master inside): Talk = the NPC's dialogue choices, Shop = the arena
// points store, Board = the global champions' leaderboard. Folder tabs match
// the quest-giving spell trader's; the Tab key also cycles (see
// handleArenaGladiatorInput).
func (ui *UISystem) drawArenaGladiatorDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	ui.drawDialogFolderTabs(screen, dialogX, dialogY, []string{"Talk", "Shop", "Board"})
	switch ui.game.dialogTab {
	case 1:
		ui.drawMerchantDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case 2:
		ui.drawArenaBoardContent(screen, dialogX+20, dialogY+20, dialogY+dialogHeight-20)
	default:
		ui.game.arenaBoardScroll = 0   // leaving the board resets the view
		ui.game.arenaBoardStale = true // re-entry re-reads the board file
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	}
}

// arenaBoardLines flattens the leaderboard into display lines - compact one
// per party, expanded (detail) with member roster and per-champion victories.
// Building lines first makes scrolling trivial and overlap impossible: rows
// only ever push each other down.
func buildArenaBoardLines(detail bool) []string {
	board := arena.Load()
	if len(board.Entries) == 0 {
		return []string{"No victories recorded yet. The sand waits."}
	}
	lines := make([]string, 0, len(board.Entries)*2)
	for rank, e := range board.Entries {
		names := make([]string, 0, len(e.Members))
		roster := make([]string, 0, len(e.Members))
		for _, m := range e.Members {
			names = append(names, m.Name)
			roster = append(roster, fmt.Sprintf("%s (%s L%d)", m.Name, m.Class, m.Level))
		}
		lines = append(lines, fmt.Sprintf("%d. %s - %d champions, %d pts",
			rank+1, strings.Join(names, ", "), e.TotalKills(), e.TotalPoints))
		if !detail {
			continue
		}
		lines = append(lines, "   party: "+strings.Join(roster, ", "))
		for _, name := range sortedKeys(e.Kills) {
			byTier := e.Kills[name]
			pieces := make([]string, 0, len(byTier))
			for _, t := range sortedKeys(byTier) {
				pieces = append(pieces, fmt.Sprintf("%s x%d", t, byTier[t]))
			}
			lines = append(lines, fmt.Sprintf("     %s: %s", name, strings.Join(pieces, ", ")))
		}
		lines = append(lines, "")
	}
	return lines
}

// drawArenaBoardContent renders the scrollable leaderboard (mouse wheel; the
// scroll offset is clamped here against the current line count, so releasing
// Shift or a shrinking board self-heals the view).
func (ui *UISystem) drawArenaBoardContent(screen *ebiten.Image, x, y, maxY int) {
	drawDebugText(screen, "ARENA CHAMPIONS' BOARD (hold SHIFT for details, wheel to scroll)", x, y)
	y += 22

	detail := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	if ui.game.arenaBoardStale || ui.game.arenaBoardLines == nil || detail != ui.game.arenaBoardDetail {
		ui.game.arenaBoardLines = buildArenaBoardLines(detail)
		ui.game.arenaBoardDetail = detail
		ui.game.arenaBoardStale = false
	}
	lines := ui.game.arenaBoardLines
	const lineH = 14
	visible := (maxY - y) / lineH
	if visible < 1 {
		visible = 1
	}
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ui.game.arenaBoardScroll > maxScroll {
		ui.game.arenaBoardScroll = maxScroll
	}
	if ui.game.arenaBoardScroll < 0 {
		ui.game.arenaBoardScroll = 0
	}
	start := ui.game.arenaBoardScroll
	for i := start; i < len(lines) && i < start+visible; i++ {
		drawDebugText(screen, lines[i], x, y)
		y += lineH
	}
	if maxScroll > 0 {
		drawDebugText(screen, fmt.Sprintf("(%d-%d of %d)", start+1, min(start+visible, len(lines)), len(lines)), x, maxY+6)
	}
}
