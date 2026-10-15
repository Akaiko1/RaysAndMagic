package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func (g *MMGame) useDeviceFromInventory(index, charIndex int, owner ...*character.MMCharacter) bool {
	if g.party == nil {
		return false
	}
	bag := g.party.Bag(owner...).Items()
	if index < 0 || index >= len(bag) {
		return false
	}
	return g.useDevice(&bag[index], charIndex)
}

func deviceJump(it items.Item) (spells.SpellDefinition, bool) {
	if it.Type != items.ItemDevice || it.UseAction != "jump" || it.Attributes["use_cooldown_seconds"] <= 0 {
		return spells.SpellDefinition{}, false
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(it.SpellEffect))
	return def, err == nil && def.JumpTiles > 0
}

// Preparation ignores short recovery, as the spell gate does. Any capable
// active hero can use an active party bag, even when its owner cannot act.
func (g *MMGame) canPrepareJumpDevice(spellID string) bool {
	required, err := spells.GetSpellDefinitionByID(spells.SpellID(spellID))
	if err != nil || required.JumpTiles <= 0 || g.party == nil || g.partyRooted() {
		return false
	}
	ready := false
	for _, hero := range g.party.Members {
		if hero != nil && hero.CanUseCombatAction() {
			ready = true
			break
		}
	}
	if !ready {
		return false
	}
	for _, it := range g.party.CarriedItems() {
		if def, ok := deviceJump(it); ok && def.JumpTiles+float64(it.Attributes["use_jump_extra_tiles"]) >= required.JumpTiles {
			return true
		}
	}
	return false
}

func (g *MMGame) useDevice(item *items.Item, charIndex int) bool {
	if item == nil || item.Type != items.ItemDevice || g.party == nil || g.combat == nil {
		return false
	}
	if item.UseAction == "throw_flask" {
		if key, d := flaskDefinition(*item); d != nil {
			return g.equipFlask(charIndex, key)
		}
		return false
	}
	if !g.canSpendCombatAction(charIndex) {
		g.AddCombatMessage(uitext.Text("item.device_cannot_act"))
		return false
	}
	if item.DeviceCooldownFrames > 0 {
		g.AddCombatMessage(uitext.Text("item.device_recharging", item.Name))
		return false
	}
	if item.UseAction == "summon" && item.Attributes["summon_distance_tiles"] > 0 && item.Attributes["use_cooldown_seconds"] > 0 {
		if !g.SummonRandomMonsterNearPlayer(float64(item.Attributes["summon_distance_tiles"])) {
			g.AddCombatMessage(uitext.Text("item.device_summon_failed"))
			return false
		}
		g.armDeviceRecharge(item)
		g.AddCombatMessage(uitext.Text("item.device_summon_success"))
		g.consumeCharacterActionWithRTCooldown(charIndex, g.combat.CalculateActionCooldownFrames(g.party.Members[charIndex]))
		return true
	}
	jump, ok := deviceJump(*item)
	if !ok {
		g.AddCombatMessage(uitext.Text("item.device_unavailable", item.Name))
		return false
	}
	hero := g.party.Members[charIndex]
	cooldown := g.combat.SpellCooldownFrames(hero, jump.ID)
	return g.combat.tryPartyJump(item.Name, jump.JumpTiles+float64(item.Attributes["use_jump_extra_tiles"]), func() {
		g.armDeviceRecharge(item)
		// Jump itself ends the entire TB movement turn. Do not spend a preliminary
		// attack slot: that would incorrectly grant the monsters an extra action.
		hero.RTCooldown = max(hero.RTCooldown, cooldown)
	}) == castCommitted
}

// armDeviceRecharge starts a used device's recharge (use_cooldown_seconds,
// never Speed-scaled) on the one device clock.
func (g *MMGame) armDeviceRecharge(item *items.Item) {
	item.DeviceCooldownFrames = item.Attributes["use_cooldown_seconds"] * g.config.GetTPS()
	g.combat.deviceClock.remaining = max(g.combat.deviceClock.remaining, item.DeviceCooldownFrames)
}

// An idle world needs no inventory walk. A use arms the bounded sweep; carried
// inventory changes, timeline replacement or newly restored containers wake it.
// References to slice elements are never cached across inventory transfers.
type deviceCooldownSweep struct {
	party      *character.Party
	revision   uint64
	rosters    [3]int
	containers int
	remaining  int
}

func (g *MMGame) tickDeviceCooldowns(frames int) {
	if frames <= 0 || g.party == nil || g.combat == nil {
		return
	}
	clock := &g.combat.deviceClock
	rosters := [3]int{len(g.party.Members), len(g.party.Reserve), len(g.party.Captive)}
	if clock.remaining == 0 && clock.party == g.party && clock.revision == g.party.ContentRevision() && clock.rosters == rosters && clock.containers == len(g.groundContainers) {
		return
	}
	clock.party, clock.revision, clock.rosters, clock.containers = g.party, g.party.ContentRevision(), rosters, len(g.groundContainers)
	clock.remaining = 0
	g.eachCarriedItem(func(it *items.Item) bool {
		if it.DeviceCooldownFrames > 0 {
			it.DeviceCooldownFrames = max(0, it.DeviceCooldownFrames-frames)
			clock.remaining = max(clock.remaining, it.DeviceCooldownFrames)
		}
		return false
	})
}

// eachLiveItem visits every item the live game holds outside the stash: each
// roster hero's equipment, then everything carried (eachCarriedItem).
// Changes through the pointer stick; true stops the walk and is returned.
func (g *MMGame) eachLiveItem(visit func(*items.Item) bool) bool {
	if g.party == nil {
		return false
	}
	for hero := range g.party.Heroes() {
		for slot, it := range hero.Equipment {
			stop := visit(&it)
			hero.Equipment[slot] = it
			if stop {
				return true
			}
		}
	}
	return g.eachCarriedItem(visit)
}

// eachCarriedItem visits every live item not worn: the shared bag, each
// roster hero's bag and quick slots, and ground containers. Devices live only
// here - they are never equipped.
func (g *MMGame) eachCarriedItem(visit func(*items.Item) bool) bool {
	bag := func(stock []items.Item) bool {
		for i := range stock {
			if visit(&stock[i]) {
				return true
			}
		}
		return false
	}
	if g.party == nil {
		return false
	}
	if bag(g.party.Inventory) {
		return true
	}
	for hero := range g.party.Heroes() {
		if bag(hero.Inventory) {
			return true
		}
		for _, it := range hero.QuickSlots {
			if it != nil && visit(it) {
				return true
			}
		}
	}
	for i := range g.groundContainers {
		if bag(g.groundContainers[i].Items) {
			return true
		}
	}
	return false
}

func deviceCooldownProgress(item items.Item) float64 {
	total := item.Attributes["use_cooldown_seconds"] * config.GetTargetTPS()
	if item.Type != items.ItemDevice || total <= 0 || item.DeviceCooldownFrames <= 0 {
		return 1
	}
	return max(0, min(1, 1-float64(item.DeviceCooldownFrames)/float64(total)))
}

func deviceCooldownTurns(item items.Item) int {
	round := config.GetTargetTPS() * config.CombatRoundSeconds
	return (max(0, item.DeviceCooldownFrames) + round - 1) / round
}
