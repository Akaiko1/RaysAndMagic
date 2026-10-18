package game

import (
	"fmt"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/status"
	"ugataima/internal/storage"
)

func TestEnvironmentContinuousOverlap(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, overlap := range []bool{false, true} {
			t.Run(fmt.Sprintf("tb=%v/overlap=%v", tb, overlap), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				g.combat = NewCombatSystem(g)
				g.turnBasedMode = tb
				field := config.EnvironmentEffect{ID: "lava", Kind: "occupation", Name: "Lava", School: "fire", Damage: 12, Rect: [4]int{6, 7, 6, 7}}
				a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{field}}
				wm.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
				g.setPartyPosition(6.5*ts, 7.5*ts)
				hero := g.party.Members[0]
				hero.MaxHitPoints, hero.HitPoints = 10000, 10000
				g.damageEnvironment(field, 1)
				one := 10000 - hero.HitPoints
				hero.HitPoints = 10000
				if overlap {
					a.Effects = append(a.Effects, config.EnvironmentEffect{ID: "lane", Kind: "lane", Name: "Flame Lane", School: "fire", Damage: 45, SustainDamage: 12, Rect: field.Rect, DurationSeconds: 6})
					g.adventureVisit("forest").effectState("lane").Remaining = roundClock()
				}
				if tb {
					tickEnvironmentFor(g, 3, true)
				} else {
					tickEnvironmentFor(g, .25, false)
					if hero.HitPoints != 10000 {
						t.Fatal("partial second charged early")
					}
					g.adventure = cloneAdventureState(g.adventure)
					for range 11 {
						tickEnvironmentFor(g, .25, false)
					}
				}
				if got := 10000 - hero.HitPoints; got != 3*one {
					t.Fatalf("continuous damage=%d want %d", got, 3*one)
				}
			})
		}
	}
}

func TestSolsticeAuthoredRoutesAndRewards(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, _ := bootOpenWorldGame(t, false)
	if _, err := config.LoadAlchemySpawns("assets/alchemy_spawns.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"solstice_fire", "solstice_water", "solstice_earth", "solstice_air"} {
		if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
			t.Fatal(err)
		}
		a := g.adventureConfig(key)
		if len(a.JumpLinks) != 3 {
			t.Fatalf("%s Jump links", key)
		}
		startX, startY := g.GetPlayerTilePosition()
		job := &harvestSearch{region: key, start: [2]int{startX, startY}, queue: [][2]int{{startX, startY}}, seen: map[[2]int]bool{{startX, startY}: true}, occupied: g.harvestOccupied(), buildings: g.buildingOccupiedTiles()}
		for !g.advanceHarvestSearch(job) {
		}
		boss := g.adventureBoss(a)
		ts := float64(g.config.GetTileSize())
		bossTile := [2]int{TileIndex(boss.X, ts), TileIndex(boss.Y, ts)}
		if !job.seen[bossTile] {
			t.Fatalf("%s population topology cannot reach boss at %v", key, bossTile)
		}
		for _, link := range a.JumpLinks {
			for _, p := range [][2]int{{link[0], link[1]}, {link[2], link[3]}} {
				if !job.seen[p] {
					t.Fatalf("%s cannot reach Jump endpoint %v", key, p)
				}
				if g.harvestPlacementAllowed(key, p[0], p[1]) {
					t.Fatalf("resource on Jump endpoint %v", p)
				}
			}
		}
		loot, gold := rollWeightedLootTable(key + "_archive")
		if len(loot) != 3 || gold < 150 || gold > 250 {
			t.Fatalf("%s archive supplies/gear/gold: %v %d", key, loot, gold)
		}
	}
	if err := g.transitionToMap(mapTransition{mapKey: "solstice_air", arrival: mapArrivalEntrance}); err != nil {
		t.Fatal(err)
	}
	a := g.adventureConfig("solstice_air")
	v := g.adventureVisit("solstice_air")
	if g.effectEnabled(a, v, *a.Effect("transfer_jet")) {
		t.Fatal("gallery brake starts open")
	}
	g.useAdventureControl("gallery_vane")
	if !g.effectEnabled(a, v, *a.Effect("transfer_jet")) {
		t.Fatal("gallery vane does nothing")
	}
	v.effectState("regulator_lane_0").Warning = roundClock()
	if g.effectEnabled(a, v, *a.Effect("arena_jet_0")) {
		t.Fatal("escape vane opened before hub control")
	}
	g.useAdventureControl("hub_vane")
	if !g.effectEnabled(a, v, *a.Effect("arena_jet_0")) || !g.effectEnabled(a, v, *a.Effect("return_jet")) {
		t.Fatal("hub vane failed to secure arena and return route")
	}
}

