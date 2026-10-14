package character

import "maps"

// PersonalSpellEffect retains the cast's mastery and remaining budget. Power
// comes from the current catalog, not a second saved copy of authored values.
type PersonalSpellEffect struct {
	SpellID string `json:"spell_id"`
	Tier    int    `json:"tier"`
	Charges int    `json:"charges,omitempty"`
	Frames  int    `json:"frames,omitempty"`
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
	Effects     []PersonalSpellEffect `json:"effects,omitempty"`
	Automatic   map[string]bool       `json:"automatic,omitempty"`
	ReuseFrames map[string]int        `json:"reuse_frames,omitempty"`
	// LegacyAutoFrames is the old home of the autocast budget, migrated to
	// MMCharacter.AutocastFrames on load.
	LegacyAutoFrames int           `json:"auto_frames,omitempty"`
	Beacons          []SpellBeacon `json:"beacons,omitempty"`
}

func (s SpellState) Clone() SpellState {
	s.Effects = append([]PersonalSpellEffect(nil), s.Effects...)
	s.Beacons = append([]SpellBeacon(nil), s.Beacons...)
	s.Automatic = maps.Clone(s.Automatic)
	s.ReuseFrames = maps.Clone(s.ReuseFrames)
	return s
}

// Reusable reports whether the spell's personal reuse lockout has run out.
func (s SpellState) Reusable(id string) bool {
	return s.ReuseFrames[id] <= 0
}

func (s *SpellState) Tick(frames int) {
	for id, remaining := range s.ReuseFrames {
		if remaining <= frames {
			delete(s.ReuseFrames, id)
		} else {
			s.ReuseFrames[id] = remaining - frames
		}
	}
	n := 0
	for _, effect := range s.Effects {
		if effect.Frames > 0 {
			effect.Frames = max(0, effect.Frames-frames)
			if effect.Frames == 0 {
				continue
			}
		}
		s.Effects[n] = effect
		n++
	}
	s.Effects = s.Effects[:n]
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
