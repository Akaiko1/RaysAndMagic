package config

import "testing"

func TestCaravanSkippedWaypointValidation(t *testing.T) {
	previous := GlobalEcology
	t.Cleanup(func() { GlobalEcology = previous })
	if err := LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		points []RoutePoint
		valid  bool
	}{
		{"interior", []RoutePoint{{Map: "forest"}, {Map: "forest", Skip: true}, {Map: "forest"}}, true},
		{"first", []RoutePoint{{Map: "forest", Skip: true}, {Map: "forest"}}, false},
		{"last", []RoutePoint{{Map: "forest"}, {Map: "forest", Skip: true}}, false},
		{"entrance", []RoutePoint{{Map: "desert"}, {Map: "forest", Skip: true}, {Map: "forest"}}, false},
		{"exit", []RoutePoint{{Map: "forest"}, {Map: "forest", Skip: true}, {Map: "city"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *GlobalEcology
			cfg.Caravan.Routes = []CaravanRoute{{ID: "test", Points: tc.points}}
			if err := cfg.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestCaravanAlertCooldownValidation(t *testing.T) {
	previous := GlobalEcology
	t.Cleanup(func() { GlobalEcology = previous })
	if err := LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	if GlobalEcology.Caravan.AttackAlertCooldownSeconds != 20 {
		t.Fatal("shipped alert cooldown must be 20 seconds")
	}
	for _, seconds := range []int{-1, 0, 1, 37} {
		cfg := *GlobalEcology
		cfg.Caravan.AttackAlertCooldownSeconds = seconds
		if err := cfg.Validate(); (err == nil) != (seconds > 0) {
			t.Errorf("seconds=%d: %v", seconds, err)
		}
	}
}
