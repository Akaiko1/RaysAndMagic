package game

import (
	"context"
	"image"
	"math"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
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

type recordingMapRenderUploadDestination struct {
	images  []*ebiten.Image
	options []ebiten.DrawImageOptions
}

func (d *recordingMapRenderUploadDestination) DrawImage(img *ebiten.Image, opts *ebiten.DrawImageOptions) {
	d.images = append(d.images, img)
	if opts == nil {
		d.options = append(d.options, ebiten.DrawImageOptions{})
		return
	}
	d.options = append(d.options, *opts)
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
	t.Cleanup(r.resetMapRenderResourceResidency)
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
	queuedBefore := append([]string(nil), r.mapRenderResourcePrewarmMapKeys...)
	r.prewarmPendingMapRenderResources()
	if !r.mapRenderResourcePrewarmPending {
		t.Fatal("entry menu consumed map prewarm before gameplay")
	}
	if r.mapRenderResourcePrewarmActive != nil {
		t.Fatal("entry menu started map prewarm")
	}
	if !reflect.DeepEqual(r.mapRenderResourcePrewarmMapKeys, queuedBefore) {
		t.Fatalf("entry menu changed queued prewarm: got %v want %v", r.mapRenderResourcePrewarmMapKeys, queuedBefore)
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

func closedPreparedSprites() <-chan graphics.PreparedSpriteResource {
	ch := make(chan graphics.PreparedSpriteResource)
	close(ch)
	return ch
}

func closedPreparedSkies() <-chan mapRenderPreparedSky {
	ch := make(chan mapRenderPreparedSky)
	close(ch)
	return ch
}

func TestMapRenderStreamingAdvancesOneStateCellPerUpdate(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*Renderer, *mapRenderPrewarmTask, *int)
		wantSteps        int
		wantActive       bool
		wantResident     bool
		wantSpritesDone  bool
		wantPreparedHeld bool
	}{
		{
			name: "CPU worker not ready leaves derived work untouched",
			setup: func(_ *Renderer, task *mapRenderPrewarmTask, _ *int) {
				task.preparedSprites = make(chan graphics.PreparedSpriteResource)
				task.preparedSkies = make(chan mapRenderPreparedSky)
			},
			wantActive: true, wantPreparedHeld: true,
		},
		{
			name: "one prepared result is committed without running a derived step",
			setup: func(_ *Renderer, task *mapRenderPrewarmTask, _ *int) {
				sprites := make(chan graphics.PreparedSpriteResource, 1)
				sprites <- graphics.PreparedSpriteResource{Request: graphics.SpriteResourceRequest{Name: "missing"}}
				close(sprites)
				task.preparedSprites = sprites
				task.preparedSkies = closedPreparedSkies()
			},
			wantActive: true, wantPreparedHeld: true,
		},
		{
			name: "one derived step runs per update",
			setup: func(_ *Renderer, task *mapRenderPrewarmTask, steps *int) {
				task.spritesDone = true
				task.skiesDone = true
				task.steps = []func(){func() { *steps++ }, func() { *steps++ }}
			},
			wantSteps: 1, wantActive: true, wantSpritesDone: true,
		},
		{
			name: "final derived step commits exactly one region",
			setup: func(_ *Renderer, task *mapRenderPrewarmTask, steps *int) {
				task.spritesDone = true
				task.skiesDone = true
				task.steps = []func(){func() { *steps++ }}
			},
			wantSteps: 1, wantResident: true, wantSpritesDone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &MMGame{appScreen: AppScreenInGame, sprites: graphics.NewSpriteManager()}
			r := &Renderer{game: g, mapRenderResourcePrewarmPending: true}
			task := &mapRenderPrewarmTask{mapKey: "forest"}
			task.prewarmer = newMapRenderPrewarmer(r, task)
			steps := 0
			tt.setup(r, task, &steps)
			r.mapRenderResourcePrewarmActive = task

			r.prewarmPendingMapRenderResources()

			if steps != tt.wantSteps {
				t.Fatalf("derived steps = %d, want %d", steps, tt.wantSteps)
			}
			if got := r.mapRenderResourcePrewarmActive != nil; got != tt.wantActive {
				t.Fatalf("active loader = %v, want %v", got, tt.wantActive)
			}
			if got := containsString(r.mapRenderResidentMapKeys, "forest"); got != tt.wantResident {
				t.Fatalf("forest resident = %v, want %v", got, tt.wantResident)
			}
			if tt.wantSpritesDone && !task.spritesDone {
				t.Fatal("prepared sprite stage lost its completed state")
			}
			if tt.wantPreparedHeld && task.nextStep != 0 {
				t.Fatalf("prepared stage advanced derived index to %d", task.nextStep)
			}
		})
	}
}

