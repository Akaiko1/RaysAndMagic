package game

import (
	"encoding/json"
	"slices"
	"testing"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func ecologyTestGame(t *testing.T) (*MMGame, *world.WorldManager, float64) {
	t.Helper()
	g, _, tile := tbBehaviorGame(t, 30, 30)
	g.turnBasedMode = false
	prevTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = prevTiles })
	world.GlobalTileManager = world.NewTileManager(g.config.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	old := config.GlobalEcology
	t.Cleanup(func() { config.GlobalEcology = old })
	if err := config.LoadEcology("../../assets/ecology.yaml"); err != nil {
		t.Fatal(err)
	}
	oldLoots := config.GlobalLoots
	t.Cleanup(func() { config.GlobalLoots = oldLoots })
	config.MustLoadLootTables("../../assets/loots.yaml")
	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps = map[string]*world.World3D{"desert": g.world}
	wm.CurrentMapKey = "desert"
	wm.MapConfigs = map[string]*config.MapConfig{"desert": {Biome: "desert"}}
	setTestWorldManager(t, wm)
	config.GlobalEcology.Caravan.Routes = []config.CaravanRoute{{ID: "test", Points: []config.RoutePoint{{Map: "desert", X: 20, Y: 20}, {Map: "desert", X: 23, Y: 20}, {Map: "desert", X: 23, Y: 23}}}}
	g.world.NPCs = []*character.NPC{{Key: config.GlobalEcology.Caravan.Merchant}}
	g.calendarDay = 1
	g.reusableDeadSet = map[string]bool{}
	g.reusableEncounterRewardsMap = map[*monster.EncounterRewards]int{}
	return g, wm, tile
}
func TestWildlifePopulationLifecycle(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "remote"}[remote], func(t *testing.T) {
			g, wm, _ := ecologyTestGame(t)
			var day, night config.WildlifePopulation
			for _, p := range config.GlobalEcology.Populations {
				if p.Map == wm.CurrentMapKey && p.Phase == "day" && day.Monster == "" {
					day = p
				}
				if p.Map == wm.CurrentMapKey && p.Phase == "night" && night.Monster == "" {
					night = p
				}
			}
			if day.Monster == "" || night.Monster == "" {
				t.Fatalf("fixture needs a day and a night population on %q", wm.CurrentMapKey)
			}
			w := g.world
			if remote {
				g.world = newTestWorld(g.config)
				wm.LoadedMaps["other"] = g.world
				wm.CurrentMapKey = "other"
			}
			count := func(key string) int {
				n := 0
				for _, m := range w.Monsters {
					if m.IsAlive() && m.Key == key {
						n++
					}
				}
				return n
			}
			g.replenishWildlife()
			if count(day.Monster) != day.Count {
				t.Fatal("day cap")
			}
			for _, m := range w.Monsters {
				if m.Key == day.Monster {
					m.HitPoints = 0
					break
				}
			}
			g.replenishWildlife()
			if count(day.Monster) != day.Count-1 {
				t.Fatal("same-phase replacement")
			}
			data, _ := json.Marshal(g.ecology)
			g.ecology = EcologyState{}
			if err := json.Unmarshal(data, &g.ecology); err != nil {
				t.Fatal(err)
			}
			g.replenishWildlife()
			if count(day.Monster) != day.Count-1 {
				t.Fatal("load replaced wildlife")
			}
			g.dayNightDay++
			g.dayNightIsNight = true
			g.replenishWildlife()
			if count(day.Monster) != day.Count-1 || count(night.Monster) != night.Count {
				t.Fatal("night removed survivors or wrong cap")
			}
			g.dayNightDay++
			g.dayNightIsNight = false
			g.replenishWildlife()
			if count(day.Monster) != day.Count || count(night.Monster) != night.Count {
				t.Fatal("dawn did not retain/refill")
			}
			if worldHasLivingMonstersInRect(w, 0, 0, w.Width, w.Height, g.config.GetTileSize()) {
				t.Fatal("wildlife blocks map clear")
			}
		})
	}
}
func TestApproachDawnPopulation(t *testing.T) {
	for _, mode := range []string{"local", "remote", "stitched"} {
		t.Run(mode, func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			var p config.WildlifePopulation
			for _, candidate := range config.GlobalEcology.Populations {
				if candidate.Map == "solstice_approach" {
					p = candidate
					break
				}
			}
			if !p.Hostile || len(p.Species()) != 3 {
				t.Fatal("missing mixed hostile population")
			}
			config.GlobalEcology.Populations = []config.WildlifePopulation{p}
			if mode == "stitched" {
				t.Chdir("../..")
				if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
					t.Fatal(err)
				}
				if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
					t.Fatal(err)
				}
				wm.MapConfigs = map[string]*config.MapConfig{p.Map: wm.MapConfigs[p.Map], "forest": wm.MapConfigs["forest"]}
				wm.SetOpenWorldConfig(&config.OpenWorldConfig{VoidTile: "oob_cliff", Placements: map[string]config.OpenWorldPlacement{p.Map: {X: 5, Y: 5}, "forest": {X: 120, Y: 5}}})
				if err := wm.LoadAllMaps(); err != nil {
					t.Fatal(err)
				}
				if !wm.IsOpenWorldRegion(p.Map) {
					t.Fatal("fixture did not build a stitched region")
				}
				g.world = wm.OpenWorld
				g.collisionSystem = collision.NewCollisionSystem(g.world, tile)
			}
			w := g.world
			wm.LoadedMaps = map[string]*world.World3D{p.Map: w}
			wm.CurrentMapKey = p.Map
			w.Monsters = nil
			// Authored survivors count toward the cap but unrelated golems do not.
			for i := 0; i < 2; i++ {
				m := monster.NewMonster3DFromConfig((12.5+float64(i))*tile, 12.5*tile, p.Species()[i], g.config)
				m.HomeMap = p.Map
				w.Monsters = append(w.Monsters, m)
			}
			golem := monster.NewMonster3DFromConfig(15.5*tile, 12.5*tile, "solstice_rootbound_sentinel", g.config)
			golem.HomeMap = p.Map
			w.Monsters = append(w.Monsters, golem)
			if mode == "remote" {
				g.world = newTestWorld(g.config)
				wm.LoadedMaps["elsewhere"] = g.world
				wm.CurrentMapKey = "elsewhere"
			}
			count := func() int {
				n := 0
				for _, m := range w.Monsters {
					if m == golem {
						continue
					}
					if m.IsAlive() {
						if mode == "stitched" && m.Population == p.Identity() {
							r := wm.OpenWorldRegionByKey(p.Map)
							if m.X < float64(r.OffsetX)*tile || m.Y < float64(r.OffsetY)*tile || m.X >= float64(r.OffsetX+r.Width)*tile || m.Y >= float64(r.OffsetY+r.Height)*tile {
								t.Fatal("spawn escaped its stitched region")
							}
						}
						n++
						if !slices.Contains(p.Species(), m.Key) || m.IsAmbient() {
							t.Fatalf("invalid replenishment: %s", m.Key)
						}
					}
				}
				return n
			}
			g.dayNightIsNight = true
			g.replenishWildlife()
			if count() != 2 {
				t.Fatal("spawned at night")
			}
			g.dayNightDay++
			g.dayNightIsNight = false
			g.replenishWildlife()
			if count() != p.Count {
				t.Fatalf("dawn count=%d want %d", count(), p.Count)
			}
			w.Monsters[len(w.Monsters)-1].HitPoints = 0
			golem.HitPoints = 0
			data, err := json.Marshal(g.ecology)
			if err != nil {
				t.Fatal(err)
			}
			g.ecology = EcologyState{}
			if err = json.Unmarshal(data, &g.ecology); err != nil {
				t.Fatal(err)
			}
			g.replenishWildlife()
			if count() != p.Count-1 {
				t.Fatal("load refilled in the same morning")
			}
			g.dayNightDay++
			g.replenishWildlife()
			if count() != p.Count {
				t.Fatal("next dawn did not replenish")
			}
		})
	}
}

