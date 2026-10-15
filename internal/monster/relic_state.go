package monster

import "maps"

type CatalysisMark struct {
	Percent int `json:"percent"`
	Frames  int `json:"frames"`
}

// RelicState belongs to this monster, including across map re-entry and saves.
// Actor IDs distinguish the hero who prepared Catalysis from the flask owner.
type RelicState struct {
	BurnKilled    bool                     `json:"-"`                         // its own burn tick killed it, even as the burn ran out
	CrumbleArmor  int                      `json:"crumble_armor,omitempty"`   // armor eroded so far
	CrumbleCapPct int                      `json:"crumble_cap_pct,omitempty"` // erosion never passes this share of the current armor
	Catalysis     map[uint64]CatalysisMark `json:"catalysis,omitempty"`
}

func (s RelicState) Clone() RelicState {
	s.Catalysis = maps.Clone(s.Catalysis)
	return s
}

func (s *RelicState) Tick(frames int) {
	if frames <= 0 {
		return
	}
	for id, mark := range s.Catalysis {
		mark.Frames -= frames
		if mark.Frames <= 0 {
			delete(s.Catalysis, id)
		} else {
			s.Catalysis[id] = mark
		}
	}
}

// Erosion is the armor Crumbling removes from a monster whose armor is now
// armor: the eroded amount, at most CrumbleCapPct of that current armor, so
// switches that cut the armor before or after the hits give the same result.
func (s RelicState) Erosion(armor int) int {
	return max(0, min(s.CrumbleArmor, max(0, armor)*s.CrumbleCapPct/100))
}

// Normalized repairs saved relic state: erosion is never negative and its cap
// never passes the current authored one, capPercent (0: no crumbling card in
// the catalog, keep it). Removing a card from active slots still leaves
// existing erosion.
func (s RelicState) Normalized(capPercent int) RelicState {
	s = s.Clone()
	s.CrumbleArmor = max(0, s.CrumbleArmor)
	s.CrumbleCapPct = max(0, s.CrumbleCapPct)
	if capPercent > 0 {
		s.CrumbleCapPct = min(s.CrumbleCapPct, capPercent)
	}
	for id, mark := range s.Catalysis {
		if id == 0 || mark.Frames <= 0 || mark.Percent <= 0 || mark.Percent > 100 {
			delete(s.Catalysis, id)
		}
	}
	return s
}