func TestGameLoopUpdateAdvancesMapRenderStreaming(t *testing.T) {
	g := &MMGame{appScreen: AppScreenInGame, exitRequested: true, threading: threading.NewThreadingComponents(nil)}
	r := &Renderer{game: g, mapRenderResourcePrewarmPending: true}
	steps := 0
	task := &mapRenderPrewarmTask{
		mapKey: "forest", spritesDone: true, skiesDone: true,
		steps: []func(){func() { steps++ }, func() { steps++ }},
	}
	task.prewarmer = newMapRenderPrewarmer(r, task)
	r.mapRenderResourcePrewarmActive = task
	gl := &GameLoop{game: g, renderer: r}

	if err := gl.Update(); err != ErrExit {
		t.Fatalf("Update error = %v, want %v", err, ErrExit)
	}
	if steps != 1 || task.nextStep != 1 {
		t.Fatalf("streaming progress = steps:%d index:%d, want 1,1", steps, task.nextStep)
	}
}

func TestMapRenderStreamingProgressRespectsAppScreen(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name         string
		screen       AppScreen
		active       bool
		wantQueued   int
		wantActive   bool
		wantProgress int
	}{
		{name: "main menu keeps queued loader cold", screen: AppScreenMainMenu, wantQueued: 1},
		{name: "party creation keeps queued loader cold", screen: AppScreenPartyCreate, wantQueued: 1},
		{name: "gameplay starts queued loader", screen: AppScreenInGame, wantActive: true},
		{name: "main menu pauses active loader", screen: AppScreenMainMenu, active: true, wantActive: true},
		{name: "party creation pauses active loader", screen: AppScreenPartyCreate, active: true, wantActive: true},
		{name: "gameplay advances active loader", screen: AppScreenInGame, active: true, wantActive: true, wantProgress: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &MMGame{
				appScreen: tt.screen, exitRequested: true, config: cfg,
				sprites: graphics.NewSpriteManager(), threading: threading.NewThreadingComponents(nil),
			}
			r := &Renderer{
				game: g, mapRenderResourcePrewarmPending: true,
				mapRenderResourcePrewarmMapKeys: []string{"forest"},
			}
			steps := 0
			if tt.active {
				r.mapRenderResourcePrewarmMapKeys = nil
				task := &mapRenderPrewarmTask{
					mapKey: "forest", spritesDone: true, skiesDone: true,
					steps: []func(){func() { steps++ }, func() { steps++ }},
				}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				r.mapRenderResourcePrewarmActive = task
			}
			gl := &GameLoop{game: g, renderer: r}

			if err := gl.Update(); err != ErrExit {
				t.Fatalf("Update error = %v, want %v", err, ErrExit)
			}
			if got := len(r.mapRenderResourcePrewarmMapKeys); got != tt.wantQueued {
				t.Fatalf("queued loaders = %d, want %d", got, tt.wantQueued)
			}
			if got := r.mapRenderResourcePrewarmActive != nil; got != tt.wantActive {
				t.Fatalf("active loader = %v, want %v", got, tt.wantActive)
			}
			if steps != tt.wantProgress {
				t.Fatalf("streaming steps = %d, want %d", steps, tt.wantProgress)
			}
			r.resetMapRenderResourceResidency()
		})
	}
}

