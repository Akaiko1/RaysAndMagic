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
}
