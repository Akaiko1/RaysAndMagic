package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func packSlotTestGame(t *testing.T, active bool) (*MMGame, *world.WorldManager, *world.World3D) {
	t.Helper()
	g, tile := summonTileWorld(t)
	w := g.world
	for i, key := range []string{"wolf", "wolf", "wolf", "treant", "wolf"} {
		tx, ty := 20+i*2, 20
		w.MonsterSpawns = append(w.MonsterSpawns, world.MonsterSpawn{X: tx, Y: ty, MonsterKey: key})
		x, y := TileCenterFromTile(tx, ty, tile)
		w.Monsters = append(w.Monsters, monster.NewMonster3DFromConfig(x, y, key, g.config))
	}
	g.config.DayNight.Packs = []config.DayNightPackConfig{{Map: "pack_test", DayMonster: "wolf", NightMonster: "forest_spider", Count: 5, MinPlayerDistTiles: 0.5}}
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "pack_test"
	wm.LoadedMaps = map[string]*world.World3D{"pack_test": w}
	wm.MapConfigs = map[string]*config.MapConfig{"pack_test": {}}
	if !active {
		g.world = newTestWorld(g.config)
		wm.CurrentMapKey = "other"
		wm.LoadedMaps["other"] = g.world
		wm.MapConfigs["other"] = &config.MapConfig{}
	}
	setTestWorldManager(t, wm)
	g.reusableDeadSet = make(map[string]bool)
	g.reusableEncounterRewardsMap = make(map[*monster.EncounterRewards]int)
	return g, wm, w
}

func livePhasePack(w *world.World3D, night bool) []*monster.Monster3D {
	var result []*monster.Monster3D
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() && m.PackKey == dayNightPackTag("pack_test", night) {
			result = append(result, m)
		}
	}
	return result
}

