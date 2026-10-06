package game

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

func loadingFixture(t *testing.T) *GameLoop {
	t.Helper()
	cfg := loadTestConfig(t)
	setTestWorldManager(t, nil)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.appScreen = AppScreenInGame
	g.audioSliderDrag = -1
	g.sprites = graphics.NewSpriteManager()
	g.threading = threading.NewThreadingComponents(nil)
	r := &Renderer{game: g, mapRenderResourcesByMap: map[string]*mapRenderRegionResources{currentMapKey(): {}}}
	gl := &GameLoop{game: g, renderer: r, ui: NewUISystem(g), inputHandler: NewInputHandler(g)}
	g.gameLoop = gl
	gl.ensureResourceLoading()
	t.Cleanup(gl.closeResourceLoading)
	return gl
}

func TestLoadingPreparationDrainsReadySources(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(fmt.Sprintf("paused=%v", paused), func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			gl.loading.awaitingFrame = paused
			prepared := make(chan graphics.PreparedSpriteResource, 2)
			task := &mapRenderPrewarmTask{
				mapKey: currentMapKey(), skiesDone: true,
				preparedSprites: prepared, cpuImages: make(map[*ebiten.Image]*image.RGBA),
				// Keep publication pending after the source batch. Readiness must
				// still include the unfinished derived resources.
				steps: []mapRenderPrewarmStep{func(time.Time) bool { return false }},
			}
			task.prewarmer = newMapRenderPrewarmer(r, task)
			r.mapRenderResourcePrewarmActive = task
			t.Cleanup(r.resetMapRenderResourceResidency)
			for i := 0; i < cap(prepared); i++ {
				req := graphics.SpriteResourceRequest{Name: fmt.Sprintf("ready-%d", i)}
				gl.game.sprites.ResourceImages(req)
				cpu := image.NewRGBA(image.Rect(0, 0, 8, 8))
				prepared <- graphics.PreparedSpriteResource{Request: req, CPU: cpu, Image: cpu, Found: true}
			}
			close(prepared)
			started := time.Now()
			gl.advanceResourceLoading()
			_, budget := loadingPreparationBudget(paused)
			exhausted := time.Since(started) >= budget
			if !paused {
				// Background preparation spreads work over successive Updates.
				for range 3 {
					gl.advanceResourceLoading()
				}
			}
			for i := 0; i < cap(prepared); i++ {
				req := graphics.SpriteResourceRequest{Name: fmt.Sprintf("ready-%d", i)}
				if !exhausted && len(gl.game.sprites.ResourceImages(req)) != 1 {
					t.Fatal("ready sources did not finish within the mode's preparation budget")
				}
			}
			if r.mapRenderResourcePrewarmActive != task {
				t.Fatal("unfinished derived work was published")
			}
		})
	}
}

func TestLoadingBackgroundBoundsPixelWrites(t *testing.T) {
	for _, kind := range []string{"floor", "region"} {
		t.Run(kind, func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			cpu := image.NewRGBA(image.Rect(0, 0, 512, 1024))
			img := ebiten.NewImage(512, 1024)
			_, cancel := context.WithCancel(context.Background())
			var uploaded func() int
			if kind == "floor" {
				p := &floorPreparation{cancel: cancel, prepared: &preparedFloor{pixels: cpu}, image: img}
				r.floorPreparation = p
				uploaded = func() int { return p.row * cpu.Stride }
			} else {
				commit := &mapRenderSkyCommit{name: "large", cpu: cpu, image: img}
				task := &mapRenderPrewarmTask{mapKey: currentMapKey(), cancel: cancel, skyCommit: commit}
				task.prewarmer = newMapRenderPrewarmer(r, task)
				r.mapRenderResourcePrewarmActive = task
				uploaded = func() int { return commit.row * cpu.Stride }
			}
			gl.advanceResourceLoading()
			if bytes := uploaded(); bytes != 256<<10 {
				t.Fatalf("background Update wrote %d bytes; want one 256 KiB chunk", bytes)
			}
		})
	}
}