func TestEcologyRelationshipsAndKillCredit(t *testing.T) {
	for _, mode := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[mode], func(t *testing.T) {
			g, _, tile := ecologyTestGame(t)
			g.turnBasedMode = mode
			rabbit := monster.NewMonster3DFromConfig(15.5*tile, 15.5*tile, "desert_rabbit", g.config)
			fox := monster.NewMonster3DFromConfig(16.5*tile, 15.5*tile, "fennec", g.config)
			caravan := monster.NewMonster3DFromConfig(20.5*tile, 20.5*tile, "desert_caravan", g.config)
			g.addEcologyActor(g.world, rabbit)
			g.addEcologyActor(g.world, fox)
			g.addEcologyActor(g.world, caravan)
			placePlayerAtTile(g, 1, 1, tile)
			g.refreshMonsterAIState()
			if fox.AIFoe != rabbit || !rabbit.AmbientFlee {
				t.Fatal("predation targeting")
			}
			if rabbit.TargetsParty() || fox.TargetsParty() || caravan.TargetsParty() {
				t.Fatal("ambient targets party")
			}
			rabbit.HitPoints = 1
			fox.DamageMin = 100
			fox.DamageMax = 100
			if !g.combat.commitMonsterAttack(fox, monsterAttackDestination{foe: rabbit}, map[bool]monsterAttackCadence{false: monsterAttackRealtime, true: monsterAttackTurn}[mode]) {
				t.Fatal("predator could not attack")
			}
			if rabbit.IsAlive() || !rabbit.NoKillRewards || len(g.groundContainers) > 0 {
				t.Fatalf("predation HP=%d suppressed=%v bags=%d log=%v", rabbit.HitPoints, rabbit.NoKillRewards, len(g.groundContainers), g.combatLogHistory)
			}
			placePlayerAtTile(g, 17, 15, tile)
			fox.WasAttacked = true
			g.refreshMonsterAIState()
			if !fox.AmbientFlee || fox.AIFoe != nil || fox.TargetsParty() {
				t.Fatal("party flee must override hunting and retaliation")
			}
		})
	}
}

