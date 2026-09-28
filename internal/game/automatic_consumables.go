package game

import (
	"math"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Automatic drinking uses the manual consumption path; it only selects a source.
// Its clock advances in the simulation, never while a modal owns inventory indices.
func (g *MMGame) updateAutomaticConsumables() {
	if g == nil || g.party == nil || g.config == nil || g.automaticInventoryBlocked() || g.gameplayPausedByOverlay() {
		return
	}
	rules := g.config.Characters.AutoDrink
	if g.config.AutoPotionThreshold(false) <= 0 && g.config.AutoPotionThreshold(true) <= 0 {
		return
	}
	for index, ch := range g.party.Members {
		if ch == nil {
			continue
		}
		if !g.turnBasedMode && ch.AutoDrinkCooldown > 0 {
			ch.AutoDrinkCooldown--
		}
		if ch.AutoDrinkCooldown > 0 || !ch.CanUseCombatAction() {
			continue
		}
		resources := []string{}
		if ch.HitPoints*100 < ch.MaxHitPoints*g.config.AutoPotionThreshold(false) {
			resources = append(resources, "heal_base")
		}
		if ch.MaxSpellPoints > 0 && ch.SpellPoints*100 < ch.MaxSpellPoints*g.config.AutoPotionThreshold(true) {
			resources = append(resources, "mana_base")
		}
		used := false
		for _, resource := range resources {
			for slot, item := range ch.QuickSlots {
				if g.automaticRestorative(item, resource) && g.useQuickConsumable(index, slot) {
					used = true
					break
				}
			}
			for _, owner := range []*character.MMCharacter{ch, nil} {
				if used {
					break
				}
				bag := g.party.Bag(owner)
				for itemIndex := range bag.Items() {
					if g.automaticRestorative(&bag.Items()[itemIndex], resource) && g.UseConsumableFromInventory(itemIndex, index, owner) {
						used = true
						break
					}
				}
			}
			if used {
				break
			}
		}

		if used {
			seconds := rules.IntervalSeconds
			if g.turnBasedMode {
				seconds = max(seconds, float64(TurnBasedPeriodicEffectSeconds))
			}
			ch.AutoDrinkCooldown = max(1, int(math.Ceil(seconds*float64(g.config.GetTPS()))))
		}
	}
}

func (g *MMGame) automaticRestorative(item *items.Item, resource string) bool {
	if item == nil || item.Type != items.ItemConsumable || item.Count() <= 0 || item.Attributes[resource] <= 0 || item.Attributes["revive"] > 0 || item.Attributes["summon_distance_tiles"] > 0 {
		return false
	}
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	return ok && g.config.AllowsAutoPotion(def, resource == "mana_base")
}

// useQuickConsumable keeps the existing temporary-entry/picker ownership contract.
func (g *MMGame) useQuickConsumable(charIdx, slotIdx int) bool {
	ch := g.party.Members[charIdx]
	item := ch.QuickSlots[slotIdx]
	if item == nil || item.Type != items.ItemConsumable {
		return false
	}
	drink := *item
	drink.Quantity = 1
	g.party.Inventory = append(g.party.Inventory, drink)
	idx := len(g.party.Inventory) - 1
	used := g.UseConsumableFromInventory(idx, charIdx)
	switch {
	case used:
		g.decrementQuickSlot(ch, slotIdx)
	case g.revivalPickerOpen || g.healPickerOpen:
		g.pickerQuickChar, g.pickerQuickSlot = charIdx, slotIdx
	default:
		g.party.RemoveItem(idx)
	}
	return used
}

func (g *MMGame) automaticInventoryBlocked() bool {
	ui := &UISystem{game: g}
	if g.gameLoop != nil && g.gameLoop.ui != nil {
		ui = g.gameLoop.ui
	}
	return ui.inventoryInputBlocked()
}
