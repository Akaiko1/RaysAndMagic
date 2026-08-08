package game

import (
	"math"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
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
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
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
		SizeClass:      "small_prop",
		Summons:        []*character.NPCSummon{{Monster: "dragon_red"}},
		EncounterData: &character.NPCEncounter{
			Monsters: []*character.EncounterMonster{{Type: "goblin"}},
			Rewards: &monster.EncounterRewards{
				TreasureChest: &monster.TreasureChestReward{Sprite: "clockwork_chest"},
			},
		},
	})
	w.NPCs = append(w.NPCs, &character.NPC{
		Sprite: "innkeeper_female", RenderCategory: "npc", SizeClass: "person",
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
		{name: "chest_wooden", prefix: "npc", warmVisibleBounds: true},
		{name: "chest_iron", prefix: "npc", warmVisibleBounds: true},
		{name: "innkeeper_female", prefix: "npc", warmVisibleBounds: false},
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

func TestOpenWorldPrewarmScopesSourcesAndDerivedResourcesToRegion(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")

	previousTileManager := world.GlobalTileManager
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() {
		world.GlobalTileManager = previousTileManager
		world.GlobalWorldManager = previousWorldManager
	})
	world.GlobalWorldManager = nil
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
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
		{X: 32, Y: 32, Sprite: "chest_wooden", RenderCategory: "scenery", SizeClass: "small_prop"},
		{X: 160, Y: 32, Sprite: "chest_iron", RenderCategory: "scenery", SizeClass: "small_prop"},
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
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "local static source", got: containsString(plan.tileSprites, "forest_oak"), want: true},
		{name: "remote static source", got: containsString(plan.tileSprites, "mushroom_ring"), want: false},
		{name: "local NPC source", got: containsString(plan.npcDecodeSprites, "chest_wooden"), want: true},
		{name: "remote NPC source", got: containsString(plan.npcDecodeSprites, "chest_iron"), want: false},
		{name: "local monster source", got: containsMonsterResource(plan.monsterDecode, "wolf"), want: true},
		{name: "remote monster source", got: containsMonsterResource(plan.monsterDecode, "forest_spider"), want: false},
		{name: "local container source", got: containsString(plan.containerDecode, "pyramid_chest"), want: true},
		{name: "remote container source", got: containsString(plan.containerDecode, "clockwork_chest"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("present in left-region plan = %v, want %v", tc.got, tc.want)
			}
		})
	}

	if !containsString(plan.tileSprites, "forest_oak") {
		t.Errorf("left static sprite missing: %v", plan.tileSprites)
	}
	if containsString(plan.tileSprites, "mushroom_ring") {
		t.Errorf("right static sprite leaked into left plan: %v", plan.tileSprites)
	}
	if !containsString(plan.npcDecodeSprites, "chest_wooden") {
		t.Errorf("left NPC source missing: %v", plan.npcDecodeSprites)
	}
	if containsString(plan.npcDecodeSprites, "chest_iron") {
		t.Errorf("right NPC source leaked into left plan: %v", plan.npcDecodeSprites)
	}
	if !containsNPCResource(plan.npcSprites, mapNPCPrewarmResource{name: "chest_wooden", prefix: "npc", warmVisibleBounds: true}) {
		t.Errorf("left NPC standee missing: %+v", plan.npcSprites)
	}
	if containsNPCResource(plan.npcSprites, mapNPCPrewarmResource{name: "chest_iron", prefix: "npc", warmVisibleBounds: true}) {
		t.Errorf("right NPC leaked into left standee plan: %+v", plan.npcSprites)
	}
	if !containsMonsterResource(plan.monsterDecode, "wolf") {
		t.Errorf("left monster source missing: %+v", plan.monsterDecode)
	}
	if containsMonsterResource(plan.monsterDecode, "forest_spider") {
		t.Errorf("right monster source leaked into left plan: %+v", plan.monsterDecode)
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
	if containsString(plan.containerDecode, "clockwork_chest") {
		t.Errorf("right container source leaked into left plan: %v", plan.containerDecode)
	}
}

func testMapRenderResources(keys ...standeeCoreKey) *mapRenderRegionResources {
	resources := &mapRenderRegionResources{
		standees:  make(map[standeeCoreKey]struct{}),
		sources:   make(map[mapRenderSourceKey]struct{}),
		processed: make(map[processedSpriteKey]struct{}),
		walls:     make(map[*ebiten.Image]struct{}),
		skies:     make(map[string]struct{}),
	}
	for _, key := range keys {
		resources.standees[key] = struct{}{}
	}
	return resources
}

