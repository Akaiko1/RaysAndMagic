package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func fishTestGame(t *testing.T, region string) (*MMGame, *world.WorldManager, float64) {
	t.Helper()
	g, wm, tile := ecologyTestGame(t)
	wm.CurrentMapKey = region
	wm.LoadedMaps = map[string]*world.World3D{region: g.world}
	wm.MapConfigs = map[string]*config.MapConfig{region: {Biome: region}}
	g.gameLoop = &GameLoop{game: g}
	g.world.Monsters = nil
	g.world.NPCs = nil
	placePlayerAtTile(g, 9, 10, tile)
	water, _ := world.GlobalTileManager.GetTileTypeFromKey("forest_stream")
	g.world.Tiles[10][12], g.world.Tiles[10][13] = water, water
	oldFish := config.GlobalEcology.Fish
	copyFish := *oldFish
	copyFish.SpawnChancePerTile = 0
	config.GlobalEcology.Fish = &copyFish
	t.Cleanup(func() { config.GlobalEcology.Fish = oldFish })
	return g, wm, tile
}

func TestFishLifecycle(t *testing.T) {
	for _, species := range []struct{ region, key, scale string }{{"forest", "common_carp", "Carp Scale"}, {"sakura_garden", "koi", "Koi Scale"}, {"highlands", "rainbow_salmon", "Rainbow Salmon Scale"}} {
		for _, tb := range []bool{false, true} {
			for _, outcome := range []string{"water", "beach", "arrow", "melee"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", species.key, tb, outcome), func(t *testing.T) {
					g, _, tile := fishTestGame(t, species.region)
					g.turnBasedMode = tb
					roll := 1.0
					if outcome == "beach" {
						roll = 0
					}
					m := g.spawnLeapingFish(species.region, species.key, [2]int{12, 10}, config.GlobalEcology.Fish, roll)
					if m == nil || m.FishLeap == nil || m.CurrentAIBehavior() != monster.AIBehaviorInert || m.TargetsParty() {
						t.Fatal("fish did not start inert flight")
					}
					if math.Abs(m.FishLeap.ToX-m.X)+math.Abs(m.FishLeap.ToY-m.Y) != tile {
						t.Fatal("not a cardinal one-tile jump")
					}
					for i := 0; i < g.config.GetTPS()/2; i++ {
						g.frameCount++
						g.updateFish()
					}
					if m.VisualHeightTiles() <= 0 || m.X == m.FishLeap.FromX && m.Y == m.FishLeap.FromY {
						t.Fatal("flight did not advance")
					}
					if outcome == "arrow" {
						shot := &Arrow{ID: "fish-shot", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: g.party.Members[0], BowKey: "elven_bow", Damage: 100, DamageType: "physical"}
						shot.X, shot.Y = m.X, m.Y
						g.arrows = []Arrow{*shot}
						g.camera.Angle = math.Atan2(m.Y-g.camera.Y, m.X-g.camera.X)
						g.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, m.X, m.Y, 8, 8, collision.CollisionTypeProjectile, false))
						g.combat.CheckProjectileMonsterCollisions()
					} else if outcome == "melee" {
						g.combat.ApplyDamageToMonster(m, 100, "Iron Sword", false)
					}
					if outcome == "arrow" || outcome == "melee" {
						if m.IsAlive() {
							t.Fatal("airborne fish survived lethal party hit")
						}
						g.combat.finishMonsterKill(m) // repeated finalization must not duplicate loot
						g.gameLoop.removeDeadMonstersByID()
					}
					for i := 0; i < g.config.GetTPS()*3; i++ {
						g.frameCount++
						g.updateFish()
					}
					if len(g.world.Monsters) != 0 || g.collisionSystem.GetEntityByID(m.ID) != nil {
						t.Fatal("completed flight kept actor/collision")
					}
					want := 1
					if outcome == "water" {
						want = 0
					}
					if len(g.groundContainers) != want {
						t.Fatalf("drops=%d want %d", len(g.groundContainers), want)
					}
					if want == 1 {
						bag := g.groundContainers[0]
						if len(bag.Items) != 1 || bag.Items[0].Name != species.scale || bag.Gold != 0 || !g.world.CanMoveTo(bag.X, bag.Y) {
							t.Fatalf("wrong/unreachable scale: %+v", bag)
						}
					}
				})
			}
		}
	}
}

