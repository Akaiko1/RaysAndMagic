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

type arenaBoardLine struct {
	text     string
	entryKey string
}

// buildArenaBoardLines flattens the leaderboard into display lines - compact one
// per party, expanded (detail) with member roster and per-champion victories.
// Each visual line retains its source entry so detail-mode rebuilds can keep
// the same leaderboard record centered instead of preserving a meaningless
// raw line offset.
func buildArenaBoardLines(detail bool, maxWidth int) []arenaBoardLine {
	return buildArenaBoardLinesFrom(arena.Load(), detail, maxWidth)
}

func buildArenaBoardLinesFrom(board *arena.Board, detail bool, maxWidth int) []arenaBoardLine {
	if len(board.Entries) == 0 {
		wrapped := wrapArenaBoardLine("No victories recorded yet. The sand waits.", maxWidth)
		lines := make([]arenaBoardLine, 0, len(wrapped))
		for _, line := range wrapped {
			lines = append(lines, arenaBoardLine{text: line})
		}
		return lines
	}
	lines := make([]arenaBoardLine, 0, len(board.Entries)*2)
	for rank, e := range board.Entries {
		entryKey := e.RunID
		if entryKey == "" {
			entryKey = fmt.Sprintf("rank:%d", rank)
		}
		names := make([]string, 0, len(e.Members))
		roster := make([]string, 0, len(e.Members))
		for _, m := range e.Members {
			names = append(names, m.Name)
			roster = append(roster, fmt.Sprintf("%s (%s L%d)", m.Name, m.Class, m.Level))
		}
		entryLines := []string{fmt.Sprintf("%d. %s - %d champions, %d pts",
			rank+1, strings.Join(names, ", "), e.TotalKills(), e.TotalPoints)}
		if !detail {
			for _, line := range wrapArenaBoardLine(entryLines[0], maxWidth) {
				lines = append(lines, arenaBoardLine{text: line, entryKey: entryKey})
			}
			continue
		}
		entryLines = append(entryLines, "   party: "+strings.Join(roster, ", "))
		for _, name := range sortedKeys(e.Kills) {
			byTier := e.Kills[name]
			pieces := make([]string, 0, len(byTier))
			for _, t := range sortedKeys(byTier) {
				pieces = append(pieces, fmt.Sprintf("%s x%d", t, byTier[t]))
			}
			entryLines = append(entryLines, fmt.Sprintf("     %s: %s", name, strings.Join(pieces, ", ")))
		}
		entryLines = append(entryLines, "")
		for _, source := range entryLines {
			for _, line := range wrapArenaBoardLine(source, maxWidth) {
				lines = append(lines, arenaBoardLine{text: line, entryKey: entryKey})
			}
		}
	}
	return lines
}

// wrapArenaBoardLine keeps leaderboard text inside the board frame while
// preserving the detail lines' indentation. Entries are generated from player
// names and champion keys, so their length cannot be bounded by authored UI
// copy.
func wrapArenaBoardLine(line string, maxWidth int) []string {
	if line == "" || debugTextWidth(line) <= maxWidth {
		return []string{line}
	}
	maxChars := maxWidth / debugTextCharWidth
	if maxChars < 1 {
		return nil
	}
	indentLen := len(line) - len(strings.TrimLeft(line, " "))
	if indentLen >= maxChars {
		indentLen = maxChars - 1
	}
	indent := strings.Repeat(" ", indentLen)
	fragments := wrapText(strings.TrimLeft(line, " "), maxChars-indentLen)
	for i := range fragments {
		fragments[i] = indent + fragments[i]
	}
	return fragments
}

func arenaBoardAnchor(lines []arenaBoardLine, scroll, visible int) string {
	if len(lines) == 0 {
		return ""
	}
	index := scroll + visible/2
	if index < 0 {
		index = 0
	}
	if index >= len(lines) {
		index = len(lines) - 1
	}
	return lines[index].entryKey
}

func arenaBoardScrollForAnchor(lines []arenaBoardLine, entryKey string, visible int) int {
	if entryKey == "" {
		return 0
	}
	first, last := -1, -1
	for i, line := range lines {
		if line.entryKey == entryKey {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return 0
	}
	scroll := (first+last)/2 - visible/2
	if scroll < 0 {
		return 0
	}
	return scroll
}

// arenaBoardWheelDelta returns the vertical intent for the board. macOS maps a
// Shift+wheel gesture to the horizontal axis, while Shift is also the board's
// detail modifier, so use x only in that mode.
func arenaBoardWheelDelta(wheelX, wheelY float64, detailHeld bool) float64 {
	if wheelY != 0 {
		return wheelY
	}
	if detailHeld {
		return wheelX
	}
	return 0
}

// arenaBoardScrollAfterWheel turns either a mouse-wheel notch or a fractional
// trackpad delta into one predictable page-step. Casting Wheel() to int loses
// small trackpad deltas entirely, leaving the board stuck one line off the top.
func arenaBoardScrollAfterWheel(scroll int, wheelY float64) int {
	switch {
	case wheelY < 0:
		return scroll + 3
	case wheelY > 0:
		scroll -= 3
		if scroll < 0 {
			return 0
		}
	}
	return scroll
}

// drawArenaBoardContent renders the scrollable leaderboard (mouse wheel; the
// scroll offset is clamped here against the current line count, so releasing
// Shift or a shrinking board self-heals the view).
func (ui *UISystem) drawArenaBoardContent(screen *ebiten.Image, x, y, maxX, maxY int) {
	drawDebugText(screen, "ARENA CHAMPIONS' BOARD (hold SHIFT for details, wheel to scroll)", x, y)
	y += 22

	detail := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	width := maxX - x
	const lineH = debugTextCharHeight
	contentMaxY := maxY - lineH // reserve the final row for the scroll indicator
	visible := (contentMaxY - y) / lineH
	if visible < 1 {
		visible = 1
	}
	if ui.game.arenaBoardStale || ui.game.arenaBoardLines == nil || detail != ui.game.arenaBoardDetail || width != ui.game.arenaBoardWidth {
		anchor := ""
		if !ui.game.arenaBoardStale {
			anchor = arenaBoardAnchor(ui.game.arenaBoardLines, ui.game.arenaBoardScroll, visible)
		}
		ui.game.arenaBoardLines = buildArenaBoardLines(detail, width)
		if anchor != "" {
			ui.game.arenaBoardScroll = arenaBoardScrollForAnchor(ui.game.arenaBoardLines, anchor, visible)
		}
		ui.game.arenaBoardDetail = detail
		ui.game.arenaBoardWidth = width
		ui.game.arenaBoardStale = false
	}
	lines := ui.game.arenaBoardLines
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
		drawDebugText(screen, lines[i].text, x, y)
		y += lineH
	}
	if maxScroll > 0 {
		drawDebugText(screen, fmt.Sprintf("(%d-%d of %d)", start+1, min(start+visible, len(lines)), len(lines)), x, contentMaxY)
	}
}
