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

// HealablePartyIndices returns conscious, wounded members (HP below max, alive,
// not unconscious/dead/eradicated) — valid targets for a heal potion. Used by
// the heal picker when an unconscious owner can't heal themselves.
func (g *MMGame) HealablePartyIndices() []int {
	if g == nil || g.party == nil {
		return nil
	}
	var idxs []int
	for i, m := range g.party.Members {
		if m == nil {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) ||
			m.HasCondition(character.ConditionDead) ||
			m.HasCondition(character.ConditionEradicated) {
			continue
		}
		if m.HitPoints > 0 && m.HitPoints < m.MaxHitPoints {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// applyHealTo consumes the heal consumable at itemIdx and heals the member at
// targetIdx. Shared by the self-heal fast path and the heal picker. Re-validates
// the item (the index can shift between picker-open and confirm) and refuses on
// an invalid/full/incapacitated target. Returns true if the heal applied.
func (g *MMGame) applyHealTo(itemIdx, targetIdx int) bool {
	if itemIdx < 0 || itemIdx >= len(g.party.Inventory) {
		return false
	}
	if targetIdx < 0 || targetIdx >= len(g.party.Members) {
		return false
	}
	item := g.party.Inventory[itemIdx]
	base := item.Attributes["heal_base"]
	div := item.Attributes["heal_endurance_divisor"]
	if item.Type != items.ItemConsumable || base <= 0 || div <= 0 {
		return false // slot now holds something else — inventory shifted under us
	}
	ch := g.party.Members[targetIdx]
	if ch.HasCondition(character.ConditionUnconscious) || ch.HasCondition(character.ConditionDead) {
		return false // heals never revive
	}
	if ch.HitPoints >= ch.MaxHitPoints {
		return false
	}
	before := ch.HitPoints
	ch.HitPoints += base + (ch.GetEffectiveEndurance() / div)
	if ch.HitPoints > ch.MaxHitPoints {
		ch.HitPoints = ch.MaxHitPoints
	}
	if ch.HitPoints > before {
		g.TriggerPartyHeal(targetIdx) // rising green "+" overlay
	}
	g.party.RemoveItem(itemIdx)
	g.AddCombatMessage(fmt.Sprintf("%s uses %s and heals %d HP!", ch.Name, item.Name, ch.HitPoints-before))
	return true
}

// resolvePickerQuickSource finishes a heal/revival picker that was opened FROM a
// quick slot. consumed=true (potion spent) clears the source slot; consumed=false
// (cancelled / not applied) drops the temp bag copy at itemIdx and leaves the slot
// filled — so cancelling never silently moves the potion to the backpack. No-op
// when the picker was opened from the inventory (pickerQuickChar < 0).
func (g *MMGame) resolvePickerQuickSource(itemIdx int, consumed bool) {
	if g.pickerQuickChar < 0 {
		return
	}
	if !consumed {
		if itemIdx >= 0 && itemIdx < len(g.party.Inventory) {
			g.party.RemoveItem(itemIdx) // drop the temp bag copy; slot keeps the potion
		}
	} else if g.pickerQuickChar < len(g.party.Members) {
		if ch := g.party.Members[g.pickerQuickChar]; ch != nil &&
			g.pickerQuickSlot >= 0 && g.pickerQuickSlot < len(ch.QuickSlots) {
			ch.QuickSlots[g.pickerQuickSlot] = nil
		}
	}
	g.pickerQuickChar, g.pickerQuickSlot = -1, -1
}

// UseConsumableFromInventory consumes a consumable item at inventory index for the selected character.
// Handles game-side effects, inventory removal, and combat messages. Returns true if consumed.
func (g *MMGame) UseConsumableFromInventory(itemIndex int, selectedChar int) bool {
	if g == nil || g.party == nil {
		return false
	}
	// Default any picker this opens to "from inventory"; the quick-slot caller
	// re-tags it after the call if a picker actually opened.
	g.pickerQuickChar, g.pickerQuickSlot = -1, -1
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

	// Cure-poison consumable (antivenom). Checked before the heal branch so a
	// poisoned-but-full-HP character can still be cured (the heal branch refuses
	// at full HP). Applies any minor heal the item also carries.
	if item.Attributes["cure_poison"] > 0 {
		ch := g.party.Members[selectedChar]
		if !ch.HasCondition(character.ConditionPoisoned) {
			g.AddCombatMessage(fmt.Sprintf("%s isn't poisoned.", ch.Name))
			return false
		}
		ch.RemoveCondition(character.ConditionPoisoned)
		if base := item.Attributes["heal_base"]; base > 0 && !ch.HasCondition(character.ConditionUnconscious) {
			heal := base
			if div := item.Attributes["heal_endurance_divisor"]; div > 0 {
				heal = base + (ch.GetEffectiveEndurance() / div)
			}
			before := ch.HitPoints
			ch.HitPoints += heal
			if ch.HitPoints > ch.MaxHitPoints {
				ch.HitPoints = ch.MaxHitPoints
			}
			if ch.HitPoints > before {
				g.TriggerPartyHeal(selectedChar)
			}
		}
		g.party.RemoveItem(itemIndex)
		g.AddCombatMessage(fmt.Sprintf("%s drinks %s — the venom subsides.", ch.Name, item.Name))
		return true
	}

	// Healing consumable
	if base, okBase := item.Attributes["heal_base"]; okBase {
		div, okDiv := item.Attributes["heal_endurance_divisor"]
		if !okDiv || base <= 0 || div <= 0 {
			g.AddCombatMessage(fmt.Sprintf("%s is misconfigured (healing attributes)", item.Name))
			return false
		}
		ch := g.party.Members[selectedChar]
		// An unconscious owner can't heal themselves (plain heals never revive), so
		// route the potion to a conscious, wounded ally: 0 → keep it, 1 → heal them,
		// N → let the player choose (heal picker).
		if ch.HasCondition(character.ConditionUnconscious) {
			targets := g.HealablePartyIndices()
			switch len(targets) {
			case 0:
				g.AddCombatMessage("No conscious party member needs healing.")
				return false
			case 1:
				return g.applyHealTo(itemIndex, targets[0])
			default:
				g.healPickerOpen = true
				g.healPickerItemIdx = itemIndex
				return false
			}
		}
		if ch.HitPoints >= ch.MaxHitPoints {
			// Nothing to heal: keep the potion instead of wasting it.
			g.AddCombatMessage(fmt.Sprintf("%s is already at full health.", ch.Name))
			return false
		}
		return g.applyHealTo(itemIndex, selectedChar)
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
