package config

import (
	"fmt"
	"sort"
)

func validateDirectCrateSource(where string, s CrateRollSource) error {
	direct := s.Pool == "item"
	slot := s.Pool == "equipment_slot"
	if (s.ItemKey != "") != direct || (s.EquipSlot != "") != slot {
		return fmt.Errorf("%s: item and equip_slot belong to their respective source pools", where)
	}
	if !direct && (s.QuantityMin != 0 || s.QuantityMax != 0) {
		return fmt.Errorf("%s: quantities require an item source", where)
	}
	if direct || slot {
		if s.ItemType != "" || s.Amount != 0 || direct && s.Rarity != "" {
			return fmt.Errorf("%s: item/slot sources take no item_type or amount; rarity requires a slot pool", where)
		}
	}
	if direct {
		d, ok := GetItemDefinition(s.ItemKey)
		// Authored caches may contain gathered ingredients. Crafted-only output
		// remains recipe-owned, and ordinary catalog rolls keep their exclusions.
		if !ok || d.CraftedOnly || d.Type == "quest" {
			return fmt.Errorf("%s: invalid cache item %q", where, s.ItemKey)
		}
		if s.QuantityMin < 0 || s.QuantityMax < 0 || (s.QuantityMin > 0 || s.QuantityMax > 0) && s.QuantityMax < max(1, s.QuantityMin) {
			return fmt.Errorf("%s: invalid quantity_min/quantity_max", where)
		}
		if s.QuantityMax > 1 && d.Type != "consumable" && d.Type != "trinket" {
			return fmt.Errorf("%s: quantities above one require stackable pocket goods", where)
		}
	}

	return nil
}

// CrateEquipmentSlotKeys is the shared validation/runtime slot and rarity pool.
// Explicit slot rewards include named and boss equipment; ordinary catalog
// rolls preserve their separate exclusions. Sorted keys make seeded rolls stable.
func CrateEquipmentSlotKeys(slot string, tiers RarityRange) []string {
	var keys []string
	if GlobalItems != nil {
		for key, d := range GlobalItems.Items {
			if d != nil && (d.Type == "armor" || d.Type == "accessory") && d.EquipSlot == slot && !d.CraftedOnly && tiers.Contains(RarityTier(d.Rarity)) {
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	return keys
}
