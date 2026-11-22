package character

import (
	"strings"

	uitext "ugataima/assets/text"
)

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
// Runs, when set, are the row's tinted pieces; their texts join to Text.
type CardRow struct {
	Text    string
	Kind    CardRowKind
	Section string
	Runs    []CardRun
}

// CardRun is a piece of a row in a damage school's tint; School "" keeps the
// row's own color. A Whole run never breaks across lines: a wrapped row breaks
// only at the spaces of its other runs.
type CardRun struct {
	Text   string
	School string
	Whole  bool
}

// Words cuts the row where a wrapped line may break, whitespace collapsed:
// at every space outside its Whole runs. They join with single spaces back
// to the collapsed text.
func (row CardRow) Words() []string {
	if len(row.Runs) == 0 {
		return strings.Fields(row.Text)
	}
	var words []string
	var word strings.Builder
	glued := false // a space inside a Whole run, kept once the word goes on
	for _, run := range row.Runs {
		for i := 0; i < len(run.Text); i++ {
			switch c := run.Text[i]; {
			case !IsCardSpace(c):
				if glued && word.Len() > 0 {
					word.WriteByte(' ')
				}
				glued = false
				word.WriteByte(c)
			case run.Whole:
				glued = true
			default:
				if word.Len() > 0 {
					words = append(words, word.String())
					word.Reset()
				}
				glued = false
			}
		}
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}
	return words
}

// IsCardSpace reports a byte a card line breaks at: the ASCII whitespace
// strings.Fields splits plain rows at (card text is ASCII).
func IsCardSpace(c byte) bool { return strings.IndexByte(" \t\n\r\v\f", c) >= 0 }

// CardText is a line built from tinted runs.
type CardText []CardRun

func (t CardText) String() string {
	var b strings.Builder
	for _, r := range t {
		b.WriteString(r.Text)
	}
	return b.String()
}

// StyledText fills a catalog line rendered with uitext.Slot for each tinted
// part - uitext.Text("key", uitext.Slot) - with the parts in order.
func StyledText(line string, parts ...CardText) CardText {
	var out CardText
	for i, piece := range strings.Split(line, uitext.Slot) {
		if i > 0 && i <= len(parts) {
			out = append(out, parts[i-1]...)
		}
		if piece != "" {
			out = append(out, CardRun{Text: piece})
		}
	}
	return out
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