func TestFishWaterEligibility(t *testing.T) {
	for _, key := range []string{"water", "deep_water", "forest_stream", "highlands_stream"} {
		for _, shape := range []string{"isolated", "diagonal", "pair", "blocked_bank", "boundary"} {
			t.Run(key+"/"+shape, func(t *testing.T) {
				g, _, _ := fishTestGame(t, "forest")
				water, _ := world.GlobalTileManager.GetTileTypeFromKey(key)
				g.world.Tiles[10][12] = water
				g.world.Tiles[10][13] = world.TileEmpty
				if shape == "diagonal" {
					g.world.Tiles[11][13] = water
				}
				if shape == "pair" || shape == "blocked_bank" {
					g.world.Tiles[10][13] = water
				}
				if shape == "blocked_bank" {
					for _, p := range [][2]int{{11, 10}, {12, 9}, {12, 11}} {
						g.world.Tiles[p[1]][p[0]] = world.TileWall
					}
				}
				xy := [2]int{12, 10}
				if shape == "boundary" {
					xy = [2]int{0, 0}
					g.world.Tiles[0][0] = water
				}
				m := g.spawnLeapingFish("forest", "common_carp", xy, config.GlobalEcology.Fish, 0)
				want := shape == "pair" || shape == "blocked_bank"
				if (m != nil) != want {
					t.Fatalf("spawn=%v want %v", m != nil, want)
				}
				if m != nil && shape == "blocked_bank" && m.FishLeap.Beached {
					t.Fatal("beached through blocker")
				}
			})
		}
	}
}

func TestFishScheduleAndSave(t *testing.T) {
	g, wm, _ := fishTestGame(t, "forest")
	f := config.GlobalEcology.Fish
	f.SpawnChancePerTile = 1
	for i := 0; i < f.RollEveryFrames-1; i++ {
		g.frameCount++
		g.updateFish()
	}
	if len(g.world.Monsters) != 0 {
		t.Fatal("rolled before cadence boundary")
	}
	sources := len(g.fishSources("forest", f.RadiusTiles))
	g.frameCount++
	g.updateFish()
	if sources < 2 || len(g.world.Monsters) != sources {
		t.Fatal("did not roll independently for every water tile")
	}
	for i := 0; i < f.RollEveryFrames; i++ {
		g.frameCount++
		g.updateFish()
	}
	if len(g.world.Monsters) != 2*sources {
		t.Fatal("active flights suppressed the next roll")
	}
	for i := 0; i < 20; i++ {
		g.frameCount++
		g.updateFish()
	}
	save := g.buildSave(wm)
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("fish_roll_frames")) {
		t.Fatal("visual fish cadence written to save")
	}
	var restored GameSave
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Monsters) != 0 {
		t.Fatal("transient fish written to save")
	}
	for _, roster := range restored.MapMonsters {
		for _, m := range roster {
			if m.Key == "common_carp" {
				t.Fatal("transient fish written to map save")
			}
		}
	}
	before := g.frameCount
	if err = g.applySave(wm, &restored); err != nil {
		t.Fatal(err)
	}
	if len(g.world.Monsters) != 0 || len(g.fishWorlds) != 0 || g.frameCount != before {
		t.Fatal("load changed world clock or restored a fish")
	}
	g.frameCount++
	g.updateFish()
	if len(g.world.Monsters) != 0 {
		t.Fatal("reload spawned an immediate replacement")
	}
}

func TestFishSpecialMotionPrewarm(t *testing.T) {
	loadTestConfig(t)
	for _, key := range []string{"common_carp", "koi", "rainbow_salmon"} {
		requests := mapRenderSourceRequests(mapRenderPrewarmPlan{monsterDecode: []mapMonsterPrewarmResource{{key: key, spriteName: key}}})
		found := false
		for _, r := range requests {
			if r == (graphics.SpriteResourceRequest{Name: key, AnimationType: "leaping_r"}) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s leap missing from prewarm", key)
		}
	}
}

