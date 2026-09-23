package config

import "testing"

func TestFloorTransitionValidation(t *testing.T) {
	for _, tc := range []struct {
		profile  FloorTransition
		textures bool
		valid    bool
	}{
		{FloorTransitionHard, true, true}, {FloorTransitionNatural, true, true}, {FloorTransitionWater, true, true},
		{FloorTransitionCliffEast, true, true}, {FloorTransitionCliffWest, true, true}, {FloorTransitionVoid, true, true},
		{"natual", true, false}, {FloorTransitionNatural, false, false},
	} {
		t.Run(string(tc.profile), func(t *testing.T) {
			b := BiomeConfig{FloorTransitions: map[string]FloorTransition{"ground": tc.profile}, FloorTextureGroups: map[string][]string{}}
			if tc.textures {
				b.FloorTextureGroups["ground"] = []string{"ground_0"}
			}
			if err := b.ValidateFloorTransitions(); (err == nil) != tc.valid {
				t.Fatalf("validation=%v, valid=%v", err, tc.valid)
			}
		})
	}
}