// Populations spawn only on bare ground (tile type floor). Walkable grass,
// props, water and markers can sit outside a region's walls, so none of them
// may take a spawn even when nothing else is free.
func TestWildlifeSpawnsOnlyOnBareFloor(t *testing.T) {
	catalog := world.NewTileManager(testTileSizeClasses())
	if err := catalog.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	keys := catalog.GetAllTileKeys()
	slices.Sort(keys)
	fillers := []string{catalog.GetTileKey(world.TileEmpty)}
	seen := map[string]bool{"floor": true}
	for _, k := range keys {
		if d := catalog.GetTileDataByKey(k); d.Walkable && !seen[d.Type] {
			seen[d.Type] = true
			fillers = append(fillers, k)
		}
	}
	if len(fillers) < 3 {
		t.Fatalf("catalog needs walkable non-floor tiles, got %v", fillers)
	}
	bare := [][2]int{{20, 20}, {25, 8}} // far from the party at (1,1)
	for _, filler := range fillers {
		t.Run(filler, func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			var pop config.WildlifePopulation
			for _, p := range config.GlobalEcology.Populations {
				if p.Map == wm.CurrentMapKey && p.Phase == "day" {
					pop = p
					break
				}
			}
			if pop.Count <= len(bare) {
				t.Fatalf("fixture needs a day population above %d on %q", len(bare), wm.CurrentMapKey)
			}
			tm := world.GlobalTileManager
			fill, _ := tm.GetTileTypeFromKey(filler)
			for y := range g.world.Tiles {
				for x := range g.world.Tiles[y] {
					g.world.Tiles[y][x] = fill
				}
			}
			for _, xy := range bare {
				g.world.Tiles[xy[1]][xy[0]] = world.TileEmpty
			}
			g.world.Monsters = nil
			g.replenishWildlife()
			want := len(bare)
			if tm.IsBareFloor(fill) {
				want = pop.Count // control: plentiful bare ground fills the cap
			}
			n := 0
			for _, m := range g.world.Monsters {
				if m.Population != pop.Identity() {
					continue
				}
				n++
				if !tm.IsBareFloor(g.world.Tiles[int(m.Y/tile)][int(m.X/tile)]) {
					t.Fatalf("%s spawned on %s", m.Key, tm.GetTileKey(g.world.Tiles[int(m.Y/tile)][int(m.X/tile)]))
				}
			}
			if n != want {
				t.Fatalf("spawned %d, want %d", n, want)
			}
		})
	}
}
func TestCaravanTripRewardsAndCapacity(t *testing.T) {
	g, _, tile := ecologyTestGame(t)
	// This fixture tests delivery, not a random rabbit occupying the spawn tile.
	config.GlobalEcology.Populations = nil
	g.ecology.Unlocked = true
	g.updateEcology()
	_, m := g.ecologyActor()
	if m == nil {
		t.Fatal("no caravan")
	}
	g.ecology.StopFrames = 0
	for i := 0; i < 4; i++ {
		r := g.caravanRoute()
		p := r.Points[g.ecology.Checkpoint]
		m.X, m.Y = (float64(p.X)+.5)*tile, (float64(p.Y)+.5)*tile
		g.ecology.StopFrames = 0
		g.updateEcology()
	}
	total := 0
	for _, n := range g.ecology.Stock {
		total += n
	}
	caravan := config.GlobalEcology.Caravan
	if total != caravan.RewardUnits || g.ecology.Deliveries != 1 {
		t.Fatalf("delivery total=%d trips=%d, want %d units in 1 trip", total, g.ecology.Deliveries, caravan.RewardUnits)
	}
	pool := config.CaravanTradePool()
	if len(pool) <= caravan.StockSlots {
		t.Fatalf("trade pool has %d goods, need more than %d shelf slots to test overflow", len(pool), caravan.StockSlots)
	}
	g.ecology.Stock = map[string]int{}
	for _, k := range pool[:caravan.StockSlots] {
		g.ecology.Stock[k] = 1
	}
	before := cloneEcologyState(g.ecology)
	for i := 0; i < 100; i++ {
		g.depositCaravanGoods()
	}
	if len(g.ecology.Stock) != caravan.StockSlots {
		t.Fatal("slot overflow")
	}
	for k := range g.ecology.Stock {
		if before.Stock[k] == 0 {
			t.Fatal("full shelf rolled a new type")
		}
	}
	g.dialogNPC = g.world.NPCs[0]
	entry := g.dialogNPC.MerchantStock[0]
	n := entry.Quantity
	g.party.Gold = 0
	if got := g.merchantMaxUnits(entry); got != n {
		t.Fatalf("free shelf needs gold: max %d want %d", got, n)
	}
	gold := g.party.Gold
	if !g.buyMerchantUnits(entry, n) || g.ecology.Stock[entry.RewardKey] != 0 || g.party.Gold != gold {
		t.Fatal("free collection failed")
	}
	if len(g.ecology.Stock) != caravan.StockSlots-1 {
		t.Fatal("empty stack must release slot")
	}
}

