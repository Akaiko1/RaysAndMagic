package config

import (
	"slices"
	"testing"
)

// Rules an authored adventure must meet so nothing silently never fires.
func TestValidateAdventuresRules(t *testing.T) {
	lane := EnvironmentEffect{ID: "lane_1", Name: "Lane", Kind: "lane", Rect: [4]int{1, 1, 2, 2}, School: "fire", Damage: 5, WarningSeconds: 1, WarningRounds: 1, DurationSeconds: 3}
	gated := EnvironmentEffect{ID: "vent", Name: "Vent", Kind: "occupation", Rect: [4]int{4, 4, 5, 5}, School: "fire", Damage: 5, BossBelowPercent: 50, WarningSeconds: 1.25, WarningRounds: 1}
	base := func() AdventureConfig {
		return AdventureConfig{OpeningOwned: true, Element: "fire", Boss: &AdventureBoss{Monster: "boss", CooldownSeconds: 6}, Effects: []EnvironmentEffect{lane, gated}}
	}
	for _, tc := range []struct {
		name string
		edit func(a *AdventureConfig)
		ok   bool
	}{
		{"valid scheduled boss map", func(*AdventureConfig) {}, true},
		{"HP-gated effect without real-time warning", func(a *AdventureConfig) { a.Effects[1].WarningSeconds = 0 }, false},
		{"HP-gated effect without turn-based warning", func(a *AdventureConfig) { a.Effects[1].WarningRounds = 0 }, false},
		{"HP-gated effect without a boss", func(a *AdventureConfig) { a.Boss = nil; a.Effects = a.Effects[1:] }, false},
		{"ungated effect without warning", func(a *AdventureConfig) {
			a.Effects[1].BossBelowPercent, a.Effects[1].WarningSeconds, a.Effects[1].WarningRounds = 0, 0, 0
		}, true},
		{"jump links on a scheduled map", func(a *AdventureConfig) { a.JumpLinks = [][4]int{{1, 1, 3, 1}} }, true},
		{"jump links on an unscheduled map", func(a *AdventureConfig) {
			a.OpeningOwned, a.Element, a.JumpLinks = false, "", [][4]int{{1, 1, 3, 1}}
		}, false},
		{"negative minimum armor", func(a *AdventureConfig) { a.Boss.ArmorFloor = -1 }, false},
		{"lane cooldown of one round", func(a *AdventureConfig) { a.Boss.CooldownSeconds = 3 }, false},
		{"lane cooldown of part of a round", func(a *AdventureConfig) { a.Boss.CooldownSeconds = 7 }, false},
		{"phase lane cooldown of one round", func(a *AdventureConfig) {
			a.Boss.Phases = []AdventureBossPhase{{BelowPercent: 25, Patterns: [][]string{{"lane_1"}}, CooldownSeconds: 3}}
		}, true},
		{"phase lane cooldown of part of a round", func(a *AdventureConfig) {
			a.Boss.Phases = []AdventureBossPhase{{BelowPercent: 25, Patterns: [][]string{{"lane_1"}}, CooldownSeconds: 4}}
		}, false},
		{"damage as a percent of the boss attack", func(a *AdventureConfig) { a.Effects[0].Damage, a.Effects[0].DamagePercent = 0, 150 }, true},
		{"both flat and percent damage", func(a *AdventureConfig) { a.Effects[0].DamagePercent = 150 }, false},
		{"neither flat nor percent damage", func(a *AdventureConfig) { a.Effects[0].Damage = 0 }, false},
		{"percent damage without a boss", func(a *AdventureConfig) {
			a.Boss, a.Effects = nil, []EnvironmentEffect{{ID: "vent", Name: "Vent", Kind: "occupation", Rect: [4]int{4, 4, 5, 5}, School: "fire", DamagePercent: 25}}
		}, false},
		{"lingering damage both flat and percent", func(a *AdventureConfig) { a.Effects[0].SustainDamage, a.Effects[0].SustainPercent = 3, 25 }, false},
		{"lingering damage on a lane", func(a *AdventureConfig) { a.Effects[0].SustainPercent = 25 }, true},
		{"lingering damage on an occupation", func(a *AdventureConfig) { a.Effects[1].SustainPercent = 25 }, false},
		{"regeneration up to a ceiling", func(a *AdventureConfig) { a.Boss.HealPercent, a.Boss.HealCeilingPercent = 5, 50 }, true},
		{"regeneration without a ceiling", func(a *AdventureConfig) { a.Boss.HealPercent = 5 }, false},
		{"armor reduction past 100%", func(a *AdventureConfig) {
			a.Controls = []EnvironmentControl{{ID: "brace", ArmorReductionPercent: 120}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := base()
			tc.edit(&a)
			cfg := MapConfigs{Maps: map[string]MapConfig{"m": {Adventure: &a}}}
			if err := cfg.ValidateAdventures(); (err == nil) != tc.ok {
				t.Fatalf("valid=%v, want %v: %v", err == nil, tc.ok, err)
			}
		})
	}
}

// The push aims at the lanes the boss fires now: its phase's patterns, each
// lane once, else its default lanes.
func TestAdventureBossPhaseLanes(t *testing.T) {
	b := &AdventureBoss{Lanes: []string{"a", "b"}, CooldownSeconds: 6, Phases: []AdventureBossPhase{
		{BelowPercent: 50, Patterns: [][]string{{"c", "d"}, {"c"}}},
		{BelowPercent: 25, Patterns: [][]string{{"e"}}},
	}}
	for _, tc := range []struct {
		hp   int
		want []string
	}{{100, []string{"a", "b"}}, {51, []string{"a", "b"}}, {50, []string{"c", "d"}}, {25, []string{"e"}}} {
		if got := b.PhaseLanes(tc.hp, 100); !slices.Equal(got, tc.want) {
			t.Fatalf("HP %d: lanes %v, want %v", tc.hp, got, tc.want)
		}
	}
}
