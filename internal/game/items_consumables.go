package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

// RevivablePartyIndices returns the indices of party members who can be
// revived by a revival potion: currently Dead OR Unconscious, but NOT
// Eradicated. Used by the revival-picker UI so dead members (who can't be
// portrait-clicked) can still be chosen as targets.
func (g *MMGame) RevivablePartyIndices() []int {
	if g == nil || g.party == nil {
		return nil
	}
	var idxs []int
	for i, m := range g.party.Members {
		if m == nil || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		if m.HasCondition(character.ConditionDead) ||
			m.HasCondition(character.ConditionUnconscious) ||
			m.HitPoints <= 0 {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// applyReviveTo consumes the revival item at itemIdx and revives the party
// member at targetIdx. Shared by the 1-target fast path and the picker
// confirmation. Re-validates the item at itemIdx is actually a revive
// consumable: the index can become stale between picker-open and confirm
// if other inventory operations (drop/sell/use) shift the slice.
// Returns true if the revive applied.
func (g *MMGame) applyReviveTo(itemIdx, targetIdx int) bool {
	if itemIdx < 0 || itemIdx >= len(g.party.Inventory) {
		return false
	}
	if targetIdx < 0 || targetIdx >= len(g.party.Members) {
		return false
	}
	item := g.party.Inventory[itemIdx]
	if item.Type != items.ItemConsumable || item.Attributes["revive"] <= 0 {
		// Slot now holds something else — inventory shifted under us.
		return false
	}
	ch := g.party.Members[targetIdx]
	ch.RemoveCondition(character.ConditionUnconscious)
	ch.RemoveCondition(character.ConditionDead)
	if item.Attributes["full_heal"] > 0 {
		ch.HitPoints = ch.MaxHitPoints
	} else if ch.HitPoints <= 0 {
		ch.HitPoints = 1
	}
	g.party.RemoveItem(itemIdx)
	g.AddCombatMessage(fmt.Sprintf("%s uses %s and is revived!", ch.Name, item.Name))
	return true
}

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
	// Revive consumable: branch on number of revivable party members.
	//   0 → nothing to do, don't waste the potion
	//   1 → revive immediately (no picker needed)
	//   N → open revivalPicker, target gets revived when user confirms
	if item.Attributes["revive"] > 0 {
		targets := g.RevivablePartyIndices()
		switch len(targets) {
		case 0:
			g.AddCombatMessage("No one in the party needs reviving.")
			return false
		case 1:
			return g.applyReviveTo(itemIndex, targets[0])
		default:
			g.revivalPickerOpen = true
			g.revivalPickerItemIdx = itemIndex
			return false
		}
	}

	// Healing consumable
	if base, okBase := item.Attributes["heal_base"]; okBase {
		if div, okDiv := item.Attributes["heal_endurance_divisor"]; okDiv && base > 0 && div > 0 {
			ch := g.party.Members[selectedChar]
			healAmount := base + (ch.GetEffectiveEndurance() / div)
			before := ch.HitPoints
			ch.HitPoints += healAmount
			if ch.HitPoints > ch.MaxHitPoints {
				ch.HitPoints = ch.MaxHitPoints
			}
			actual := ch.HitPoints - before
			if actual > 0 {
				g.TriggerPartyHeal(selectedChar) // rising green "+" overlay
			}
			// Potions do not remove Unconscious by themselves
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
