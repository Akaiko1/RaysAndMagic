package items

import "ugataima/internal/mathutil"

// WeaponSignatureState travels with the physical weapon through transfers,
// equipment, stash and saves. A Solar Flare shot holds a transient reservation
// on launch and only its landed hit spends the saved charge; Skirmish and
// Wind-up spend theirs as the shot is fired.
type WeaponSignatureState struct {
	Charges      int        `json:"charges,omitempty"`
	MoveDistance float64    `json:"move_distance,omitempty"` // tiles moved toward the next charge
	Wind         RoundClock `json:"wind,omitzero"`           // Wind-up: the round toward the next spring
}

// RoundClock is the shape of status.DualClock, the one RT/TB clock. items
// cannot import status (status imports config, which imports items), so the
// game converts a pointer to it: (*status.DualClock)(&clock).
type RoundClock struct {
	Frames int `json:"frames,omitempty"`
	Turns  int `json:"turns,omitempty"`
	Rate   int `json:"rate,omitempty"`
}

// ScaleTicks keeps the clock's seconds at a tick rate k times the old one
// (status.TickScaler): frames and rate scale, turns stay.
func (c *RoundClock) ScaleTicks(k float64) {
	c.Frames, c.Rate = mathutil.ScaleCount(c.Frames, k), mathutil.ScaleCount(c.Rate, k)
}