// Arrival is credited exactly when movement has finished the leg, so a stop
// short of the centre (reload, blocked last step) cannot stall the route.
func TestCaravanArrivalMatchesMovement(t *testing.T) {
	goal := monster.TileCoord{X: 23, Y: 20} // checkpoint 1 of the fixture route
	pending := []monster.TileCoord{goal}
	cases := []struct {
		name     string
		dx, dy   float64 // offset from the goal centre, in tiles
		path     []monster.TileCoord
		index    int
		move     bool // drive the real movement loop instead of one credit check
		advances bool
	}{
		{name: "centre, path pending", path: pending, advances: true},
		{name: "off centre, no path", dx: .3, dy: .3, advances: true},
		{name: "off centre, path finished", dx: .3, dy: .3, path: pending, index: 1, advances: true},
		{name: "off centre, path pending", dx: .3, dy: .3, path: pending, advances: false},
		{name: "next tile, no path", dx: -1, advances: false},
		{name: "reloaded off centre, real movement", dx: .1, dy: -.35, move: true, advances: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, _, tile := ecologyTestGame(t)
			config.GlobalEcology.Populations = nil
			g.ecology.Unlocked = true
			g.spawnCaravan()
			_, m := g.ecologyActor()
			if m == nil || g.ecology.Checkpoint != 1 {
				t.Fatal("fixture caravan must head for checkpoint 1")
			}
			cx, cy := (float64(goal.X)+.5)*tile, (float64(goal.Y)+.5)*tile
			m.X, m.Y = cx+tc.dx*tile, cy+tc.dy*tile
			m.PathTiles, m.PathIndex = tc.path, tc.index
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			placePlayerAtTile(g, 1, 1, tile)
			startX, startY := m.X, m.Y
			if tc.move {
				for tick := 0; tick < 30 && g.ecology.Checkpoint == 1; tick++ {
					g.prepareAmbientTarget(m)
					m.UpdateAmbient(g.collisionSystem, m.AITargetX, m.AITargetY, false)
					g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
					g.updateEcology()
				}
			} else {
				g.updateEcology()
			}
			if advanced := g.ecology.Checkpoint == 2; advanced != tc.advances {
				t.Fatalf("checkpoint %d at (%.1f, %.1f), want advance=%v", g.ecology.Checkpoint, m.X, m.Y, tc.advances)
			}
			if tc.advances && !tc.move && (m.X != cx || m.Y != cy) {
				t.Fatalf("credited caravan at (%.1f, %.1f), want snapped to (%.1f, %.1f)", m.X, m.Y, cx, cy)
			}
			if !tc.advances && (m.X != startX || m.Y != startY) {
				t.Fatal("a refused credit must not move the caravan")
			}
		})
	}
}
func TestCaravanDeathRespawnAndSave(t *testing.T) {
	g, wm, _ := ecologyTestGame(t)
	config.GlobalEcology.Populations = nil
	g.ecology.Unlocked = true
	g.updateEcology()
	_, m := g.ecologyActor()
	oldID := m.ID
	m.HitPoints = 0
	g.updateEcology()
	g.updateEcology()
	if g.ecology.ActorID != oldID {
		t.Fatal("respawn before dawn")
	}
	g.calendarDay++
	g.updateEcology()
	_, m = g.ecologyActor()
	if m == nil || !m.IsAlive() || m.ID == oldID {
		t.Fatal("no dawn replacement")
	}
	g.ecology.Stock = map[string]int{"clock_hand": 3}
	g.syncCaravanStock()
	save := g.buildSave(wm)
	g.ecology.Stock["clock_hand"] = 99
	if save.Ecology.Stock["clock_hand"] != 3 {
		t.Fatal("save aliases runtime stock")
	}
	data, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var loaded GameSave
	if err = json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if err = g.applySave(wm, &loaded); err != nil {
		t.Fatal(err)
	}
	_, restored := g.ecologyActor()
	if restored == nil || restored.ID != m.ID || g.ecology.Stock["clock_hand"] != 3 {
		t.Fatal("campaign ecology not restored")
	}
}

