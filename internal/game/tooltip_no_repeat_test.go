package game

import (
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// No card states a line twice - a description is never repeated as the
// closing quote - across every item, weapon, technique, trap and spell, in
// compact and full detail.
func TestNoTooltipRepeatsALine(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	oldTechniques := config.GlobalTechniques
	t.Cleanup(func() { config.GlobalTechniques = oldTechniques })
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	char := cs.game.party.Members[0]
	var cards []items.Item
	for key := range config.GlobalItems.Items {
		cards = append(cards, items.CreateItemFromYAML(key))
	}
	for key := range config.GlobalWeapons.Weapons {
		cards = append(cards, items.CreateWeaponFromYAML(key))
	}
	for _, d := range config.GlobalTechniques.Techniques {
		it, _ := config.TechniqueItem(d.Key)
		cards = append(cards, it)
	}
	for key := range config.GlobalTrapConfig.Traps {
		it, _ := config.TrapItem(key)
		cards = append(cards, it)
	}
	for key := range config.GlobalSpells.Spells {
		if it, err := spells.CreateSpellItem(spells.SpellID(key)); err == nil {
			cards = append(cards, it)
		}
	}
	for _, it := range cards {
		for _, full := range []bool{false, true} {
			seen := map[string]bool{}
			for _, line := range strings.Split(GetItemTooltipRows(it, char, cs, full).String(), "\n") {
				n := strings.Trim(strings.TrimSpace(line), "\"")
				if len(n) < 16 {
					continue
				}
				if seen[n] {
					t.Errorf("%s (full=%v) repeats %q", it.Name, full, n)
				}
				seen[n] = true
			}
		}
	}
}