func TestMapRenderResidencyKeepsVisibleRegionsAndEvictsOutsideHysteresis(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "desert"}

	shared := standeeCoreKey{name: "tree:shared"}
	forestOnly := standeeCoreKey{name: "mob:forest"}
	desertOnly := standeeCoreKey{name: "mob:desert"}
	r := &Renderer{
		mapRenderResidentMapKeys: []string{"forest", "desert"},
		mapRenderResourcesByMap: map[string]*mapRenderRegionResources{
			"forest": testMapRenderResources(shared, forestOnly),
			"desert": testMapRenderResources(shared, desertOnly),
		},
	}

	if !r.prepareMapRenderResidency("highlands") {
		t.Fatal("new region reported as already resident")
	}
	if want := []string{"forest", "desert"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("visible residents before third prewarm = %v, want %v", r.mapRenderResidentMapKeys, want)
	}
	r.commitMapRenderResidency("highlands", testMapRenderResources())
	if want := []string{"forest", "desert", "highlands"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("residents after commit = %v, want %v", r.mapRenderResidentMapKeys, want)
	}
	if _, ok := r.mapRenderResourcesByMap["desert"].standees[shared]; !ok {
		t.Fatal("standee shared with the retained region lost its ownership")
	}

	if r.prepareMapRenderResidency("desert") {
		t.Fatal("resident region requested a redundant rebuild")
	}
	if want := []string{"forest", "highlands", "desert"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("touch order = %v, want %v", r.mapRenderResidentMapKeys, want)
	}

	r.evictMapRenderResidencyOutside(map[string]struct{}{"desert": {}, "highlands": {}})
	if want := []string{"highlands", "desert"}; !reflect.DeepEqual(r.mapRenderResidentMapKeys, want) {
		t.Fatalf("residents after leaving unload sector = %v, want %v", r.mapRenderResidentMapKeys, want)
	}
	if _, ok := r.mapRenderResourcesByMap["forest"]; ok {
		t.Fatal("region outside the unload sector kept its ownership manifest")
	}

	lazy := standeeCoreKey{name: "mob:late_spawn"}
	r.trackResidentStandeeKey(lazy)
	if _, ok := r.mapRenderResourcesByMap["desert"].standees[lazy]; !ok {
		t.Fatal("lazily generated standee was not attached to the current resident region")
	}
}

