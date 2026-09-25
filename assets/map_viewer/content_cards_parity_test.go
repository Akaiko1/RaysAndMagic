package main

import (
	"encoding/json"
	"strings"
	"testing"
	"ugataima/internal/boot"
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
				var want string
				var it items.Item
				switch c.kind {
				case cardWeapon:
					it = items.CreateWeaponFromYAML(c.key)
				case cardItem:
					it = items.CreateItemFromYAML(c.key)
				case cardSpell:
					if trap, ok := config.TrapItem(c.key); ok {
						it = trap
					} else {
						var err error
						it, err = spells.CreateSpellItem(spells.SpellID(c.key))
						if err != nil {
							t.Fatal(err)
						}
						want = game.GetSpellTooltip(spells.SpellID(c.key), nil, nil, true)
					}
				}
				if want == "" {
					want = game.GetItemTooltip(it, nil, nil, true)
				}
				if got := strings.Join(c.tooltipRows, "\n"); got != want {
					t.Fatalf("catalog differs from shared base tooltip:\n%s\nwant:\n%s", got, want)
				}
				before, _ := json.Marshal(it)
				saved := cardForSavedItem(it)
				if got := strings.Join(saved.tooltipRows, "\n"); got != game.GetItemTooltip(it, nil, nil, true) {
					t.Fatalf("save item differs from shared base tooltip:\n%s", got)
				}
				after, _ := json.Marshal(it)
				if string(before) != string(after) {
					t.Fatal("save hover mutated the item")
				}
			})
		}
	}
	// Starting equipment hovers resolve through these same catalogs.
	cards := append(buildItemsCards(), buildSpellCards()...)
	for _, ch := range buildCharacterDetails(cfg) {
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
