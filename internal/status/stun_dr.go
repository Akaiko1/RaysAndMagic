package status

import "ugataima/internal/config"

// Stun diminishing returns: each stun that lands inside the reset window lasts
// the next share of the stun row's repeat_percents (100% -> 50% -> 25% -> 0%,
// immune), so no target, boss or hero, can be stun-locked. The chain length is
// mode-agnostic; the reset window (repeat_reset_seconds, whole rounds) is one
// DualClock, so a TB<->RT switch mid-fight never speeds up the reset.

// StunDRChain points at one target's diminishing-returns state, so monsters and
// heroes share the rule while keeping their own saved fields.
type StunDRChain struct {
	Stacks *int
	Memory *DualClock
}

// Step scales a requested stun of rounds by the chain's current share (rounding
// up), advances the chain toward immunity and restarts the reset window. It
// returns the rounds that land and their RT frames.
func (c StunDRChain) Step(rounds, tps int) (int, int) {
	stun := config.Status("stun")
	shares := stun.RepeatPercents
	pct := shares[min(*c.Stacks, len(shares)-1)]
	if *c.Stacks < len(shares)-1 {
		*c.Stacks++
	}
	*c.Memory = DualClock{}
	c.Memory.Arm(stun.RepeatResetSeconds, tps)
	landed := ceilPct(rounds, pct)
	return landed, config.RoundFrames(landed, tps)
}

// Forget counts one stun-free tick of the running mode toward clearing the
// chain.
func (c StunDRChain) Forget(turn bool) {
	if c.Memory.Tick(turn) {
		*c.Stacks = 0
	}
}

// Reset clears the chain, e.g. after a rest.
func (c StunDRChain) Reset() {
	*c.Stacks, *c.Memory = 0, DualClock{}
}

// StunDRMemory restores a saved reset window at the one round rate.
func StunDRMemory(frames, turns, tps int) DualClock {
	if frames <= 0 && turns <= 0 {
		return DualClock{}
	}
	return DualClock{Frames: frames, Turns: turns, Rate: config.RoundFrames(1, normalTPS(tps))}
}

// ceilPct scales v by pct% (0-100), rounding UP, so a stun short of immunity
// keeps at least one round. Only pct 0 or v<=0 yields 0.
func ceilPct(v, pct int) int {
	if v <= 0 || pct <= 0 {
		return 0
	}
	return (v*pct + 99) / 100
}
