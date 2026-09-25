package game

import (
	"fmt"

	"ugataima/internal/config"
)

// Validate against the actual runtime registry once the game is initialized.
// A YAML capability without a timer must fail at boot, never appear to cast.
func (g *MMGame) validateTraversalBuffs() error {
	if config.GlobalSpells == nil {
		return nil
	}
	for id, def := range config.GlobalSpells.Spells {
		if !def.TerrainPassage {
			continue
		}
		timerID := id
		if def.Fly {
			timerID = "fly"
		}
		if !g.isTimedBuffID(timerID) {
			return fmt.Errorf("spell %q: terrain_passage requires timed-buff registry entry %q", id, timerID)
		}
		if def.Fly {
			canonical, ok := config.GetSpellDefinition("fly")
			if !ok || !canonical.Fly || !canonical.TerrainPassage {
				return fmt.Errorf("spell %q: fly requires the canonical fly terrain-passage buff", id)
			}
		}
	}
	return nil
}

// partyHasTerrainPassage derives capability from active buff providers, never
// from a particular spell ID or a world's potentially stale movement flags.
// Another registered timed buff can grant the same capability through YAML.
func (g *MMGame) partyHasTerrainPassage() bool {
	for _, buff := range g.timedBuffs() {
		if !*buff.active {
			continue
		}
		if def, ok := config.GetSpellDefinition(string(buff.id)); ok && def.TerrainPassage {
			return true
		}
	}
	return false
}

// updateTimedBuffs settles the party only when the last terrain-passage
// provider expires. Expiry order cannot revoke another active provider.
func (g *MMGame) updateTimedBuffs() {
	hadPassage := g.partyHasTerrainPassage()
	for _, buff := range g.timedBuffs() {
		tickBuff(buff.active, buff.duration, buff.onExpire)
		g.updateUtilityStatus(buff.id, *buff.duration, *buff.active)
	}
	if hadPassage && !g.partyHasTerrainPassage() {
		g.settleAfterTerrainPassage()
	}
}
