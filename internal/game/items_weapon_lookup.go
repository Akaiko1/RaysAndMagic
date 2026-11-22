package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

var equippedWeaponSlots = [...]items.EquipSlot{items.SlotMainHand, items.SlotOffHand}

// lookupWeaponConfig resolves a weapon item's weapons.yaml entry. Returns nil
// and logs a warning if it is missing.
func lookupWeaponConfig(weapon items.Item) *config.WeaponDefinitionConfig {
	weaponDef, _, exists := config.WeaponDefOf(weapon)
	if !exists {
		fmt.Printf("[WARN] weapon '%s' not found in weapons.yaml\n", weapon.Name)
		return nil
	}
	return weaponDef
}

// lookupWeaponConfigByKey resolves a weapon by YAML key. Returns nil and logs a
// warning if the key is missing from weapons.yaml.
func lookupWeaponConfigByKey(weaponKey string) *config.WeaponDefinitionConfig {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		fmt.Printf("[WARN] weapon key '%s' not found in weapons.yaml\n", weaponKey)
		return nil
	}
	return weaponDef
}

// equippedWeaponDefinitions resolves both hands through the canonical
// name-indexed weapon catalog without allocating a slice.
func equippedWeaponDefinitions(member *character.MMCharacter) [2]*config.WeaponDefinitionConfig {
	var defs [2]*config.WeaponDefinitionConfig
	if member == nil {
		return defs
	}
	for i, slot := range equippedWeaponSlots {
		if weapon, ok := member.Equipment[slot]; ok && weapon.Type == items.ItemWeapon {
			defs[i] = lookupWeaponConfig(weapon)
		}
	}
	return defs
}

// strongestWeaponPerk is the wielded weapon with the best value of one caster
// perk (strength > 0). Either hand counts; two weapons never stack a perk.
func strongestWeaponPerk(member *character.MMCharacter, strength func(*config.WeaponDefinitionConfig) float64) *config.WeaponDefinitionConfig {
	var best *config.WeaponDefinitionConfig
	top := 0.0
	for _, def := range equippedWeaponDefinitions(member) {
		if def != nil {
			if s := strength(def); s > top {
				best, top = def, s
			}
		}
	}
	return best
}