func TestMapRenderStandeeWorkerCompletionAndCancellation(t *testing.T) {
	tests := []struct {
		name       string
		cancel     bool
		wantResult bool
	}{
		{name: "active job completes", wantResult: true},
		{name: "cancelled job is dropped", cancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}
			cpu := image.NewRGBA(image.Rect(0, 0, 4, 4))
			results := prepareMapRenderStandees(ctx, []mapRenderStandeeJob{{
				key: standeeCoreKey{name: "mob:test"}, cpu: cpu,
			}}, 0.5)
			prepared, ok := <-results
			if ok != tt.wantResult {
				t.Fatalf("worker result present = %v, want %v", ok, tt.wantResult)
			}
			if ok && (prepared.prepared.core == nil || len(prepared.prepared.coreMips) == 0) {
				t.Fatal("worker returned an incomplete standee preparation")
			}
		})
	}
}

func TestMapRenderSkyCommitKeepsLazyLoadedWinner(t *testing.T) {
	winner := ebiten.NewImage(2, 2)
	g := &MMGame{skyPanoramaCache: map[string]*ebiten.Image{"forest": winner}}
	r := &Renderer{game: g}
	task := &mapRenderPrewarmTask{
		mapKey: "forest",
		skyCommit: &mapRenderSkyCommit{
			name: "forest", image: ebiten.NewImage(2, 2), cpu: image.NewRGBA(image.Rect(0, 0, 2, 2)),
		},
	}
	task.prewarmer = newMapRenderPrewarmer(r, task)

	if !r.drainPreparedMapRenderResource(task) {
		t.Fatal("sky commit did not advance")
	}
	if g.skyPanoramaCache["forest"] != winner {
		t.Fatal("background sky commit replaced the lazy-loaded panorama")
	}
	if _, ok := task.prewarmer.uploads[winner]; !ok {
		t.Fatal("lazy-loaded panorama was not retained for upload warming")
	}
}

func TestMapRenderStreamingQueueAndCancellationCases(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "current"}
	tests := []struct {
		name       string
		resident   []string
		queued     []string
		activeKey  string
		schedule   string
		keep       map[string]struct{}
		wantQueue  []string
		wantActive bool
	}{
		{name: "current region gets queue priority", queued: []string{"east"}, schedule: "current", wantQueue: []string{"current", "east"}},
		{name: "resident region is not queued", resident: []string{"current"}, schedule: "current"},
		{name: "active region is not queued twice", activeKey: "current", schedule: "current", wantActive: true},
		{name: "queued region outside retain set is cancelled", queued: []string{"current", "far"}, keep: map[string]struct{}{"current": {}}, wantQueue: []string{"current"}},
		{name: "active region outside retain set is cancelled", activeKey: "far", keep: map[string]struct{}{"current": {}}},
		{name: "active region inside retain set continues", activeKey: "current", keep: map[string]struct{}{"current": {}}, wantActive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Renderer{
				game:                            &MMGame{sprites: graphics.NewSpriteManager()},
				mapRenderResidentMapKeys:        append([]string(nil), tt.resident...),
				mapRenderResourcePrewarmMapKeys: append([]string(nil), tt.queued...),
				mapRenderResourcesByMap:         make(map[string]*mapRenderRegionResources),
			}
			for _, key := range tt.resident {
				r.mapRenderResourcesByMap[key] = testMapRenderResources()
			}
			if tt.activeKey != "" {
				task := &mapRenderPrewarmTask{mapKey: tt.activeKey}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				r.mapRenderResourcePrewarmActive = task
			}
			if tt.schedule != "" {
				r.scheduleMapRenderResourcePrewarm(tt.schedule)
			}
			if tt.keep != nil {
				r.cancelMapRenderPrewarmOutside(tt.keep)
			}
			if !reflect.DeepEqual(r.mapRenderResourcePrewarmMapKeys, tt.wantQueue) {
				t.Fatalf("queued regions = %v, want %v", r.mapRenderResourcePrewarmMapKeys, tt.wantQueue)
			}
			if got := r.mapRenderResourcePrewarmActive != nil; got != tt.wantActive {
				t.Fatalf("active loader = %v, want %v", got, tt.wantActive)
			}
		})
	}
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

