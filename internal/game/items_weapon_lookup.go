package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

var equippedWeaponSlots = [...]items.EquipSlot{items.SlotMainHand, items.SlotOffHand}

// lookupWeaponConfigByName resolves a weapon by display name. Returns nil and
// logs a warning if the weapon is missing from weapons.yaml.
func lookupWeaponConfigByName(weaponName string) *config.WeaponDefinitionConfig {
	weaponDef, _, exists := config.GetWeaponDefinitionByName(weaponName)
	if !exists {
		fmt.Printf("[WARN] weapon '%s' not found in weapons.yaml\n", weaponName)
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
			defs[i] = lookupWeaponConfigByName(weapon.Name)
		}
	}
	return defs
}
