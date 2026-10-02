package game

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Every heading a card prints is in the one shared list the game layout and
// the editor catalog read (character.IsCardSectionTitle), in the catalog
// variant and the hero variant, compact and full. A new section that skipped
// the list would render as body text in one of them. A compact card's Shift
// hint is its last line, below the set block, value and description.
func TestCardSectionTitlesAreRegistered(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	hero := cs.game.party.Members[0]
	looksLikeHeading := func(line string) bool {
		if len(line) < 3 || strings.ContainsAny(line, ":0123456789") {
			return false
		}
		for _, r := range line {
			if !unicode.IsUpper(r) && r != ' ' {
				return false
			}
		}
		return true
	}
	check := func(name, card string) {
		if strings.Contains(card, shiftDetailHint) && !strings.HasSuffix(card, "\n\n"+shiftDetailHint) {
			t.Errorf("%s: the Shift hint is not the last line:\n%s", name, card)
		}
		for _, line := range strings.Split(card, "\n") {
			if looksLikeHeading(line) && !character.IsCardSectionTitle(line) {
				t.Errorf("%s: heading %q is not a registered card section", name, line)
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
			check(it.Name, GetItemTooltip(it, nil, nil, full))
			check(it.Name+" (hero)", GetItemTooltip(it, hero, cs, full))
		}
		for key := range config.GlobalSpells.Spells {
			check(key, GetSpellTooltip(spells.SpellID(key), nil, nil, full))
			check(key+" (hero)", GetSpellTooltip(spells.SpellID(key), hero, cs, full))
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