func TestEcologyRemoteMovementAndCombat(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[turn], func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			w := g.world
			g.ecology.Unlocked = true
			g.spawnCaravan()
			_, caravan := g.ecologyActor()
			start := caravan.X
			g.world = newTestWorldSized(g.config, 30, 30)
			wm.LoadedMaps["other"] = g.world
			wm.CurrentMapKey = "other"
			g.turnBasedMode = turn
			for i := 0; i < 20; i++ {
				g.frameCount++
				g.simulateRemoteEcology(turn, true)
			}
			if caravan.X <= start {
				t.Fatalf("remote caravan did not move: %.2f -> %.2f", start, caravan.X)
			}
			enemy := monster.NewMonster3DFromConfig(caravan.X+tile, caravan.Y, "bandit", g.config)
			enemy.ProjectileWeapon = ""
			enemy.RangedAttackRange = 0
			w.Monsters = append(w.Monsters, enemy)
			hp := caravan.HitPoints
			for i := 0; i < 100; i++ {
				g.frameCount++
				g.simulateRemoteEcology(turn, true)
			}
			if caravan.HitPoints >= hp {
				t.Fatalf("remote caravan immune to attacks: %d -> %d, foe=%v", hp, caravan.HitPoints, enemy.AIFoe)
			}
		})
	}
}

