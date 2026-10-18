package status

// Shared timed-status mechanics for monsters AND party members. The two sides
// keep their own fields and effects (damage amounts, condition flags, action
// gating); what lives here is the clockwork they all share:
//
//   - dual-clock statuses (stun, root, shred, soak, cooldowns): an RT frame
//     counter and a TB turn counter tied together by a frames-per-turn rate.
//     RT spends one frame; TB spends its authored frame share. The turn
//     counter describes the rounded-up remainder; expiry ends the status and clears
//     the rest - a mode switch can neither make it permanent nor refund time;
//   - DoT statuses (poison, burn): a duration plus a once-per-second damage
//     cadence in RT, or the same elapsed span per round in TB;
//   - refresh-never-shortens application, so re-applying a status extends it
//     but a weak source can't cut a strong one short.

// Refresh extends a single status clock to at least `add`, never shortening a
// running status. Reports whether the status is active afterwards.
import (
	"math"

	"ugataima/internal/config"
)

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
// (RT frames per TB turn): turns round up for action gating, while the frame
// budget keeps exact progress, so a mode switch can never revive spent time.
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

// TickTurnRated spends one authored turn from the exact frame remainder.
// Rounding the remainder up to a turn boundary would refund partial RT time.
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
		*frames = max(0, *frames-*rate)
		if *frames == 0 {
			*turns, *rate = 0, 0
			return true
		}
		*turns = min(*turns, (*frames+*rate-1) / *rate)
	}
	return false
}

// DualClock is one rated dual-clock status held as a value: the RT frames, the
// TB turns and their frames-per-turn rate, with the rules above.
type DualClock struct {
	Frames int `json:"frames,omitempty"`
	Turns  int `json:"turns,omitempty"`
	Rate   int `json:"rate,omitempty"`
}

// Refresh extends the clock without ever shortening it (RefreshDualRated).
func (c *DualClock) Refresh(frames, turns int) bool {
	return RefreshDualRated(&c.Frames, &c.Turns, &c.Rate, frames, turns)
}

// Arm starts a cooldown of whole rounds on both mode clocks: seconds of RT,
// the same rounds of TB.
// No time arms nothing.
func (c *DualClock) Arm(seconds, tps int) {
	if seconds <= 0 {
		return
	}
	c.armRounds(seconds*normalTPS(tps), config.CombatRounds(seconds), tps)
}

// ArmFrames arms a span authored in RT frames (fractional seconds): TB rounds
// it up to whole turns, as Arm does.
func (c *DualClock) ArmFrames(frames, tps int) {
	if frames <= 0 {
		return
	}
	round := config.RoundFrames(1, normalTPS(tps))
	c.armRounds(frames, (frames+round-1)/round, tps)
}

// armRounds refreshes the clock at the one RT/TB conversion: a turn spends a
// round's frames whatever span was armed, so a span that is not whole rounds
// keeps the same exchange rate as every other clock, also after a save.
func (c *DualClock) armRounds(frames, turns, tps int) {
	if c.Refresh(frames, turns) {
		c.Rate = config.RoundFrames(1, normalTPS(tps))
	}
}

func normalTPS(tps int) int {
	if tps <= 0 {
		return config.DefaultTPS
	}
	return tps
}

// FromFrames is the clock with frames of RT left, at a round's frames per TB
// turn: how a save that kept only the frames restores it.
func FromFrames(frames, tps int) DualClock {
	var c DualClock
	c.ArmFrames(frames, tps)
	return c
}

// TickFrame spends one RT frame (TickFrameRated).
func (c *DualClock) TickFrame() (expired bool) { return TickFrameRated(&c.Frames, &c.Turns, &c.Rate) }

// TickTurn spends one TB turn (TickTurnRated).
func (c *DualClock) TickTurn() (expired bool) { return TickTurnRated(&c.Turns, &c.Frames, &c.Rate) }

// Tick spends one tick of the running mode's clock: a turn when turn is set,
// else a frame.
func (c *DualClock) Tick(turn bool) (expired bool) {
	if turn {
		return c.TickTurn()
	}
	return c.TickFrame()
}

// Every runs a repeating period of whole rounds: an idle clock arms on this
// tick, and each lapse is reported once. A turn-based tick spends a whole
// period, so a one-round period lapses every turn.
func (c *DualClock) Every(seconds, tps int, turn bool) (lapsed bool) {
	return c.EveryClock(func() (period DualClock) {
		period.Arm(seconds, tps)
		return period
	}, turn)
}

// EveryClock runs a repeating period: an idle clock takes period()'s span on
// this tick, and each lapse is reported once.
func (c *DualClock) EveryClock(period func() DualClock, turn bool) (lapsed bool) {
	if !c.Active() {
		*c = period()
	}
	return c.Tick(turn)
}

// Active reports a running status.
func (c DualClock) Active() bool { return c.Frames > 0 || c.Turns > 0 }

// Saved is the clock a save writes: nil when the status is not running.
func (c DualClock) Saved() *DualClock {
	if !c.Active() {
		return nil
	}
	return &c
}

// Restore reads a saved clock back; nil is a status that was not running.
func (c *DualClock) Restore(saved *DualClock) {
	*c = DualClock{}
	if saved != nil {
		*c = *saved
	}
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
		tps = config.DefaultTPS
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

// RestoreDoTTickTimer validates a persisted sub-second DoT cadence phase.
// Inactive effects cannot retain a phase, and malformed/future saves are
// clamped below the next full tick instead of receiving immediate damage.
func RestoreDoTTickTimer(remaining, savedTimer, tps int) int {
	if remaining <= 0 || savedTimer <= 0 {
		return 0
	}
	if tps <= 0 {
		tps = config.DefaultTPS
	}
	if savedTimer >= tps {
		return tps - 1
	}
	return savedTimer
}

// Clear ends a DoT outright (cure): both the duration and the cadence timer.
// RemoveCondition/flag cleanup stays with the caller.
func Clear(remaining, tickTimer *int) {
	*remaining = 0
	*tickTimer = 0
}

// PairClock is an authored timer pair, RT seconds beside TB rounds, with part
// of it left (1 = all): a turn spends the pair's frames per round.
func PairClock(seconds float64, rounds, tps int, part float64) DualClock {
	full := int(math.Ceil(seconds*float64(normalTPS(tps)) - 1e-9))
	c := DualClock{Frames: int(math.Ceil(part*float64(full) - 1e-9)), Turns: int(math.Ceil(part*float64(rounds) - 1e-9))}
	if !c.Active() {
		return DualClock{}
	}
	c.Rate = DualRate(full, rounds)
	return c
}

// MigrateFrames moves an older save's frame count onto clock, unless the save
// already held the clock, and clears the count.
func MigrateFrames(clock *DualClock, legacyFrames *int, tps int) {
	if *legacyFrames > 0 && !clock.Active() {
		*clock = FromFrames(*legacyFrames, tps)
	}
	*legacyFrames = 0
}

// TickMap spends one tick of the running mode on the clock of every entry of
// m and drops the entries whose clock ran out.
func TickMap[K comparable, V any](m map[K]V, turn bool, clock func(*V) *DualClock) {
	for k, v := range m {
		c := clock(&v)
		if c.Tick(turn); !c.Active() {
			delete(m, k)
			continue
		}
		m[k] = v
	}
}

// Self is the clock of a map that holds bare clocks (TickMap).
func Self(c *DualClock) *DualClock { return c }
