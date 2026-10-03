package character

import "strings"

// CardRowKind is authored by the card producer, independent of its wording.
type CardRowKind uint8

const (
	CardRowBody CardRowKind = iota
	CardRowTitle
	CardRowCategory
	CardRowSection
	CardRowResult
	CardRowDetail
	CardRowDescription
	CardRowFlavor
	CardRowHint
	CardRowSpacer
)

// CardRow carries its role through layout and wrapping. Section identifies the
// mechanic owning a row, including equipment-set presentation overrides.
type CardRow struct {
	Text    string
	Kind    CardRowKind
	Section string
}

type CardRows []CardRow

// Add splits authored multiline text while keeping the role on each fragment.
func (rows *CardRows) Add(kind CardRowKind, text string) {
	for _, line := range strings.Split(text, "\n") {
		role := kind
		if line == "" {
			role = CardRowSpacer
		}
		row := CardRow{Text: line, Kind: role}
		if role == CardRowSection {
			row.Section = line
		}
		*rows = append(*rows, row)
	}
}

func (rows CardRows) Lines() []string {
	if rows == nil {
		return nil
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = row.Text
	}
	return lines
}

func (rows CardRows) String() string { return strings.Join(rows.Lines(), "\n") }

// PlainCardRows adapts unstructured hover copy. It never interprets wording as
// a heading, result or flavor quote; structured producers supply their own roles.
func PlainCardRows(lines []string) CardRows {
	var rows CardRows
	for i, line := range lines {
		kind := CardRowBody
		if i == 0 {
			kind = CardRowTitle
		}
		rows.Add(kind, line)
	}
	return rows
}
