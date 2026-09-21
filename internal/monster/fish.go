package monster

import (
	"fmt"
	"math"
)

// FishLeapState exists only for the brief flight. It is deliberately not saved.
type FishLeapState struct {
	FromX, FromY, ToX, ToY         float64
	Progress, Duration, PeakHeight float64
	Beached                        bool
}

// ValidateFishLeap rejects incomplete transient actors before world registration.
func (m *Monster3D) ValidateFishLeap() error {
	if !m.IsFish() {
		return nil
	}
	s := m.FishLeap
	if s == nil {
		return fmt.Errorf("fish %q requires leap state; use an ecology fish spawn", m.Key)
	}
	for _, v := range []float64{s.FromX, s.FromY, s.ToX, s.ToY, s.Progress, s.Duration, s.PeakHeight} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("fish %q has nonfinite leap state", m.Key)
		}
	}
	if s.Duration <= 0 || s.PeakHeight <= 0 || s.Progress < 0 || s.Progress > 1 {
		return fmt.Errorf("fish %q has invalid leap timing or height", m.Key)
	}
	return nil
}

func (m *Monster3D) AdvanceFishLeap(seconds float64) bool {
	s := m.FishLeap
	if err := m.ValidateFishLeap(); err != nil {
		panic(err)
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
