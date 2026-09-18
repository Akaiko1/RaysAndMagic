package config

import (
	"fmt"
	"sort"
	"strings"
	uitext "ugataima/assets/text"

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
		parts = append(parts, uitext.Text("item.intellect_base", d.IntellectScalingDivisor))
	}
	if d.PersonalityScalingDivisor > 0 {
		parts = append(parts, uitext.Text("item.personality_base", d.PersonalityScalingDivisor))
	}
	return parts
}

// ItemMechanicLines lists the per-item special-mechanic rows - projectile
// reflection, hostile-status duration, growing scales, draught wards. ONE
// formatter shared by EffectLines (editor) and the unified in-game tooltip,
// so the two can never drift (tooltip parity contract).
func (d *ItemDefinitionConfig) ItemMechanicLines() []string {
	var lines []string
	hasTimedBuff := d.HasTimedBuff()
	if d.ProjectileReflectPct > 0 {
		lines = append(lines, uitext.Text("item.mirror_scales_chance_to_turn_a_projectile", d.ProjectileReflectPct))
	}
	if d.StatusDurationPct != 0 {
		lines = append(lines, uitext.Text("item.hostile_statuses_on_the_wearer_last_as", 100+d.StatusDurationPct))
	}
	if d.ScaleStackAC > 0 {
		lines = append(lines, uitext.Text("item.growing_scales_ac_per_hit_taken_max", d.ScaleStackAC, d.ScaleStackAC*d.ScaleStackMax))
	}
	if hasTimedBuff && d.ResistBuffSchoolPct > 0 && d.ResistBuffSchool != "" {
		lines = append(lines, uitext.Text("item.party_ward_resistance_for_s", TitleWords(d.ResistBuffSchool), d.ResistBuffSchoolPct, d.BuffDurationSeconds))
	}
	if hasTimedBuff && d.BuffDodgePct > 0 {
		lines = append(lines, uitext.Text("item.party_dodge_for_s", d.BuffDodgePct, d.BuffDurationSeconds))
	}
	if hasTimedBuff && d.BuffArmorClass > 0 {
		lines = append(lines, uitext.Text("item.party_stoneskin_armor_class_for_s", d.BuffArmorClass, d.BuffDurationSeconds))
	}
	return lines
}

// PartyArmorLine describes the party_armor_bonus "shield wall" aura, or "" if
// the item grants none. One formatter for the wording, shared by EffectLines
// and the unified armor tooltip (which builds its own EFFECTS section).
func (d *ItemDefinitionConfig) PartyArmorLine() string {
	if d.PartyArmorBonus <= 0 {
		return ""
	}
	return uitext.Text("item.shield_wall_ac_to_every_other_party", d.PartyArmorBonus)
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
				return []string{uitext.Text("item.resistance_to_every_damage_school", common)}
			}
			return []string{uitext.Text("item.resistance_to_every_non_physical_school_physical", common, phys)}
		}
		return []string{uitext.Text("item.resistance_to_every_non_physical_school", common)}
	}
	schools := make([]string, 0, len(d.Resistances))
	for s := range d.Resistances {
		schools = append(schools, s)
	}
	sort.Strings(schools)
	var parts []string
	for _, s := range schools {
		if v := d.Resistances[s]; v > 0 {
			parts = append(parts, uitext.Text("item.resist", v, strings.ToUpper(s[:1])+s[1:]))
		}
	}
	return parts
}

// EffectLines is the full character-independent mechanics list: armor values,
// stat bonuses, resistances, consumable behavior, and authored tooltip effects.
func (d *ItemDefinitionConfig) EffectLines() []string {
	return d.effectLines(true)
}

// CoreEffectLines omits armor values rendered in the structured defense section.
func (d *ItemDefinitionConfig) CoreEffectLines() []string {
	return d.effectLines(false)
}