func TestLazyStandeeOwnershipCoversCurrentRegionStates(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "forest"}

	tests := []struct {
		name            string
		resident        bool
		activeMapKey    string
		activeCancelled bool
		wantResident    bool
		wantActive      bool
	}{
		{name: "resident region", resident: true, wantResident: true},
		{name: "active region", activeMapKey: "forest", wantActive: true},
		{name: "resident and active transition", resident: true, activeMapKey: "forest", wantResident: true, wantActive: true},
		{name: "cancelled active region", activeMapKey: "forest", activeCancelled: true},
		{name: "different active region", activeMapKey: "desert"},
		{name: "no regional owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Renderer{}
			var resident *mapRenderRegionResources
			if tt.resident {
				resident = &mapRenderRegionResources{}
				r.mapRenderResourcesByMap = map[string]*mapRenderRegionResources{"forest": resident}
			}
			var activeResources *mapRenderRegionResources
			if tt.activeMapKey != "" {
				task := &mapRenderPrewarmTask{mapKey: tt.activeMapKey, cancelled: tt.activeCancelled}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				activeResources = task.prewarmer.resources
				r.mapRenderResourcePrewarmActive = task
			}

			key := standeeCoreKey{name: "mob:late_spawn"}
			r.trackResidentStandeeKey(key)
			r.trackResidentStandeeKey(key)

			residentOwned := false
			if resident != nil {
				_, residentOwned = resident.standees[key]
			}
			if residentOwned != tt.wantResident {
				t.Fatalf("resident ownership = %v, want %v", residentOwned, tt.wantResident)
			}
			activeOwned := false
			if activeResources != nil {
				_, activeOwned = activeResources.standees[key]
			}
			if activeOwned != tt.wantActive {
				t.Fatalf("active ownership = %v, want %v", activeOwned, tt.wantActive)
			}
		})
	}
}

