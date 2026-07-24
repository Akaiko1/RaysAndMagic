package status

// Shared timed-status mechanics for monsters AND party members. The two sides
// keep their own fields and effects (damage amounts, condition flags, action
// gating); what lives here is the clockwork they all share:
//
//   - dual-clock statuses (stun): an RT frame counter and a TB turn counter,
//     only the current mode's clock ticks, and whichever expires first ends the
//     status and clears the other - a mode switch can never make it permanent;
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

// TickFrame advances a dual-clock status by one RT frame. When this tick
// expires it, the counterpart TB clock is cleared too (the status is OVER, not
// waiting for a mode switch to resume). Returns true exactly on the expiring
// tick.
func TickFrame(frames, turns *int) (expired bool) {
	if *frames <= 0 {
		return false
	}
	*frames--
	if *frames > 0 {
		return false
	}
	*frames = 0
	*turns = 0
	return true
}

// TickTurn advances a dual-clock status by one TB turn, mirroring TickFrame.
func TickTurn(turns, frames *int) (expired bool) {
	if *turns <= 0 {
		return false
	}
	*turns--
	if *turns > 0 {
		return false
	}
	*turns = 0
	*frames = 0
	return true
}

// Plain dual-clock ticks freeze the INACTIVE clock at its full value, so
// spending 2 of 3 turns in TB and switching to RT handed a stun its whole RT
// duration back (and vice versa) - a mode-flip farm. The rated variants keep
// the clocks proportionally synced through `rate` (RT frames per TB turn):
// after every tick the inactive clock is clamped to the active one's
// equivalent, so a mode switch can never revive spent time. Callers keep rate
// in persisted state. Legacy saves have a zero rate, so DualRate recovers the
// best available approximation from their remaining pair on the next tick.

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

// TickDoTFrame advances a DoT by one RT frame: the duration counts down every
// frame, and the cadence timer fires a damage tick once per `tps` frames.
// dealTick is true on the frames the DoT's damage lands; expired is true
// exactly on the tick the DoT runs out (the cadence timer is reset then).
func TickDoTFrame(remaining, tickTimer *int, tps int) (dealTick, expired bool) {
	if *remaining <= 0 {
		return false, false
	}
	if tps <= 0 {
		tps = 60
	}
	*remaining--
	*tickTimer++
	if *tickTimer >= tps {
		*tickTimer = 0
		dealTick = true
	}
	if *remaining <= 0 {
		*remaining = 0
		*tickTimer = 0
		expired = true
	}
	return dealTick, expired
}

// TickDoTTurn advances a DoT by one TB turn (framesPerTurn of duration) and
// always lands one damage tick while active - the per-turn analogue of
// TickDoTFrame's once-per-second cadence. Returns dealTick=true when the DoT
// was active this turn, expired=true when this turn finished it.
func TickDoTTurn(remaining, tickTimer *int, framesPerTurn int) (dealTick, expired bool) {
	if *remaining <= 0 {
		return false, false
	}
	if framesPerTurn <= 0 {
		framesPerTurn = 60
	}
	*remaining -= framesPerTurn
	if *remaining <= 0 {
		*remaining = 0
		*tickTimer = 0
		expired = true
	}
	return true, expired
}

// Clear ends a DoT outright (cure): both the duration and the cadence timer.
// RemoveCondition/flag cleanup stays with the caller.
func Clear(remaining, tickTimer *int) {
	*remaining = 0
	*tickTimer = 0
}
