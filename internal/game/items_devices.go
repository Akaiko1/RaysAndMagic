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
	if !g.canSpendCombatAction(charIndex) {
		g.AddCombatMessage(uitext.Text("item.device_cannot_act"))
		return false
	}
	if item.DeviceCooldownFrames > 0 {
		g.AddCombatMessage(uitext.Text("item.device_recharging", item.Name))
		return false
	}
	jump, ok := deviceJump(*item)
	if !ok {
		g.AddCombatMessage(uitext.Text("item.device_unavailable", item.Name))
		return false
	}
	hero := g.party.Members[charIndex]
	cooldown := g.combat.SpellCooldownFrames(hero, jump.ID)
	return g.combat.tryPartyJump(item.Name, jump.JumpTiles+float64(item.Attributes["use_jump_extra_tiles"]), func() {
		item.DeviceCooldownFrames = item.Attributes["use_cooldown_seconds"] * g.config.GetTPS()
		g.combat.deviceClock.remaining = max(g.combat.deviceClock.remaining, item.DeviceCooldownFrames)
		// Jump itself ends the entire TB movement turn. Do not spend a preliminary
		// attack slot: that would incorrectly grant the monsters an extra action.
		hero.RTCooldown = max(hero.RTCooldown, cooldown)
	}) == castCommitted
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
	advance := func(it *items.Item) {
		if it != nil && it.DeviceCooldownFrames > 0 {
			it.DeviceCooldownFrames = max(0, it.DeviceCooldownFrames-frames)
			clock.remaining = max(clock.remaining, it.DeviceCooldownFrames)
		}
	}
	bag := func(stock []items.Item) {
		for i := range stock {
			advance(&stock[i])
		}
	}
	bag(g.party.Inventory)
	for _, roster := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive} {
		for _, hero := range roster {
			if hero == nil {
				continue
			}
			bag(hero.Inventory)
			for _, it := range hero.QuickSlots {
				advance(it)
			}
		}
	}
	for i := range g.groundContainers {
		bag(g.groundContainers[i].Items)
	}
}

func deviceCooldownProgress(item items.Item) float64 {
	total := item.Attributes["use_cooldown_seconds"] * config.GetTargetTPS()
	if item.Type != items.ItemDevice || total <= 0 || item.DeviceCooldownFrames <= 0 {
		return 1
	}
	return max(0, min(1, 1-float64(item.DeviceCooldownFrames)/float64(total)))
}

func deviceCooldownTurns(item items.Item) int {
	round := config.GetTargetTPS() * TurnBasedPeriodicEffectSeconds
	return (max(0, item.DeviceCooldownFrames) + round - 1) / round
}
