package game

import (
	"strings"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// RevivablePartyIndices returns the indices of party members who can be
// revived by a revival potion: currently Dead OR Unconscious, but NOT
// Eradicated. Used by the revival-picker UI to list eligible targets.
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
	bag := g.party.Bag(g.pickerInventoryOwner)
	if itemIdx < 0 || itemIdx >= len(bag.Items()) {
		return false
	}
	if targetIdx < 0 || targetIdx >= len(g.party.Members) {
		return false
	}
	item := bag.Items()[itemIdx]
	if item.Type != items.ItemConsumable || item.Attributes["revive"] <= 0 {
		// Slot now holds something else - inventory shifted under us.
		return false
	}
	ch := g.party.Members[targetIdx]
	if item.Attributes["full_heal"] > 0 {
		ch.Revive(ch.MaxHitPoints, false)
		g.cleanseHealingRecipient(ch)
	} else {
		ch.Revive(1, false)
	}
	bag.Consume(itemIdx, 1)
	g.AddCombatMessage(uitext.Text("combat.uses_and_is_revived", ch.Name, item.Name))
	return true
}

// HealablePartyIndices returns wounded members eligible for ordinary healing.
// Used by the heal picker when an incapacitated owner cannot heal themselves.
func (g *MMGame) HealablePartyIndices() []int {
	if g == nil || g.party == nil {
		return nil
	}
	var idxs []int
	for i, m := range g.party.Members {
		if !m.IsLiving() {
			continue
		}
		if g.healingPotionHasEffect(m) {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// applyFlatHeal adds a heal_base(+Endurance/div) amount to the member at
// charIdx, clamped to MaxHitPoints, firing the rising "+" VFX if it actually
// raised HP. Refuses on a member who is not living (0 HP, Dead, Eradicated):
// heals never revive. Shared by applyHealTo and any
// consumable that carries a secondary heal (e.g. antivenom's minor heal) so
// there is exactly one clamp/VFX/revive-guard implementation, not per-caller
// copies that can drift.
func (g *MMGame) applyFlatHeal(charIdx int, base, div int) {
	if charIdx < 0 || charIdx >= len(g.party.Members) || base <= 0 {
		return
	}
	ch := g.party.Members[charIdx]
	if !ch.IsLiving() {
		return
	}
	heal := character.ConsumableRestore(ch, base, div, false, g.party.PotionSupport(ch, base, div, false))
	before := ch.HitPoints
	ch.HitPoints += heal
	if ch.HitPoints > ch.MaxHitPoints {
		ch.HitPoints = ch.MaxHitPoints
	}
	g.potionOverflow(ch, heal-(ch.HitPoints-before))
	ch.RemoveCondition(character.ConditionUnconscious) // as healMember: a living member wakes
	if ch.HitPoints > before {
		g.TriggerPartyHeal(charIdx) // rising green "+" overlay
	}
}

// applyHealTo consumes the heal consumable at itemIdx and heals the member at
// targetIdx. Shared by the self-heal fast path and the heal picker. Re-validates
// the item (the index can shift between picker-open and confirm) and refuses on
// an invalid/full/incapacitated target. Returns true if the heal applied.
func (g *MMGame) applyHealTo(itemIdx, targetIdx int) bool {
	bag := g.party.Bag(g.pickerInventoryOwner)
	if itemIdx < 0 || itemIdx >= len(bag.Items()) {
		return false
	}
	if targetIdx < 0 || targetIdx >= len(g.party.Members) {
		return false
	}
	item := bag.Items()[itemIdx]
	base := item.Attributes["heal_base"]
	div := item.Attributes["heal_endurance_divisor"]
	if item.Type != items.ItemConsumable || base <= 0 || div <= 0 {
		return false // slot now holds something else - inventory shifted under us
	}
	ch := g.party.Members[targetIdx]
	if !ch.IsLiving() {
		return false // heals never revive - Eradicated needs the Resurrect spell
	}
	if !g.healingPotionHasEffect(ch) {
		return false
	}
	before := ch.HitPoints
	g.applyFlatHeal(targetIdx, base, div)
	g.cleanseHealingRecipient(ch)
	bag.Consume(itemIdx, 1)
	g.logCombatLine(logToneGood, uitext.Text("combat.uses_and_heals_hp", logSlot, logSlot, logSlot), logHeroName(ch), logItemName(item), logHealed(ch.HitPoints-before))
	return true
}

// resolvePickerQuickSource finishes a heal/revival picker that was opened FROM a
// quick slot. consumed=true (potion spent) clears the source slot; consumed=false
// (cancelled / not applied) drops the temp bag copy at itemIdx and leaves the slot
// filled - so cancelling never silently moves the potion to the backpack. No-op
// when the picker was opened from the inventory (pickerQuickChar < 0).
func (g *MMGame) resolvePickerQuickSource(itemIdx int, consumed bool) {
	defer func() { g.pickerInventoryOwner = nil }()
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
			g.decrementQuickSlot(ch, g.pickerQuickSlot)
		}
	}
	g.pickerQuickChar, g.pickerQuickSlot = -1, -1
}

// Picker cancellation helpers: the ONE body shared by the ESC edge (HandleInput)
// and the popup's close button, so the potion-source bookkeeping cannot drift.
// The promotion picker has no cancel path by design (the promotion is already
// committed when it opens).
func (g *MMGame) cancelRevivalPicker() {
	g.resolvePickerQuickSource(g.revivalPickerItemIdx, false)
	g.revivalPickerOpen = false
}

func (g *MMGame) cancelHealPicker() {
	g.resolvePickerQuickSource(g.healPickerItemIdx, false)
	g.healPickerOpen = false
}

func (g *MMGame) cancelTownPortalPicker() {
	g.townPortalPickerOpen, g.townPortalCaster, g.townPortalSpell = false, nil, ""
	g.beaconPicker, g.beaconConfirming = false, false
	g.identifiedMonster, g.identifyScroll = nil, 0
	g.identifySelecting, g.identifyConfirming = false, nil
	g.allySpellSelecting = false
}

// UseConsumableFromInventory consumes a consumable item at inventory index for the selected character.
// Handles game-side effects, inventory removal, and combat messages. Returns true if consumed.
func (g *MMGame) UseConsumableFromInventory(itemIndex int, selectedChar int, owner ...*character.MMCharacter) bool {
	if g == nil || g.party == nil {
		return false
	}
	// Default any picker this opens to "from inventory"; the quick-slot caller
	// re-tags it after the call if a picker actually opened.
	bag := g.party.Bag(owner...)
	g.pickerInventoryOwner = bag.Owner
	defer func() {
		if !g.healPickerOpen && !g.revivalPickerOpen {
			g.pickerInventoryOwner = nil
		}
	}()
	g.pickerQuickChar, g.pickerQuickSlot = -1, -1
	if itemIndex < 0 || itemIndex >= len(bag.Items()) {
		return false
	}
	if selectedChar < 0 || selectedChar >= len(g.party.Members) {
		return false
	}

	item := bag.Items()[itemIndex]
	if item.Type != items.ItemConsumable {
		return false
	}
	if key, d := flaskDefinition(item); d != nil {
		return g.equipFlask(selectedChar, key)
	}
	// Attribute-driven behaviors (single source of truth)
	// Revive consumable: branch on number of revivable party members.
	//   0 -> nothing to do, don't waste the potion
	//   1 -> revive immediately (no picker needed)
	//   N -> open revivalPicker, target gets revived when user confirms
	if item.Attributes["revive"] > 0 {
		targets := g.RevivablePartyIndices()
		switch len(targets) {
		case 0:
			g.AddCombatMessage(uitext.Text("combat.no_one_in_the_party_needs"))
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
		if ch.HasCondition(character.ConditionPoisoned) {
			ch.CurePoison()
			g.applyFlatHeal(selectedChar, item.Attributes["heal_base"], item.Attributes["heal_endurance_divisor"])
			if item.Attributes["heal_base"] > 0 {
				g.cleanseHealingRecipient(ch)
			}
			bag.Consume(itemIndex, 1)
			g.AddCombatMessage(uitext.Text("combat.drinks_the_venom_subsides", ch.Name, item.Name))
			return true
		}
		// Without poison, a restorative antidote follows ordinary healing below,
		// including full-health rejection and incapacitated-owner redirection.
		if item.Attributes["heal_base"] <= 0 {
			g.AddCombatMessage(uitext.Text("combat.isn_t_poisoned", ch.Name))
			return false
		}
	}

	// Healing consumable
	if base, okBase := item.Attributes["heal_base"]; okBase {
		div, okDiv := item.Attributes["heal_endurance_divisor"]
		if !okDiv || base <= 0 || div <= 0 {
			g.AddCombatMessage(uitext.Text("combat.is_misconfigured_healing_attributes", item.Name))
			return false
		}
		ch := g.party.Members[selectedChar]
		// An owner who cannot receive ordinary healing cannot drink this potion for
		// themselves (plain heals never revive), so route it to a valid wounded ally:
		// 0 -> keep it, 1 -> heal them, N -> let the player choose (heal picker).
		if !ch.IsLiving() {
			targets := g.HealablePartyIndices()
			switch len(targets) {
			case 0:
				g.AddCombatMessage(uitext.Text("combat.no_conscious_party_member_needs_healing"))
				return false
			case 1:
				return g.applyHealTo(itemIndex, targets[0])
			default:
				g.healPickerOpen = true
				g.healPickerItemIdx = itemIndex
				return false
			}
		}
		if !g.healingPotionHasEffect(ch) {
			// Nothing to heal: keep the potion instead of wasting it.
			g.AddCombatMessage(uitext.Text("combat.is_already_at_full_health", ch.Name))
			return false
		}
		return g.applyHealTo(itemIndex, selectedChar)
	}

	// Timed consumable buffs keep their static mechanics in items.yaml. The
	// item key is also the replace-on-recast and save identity.
	if def, itemKey, ok := config.ItemDefOf(item); ok && def.HasTimedBuff() {
		buff, valid := timedCombatBuffFromItem(itemKey, def, def.BuffDurationSeconds*g.config.GetTPS())
		if !valid {
			g.AddCombatMessage(uitext.Text("combat.is_misconfigured_buff_attributes", item.Name))
			return false
		}
		g.addCombatBuff(buff)
		bag.Consume(itemIndex, 1)
		g.AddCombatMessage(uitext.Text("combat.party_uses_timed_buff", item.Name, strings.Join(def.ItemMechanicLines(), "; ")))
		return true
	}

	// Mana consumable (mana potion): restores mana_base + Personality/divisor SP.
	// Mirrors the heal branch: refused at full SP so the potion isn't wasted.
	if base, okBase := item.Attributes["mana_base"]; okBase {
		div, okDiv := item.Attributes["mana_personality_divisor"]
		if !okDiv || base <= 0 || div <= 0 {
			g.AddCombatMessage(uitext.Text("combat.is_misconfigured_mana_attributes", item.Name))
			return false
		}
		ch := g.party.Members[selectedChar]
		if ch.HasCondition(character.ConditionUnconscious) || ch.HasCondition(character.ConditionDead) || ch.HasCondition(character.ConditionEradicated) {
			g.AddCombatMessage(uitext.Text("combat.can_t_drink_right_now", ch.Name))
			return false
		}
		if ch.SpellPoints >= ch.MaxSpellPoints {
			g.AddCombatMessage(uitext.Text("combat.is_already_brimming_with_mana", ch.Name))
			return false
		}
		restore := character.ConsumableRestore(ch, base, div, true, g.party.PotionSupport(ch, base, div, true))
		before := ch.SpellPoints
		ch.SpellPoints += restore
		if ch.SpellPoints > ch.MaxSpellPoints {
			ch.SpellPoints = ch.MaxSpellPoints
		}
		bag.Consume(itemIndex, 1)
		g.logCombatLine(logToneGood, uitext.Text("combat.drinks_and_recovers_sp", logSlot, logSlot, logSlot), logHeroName(ch), logItemName(item), logRecoveredSP(ch.SpellPoints-before))
		return true
	}

	// Summoning consumable
	if dist, ok := item.Attributes["summon_distance_tiles"]; ok {
		if dist > 0 {
			if g.SummonRandomMonsterNearPlayer(float64(dist)) {
				bag.Consume(itemIndex, 1)
				g.AddCombatMessage(uitext.Text("item.device_summon_success"))
				return true
			}
			g.AddCombatMessage(uitext.Text("item.device_summon_failed"))
			return false
		}
		g.AddCombatMessage(uitext.Text("combat.is_misconfigured_summon_distance", item.Name))
		return false
	}

	// Unknown consumable behavior
	g.AddCombatMessage(uitext.Text("dialog.nothing_happens"))
	return false
}
