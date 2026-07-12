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
		ui.drawArenaBoardContent(screen, dialogX+20, dialogY+20, dialogX+dialogWidth-20, dialogY+dialogHeight-20)
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
func buildArenaBoardLines(detail bool, maxWidth int) []string {
	board := arena.Load()
	if len(board.Entries) == 0 {
		return wrapArenaBoardLine("No victories recorded yet. The sand waits.", maxWidth)
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
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapArenaBoardLine(line, maxWidth)...)
	}
	return wrapped
}

// wrapArenaBoardLine keeps leaderboard text inside the board frame while
// preserving the detail lines' indentation. Entries are generated from player
// names and champion keys, so their length cannot be bounded by authored UI
// copy.
func wrapArenaBoardLine(line string, maxWidth int) []string {
	if line == "" || debugTextWidth(line) <= maxWidth {
		return []string{line}
	}
	indentLen := 0
	for indentLen < len(line) && line[indentLen] == ' ' {
		indentLen++
	}
	indent := line[:indentLen]
	words := strings.Fields(line[indentLen:])
	lines := make([]string, 0, 1+len(words)/3)
	current := indent
	for _, word := range words {
		candidate := word
		if current != indent {
			candidate = current + " " + word
		} else {
			candidate = indent + word
		}
		if debugTextWidth(candidate) > maxWidth && current != indent {
			lines = append(lines, current)
			current = indent + word
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// drawArenaBoardContent renders the scrollable leaderboard (mouse wheel; the
// scroll offset is clamped here against the current line count, so releasing
// Shift or a shrinking board self-heals the view).
func (ui *UISystem) drawArenaBoardContent(screen *ebiten.Image, x, y, maxX, maxY int) {
	drawDebugText(screen, "ARENA CHAMPIONS' BOARD (hold SHIFT for details, wheel to scroll)", x, y)
	y += 22

	detail := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	width := maxX - x
	if ui.game.arenaBoardStale || ui.game.arenaBoardLines == nil || detail != ui.game.arenaBoardDetail || width != ui.game.arenaBoardWidth {
		ui.game.arenaBoardLines = buildArenaBoardLines(detail, width)
		ui.game.arenaBoardDetail = detail
		ui.game.arenaBoardWidth = width
		ui.game.arenaBoardStale = false
	}
	lines := ui.game.arenaBoardLines
	const lineH = debugTextCharHeight
	contentMaxY := maxY - lineH // reserve the final row for the scroll indicator
	visible := (contentMaxY - y) / lineH
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
		drawDebugText(screen, fmt.Sprintf("(%d-%d of %d)", start+1, min(start+visible, len(lines)), len(lines)), x, contentMaxY)
	}
}