func TestFishTravelAndReachableLoot(t *testing.T) {
	for _, beached := range []bool{false, true} {
		t.Run(fmt.Sprintf("travel/beached=%v", beached), func(t *testing.T) {
			g, wm, _ := fishTestGame(t, "forest")
			roll := 1.0
			if beached {
				roll = 0
			}
			g.spawnLeapingFish("forest", "common_carp", [2]int{12, 10}, config.GlobalEcology.Fish, roll)
			old := g.world
			g.world = newTestWorldSized(g.config, 30, 30)
			wm.LoadedMaps["other"], wm.CurrentMapKey = g.world, "other"
			g.frameCount++
			g.updateFish()
			if len(old.Monsters) != 0 || len(g.groundContainers) != 0 {
				t.Fatal("travel retained fish or awarded remote loot")
			}
		})
	}
	g, _, tile := fishTestGame(t, "forest")
	deep, _ := world.GlobalTileManager.GetTileTypeFromKey("deep_water")
	g.world.Tiles[10][12], g.world.Tiles[10][13] = deep, deep
	m := g.spawnLeapingFish("forest", "common_carp", [2]int{12, 10}, config.GlobalEcology.Fish, 1)
	m.AdvanceFishLeap(.9)
	g.combat.ApplyDamageToMonster(m, 100, "Iron Sword", false)
	if len(g.groundContainers) != 1 {
		t.Fatal("missing deep-water scale")
	}
	bag := g.groundContainers[0]
	if !g.world.CanMoveTo(bag.X, bag.Y) || Distance(bag.X, bag.Y, m.X, m.Y) > 2*tile {
		t.Fatal("scale not on nearby reachable ground")
	}
}

func TestFishAuthoredWater(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = oldTM, oldWM })
	world.GlobalTileManager = world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, merged := range []bool{false, true} {
		wm := world.NewWorldManager(cfg)
		if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
			t.Fatal(err)
		}
		if merged {
			wm.SetOpenWorldConfig(config.MustLoadOpenWorldConfig("assets/open_world.yaml"))
		}
		if err := wm.LoadAllMaps(); err != nil {
			t.Fatal(err)
		}
		world.GlobalWorldManager = wm
		for _, key := range []string{"forest", "sakura_garden", "highlands"} {
			if err := wm.SwitchToMap(key); err != nil {
				t.Fatal(err)
			}
			g := &MMGame{world: ecologyWorld(key), config: cfg}
			count := 0
			for y := 0; y < g.world.Height; y++ {
				for x := 0; x < g.world.Width; x++ {
					if g.fishRegionContains(key, x, y) && fishWater(g.world, x, y) && len(g.fishDestinations(key, x, y, true)) > 0 {
						count++
					}
				}
			}
			if count == 0 {
				t.Errorf("merged=%v %s has no connected fish water", merged, key)
			}
			t.Logf("merged=%v %s: %d water spawn cells", merged, key, count)
		}
	}
}

func TestFishPauseSharedWorldClock(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) { testFishPauseSharedWorldClock(t, tb) })
	}
}

func testFishPauseSharedWorldClock(t *testing.T, turnBased bool) {
	t.Chdir("../..")
	old := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = old })
	if err := config.LoadEcology("assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	g, _, _ := bootOpenWorldGame(t, true)
	g.turnBasedMode = turnBased
	g.frameCount = 100
	m := monster.NewMonster3DFromConfig(g.camera.X+128, g.camera.Y, "common_carp", g.config)
	m.FishLeap = &monster.FishLeapState{FromX: m.X, FromY: m.Y, ToX: m.X + 64, ToY: m.Y, Duration: 1.8, PeakHeight: .7}
	g.registerSpawnedMonster(m)
	g.gameVictory = true
	for i := 0; i < 20; i++ {
		g.gameLoop.ui.renderedModalSnapshot = g.gameLoop.ui.topModalSnapshot()
		g.gameLoop.updateExploration()
	}
	if m.FishLeap.Progress != 0 || g.frameCount != 100 {
		t.Fatal("fish advanced behind paused overlay")
	}
	g.gameVictory = false
	for i := 0; i < 10; i++ {
		g.gameLoop.ui.renderedModalSnapshot = g.gameLoop.ui.topModalSnapshot()
		g.gameLoop.updateExploration()
	}
	if m.FishLeap.Progress <= 0 {
		t.Fatal("fish did not resume with the world")
	}
}
