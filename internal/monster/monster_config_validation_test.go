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
			def:     MonsterDefinition{Name: "X", Boss: true, Abilities: Abilities{Teleport: TeleportAbility{Chance: 0.1}}},
			wantErr: "ability teleport needs at_hp",
		},
		{
			name:    "threshold_without_chance",
			def:     MonsterDefinition{Name: "X", Boss: true, Abilities: Abilities{Teleport: TeleportAbility{AtHP: 300}}},
			wantErr: "ability teleport needs a chance in (0,1]",
		},
		{
			name: "complete_pair_ok",
			def:  MonsterDefinition{Name: "X", Boss: true, Abilities: Abilities{Teleport: TeleportAbility{AtHP: 300, Chance: 0.1}}},
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

// A stun lasts its seconds in RT and the same span in TB rounds, so the one
// authored duration must be whole 3 s rounds.
func TestValidateMonsterConfiguration_StunDurationIsWholeRounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		wantErr bool
	}{
		{name: "one round", seconds: 3},
		{name: "two rounds", seconds: 6},
		{name: "missing duration", wantErr: true},
		{name: "part of a round", seconds: 4, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
				"stunner": {
					Name:      "Stunner",
					SizeClass: "person",
					Abilities: Abilities{Stun: TimedStatusAbility{Chance: 0.5, Seconds: tc.seconds}},
				},
			}}
			err := validateMonsterConfiguration(cfg)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "stun seconds") || !strings.Contains(err.Error(), "whole number of 3s rounds") {
					t.Fatalf("a stun that is not whole rounds should fail clearly, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("whole-round stun should pass: %v", err)
			}
		})
	}
}

func TestValidateMonsterConfiguration_ChampionRejectsMeleeDamageType(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"arena_champion": {
			Name:                      "Arena Champion",
			SizeClass:                 "person",
			Champion:                  "arena_champion",
			DeprecatedMeleeDamageType: "fire",
		},
	}}

	err := validateMonsterConfiguration(cfg)
	if err == nil || !strings.Contains(err.Error(), "removed melee_damage_type") {
		t.Fatalf("champion melee_damage_type conflict should fail clearly, got: %v", err)
	}
}
