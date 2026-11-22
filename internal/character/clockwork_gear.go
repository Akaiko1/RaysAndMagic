package character

import (
	"ugataima/internal/config"
	"ugataima/internal/status"
)

// ClockworkGearState holds one hero's Clock Tower gear clocks. It is saved with
// the hero, like the reactive state, so a fight resumed from a save keeps them.
type ClockworkGearState struct {
	// Chrono Cape: the rewind's cooldown; the round whose lapse rewinds, and
	// the HP when it began.
	RewindCD    status.DualClock `json:"rewind_cd,omitzero"`
	RewindRound status.DualClock `json:"rewind_round,omitzero"`
	RewindHP    int              `json:"rewind_hp,omitempty"`
	// Chainwork: the round after a heal, then its free follow-up spell.
	Pendulum status.DualClock `json:"pendulum,omitzero"`
	// Cogleather: a fight started and the opener is not spent yet.
	QuickDraw bool `json:"quick_draw,omitempty"`
}

// SetFightOpenerActions sums Cogleather-style fight openers from completed sets.
func (c *MMCharacter) SetFightOpenerActions() int {
	total := 0
	c.forEachCompletedSet(func(set *config.ItemSetConfig) { total += set.FightOpenerActions })
	return total
}

// SetHealFollowUpSpell reports a completed set whose heals swing back as a
// free offensive spell (Chainwork).
func (c *MMCharacter) SetHealFollowUpSpell() bool {
	follow := false
	c.forEachCompletedSet(func(set *config.ItemSetConfig) { follow = follow || set.HealFollowUpSpell })
	return follow
}

// SetStationaryDamageReductionPct sums the hold-your-ground reductions of
// completed sets (Clockplate), capped below a full block.
func (c *MMCharacter) SetStationaryDamageReductionPct() int {
	total := 0
	c.forEachCompletedSet(func(set *config.ItemSetConfig) { total += set.StationaryDamageReductionPct })
	return min(total, config.MaxStationaryDamageReductionPct)
}

// RewindCooldownSeconds is the shortest rewind cooldown among worn items
// (Chrono Cape); 0 when nothing worn rewinds.
func (c *MMCharacter) RewindCooldownSeconds() int {
	best := 0
	c.forEachEquippedItemDefinition(func(def *config.ItemDefinitionConfig) {
		if s := def.RewindCooldownSeconds; s > 0 && (best == 0 || s < best) {
			best = s
		}
	})
	return best
}
