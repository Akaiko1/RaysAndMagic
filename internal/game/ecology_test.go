package game

import (
	"encoding/json"
	"testing"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"

	"ugataima/internal/character"
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
			if count("desert_rabbit") != 7 {
				t.Fatal("day cap")
			}
			for _, m := range w.Monsters {
				if m.Key == "desert_rabbit" {
					m.HitPoints = 0
					break
				}
			}
			g.replenishWildlife()
			if count("desert_rabbit") != 6 {
				t.Fatal("same-phase replacement")
			}
			data, _ := json.Marshal(g.ecology)
			g.ecology = EcologyState{}
			if err := json.Unmarshal(data, &g.ecology); err != nil {
				t.Fatal(err)
			}
			g.replenishWildlife()
			if count("desert_rabbit") != 6 {
				t.Fatal("load replaced wildlife")
			}
			g.dayNightDay++
			g.dayNightIsNight = true
			g.replenishWildlife()
			if count("desert_rabbit") != 6 || count("fennec") != 2 {
				t.Fatal("night removed survivors or wrong cap")
			}
			g.dayNightDay++
			g.dayNightIsNight = false
			g.replenishWildlife()
			if count("desert_rabbit") != 7 || count("fennec") != 2 {
				t.Fatal("dawn did not retain/refill")
			}
			if worldHasLivingMonstersInRect(w, 0, 0, w.Width, w.Height, g.config.GetTileSize()) {
				t.Fatal("wildlife blocks map clear")
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
	if total != 3 || g.ecology.Deliveries != 1 {
		t.Fatalf("delivery total=%d trips=%d", total, g.ecology.Deliveries)
	}
	pool := config.CaravanTradePool()
	g.ecology.Stock = map[string]int{}
	for _, k := range pool[:12] {
		g.ecology.Stock[k] = 1
	}
	before := cloneEcologyState(g.ecology)
	for i := 0; i < 100; i++ {
		g.depositCaravanGoods()
	}
	if len(g.ecology.Stock) != 12 {
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
	if len(g.ecology.Stock) != 11 {
		t.Fatal("empty stack must release slot")
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
func TestCaravanEligibleGoods(t *testing.T) {
	ecologyTestGame(t)
	found := map[string]bool{}
	for _, k := range config.CaravanTradePool() {
		found[k] = true
	}
	for _, key := range []string{"clock_hand", "brass_gear", "pocket_watch", "red_dragon_scale", "green_dragon_scale", "gold_dragon_scale"} {
		if !found[key] {
			t.Errorf("missing trade good %s", key)
		}
	}
	for _, key := range []string{"black_dragon_scale", "ordinary_key", "inlaid_key", "health_potion", "bandit_card"} {
		if found[key] {
			t.Errorf("invalid trade good %s", key)
		}
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
