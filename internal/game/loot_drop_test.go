package game

import (
	"slices"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestRollLootEntriesHonorsRollCount(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	drops := rollLootEntries([]config.LootEntry{{
		Type:   "item",
		Key:    "black_dragon_scale",
		Chance: 1,
		Rolls:  3,
	}})
	if len(drops) != 3 {
		t.Fatalf("guaranteed three-roll entry dropped %d items, want 3", len(drops))
	}
}

// One card rule: bosses drop their card at card_drop_chance.boss, every other
// monster at card_drop_chance.monster. The boot rejects a table that disagrees.
func TestCardDropsFollowTheRule(t *testing.T) {
	crateTestGame(t)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	shipped := config.GlobalLoots
	t.Cleanup(func() { config.GlobalLoots = shipped })
	if err := ValidateCardDrops(); err != nil {
		t.Fatalf("shipped loot breaks the card rule: %v", err)
	}
	rule := shipped.CardDropChance
	for _, tc := range []struct {
		monster, card string
		chance        float64
	}{
		{"goblin", "goblin_card", rule.Boss},                        // ordinary mob at the boss rate
		{"golden_thief_bug", "golden_thief_bug_card", rule.Monster}, // boss at the mob rate
	} {
		edited := *shipped
		edited.Loots = map[string][]config.LootEntry{}
		for key, entries := range shipped.Loots {
			edited.Loots[key] = slices.Clone(entries)
		}
		found := false
		for i, e := range edited.Loots[tc.monster] {
			if e.Key == tc.card {
				edited.Loots[tc.monster][i].Chance, found = tc.chance, true
			}
		}
		if !found {
			t.Fatalf("%s no longer drops %s", tc.monster, tc.card)
		}
		config.GlobalLoots = &edited
		if ValidateCardDrops() == nil {
			t.Fatalf("%s card at %g accepted", tc.monster, tc.chance)
		}
	}
	// A card at the right rate is still its own roll, never a first_of rung.
	edited := *shipped
	edited.Loots = map[string][]config.LootEntry{"goblin": {{Type: "first_of", Key: "g", Of: []config.LootEntry{{Type: "item", Key: "goblin_card", Chance: rule.Monster}}}}}
	config.GlobalLoots = &edited
	if ValidateCardDrops() == nil {
		t.Fatal("a card inside a first_of group passed the card rule")
	}
}

// A landed entry with min/max drops that many units: one stack for goods that
// stack, separate pieces for gear; without them, one unit.
func TestLootEntryQuantity(t *testing.T) {
	crateTestGame(t)
	for _, tc := range []struct {
		name         string
		entry        config.LootEntry
		items, units int
	}{
		{"potion stack", config.LootEntry{Type: "item", Key: "health_potion", Chance: 1, Min: 3, Max: 3}, 1, 3},
		{"single potion by default", config.LootEntry{Type: "item", Key: "health_potion", Chance: 1}, 1, 1},
		{"gear as pieces", config.LootEntry{Type: "weapon", Key: "iron_sword", Chance: 1, Min: 2, Max: 2}, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drops := rollLootEntries([]config.LootEntry{tc.entry})
			units := 0
			for _, it := range drops {
				units += it.Count()
			}
			if len(drops) != tc.items || units != tc.units {
				t.Fatalf("drops = %d items / %d units, want %d / %d", len(drops), units, tc.items, tc.units)
			}
		})
	}
	// The range is inclusive at both ends.
	seen := map[int]bool{}
	for range 400 {
		seen[rollLootEntries([]config.LootEntry{{Type: "item", Key: "health_potion", Chance: 1, Min: 1, Max: 4}})[0].Count()] = true
	}
	for n := 1; n <= 4; n++ {
		if !seen[n] {
			t.Fatalf("1-4 potions never dropped %d in 400 rolls (seen %v)", n, seen)
		}
	}
	if seen[0] || seen[5] {
		t.Fatalf("quantity left its range: %v", seen)
	}
}