// Case table: all rows cross day/night and active/inactive maps. RT/TB share
// syncDayNightPacks; both are exercised. Modern/legacy save and merged-region
// projection are covered separately below. Counts are caps, never retries.
func TestDayNightPacksUseVacantAuthoredSlots(t *testing.T) {
	cases := []struct {
		name   string
		killed int
		want   int
		setup  func(*MMGame, *world.World3D, bool)
	}{
		{name: "living roamers keep all slots", want: 0, setup: func(g *MMGame, w *world.World3D, _ bool) {
			for _, m := range w.Monsters {
				m.X, m.Y = 100, 100
			}
		}},
		{name: "three wolves and one treant killed", killed: 4, want: 4},
		{name: "fully cleared", killed: 5, want: 5},
		{name: "configured count caps free slots", killed: 5, want: 2, setup: func(g *MMGame, w *world.World3D, _ bool) { g.config.DayNight.Packs[0].Count = 2 }},
		{name: "removed corpses", killed: 5, want: 5, setup: func(g *MMGame, w *world.World3D, _ bool) { w.Monsters = nil }},
		{name: "no authored roster", killed: 5, setup: func(g *MMGame, w *world.World3D, _ bool) { w.MonsterSpawns = nil }},
		{name: "duplicate slot", killed: 5, want: 5, setup: func(g *MMGame, w *world.World3D, _ bool) {
			w.MonsterSpawns = append(w.MonsterSpawns, w.MonsterSpawns[0])
		}},
		{name: "invalid slots", killed: 5, want: 5, setup: func(g *MMGame, w *world.World3D, _ bool) {
			w.MonsterSpawns = append(w.MonsterSpawns, world.MonsterSpawn{X: -1, Y: 20}, world.MonsterSpawn{X: 20, Y: 999})
		}},
		{name: "blocked terrain", killed: 5, want: 4, setup: func(g *MMGame, w *world.World3D, _ bool) {
			wall, _ := world.GlobalTileManager.GetTileTypeFromKey("wall")
			w.Tiles[20][20] = wall
		}},
		{name: "foreign mob on vacant slot", killed: 5, want: 4, setup: func(g *MMGame, w *world.World3D, _ bool) {
			m := monster.NewMonster3DFromConfig(100, 100, "dragon", g.config)
			m.X, m.Y = TileCenterFromTile(20, 20, g.config.GetTileSize())
			w.Monsters = append(w.Monsters, m)
		}},
		{name: "legacy survivor without authored anchor", killed: 4, want: 4, setup: func(g *MMGame, w *world.World3D, _ bool) {
			m := w.Monsters[4]
			m.X, m.Y = 100, 100
			m.SpawnX, m.SpawnY = TileCenterFromTile(27, 19, g.config.GetTileSize())
		}},
		{name: "outgoing pack releases slot", killed: 4, want: 4, setup: func(g *MMGame, w *world.World3D, night bool) {
			x, y := TileCenterFromTile(20, 20, g.config.GetTileSize())
			m := monster.NewMonster3DFromConfig(x, y, "wolf", g.config)
			m.PackKey = dayNightPackTag("pack_test", !night)
			w.Monsters = append(w.Monsters, m)
			if w == g.world {
				w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			}
		}},
		{name: "clear gate still blocks partial clear", killed: 4, setup: func(g *MMGame, w *world.World3D, _ bool) { g.config.DayNight.Packs[0].RequireMapClear = true }},
		{name: "clear gate allows full clear", killed: 5, want: 5, setup: func(g *MMGame, w *world.World3D, _ bool) { g.config.DayNight.Packs[0].RequireMapClear = true }},
		{name: "clear gate sees outgoing survivors", killed: 5, setup: func(g *MMGame, w *world.World3D, night bool) {
			g.config.DayNight.Packs[0].RequireMapClear = true
			x, y := TileCenterFromTile(20, 20, g.config.GetTileSize())
			m := monster.NewMonster3DFromConfig(x, y, "wolf", g.config)
			m.PackKey = dayNightPackTag("pack_test", !night)
			w.Monsters = append(w.Monsters, m)
		}},
		{name: "mixed pack shares vacancies", killed: 4, want: 4, setup: func(g *MMGame, w *world.World3D, _ bool) {
			members := []config.PackMemberConfig{{Monster: "wolf", Count: 2}, {Monster: "forest_spider", Count: 3, QuestProgress: true}}
			g.config.DayNight.Packs[0].DayMonsters = members
			g.config.DayNight.Packs[0].NightMonsters = members
		}},
	}
	for _, tc := range cases {
		for _, active := range []bool{false, true} {
			for _, night := range []bool{false, true} {
				for _, tb := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/active=%v/night=%v/TB=%v", tc.name, active, night, tb), func(t *testing.T) {
						g, _, w := packSlotTestGame(t, active)
						g.turnBasedMode = tb
						for _, m := range w.Monsters[:tc.killed] {
							m.HitPoints = 0
						}
						if tc.setup != nil {
							tc.setup(g, w, night)
						}
						g.syncDayNightPacks(night)
						if active {
							(&GameLoop{game: g}).removeDeadMonstersByID()
						}
						pack := livePhasePack(w, night)
						if len(pack) != tc.want {
							t.Fatalf("spawned %d, want %d", len(pack), tc.want)
						}
						seen := make(map[[2]int]bool)
						for _, m := range pack {
							pos := [2]int{TileIndex(m.X, g.config.GetTileSize()), TileIndex(m.Y, g.config.GetTileSize())}
							if seen[pos] {
								t.Fatal("two pack monsters share a slot")
							}
							seen[pos] = true
							if pos[1] != 20 || pos[0] < 20 || pos[0] > 28 || pos[0]%2 != 0 {
								t.Fatalf("spawn outside authored slots: %v", pos)
							}
							if m.SpawnX != m.X || m.SpawnY != m.Y {
								t.Fatal("pack home does not reserve its slot")
							}
							if active && g.collisionSystem.GetEntityByID(m.ID) == nil {
								t.Fatal("pack missing collision registration")
							}
							if tc.name == "mixed pack shares vacancies" && m.QuestProgressIgnored != (m.Key != "forest_spider") {
								t.Fatal("quest eligibility changed")
							}
						}
					})
				}
			}
		}
	}
}

