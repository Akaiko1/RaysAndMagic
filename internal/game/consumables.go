package game

import (
    "fmt"
    "ugataima/internal/items"
)

// UseConsumableFromInventory consumes a consumable item at inventory index for the selected character.
// Handles game-side effects, inventory removal, and combat messages. Returns true if consumed.
func (g *MMGame) UseConsumableFromInventory(itemIndex int, selectedChar int) bool {
    if g == nil || g.party == nil {
        return false
    }
    if itemIndex < 0 || itemIndex >= len(g.party.Inventory) {
        return false
    }
    if selectedChar < 0 || selectedChar >= len(g.party.Members) {
        return false
    }

    item := g.party.Inventory[itemIndex]
    if item.Type != items.ItemConsumable {
        return false
    }
    // Attribute-driven behaviors (single source of truth)
    // Healing consumable
    if base, okBase := item.Attributes["heal_base"]; okBase {
        if div, okDiv := item.Attributes["heal_endurance_divisor"]; okDiv && base > 0 && div > 0 {
            ch := g.party.Members[selectedChar]
            healAmount := base + (ch.Endurance / div)
            before := ch.HitPoints
            ch.HitPoints += healAmount
            if ch.HitPoints > ch.MaxHitPoints {
                ch.HitPoints = ch.MaxHitPoints
            }
            actual := ch.HitPoints - before
            // Direct heal (spell) is required to remove Unconscious; potions do not revive
            g.party.RemoveItem(itemIndex)
            g.AddCombatMessage(fmt.Sprintf("%s uses %s and heals %d HP!", ch.Name, item.Name, actual))
            return true
        }
        g.AddCombatMessage(fmt.Sprintf("%s is misconfigured (healing attributes)", item.Name))
        return false
    }

    // Summoning consumable
    if dist, ok := item.Attributes["summon_distance_tiles"]; ok {
        if dist > 0 {
            if g.SummonRandomMonsterNearPlayer(float64(dist)) {
                g.party.RemoveItem(itemIndex)
                g.AddCombatMessage("A ripple in the air answers your call.")
                return true
            }
            g.AddCombatMessage("The air resists; no space to summon.")
            return false
        }
        g.AddCombatMessage(fmt.Sprintf("%s is misconfigured (summon distance)", item.Name))
        return false
    }

    // Unknown consumable behavior
    g.AddCombatMessage("Nothing happens.")
    return false
}
