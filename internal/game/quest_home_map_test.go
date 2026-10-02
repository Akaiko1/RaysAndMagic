package game

import (
	"testing"

	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// A target_map kill quest counts the monsters created on its map wherever
// they stand or die: a forest spider that chased the party into the desert is
// still a lake spider, and one born in the desert never is. A full quota of
// forest-born spiders at the lake keeps the quest open in every row, so the
// subject alone decides the kill credit and the census.
func TestTargetMapQuestsCountMonstersByHome(t *testing.T) {
	t.Chdir("../..")
	for _, tc := range []struct {
		name              string
		born, dies        string
		byAlly            bool
		reload            string // "", "unified", "split" or "legacy" (a save written before home_map)
		counts            bool
		wantHomeAfterLoad string
	}{
		{name: "killed at home", born: "forest", dies: "forest", counts: true},
		{name: "lured into the desert", born: "forest", dies: "desert", counts: true},
		{name: "lured into the desert, killed by a summon", born: "forest", dies: "desert", byAlly: true, counts: true},
		{name: "desert-born lured into the forest", born: "desert", dies: "forest", counts: false},
		{name: "desert-born at home", born: "desert", dies: "desert", counts: false},
		{name: "lured, saved and reloaded", born: "forest", dies: "desert", reload: "unified", counts: true, wantHomeAfterLoad: "forest"},
		{name: "lured, saved and loaded on split maps", born: "forest", dies: "desert", reload: "split", counts: true, wantHomeAfterLoad: "forest"},
		{name: "legacy save, away from home", born: "forest", dies: "desert", reload: "legacy", counts: false, wantHomeAfterLoad: "desert"},
		{name: "legacy save, at home", born: "forest", dies: "forest", reload: "legacy", counts: true, wantHomeAfterLoad: "forest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, wm, _ := bootOpenWorldGame(t, true)
			if !g.openWorldActive() {
				t.Fatal("fixture is not on the unified world")
			}
			// A walkable spot in the region: where one of its authored monsters stands.
			ts := float64(g.config.GetTileSize())
			at := func(region string) (float64, float64) {
				for _, m := range wm.OpenWorld.Monsters {
					if r := wm.OpenWorldRegionAtTile(TileIndex(m.X, ts), TileIndex(m.Y, ts)); r != nil && r.MapKey == region {
						return m.X, m.Y
					}
				}
				t.Fatalf("region %s has no monsters", region)
				return 0, 0
			}
			spawn := func(region string) *monster.Monster3D {
				x, y := at(region)
				m := monster.NewMonster3DFromConfig(x, y, "forest_spider", g.config)
				g.registerSpawnedMonster(m)
				return m
			}
			def := g.questManager.Definitions()["lake_spiders"]
			if def == nil || def.TargetCount <= 0 {
				t.Fatalf("lake_spiders lost its target_count: %+v", def)
			}
			// Authored lake spiders, if the map ever places any, count too.
			census := g.countLivingQuestTargets(def)
			for i := 0; i < def.TargetCount; i++ {
				spawn("forest")
			}
			census += def.TargetCount
			subject := spawn(tc.born)
			if subject.HomeMap != tc.born {
				t.Fatalf("spawned in %s, home = %q", tc.born, subject.HomeMap)
			}
			subject.X, subject.Y = at(tc.dies)
			g.camera.X, g.camera.Y = at(tc.dies)
			g.syncOpenWorldRegion()
			(&InputHandler{game: g}).handleGiveQuest("lake_spiders")

			if tc.reload != "" {
				save := g.buildSave(wm)
				if tc.reload == "legacy" {
					for key := range save.MapMonsters {
						for i := range save.MapMonsters[key] {
							save.MapMonsters[key][i].HomeMap = ""
						}
					}
				}
				if tc.reload == "split" {
					g, wm, _ = bootOpenWorldGame(t, false)
				}
				if err := g.applySave(wm, &save); err != nil {
					t.Fatalf("load: %v", err)
				}
				id := subject.ID
				subject = nil
				for _, m := range g.world.Monsters {
					if m.ID == id {
						subject = m
					}
				}
				if subject == nil {
					t.Fatal("subject spider missing from the loaded world")
				}
				if subject.HomeMap != tc.wantHomeAfterLoad {
					t.Fatalf("home after load = %q, want %q", subject.HomeMap, tc.wantHomeAfterLoad)
				}
			}

			q := g.questManager.GetQuest("lake_spiders")
			if q == nil || q.Target() != def.TargetCount || q.CurrentCount != 0 {
				t.Fatalf("lake_spiders = %+v, want active at 0/%d", q, def.TargetCount)
			}
			wantCensus := census
			if tc.counts {
				wantCensus++
			}
			if got := g.countLivingQuestTargets(q.Definition); got != wantCensus {
				t.Fatalf("living lake spiders = %d, want %d", got, wantCensus)
			}

			if tc.byAlly {
				ally := g.combat.spawnPartyAlly("frost_elemental", summonSpellOwner(spells.SpellID("summon_ice_elemental")))
				if ally == nil {
					t.Fatal("no room to summon")
				}
				packet := singleMonsterDamagePacket(damagecalc.Parts{True: subject.HitPoints + 1000}, "physical", 0)
				g.combat.strikeMonsterPacketFor(ally, subject, packet, nil, false, true, true, false)
			} else {
				subject.HitPoints = 0
				g.combat.finishMonsterKill(subject)
			}
			if subject.IsAlive() {
				t.Fatal("subject survived")
			}
			want := 0
			if tc.counts {
				want = 1
			}
			if q.CurrentCount != want {
				t.Fatalf("lake_spiders progress = %d, want %d", q.CurrentCount, want)
			}
		})
	}
}

// Every monster records the map it was created on: authored rosters on both
// world layouts, night packs, and runtime spawns on the unified world.
func TestMonstersRecordTheirHomeMap(t *testing.T) {
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := float64(cfg.GetTileSize())
	regionOf := func(m *monster.Monster3D) string {
		if r := wm.OpenWorldRegionAtTile(TileIndex(m.X, ts), TileIndex(m.Y, ts)); r != nil {
			return r.MapKey
		}
		return ""
	}
	check := func(stage string) {
		t.Helper()
		for _, m := range wm.OpenWorld.Monsters {
			if m.HomeMap == "" || m.HomeMap != regionOf(m) {
				t.Fatalf("%s: %s at region %q has home %q", stage, m.Key, regionOf(m), m.HomeMap)
			}
		}
	}
	check("authored")
	// Night packs fill the vacancies a cleared roster leaves.
	for _, m := range wm.OpenWorld.Monsters {
		m.HitPoints = 0
	}
	g.syncDayNightPacks(true)
	packs := 0
	for _, m := range wm.OpenWorld.Monsters {
		if m.PackKey != "" {
			packs++
		}
	}
	if packs == 0 {
		t.Fatal("no night pack spawned (positive control)")
	}
	check("night packs")

	_, wmSplit, _ := bootOpenWorldGame(t, false)
	for key, w := range wmSplit.LoadedMaps {
		for _, m := range w.Monsters {
			if m.HomeMap != key {
				t.Fatalf("split map %s: %s has home %q", key, m.Key, m.HomeMap)
			}
		}
	}
}
