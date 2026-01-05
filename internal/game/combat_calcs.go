package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// CalculateSpellDamage returns base/stat/total damage for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual projectile damage.
func (cs *CombatSystem) CalculateSpellDamage(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	effectiveIntellect := char.GetEffectiveIntellect(cs.game.statBonus)
	baseDamage, intellectBonus, totalDamage := spells.CalculateSpellDamageByID(spellID, effectiveIntellect)
	masteryBonus := cs.spellMasteryBonus(char, spellID)
	if masteryBonus > 0 {
		baseDamage += masteryBonus
		totalDamage += masteryBonus
	}
	return baseDamage, intellectBonus, totalDamage
}

// CalculateSpellHealing returns base/stat/total healing for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual healing.
func (cs *CombatSystem) CalculateSpellHealing(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	effectivePersonality := char.GetEffectivePersonality(cs.game.statBonus)
	baseHeal, personalityBonus, totalHeal := spells.CalculateHealingAmountByID(spellID, effectivePersonality)
	masteryBonus := cs.spellMasteryBonus(char, spellID)
	if masteryBonus > 0 {
		baseHeal += masteryBonus
		totalHeal += masteryBonus
	}
	return baseHeal, personalityBonus, totalHeal
}

// CalculateSpellDurationSeconds returns duration in seconds with mastery bonus applied.
func (cs *CombatSystem) CalculateSpellDurationSeconds(spellID spells.SpellID, char *character.MMCharacter) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	if def.Duration <= 0 {
		return 0
	}
	seconds := def.Duration + cs.spellMasteryBonus(char, spellID)
	if char != nil && def.School != "" {
		school := character.MagicSchoolIDToLegacy(character.MagicSchoolID(def.School))
		if skill, exists := char.MagicSchools[school]; exists && skill != nil {
			multiplier := 1.0 + (float64(skill.Level) * 0.1)
			seconds = int(float64(seconds) * multiplier)
		}
	}
	return seconds
}

// CalculateSpellDurationFrames returns duration in frames with mastery bonus applied.
func (cs *CombatSystem) CalculateSpellDurationFrames(spellID spells.SpellID, char *character.MMCharacter) int {
	if cs == nil || cs.game == nil {
		return 0
	}
	seconds := cs.CalculateSpellDurationSeconds(spellID, char)
	if seconds <= 0 {
		return 0
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = config.GetTargetTPS()
	}
	return seconds * tps
}

// CalculateSpellStatBonus returns the spell's stat bonus with mastery applied (e.g., Bless).
func (cs *CombatSystem) CalculateSpellStatBonus(spellID spells.SpellID, char *character.MMCharacter) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	if def.StatBonus <= 0 {
		return 0
	}
	return def.StatBonus + cs.spellMasteryBonus(char, spellID)
}

// CalculateWeaponCritChance returns total crit chance (weapon base + luck bonus), clamped to [0,100].
func (cs *CombatSystem) CalculateWeaponCritChance(weapon items.Item, char *character.MMCharacter) int {
	baseCrit := 0
	if def, _, ok := config.GetWeaponDefinitionByName(weapon.Name); ok && def != nil {
		baseCrit = def.CritChance
	}
	total := baseCrit + cs.CalculateCriticalChance(char)
	if total < 0 {
		return 0
	}
	if total > 100 {
		return 100
	}
	return total
}

// CalculateArmorClassContribution returns this item's AC contribution based on endurance scaling and mastery.
func (cs *CombatSystem) CalculateArmorClassContribution(item items.Item, char *character.MMCharacter) int {
	if cs == nil || cs.game == nil || char == nil {
		return 0
	}
	baseArmor := item.Attributes["armor_class_base"]
	baseArmor += cs.armorMasteryBonus(char, item)
	enduranceDiv := item.Attributes["endurance_scaling_divisor"]
	if enduranceDiv > 0 {
		effectiveEndurance := char.GetEffectiveEndurance(cs.game.statBonus)
		baseArmor += effectiveEndurance / enduranceDiv
	}
	return baseArmor
}

// CalculateTotalArmorClass returns total AC from all equipped armor slots.
func (cs *CombatSystem) CalculateTotalArmorClass(char *character.MMCharacter) int {
	if cs == nil || cs.game == nil || char == nil {
		return 0
	}
	total := 0
	armorSlots := []items.EquipSlot{
		items.SlotArmor,
		items.SlotHelmet,
		items.SlotBoots,
		items.SlotCloak,
		items.SlotGauntlets,
		items.SlotBelt,
	}
	for _, slot := range armorSlots {
		if armorPiece, hasArmor := char.Equipment[slot]; hasArmor {
			total += cs.CalculateArmorClassContribution(armorPiece, char)
		}
	}
	return total
}

// CalculateSpellRangeTiles returns the configured range in tiles for a spell.
func (cs *CombatSystem) CalculateSpellRangeTiles(spellID spells.SpellID) (float64, bool) {
	def, ok := config.GetSpellDefinition(string(spellID))
	if !ok || def == nil {
		return 0, false
	}
	if def.Physics != nil && def.Physics.RangeTiles > 0 {
		return def.Physics.RangeTiles, true
	}
	if def.Range > 0 {
		return float64(def.Range), true
	}
	return 0, false
}
