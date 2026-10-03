package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Invariant: producers retain row roles through all item/spell categories,
// base/live contexts and compact/full views. Details stay in their section;
// the compact expansion hint follows all appended content. Persistence: N/A.
func TestCardSectionRolesAndHint(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	hero := cs.game.party.Members[0]
	check := func(name string, rows character.CardRows, full bool) {
		t.Helper()
		section := ""
		detailSeen := false
		for i, row := range rows {
			switch row.Kind {
			case character.CardRowSection:
				if row.Text == "" || row.Section != row.Text {
					t.Fatalf("%s: section lacks identity: %+v", name, row)
				}
				section, detailSeen = row.Section, false
			case character.CardRowResult, character.CardRowDetail:
				if row.Section != "" && row.Section != section {
					t.Fatalf("%s: row detached from mechanic: %+v", name, row)
				}
				if row.Kind == character.CardRowDetail {
					if !full {
						t.Fatalf("%s: compact card leaked detail: %+v", name, row)
					}
					detailSeen = true
				} else if row.Section != "" && detailSeen {
					t.Fatalf("%s: result buried below detail: %+v", name, row)
				}
			case character.CardRowHint:
				if i != len(rows)-1 {
					t.Fatalf("%s: expansion hint is not last: %s", name, rows.String())
				}
			}
		}
	}
	var cards []items.Item
	for key := range config.GlobalWeapons.Weapons {
		cards = append(cards, items.CreateWeaponFromYAML(key))
	}
	for key := range config.GlobalItems.Items {
		if it, err := items.TryCreateItemFromYAML(key); err == nil {
			cards = append(cards, it)
		}
	}
	for _, key := range config.TrapKeysOrdered() {
		if it, ok := config.TrapItem(key); ok {
			cards = append(cards, it)
		}
	}
	for _, d := range config.GlobalTechniques.Techniques {
		if it, ok := config.TechniqueItem(d.Key); ok {
			cards = append(cards, it)
		}
	}
	for _, full := range []bool{false, true} {
		for _, it := range cards {
			check(it.Name, GetItemTooltipRows(it, nil, nil, full), full)
			check(it.Name+" (hero)", GetItemTooltipRows(it, hero, cs, full), full)
		}
		for key := range config.GlobalSpells.Spells {
			check(key, GetSpellTooltipRows(spells.SpellID(key), nil, nil, full), full)
			check(key+" (hero)", GetSpellTooltipRows(spells.SpellID(key), hero, cs, full), full)
		}
	}
}

// A mortar always lands exactly mortar_range_tiles away, whatever its
// projectile physics says, so its card states that range. The physics range
// is moved off the mortar range to prove the card follows the landing rule.
func TestMortarCardRangeIsTheLandingRange(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := cs.game.party.Members[0]
	for key, def := range config.GlobalSpells.Spells {
		if def == nil || def.MortarRangeTiles <= 0 || def.Physics == nil {
			continue
		}
		t.Run(key, func(t *testing.T) {
			prev := def.Physics.RangeTiles
			def.Physics.RangeTiles = def.MortarRangeTiles + 3
			t.Cleanup(func() { def.Physics.RangeTiles = prev })
			want := fmt.Sprintf("Range: %.0f tiles", def.MortarRangeTiles)
			for _, card := range []string{GetSpellTooltip(spells.SpellID(key), nil, nil, true), GetSpellTooltip(spells.SpellID(key), caster, cs, true)} {
				if !strings.Contains(card, want) {
					t.Fatalf("card lacks %q:\n%s", want, card)
				}
			}
			spell, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
			if err != nil {
				t.Fatal(err)
			}
			cs.game.pendingMortars = nil
			if !cs.castMortarSpell(spells.SpellID(key), spell, caster, false) || len(cs.game.pendingMortars) != 1 {
				t.Fatal("mortar cast failed")
			}
			m := cs.game.pendingMortars[0]
			tiles := math.Hypot(m.X-cs.game.camera.X, m.Y-cs.game.camera.Y) / float64(cs.game.config.GetTileSize())
			if math.Abs(tiles-def.MortarRangeTiles) > 1e-6 {
				t.Fatalf("the mortar lands %.2f tiles out, the card says %.0f", tiles, def.MortarRangeTiles)
			}
		})
	}
}
