package status

// StunDRFactorsPct scales each stun that lands inside the reset window:
// 100% -> 50% -> 25% -> 0% (immune), so no target, boss or hero, can be
// stun-locked. The chain length is mode-agnostic; the reset window is tracked
// per mode so a TB<->RT switch mid-fight never speeds up the reset.
var StunDRFactorsPct = []int{100, 50, 25, 0}

const (
	StunDRResetTurns   = 4 // TB: stun-free turns that clear the chain
	StunDRResetSeconds = 8 // RT: stun-free seconds that clear the chain
)

// StunDRChain points at one target's diminishing-returns state, so monsters and
// heroes share the rule while keeping their own saved fields.
type StunDRChain struct {
	Stacks, MemoryTurns, MemoryFrames *int
}

// Step scales a requested stun by the chain's current factor (rounding up),
// advances the chain toward immunity and refreshes both reset windows.
func (c StunDRChain) Step(turns, frames, tps int) (int, int) {
	i := min(*c.Stacks, len(StunDRFactorsPct)-1)
	pct := StunDRFactorsPct[i]
	if *c.Stacks < len(StunDRFactorsPct)-1 {
		*c.Stacks++
	}
	*c.MemoryTurns = StunDRResetTurns
	*c.MemoryFrames = StunDRResetSeconds * tps
	return ceilPct(turns, pct), ceilPct(frames, pct)
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

// ceilPct scales v by pct% (0-100), rounding UP. A TB stun is usually one
// turn; floor division sent it to 0 below 100% while its RT twin stayed
// nonzero, leaving the stun overlay stuck. Only pct 0 or v<=0 yields 0.
func ceilPct(v, pct int) int {
	if v <= 0 || pct <= 0 {
		return 0
	}
	return (v*pct + 99) / 100
}
