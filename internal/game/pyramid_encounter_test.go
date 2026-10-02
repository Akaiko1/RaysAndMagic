package game

import (
	"math"
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// The sanctum reliquaries belong to the encounter's authored roster - the
// requested count of each type NEAREST its first chest (the multi-chest
// anchor) - not to the lower Isis, Minotaurs, or hidden Dragon on the same
// map. Guards both the plural encounter binding and its spatial anchor.
func TestPyramidSanctumIsisBindReliquaries(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "pyramid_3")
	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("pyramid_3 world did not load")
	}
	encounters := wm.MapConfigs["pyramid_3"].ClearEncounters
	if len(encounters) != 1 || encounters[0].Rewards == nil || len(encounters[0].Rewards.TreasureChests) == 0 {
		t.Fatalf("pyramid_3 must author one multi-chest clear encounter, got %+v", encounters)
	}
	enc := encounters[0]
	want := map[string]int{}
	for _, req := range enc.Monsters {
		want[req.Type] += req.Count
	}
	ts := cfg.GetTileSize()
	anchor := enc.Rewards.TreasureChests[0]
	ax, ay := TileCenterFromTile(anchor.TileX, anchor.TileY, ts)
	dist := func(m *monsterPkg.Monster3D) float64 { return math.Hypot(m.X-ax, m.Y-ay) }

	bound := map[string]int{}
	farthestBound := map[string]float64{}
	for _, m := range w.Monsters {
		if m == nil || !m.IsEncounterMonster {
			continue
		}
		bound[m.Key]++
		farthestBound[m.Key] = max(farthestBound[m.Key], dist(m))
		if want[m.Key] == 0 {
			t.Fatalf("only the authored roster may bind reliquaries, got %q", m.Key)
		}
		if m.EncounterRewards == nil || len(m.EncounterRewards.TreasureChests) != len(enc.Rewards.TreasureChests) {
			t.Fatalf("bound %s must carry all %d reliquaries, got %+v", m.Key, len(enc.Rewards.TreasureChests), m.EncounterRewards)
		}
	}
	for key, n := range want {
		if bound[key] != n {
			t.Fatalf("bound %s = %d, want the authored %d", key, bound[key], n)
		}
	}

	for _, m := range w.Monsters {
		if m == nil || m.IsEncounterMonster {
			continue
		}
		if m.EncounterRewards != nil {
			t.Fatalf("ordinary pyramid mob %q unexpectedly binds the reliquaries", m.Key)
		}
		if want[m.Key] > 0 && dist(m) < farthestBound[m.Key] {
			t.Fatalf("unbound %s stands nearer the anchor chest than a bound one", m.Key)
		}
	}
}

func legacyPyramidReliquaryRewards() *monsterPkg.EncounterRewards {
	return &monsterPkg.EncounterRewards{TreasureChests: []monsterPkg.TreasureChestReward{
		{ID: "pyramid_black_dragon_statuette_chest", TileX: 9, TileY: 3},
		{ID: "pyramid_red_dragon_statuette_chest", TileX: 13, TileY: 3},
		{ID: "pyramid_green_dragon_statuette_chest", TileX: 17, TileY: 3},
		{ID: "pyramid_gold_dragon_statuette_chest", TileX: 21, TileY: 3},
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

	t.Run("keeps surviving isis after movement", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		dais := makeMob(t, "isis", 9, 8, rewards)
		dragon := makeMob(t, "dragon", 2, 16, rewards)
		w.Monsters = []*monsterPkg.Monster3D{dais, dragon}

		if got := g.migrateLegacyPyramidSanctumEncounter(w); got != nil {
			t.Fatal("a surviving upper Isis must keep the reliquaries pending")
		}
		if !g.loadNeedsResave {
			t.Fatal("legacy binding migration must persist the repaired save")
		}
		if !dais.IsEncounterMonster || dais.EncounterRewards != rewards {
			t.Fatal("upper dais Isis must remain bound to reliquaries")
		}
		if dragon.IsEncounterMonster || dragon.EncounterRewards != nil {
			t.Fatal("legacy non-Isis must be detached from reliquaries")
		}
	})

	t.Run("keeps exactly the dais group", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		dais := []*monsterPkg.Monster3D{
			makeMob(t, "isis", 9, 5, rewards),
			makeMob(t, "isis", 13, 8, rewards), // moved from its authored row
			makeMob(t, "isis", 17, 5, rewards),
			makeMob(t, "isis", 21, 5, rewards),
		}
		lower := []*monsterPkg.Monster3D{
			makeMob(t, "isis", 20, 17, rewards),
			makeMob(t, "isis", 21, 17, rewards),
			makeMob(t, "isis", 22, 17, rewards),
		}
		dragon := makeMob(t, "dragon", 2, 16, rewards)
		w.Monsters = append(append(append([]*monsterPkg.Monster3D{}, dais...), lower...), dragon)

		if got := g.migrateLegacyPyramidSanctumEncounter(w); got != nil {
			t.Fatal("surviving dais Isis must keep the reliquaries pending")
		}
		for _, m := range dais {
			if !m.IsEncounterMonster || m.EncounterRewards != rewards {
				t.Fatal("a dais Isis was detached from the reliquaries")
			}
		}
		for _, m := range append(lower, dragon) {
			if m.IsEncounterMonster || m.EncounterRewards != nil {
				t.Fatalf("lower map mob %s still holds the reliquaries", m.Key)
			}
		}
	})

	t.Run("trusts current encounter membership after movement", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		var dais []*monsterPkg.Monster3D
		for i, tileY := range []int{4, 6, 7, 8} {
			dais = append(dais, makeMob(t, "isis", 9+i*4, tileY, rewards))
		}
		w.Monsters = dais

		if got := g.migrateLegacyPyramidSanctumEncounter(w); got != nil {
			t.Fatal("a current encounter must not pay rewards during load")
		}
		if g.loadNeedsResave {
			t.Fatal("a current encounter must not enter the legacy migration")
		}
		for _, m := range dais {
			if !m.IsEncounterMonster || m.EncounterRewards != rewards {
				t.Fatal("saved encounter membership must survive monster movement")
			}
		}
	})

	t.Run("pays already cleared dais", func(t *testing.T) {
		w := newTestWorldSized(cfg, 30, 30)
		g := newTestGame(cfg, w)
		rewards := legacyPyramidReliquaryRewards()
		lowerIsis := makeMob(t, "isis", 20, 17, rewards)
		w.Monsters = []*monsterPkg.Monster3D{lowerIsis}

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
