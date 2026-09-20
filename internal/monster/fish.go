package monster

import "math"

// FishLeapState exists only for the brief flight. It is deliberately not saved.
type FishLeapState struct {
	FromX, FromY, ToX, ToY         float64
	Progress, Duration, PeakHeight float64
	Beached                        bool
}

func (m *Monster3D) AdvanceFishLeap(seconds float64) bool {
	s := m.FishLeap
	if s == nil {
		return true
	}
	s.Progress = math.Min(1, s.Progress+math.Max(0, seconds)/s.Duration)
	m.X = s.FromX + (s.ToX-s.FromX)*s.Progress
	m.Y = s.FromY + (s.ToY-s.FromY)*s.Progress
	m.Direction = math.Atan2(s.ToY-s.FromY, s.ToX-s.FromX)
	return s.Progress >= 1
}

func (m *Monster3D) VisualHeightTiles() float64 {
	if s := m.FishLeap; s != nil {
		return 4 * s.Progress * (1 - s.Progress) * s.PeakHeight
	}
	return m.Arbor.Height
}

func (m *Monster3D) SpecialMotionAnimation() (string, float64) {
	if s := m.FishLeap; s != nil {
		return "leaping", s.Progress
	}
	return m.ArborealAnimation()
}
