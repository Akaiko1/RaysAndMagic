package main

import (
	"fmt"
	"maps"
	"slices"

	"ugataima/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// A leaf is identified by both its group and source index: the same source can
// occur in multiple groups. Headers have index -1.
type catalogEntryRow struct {
	label, group string
	index        int
}

// Catalog content is immutable after boot; query/sort/fold changes rebuild only
// the derived rows. Maps/encounter edits invalidate caches via refreshLegend.
type catalogCache[K comparable, V any] struct {
	key   K
	value V
	valid bool
}

func (c *catalogCache[K, V]) get(key K, build func() V) V {
	if !c.valid || c.key != key {
		c.key, c.value, c.valid = key, build(), true
	}
	return c.value
}

type catalogListKey struct {
	query, group, order string
	revision, count     int
}

func (v *viewer) catalogGroupRows(prefix, query string, groups map[string][]catalogEntryRow, less func(a, b catalogEntryRow) bool) []catalogEntryRow {
	var rows []catalogEntryRow
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		entries := groups[group]
		slices.SortStableFunc(entries, func(a, b catalogEntryRow) int {
			if less(a, b) {
				return -1
			}
			if less(b, a) {
				return 1
			}
			return 0
		})
		id := prefix + group
		mark := "-"
		if v.browser.closed[id] && query == "" {
			mark = "+"
		}
		rows = append(rows, catalogEntryRow{fmt.Sprintf("%s %s (%d)", mark, group, len(entries)), id, -1})
		if mark == "-" {
			for _, entry := range entries {
				entry.group = id
				rows = append(rows, entry)
			}
		}
	}
	return rows
}

// catalogAllFolded reports whether rows list groups and every one is folded.
func (v *viewer) catalogAllFolded(rows []catalogEntryRow) bool {
	headers := false
	for _, row := range rows {
		if row.index < 0 {
			if !v.browser.closed[row.group] {
				return false
			}
			headers = true
		}
	}
	return headers
}

// foldCatalogGroups folds every listed group, or unfolds them all once every
// group is already folded.
func (v *viewer) foldCatalogGroups(rows []catalogEntryRow) {
	fold := !v.catalogAllFolded(rows)
	if v.browser.closed == nil {
		v.browser.closed = map[string]bool{}
	}
	for _, row := range rows {
		if row.index < 0 {
			v.browser.closed[row.group] = fold
		}
	}
	v.browser.revision++
}

func (v *viewer) catalogFoldControl(r rect, rows func() []catalogEntryRow) catalogControl {
	label := "Fold all"
	if v.catalogAllFolded(rows()) {
		label = "Unfold all"
	}
	return catalogControl{r, label, "", func() { v.foldCatalogGroups(rows()) }}
}

func catalogSelectedRow(rows []catalogEntryRow, selected catalogEntryRow, source int) int {
	fallback := -1
	for i, row := range rows {
		if row.index < 0 || row.index != source {
			continue
		}
		if row.group == selected.group && row.index == selected.index {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	return fallback
}

type catalogListInput struct {
	x, y, delta int
	wheel       float64
	click, held bool // left button just pressed / still down
}

func readCatalogListInput() catalogListInput {
	x, y := ebiten.CursorPosition()
	_, wheel := ebiten.Wheel()
	return catalogListInput{x, y, catalogNavigationDelta(), wheel,
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft), ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)}
}

// Both catalogs use the same occurrence-aware input path. Drag input on the
// map is separate and deliberately does not pass through this handler.
func (v *viewer) updateCatalogList(id string, r rect, rows []catalogEntryRow, scroll *int, selected *catalogEntryRow, source int, in catalogListInput, selectSource func(int)) {
	limit := max(0, len(rows)*catalogRowHeight-r.h)
	hover := pointInRect(in.x, in.y, r.x, r.y, r.w, r.h)
	if hover {
		*scroll -= int(in.wheel * catalogRowHeight * 3)
	}
	*scroll = clampInt(*scroll, 0, limit)
	var dragging bool
	if *scroll, dragging = v.dragCatalogScroll(id, r, *scroll, len(rows)*catalogRowHeight, in); dragging {
		return
	}
	i := -1
	if in.delta != 0 {
		i = catalogMoveSelection(rows, catalogSelectedRow(rows, *selected, source), in.delta)
		if i >= 0 {
			*scroll = clampInt(i*catalogRowHeight-r.h/2, 0, limit)
		}
	}
	if in.click && hover {
		i = (in.y - r.y + *scroll) / catalogRowHeight
	}
	if i < 0 || i >= len(rows) {
		return
	}
	row := rows[i]
	if row.index < 0 {
		v.toggleCatalogSection(row.group)
		return
	}
	*selected = row
	// Preserve reselect/replay behavior even when this is another occurrence
	// of the same source or the user clicks the already selected effect.
	selectSource(row.index)
}

func catalogNavigationDelta() int {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyDown):
		return 1
	case inpututil.IsKeyJustPressed(ebiten.KeyUp):
		return -1
	case inpututil.IsKeyJustPressed(ebiten.KeyPageDown):
		return 10
	case inpututil.IsKeyJustPressed(ebiten.KeyPageUp):
		return -10
	}
	return 0
}

// Move through visible leaf occurrences, not source indices.
func catalogMoveSelection(rows []catalogEntryRow, current, delta int) int {
	var leaves []int
	position := -1
	for i, row := range rows {
		if row.index >= 0 {
			if i == current {
				position = len(leaves)
			}
			leaves = append(leaves, i)
		}
	}
	if len(leaves) == 0 {
		return -1
	}
	return leaves[clampInt(position+delta, 0, len(leaves)-1)]
}

func drawCatalogList(screen *ebiten.Image, r rect, rows []catalogEntryRow, scroll, selected int) {
	list := catalogClip(screen, r)
	for i, row := range rows {
		y := r.y + i*catalogRowHeight - scroll
		if y < r.y-catalogRowHeight || y >= r.y+r.h {
			continue
		}
		fg := overlayText
		if row.index < 0 {
			drawFilledRect(list, r.x, y, r.w, catalogRowHeight, overlayButton)
			fg = overlayAccent
		} else if i == selected {
			drawFilledRect(list, r.x, y, r.w, catalogRowHeight, overlayButton)
			drawFilledRect(list, r.x, y, 3, catalogRowHeight, overlayAccent)
		}
		game.DrawShadedText(list, clipText(row.label, r.w-24), r.x+12, y+7, fg)
	}
	if len(rows) == 0 {
		game.DrawShadedText(list, "No matching entries", r.x+12, r.y+12, overlayMuted)
	}
	drawCatalogScroll(screen, r, scroll, len(rows)*catalogRowHeight)
}
