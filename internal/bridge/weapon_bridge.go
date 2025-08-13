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

	// Convert config weapon to items weapon definition
	itemsWeapon := &items.WeaponDefinitionFromYAML{
		Name:               weaponDef.Name,
		Description:        weaponDef.Description,
		Category:           weaponDef.Category,
		Damage:             weaponDef.Damage,
		Range:              weaponDef.Range,
		BonusStat:          weaponDef.BonusStat,
		BonusStatSecondary: weaponDef.BonusStatSecondary,
		DamageType:         weaponDef.DamageType,
		MaxProjectiles:     weaponDef.MaxProjectiles,
		HitBonus:           weaponDef.HitBonus,
		CritChance:         weaponDef.CritChance,
		Rarity:             weaponDef.Rarity,
	}

	return itemsWeapon, true
}