func TestLoadingRegionHandoffWaitsForNextUpdate(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(fmt.Sprintf("paused=%v", paused), func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			gl.loading.awaitingFrame = paused
			task := &mapRenderPrewarmTask{
				mapKey: "finished", spritesDone: true, skiesDone: true,
				steps: []mapRenderPrewarmStep{func(time.Time) bool { return true }},
			}
			task.prewarmer = newMapRenderPrewarmer(r, task)
			// Warm the shared shaders first; their first compilation must not
			// exhaust the deadline and accidentally hide a same-Update handoff.
			r.finalizeMapRenderPrewarm(task)
			r.mapRenderResourcePrewarmActive = task
			r.scheduleMapRenderResourcePrewarm("next")
			gl.advanceResourceLoading()
			if !containsString(r.mapRenderResidentMapKeys, "finished") || r.mapRenderResourcePrewarmActive != nil || !containsString(r.mapRenderResourcePrewarmMapKeys, "next") {
				t.Fatal("region handoff was lost or started its successor in the same Update")
			}
			gl.advanceResourceLoading()
			if containsString(r.mapRenderResourcePrewarmMapKeys, "next") {
				t.Fatal("the next Update did not resume the queued region")
			}
		})
	}
}

func TestLoadingUpdateFreezesSimulationAndRetiresInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, stage := range []string{"worker", "upload", "floor", "fresh_frame"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, stage), func(t *testing.T) {
				gl := loadingFixture(t)
				g := gl.game
				g.turnBasedMode = tb
				g.frameCount, g.uiFrameCount, g.spellInputCooldown = 71, 31, 17
				g.party.Members[0].RTCooldown = 25
				g.screenBannerQueue = []screenBanner{{text: "Quest news", frame: 13}}
				g.mouseLeftClicks = []queuedClick{{x: 10, y: 10}}
				g.mouseRightClicks = []queuedClick{{x: 10, y: 10}}
				gl.ui.displayedInput.ready = true
				gl.loading.begin(time.Now())
				switch stage {
				case "worker":
					gl.loading.pattern = make(chan struct{})
				case "upload":
					gl.loading.uploads = []*ebiten.Image{ebiten.NewImage(1, 1)}
				case "floor":
					_, cancel := context.WithCancel(context.Background())
					gl.renderer.floorPreparation = &floorPreparation{cancel: cancel, result: make(chan preparedFloor)}
				}
				hp := g.party.Members[0].HitPoints
				for i := 0; i < 3; i++ {
					if err := gl.Update(); err != nil {
						t.Fatal(err)
					}
				}
				if g.frameCount != 71 || g.spellInputCooldown != 17 || g.party.Members[0].RTCooldown != 25 || g.party.Members[0].HitPoints != hp {
					t.Fatal("loading advanced world clocks or combat")
				}
				if g.uiFrameCount != 34 {
					t.Fatal("loading froze the interface clock")
				}
				if len(g.mouseLeftClicks)+len(g.mouseRightClicks) != 0 || gl.ui.displayedInput.ready || g.worldClickAllowed() {
					t.Fatal("loading retained actionable input")
				}
				if g.screenBannerQueue[0].frame != 13 {
					t.Fatal("loading consumed quest news")
				}
				if !gl.loading.awaitingFrame {
					t.Fatal("Update resumed without presenting a fresh frame")
				}
			})
		}
	}
}

func TestLoadingRegionReadinessIncludesOnlyRequiredSubmissions(t *testing.T) {
	for _, kind := range []string{"resident", "required_upload", "nearby_upload", "required_shader", "nearby_shader"} {
		t.Run(kind, func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			key := currentMapKey()
			if kind == "nearby_upload" || kind == "nearby_shader" {
				key = "neighbour"
			}
			task := &mapRenderPrewarmTask{mapKey: key}
			if kind == "required_upload" || kind == "nearby_upload" {
				r.mapRenderUploadQueue = []mapRenderUpload{{task: task}}
			}
			if kind == "required_shader" || kind == "nearby_shader" {
				r.mapRenderShaderWarmTasks = []*mapRenderPrewarmTask{task}
			}
			want := kind != "required_upload" && kind != "required_shader"
			if r.loadingRegionsReady() != want {
				t.Fatal("required/speculative readiness is incorrect")
			}
		})
	}
}

