package bridge

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SetupWeaponBridge configures the weapon accessor bridge
func SetupWeaponBridge() {
	items.GlobalWeaponAccessor = getWeaponFromConfig
	// Resolve display name -> YAML key through the config's O(1) name index, so
	// flavor-named weapons (commas, macrons, "of the ...") map to the right key.
	items.GlobalWeaponKeyByName = func(name string) (string, bool) {
		_, key, ok := config.GetWeaponDefinitionByName(name)
		return key, ok
	}
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
