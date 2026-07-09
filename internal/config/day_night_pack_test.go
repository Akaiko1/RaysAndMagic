package config

import "testing"

func TestDayNightPackPhaseMembers(t *testing.T) {
	mono := DayNightPackConfig{DayMonster: "wolf", NightMonster: "forest_spider", Count: 8}
	if got := mono.PhaseMembers(false); len(got) != 1 || got[0].Monster != "wolf" || got[0].Count != 8 {
		t.Fatalf("mono day = %+v, want one wolf x8", got)
	}
	if got := mono.PhaseMembers(true); len(got) != 1 || got[0].Monster != "forest_spider" || got[0].Count != 8 {
		t.Fatalf("mono night = %+v, want one forest_spider x8", got)
	}

	// Night-only pack: day phase resolves to nothing.
	nightOnly := DayNightPackConfig{NightMonster: "mummy", Count: 5}
	if got := nightOnly.PhaseMembers(false); got != nil {
		t.Fatalf("night-only day = %+v, want nil", got)
	}
	if got := nightOnly.PhaseMembers(true); len(got) != 1 || got[0].Monster != "mummy" {
		t.Fatalf("night-only night = %+v, want one mummy", got)
	}

	// Mixed list wins over the single-monster shorthand for that phase.
	mixed := DayNightPackConfig{
		NightMonster:  "ignored",
		Count:         99,
		NightMonsters: []PackMemberConfig{{Monster: "ningyo", Count: 4}, {Monster: "vengeful_ningyo", Count: 1}},
	}
	got := mixed.PhaseMembers(true)
	if len(got) != 2 || got[0].Monster != "ningyo" || got[0].Count != 4 || got[1].Monster != "vengeful_ningyo" || got[1].Count != 1 {
		t.Fatalf("mixed night = %+v, want ningyo x4 + vengeful_ningyo x1", got)
	}
}
