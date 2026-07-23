package game

import (
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsEnvironmentResource(values []processedSpriteKey, tileType world.TileType3D, name string) bool {
	for _, value := range values {
		if value.tileType == tileType && value.spriteName == name {
			return true
		}
	}
	return false
}

func containsNPCResource(values []mapNPCPrewarmResource, want mapNPCPrewarmResource) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsMonsterResource(values []mapMonsterPrewarmResource, key string) bool {
	for _, value := range values {
		if value.key == key && value.spriteName != "" {
			return true
		}
	}
	return false
}

func TestMapRenderPrewarmPlanCoversColdWorldResources(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")

	previousTileManager := world.GlobalTileManager
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() {
		world.GlobalTileManager = previousTileManager
		world.GlobalWorldManager = previousWorldManager
	})
	world.GlobalWorldManager = nil
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	tileType := func(key string) world.TileType3D {
		t.Helper()
		value, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
		if !ok {
			t.Fatalf("missing tile %q", key)
		}
		return value
	}
	tree := tileType("tree")
	wall := tileType("church_wall")
	mushrooms := tileType("mushroom_ring")
	fireflies := tileType("firefly_swarm")

	w := newTestWorldSized(cfg, 3, 2)
	w.Tiles = [][]world.TileType3D{
		{tree, wall, mushrooms},
		{fireflies, tree, wall},
	}
	w.Monsters = append(w.Monsters, monster.NewMonster3DFromConfig(32, 32, "forest_spider", cfg))
	w.MonsterSpawns = append(w.MonsterSpawns, world.MonsterSpawn{MonsterKey: "wolf"})
	w.NPCs = append(w.NPCs, &character.NPC{
		Sprite:         "chest_wooden",
		VisitedSprite:  "chest_iron",
		RenderCategory: "scenery",
		Summons:        []*character.NPCSummon{{Monster: "dragon_red"}},
		EncounterData: &character.NPCEncounter{
			Monsters: []*character.EncounterMonster{{Type: "goblin"}},
			Rewards: &monster.EncounterRewards{
				TreasureChest: &monster.TreasureChestReward{Sprite: "clockwork_chest"},
			},
		},
	})

	g := newTestGame(cfg, w)
	g.sprites = graphics.NewSpriteManager()
	ApplySpriteColorKey(g.sprites, cfg)
	g.groundContainers = []GroundContainer{{Sprite: "pyramid_chest", Gold: 1}}
	r := &Renderer{game: g}
	r.buildTransparentSpriteCache()

	if !r.mapRenderResourcePrewarmPending {
		t.Fatal("map scan did not schedule resource prewarm")
	}
	if got, want := len(r.mapRenderTileTypes), 4; got != want {
		t.Fatalf("unique tile inventory = %d, want %d (%v)", got, want, r.mapRenderTileTypes)
	}

	plan := r.collectMapRenderPrewarmPlan("")
	for _, name := range []string{"forest_oak", "church_wall", "mushroom_ring"} {
		if !containsString(plan.tileSprites, name) {
			t.Errorf("tile sprite %q missing from plan: %v", name, plan.tileSprites)
		}
	}
	if containsString(plan.tileSprites, "firefly_swarm") {
		t.Error("procedural firefly tile unnecessarily prewarms its legacy PNG")
	}
	if !containsString(plan.wallSprites, "church_wall") {
		t.Errorf("textured wall missing from wall plan: %v", plan.wallSprites)
	}
	if !containsString(plan.treeSprites, "forest_oak") {
		t.Errorf("tree sprite missing from standee plan: %v", plan.treeSprites)
	}
	if !containsEnvironmentResource(plan.environmentSprites, mushrooms, "mushroom_ring") {
		t.Errorf("transparent environment sprite missing from plan: %+v", plan.environmentSprites)
	}
	if containsEnvironmentResource(plan.environmentSprites, fireflies, "firefly_swarm") {
		t.Error("procedural firefly tile entered the image-backed environment plan")
	}
	for _, want := range []mapNPCPrewarmResource{
		{name: "chest_wooden", prefix: "npc"},
		{name: "chest_iron", prefix: "npc"},
	} {
		if !containsNPCResource(plan.npcSprites, want) {
			t.Errorf("NPC resource %+v missing from plan: %+v", want, plan.npcSprites)
		}
	}
	for _, key := range []string{"forest_spider", "wolf", "dragon_red", "goblin"} {
		if !containsMonsterResource(plan.monsterSprites, key) {
			t.Errorf("monster %q missing from plan: %+v", key, plan.monsterSprites)
		}
	}
	for _, name := range []string{"pyramid_chest", "clockwork_chest", "bag_rare", "chest"} {
		if !containsString(plan.containerSprites, name) {
			t.Errorf("container sprite %q missing from plan", name)
		}
	}
	if containsString(plan.containerSprites, "chest_golden") {
		t.Error("unrelated indexed chest entered the map-scoped container plan")
	}

	g.appScreen = AppScreenMainMenu
	r.prewarmPendingMapRenderResources()
	if !r.mapRenderResourcePrewarmPending {
		t.Fatal("entry menu consumed map prewarm before gameplay")
	}
}

