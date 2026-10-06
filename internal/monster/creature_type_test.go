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
		{"type: FORMLESS", TypeFormless, ""},
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
// monster card always prints what the ability really does.
func TestMonsterAbilitiesRequireTheirNumbers(t *testing.T) {
	previous := MonsterConfig
	t.Cleanup(func() { MonsterConfig = previous })
	for _, tc := range []struct {
		name, yaml, wantErr string
	}{
		{"complete fireburst", "fireburst_chance: 0.2\n    fireburst_damage_min: 6\n    fireburst_damage_max: 9", ""},
		{"fireburst without damage", "fireburst_chance: 0.2", "fireburst_damage_min"},
		{"fireburst max below min", "fireburst_chance: 0.2\n    fireburst_damage_min: 9\n    fireburst_damage_max: 6", "fireburst_damage_min"},
		{"complete piercing shot", "piercing_shot_chance: 0.2\n    piercing_shot_targets: 2", ""},
		{"piercing shot without targets", "piercing_shot_chance: 0.2", "piercing_shot_targets"},
		{"complete ally heal", "ally_heal_chance: 0.2\n    ally_heal_amount: 10\n    ally_heal_radius_tiles: 2", ""},
		{"ally heal without radius", "ally_heal_chance: 0.2\n    ally_heal_amount: 10", "ally_heal_radius_tiles"},
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
