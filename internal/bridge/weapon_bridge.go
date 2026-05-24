package bridge

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SetupWeaponBridge configures the weapon accessor bridge
func SetupWeaponBridge() {
	items.GlobalWeaponAccessor = getWeaponFromConfig
}

// getWeaponFromConfig retrieves weapon definition from config
func getWeaponFromConfig(weaponKey string) (*items.WeaponDefinitionFromYAML, bool) {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		return nil, false
	}

	return &items.WeaponDefinitionFromYAML{
		Name:        weaponDef.Name,
		Description: weaponDef.Description,
		Category:    weaponDef.Category,
		Rarity:      weaponDef.Rarity,
		Value:       weaponDef.Value,
	}, true
}
