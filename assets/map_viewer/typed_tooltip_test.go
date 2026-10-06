package main

import (
	"strings"
	"testing"
	"ugataima/internal/character"
)

// Wording never assigns a role: even identical text can be a title, section,
// result, detail or prose. Every wrapped fragment keeps the producer's role.
func TestEditorTypedRolesIgnoreWording(t *testing.T) {
	for _, kind := range []character.CardRowKind{
		character.CardRowBody, character.CardRowTitle, character.CardRowCategory,
		character.CardRowSection, character.CardRowResult, character.CardRowDetail,
		character.CardRowDescription, character.CardRowFlavor, character.CardRowHint,
	} {
		card := contentCard{tooltipRows: character.CardRows{
			{Text: "DAMAGE", Kind: kind},
			{Text: "\"not necessarily flavor\"", Kind: kind},
			{Text: strings.Repeat("Range: ambiguous wording ", 15), Kind: kind},
		}}
		assertEditorTooltipRoles(t, &card)
	}
}
