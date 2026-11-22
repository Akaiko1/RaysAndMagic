package config

import "fmt"

// Expand before validation/publication: death, theft, previews and map loot
// pools all read the same flat table. Cycles fail at boot.
func expandMonsterLootGroups(c *LootTablesConfig) error {
	for key := range c.LootGroups {
		if _, exists := c.Loots[key]; exists {
			return fmt.Errorf("loot group %q duplicates a monster table", key)
		}
	}
	resolved := make(map[string][]LootEntry)
	visiting := make(map[string]bool)
	var resolve func(string) ([]LootEntry, error)
	resolve = func(key string) ([]LootEntry, error) {
		if result, ok := resolved[key]; ok {
			return result, nil
		}
		if visiting[key] {
			return nil, fmt.Errorf("cyclic loot group %q", key)
		}
		entries, ok := c.LootGroups[key]
		if !ok {
			entries, ok = c.Loots[key]
		}
		if !ok {
			return nil, fmt.Errorf("unknown loot group %q", key)
		}
		visiting[key] = true
		var flat []LootEntry
		for i, e := range entries {
			if e.Type == "gold" {
				if _, group := c.LootGroups[key]; group {
					return nil, fmt.Errorf("loot group %q: gold belongs in a monster's own table", key)
				}
				if e.KillOnly {
					return nil, fmt.Errorf("loots %q: gold is rolled at spawn; kill_only does not apply to it", key)
				}
				flat = append(flat, e) // splitMonsterGold validates it
				continue
			}
			if e.Type == "first_of" {
				// An ordered group stays one entry: its first success is its drop.
				if e.Chance != 0 || e.Rolls < 0 || e.Min != 0 || e.Max != 0 {
					return nil, fmt.Errorf("loot group %q: first_of references accept rolls, not chance or min/max", key)
				}
				if _, group := c.LootGroups[e.Key]; !group {
					return nil, fmt.Errorf("loot group %q: first_of %q is not a loot_groups entry", key, e.Key)
				}
				children, err := resolve(e.Key)
				if err != nil {
					return nil, err
				}
				for _, child := range children {
					if child.Type == "first_of" || child.Type == "gold" {
						return nil, fmt.Errorf("loot group %q: first_of %q holds a %s entry", key, e.Key, child.Type)
					}
				}
				e.Of = children
				flat = append(flat, e)
				continue
			}
			if e.Type != "table" {
				if err := validateLootEntry(key, i, e); err != nil {
					return nil, err
				}
				flat = append(flat, e)
				continue
			}
			if e.Chance != 0 || e.Rolls < 0 || e.Min != 0 || e.Max != 0 {
				return nil, fmt.Errorf("loot group %q: table references accept rolls, not chance or min/max", key)
			}
			children, err := resolve(e.Key)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				if child.Type == "gold" {
					continue // a referenced table's gold stays its own
				}
				child.Rolls = child.RollCount() * e.RollCount()
				// A kill_only reference makes every drop it brings a kill trophy.
				child.KillOnly = child.KillOnly || e.KillOnly
				flat = append(flat, child)
			}
		}
		delete(visiting, key)
		resolved[key] = flat
		return flat, nil
	}
	for key := range c.LootGroups {
		if _, err := resolve(key); err != nil {
			return err
		}
	}
	for key := range c.Loots {
		if _, err := resolve(key); err != nil {
			return err
		}
	}
	for key := range c.Loots {
		c.Loots[key] = resolved[key]
	}
	return nil
}
