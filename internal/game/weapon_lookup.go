package game

import (
	"fmt"
	"ugataima/internal/config"
)

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