func TestLoadingBannerLifetime(t *testing.T) {
	start := time.Unix(100, 0)
	for _, tt := range []struct {
		name              string
		elapsed, finished time.Duration
		visible           bool
	}{
		{"immediate", 0, 0, false}, {"below_delay", 199 * time.Millisecond, 0, false},
		{"delayed", 260 * time.Millisecond, 0, true}, {"long", time.Second, 0, true},
		{"fast_complete", 150 * time.Millisecond, 100 * time.Millisecond, false},
		{"fade", time.Second + 60*time.Millisecond, time.Second, true},
		{"settled", 2 * time.Second, time.Second, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := &gameLoadingState{}
			l.begin(start)
			l.begin(start.Add(50 * time.Millisecond))
			if !l.started.Equal(start) {
				t.Fatal("related demand restarted the delay")
			}
			if tt.finished != 0 {
				l.finished = start.Add(tt.finished)
			}
			alpha := l.bannerAlpha(start.Add(tt.elapsed))
			if (alpha > 0) != tt.visible || alpha < 0 || alpha > 1 {
				t.Fatalf("unexpected alpha %v", alpha)
			}
		})
	}
}

// Streamed pixels wait in the world-pass stash; a reload that resets residency
// before the next pass (save-scumming during the loading banner) must drop
// them, or every reload leaks one batch of decoded sprites.
func TestLoadingGenerationAndExitCancelOwnedWork(t *testing.T) {
	for _, entry := range []string{"generation", "title", "shutdown"} {
		t.Run(entry, func(t *testing.T) {
			gl := loadingFixture(t)
			old := gl.loading.stream
			old.Request(graphics.SpriteResourceRequest{Name: "obsolete"})
			streamed := ebiten.NewImage(1, 1)
			defer streamed.Deallocate()
			gl.renderer.observeLazySpriteLoad(graphics.SpriteResourceRequest{Name: "streamed"},
				map[*ebiten.Image]*image.RGBA{streamed: image.NewRGBA(image.Rect(0, 0, 1, 1))})
			if len(gl.renderer.lazySpriteCPUPixels) != 1 {
				t.Fatal("streamed pixels never reached the stash (positive control)")
			}
			ctx, cancel := context.WithCancel(context.Background())
			gl.renderer.mapRenderResourcePrewarmActive = &mapRenderPrewarmTask{ctx: ctx, cancel: cancel}
			gl.loading.begin(time.Now())
			switch entry {
			case "generation":
				gl.renderer.resetMapRenderResourceResidency()
				gl.ensureResourceLoading()
				if gl.loading.stream == old || !gl.loading.awaitingFrame {
					t.Fatal("map reset lost fresh-frame barrier")
				}
			case "title":
				gl.game.appScreen = AppScreenMainMenu
				if err := gl.Update(); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				gl.game.Shutdown()
			}
			if old.Pending() || ctx.Err() == nil {
				t.Fatal("obsolete resource work survived its owner")
			}
			if n := len(gl.renderer.lazySpriteCPUPixels); n != 0 {
				t.Fatalf("%d streamed pixel buffers survived their owner", n)
			}
		})
	}
}

func TestFloorLoadingPublishesAtomicallyAndCancelsPartialImages(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelEarly), func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			cpu := image.NewRGBA(image.Rect(0, 0, 8, 8))
			result := make(chan preparedFloor, 1)
			result <- preparedFloor{pixels: cpu, groups: map[string]floorTextureGroup{}, count: 1, width: 8, height: 8}
			ctx, cancel := context.WithCancel(context.Background())
			r.floorPreparation = &floorPreparation{key: "new", cancel: cancel, result: result}
			r.floorTexturesKey = "old"
			r.advanceFloorPreparation(32)
			if r.floorTexturesKey != "old" || r.floorPreparation == nil || r.floorPreparation.row != 1 {
				t.Fatal("partial floor atlas became visible")
			}
			if cancelEarly {
				r.cancelFloorPreparation()
			} else {
				for r.floorPreparation != nil {
					r.advanceFloorPreparation(32)
				}
				if r.floorTexturesKey != "new" || r.floorTexAtlas == nil || len(gl.loading.uploads) != 1 {
					t.Fatal("finished floor atlas was not submitted")
				}
			}
			if ctx.Err() == nil {
				t.Fatal("floor worker context survived completion/cancellation")
			}
			if cancelEarly && r.floorTexturesKey != "old" {
				t.Fatal("cancelled floor changed the atlas")
			}
		})
	}
}

