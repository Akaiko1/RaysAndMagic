package monster

import (
	"maps"
	"ugataima/internal/status"
)

// ElementalWeaponMark is value-only so snapshots own independent mark state.
// Count is buildup for pressure, stored load or transfer. Old positional marks
// deserialize with count zero and begin the new rule on their next direct hit.
type ElementalWeaponMark struct {
	status.DualClock
	Count int `json:"count,omitempty"`
}

func (m *Monster3D) SetElementalMark(kind string, mark ElementalWeaponMark, frames, turns int) {
	if m.ElementalMarks == nil {
		m.ElementalMarks = make(map[string]ElementalWeaponMark)
	}
	mark.DualClock = status.DualClock{}
	mark.Refresh(frames, turns)
	m.ElementalMarks[kind] = mark
}

func (m *Monster3D) TickElementalMarks(turn bool) {
	status.TickMap(m.ElementalMarks, turn, func(k *ElementalWeaponMark) *status.DualClock { return &k.DualClock })
}

func (m *Monster3D) RestoreElementalMarks(saved map[string]ElementalWeaponMark) {
	m.ElementalMarks = maps.Clone(saved)
	for kind, mark := range m.ElementalMarks {
		if kind == "backwash" || mark.Frames <= 0 || mark.Turns <= 0 || mark.Rate <= 0 {
			delete(m.ElementalMarks, kind)
		}
	}
}
