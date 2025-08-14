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

    switch item.Name {
    case "Health Potion":
        ch := g.party.Members[selectedChar]
        healAmount := 25 + (ch.Endurance / 4)
        before := ch.HitPoints
        ch.HitPoints += healAmount
        if ch.HitPoints > ch.MaxHitPoints {
            ch.HitPoints = ch.MaxHitPoints
        }
        actual := ch.HitPoints - before
        // Remove the potion
        g.party.RemoveItem(itemIndex)
        g.AddCombatMessage(fmt.Sprintf("%s drinks a Health Potion and heals %d HP!", ch.Name, actual))
        return true

    case "Dead Branch":
        // Summon a random monster ~2 tiles away from player
        if g.SummonRandomMonsterNearPlayer(2.0) {
            g.party.RemoveItem(itemIndex)
            g.AddCombatMessage("The Dead Branch crackles and a monster appears!")
            return true
        }
        g.AddCombatMessage("The branch fizzles... no place to summon.")
        return false

    default:
        g.AddCombatMessage("Nothing happens.")
        return false
    }
}

