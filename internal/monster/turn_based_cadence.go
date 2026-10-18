package monster

import (
	"ugataima/internal/config"
	"ugataima/internal/status"
)

// Turn-based cadence: swings per attack action and steps per movement action.
//
// A monster authors one attack speed (attack_cooldown_multiplier) and one
// movement speed (speed). Real time uses them directly; turn-based combat
// converts each into a count per action with the same rule:
//
//	swings = 1 / attack_cooldown_multiplier      (0 or unset = 1)
//	       x 1 / abilities.enrage.cooldown_mult  (while enraged)
//	steps  = speed / monster_ai.turn_step_speed  (speed 0 = never moves)
//
// The rate is fractional and accumulates: each action takes round(carry +
// rate) and keeps the remainder for the next one, so a 1.43 rate gives
// 1, 2, 1, 2, 1, ... and averages exactly 1.43. Every attack swings at least
// once and every walking monster steps at least once per action; caps keep a
// single action bounded.
//
// Two documented differences, both explicit:
//   - Champions are hero builds and swing ChampionTurnBasedSwings times.
//   - Ambient monsters (wildlife, caravans) stroll rather than fight: their
//     steps have no floor, so one slower than the step speed idles some turns.

// MaxTurnBasedSwings caps one attack action's swings.
const MaxTurnBasedSwings = 8

// MaxTurnBasedSteps caps one movement action's steps.
const MaxTurnBasedSteps = 4

// ChampionTurnBasedSwings is the champion rule. A champion is a hero build:
// its real-time speed comes from its weapons, but its turn-based swings are
// fixed at 2 per action - the duel balance the arena tiers are tuned for -
// regardless of attack_cooldown_multiplier or enrage.
const ChampionTurnBasedSwings = 2

// TurnBasedAttackRate is the average swings per turn-based attack action for
// the given cadence. Game and editor both read it, so they never disagree.
func TurnBasedAttackRate(champion bool, cooldownMult, enrageCooldownMult float64, enraged bool) float64 {
	if champion {
		return ChampionTurnBasedSwings
	}
	rate := 1.0
	if cooldownMult > 0 {
		rate /= cooldownMult
	}
	if enraged && enrageCooldownMult > 0 {
		rate /= enrageCooldownMult
	}
	return min(MaxTurnBasedSwings, max(1, rate))
}

// TurnBasedAttackRate is this monster's live rate, enrage included.
func (m *Monster3D) TurnBasedAttackRate() float64 {
	return TurnBasedAttackRate(m.IsChampion(), m.AttackCooldownMultiplier, m.Abilities.Enrage.CooldownMult, m.IsEnraged())
}

// TakeTurnBasedSwings spends one attack action: it returns the swings this
// action delivers and banks the remainder for the next one.
func (m *Monster3D) TakeTurnBasedSwings() int {
	return nextTurnBasedCount(&m.TurnBasedSwingCarry, m.TurnBasedAttackRate())
}

// PeekTurnBasedSwings previews the next attack action's swings without
// spending them.
func (m *Monster3D) PeekTurnBasedSwings() int {
	carry := m.TurnBasedSwingCarry
	return nextTurnBasedCount(&carry, m.TurnBasedAttackRate())
}

// LoseTurnBasedAttackTime takes seconds of attacking out of the swing bank,
// at the same rate real time loses them (a delayed attack cooldown): seconds
// of a round's swings. A full round's loss leaves an action with no swing.
func (m *Monster3D) LoseTurnBasedAttackTime(seconds int) {
	m.TurnBasedSwingCarry -= m.TurnBasedAttackRate() * float64(seconds) / float64(config.CombatRoundSeconds)
}

// TurnBasedStepRate is the average steps per turn-based movement action for a
// speed. stepSpeed is config monster_ai.turn_step_speed. Ambient monsters have
// no one-step floor (see the package comment above).
func TurnBasedStepRate(speed, stepSpeed float64, ambient bool) float64 {
	if speed <= 0 || stepSpeed <= 0 {
		return 0
	}
	floor := 1.0
	if ambient {
		floor = 0
	}
	return min(MaxTurnBasedSteps, max(floor, speed/stepSpeed))
}

// TurnBasedStepRate is this monster's rate from its authored speed. Slow is
// not applied here: in turn-based combat it costs whole turns instead.
func (m *Monster3D) TurnBasedStepRate() float64 {
	return TurnBasedStepRate(m.Speed, m.turnStepSpeed(), m.IsAmbient())
}

// turnStepSpeed is config monster_ai.turn_step_speed: the speed worth one tile
// per action, and so the real time one turn-based turn stands for.
func (m *Monster3D) turnStepSpeed() float64 {
	cfg := m.config
	if cfg == nil {
		cfg = config.GlobalConfig
	}
	return cfg.GetTurnStepSpeed()
}

// PeekTurnBasedSteps previews the steps of the next actions without spending
// them (the battlefield overlay's movement reach).
func (m *Monster3D) PeekTurnBasedSteps(actions int) int {
	carry, rate, total := m.TurnBasedStepCarry, m.TurnBasedStepRate(), 0
	for i := 0; i < actions; i++ {
		total += nextTurnBasedCount(&carry, rate)
	}
	return total
}

// WalkTurn spends one turn-based movement action through step, which makes a
// single one-tile move and reports whether the monster moved. The action's
// step count is drawn when the first step lands, so an action blocked at its
// first step keeps the banked fraction; an ambient monster with no step this
// turn still banks its fraction and idles. between runs before every extra
// step (hazards the crossed tile triggers) and stops the walk by returning
// false; nil means nothing to check.
func (m *Monster3D) WalkTurn(step func(first bool) bool, between func() bool) {
	steps := m.PeekTurnBasedSteps(1)
	if steps == 0 {
		m.TakeTurnBasedSteps()
		return
	}
	for i := 0; i < steps; i++ {
		if i > 0 && between != nil && !between() {
			return
		}
		if !step(i == 0) {
			return
		}
		if i == 0 {
			m.TakeTurnBasedSteps()
		}
	}
}

// TakeTurnBasedSteps spends one movement action's steps and banks the rest.
func (m *Monster3D) TakeTurnBasedSteps() int {
	return nextTurnBasedCount(&m.TurnBasedStepCarry, m.TurnBasedStepRate())
}

// TurnBasedSequence lists the counts of the first n actions from a fresh carry,
// exactly as combat deals them. The editor shows it beside each rate.
func TurnBasedSequence(rate float64, n int) []int {
	carry := 0.0
	counts := make([]int, n)
	for i := range counts {
		counts[i] = nextTurnBasedCount(&carry, rate)
	}
	return counts
}

// nextTurnBasedCount is the accumulator step for swings and steps: the shared
// accumulator rounded to the nearest whole count.
func nextTurnBasedCount(carry *float64, rate float64) int {
	return status.AccrueNearest(carry, rate)
}
