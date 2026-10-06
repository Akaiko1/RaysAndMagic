package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Resistances are an item's defining power: every card that shows the item
// (bearer, shop and editor; compact and full) must list each resist row.
func TestItemTooltipListsEveryResistance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.party.Members = []*character.MMCharacter{ch}
	checked := 0
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		def := config.GlobalItems.Items[key]
		want := def.ResistLines()
		if len(want) == 0 {
			continue
		}
		checked++
		it := items.CreateItemFromYAML(key)
		for _, bearer := range []*character.MMCharacter{nil, ch} {
			for _, full := range []bool{false, true} {
				card := GetItemTooltip(it, bearer, cs, full)
				for _, line := range want {
					if !strings.Contains(card, line) {
						t.Errorf("%s bearer=%v full=%v: missing %q:\n%s", key, bearer != nil, full, line, card)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no authored item has resistances")
	}
}

// Stat descriptions must quote the REAL balance constants - hand-typed
// divisors drifted twice (Intellect/3 and Personality/4 were both invented).
func TestStatDescriptions_QuoteRealConstants(t *testing.T) {
	intDesc := character.StatDescription("intellect")
	if !strings.Contains(intDesc, fmt.Sprintf("Intellect/%d", spells.SpellIntellectDivisor)) {
		t.Errorf("intellect description must quote spell divisor %d: %s", spells.SpellIntellectDivisor, intDesc)
	}
	if !strings.Contains(intDesc, fmt.Sprintf("/%d", character.TrapStatScalingDivisor)) {
		t.Errorf("intellect description must quote trap divisor %d: %s", character.TrapStatScalingDivisor, intDesc)
	}
	persDesc := character.StatDescription("personality")
	if !strings.Contains(persDesc, fmt.Sprintf("Personality/%d", spells.HealingPersonalityDivisor)) {
		t.Errorf("personality description must quote healing divisor %d: %s", spells.HealingPersonalityDivisor, persDesc)
	}
}

func TestStatDescriptions_DoNotPretendWeaponScalingIsUniversal(t *testing.T) {
	for _, stat := range []string{"might", "accuracy"} {
		desc := character.StatDescription(stat)
		oldWeaponFormula := fmt.Sprintf("%s/%d", strings.ToUpper(stat[:1])+stat[1:], character.WeaponPrimaryStatDivisor)
		if strings.Contains(desc, oldWeaponFormula) {
			t.Errorf("%s description should point to weapon tooltips, not hard-code universal weapon scaling: %q", stat, desc)
		}
		if !strings.Contains(strings.ToLower(desc), "weapons that scale from") {
			t.Errorf("%s description should say weapon scaling is weapon-specific: %q", stat, desc)
		}
	}
}
