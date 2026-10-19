package character

import "ugataima/internal/config"

// Read the actual personal bag at payout time: transfers and saves need no
// second cached ownership flag. Shared stock and quick-slot links do not count.
func (c *MMCharacter) PersonalInventoryRegenPct() int {
	best := 0
	for _, it := range c.Inventory {
		if def, _, ok := config.ItemDefOf(it); ok {
			best = max(best, def.PersonalInventoryRegenPct)
		}
	}
	return best
}

// WeaponBaseDamageBonus combines active party and equipped-item contributions.
// The shared weapon formula applies it before weapon-specific multipliers.
func (c *MMCharacter) WeaponBaseDamageBonus() int {
	bonus := c.BonusWeaponBaseDamage
	for _, it := range c.Equipment {
		if def, _, ok := config.ItemDefOf(it); ok {
			bonus += def.WeaponBaseDamageBonus
		}
	}
	return bonus
}