func TestMapRenderTaskQueueRemovalClearsBackingEntries(t *testing.T) {
	t.Run("upload queue", func(t *testing.T) {
		tests := []struct {
			name            string
			makeQueue       func() []mapRenderUpload
			dropKey         string
			submit          bool
			withoutScreen   bool
			wantDraws       int
			wantUploads     int
			wantFirstMapKey string
		}{
			{
				name: "drop compacts retained task",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{
						{task: &mapRenderPrewarmTask{mapKey: "old"}},
						{task: &mapRenderPrewarmTask{mapKey: "keep"}},
						{task: &mapRenderPrewarmTask{mapKey: "old"}},
					}
				},
				dropKey: "old", wantUploads: 1, wantFirstMapKey: "keep",
			},
			{
				name: "nil screen leaves valid upload queued",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{{
						image: ebiten.NewImage(1, 1), task: &mapRenderPrewarmTask{mapKey: "wait"},
					}}
				},
				submit: true, withoutScreen: true, wantUploads: 1, wantFirstMapKey: "wait",
			},
			{
				name: "invalid entries are removed without drawing",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{
						{task: &mapRenderPrewarmTask{mapKey: "nil-image"}},
						{image: ebiten.NewImage(1, 1)},
						{image: ebiten.NewImage(1, 1), task: &mapRenderPrewarmTask{mapKey: "cancelled", cancelled: true}},
					}
				},
				submit: true,
			},
			{
				name: "valid entries draw directly and drain",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{
						{image: ebiten.NewImage(2, 3), task: &mapRenderPrewarmTask{mapKey: "first"}},
						{image: ebiten.NewImage(4, 5), task: &mapRenderPrewarmTask{mapKey: "second"}},
					}
				},
				submit: true, wantDraws: 2,
			},
			{
				name: "image count limit retains queue tail",
				makeQueue: func() []mapRenderUpload {
					queue := make([]mapRenderUpload, mapRenderUploadFrameImages+1)
					for i := range queue {
						queue[i] = mapRenderUpload{
							image: ebiten.NewImage(1, 1), task: &mapRenderPrewarmTask{mapKey: "batch"},
						}
					}
					queue[len(queue)-1].task.mapKey = "tail"
					return queue
				},
				submit: true, wantDraws: mapRenderUploadFrameImages,
				wantUploads: 1, wantFirstMapKey: "tail",
			},
			{
				name: "byte limit retains queue tail",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{
						{image: ebiten.NewImage(1024, 1024), task: &mapRenderPrewarmTask{mapKey: "first"}},
						{image: ebiten.NewImage(1024, 1024), task: &mapRenderPrewarmTask{mapKey: "second"}},
						{image: ebiten.NewImage(1, 1), task: &mapRenderPrewarmTask{mapKey: "tail"}},
					}
				},
				submit: true, wantDraws: 2, wantUploads: 1, wantFirstMapKey: "tail",
			},
			{
				name: "oversized first upload still makes progress",
				makeQueue: func() []mapRenderUpload {
					return []mapRenderUpload{
						{image: ebiten.NewImage(1536, 1536), task: &mapRenderPrewarmTask{mapKey: "oversized"}},
						{image: ebiten.NewImage(1, 1), task: &mapRenderPrewarmTask{mapKey: "tail"}},
					}
				},
				submit: true, wantDraws: 1, wantUploads: 1, wantFirstMapKey: "tail",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				backing := tt.makeQueue()
				images := make(map[*ebiten.Image]struct{})
				queued := make(map[*ebiten.Image]struct{})
				for _, upload := range backing {
					if upload.image != nil {
						images[upload.image] = struct{}{}
						queued[upload.image] = struct{}{}
					}
				}
				r := &Renderer{mapRenderUploadQueue: backing, mapRenderUploadQueued: queued}
				dst := &recordingMapRenderUploadDestination{}
				if tt.submit && tt.withoutScreen {
					r.drawMapRenderPrewarmUploads(nil)
				} else if tt.submit {
					r.submitMapRenderPrewarmUploads(dst)
				} else {
					r.dropMapRenderUploads(tt.dropKey)
				}
				for image := range images {
					image.Deallocate()
				}
				if len(dst.images) != tt.wantDraws {
					t.Fatalf("direct draws = %d, want %d", len(dst.images), tt.wantDraws)
				}
				for i, opts := range dst.options {
					if opts.ColorScale.A() != 0 {
						t.Fatalf("draw %d alpha = %v, want 0", i, opts.ColorScale.A())
					}
					bounds := dst.images[i].Bounds()
					x0, y0 := opts.GeoM.Apply(float64(bounds.Min.X), float64(bounds.Min.Y))
					x1, y1 := opts.GeoM.Apply(float64(bounds.Max.X), float64(bounds.Max.Y))
					if math.Abs((x1-x0)-1) > 1e-9 || math.Abs((y1-y0)-1) > 1e-9 {
						t.Fatalf("draw %d dimensions = %vx%v, want 1x1", i, x1-x0, y1-y0)
					}
				}
				if len(r.mapRenderUploadQueue) != tt.wantUploads {
					t.Fatalf("remaining uploads = %d, want %d", len(r.mapRenderUploadQueue), tt.wantUploads)
				}
				if tt.wantUploads > 0 && r.mapRenderUploadQueue[0].task.mapKey != tt.wantFirstMapKey {
					t.Fatalf("first retained upload map = %q, want %q", r.mapRenderUploadQueue[0].task.mapKey, tt.wantFirstMapKey)
				}
				for i := len(r.mapRenderUploadQueue); i < len(backing); i++ {
					if backing[i].image != nil || backing[i].task != nil {
						t.Fatalf("backing upload %d still retains a removed entry", i)
					}
				}
			})
		}
	})

	t.Run("shader queue", func(t *testing.T) {
		tests := []struct {
			name      string
			tasks     []*mapRenderPrewarmTask
			dropKey   string
			draw      bool
			wantTasks int
		}{
			{
				name: "drop compacts retained task",
				tasks: []*mapRenderPrewarmTask{
					{mapKey: "old"}, {mapKey: "keep"}, {mapKey: "old"},
				},
				dropKey: "old", wantTasks: 1,
			},
			{
				name: "draw releases popped task",
				tasks: []*mapRenderPrewarmTask{
					{mapKey: "first"}, {mapKey: "second"},
				},
				draw: true, wantTasks: 1,
			},
			{
				name: "draw skips and releases cancelled task",
				tasks: []*mapRenderPrewarmTask{
					{mapKey: "cancelled", cancelled: true}, {mapKey: "live"},
				},
				draw: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				backing := tt.tasks
				r := &Renderer{mapRenderShaderWarmTasks: backing}
				if tt.draw {
					screen := ebiten.NewImage(1, 1)
					r.drawMapRenderShaderWarm(screen)
					screen.Deallocate()
				} else {
					r.dropMapRenderUploads(tt.dropKey)
				}
				if len(r.mapRenderShaderWarmTasks) != tt.wantTasks {
					t.Fatalf("remaining shader tasks = %d, want %d", len(r.mapRenderShaderWarmTasks), tt.wantTasks)
				}
				if tt.draw {
					removed := len(backing) - tt.wantTasks
					for i := 0; i < removed; i++ {
						if backing[i] != nil {
							t.Fatalf("popped shader task %d is still retained", i)
						}
					}
				} else {
					for i := tt.wantTasks; i < len(backing); i++ {
						if backing[i] != nil {
							t.Fatalf("backing shader task %d still retains a removed entry", i)
						}
					}
				}
			})
		}
	})
}