func TestVisibleOpenWorldMapKeysIncludesEveryRegionInView(t *testing.T) {
	wm := &world.WorldManager{OpenWorldRegions: []world.OpenWorldRegion{
		{MapKey: "current", OffsetX: 0, OffsetY: 0, Width: 10, Height: 10},
		{MapKey: "east", OffsetX: 10, OffsetY: 0, Width: 10, Height: 10},
		{MapKey: "north_east", OffsetX: 10, OffsetY: -10, Width: 10, Height: 10},
		{MapKey: "south_east", OffsetX: 10, OffsetY: 10, Width: 10, Height: 10},
		{MapKey: "far", OffsetX: 40, OffsetY: 0, Width: 10, Height: 10},
	}}
	tests := []struct {
		name     string
		angle    float64
		distance float64
		want     []string
	}{
		{name: "three neighbours share the view", angle: 0, distance: 20, want: []string{"current", "east", "north_east", "south_east"}},
		{name: "regions behind the camera stay cold", angle: math.Pi, distance: 20, want: []string{"current"}},
		{name: "far region stays cold", angle: 0, distance: 3, want: []string{"current", "east"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			camera := &FirstPersonCamera{X: 9, Y: 5, Angle: tt.angle, FOV: math.Pi / 2, ViewDist: tt.distance}
			if got := visibleOpenWorldMapKeys(wm, camera, 1, 0, 0); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("visible regions = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncVisibleMapRenderResidencyQueuesMultipleNeighboursOnce(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	w := newTestWorldSized(loadTestConfig(t), 30, 20)
	wm := &world.WorldManager{
		CurrentMapKey: "current",
		OpenWorld:     w,
		OpenWorldRegions: []world.OpenWorldRegion{
			{MapKey: "current", OffsetX: 0, OffsetY: 0, Width: 10, Height: 10},
			{MapKey: "east", OffsetX: 10, OffsetY: 0, Width: 10, Height: 10},
			{MapKey: "south_east", OffsetX: 10, OffsetY: 10, Width: 10, Height: 10},
		},
	}
	world.GlobalWorldManager = wm
	cfg := loadTestConfig(t)
	tileSize := float64(cfg.GetTileSize())
	g := &MMGame{
		world:  w,
		config: cfg,
		camera: &FirstPersonCamera{X: 9 * tileSize, Y: 5 * tileSize, Angle: 0, FOV: math.Pi / 2, ViewDist: 20 * tileSize},
	}
	r := &Renderer{game: g}

	r.syncVisibleMapRenderResidency()
	r.syncVisibleMapRenderResidency()
	want := []string{"current", "east", "south_east"}
	if got := r.mapRenderResourcePrewarmMapKeys; !reflect.DeepEqual(got, want) {
		t.Fatalf("queued regions = %v, want %v", got, want)
	}

	r.mapRenderResourcePrewarmMapKeys = nil
	r.mapRenderResourcePrewarmPending = false
	for _, mapKey := range want {
		r.commitMapRenderResidency(mapKey, testMapRenderResources())
	}
	g.camera.Angle = math.Pi
	r.syncVisibleMapRenderResidency()
	if got := r.mapRenderResidentMapKeys; !reflect.DeepEqual(got, want) {
		t.Fatalf("camera turn evicted nearby residents: got %v, want %v", got, want)
	}
	if len(r.mapRenderResourcePrewarmMapKeys) != 0 {
		t.Fatalf("camera turn requeued resident regions: %v", r.mapRenderResourcePrewarmMapKeys)
	}
}

func TestMapRenderResidencyEvictsOrRetainsWholeManifest(t *testing.T) {
	t.Chdir("../..")
	tests := []struct {
		name   string
		shared bool
	}{
		{name: "unique resource is evicted", shared: false},
		{name: "overlapping resource is retained", shared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sprites := graphics.NewSpriteManager()
			source := sprites.GetSprite("forest_oak")
			processedKey := processedSpriteKey{tileType: world.TileType3D(1234), spriteName: "forest_oak"}
			processed := ebiten.NewImage(4, 4)
			sourceKey := mapRenderSourceKey{name: "forest_oak"}
			forest := testMapRenderResources()
			forest.sources[sourceKey] = struct{}{}
			forest.processed[processedKey] = struct{}{}
			forest.walls[source] = struct{}{}
			desert := testMapRenderResources()
			if tt.shared {
				desert.sources[sourceKey] = struct{}{}
				desert.processed[processedKey] = struct{}{}
				desert.walls[source] = struct{}{}
			}
			r := &Renderer{
				game:                     &MMGame{sprites: sprites},
				mapRenderResidentMapKeys: []string{"forest", "desert"},
				mapRenderResourcesByMap: map[string]*mapRenderRegionResources{
					"forest": forest,
					"desert": desert,
				},
				processedSpriteCache: map[processedSpriteKey]*ebiten.Image{processedKey: processed},
				wallRipmaps:          map[*ebiten.Image]*wallRipmap{source: {}},
			}

			r.evictMapRenderResidencyOutside(map[string]struct{}{"desert": {}})
			reloaded := sprites.GetSprite("forest_oak")
			if gotSame := reloaded == source; gotSame != tt.shared {
				t.Fatalf("source pointer retained = %v, want %v", gotSame, tt.shared)
			}
			_, processedPresent := r.processedSpriteCache[processedKey]
			if processedPresent != tt.shared {
				t.Fatalf("processed resource retained = %v, want %v", processedPresent, tt.shared)
			}
			_, wallPresent := r.wallRipmaps[source]
			if wallPresent != tt.shared {
				t.Fatalf("wall resource retained = %v, want %v", wallPresent, tt.shared)
			}
			sprites.EvictResource("forest_oak", "")
			processed.Deallocate()
		})
	}
}

func TestMapRenderResidencyOwnsSkyPanoramas(t *testing.T) {
	tests := []struct {
		name       string
		activeFade bool
		wantCached bool
	}{
		{name: "inactive evicted sky is released", wantCached: false},
		{name: "outgoing fade sky survives until fade completes", activeFade: true, wantCached: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldSky := ebiten.NewImage(4, 4)
			currentSky := ebiten.NewImage(4, 4)
			defer currentSky.Deallocate()
			g := &MMGame{
				skyPanorama: currentSky,
				skyPanoramaCache: map[string]*ebiten.Image{
					"old":     oldSky,
					"current": currentSky,
				},
				sprites: graphics.NewSpriteManager(),
			}
			if tt.activeFade {
				g.skyPanoramaPrev = oldSky
			}
			resources := testMapRenderResources()
			resources.skies["old"] = struct{}{}
			keep := testMapRenderResources()
			keep.skies["current"] = struct{}{}
			r := &Renderer{game: g}

			r.deallocateMapRenderRegion(resources, keep)
			_, cached := g.skyPanoramaCache["old"]
			if cached != tt.wantCached {
				t.Fatalf("old sky cached = %v, want %v", cached, tt.wantCached)
			}
			if cached {
				g.skyPanoramaPrev = nil
				r.deallocateUnusedSkyPanoramas(keep.skies)
				if _, stillCached := g.skyPanoramaCache["old"]; stillCached {
					t.Fatal("completed fade retained the orphaned old sky")
				}
			}
		})
	}
}
