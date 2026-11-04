package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"ugataima/internal/boot"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Exercise the editor's actual catalog and save-browser entry points, including
// all authored kinds. Shop and editor must render the same complete base text.
func TestEditorCardsWired(t *testing.T) {
	t.Chdir("../..")
	cfg, _ := boot.LoadGameData()
	for _, cards := range [][]contentCard{buildItemsCards(), buildSpellCards()} {
		for _, c := range cards {
			t.Run(c.key, func(t *testing.T) {
				var want character.CardRows
				var it items.Item
				switch c.kind {
				case cardWeapon:
					it = items.CreateWeaponFromYAML(c.key)
				case cardItem:
					it = items.CreateItemFromYAML(c.key)
				case cardAction:
					if trap, ok := config.TrapItem(c.key); ok {
						it = trap
					} else if technique, ok := config.TechniqueItem(c.key); ok {
						it = technique
					} else {
						t.Fatalf("action card %q is neither a trap nor a technique", c.key)
					}
				case cardSpell:
					var err error
					it, err = spells.CreateSpellItem(spells.SpellID(c.key))
					if err != nil {
						t.Fatal(err)
					}
					want = game.GetSpellTooltipRows(spells.SpellID(c.key), nil, nil, true)
				}
				if len(want) == 0 {
					want = game.GetItemTooltipRows(it, nil, nil, true)
				}
				if got := c.tooltipRows; !reflect.DeepEqual(got, want) {
					t.Fatalf("catalog differs from shared base tooltip:\n%s\nwant:\n%s", got.String(), want.String())
				}
				assertEditorTooltipRoles(t, &c)
				before, _ := json.Marshal(it)
				saved := cardForSavedItem(it)
				if got := saved.tooltipRows; !reflect.DeepEqual(got, game.GetItemTooltipRows(it, nil, nil, true)) {
					t.Fatalf("save item differs from shared base tooltip:\n%s", got.String())
				}
				assertEditorTooltipRoles(t, &saved)
				after, _ := json.Marshal(it)
				if string(before) != string(after) {
					t.Fatal("save hover mutated the item")
				}
			})
		}
	}
	// Every technique has a catalog card.
	listed := map[string]bool{}
	for _, c := range buildSpellCards() {
		listed[c.key] = true
	}
	for _, d := range config.GlobalTechniques.Techniques {
		if !listed[d.Key] {
			t.Errorf("technique %s is missing from the catalog", d.Key)
		}
	}
	// Starting equipment hovers resolve through these same catalogs.
	cards := append(buildItemsCards(), buildSpellCards()...)
	for _, ch := range buildCharacterDetails(cfg, charTextCols(windowWidth)) {
		for _, row := range ch.rows {
			if !row.hasIcon {
				continue
			}
			found := false
			for _, c := range cards {
				if c.kind == row.iconKind && c.key == row.iconKey {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: missing equipment/spell tooltip for %s", ch.portrait, row.text)
			}
		}
	}
}

func assertEditorTooltipRoles(t *testing.T, card *contentCard) {
	t.Helper()
	rendered := cardTooltipLines(card)
	index := 0
	for _, row := range card.tooltipRows {
		if len(row.Runs) > 0 {
			// A tinted row wraps between its words: its lines rejoin to them.
			var got []string
			for index < len(rendered) && len(got) < len(row.Words()) && rendered[index].kind == row.Kind {
				got = append(got, rendered[index].text)
				index++
				if strings.Join(got, " ") == strings.Join(row.Words(), " ") {
					break
				}
			}
			if strings.Join(got, " ") != strings.Join(row.Words(), " ") {
				t.Fatalf("editor lost %v role or text while wrapping %q: %q", row.Kind, row.Text, got)
			}
			continue
		}
		for _, fragment := range wrapTooltipLines(row.Text, 80) {
			if index >= len(rendered) || rendered[index].kind != row.Kind || rendered[index].text != fragment {
				t.Fatalf("editor lost %v role while wrapping %q", row.Kind, row.Text)
			}
			index++
		}
	}
	if index != len(rendered) {
		t.Fatal("editor added unauthored rows")
	}
}

// A card row with tinted runs breaks in the editor only where the game's card
// may - never inside a damage share - and draws each share in its school tint.
func TestEditorTintedCardRowWrapsBetweenShares(t *testing.T) {
	lead := strings.Repeat("x", 70)
	row := character.CardRow{Kind: character.CardRowResult, Text: lead + " 13 Normal + 6 Fire",
		Runs: []character.CardRun{{Text: lead + " "}, {Text: "13 Normal +", Whole: true}, {Text: " "}, {Text: "6 Fire", School: "fire", Whole: true}}}
	lines := cardTooltipLines(&contentCard{tooltipRows: character.CardRows{row}})
	var whole, tinted bool
	for _, line := range lines {
		if strings.Contains(line.text, "13") != strings.Contains(line.text, "Normal +") {
			t.Fatalf("a share split across lines: %q", line.text)
		}
		whole = whole || strings.Contains(line.text, "13 Normal +")
		for _, r := range line.runs {
			tinted = tinted || r.Text == "6 Fire" && r.Tint == game.SchoolTextColor("fire")
		}
	}
	if len(lines) < 2 || !whole {
		t.Fatalf("fixture: the row should wrap with the share whole: %+v", lines)
	}
	if !tinted {
		t.Fatalf("the fire share lost its tint: %+v", lines)
	}
}
