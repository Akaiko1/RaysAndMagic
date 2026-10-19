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
		{"complete party attack", partyAttackYAML(""), ""},
		{"party attack without damage", partyAttackYAML("damage_percent: 0"), "needs damage_percent in 1..200"},
		{"party attack past the cap", partyAttackYAML("damage_percent: 201"), "needs damage_percent in 1..200"},
		{"party attack in fixed numbers", partyAttackYAML("damage_min: 6"), "damage_min"},
		{"party attack without riders", strings.Replace(partyAttackYAML(""), ", riders: true", "", 1), "needs riders"},
		{"party attack without targets", partyAttackYAML("targets: 0"), "targets must be all or a positive count"},
		{"party attack without a name", partyAttackYAML(`name: ""`), "needs a name"},
		{"party attack trigger outside the set", partyAttackYAML("trigger: sometimes"), "trigger must be"},
		{"party attack delivery outside the set", partyAttackYAML("delivery: thrown"), "delivery must be"},
		{"party attack in an unknown school", partyAttackYAML("school: flame"), "needs a known school"},
		{"attack-trigger entry with a reach", partyAttackYAML("range_tiles: 3"), "range_tiles is a nova's reach"},
		{"nova on a common monster", partyAttackYAML("trigger: nova; range_tiles: 3"), "a nova needs boss: true"},
		{"boss nova", "boss: true\n    " + partyAttackYAML("trigger: nova; range_tiles: 3"), ""},
		{"the same party attack twice", "abilities:\n      party_attacks:\n        - " + partyAttackEntry("") + "\n        - " + partyAttackEntry(""), "lists \"fireburst\" twice"},
		{"kind from before party_attacks", "abilities:\n      fireburst: {chance: 0.2, damage_min: 6, damage_max: 9}", "fireburst"},
		{"complete ally heal", "abilities:\n      ally_heal: {chance: 0.2, amount: 10, radius_tiles: 2}", ""},
		{"ally heal without radius", "abilities:\n      ally_heal: {chance: 0.2, amount: 10}", "ally_heal needs amount and radius_tiles"},
		{"ability without its chance", "abilities:\n      poison: {duration_seconds: 4}", "poison needs a chance"},
		{"whole-round stun", "abilities:\n      stun: {chance: 0.5, seconds: 6}", ""},
		{"stun without seconds", "abilities:\n      stun: {chance: 0.5}", "stun seconds"},
		{"stun for part of a round", "abilities:\n      stun: {chance: 0.5, seconds: 4}", "whole number of 3s rounds"},
		{"one-shot summon", "boss: true\n    abilities:\n      summon: {chance: 0, first_guaranteed: true, monsters: [probe]}", ""},
		{"summon that can never fire", "boss: true\n    abilities:\n      summon: {chance: 0, monsters: [probe]}", "summon needs a chance"},
		{"timed summon", "boss: true\n    abilities:\n      summon: {cooldown_seconds: 12, monsters: [probe], max: 1}", ""},
		{"timed summon with a roll", "boss: true\n    abilities:\n      summon: {cooldown_seconds: 12, chance: 0.2, monsters: [probe]}", "summon cooldown_seconds"},
		{"timed summon with a first summon", "boss: true\n    abilities:\n      summon: {cooldown_seconds: 12, first_guaranteed: true, monsters: [probe]}", "summon cooldown_seconds"},
		{"summon timer of part of a round", "boss: true\n    abilities:\n      summon: {cooldown_seconds: 4, monsters: [probe]}", "whole number of 3s rounds"},
		{"boss push", "boss: true\n    abilities:\n      push: {cooldown_seconds: 9}", ""},
		{"push cooldown of part of a round", "boss: true\n    abilities:\n      push: {cooldown_seconds: 4}", "whole number of 3s rounds"},
		{"push from before cooldowns", "boss: true\n    abilities:\n      push: {every_actions: 3}", "every_actions"},
		{"push on a common monster", "abilities:\n      push: {cooldown_seconds: 9}", "push is a boss ability"},
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

// partyAttackEntry is a complete fireburst entry; override replaces fields
// ("key: x; targets: 2").
func partyAttackEntry(override string) string {
	fields := []string{"key: fireburst", `name: "Fireburst"`, "trigger: attack", "delivery: spell", "school: fire", "chance: 0.2", "damage_percent: 80", "targets: all", "riders: true"}
	for _, field := range strings.Split(override, "; ") {
		if field == "" {
			continue
		}
		name := strings.SplitN(field, ":", 2)[0]
		replaced := false
		for i := range fields {
			if strings.HasPrefix(fields[i], name+":") {
				fields[i], replaced = field, true
			}
		}
		if !replaced {
			fields = append(fields, field)
		}
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

func partyAttackYAML(override string) string {
	return "abilities:\n      party_attacks:\n        - " + partyAttackEntry(override)
}