func TestLoadingPrioritizesRequiredRegions(t *testing.T) {
	r := &Renderer{mapRenderResourcePrewarmMapKeys: []string{"nearby", "visible", "distant"}}
	r.prioritizeLoadingRegions([]string{"visible"})
	if !reflect.DeepEqual(r.mapRenderResourcePrewarmMapKeys, []string{"visible", "nearby", "distant"}) {
		t.Fatal("speculative region stayed ahead of required work")
	}
}

func TestFloorLoadingWorkerAndCachedReturn(t *testing.T) {
	for _, kind := range []string{"decode", "missing", "cached_return"} {
		t.Run(kind, func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			path := filepath.Join(t.TempDir(), "floor.png")
			if kind != "missing" {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			r.startFloorPreparation("new", map[string][]string{"ground": {path}})
			if r.floorTexAtlas != nil {
				t.Fatal("floor worker published synchronously")
			}
			if kind == "cached_return" {
				wm := world.NewWorldManager(gl.game.config)
				wm.CurrentMapKey = "cached"
				wm.MapConfigs["cached"] = &config.MapConfig{Biome: "old"}
				wm.Biomes["old"] = config.BiomeConfig{FloorTextureGroups: map[string][]string{"ground": {path}}}
				setTestWorldManager(t, wm)
				r.floorTexAtlas = ebiten.NewImage(8, 8)
				r.floorTexturesKey = "old"
				r.loadCurrentMapFloorTextures()
				if r.floorPreparation != nil || r.floorTexturesKey != "old" {
					t.Fatal("return to cached biome retained obsolete floor job")
				}
				return
			}
			deadline := time.Now().Add(5 * time.Second)
			for r.floorPreparation != nil && time.Now().Before(deadline) {
				r.advanceFloorPreparation(32)
				time.Sleep(time.Millisecond)
			}
			if r.floorPreparation != nil || r.floorTexturesKey != "new" {
				t.Fatal("floor decode never settled")
			}
			if (r.floorTexAtlas != nil) != (kind == "decode") {
				t.Fatal("floor decode/failure did not publish expected fallback")
			}
		})
	}
}

func TestLoadingPreflightsDynamicCombatAnimation(t *testing.T) {
	gl := loadingFixture(t)
	t.Chdir("../..")
	mon := monster.NewMonster3DFromConfig(64, 64, "goblin", gl.game.config)
	gl.game.world.Monsters = []*monster.Monster3D{mon}
	gl.game.mainMenuOpen = true
	if err := gl.Update(); err != nil {
		t.Fatal(err)
	}
	if !gl.loading.awaitingFrame || !gl.loading.stream.Pending() {
		t.Fatal("cold actor animation reached gameplay without the loading gate")
	}
	deadline := time.Now().Add(5 * time.Second)
	for gl.loading.stream.Pending() && time.Now().Before(deadline) {
		gl.advanceResourceLoading()
		time.Sleep(time.Millisecond)
	}
	if gl.loading.stream.Pending() {
		t.Fatal("actor resources never completed")
	}
	if gl.game.authoredMonsterAttackFrameCount(mon) == 0 {
		t.Fatal("authored combat timing did not survive asynchronous preparation")
	}
}

func TestLoadingRetiresDragActionsWithoutLosingPickedUpFragments(t *testing.T) {
	for _, pickedUp := range []bool{false, true} {
		t.Run(fmt.Sprintf("picked_up=%v", pickedUp), func(t *testing.T) {
			gl := loadingFixture(t)
			g := gl.game
			g.dragPickedUp, g.stashDragPickedUp = pickedUp, pickedUp
			g.dragArmed, g.stashDragArmed = true, true
			g.dragDropAt, g.stashDragDrop = 1, true
			gl.loading.begin(time.Now())
			if err := gl.Update(); err != nil {
				t.Fatal(err)
			}
			if g.dragDropAt != 0 || g.stashDragDrop {
				t.Fatal("loading retained a pending inventory action")
			}
			if g.dragPickedUp != pickedUp || g.stashDragPickedUp != pickedUp {
				t.Fatal("loading lost ownership of a carried fragment")
			}
			if !pickedUp && (g.dragArmed || g.stashDragArmed) {
				t.Fatal("ordinary gesture survived loading")
			}
		})
	}
}

