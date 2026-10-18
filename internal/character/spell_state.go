package character

import (
	"maps"

	"ugataima/internal/status"
)

// PersonalSpellEffect retains the cast's mastery and remaining budget. Power
// comes from the current catalog, not a second saved copy of authored values.
type PersonalSpellEffect struct {
	SpellID string           `json:"spell_id"`
	Tier    int              `json:"tier"`
	Charges int              `json:"charges,omitempty"`
	Clock   status.DualClock `json:"clock,omitzero"` // idle: lasts until its charges are spent
	// LegacyFrames is an older save's remaining time, moved into Clock on load.
	LegacyFrames int `json:"frames,omitempty"`
}

// SpellBeacon stores map-local coordinates so a saved mark survives switching
// between stitched and separate worlds. Generation rejects reset dungeons.
type SpellBeacon struct {
	MapKey      string `json:"map_key"`
	X, Y, Angle float64
	Generation  int `json:"generation"`
}

// SpellState belongs to the hero, including while benched or transferred.
type SpellState struct {
	Effects   []PersonalSpellEffect       `json:"effects,omitempty"`
	Automatic map[string]bool             `json:"automatic,omitempty"`
	Reuse     map[string]status.DualClock `json:"reuse,omitempty"`
	// LegacyReuseFrames is an older save's reuse lockouts, moved into Reuse.
	LegacyReuseFrames map[string]int `json:"reuse_frames,omitempty"`
	// LegacyAutoFrames is the old home of the autocast budget, migrated to
	// MMCharacter.AutocastCD on load.
	LegacyAutoFrames int           `json:"auto_frames,omitempty"`
	Beacons          []SpellBeacon `json:"beacons,omitempty"`
}

func (s SpellState) Clone() SpellState {
	s.Effects = append([]PersonalSpellEffect(nil), s.Effects...)
	s.Beacons = append([]SpellBeacon(nil), s.Beacons...)
	s.Automatic = maps.Clone(s.Automatic)
	s.Reuse = maps.Clone(s.Reuse)
	s.LegacyReuseFrames = maps.Clone(s.LegacyReuseFrames)
	return s
}

// Reusable reports whether the spell's personal reuse lockout has run out.
func (s SpellState) Reusable(id string) bool {
	return !s.Reuse[id].Active()
}

// ArmReuse starts a spell's personal reuse lockout.
func (s *SpellState) ArmReuse(id string, seconds, tps int) {
	if s.Reuse == nil {
		s.Reuse = make(map[string]status.DualClock)
	}
	var c status.DualClock
	c.Arm(seconds, tps)
	s.Reuse[id] = c
}

// Tick spends one tick of every reuse lockout and timed effect.
func (s *SpellState) Tick(turn bool) {
	status.TickMap(s.Reuse, turn, status.Self)
	n := 0
	for _, effect := range s.Effects {
		if effect.Clock.Active() && effect.Clock.Tick(turn) {
			continue
		}
		s.Effects[n] = effect
		n++
	}
	s.Effects = s.Effects[:n]
}

// MigrateLegacyClocks moves an older save's frame counts onto the clocks.
func (s *SpellState) MigrateLegacyClocks(tps int) {
	for id, frames := range s.LegacyReuseFrames {
		if frames > 0 {
			if s.Reuse == nil {
				s.Reuse = make(map[string]status.DualClock)
			}
			s.Reuse[id] = status.FromFrames(frames, tps)
		}
	}
	s.LegacyReuseFrames = nil
	for i := range s.Effects {
		status.MigrateFrames(&s.Effects[i].Clock, &s.Effects[i].LegacyFrames, tps)
	}
}

func (s *SpellState) Set(effect PersonalSpellEffect) {
	for i := range s.Effects {
		if s.Effects[i].SpellID == effect.SpellID {
			s.Effects[i] = effect
			return
		}
	}
	s.Effects = append(s.Effects, effect)
}