func (d *ItemDefinitionConfig) effectLines(includeStructured bool) []string {
	var lines []string
	if includeStructured && d.ArmorClassBase > 0 {
		lines = append(lines, uitext.Text("item.armor_class", d.ArmorClassBase))
	}
	if includeStructured && d.EnduranceScalingDivisor > 0 {
		lines = append(lines, uitext.Text("item.ac_endurance", d.EnduranceScalingDivisor))
	}
	lines = append(lines, d.StatBonusLines()...)
	lines = append(lines, d.ResistLines()...)
	if d.HealBase > 0 {
		if d.HealEnduranceDivisor > 0 {
			lines = append(lines, uitext.Text("item.heals_endurance_hp", d.HealBase, d.HealEnduranceDivisor))
		} else {
			lines = append(lines, uitext.Text("item.heals_hp", d.HealBase))
		}
	}
	if d.ManaBase > 0 {
		if d.ManaPersonalityDivisor > 0 {
			lines = append(lines, uitext.Text("item.restores_personality_sp", d.ManaBase, d.ManaPersonalityDivisor))
		} else {
			lines = append(lines, uitext.Text("item.restores_sp", d.ManaBase))
		}
	}
	if ln := d.PartyArmorLine(); ln != "" {
		lines = append(lines, ln)
	}
	lines = append(lines, d.ItemMechanicLines()...)
	if d.CurePoison {
		lines = append(lines, uitext.Text("item.cures_poison"))
	}
	if d.Revive {
		if d.FullHeal {
			lines = append(lines, uitext.Text("item.revives_a_fallen_ally_at_full_health"))
		} else {
			lines = append(lines, uitext.Text("item.revives_a_fallen_ally"))
		}
	}
	if d.SummonDistanceTiles > 0 {
		lines = append(lines, uitext.Text("item.summons_tiles_away", d.SummonDistanceTiles))
	}
	if d.OpensMap {
		lines = append(lines, uitext.Text("item.opens_the_world_map_overlay"))
	}
	if d.PromotesLich {
		lines = append(lines, uitext.Text("item.offers_a_party_member_the_path_of"))
	}
	lines = append(lines, d.TooltipEffects...)
	if cl := d.CardEffectLines(); len(cl) > 0 {
		lines = append(lines, uitext.Text("item.collection")+strings.Join(cl, ", "))
	}
	lines = append(lines, d.SetLines()...)
	return lines
}