func TestOpenWorldPrewarmKeepsStaticDecodeGlobalAndStandeesRegionScoped(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")

	previousTileManager := world.GlobalTileManager
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() {
		world.GlobalTileManager = previousTileManager
		world.GlobalWorldManager = previousWorldManager
	})
	world.GlobalWorldManager = nil
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	tileType := func(key string) world.TileType3D {
		t.Helper()
		value, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
		if !ok {
			t.Fatalf("missing tile %q", key)
		}
		return value
	}

	w := newTestWorldSized(cfg, 4, 1)
	w.Tiles = [][]world.TileType3D{{
		tileType("tree"), tileType("tree"),
		tileType("mushroom_ring"), tileType("mushroom_ring"),
	}}
	leftMonster := monster.NewMonster3DFromConfig(32, 32, "wolf", cfg)
	rightMonster := monster.NewMonster3DFromConfig(160, 32, "forest_spider", cfg)
	w.Monsters = []*monster.Monster3D{leftMonster, rightMonster}
	w.NPCs = []*character.NPC{
		{X: 32, Y: 32, Sprite: "chest_wooden", RenderCategory: "scenery"},
		{X: 160, Y: 32, Sprite: "chest_iron", RenderCategory: "scenery"},
	}

	g := newTestGame(cfg, w)
	g.sprites = graphics.NewSpriteManager()
	ApplySpriteColorKey(g.sprites, cfg)
	g.groundContainers = []GroundContainer{
		{X: 32, Y: 32, Sprite: "pyramid_chest"},
		{X: 160, Y: 32, Sprite: "clockwork_chest"},
	}
	r := &Renderer{game: g}
	r.buildTransparentSpriteCache()

	scope := mapRenderPrewarmScope{
		mapKey: "left",
		region: &world.OpenWorldRegion{
			MapKey: "left", OffsetX: 0, OffsetY: 0, Width: 2, Height: 1,
		},
		tileSize: float64(cfg.GetTileSize()),
	}
	plan := r.collectMapRenderPrewarmPlanForScope(scope)

	for _, name := range []string{"forest_oak", "mushroom_ring"} {
		if !containsString(plan.tileSprites, name) {
			t.Errorf("shared static sprite %q missing: %v", name, plan.tileSprites)
		}
	}
	for _, name := range []string{"chest_wooden", "chest_iron"} {
		if !containsString(plan.npcDecodeSprites, name) {
			t.Errorf("global NPC decode %q missing: %v", name, plan.npcDecodeSprites)
		}
	}
	if !containsNPCResource(plan.npcSprites, mapNPCPrewarmResource{name: "chest_wooden", prefix: "npc"}) {
		t.Errorf("left NPC standee missing: %+v", plan.npcSprites)
	}
	if containsNPCResource(plan.npcSprites, mapNPCPrewarmResource{name: "chest_iron", prefix: "npc"}) {
		t.Errorf("right NPC leaked into left standee plan: %+v", plan.npcSprites)
	}
	for _, key := range []string{"wolf", "forest_spider"} {
		if !containsMonsterResource(plan.monsterDecode, key) {
			t.Errorf("global monster decode %q missing: %+v", key, plan.monsterDecode)
		}
	}
	if !containsMonsterResource(plan.monsterSprites, "wolf") {
		t.Errorf("left monster standee missing: %+v", plan.monsterSprites)
	}
	if containsMonsterResource(plan.monsterSprites, "forest_spider") {
		t.Errorf("right monster leaked into left standee plan: %+v", plan.monsterSprites)
	}
	if !containsString(plan.containerSprites, "pyramid_chest") {
		t.Errorf("left container missing: %v", plan.containerSprites)
	}
	if containsString(plan.containerSprites, "clockwork_chest") {
		t.Errorf("right container leaked into left plan: %v", plan.containerSprites)
	}
	if !containsString(plan.containerDecode, "clockwork_chest") {
		t.Errorf("right container missing from shared decode plan: %v", plan.containerDecode)
	}
}

func TestMapRenderResidencyKeepsTwoRegionsAndSharedStandees(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "desert"}

	shared := standeeCoreKey{name: "tree:shared"}
	forestOnly := standeeCoreKey{name: "mob:forest"}
	desertOnly := standeeCoreKey{name: "mob:desert"}
	r := &Renderer{
		mapRenderResidentMapKeys:      []string{"forest", "desert"},
		mapRenderSharedResourcesReady: true,
		mapRenderSharedStandeeKeys: map[standeeCoreKey]struct{}{
			shared: {},
		},
		mapRenderStandeeKeysByMap: map[string]map[standeeCoreKey]struct{}{
			"forest": {shared: {}, forestOnly: {}},
			"desert": {shared: {}, desertOnly: {}},
		},
	}

	if !r.prepareMapRenderResidency("highlands") {
		t.Fatal("new region reported as already resident")
	}
	if want := []string{"forest", "desert"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("prewarm changed residents early = %v, want %v", r.mapRenderResidentMapKeys, want)
	}
	r.commitMapRenderResidency("highlands", map[standeeCoreKey]struct{}{})
	if want := []string{"desert", "highlands"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("residents after commit = %v, want %v", r.mapRenderResidentMapKeys, want)
	}
	if _, ok := r.mapRenderStandeeKeysByMap["forest"]; ok {
		t.Fatal("oldest region metadata was not evicted after commit")
	}
	if _, ok := r.mapRenderSharedStandeeKeys[shared]; !ok {
		t.Fatal("shared standee was evicted with the oldest region")
	}

	if r.prepareMapRenderResidency("desert") {
		t.Fatal("resident region requested a redundant rebuild")
	}
	if want := []string{"highlands", "desert"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("touch order = %v, want %v", r.mapRenderResidentMapKeys, want)
	}

	lazy := standeeCoreKey{name: "mob:late_spawn"}
	r.trackResidentStandeeKey(lazy)
	if _, ok := r.mapRenderStandeeKeysByMap["desert"][lazy]; !ok {
		t.Fatal("lazily generated standee was not attached to the current resident region")
	}
}