func TestDrawMapRenderPrewarmUploadsUsesProductionDestination(t *testing.T) {
	img := ebiten.NewImage(1, 1)
	screen := ebiten.NewImage(1, 1)
	t.Cleanup(func() {
		img.Deallocate()
		screen.Deallocate()
	})
	r := &Renderer{
		mapRenderUploadQueue: []mapRenderUpload{{
			image: img,
			task:  &mapRenderPrewarmTask{mapKey: "production"},
		}},
		mapRenderUploadQueued: map[*ebiten.Image]struct{}{img: {}},
	}

	r.drawMapRenderPrewarmUploads(screen)

	if len(r.mapRenderUploadQueue) != 0 {
		t.Fatalf("production upload queue length = %d, want 0", len(r.mapRenderUploadQueue))
	}
	if _, queued := r.mapRenderUploadQueued[img]; queued {
		t.Fatal("production upload remained in deduplication set")
	}
}

func TestMapRenderRegionListRemovalClearsBackingStrings(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Renderer)
		list func(*Renderer) []string
	}{
		{
			name: "pending queue",
			run: func(r *Renderer) {
				r.cancelMapRenderPrewarmOutside(map[string]struct{}{"keep": {}})
			},
			list: func(r *Renderer) []string { return r.mapRenderResourcePrewarmMapKeys },
		},
		{
			name: "resident list",
			run: func(r *Renderer) {
				r.evictMapRenderResidencyOutside(map[string]struct{}{"keep": {}})
			},
			list: func(r *Renderer) []string { return r.mapRenderResidentMapKeys },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backing := []string{"old_a", "keep", "old_b"}
			r := &Renderer{
				mapRenderResourcePrewarmMapKeys: backing,
				mapRenderResidentMapKeys:        backing,
				mapRenderResourcesByMap: map[string]*mapRenderRegionResources{
					"old_a": testMapRenderResources(),
					"keep":  testMapRenderResources(),
					"old_b": testMapRenderResources(),
				},
			}
			tt.run(r)
			if got := tt.list(r); !reflect.DeepEqual(got, []string{"keep"}) {
				t.Fatalf("retained keys = %v, want [keep]", got)
			}
			for i := 1; i < len(backing); i++ {
				if backing[i] != "" {
					t.Fatalf("backing key %d = %q, want empty", i, backing[i])
				}
			}
		})
	}

	t.Run("pending dequeue", func(t *testing.T) {
		backing := []string{"next", "tail"}
		r := &Renderer{
			game:                            &MMGame{config: loadTestConfig(t), sprites: graphics.NewSpriteManager()},
			mapRenderResourcePrewarmMapKeys: backing,
		}
		r.startNextMapRenderPrewarm()
		if backing[0] != "" {
			t.Fatalf("dequeued backing key = %q, want empty", backing[0])
		}
		if got := r.mapRenderResourcePrewarmMapKeys; !reflect.DeepEqual(got, []string{"tail"}) {
			t.Fatalf("remaining pending keys = %v, want [tail]", got)
		}
		r.resetMapRenderResourceResidency()
	})
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