func TestLoadingRetainsReusedSourcePixelsAcrossRenderMisses(t *testing.T) {
	gl := loadingFixture(t)
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("assets/sprites/environment", 0755); err != nil {
		t.Fatal(err)
	}
	var sources []*ebiten.Image
	var expected [][]byte
	for i, name := range []string{"first", "second"} {
		cpu := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for p := range cpu.Pix {
			cpu.Pix[p] = byte(100 + i*40)
		}
		file, err := os.Create("assets/sprites/environment/" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, cpu); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	for _, name := range []string{"first", "second"} {
		req := graphics.SpriteResourceRequest{Name: name}
		for prepared := range gl.game.sprites.PrepareResources(context.Background(), []graphics.SpriteResourceRequest{req}) {
			expected = append(expected, append([]byte(nil), prepared.CPU.Pix...))
			gl.game.sprites.CommitPreparedResource(prepared)
		}
		sources = append(sources, gl.game.sprites.ResourceImages(req)[0])
	}
	r := gl.renderer
	for pass := 0; pass <= len(sources); pass++ {
		missed := false
		func() {
			gl.loading.rendering = true
			defer func() {
				gl.loading.rendering = false
				if caught := recover(); caught != nil {
					if _, ok := caught.(loadingRenderMiss); !ok {
						panic(caught)
					}
					missed = true
				}
			}()
			r.withMapRenderSourceTracking(func() {
				for i, source := range sources {
					pixels := make([]byte, len(expected[i]))
					r.readRenderPixels(source, pixels)
					if !reflect.DeepEqual(pixels, expected[i]) {
						t.Fatal("reused source pixels changed")
					}
				}
			})
		}()
		if missed != (pass < len(sources)) || r.loadDiagnostics.readbacks != 0 {
			t.Fatalf("pass %d: missed=%v GPU readbacks=%d", pass, missed, r.loadDiagnostics.readbacks)
		}
		deadline := time.Now().Add(5 * time.Second)
		for gl.loading.stream.Pending() && time.Now().Before(deadline) {
			gl.advanceResourceLoading()
			time.Sleep(time.Millisecond)
		}
		if gl.loading.stream.Pending() {
			t.Fatal("CPU request never completed")
		}
	}
	if len(r.lazySpriteCPUPixels) != 0 {
		t.Fatal("completed world pass retained temporary CPU pixels")
	}
	// Both resident sources now need a new consumer. Discovery must collect
	// their missing CPU copies before starting one bounded worker batch.
	r.scheduleMapRenderResourcePrewarm("batch")
	r.startNextMapRenderPrewarm()
	task := r.mapRenderResourcePrewarmActive
	task.plan = mapRenderPrewarmPlan{wallSprites: []string{"first", "second"}}
	sawBatch := false
	deadline := time.Now().Add(5 * time.Second)
	for r.mapRenderResourcePrewarmActive != nil && time.Now().Before(deadline) {
		r.prewarmPendingMapRenderResources()
		pending := 0
		for _, complete := range task.prewarmer.cpuRequested {
			if !complete {
				pending++
			}
		}
		sawBatch = sawBatch || pending == 2
		time.Sleep(time.Millisecond)
	}
	if !sawBatch || r.mapRenderResourcePrewarmActive != nil || r.loadDiagnostics.readbacks != 0 {
		t.Fatal("CPU requests did not finish as a shared batch without GPU readback")
	}
	for _, source := range sources {
		if rm := r.wallRipmaps[source]; rm == nil || rm.building || len(rm.owned) == 0 {
			t.Fatal("batched decode did not publish wall filtering data")
		}
	}
	r.resetMapRenderResourceResidency()
}
