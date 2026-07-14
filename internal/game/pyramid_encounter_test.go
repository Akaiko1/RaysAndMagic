package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// The sanctum reliquaries belong to the four Isis at the upper dais, not to
// the lower Isis, Minotaurs, or hidden Dragon on the same map. This guards both
// the plural encounter binding and its multi-chest spatial anchor.
func TestPyramidSanctumIsisBindReliquaries(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "pyramid_3")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("pyramid_3 world did not load")
	}

	bound := 0
	for _, m := range w.Monsters {
		if m == nil || !m.IsEncounterMonster {
			continue
		}
		bound++
		if m.Key != "isis" {
			t.Fatalf("only sanctum Isis may bind reliquaries, got %q", m.Key)
		}
		if ty := int(m.Y / cfg.GetTileSize()); ty != 5 {
			t.Fatalf("bound Isis must be at upper dais row 5, got tile (%d,%d)", int(m.X/cfg.GetTileSize()), ty)
		}
		if m.EncounterRewards == nil || len(m.EncounterRewards.TreasureChests) != 4 {
			t.Fatalf("bound Isis must carry all four reliquaries, got %+v", m.EncounterRewards)
		}
	}
	if bound != 4 {
		t.Fatalf("bound sanctum Isis = %d, want 4", bound)
	}

	for _, m := range w.Monsters {
		if m == nil || m.IsEncounterMonster {
			continue
		}
		if m.Key == "dragon" || m.Key == "minotaur" || m.Key == "isis" {
			if m.EncounterRewards != nil {
				t.Fatalf("ordinary pyramid mob %q unexpectedly binds the reliquaries", m.Key)
			}
		}
	}

}

func legacyPyramidReliquaryRewards() *monsterPkg.EncounterRewards {
	return &monsterPkg.EncounterRewards{TreasureChests: []monsterPkg.TreasureChestReward{
		{ID: "pyramid_black_dragon_statuette_chest"},
		{ID: "pyramid_red_dragon_statuette_chest"},
		{ID: "pyramid_green_dragon_statuette_chest"},
		{ID: "pyramid_gold_dragon_statuette_chest"},
	}}
}

func TestMigrateLegacyPyramidSanctumEncounter(t *testing.T) {
	cfg := loadTestConfig(t)
	ts := float64(cfg.GetTileSize())
	makeMob := func(t *testing.T, key string, tx, ty int, rewards *monsterPkg.EncounterRewards) *monsterPkg.Monster3D {
		t.Helper()
		m := monsterPkg.NewMonster3DFromConfig(float64(tx)*ts+ts/2, float64(ty)*ts+ts/2, key, cfg)
		if m == nil {
			t.Fatalf("%s must load from monsters.yaml", key)
		}
		m.IsEncounterMonster = true
		m.EncounterRewards = rewards
		return m
	}

	t.Run("keeps surviving dais isis only", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		dais := makeMob(t, "isis", 9, 5, rewards)
		dragon := makeMob(t, "dragon", 2, 16, rewards)
		lowerIsis := makeMob(t, "isis", 6, 19, rewards)
		w.Monsters = []*monsterPkg.Monster3D{dais, dragon, lowerIsis}

		if got := g.migrateLegacyPyramidSanctumEncounter(w); got != nil {
			t.Fatal("a surviving upper Isis must keep the reliquaries pending")
		}
		if !g.loadNeedsResave {
			t.Fatal("legacy binding migration must persist the repaired save")
		}
		if !dais.IsEncounterMonster || dais.EncounterRewards != rewards {
			t.Fatal("upper dais Isis must remain bound to reliquaries")
		}
		for _, m := range []*monsterPkg.Monster3D{dragon, lowerIsis} {
			if m.IsEncounterMonster || m.EncounterRewards != nil {
				t.Fatalf("lower map mob %s must be detached from reliquaries", m.Key)
			}
		}
	})

	t.Run("pays already cleared dais", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		dragon := makeMob(t, "dragon", 2, 16, rewards)
		lowerIsis := makeMob(t, "isis", 6, 19, rewards)
		w.Monsters = []*monsterPkg.Monster3D{dragon, lowerIsis}

		if got := g.migrateLegacyPyramidSanctumEncounter(w); got != rewards {
			t.Fatal("an old save with all upper Isis dead must receive overdue reliquaries")
		}
		if !g.loadNeedsResave {
			t.Fatal("legacy completion migration must persist the repaired save")
		}
		for _, m := range w.Monsters {
			if m.IsEncounterMonster || m.EncounterRewards != nil {
				t.Fatalf("remaining lower map mob %s must be detached from reliquaries", m.Key)
			}
		}
	})
}