func TestAdventureBossPatternIsAtomic(t *testing.T) {
	g, wm, _ := travelFixture(t)
	g.combat = NewCombatSystem(g)
	a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{{ID: "a", Kind: "lane", WarningSeconds: 1, WarningRounds: 1}, {ID: "b", Kind: "lane", WarningSeconds: 1, WarningRounds: 1}}, Boss: &config.AdventureBoss{Monster: "boss", CooldownSeconds: 6, Phases: []config.AdventureBossPhase{{BelowPercent: 25, Patterns: [][]string{{"a", "b"}}}}}}
	wm.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
	m := &monster.Monster3D{Key: "boss", HomeMap: "forest", HitPoints: 25, MaxHitPoints: 100}
	v := g.adventureVisit("forest")
	v.effectState("b").Remaining = roundClock()
	if g.combat.runChamberBossFreeActions(m); v.effectState("a").Warning.Active() {
		t.Fatal("half a pattern armed")
	}
	if !m.LaneCD.Active() {
		t.Fatal("a pattern that could not fire kept the lane cooldown ready")
	}
	v.effectState("b").Remaining = status.DualClock{}
	m.LaneCD = status.DualClock{}
	if g.combat.runChamberBossFreeActions(m); !v.effectState("a").Warning.Active() || !v.effectState("b").Warning.Active() {
		t.Fatal("pair did not arm together")
	}
}

func TestAuthoredShortcutOverridesReturnPose(t *testing.T) {
	g, _, ts := travelFixture(t)
	g.mapReturnPoses = map[string]MapPose{"forest": {X: 2.5 * ts, Y: 2.5 * ts}}
	target := [2]int{6, 7}
	if err := g.transitionToMap(mapTransition{mapKey: "forest", arrival: mapArrivalEntrance, arrivalTile: &target}); err != nil {
		t.Fatal(err)
	}
	x, y := g.GetPlayerTilePosition()
	if x != 6 || y != 7 {
		t.Fatalf("shortcut landed %d,%d", x, y)
	}
	bad := [2]int{-1, 7}
	if err := g.transitionToMap(mapTransition{mapKey: "forest", arrival: mapArrivalEntrance, arrivalTile: &bad}); err == nil {
		t.Fatal("bad arrival accepted")
	}
}

func TestAnnouncedTransferReleasesStandingParty(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			g, wm, ts := travelFixture(t)
			g.combat = NewCombatSystem(g)
			g.turnBasedMode = tb
			g.setPartyPosition(6.5*ts, 7.5*ts)
			g.camera.Angle = .42
			a := &config.AdventureConfig{Effects: []config.EnvironmentEffect{
				{ID: "lane", Kind: "lane", Name: "Lightning", Rect: [4]int{6, 6, 8, 6}, School: "air", Damage: 45, WarningSeconds: 1.2, WarningRounds: 1, DurationSeconds: 6},
				{ID: "jet", Kind: "transfer", Name: "Jet", Rect: [4]int{6, 7, 6, 7}, Destination: [2]int{8, 7}, TriggerLane: "lane"},
			}}
			wm.MapConfigs["forest"] = &config.MapConfig{Adventure: a}
			g.adventureVisit("forest").effectState("lane").Warning = status.PairClock(1.2, 1, g.config.GetTPS(), 1)
			g.applyEnvironmentArrival()
			if x, _ := g.GetPlayerTilePosition(); x != 6 {
				t.Fatal("jet fired before warning")
			}
			if tb {
				tickEnvironmentFor(g, 3, true)
			} else {
				tickEnvironmentFor(g, .6, false)
				if x, _ := g.GetPlayerTilePosition(); x != 6 {
					t.Fatal("partial warning teleported")
				}
				g.adventure = cloneAdventureState(g.adventure)
				tickEnvironmentFor(g, .6, false)
			}
			if x, y := g.GetPlayerTilePosition(); x != 8 || y != 7 || g.camera.Angle != .42 {
				t.Fatalf("receiving pose %d,%d angle %v", x, y, g.camera.Angle)
			}
			tickEnvironmentFor(g, 1, false)
			if x, _ := g.GetPlayerTilePosition(); x != 8 {
				t.Fatal("transfer repeated")
			}
		})
	}
}
