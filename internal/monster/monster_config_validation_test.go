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
			def:     MonsterDefinition{Name: "X", Boss: true, TeleportChance: 0.1},
			wantErr: "teleport_chance but no teleport_at_hp",
		},
		{
			name:    "threshold_without_chance",
			def:     MonsterDefinition{Name: "X", Boss: true, TeleportAtHP: 300},
			wantErr: "teleport_at_hp but no teleport_chance",
		},
		{
			name: "complete_pair_ok",
			def:  MonsterDefinition{Name: "X", Boss: true, TeleportAtHP: 300, TeleportChance: 0.1},
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

func TestValidateMonsterConfiguration_StunRequiresBothModeClocks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		turns   int
		wantErr bool
	}{
		{name: "complete pair", seconds: 2, turns: 1},
		{name: "missing seconds", turns: 1, wantErr: true},
		{name: "missing turns", seconds: 2, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
				"stunner": {
					Name:            "Stunner",
					SizeClass:       "person",
					StunCharChance:  0.5,
					StunCharSeconds: tc.seconds,
					StunCharTurns:   tc.turns,
				},
			}}
			err := validateMonsterConfiguration(cfg)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "stun_char_seconds and stun_char_turns") {
					t.Fatalf("incomplete stun clocks should fail clearly, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("complete stun clocks should pass: %v", err)
			}
		})
	}
}

func TestValidateMonsterConfiguration_ChampionRejectsMeleeDamageType(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"arena_champion": {
			Name:            "Arena Champion",
			SizeClass:       "person",
			Champion:        "arena_champion",
			MeleeDamageType: "fire",
		},
	}}

	err := validateMonsterConfiguration(cfg)
	if err == nil || !strings.Contains(err.Error(), "equipped weapon") {
		t.Fatalf("champion melee_damage_type conflict should fail clearly, got: %v", err)
	}
}
