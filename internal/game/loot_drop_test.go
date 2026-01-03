package game

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

func assertLootDrops(t *testing.T, monsterKey string, trials int) {
	t.Helper()

	cs := newTestCombatSystemWithConfig(t)

	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	entries := config.GetLootTable(monsterKey)
	if len(entries) == 0 {
		t.Fatalf("expected %s loot entries", monsterKey)
	}

	cs.game.party.Inventory = nil
	for i := 0; i < trials; i++ {
		monster := monsterPkg.NewMonster3DFromConfig(0, 0, monsterKey, cs.game.config)
		cs.checkMonsterLootDrop(monster)
	}

	counts := make(map[string]int)
	for _, item := range cs.game.party.Inventory {
		counts[item.Name]++
	}
	for _, e := range entries {
		var (
			dropName string
			err      error
		)
		switch e.Type {
		case "item":
			var it items.Item
			it, err = items.TryCreateItemFromYAML(e.Key)
			dropName = it.Name
		case "weapon":
			var it items.Item
			it, err = items.TryCreateWeaponFromYAML(e.Key)
			dropName = it.Name
		default:
			continue
		}
		if err != nil {
			t.Fatalf("%s loot %s %s: %v", monsterKey, e.Type, e.Key, err)
		}

		drops := counts[dropName]
		observedRate := float64(drops) / float64(trials)
		expectedRate := e.Chance
		stdDev := (expectedRate * (1.0 - expectedRate)) / float64(trials)
		if stdDev > 0 {
			stdDev = math.Sqrt(stdDev)
		}
		t.Logf("%s loot %s (%s): expected=%.2f%% observed=%.2f%% count=%d/%d",
			monsterKey, dropName, e.Key, expectedRate*100, observedRate*100, drops, trials)

		if drops == 0 {
			t.Fatalf("expected %s to drop %s over %d trials", monsterKey, dropName, trials)
		}

		// Allow a 4σ window to keep the test stable while still validating the chance.
		tolerance := 4.0 * stdDev
		if observedRate < expectedRate-tolerance || observedRate > expectedRate+tolerance {
			t.Fatalf("%s loot rate for %s out of bounds: expected %.2f%% ± %.2f%%, observed %.2f%%",
				monsterKey, dropName, expectedRate*100, tolerance*100, observedRate*100)
		}
	}
}

func TestLootDropsOrc(t *testing.T) {
	assertLootDrops(t, "orc", 1000)
}

func TestLootDropsForestOrc(t *testing.T) {
	assertLootDrops(t, "forest_orc", 1000)
}

func TestLootDropsPixie(t *testing.T) {
	assertLootDrops(t, "pixie", 1000)
}

func TestLootDropsDragon(t *testing.T) {
	assertLootDrops(t, "dragon", 1000)
}
