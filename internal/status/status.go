package status

// Shared timed-status mechanics for monsters AND party members. The two sides
// keep their own fields and effects (damage amounts, condition flags, action
// gating); what lives here is the clockwork they all share:
//
//   - dual-clock statuses (stun, root, shred, soak, cooldowns): an RT frame
//     counter and a TB turn counter tied together by a frames-per-turn rate.
//     Only the current mode's clock ticks; the other is clamped to the same
//     remaining time, and whichever expires first ends the status and clears
//     the rest - a mode switch can neither make it permanent nor refund time;
//   - DoT statuses (poison, burn): a duration plus a once-per-second damage
//     cadence in RT, or one damage tick per turn in TB;
//   - refresh-never-shortens application, so re-applying a status extends it
//     but a weak source can't cut a strong one short.

// Refresh extends a single status clock to at least `add`, never shortening a
// running status. Reports whether the status is active afterwards.
func Refresh(remaining *int, add int) bool {
	if add > *remaining {
		*remaining = add
	}
	return *remaining > 0
}

// RefreshDual extends both clocks of a dual-clock status to at least the given
// values. Reports whether the status is active afterwards.
func RefreshDual(frames, turns *int, addFrames, addTurns int) bool {
	Refresh(frames, addFrames)
	Refresh(turns, addTurns)
	return *frames > 0 || *turns > 0
}

// RefreshDualRated extends a rated dual clock without letting a weaker
// reapplication replace the active frames-per-turn contract. When either clock
// is genuinely extended, the resulting pair becomes the new contract.
func RefreshDualRated(frames, turns, rate *int, addFrames, addTurns int) bool {
	extended := addFrames > *frames || addTurns > *turns
	if !RefreshDual(frames, turns, addFrames, addTurns) {
		*rate = 0
		return false
	}
	if *rate <= 0 || extended {
		*rate = DualRate(*frames, *turns)
	}
	return true
}

// A dual-clock tick that only decremented the ACTIVE clock froze the inactive
// one at its full value, so spending 2 of 3 turns in TB and switching to RT
// handed a stun its whole RT duration back (and vice versa) - a mode-flip farm.
// The rated ticks below keep both clocks proportionally synced through `rate`
// (RT frames per TB turn): after every tick the inactive clock is clamped to the
// active one's equivalent, so a mode switch can never revive spent time.
// Callers keep rate in persisted state. Legacy saves have a zero rate, so
// DualRate recovers the best available approximation from their remaining pair
// on the next tick.

// DualRate derives the frames-per-turn exchange rate of an active pair.
func DualRate(frames, turns int) int {
	if turns <= 0 || frames <= 0 {
		return 0
	}
	return (frames + turns - 1) / turns
}

// TickFrameRated advances the RT clock one frame and clamps the TB clock to
// the equivalent remainder. Expiry clears everything, as with TickFrame.
func TickFrameRated(frames, turns, rate *int) (expired bool) {
	if *frames <= 0 {
		return false
	}
	if *rate <= 0 {
		*rate = DualRate(*frames, *turns)
	}
	*frames--
	if *frames <= 0 {
		*frames, *turns, *rate = 0, 0, 0
		return true
	}
	if *rate > 0 {
		if t := (*frames + *rate - 1) / *rate; t < *turns {
			*turns = t
		}
	}
	return false
}

// TickTurnRated mirrors TickFrameRated for one TB turn.
func TickTurnRated(turns, frames, rate *int) (expired bool) {
	if *turns <= 0 {
		return false
	}
	if *rate <= 0 {
		*rate = DualRate(*frames, *turns)
	}
	*turns--
	if *turns <= 0 {
		*turns, *frames, *rate = 0, 0, 0
		return true
	}
	if *rate > 0 {
		if f := *turns * *rate; f < *frames {
			*frames = f
		}
	}
	return false
}

// TickDoT advances a DoT by elapsedFrames of game time and reports how many
// once-per-second damage ticks that time contains. It is the ONE implementation
// for both modes: RT passes 1 (one frame), a TB round passes the seconds that
// round represents - so a round is billed exactly like the same span of real
// time and neither mode deals more total damage than the DoT's duration.
// Elapsed time is capped at the remaining duration, and the cadence timer
// carries the sub-second remainder across calls, so a duration that is not a
// whole multiple of the round length still lands its full tick count.
// expired is true exactly on the call that runs the DoT out.
func TickDoT(remaining, tickTimer *int, elapsedFrames, tps int) (ticks int, expired bool) {
	if *remaining <= 0 || elapsedFrames <= 0 {
		return 0, false
	}
	if tps <= 0 {
		tps = 60
	}
	elapsed := elapsedFrames
	if elapsed > *remaining {
		elapsed = *remaining
	}
	*remaining -= elapsed
	*tickTimer += elapsed
	for *tickTimer >= tps {
		*tickTimer -= tps
		ticks++
	}
	if *remaining <= 0 {
		*remaining = 0
		*tickTimer = 0
		expired = true
	}
	return ticks, expired
}

// Clear ends a DoT outright (cure): both the duration and the cadence timer.
// RemoveCondition/flag cleanup stays with the caller.
func Clear(remaining, tickTimer *int) {
	*remaining = 0
	*tickTimer = 0
}
