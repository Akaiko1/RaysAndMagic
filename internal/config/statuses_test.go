package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The status table is the one source of every status's names and numbers, so
// a broken row fails the load and is never published. Each case edits one row
// of the shipped table; the shipped table itself is the positive control.
func TestStatusTableRejectsBrokenRows(t *testing.T) {
	original, err := os.ReadFile("../../assets/statuses.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, want string
		edit       func(rows map[string]map[string]any)
	}{
		{"shipped table", "", func(map[string]map[string]any) {}},
		{"missing row", `"poison" is missing`, func(r map[string]map[string]any) { delete(r, "poison") }},
		{"unknown row", `unknown status "frozen"`, func(r map[string]map[string]any) { r["frozen"] = map[string]any{"name": "Frozen", "monsters": "x"} }},
		{"missing number", "minimum_damage is required", func(r map[string]map[string]any) { delete(r["burn"], "minimum_damage") }},
		{"stray number", "seconds is not read", func(r map[string]map[string]any) { r["root"]["seconds"] = 3 }},
		{"unknown placeholder", "{nope} has no value", func(r map[string]map[string]any) { r["root"]["party"] = "Pinned for {nope}." }},
		{"side mismatch", "must match the sides", func(r map[string]map[string]any) { delete(r["weaken"], "monsters") }},
		{"short off the party", "short is required", func(r map[string]map[string]any) { delete(r["stun"], "short") }},
		{"melt not whole rounds", "whole number of 3s rounds", func(r map[string]map[string]any) { r["armor_melt"]["seconds"] = 14 }},
		{"repeat reset not whole rounds", "whole number of 3s rounds", func(r map[string]map[string]any) { r["stun"]["repeat_reset_seconds"] = 8 }},
		{"slow skips every turn", "skip_every_turns 1 must be 2..100", func(r map[string]map[string]any) { r["slow"]["skip_every_turns"] = 1 }},
		{"repeats never fail", "run from 100 down to 0", func(r map[string]map[string]any) { r["stun"]["repeat_percents"] = []int{100, 50} }},
		{"repeats grow", "must never grow", func(r map[string]map[string]any) { r["stun"]["repeat_percents"] = []int{100, 200, 25, 0} }},
		{"burn heals", "percent_per_second -3 must be 1..100", func(r map[string]map[string]any) { r["burn"]["percent_per_second"] = -3 }},
		{"boss burn over a whole bar", "boss_percent_per_second 101 must be 1..100", func(r map[string]map[string]any) { r["burn"]["boss_percent_per_second"] = 101 }},
		{"poison floor heals", "minimum_damage -1 must be 1..1000", func(r map[string]map[string]any) { r["poison"]["minimum_damage"] = -1 }},
		{"slow stops the party", "hinder_percent 100 must be 1..99", func(r map[string]map[string]any) { r["slow"]["hinder_percent"] = 100 }},
		{"unknown field", "field speed not found", func(r map[string]map[string]any) { r["slow"]["speed"] = 3 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file struct {
				Statuses map[string]map[string]any `yaml:"statuses"`
			}
			if err := yaml.Unmarshal(original, &file); err != nil {
				t.Fatal(err)
			}
			tc.edit(file.Statuses)
			encoded, err := yaml.Marshal(file)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "statuses.yaml")
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			before := globalStatuses
			t.Cleanup(func() { globalStatuses = before })
			err = LoadStatuses(path)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("shipped table rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			if globalStatuses == nil || Status("poison") != before["poison"] {
				t.Fatal("a broken table was published")
			}
		})
	}
}

// Every status text resolves its placeholders from the row's own numbers, so
// the rendered text states the mechanic's real values.
func TestStatusTextsStateTheirNumbers(t *testing.T) {
	poison, slow, melt := Status("poison"), Status("slow"), Status("armor_melt")
	p := slow.HinderPercent
	for _, tc := range []struct{ got, want string }{
		{poison.PartyText(), fmt.Sprintf("Loses %d%% of max HP per second, at least %d (%d%% per TB turn, at least %d).", poison.PercentPerSecond, poison.MinimumDamage, poison.PercentPerSecond*CombatRoundSeconds, poison.MinimumDamage*CombatRoundSeconds)},
		{slow.MonsterText(), fmt.Sprintf("loses one turn in every %d", slow.SkipEveryTurns)},
		{slow.PartyText(), fmt.Sprintf("Moves %d%% slower and recovers %d%% slower", p, int(math.Round(100*float64(p)/float64(100-p))))},
		{melt.RuleText(), fmt.Sprintf("Lasts %ds / %d TB turns.", melt.Seconds, melt.Seconds/CombatRoundSeconds)},
	} {
		if !strings.Contains(tc.got, tc.want) {
			t.Errorf("%q does not contain %q", tc.got, tc.want)
		}
	}
	for _, key := range StatusKeys() {
		s := Status(key)
		for _, text := range []string{s.PartyText(), s.MonsterText(), s.RuleText()} {
			if strings.ContainsAny(text, "{}") {
				t.Errorf("%s: unresolved placeholder in %q", key, text)
			}
		}
	}
}
