package config

import "testing"

// A class's builds are complete and distinct: known stats, a secondary apart
// from the primary, targets in 1..99, 1..MaxAutoBuildGear gear keys, and a
// named, unique key that is not the default's.
func TestClassAutoBuildsValidation(t *testing.T) {
	good := AutoStatsConfig{Speed: 16, Endurance: 20, Primary: "might", Secondary: "speed", Gear: []string{"muramasa"}}
	build := func(key string, mutate func(*AutoStatsConfig)) AutoBuild {
		a := good
		a.Gear = append([]string(nil), good.Gear...)
		if mutate != nil {
			mutate(&a)
		}
		return AutoBuild{Key: key, Name: "Build", AutoStatsConfig: a}
	}
	if err := validateClassProgression("test", ClassStats{AutoStats: good, AutoBuilds: []AutoBuild{build("swift", nil)}}); err != nil {
		t.Fatalf("control: a complete build must validate: %v", err)
	}
	for _, tc := range []struct {
		name   string
		builds []AutoBuild
	}{
		{"unknown primary", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Primary = "charm" })}},
		{"secondary repeats primary", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Secondary = "might" })}},
		{"speed target out of range", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Speed = 0 })}},
		{"endurance target out of range", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Endurance = 100 })}},
		{"no gear", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Gear = nil })}},
		{"too much gear", []AutoBuild{build("a", func(a *AutoStatsConfig) { a.Gear = make([]string, MaxAutoBuildGear+1) })}},
		{"empty key", []AutoBuild{build("", nil)}},
		{"default key", []AutoBuild{build(DefaultAutoBuildKey, nil)}},
		{"repeated key", []AutoBuild{build("a", nil), build("a", nil)}},
		{"no name", []AutoBuild{{Key: "a", AutoStatsConfig: good}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateClassProgression("test", ClassStats{AutoStats: good, AutoBuilds: tc.builds}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