// Trade goods are priced, non-legendary trinkets that are not door keys and
// that some monster table or the boss loot drops (chance > 0); each listed once.
func TestCaravanEligibleGoods(t *testing.T) {
	ecologyTestGame(t)
	shipped := config.CaravanTradePool()
	if len(shipped) == 0 {
		t.Fatal("shipped loot tables yield no caravan trade goods")
	}

	oldItems, oldLoots := config.GlobalItems, config.GlobalLoots
	t.Cleanup(func() { config.GlobalItems, config.GlobalLoots = oldItems, oldLoots })
	trinket := func(rarity string, value, doorKey int) *config.ItemDefinitionConfig {
		return &config.ItemDefinitionConfig{Type: "trinket", Rarity: rarity, Value: value, DoorKey: doorKey}
	}
	config.GlobalItems = &config.ItemSystemConfig{Items: map[string]*config.ItemDefinitionConfig{
		"gear":       trinket("common", 5, 0),
		"boss_scale": trinket("rare", 9, 0),
		"relic":      trinket("legendary", 50, 0),
		"door_key":   trinket("common", 5, 1),
		"pebble":     trinket("common", 0, 0),
		"never":      trinket("common", 5, 0),
		"potion":     {Type: "consumable", Rarity: "common", Value: 5},
	}}
	item := func(key string, chance float64) config.LootEntry {
		return config.LootEntry{Type: "item", Key: key, Chance: chance}
	}
	config.GlobalLoots = &config.LootTablesConfig{
		Loots: map[string][]config.LootEntry{
			"mob_a": {item("gear", .5), item("relic", .1), item("door_key", .2), item("pebble", .3), item("potion", .4), item("never", 0)},
			"mob_b": {item("gear", .2), {Type: "weapon", Key: "gear", Chance: .3}, item("undefined", .5)},
		},
		BossLoot: []config.LootEntry{item("boss_scale", .1)},
	}
	if got, want := config.CaravanTradePool(), []string{"boss_scale", "gear"}; !slices.Equal(got, want) {
		t.Fatalf("trade pool = %v, want %v", got, want)
	}
}

func TestCaravanRangedAttackAndPassiveHostiles(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "visible", true: "offscreen"}[remote], func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			g.ecology.Unlocked = true
			g.spawnCaravan()
			w, caravan := g.ecologyActor()
			caravan.Speed = 0
			enemy := monster.NewMonster3DFromConfig(caravan.X+3*tile, caravan.Y, "bandit", g.config)
			enemy.RangedAttackRange = 5 * tile
			enemy.AlertRadius = 8 * tile
			enemy.PassiveUntilAttacked = true
			w.Monsters = append(w.Monsters, enemy)
			w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
			placePlayerAtTile(g, 1, 1, tile)
			g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
			t.Cleanup(g.threading.Shutdown)
			if remote {
				g.world = newTestWorldSized(g.config, 30, 30)
				wm.LoadedMaps["other"] = g.world
				wm.CurrentMapKey = "other"
			}
			hp := caravan.HitPoints
			for i := 0; i < 240; i++ {
				g.frameCount++
				if remote {
					g.simulateRemoteEcology(false, false)
					g.advanceRemoteEcologyProjectiles()
				} else {
					g.refreshMonsterAIState()
					g.combat.HandleMonsterInteractions()
					gl := &GameLoop{game: g}
					gl.updateProjectilesAndImpacts()
				}
			}
			if caravan.HitPoints >= hp {
				t.Fatalf("caravan took no ranged damage (HP %d); foe=%v", caravan.HitPoints, enemy.AIFoe)
			}
		})
	}
}

