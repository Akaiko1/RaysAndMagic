package monster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Creature types gate immunities by exact comparison, so the catalog stores
// one canonical spelling however the YAML writes it - including the revenge
// type a boss rallies on death.
func TestMonsterTypesAreCanonicalFromLoad(t *testing.T) {
	previous := MonsterConfig // loading installs the probe catalog globally
	t.Cleanup(func() { MonsterConfig = previous })
	for _, tc := range []struct {
		yaml, wantType, wantRally string
	}{
		{"type: undead", TypeUndead, ""},
		{"type: \" Undead \"", TypeUndead, ""},
		{"type: FORMLESS\n    boss: true", TypeFormless, ""},
		{"type: Dragon\n    boss: true\n    death_rallies_type: \" Human\"", TypeDragon, "human"},
		{"type: \"\"", "", ""},
	} {
		t.Run(tc.yaml, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "monsters.yaml")
			data := "monsters:\n  probe:\n    name: Probe\n    size_class: person\n    " + tc.yaml + "\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadMonsterConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Monsters["probe"]; got.Type != tc.wantType || got.DeathRalliesType != tc.wantRally {
				t.Fatalf("type %q rally %q, want %q and %q", got.Type, got.DeathRalliesType, tc.wantType, tc.wantRally)
			}
		})
	}
}

// An ability names its own numbers; combat has no hidden fallbacks, so the
// monster card always prints what the ability really does. The loader is
// strict: an unknown ability kind or field, or a key from before abilities had
// their own map, fails the load instead of switching the mechanic off.
func TestMonsterAbilitiesRequireTheirNumbers(t *testing.T) {
	previous := MonsterConfig
	t.Cleanup(func() { MonsterConfig = previous })
	for _, tc := range []struct {
		name, yaml, wantErr string
	}{
		{"complete fireburst", "abilities:\n      fireburst: {chance: 0.2, damage_min: 6, damage_max: 9}", ""},
		{"fireburst without damage", "abilities:\n      fireburst: {chance: 0.2}", "fireburst needs damage_min"},
		{"fireburst max below min", "abilities:\n      fireburst: {chance: 0.2, damage_min: 9, damage_max: 6}", "fireburst needs damage_min"},
		{"complete piercing shot", "abilities:\n      piercing_shot: {chance: 0.2, targets: 2}", ""},
		{"piercing shot without targets", "abilities:\n      piercing_shot: {chance: 0.2}", "piercing_shot needs targets"},
		{"complete ally heal", "abilities:\n      ally_heal: {chance: 0.2, amount: 10, radius_tiles: 2}", ""},
		{"ally heal without radius", "abilities:\n      ally_heal: {chance: 0.2, amount: 10}", "ally_heal needs amount and radius_tiles"},
		{"ability without its chance", "abilities:\n      poison: {duration_seconds: 4}", "poison needs a chance"},
		{"whole-round stun", "abilities:\n      stun: {chance: 0.5, seconds: 6}", ""},
		{"stun without seconds", "abilities:\n      stun: {chance: 0.5}", "stun seconds"},
		{"stun for part of a round", "abilities:\n      stun: {chance: 0.5, seconds: 4}", "whole number of 3s rounds"},
		{"one-shot summon", "boss: true\n    abilities:\n      summon: {chance: 0, first_guaranteed: true, monsters: [probe]}", ""},
		{"summon that can never fire", "boss: true\n    abilities:\n      summon: {chance: 0, monsters: [probe]}", "summon needs a chance"},
		{"unknown ability kind", "abilities:\n      posion: {chance: 0.2, duration_seconds: 4}", "posion"},
		{"unknown ability field", "abilities:\n      poison: {chanse: 0.2, duration_seconds: 4}", "chanse"},
		{"key from before the abilities map", "poison_chance: 0.2", "poison_chance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "monsters.yaml")
			data := "monsters:\n  probe:\n    name: Probe\n    size_class: person\n    " + tc.yaml + "\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadMonsterConfig(path)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("complete ability rejected: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("missing %s passed validation: %v", tc.wantErr, err)
			}
		})
	}
}