func TestActiveMapRenderOwnershipParticipatesInRetention(t *testing.T) {
	t.Chdir("../..")
	type resourceCase struct {
		name  string
		setup func(*Renderer, *mapRenderRegionResources, *mapRenderRegionResources) (func() bool, func())
	}
	resources := []resourceCase{
		{
			name: "standee",
			setup: func(r *Renderer, old, active *mapRenderRegionResources) (func() bool, func()) {
				key := standeeCoreKey{name: "mob:shared"}
				img := ebiten.NewImage(4, 4)
				r.standeeCoreCache = map[standeeCoreKey]*ebiten.Image{key: img}
				old.standees[key] = struct{}{}
				active.standees[key] = struct{}{}
				return func() bool { return r.standeeCoreCache[key] == img }, img.Deallocate
			},
		},
		{
			name: "source",
			setup: func(r *Renderer, old, active *mapRenderRegionResources) (func() bool, func()) {
				key := mapRenderSourceKey{name: "forest_oak"}
				img := r.game.sprites.GetSprite(key.name)
				old.sources[key] = struct{}{}
				active.sources[key] = struct{}{}
				return func() bool { return r.game.sprites.GetSprite(key.name) == img }, func() {
					r.game.sprites.EvictResource(key.name, "")
				}
			},
		},
		{
			name: "processed sprite",
			setup: func(r *Renderer, old, active *mapRenderRegionResources) (func() bool, func()) {
				key := processedSpriteKey{tileType: world.TileType3D(1234), spriteName: "shared"}
				img := ebiten.NewImage(4, 4)
				r.processedSpriteCache = map[processedSpriteKey]*ebiten.Image{key: img}
				old.processed[key] = struct{}{}
				active.processed[key] = struct{}{}
				return func() bool { return r.processedSpriteCache[key] == img }, img.Deallocate
			},
		},
		{
			name: "wall ripmap",
			setup: func(r *Renderer, old, active *mapRenderRegionResources) (func() bool, func()) {
				img := ebiten.NewImage(4, 4)
				r.wallRipmaps = map[*ebiten.Image]*wallRipmap{img: {}}
				old.walls[img] = struct{}{}
				active.walls[img] = struct{}{}
				return func() bool { return r.wallRipmaps[img] != nil }, img.Deallocate
			},
		},
		{
			name: "sky",
			setup: func(r *Renderer, old, active *mapRenderRegionResources) (func() bool, func()) {
				const name = "shared_sky"
				img := ebiten.NewImage(4, 4)
				r.game.skyPanoramaCache[name] = img
				old.skies[name] = struct{}{}
				active.skies[name] = struct{}{}
				return func() bool { return r.game.skyPanoramaCache[name] == img }, img.Deallocate
			},
		},
	}
	states := []struct {
		name        string
		keepActive  bool
		wantPresent bool
	}{
		{name: "continued active owner retains resource", keepActive: true, wantPresent: true},
		{name: "cancelled active owner releases resource", wantPresent: false},
	}
	for _, resource := range resources {
		for _, state := range states {
			t.Run(resource.name+"/"+state.name, func(t *testing.T) {
				g := &MMGame{
					sprites:          graphics.NewSpriteManager(),
					skyPanoramaCache: make(map[string]*ebiten.Image),
				}
				oldResources := testMapRenderResources()
				activeResources := testMapRenderResources()
				r := &Renderer{game: g}
				present, cleanup := resource.setup(r, oldResources, activeResources)
				t.Cleanup(cleanup)

				task := &mapRenderPrewarmTask{mapKey: "active"}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				task.prewarmer.resources = activeResources
				r.mapRenderResourcePrewarmActive = task
				keep := map[string]struct{}{"other": {}}
				if state.keepActive {
					r.mapRenderResidentMapKeys = []string{"old"}
					r.mapRenderResourcesByMap = map[string]*mapRenderRegionResources{"old": oldResources}
					keep["active"] = struct{}{}
				}

				r.cancelMapRenderPrewarmOutside(keep)
				r.evictMapRenderResidencyOutside(keep)

				if got := present(); got != state.wantPresent {
					t.Fatalf("resource present = %v, want %v", got, state.wantPresent)
				}
				if got := r.mapRenderResourcePrewarmActive != nil; got != state.keepActive {
					t.Fatalf("active owner retained = %v, want %v", got, state.keepActive)
				}
				r.resetMapRenderResourceResidency()
			})
		}
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
