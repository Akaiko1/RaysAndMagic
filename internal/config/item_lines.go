package config

import (
	"fmt"
	"sort"
	"strings"
)

// Presentation lines for items — the ONE formatter behind the in-game item
// tooltip and the map-editor card (same contract as the weapon/spell/trap
// EffectLines). New YAML fields get a line HERE, and every consumer shows it.

// nonPhysicalSchools is the resist-collapse order (matches damage schools).
var nonPhysicalSchools = []string{"fire", "water", "air", "earth", "body", "mind", "spirit", "light", "dark"}

// StatBonusLines lists the item's flat stat bonuses and scaling-divisor
// bonuses (divisors are STAT bonuses computed from the base stat — they feed
// everything the stat feeds).
func (d *ItemDefinitionConfig) StatBonusLines() []string {
	var parts []string
	flat := []struct {
		label string
		val   int
	}{
		{"Might", d.BonusMight},
		{"Intellect", d.BonusIntellect},
		{"Personality", d.BonusPersonality},
		{"Endurance", d.BonusEndurance},
		{"Accuracy", d.BonusAccuracy},
		{"Speed", d.BonusSpeed},
		{"Luck", d.BonusLuck},
	}
	for _, b := range flat {
		if b.val != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d", b.label, b.val))
		}
	}
	if d.IntellectScalingDivisor > 0 {
		parts = append(parts, fmt.Sprintf("Intellect +base/%d", d.IntellectScalingDivisor))
	}
	if d.PersonalityScalingDivisor > 0 {
		parts = append(parts, fmt.Sprintf("Personality +base/%d", d.PersonalityScalingDivisor))
	}
	return parts
}

// ResistLines lists per-school resistances, collapsing to one "all except
// physical" line when every non-physical school shares a value.
func (d *ItemDefinitionConfig) ResistLines() []string {
	if len(d.Resistances) == 0 {
		return nil
	}
	allEqual, common := true, d.Resistances[nonPhysicalSchools[0]]
	for _, s := range nonPhysicalSchools {
		if d.Resistances[s] != common {
			allEqual = false
			break
		}
	}
	if allEqual && common > 0 {
		phys := d.Resistances["physical"]
		if phys > 0 {
			return []string{fmt.Sprintf("Resist +%d%% to all damage (+%d%% physical)", common, phys)}
		}
		return []string{fmt.Sprintf("Resist +%d%% to all damage except physical", common)}
	}
	schools := make([]string, 0, len(d.Resistances))
	for s := range d.Resistances {
		schools = append(schools, s)
	}
	sort.Strings(schools)
	var parts []string
	for _, s := range schools {
		if v := d.Resistances[s]; v > 0 {
			parts = append(parts, fmt.Sprintf("+%d%% %s resist", v, strings.ToUpper(s[:1])+s[1:]))
		}
	}
	return parts
}

// EffectLines is the full character-independent mechanics list: armor values,
// stat bonuses, resistances and consumable behavior.
func (d *ItemDefinitionConfig) EffectLines() []string {
	var lines []string
	if d.ArmorClassBase > 0 {
		lines = append(lines, fmt.Sprintf("Armor class %d", d.ArmorClassBase))
	}
	if d.EnduranceScalingDivisor > 0 {
		lines = append(lines, fmt.Sprintf("AC +Endurance/%d", d.EnduranceScalingDivisor))
	}
	lines = append(lines, d.StatBonusLines()...)
	lines = append(lines, d.ResistLines()...)
	if d.HealBase > 0 {
		if d.HealEnduranceDivisor > 0 {
			lines = append(lines, fmt.Sprintf("Heals %d + Endurance/%d HP", d.HealBase, d.HealEnduranceDivisor))
		} else {
			lines = append(lines, fmt.Sprintf("Heals %d HP", d.HealBase))
		}
	}
	if d.Revive {
		if d.FullHeal {
			lines = append(lines, "Revives a fallen ally at FULL health")
		} else {
			lines = append(lines, "Revives a fallen ally")
		}
	}
	if d.SummonDistanceTiles > 0 {
		lines = append(lines, fmt.Sprintf("Summons ~%d tiles away", d.SummonDistanceTiles))
	}
	if d.OpensMap {
		lines = append(lines, "Opens the world map overlay")
	}
	if d.PromotesLich {
		lines = append(lines, "Offers a party member the path of the Lich")
	}
	return lines
}
