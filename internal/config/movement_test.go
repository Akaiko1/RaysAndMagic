package config

import "testing"

// TestGetRunMultiplier_FallbackAndConfigured: an unset/<=1 run multiplier falls
// back to the default; a configured >1 value is used as-is.
func TestGetRunMultiplier_FallbackAndConfigured(t *testing.T) {
	var c Config
	if got := c.GetRunMultiplier(); got != RunMultiplierDefault {
		t.Errorf("unset run_multiplier = %v, want default %v", got, RunMultiplierDefault)
	}
	c.Movement.RunMultiplier = 1.0 // <=1 is treated as unset (no sprint slower than walk)
	if got := c.GetRunMultiplier(); got != RunMultiplierDefault {
		t.Errorf("run_multiplier=1 = %v, want default %v", got, RunMultiplierDefault)
	}
	c.Movement.RunMultiplier = 3.5
	if got := c.GetRunMultiplier(); got != 3.5 {
		t.Errorf("configured run_multiplier = %v, want 3.5", got)
	}
}