func TestDayNightPackPartyDistanceAndNoTopUp(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			g, _, w := packSlotTestGame(t, active)
			w.Monsters = nil
			g.camera.X, g.camera.Y = TileCenterFromTile(20, 20, g.config.GetTileSize())
			g.syncDayNightPacks(true)
			want := 5
			if active {
				want = 4
			}
			if len(livePhasePack(w, true)) != want {
				t.Fatal("party distance was not scoped to active world")
			}
			g.camera.X, g.camera.Y = 64, 64
			g.dayNightIsNight = true
			g.dayNightFrames = g.dayNightCycleFrames() / 2
			g.updateDayNight()
			if len(livePhasePack(w, true)) != want {
				t.Fatal("skipped slots were topped up outside a phase change")
			}
		})
	}
}

func TestDayNightSlotSaveRoundTrip(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("active=%v/legacy=%v", active, legacy), func(t *testing.T) {
				g, wm, w := packSlotTestGame(t, active)
				w.Monsters = w.Monsters[4:]
				w.Monsters[0].X, w.Monsters[0].Y = 100, 100
				save := g.buildSave(wm)
				if legacy {
					save.MapMonsters["pack_test"][0].SpawnPosition = nil
				}
				data, err := json.Marshal(save)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &save); err != nil {
					t.Fatal(err)
				}
				g.restoreSavedMonsters(wm, &save)
				g.syncDayNightPacks(true)
				if len(livePhasePack(w, true)) != 4 {
					t.Fatal("load freed a surviving base monster's slot")
				}
				for _, m := range livePhasePack(w, true) {
					m.X, m.Y = 120, 120
				}
				save = g.buildSave(wm)
				g.restoreSavedMonsters(wm, &save)
				if slots := g.availablePackSpawnTiles(w, 0.5, 0, 0, w.Width, w.Height); len(slots) != 0 {
					t.Fatalf("saved roaming packs lost %d slot reservations", len(slots))
				}
			})
		}
	}
}

func TestDayNightSlotsStayInMergedRegionAfterSave(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	oldTM := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = oldTM })
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, orient := range []string{"none", "rot90", "rot180", "rot270", "mirror_x", "mirror_y"} {
		t.Run(orient, func(t *testing.T) {
			wm := world.NewWorldManager(cfg)
			if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
				t.Fatal(err)
			}
			wm.MapConfigs = map[string]*config.MapConfig{"forest": wm.MapConfigs["forest"], "desert": wm.MapConfigs["desert"]}
			wm.SetOpenWorldConfig(&config.OpenWorldConfig{VoidTile: "oob_cliff", Placements: map[string]config.OpenWorldPlacement{
				"forest": {X: 20, Y: 20, Orient: orient}, "desert": {X: 400, Y: 20},
			}})
			if err := wm.LoadAllMaps(); err != nil {
				t.Fatal(err)
			}
			wm.CurrentMapKey = "forest"
			setTestWorldManager(t, wm)
			w := wm.OpenWorld
			if w == nil {
				t.Fatal("merged fixture missing")
			}
			g := newTestGame(cfg, w)
			g.config.DayNight.Packs = []config.DayNightPackConfig{{Map: "forest", DayMonster: "wolf", NightMonster: "forest_spider", Count: 5}}
			r := wm.OpenWorldRegionByKey("forest")
			vacant := make(map[[2]int]bool)
			for _, m := range w.Monsters {
				tx, ty := TileIndex(m.SpawnX, cfg.GetTileSize()), TileIndex(m.SpawnY, cfg.GetTileSize())
				inForest := tx >= r.OffsetX && tx < r.OffsetX+r.Width && ty >= r.OffsetY && ty < r.OffsetY+r.Height
				if !inForest {
					m.HitPoints = 0 // Many foreign vacancies must not expand the forest pack.
				} else if len(vacant) < 4 && world.GlobalTileManager.IsWalkable(w.Tiles[ty][tx]) {
					m.HitPoints = 0
					vacant[[2]int{tx, ty}] = true
				}
			}
			if len(vacant) != 4 {
				t.Fatal("fixture needs four usable forest slots")
			}
			checkPack := func(night bool) {
				count := 0
				for _, m := range w.Monsters {
					if m.PackKey != dayNightPackTag("forest", night) {
						continue
					}
					count++
					home := [2]int{TileIndex(m.SpawnX, cfg.GetTileSize()), TileIndex(m.SpawnY, cfg.GetTileSize())}
					if !vacant[home] {
						t.Fatalf("pack escaped vacant forest slots: %v", home)
					}
				}
				if count != 4 {
					t.Fatalf("pack size %d, want 4", count)
				}
			}
			g.syncDayNightPacks(true)
			checkPack(true)
			saved := g.buildSave(wm)
			g.restoreSavedMonsters(wm, &saved)
			checkPack(true)
			if slots := g.availablePackSpawnTiles(w, 8, r.OffsetX, r.OffsetY, r.Width, r.Height); len(slots) != 0 {
				t.Fatal("reload released occupied forest slots")
			}
			g.syncDayNightPacks(false)
			checkPack(false)
		})
	}
}

