package config

import (
	"fmt"
	"sort"
	"strings"

	damagecalc "ugataima/internal/damage"
)

// Presentation lines for items - the ONE formatter behind the in-game item
// tooltip and the map-editor card (same contract as the weapon/spell/trap
// EffectLines). New YAML fields get a line HERE, and every consumer shows it.

func nonPhysicalDamageSchools() []damagecalc.Type {
	all := damagecalc.Types()
	out := make([]damagecalc.Type, 0, len(all)-1)
	for _, school := range all {
		if school != damagecalc.Physical {
			out = append(out, school)
		}
	}
	return out
}

// StatBonusLines lists the item's flat stat bonuses and scaling-divisor
// bonuses (divisors are STAT bonuses computed from the base stat - they feed
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

// PartyArmorLine describes the party_armor_bonus "shield wall" aura, or "" if
// the item grants none. One formatter for the wording, shared by EffectLines
// and the unified armor tooltip (which builds its own EFFECTS section).
func (d *ItemDefinitionConfig) PartyArmorLine() string {
	if d.PartyArmorBonus <= 0 {
		return ""
	}
	return fmt.Sprintf("Shield wall: +%d AC to every other party member", d.PartyArmorBonus)
}

// ResistLines lists per-school resistances, collapsing to one "all except
// physical" line when every non-physical school shares a value.
func (d *ItemDefinitionConfig) ResistLines() []string {
	if len(d.Resistances) == 0 {
		return nil
	}
	nonPhysicalSchools := nonPhysicalDamageSchools()
	allEqual, common := true, d.Resistances[nonPhysicalSchools[0].String()]
	for _, school := range nonPhysicalSchools {
		if d.Resistances[school.String()] != common {
			allEqual = false
			break
		}
	}
	if allEqual && common > 0 {
		phys := d.Resistances[damagecalc.Physical.String()]
		if phys > 0 {
			if phys == common {
				return []string{fmt.Sprintf("+%d%% resistance to every damage school", common)}
			}
			return []string{fmt.Sprintf("+%d%% resistance to every non-physical school; +%d%% Physical resistance", common, phys)}
		}
		return []string{fmt.Sprintf("+%d%% resistance to every non-physical school", common)}
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
// stat bonuses, resistances, consumable behavior, and authored tooltip effects.
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
	if d.ManaBase > 0 {
		if d.ManaPersonalityDivisor > 0 {
			lines = append(lines, fmt.Sprintf("Restores %d + Personality/%d SP", d.ManaBase, d.ManaPersonalityDivisor))
		} else {
			lines = append(lines, fmt.Sprintf("Restores %d SP", d.ManaBase))
		}
	}
	if ln := d.PartyArmorLine(); ln != "" {
		lines = append(lines, ln)
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
	lines = append(lines, d.TooltipEffects...)
	if cl := d.CardEffectLines(); len(cl) > 0 {
		lines = append(lines, "Collection: "+strings.Join(cl, ", "))
	}
	lines = append(lines, d.SetLines()...)
	return lines
}

// TooltipUsageLines returns authored usage text for simple item cards. The
// copy prevents a presentation caller from mutating the loaded YAML config.
func (d *ItemDefinitionConfig) TooltipUsageLines() []string {
	return append([]string(nil), d.TooltipUsage...)
}

// SetLines describes the equipment set this item belongs to and its completed
// bonus - shared by the item tooltip and the map-editor card.
func (d *ItemDefinitionConfig) SetLines() []string {
	return EquipmentSetLines(d.Set)
}

// SetLines describes the equipment set this weapon belongs to and its completed
// bonus - shared by the weapon tooltip and the map-editor card.
func (w *WeaponDefinitionConfig) SetLines() []string {
	if w == nil {
		return nil
	}
	return EquipmentSetLines(w.Set)
}

// EquipmentSetLines is the shared player-facing formatter for item and weapon
// set membership. Set names and numerical bonuses remain authored in items.yaml.
func EquipmentSetLines(setKey string) []string {
	set := GetItemSet(setKey)
	if set == nil {
		return nil
	}
	lines := []string{fmt.Sprintf("Set: %s (%d pieces)", set.Name, set.RequiredPieceCount())}
	var parts []string
	for _, b := range []struct {
		label string
		val   int
	}{
		{"Might", set.BonusMight}, {"Intellect", set.BonusIntellect}, {"Personality", set.BonusPersonality},
		{"Endurance", set.BonusEndurance}, {"Accuracy", set.BonusAccuracy}, {"Speed", set.BonusSpeed}, {"Luck", set.BonusLuck},
	} {
		if b.val != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d", b.label, b.val))
		}
	}
	if set.StunDurationPct != 0 {
		parts = append(parts, fmt.Sprintf("stuns suffered %d%% duration", 100+set.StunDurationPct))
	}
	if set.BonusCritChance != 0 {
		parts = append(parts, fmt.Sprintf("critical chance %+d%%", set.BonusCritChance))
	}
	if len(parts) > 0 {
		lines = append(lines, "Set bonus: "+strings.Join(parts, ", "))
	}
	return lines
}

// CardEffectLines is the SINGLE SOURCE of a monster card's collection-effect
// text, derived from its Card* fields. Shared by the item tooltip (via
// EffectLines), the card collector dialog, and the Cards menu tab. ASCII only -
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
		p = append(p, fmt.Sprintf("%d%% to self-heal %d on weapon attack", d.CardHealOnAtkPct, d.CardHealAmount))
	}
	if d.CardLethalSavePct != 0 {
		p = append(p, fmt.Sprintf("%d%% to cheat death (half HP+SP)", d.CardLethalSavePct))
	}
	if d.CardMoveAoePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on move: %d physical true damage to nearby foes", d.CardMoveAoePct, d.CardMoveAoeDmg))
	}
	if d.CardSummonChance != 0 {
		line := fmt.Sprintf("%d%% on action: summon allies (max %d)", d.CardSummonChance, d.CardSummonLimit)
		if d.CardSummonCDSeconds > 0 {
			line += fmt.Sprintf(", %ds cooldown", d.CardSummonCDSeconds)
		}
		p = append(p, line)
	}
	if d.CardDisintegratePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on direct hit: disintegrate (undead and dragons immune)", d.CardDisintegratePct))
	}
	if d.CardRegenPct != 0 {
		p = append(p, fmt.Sprintf("Regenerate %d%% max HP per regeneration tick", d.CardRegenPct))
	}
	if d.CardDoubleAttackPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on melee hit: attack again", d.CardDoubleAttackPct))
	}
	if d.CardSpellProcPct != 0 {
		p = append(p, fmt.Sprintf("%d%% a melee swing casts a Fire Bolt instead", d.CardSpellProcPct))
	}
	if d.CardDodgeBonusPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% Perfect Dodge", d.CardDodgeBonusPct))
	}
	if d.CardArmorBonus != 0 {
		p = append(p, fmt.Sprintf("+%d Armor Class", d.CardArmorBonus))
	}
	if d.CardThornsPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of damage received from monster hits reflected", d.CardThornsPct))
	}
	if d.CardPhysToDarkPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of physical damage dealt as dark", d.CardPhysToDarkPct))
	}
	if d.CardPhysToLightPct != 0 {
		p = append(p, fmt.Sprintf("%d%% of physical damage dealt as light", d.CardPhysToLightPct))
	}
	if d.CardPoisonProcPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on direct hit: poison for %ds", d.CardPoisonProcPct, d.CardPoisonDurationSec))
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
		p = append(p, fmt.Sprintf("%d%% on weapon attack: fire a bonus bolt", d.CardBonusBoltPct))
	}
	if d.CardVolleyBonusPct != 0 {
		p = append(p, fmt.Sprintf("%d%% a bow shot looses an extra arrow", d.CardVolleyBonusPct))
	}
	if d.CardStunOnHitPct != 0 {
		p = append(p, fmt.Sprintf("%d%% on direct hit: stun the target", d.CardStunOnHitPct))
	}
	if d.CardPoisonResistPct != 0 {
		p = append(p, fmt.Sprintf("%d%% chance to resist monster poison", d.CardPoisonResistPct))
	}
	if d.CardCritBonusPct != 0 {
		p = append(p, fmt.Sprintf("+%d%% critical hit chance", d.CardCritBonusPct))
	}
	if d.CardArmorPiercePct != 0 {
		p = append(p, fmt.Sprintf("%d%% on melee hit: ignore armor", d.CardArmorPiercePct))
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
