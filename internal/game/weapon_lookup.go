package game

import (
	"fmt"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// lookupWeaponConfigByName resolves a weapon by display name. Returns nil and
// logs a warning if the weapon is missing from weapons.yaml.
func lookupWeaponConfigByName(weaponName string) *config.WeaponDefinitionConfig {
	weaponKey := items.GetWeaponKeyByName(weaponName)
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		fmt.Printf("[WARN] weapon '%s' (key: %s) not found in weapons.yaml\n", weaponName, weaponKey)
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