// Use actual movement, including paired map anchors, rather than placing the
// caravan on successive checkpoints. This catches route advancement deadlocks.
func TestCaravanWalksRoundTripAcrossWorlds(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[turn], func(t *testing.T) {
			g, wm, _ := ecologyTestGame(t)
			remote := newTestWorld(g.config)
			remote.Width, remote.Height = g.world.Width, g.world.Height
			remote.Tiles = g.world.Tiles
			wm.LoadedMaps["city"] = remote
			config.GlobalEcology.Populations = nil
			config.GlobalEcology.Caravan.StopSeconds = 0
			config.GlobalEcology.Caravan.Routes = []config.CaravanRoute{{ID: "crossing", Points: []config.RoutePoint{{Map: "desert", X: 20, Y: 20}, {Map: "desert", X: 23, Y: 20}, {Map: "city", X: 20, Y: 20}, {Map: "city", X: 23, Y: 20}}}}
			g.world = newTestWorld(g.config)
			wm.LoadedMaps["party"] = g.world
			wm.CurrentMapKey = "party"
			g.ecology.Unlocked = true
			g.turnBasedMode = turn
			g.currentTurn = 1
			for i := 0; i < 8000 && g.ecology.Deliveries == 0; i++ {
				g.frameCount++
				g.updateEcology()
				g.simulateRemoteEcology(turn, true)
			}
			if g.ecology.Deliveries != 1 {
				w, m := g.ecologyActor()
				t.Fatalf("trip stalled: state=%+v world=%p actor=%+v", g.ecology, w, m)
			}
			if len(g.ecology.Stock) == 0 {
				t.Fatal("round trip did not deliver")
			}
		})
	}
}

func TestEcologyOffscreenPartyKillKeepsLootAndMap(t *testing.T) {
	g, wm, tile := ecologyTestGame(t)
	w := g.world
	rabbit := monster.NewMonster3DFromConfig(20.5*tile, 20.5*tile, "desert_rabbit", g.config)
	rabbit.Gold = 11
	g.addEcologyActor(w, rabbit)
	g.world = newTestWorld(g.config)
	wm.LoadedMaps["party"] = g.world
	wm.CurrentMapKey = "party"
	g.simulateRemoteEcology(false, false)
	v := g.ecologyViews[w]
	rabbit.HitPoints = 0
	v.combat.finishMonsterKill(rabbit)
	if len(g.groundContainers) != 1 || g.groundContainers[0].MapKey != "desert" || g.groundContainers[0].Gold != 11 {
		t.Fatalf("lost/misattributed remote reward: %+v", g.groundContainers)
	}
}

func TestCaravanWaitsAtBlockedMapEntrance(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[turn], func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			config.GlobalEcology.Populations = nil
			config.GlobalEcology.Caravan.Routes = []config.CaravanRoute{{ID: "door", Points: []config.RoutePoint{{Map: "desert", X: 20, Y: 20}, {Map: "city", X: 10, Y: 10}, {Map: "city", X: 12, Y: 10}}}}
			destination := newTestWorld(g.config)
			destination.Width, destination.Height, destination.Tiles = g.world.Width, g.world.Height, g.world.Tiles
			wm.LoadedMaps["city"] = destination
			blocker := monster.NewMonster3DFromConfig(10.5*tile, 10.5*tile, "forest_orc", g.config)
			destination.Monsters = []*monster.Monster3D{blocker}
			g.ecology.Unlocked = true
			g.turnBasedMode = turn
			g.currentTurn = 1
			g.updateEcology()
			start, m := g.ecologyActor()
			x, y := m.X, m.Y
			for i := 0; i < 20; i++ {
				g.updateEcology()
				g.refreshMonsterAIState()
				m.UpdateAmbient(g.collisionSystem, m.AITargetX, m.AITargetY, turn)
			}
			w, _ := g.ecologyActor()
			if w != start || m.X != x || m.Y != y {
				t.Fatal("walked away from blocked exit")
			}
			destination.Monsters = nil
			g.updateEcology()
			w, _ = g.ecologyActor()
			if w != destination {
				t.Fatal("did not resume through cleared entrance")
			}
		})
	}
}

