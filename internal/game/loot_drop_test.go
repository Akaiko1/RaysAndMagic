package game

import (
	"testing"

	"ugataima/internal/config"
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
