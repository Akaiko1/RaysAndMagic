package monster

import (
	"strings"
	"testing"
)

// Boss effect flags travel in pairs - a half-specified pair must fail at load
// time, not silently zero out in code.
func TestValidateMonsterConfiguration_TeleportPairs(t *testing.T) {
	cases := []struct {
		name    string
		def     MonsterDefinition
		wantErr string
	}{
		{
			name:    "chance_without_threshold",
			def:     MonsterDefinition{Name: "X", TeleportChance: 0.1},
			wantErr: "teleport_chance but no teleport_at_hp",
		},
		{
			name:    "threshold_without_chance",
			def:     MonsterDefinition{Name: "X", TeleportAtHP: 300},
			wantErr: "teleport_at_hp but no teleport_chance",
		},
		{
			name: "complete_pair_ok",
			def:  MonsterDefinition{Name: "X", TeleportAtHP: 300, TeleportChance: 0.1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.def.SizeClass == "" {
				tc.def.SizeClass = "person" // these cases exercise teleport pairs, not size
			}
			cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{"test_boss": tc.def}}
			err := validateMonsterConfiguration(cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid pair must pass: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