func TestFennecActuallyHuntsWithoutRewards(t *testing.T) {
	for _, turn := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[turn], func(t *testing.T) {
			g, wm, tile := ecologyTestGame(t)
			w := g.world
			rabbit := monster.NewMonster3DFromConfig(22.5*tile, 20.5*tile, "desert_rabbit", g.config)
			fox := monster.NewMonster3DFromConfig(20.5*tile, 20.5*tile, "fennec", g.config)
			rabbit.Population = "desert:desert_rabbit"
			fox.Population = "desert:fennec"
			g.addEcologyActor(w, rabbit)
			g.addEcologyActor(w, fox)
			g.world = newTestWorld(g.config)
			wm.LoadedMaps["party"] = g.world
			wm.CurrentMapKey = "party"
			for i := 0; i < 12000 && rabbit.IsAlive(); i++ {
				g.frameCount++
				g.simulateRemoteEcology(turn, true)
			}
			if rabbit.IsAlive() {
				t.Fatalf("predator never caught prey: fox=(%.0f,%.0f) rabbit=(%.0f,%.0f)", fox.X, fox.Y, rabbit.X, rabbit.Y)
			}
			if !rabbit.NoKillRewards || len(g.groundContainers) > 0 {
				t.Fatal("predation granted player loot")
			}
		})
	}
}

func TestAmbientActorsAreNeutralToCampAndHostileHealing(t *testing.T) {
	for _, key := range []string{"desert_rabbit", "fennec", "desert_caravan"} {
		t.Run(key, func(t *testing.T) {
			g, _, tile := ecologyTestGame(t)
			animal := monster.NewMonster3DFromConfig(g.camera.X+tile, g.camera.Y, key, g.config)
			animal.HitPoints = 1
			g.world.Monsters = []*monster.Monster3D{animal}
			g.party.Food = 10
			if _, ok := g.TryCamp(); !ok {
				t.Fatal("neutral actor blocked camping")
			}
			healer := monster.NewMonster3DFromConfig(animal.X, animal.Y, "goblin", g.config)
			healer.AllyHealRadiusPixels = 10 * tile
			if got := g.combat.pickMonsterAllyHealTarget(healer); got != nil {
				t.Fatal("hostile healer selected neutral actor")
			}
			if key == "desert_caravan" && !isExcludedFromPartyAutoTarget(animal) {
				t.Fatal("party auto-targets caravan")
			}
		})
	}
}

func TestCaravanOffscreenProjectilesAdvanceBetweenTurns(t *testing.T) {
	g, wm, tile := ecologyTestGame(t)
	g.ecology.Unlocked = true
	g.spawnCaravan()
	w, caravan := g.ecologyActor()
	caravan.Speed = 0
	enemy := monster.NewMonster3DFromConfig(caravan.X+3*tile, caravan.Y, "bandit", g.config)
	enemy.RangedAttackRange = 5 * tile
	enemy.AlertRadius = 8 * tile
	w.Monsters = append(w.Monsters, enemy)
	g.world = newTestWorldSized(g.config, 30, 30)
	wm.LoadedMaps["party"] = g.world
	wm.CurrentMapKey = "party"
	g.threading = &threading.ThreadingComponents{EntityUpdater: entities.NewEntityUpdaterWithWorkers(1)}
	t.Cleanup(g.threading.Shutdown)
	g.turnBasedMode = true
	g.simulateRemoteEcology(true, true)
	v := g.ecologyViews[w]
	if len(v.arrows) == 0 {
		t.Fatal("TB ranged actor did not fire")
	}
	x, y := v.arrows[0].X, v.arrows[0].Y
	g.frameCount++
	g.advanceRemoteEcologyProjectiles()
	if len(v.arrows) > 0 && v.arrows[0].X == x && v.arrows[0].Y == y {
		t.Fatal("arrow froze while waiting for next monster turn")
	}
	for i := 0; i < 240; i++ {
		g.frameCount++
		g.advanceRemoteEcologyProjectiles()
	}
	if v.gameLoop.hasActiveProjectiles() {
		t.Fatal("off-screen TB arrow never resolved")
	}
}
