package game

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/playerprofile"
	"ugataima/internal/threading"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
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
			exhausted := time.Since(started) >= r.loadingPace(paused).duration
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

// A background Update writes the configured number of chunks and no more: the
// GPU pays for them in the next Draw.
func TestLoadingBackgroundBoundsPixelWrites(t *testing.T) {
	for _, kind := range []string{"floor", "region", "floor, one step", "region, one step"} {
		t.Run(kind, func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			steps := gl.game.config.Graphics.Streaming.BackgroundStepsPerTick
			if strings.HasSuffix(kind, "one step") {
				steps = 1
				gl.game.config.Graphics.Streaming.BackgroundStepsPerTick = steps
				kind = strings.TrimSuffix(kind, ", one step")
			}
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
			// The step count is the ceiling; the 1.5 ms deadline may end the
			// Update sooner on a busy machine, never before one chunk.
			if bytes := uploaded(); bytes < mapRenderSpriteCommitChunkBytes || bytes > steps*mapRenderSpriteCommitChunkBytes || bytes%mapRenderSpriteCommitChunkBytes != 0 {
				t.Fatalf("background Update wrote %d bytes; want 1 to %d chunks of 256 KiB", bytes, steps)
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
				g.frameCount, g.uiFrameCount, g.actionInputStagger = 71, 31, 17
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
				if g.frameCount != 71 || g.actionInputStagger != 17 || g.party.Members[0].RTCooldown != 25 || g.party.Members[0].HitPoints != hp {
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

// A loading frame is a pause, not a scene change: a held mouse attack keeps its
// target through it, while a release or a fresh press landing on it ends the old
// hold and its block. A world-pausing overlay still blocks the hold, and the
// press that closes it stays claimed until release, loading or not.
func TestLoadingFrameKeepsHeldMouseAttack(t *testing.T) {
	for _, tc := range []struct {
		name string
		// pointer is the button per Update: Press, Hold or Release.
		pointer                 string
		held, oldBlocked, menu  bool
		closeOverlay            string // "live" or "loading": the first press closes the combat log
		wantBlocked, wantTarget bool
	}{
		{name: "hold through a loading frame", pointer: "HHH", held: true, wantTarget: true},
		{name: "fresh press on a loading frame", pointer: "PHH", oldBlocked: true},
		{name: "release and press again while loading", pointer: "RPH", held: true},
		{name: "hold while a menu opens", pointer: "HHH", held: true, menu: true, wantBlocked: true},
		{name: "press that closes an overlay", pointer: "PHH", closeOverlay: "live", wantBlocked: true},
		{name: "press that closes an overlay as loading begins", pointer: "PHH", closeOverlay: "loading", wantBlocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := loadingFixture(t)
			g, ih := gl.game, gl.inputHandler
			fp := installFakePointer(t)
			update := func() {
				if err := gl.Update(); err != nil {
					t.Fatal(err)
				}
			}
			ts := float64(g.config.GetTileSize())
			m := monster.NewMonster3DFromConfig(2.5*ts, 1.5*ts, "goblin", g.config)
			g.world.Monsters = []*monster.Monster3D{m}
			if tc.held {
				ih.mouseAttackTarget, ih.mouseAttackWorld = m, g.world
			}
			ih.mouseAttackBlocked = tc.oldBlocked
			g.menuOpen = tc.menu
			if tc.closeOverlay != "" {
				g.combat = NewCombatSystem(g) // the world runs once the overlay closes
				g.combatLogOpen = true
				update() // the paused scene blocks the button until release
				if !ih.mouseAttackBlocked || gl.loading.awaitingFrame {
					t.Fatal("fixture: an open overlay on a live frame must block the hold")
				}
				// What Draw presents: the overlay and its close button.
				gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot()
				gl.ui.beginDisplayedInput()
				gl.ui.onDisplayedInput(uiCommandClick, layoutRect{0, 0, 40, 40}, func() {
					g.combatLogOpen = false
					if tc.closeOverlay == "loading" {
						gl.loading.begin(time.Now())
					}
				})
				gl.ui.endDisplayedInput()
				fp.moveTo(10, 10)
			} else {
				gl.loading.begin(time.Now())
			}
			for i, edge := range tc.pointer {
				map[rune]func(){'P': fp.press, 'H': fp.hold, 'R': fp.release}[edge]()
				update()
				if i == 0 && tc.closeOverlay != "" {
					if g.combatLogOpen {
						t.Fatal("fixture: the press did not close the overlay")
					}
					gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot()
				}
			}
			if ih.mouseAttackBlocked != tc.wantBlocked {
				t.Fatalf("blocked = %v, want %v", ih.mouseAttackBlocked, tc.wantBlocked)
			}
			if (ih.mouseAttackTarget == m) != tc.wantTarget {
				t.Fatalf("held target kept = %v, want %v", ih.mouseAttackTarget == m, tc.wantTarget)
			}
		})
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
	// The profile observes the game at the end of every Update, after the
	// tick's deferred lookups are over.
	profile, err := playerprofile.Open(filepath.Join(t.TempDir(), "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	gl.game.playerProfile = profile
	defer profile.Close()
	if err := gl.Update(); err != nil {
		t.Fatal(err)
	}
	if !gl.loading.awaitingFrame || !gl.loading.stream.Pending() {
		t.Fatal("cold actor animation reached gameplay without the loading gate")
	}
	for _, animation := range []string{"attacking_r", "attacking_l"} {
		req := graphics.SpriteResourceRequest{Name: mon.GetSpriteType(), AnimationType: animation}
		if len(gl.game.sprites.ResourceImages(req)) != 0 {
			t.Fatalf("%s was decoded inside the Update instead of by the loading worker", animation)
		}
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
			g.drag.armed, g.stashDrag.armed = true, true
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
			if !pickedUp && (g.drag.armed || g.stashDrag.armed) {
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

// Play keeps its quotas; the loading screen takes the configured limits and
// never falls below play's, even with an empty loading_screen block.
func TestLoadingPaceByMode(t *testing.T) {
	gl := loadingFixture(t)
	r := gl.renderer
	streaming := &gl.game.config.Graphics.Streaming
	play := r.loadingPace(false)
	want := loadingPace{
		steps: streaming.BackgroundStepsPerTick, duration: mapRenderDerivedFrameBudget,
		decoders: mapRenderPlayDecoders, standeeWorkers: mapRenderPlayStandeeWorkers,
		queueBytes: mapRenderPlayQueueBytes, uploads: mapRenderPlayUploads,
	}
	if play != want {
		t.Fatalf("play pace %+v, want %+v", play, want)
	}
	screen := streaming.LoadingScreen
	held := r.loadingPace(true)
	if held.steps != 0 || !held.waitForWorkers || held.duration != mapRenderHeldUpdateBudget {
		t.Fatalf("held frame does not work to its deadline and wait for workers: %+v", held)
	}
	if held.decoders != screen.WorkerCount() || held.standeeWorkers != screen.WorkerCount() ||
		held.queueBytes != int64(screen.QueueMegabytes)<<20 ||
		held.uploads != (mapRenderUploadLimit{int64(screen.UploadMegabytesPerFrame) << 20, screen.UploadImagesPerFrame}) {
		t.Fatalf("held pace %+v ignores loading_screen %+v", held, screen)
	}
	streaming.LoadingScreen = config.LoadingScreenConfig{Workers: 1}
	if floor := r.loadingPace(true); floor.decoders != play.decoders || floor.standeeWorkers != play.standeeWorkers ||
		floor.queueBytes != play.queueBytes || floor.uploads != play.uploads {
		t.Fatalf("a loading screen below the play quotas: %+v", floor)
	}
}

// Switching the mode moves the worker gates, the active region's queue and the
// frame's upload quota together, in both directions.
func TestLoadingPaceSwitchesLiveLimits(t *testing.T) {
	gl := loadingFixture(t)
	r := gl.renderer
	task := &mapRenderPrewarmTask{mapKey: currentMapKey(), queueBudget: graphics.NewPreparationBudget(mapRenderPlayQueueBytes)}
	r.mapRenderResourcePrewarmActive = task
	t.Cleanup(r.resetMapRenderResourceResidency)
	// One byte held, then a reservation larger than the play queue.
	secondFits := func() bool {
		first, _ := task.queueBudget.Acquire(context.Background(), 1)
		defer first.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		second, ok := task.queueBudget.Acquire(ctx, mapRenderPlayQueueBytes)
		second.Release()
		return ok
	}
	for _, held := range []bool{false, true, false} {
		gl.loading.awaitingFrame = held
		pace := r.applyLoadingPace(held)
		if r.decodeGate.Limit() != pace.decoders || r.standeeGate.Limit() != pace.standeeWorkers {
			t.Fatalf("held=%v: gates admit %d and %d, pace wants %d and %d", held, r.decodeGate.Limit(), r.standeeGate.Limit(), pace.decoders, pace.standeeWorkers)
		}
		if r.uploadLimit() != pace.uploads {
			t.Fatalf("held=%v: frame uploads %+v, pace wants %+v", held, r.uploadLimit(), pace.uploads)
		}
		if got, want := secondFits(), pace.queueBytes > mapRenderPlayQueueBytes; got != want {
			t.Fatalf("held=%v: the region queue admits past the play bound = %v, want %v", held, got, want)
		}
	}
}

// A held frame waits for the active region's workers: a delivery or a closed
// queue lets the region advance, a queue with nothing live returns at once.
func TestAwaitMapRenderWorker(t *testing.T) {
	far := func() time.Time { return time.Now().Add(10 * time.Second) }
	for _, tc := range []struct {
		name    string
		prepare func(task *mapRenderPrewarmTask)
		until   func() time.Time
		want    bool
		check   func(task *mapRenderPrewarmTask) bool
	}{
		{
			name: "sprite delivered later",
			prepare: func(task *mapRenderPrewarmTask) {
				sprites := make(chan graphics.PreparedSpriteResource)
				task.preparedSprites, task.skiesDone = sprites, true
				cpu := image.NewRGBA(image.Rect(0, 0, 8, 8))
				go func() {
					sprites <- graphics.PreparedSpriteResource{Request: graphics.SpriteResourceRequest{Name: "late"}, CPU: cpu, Image: cpu, Found: true}
				}()
			},
			until: far, want: true,
			check: func(task *mapRenderPrewarmTask) bool { return task.spriteCommit != nil },
		},
		{
			name: "sprite queue closes",
			prepare: func(task *mapRenderPrewarmTask) {
				sprites := make(chan graphics.PreparedSpriteResource)
				task.preparedSprites, task.skiesDone = sprites, true
				go close(sprites)
			},
			until: far, want: true,
			check: func(task *mapRenderPrewarmTask) bool { return task.spritesDone && task.sourcesChanged },
		},
		{
			name: "standee delivered later",
			prepare: func(task *mapRenderPrewarmTask) {
				standees := make(chan mapRenderPreparedStandee)
				task.preparedStandees, task.spritesDone, task.skiesDone = standees, true, true
				go func() { standees <- mapRenderPreparedStandee{key: standeeCoreKey{name: "mob:late"}} }()
			},
			until: far, want: true,
			check: func(task *mapRenderPrewarmTask) bool { return task.standeeCommit != nil },
		},
		{
			name:    "no live queue",
			prepare: func(task *mapRenderPrewarmTask) { task.spritesDone, task.skiesDone = true, true },
			until:   far, want: false,
		},
		{
			name: "deadline passes",
			prepare: func(task *mapRenderPrewarmTask) {
				task.preparedSprites, task.skiesDone = make(chan graphics.PreparedSpriteResource), true
			},
			until: func() time.Time { return time.Now().Add(time.Millisecond) }, want: false,
			check: func(task *mapRenderPrewarmTask) bool { return task.spriteCommit == nil && !task.spritesDone },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := loadingFixture(t)
			r := gl.renderer
			task := &mapRenderPrewarmTask{mapKey: currentMapKey(), cpuImages: make(map[*ebiten.Image]*image.RGBA)}
			task.prewarmer = newMapRenderPrewarmer(r, task)
			r.mapRenderResourcePrewarmActive = task
			t.Cleanup(r.resetMapRenderResourceResidency)
			tc.prepare(task)
			started := time.Now()
			if got := r.awaitMapRenderWorker(tc.until()); got != tc.want {
				t.Fatalf("await = %v, want %v", got, tc.want)
			}
			if time.Since(started) > 5*time.Second {
				t.Fatal("await sat out its deadline")
			}
			if tc.check != nil && !tc.check(task) {
				t.Fatal("the worker result did not reach the region")
			}
		})
	}
}