// TooltipUsageLines is the shared usage policy for game and editor cards.
// Authored instructions override category defaults (for example, door keys).
func (d *ItemDefinitionConfig) TooltipUsageLines() []string {
	if len(d.TooltipUsage) > 0 {
		return append([]string(nil), d.TooltipUsage...)
	}
	text := d.usageDefaults
	if text == nil {
		return nil
	}
	switch d.Type {
	case "card":
		return append([]string(nil), text.Card...)
	case "trinket":
		return []string{text.Trinket}
	case "consumable":
		return []string{text.ActivateInventory, text.ConsumedOnUse}
	case "quest":
		var lines []string
		if d.OpensMap || d.PromotesLich {
			lines = append(lines, text.ActivateInventory)
		}
		if d.PromotesLich {
			lines = append(lines, text.ConsumedAfterPromotion)
		}
		if d.Value <= 0 {
			lines = append(lines, text.CannotSell)
		}
		if !d.Discardable {
			lines = append(lines, text.CannotDrop)
		}
		return lines
	}
	return nil
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
	lines := []string{uitext.Text("item.set_pieces", set.Name, set.RequiredPieceCount())}
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
		parts = append(parts, uitext.Text("item.stuns_suffered_duration", 100+set.StunDurationPct))
	}
	if set.BonusCritChance != 0 {
		parts = append(parts, uitext.Text("item.critical_chance", set.BonusCritChance))
	}
	if set.FieryRipostePct != 0 {
		parts = append(parts, uitext.Text("item.melee_attackers_take_back_as_fire", set.FieryRipostePct))
	}
	if len(parts) > 0 {
		lines = append(lines, uitext.Text("item.set_bonus")+strings.Join(parts, ", "))
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
		p = append(p, uitext.Text("item.move_speed", d.CardMoveSpeedPct))
	}
	if d.CardBonusActions != 0 {
		p = append(p, uitext.Text("item.party_action_turn", d.CardBonusActions))
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
		p = append(p, uitext.Text("item.ranged_damage", d.CardRangedDmgPct))
	}
	if d.CardMeleeTrueDmg != 0 {
		p = append(p, uitext.Text("item.true_melee_damage", d.CardMeleeTrueDmg))
	}
	if d.CardPhysToFirePct != 0 {
		p = append(p, uitext.Text("item.of_physical_damage_dealt_as_fire", d.CardPhysToFirePct))
	}
	if d.CardWalkOnWater {
		p = append(p, uitext.Text("item.walk_on_water"))
	}
	if d.CardHealOnAtkPct != 0 {
		p = append(p, uitext.Text("item.to_self_heal_on_weapon_attack", d.CardHealOnAtkPct, d.CardHealAmount))
	}
	if d.CardLethalSavePct != 0 {
		p = append(p, uitext.Text("item.to_cheat_death_half_hp_sp", d.CardLethalSavePct))
	}
	if d.CardMoveAoePct != 0 {
		p = append(p, uitext.Text("item.on_move_physical_true_damage_to_nearby", d.CardMoveAoePct, d.CardMoveAoeDmg))
	}
	if d.CardSummonChance != 0 {
		line := uitext.Text("item.on_action_summon_allies_max", d.CardSummonChance, d.CardSummonLimit)
		if d.CardSummonCDSeconds > 0 {
			line += uitext.Text("item.card_summon_cooldown", d.CardSummonCDSeconds)
		}
		p = append(p, line)
	}
	if d.CardDisintegratePct != 0 {
		p = append(p, uitext.Text("item.on_direct_hit_disintegrate_undead_and_dragons", d.CardDisintegratePct))
	}
	if d.CardRegenPct != 0 {
		p = append(p, uitext.Text("item.regenerate_max_hp_per_regeneration_tick", d.CardRegenPct))
	}
	if d.CardDoubleAttackPct != 0 {
		p = append(p, uitext.Text("item.on_melee_attack_strike_again", d.CardDoubleAttackPct))
	}
	if d.CardSpellProcPct != 0 {
		p = append(p, uitext.Text("item.a_melee_swing_casts_a_fire_bolt", d.CardSpellProcPct))
	}
	if d.CardDodgeBonusPct != 0 {
		p = append(p, uitext.Text("item.perfect_dodge", d.CardDodgeBonusPct))
	}
	if d.CardArmorBonus != 0 {
		p = append(p, uitext.Text("item.card_armor_bonus", d.CardArmorBonus))
	}
	if d.CardThornsPct != 0 {
		p = append(p, uitext.Text("item.of_damage_received_from_monster_hits_reflected", d.CardThornsPct))
	}
	if d.CardPhysToDarkPct != 0 {
		p = append(p, uitext.Text("item.of_physical_damage_dealt_as_dark", d.CardPhysToDarkPct))
	}
	if d.CardPhysToLightPct != 0 {
		p = append(p, uitext.Text("item.of_physical_damage_dealt_as_light", d.CardPhysToLightPct))
	}
	if d.CardPoisonProcPct != 0 {
		p = append(p, uitext.Text("item.on_direct_hit_poison_for_s", d.CardPoisonProcPct, d.CardPoisonDurationSec))
	}
	if d.CardMeleeDmgPct != 0 {
		p = append(p, uitext.Text("item.melee_damage", d.CardMeleeDmgPct))
	}
	if d.CardMaxHPBonus != 0 {
		p = append(p, uitext.Text("item.max_hp", d.CardMaxHPBonus))
	}
	if len(d.CardResistBonus) > 0 {
		keys := make([]string, 0, len(d.CardResistBonus))
		for k := range d.CardResistBonus {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p = append(p, uitext.Text("item.resistance", d.CardResistBonus[k], titleCaseLower(k)))
		}
	}
	if d.CardGoldFindPct != 0 {
		p = append(p, uitext.Text("item.gold_from_kills", d.CardGoldFindPct))
	}
	if d.CardBonusBoltPct != 0 {
		p = append(p, uitext.Text("item.on_weapon_attack_fire_a_bonus_bolt", d.CardBonusBoltPct))
	}
	if d.CardVolleyBonusPct != 0 {
		p = append(p, uitext.Text("item.on_ranged_weapon_attack_fire_an_extra", d.CardVolleyBonusPct))
	}
	if d.CardStunOnHitPct != 0 {
		p = append(p, uitext.Text("item.on_direct_hit_stun_the_target", d.CardStunOnHitPct))
	}
	if d.CardPoisonResistPct != 0 {
		p = append(p, uitext.Text("item.chance_to_resist_monster_poison", d.CardPoisonResistPct))
	}
	if d.CardCritBonusPct != 0 {
		p = append(p, uitext.Text("item.critical_hit_chance", d.CardCritBonusPct))
	}
	if d.CardArmorPiercePct != 0 {
		p = append(p, uitext.Text("item.on_melee_hit_ignore_armor", d.CardArmorPiercePct))
	}
	if len(d.CardBonusVs) > 0 {
		keys := make([]string, 0, len(d.CardBonusVs))
		for k := range d.CardBonusVs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p = append(p, uitext.Text("item.damage_vs", (d.CardBonusVs[k]-1)*100, titleCaseLower(k)))
		}
	}
	return p
}
