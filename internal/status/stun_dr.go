package status

import "ugataima/internal/config"

// Stun diminishing returns: each stun that lands inside the reset window lasts
// the next share of the stun row's repeat_percents (100% -> 50% -> 25% -> 0%,
// immune), so no target, boss or hero, can be stun-locked. The chain length is
// mode-agnostic; the reset window (repeat_reset_seconds, whole rounds) is
// tracked per mode so a TB<->RT switch mid-fight never speeds up the reset.

// StunDRChain points at one target's diminishing-returns state, so monsters and
// heroes share the rule while keeping their own saved fields.
type StunDRChain struct {
	Stacks, MemoryTurns, MemoryFrames *int
}

// Step scales a requested stun of rounds by the chain's current share (rounding
// up), advances the chain toward immunity and refreshes both reset windows. It
// returns the rounds that land and their RT frames.
func (c StunDRChain) Step(rounds, tps int) (int, int) {
	stun := config.Status("stun")
	shares := stun.RepeatPercents
	pct := shares[min(*c.Stacks, len(shares)-1)]
	if *c.Stacks < len(shares)-1 {
		*c.Stacks++
	}
	reset := config.CombatRounds(stun.RepeatResetSeconds)
	*c.MemoryTurns = reset
	*c.MemoryFrames = config.RoundFrames(reset, tps)
	landed := ceilPct(rounds, pct)
	return landed, config.RoundFrames(landed, tps)
}

// ForgetFrame counts one stun-free real-time frame toward clearing the chain.
func (c StunDRChain) ForgetFrame() {
	if *c.MemoryFrames > 0 {
		*c.MemoryFrames--
		if *c.MemoryFrames == 0 {
			*c.Stacks, *c.MemoryTurns = 0, 0
		}
	}
}

// ForgetTurn counts one stun-free turn-based turn toward clearing the chain.
func (c StunDRChain) ForgetTurn() {
	if *c.MemoryTurns > 0 {
		*c.MemoryTurns--
		if *c.MemoryTurns == 0 {
			*c.Stacks, *c.MemoryFrames = 0, 0
		}
	}
}

// Reset clears the chain, e.g. after a rest.
func (c StunDRChain) Reset() {
	*c.Stacks, *c.MemoryTurns, *c.MemoryFrames = 0, 0, 0
}

// ceilPct scales v by pct% (0-100), rounding UP, so a stun short of immunity
// keeps at least one round. Only pct 0 or v<=0 yields 0.
func ceilPct(v, pct int) int {
	if v <= 0 || pct <= 0 {
		return 0
	}
	return (v*pct + 99) / 100
}
