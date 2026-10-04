package game

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

type AdventureLoot struct {
	Items       []items.Item `json:"items,omitempty"`
	Gold        int          `json:"gold,omitempty"`
	ArenaPoints int          `json:"arena_points,omitempty"`
}

// Roll closed-visit caches at creation, before the arrival autosave. NPC visited
// flags and ground-container state own consumption; these rolls never grant.
func (g *MMGame) prepareAdventureLoot(key string, v *AdventureVisit) {
	v.Loot = map[string]AdventureLoot{}
	w := g.worldByKey(key)
	if w == nil {
		return
	}
	for _, n := range w.NPCs {
		c := config.GetCrateConfig(n.Key)
		if c == nil {
			continue
		}
		var loot AdventureLoot
		if c.LootTable != "" {
			loot.Items, loot.Gold = rollWeightedLootTable(c.LootTable)
		} else {
			loot.Items, loot.Gold, loot.ArenaPoints = g.rollCratePool(c)
		}
		v.Loot["crate:"+n.Key] = loot
	}
	for _, m := range w.Monsters {
		if m.EncounterRewards == nil {
			continue
		}
		chests := append([]monster.TreasureChestReward(nil), m.EncounterRewards.TreasureChests...)
		if m.EncounterRewards.TreasureChest != nil {
			chests = append(chests, *m.EncounterRewards.TreasureChest)
		}
		for _, c := range chests {
			if c.LootTable == "" {
				continue
			}
			k := "table:" + c.LootTable
			if _, ok := v.Loot[k]; ok {
				continue
			}
			it, gold := rollWeightedLootTable(c.LootTable)
			v.Loot[k] = AdventureLoot{Items: it, Gold: gold}
		}
	}
}

func (g *MMGame) adventureLoot(key, id string) (AdventureLoot, bool) {
	if v := g.adventure.Visits[key]; v != nil {
		l, ok := v.Loot[id]
		l.Items = append([]items.Item(nil), l.Items...)
		return l, ok
	}
	return AdventureLoot{}, false
}