// Reservation case table: duplicate matching homes, mismatched homes, missing
// homes, and a later exact owner; non-base packs/summons must not claim extra
// fallback slots. Cross modern/legacy persistence, active/inactive maps, and
// day/night. RT/TB use this same phase entry point (covered above).
func TestDayNightSlotReservationsAreOneToOneAfterLoad(t *testing.T) {
	cases := []struct {
		name  string
		count int
		x, y  int
		want  int
		extra string
	}{
		{name: "two wolves share wolf home", count: 2, x: 20, y: 20, want: 3},
		{name: "full roster shares wolf home", count: 5, x: 20, y: 20},
		{name: "full roster shares treant home", count: 5, x: 26, y: 20},
		{name: "full roster away from homes", count: 5, x: 25, y: 19},
		{name: "fallback preserves later exact owner", count: 2, x: 22, y: 19, want: 3, extra: "exact"},
		{name: "pack duplicate is not a base survivor", count: 1, x: 20, y: 20, want: 4, extra: "pack"},
		{name: "summon duplicate is not a base survivor", count: 1, x: 20, y: 20, want: 4, extra: "summon"},
	}
	for _, tc := range cases {
		for _, active := range []bool{false, true} {
			for _, legacy := range []bool{false, true} {
				for _, night := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/active=%v/legacy=%v/night=%v", tc.name, active, legacy, night), func(t *testing.T) {
						g, wm, w := packSlotTestGame(t, active)
						w.Monsters = w.Monsters[:tc.count]
						x, y := TileCenterFromTile(tc.x, tc.y, g.config.GetTileSize())
						for _, m := range w.Monsters {
							m.X, m.Y = x, y
						}
						if tc.extra == "exact" {
							m := w.Monsters[1]
							m.X, m.Y = m.SpawnX, m.SpawnY
						} else if tc.extra != "" {
							m := monster.NewMonster3DFromConfig(x, y, "wolf", g.config)
							if tc.extra == "pack" {
								m.PackKey = "other_pack"
							} else {
								m.SummonedBy = w.Monsters[0].ID
							}
							w.Monsters = append(w.Monsters, m)
						}
						save := g.buildSave(wm)
						if legacy {
							for i := range save.MapMonsters["pack_test"] {
								save.MapMonsters["pack_test"][i].SpawnPosition = nil
							}
						}
						// Re-save and reload after migration, then switch phase again.
						// Slot reconciliation must not alter saved AI home positions.
						for round := 0; round < 2; round++ {
							data, err := json.Marshal(save)
							if err != nil {
								t.Fatal(err)
							}
							if err := json.Unmarshal(data, &save); err != nil {
								t.Fatal(err)
							}
							g.restoreSavedMonsters(wm, &save)
							homes := make(map[string][2]float64)
							for _, m := range w.Monsters {
								homes[m.ID] = [2]float64{m.SpawnX, m.SpawnY}
							}
							g.syncDayNightPacks(night)
							if active {
								(&GameLoop{game: g}).removeDeadMonstersByID()
							}
							if got := len(livePhasePack(w, night)); got != tc.want {
								t.Fatalf("round %d: spawned %d, want %d", round, got, tc.want)
							}
							for _, m := range w.Monsters {
								if home, exists := homes[m.ID]; exists && home != [2]float64{m.SpawnX, m.SpawnY} {
									t.Fatal("slot reconciliation changed an AI home")
								}
							}
							save = g.buildSave(wm)
							night = !night
						}
					})
				}
			}
		}
	}
}
