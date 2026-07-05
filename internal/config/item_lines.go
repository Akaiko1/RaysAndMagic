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
	if d.CurePoison {
		lines = append(lines, "Cures poison")
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
	if cl := d.CardEffectLines(); len(cl) > 0 {
		lines = append(lines, "Collection: "+strings.Join(cl, ", "))
	}
	return lines
}

// CardEffectLines is the SINGLE SOURCE of a monster card's collection-effect
// text, derived from its Card* fields. Shared by the item tooltip (via
// EffectLines), the card collector dialog, and the Cards menu tab. ASCII only —
// the in-game bitmap font has no glyph for unicode dashes.
func (d *ItemDefinitionConfig) CardEffectLines() []string {
	var p []string
	if d.CardMoveSpeedPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% move speed", d.CardMoveSpeedPct))
	}
	if d.CardBonusActions != 0 {
		p = append(p, fmt.Sprintf("+%d party action/turn", d.CardBonusActions))
	}
	for _, s := range []struct{ key, label string }{
		{"might", "Might"}, {"intellect", "Intellect"}, {"personality", "Personality"},
		{"endurance", "Endurance"}, {"accuracy", "Accuracy"}, {"speed", "Speed"}, {"luck", "Luck"},
	} {
		if v := d.CardStatBonuses[s.key]; v != 0 {
			p = append(p, fmt.Sprintf("%+d %s", v, s.label))
		}
	}
	if d.CardRangedDmgPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% ranged damage", d.CardRangedDmgPct))
	}
	if d.CardMeleeTrueDmg != 0 {
		p = append(p, fmt.Sprintf("+%d true melee damage", d.CardMeleeTrueDmg))
	}
	if d.CardPhysToFirePct != 0 {
		p = append(p, fmt.Sprintf("%d%% of physical damage dealt as fire", d.CardPhysToFirePct))
	}
	if d.CardWalkOnWater {
		p = append(p, "Walk on water")
	}
	if d.CardHealOnAtkPct != 0 {
		p = append(p, fmt.Sprintf("%d%% to self-heal %d on attack", d.CardHealOnAtkPct, d.CardHealAmount))
	}
	if d.CardLethalSavePct != 0 {
		p = append(p, fmt.Sprintf("%d%% to cheat death (half HP+SP)", d.CardLethalSavePct))
	}
	if d.CardMoveAoePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on move: %d pure to nearby foes", d.CardMoveAoePct, d.CardMoveAoeDmg))
	}
	if d.CardSummonChance != 0 {
		p = append(p, fmt.Sprintf("%d%% on action: summon allies (max %d)", d.CardSummonChance, d.CardSummonLimit))
	}
	if d.CardDisintegratePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on hit: disintegrate the target", d.CardDisintegratePct))
	}
	if d.CardRegenPct != 0 {
		p = append(p, fmt.Sprintf("Regenerate %d%% max HP per tick", d.CardRegenPct))
	}
	if d.CardDoubleAttackPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on melee hit: attack again", d.CardDoubleAttackPct))
	}
	if d.CardSpellProcPct != 0 {
		p = append(p, fmt.Sprintf("%d%% a melee swing casts a Fire Bolt instead", d.CardSpellProcPct))
	}
	if d.CardDodgeBonusPct != 0 {
		p = append(p, fmt.Sprintf("+%d Perfect Dodge", d.CardDodgeBonusPct))
	}
	if d.CardArmorBonus != 0 {
		p = append(p, fmt.Sprintf("+%d Armor Class", d.CardArmorBonus))
	}
	if d.CardThornsPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of incoming damage reflected", d.CardThornsPct))
	}
	if d.CardPhysToDarkPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of physical damage dealt as dark", d.CardPhysToDarkPct))
	}
	if d.CardPhysToLightPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of physical damage dealt as light", d.CardPhysToLightPct))
	}
	if d.CardPoisonProcPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on hit: poison for %ds", d.CardPoisonProcPct, d.CardPoisonDurationSec))
	}
	if d.CardMeleeDmgPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% melee damage", d.CardMeleeDmgPct))
	}
	if d.CardMaxHPBonus != 0 {
		p = append(p, fmt.Sprintf("+%d max HP", d.CardMaxHPBonus))
	}
	if len(d.CardResistBonus) > 0 {
		keys := make([]string, 0, len(d.CardResistBonus))
		for k := range d.CardResistBonus {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p = append(p, fmt.Sprintf("+%d%% %s resistance", d.CardResistBonus[k], titleCaseLower(k)))
		}
	}
	if d.CardGoldFindPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% gold from kills", d.CardGoldFindPct))
	}
	if d.CardBonusBoltPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on attack: fire a bonus bolt", d.CardBonusBoltPct))
	}
	if d.CardVolleyBonusPct != 0 {
		p = append(p, fmt.Sprintf("%d%% a bow shot looses an extra arrow", d.CardVolleyBonusPct))
	}
	if d.CardStunOnHitPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on hit: stun the target", d.CardStunOnHitPct))
	}
	if d.CardPoisonResistPct != 0 {
		p = append(p, fmt.Sprintf("%d%% resist poison", d.CardPoisonResistPct))
	}
	if d.CardCritBonusPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% critical hit chance", d.CardCritBonusPct))
	}
	if d.CardArmorPiercePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on hit: ignore armor", d.CardArmorPiercePct))
	}
	if len(d.CardBonusVs) > 0 {
		keys := make([]string, 0, len(d.CardBonusVs))
		for k := range d.CardBonusVs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p = append(p, fmt.Sprintf("+%.0f%% damage vs %s", (d.CardBonusVs[k]-1)*100, titleCaseLower(k)))
		}
	}
	return p
}
