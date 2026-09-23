package config

import "testing"

func TestAutoDrinkValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config AutoDrinkConfig
		bad    bool
	}{
		{"disabled", AutoDrinkConfig{}, false},
		{"enabled", AutoDrinkConfig{35, 1}, false},
		{"zero interval", AutoDrinkConfig{35, 0}, true},
		{"negative interval", AutoDrinkConfig{0, -1}, true},
		{"negative threshold", AutoDrinkConfig{-1, 1}, true},
		{"excess threshold", AutoDrinkConfig{101, 1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.config.Validate() != nil) != tc.bad {
				t.Fatal("unexpected validation result")
			}
		})
	}
}
