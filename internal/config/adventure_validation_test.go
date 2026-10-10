package config

import "testing"

// Rules an authored adventure must meet so nothing silently never fires.
func TestValidateAdventuresRules(t *testing.T) {
	lane := EnvironmentEffect{ID: "lane_1", Name: "Lane", Kind: "lane", Rect: [4]int{1, 1, 2, 2}, School: "fire", Damage: 5, WarningSeconds: 1, WarningRounds: 1, DurationSeconds: 2, DurationRounds: 1}
	gated := EnvironmentEffect{ID: "vent", Name: "Vent", Kind: "occupation", Rect: [4]int{4, 4, 5, 5}, School: "fire", Damage: 5, BossBelowPercent: 50, WarningSeconds: 1.25, WarningRounds: 1}
	base := func() AdventureConfig {
		return AdventureConfig{OpeningOwned: true, Element: "fire", Boss: &AdventureBoss{Monster: "boss", EveryActions: 2}, Effects: []EnvironmentEffect{lane, gated}}
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
