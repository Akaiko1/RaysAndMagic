package game

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestNightMotesComeOnlyFromTileConfig(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	tm := world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalTileManager = tm

	for _, key := range tm.GetAllTileKeys() {
		tile := tm.GetTileDataByKey(key)
		got, configured := nightMotePaletteForConfig(tile)
		if tile.NightMotes == nil {
			if configured {
				t.Errorf("tile %q emitted night motes without authored config", key)
			}
			continue
		}
		if !configured || got.glow != tile.NightMotes.GlowColor || got.core != tile.NightMotes.CoreColor {
			t.Errorf("tile %q night motes = %+v, configured=%v; want authored colors", key, got, configured)
		}
	}
}

func TestNightMoteFliesOneTileForConfiguredLifetime(t *testing.T) {
	cfg := loadTestConfig(t)
	tps := cfg.GetTPS()
	tileSize := float64(cfg.GetTileSize())
	lifeTicks := int64(math.Round(cfg.Graphics.NightMotes.LifetimeSeconds * float64(tps)))
	fly := nightMote{
		startX: 32, startY: 32,
		targetX: 96, targetY: 32,
		bornTick: 10,
		dieTick:  10 + lifeTicks,
		phase:    0.4,
	}
	if got := fly.dieTick - fly.bornTick; got != lifeTicks {
		t.Fatalf("lifetime = %d ticks, want configured %d", got, lifeTicks)
	}
	mid, ok := fly.pose((fly.bornTick+fly.dieTick)/2, tileSize)
	if !ok || mid.x <= fly.startX || mid.x >= fly.targetX || mid.alpha <= 0 {
		t.Fatalf("mid-flight pose = %+v, %v", mid, ok)
	}
	last, ok := fly.pose(fly.dieTick-1, tileSize)
	if !ok || math.Hypot(last.x-fly.targetX, last.y-fly.targetY) > tileSize*0.02 {
		t.Fatalf("final pose = %+v, want arrival near adjacent tile center", last)
	}
	if _, ok := fly.pose(fly.dieTick, tileSize); ok {
		t.Fatal("mote remained alive after its configured lifetime")
	}
}

func TestNightMoteSizeRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		scale := randomNightMoteSizeScale()
		if scale < 0.70 || scale >= 1.30 {
			t.Fatalf("random size scale = %v, want in [0.70, 1.30)", scale)
		}
	}
}

func nightMoteTreeTestRenderer(t *testing.T, active int) (*Renderer, nightMoteTreeID) {
	t.Helper()
	loaded := loadTestConfig(t)
	cfgCopy := *loaded
	cfg := &cfgCopy
	cfg.Graphics.NightMotes.EmissionChance = 1
	w := newTestWorldSized(cfg, 5, 5)
	g := newTestGame(cfg, w)
	g.camera.Angle = 0
	g.camera.FOV = squareProjectionFOV(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g.camera.ViewDist = 24 * float64(cfg.GetTileSize())
	g.renderHelper = NewRenderingHelper(g)
	tileSize := float64(cfg.GetTileSize())
	x, y := TileCenterFromTile(2, 1, tileSize)
	id := nightMoteTreeID{tileX: 2, tileY: 1}
	palette := nightMotePalette{glow: [3]int{1, 2, 3}, core: [3]int{4, 5, 6}}
	r := &Renderer{
		game:       g,
		nightMotes: make([]nightMote, 0, cfg.Graphics.NightMotes.MaxActive),
		treeTilesCache: []TransparentSpriteData{{
			tileX: 2, tileY: 1, worldX: x, worldY: y,
			emitsNightMotes: true, nightMotePalette: palette,
		}},
		nightMoteNextByTree: map[nightMoteTreeID]int64{id: 1},
	}
	for i := 0; i < active; i++ {
		r.nightMotes = append(r.nightMotes, nightMote{
			bornTick: 0, dieTick: 1000, source: id, palette: palette,
		})
	}
	return r, id
}

func TestNightMotePerTreeCap(t *testing.T) {
	r, id := nightMoteTreeTestRenderer(t, 0)
	maxPerTree := r.game.config.Graphics.NightMotes.MaxPerTree
	for i := 0; i < maxPerTree-1; i++ {
		r.nightMotes = append(r.nightMotes, nightMote{bornTick: 0, dieTick: 1000, source: id})
	}
	r.updateNightMoteTrees(1)
	if got := len(r.nightMotes); got != maxPerTree {
		t.Fatalf("active motes after filling tree = %d, want %d", got, maxPerTree)
	}

	r.nightMoteNextByTree[id] = 2
	r.updateNightMoteTrees(2)
	if got := len(r.nightMotes); got != maxPerTree {
		t.Fatalf("tree emitted beyond per-tree cap: %d active, want %d", got, maxPerTree)
	}
}

func TestNightMoteAttemptsEmissionEveryConfiguredInterval(t *testing.T) {
	r, id := nightMoteTreeTestRenderer(t, 0)
	startTick := int64(100)
	interval := r.nightMoteSpawnInterval()
	r.nightMoteNextByTree[id] = startTick

	r.updateNightMoteTrees(startTick)
	if len(r.nightMotes) != 1 {
		t.Fatalf("tree emitted %d motes on a due roll with chance=1, want 1", len(r.nightMotes))
	}
	r.updateNightMoteTrees(startTick + interval - 1)
	if len(r.nightMotes) != 1 {
		t.Fatal("tree emitted again before the configured interval elapsed")
	}
	r.updateNightMoteTrees(startTick + interval)
	if len(r.nightMotes) != 2 {
		t.Fatalf("tree emitted %d total motes after the next interval, want 2", len(r.nightMotes))
	}
}

func TestNightMoteZeroChanceNeverEmits(t *testing.T) {
	r, id := nightMoteTreeTestRenderer(t, 0)
	r.game.config.Graphics.NightMotes.EmissionChance = 0
	tick := int64(100)
	r.nightMoteNextByTree[id] = tick
	r.updateNightMoteTrees(tick)
	if len(r.nightMotes) != 0 {
		t.Fatalf("zero emission chance produced %d motes", len(r.nightMotes))
	}
	if want := tick + r.nightMoteSpawnInterval(); r.nightMoteNextByTree[id] != want {
		t.Fatalf("zero-chance tree next roll = %d, want %d", r.nightMoteNextByTree[id], want)
	}
}

func TestNightMoteInitialScheduleFallsWithinOneInterval(t *testing.T) {
	r, id := nightMoteTreeTestRenderer(t, 0)
	interval := r.nightMoteSpawnInterval()
	offsets := make(map[int64]struct{})
	for i := int64(0); i < 16; i++ {
		tick := 100 + i*interval
		delete(r.nightMoteNextByTree, id)
		r.updateNightMoteTrees(tick)
		next := r.nightMoteNextByTree[id]
		if next < tick || next >= tick+interval {
			t.Fatalf("initial night-mote roll scheduled at %d, want [%d, %d)", next, tick, tick+interval)
		}
		offsets[next-tick] = struct{}{}
	}
	if len(offsets) < 2 {
		t.Fatalf("16 initial night-mote schedules used only one offset: %v", offsets)
	}
}

func TestNightMoteScanAndSpawnReuseScratch(t *testing.T) {
	r, id := nightMoteTreeTestRenderer(t, 0)
	r.updateNightMoteTrees(1)
	r.nightMotes = r.nightMotes[:0]

	allocs := testing.AllocsPerRun(100, func() {
		r.nightMotes = r.nightMotes[:0]
		r.nightMoteNextByTree[id] = 2
		r.updateNightMoteTrees(2)
	})
	if allocs != 0 {
		t.Fatalf("warm night-mote scan and spawn allocated %.2f objects, want 0", allocs)
	}
}

func TestNightMoteGlobalCap(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 3, 3)
	g := newTestGame(cfg, w)
	maxActive := cfg.Graphics.NightMotes.MaxActive
	r := &Renderer{game: g, nightMotes: make([]nightMote, maxActive)}
	tileSize := float64(cfg.GetTileSize())
	x, y := TileCenterFromTile(1, 1, tileSize)
	palette := nightMotePalette{glow: [3]int{1, 2, 3}, core: [3]int{4, 5, 6}}
	chosen := TransparentSpriteData{tileX: 1, tileY: 1, worldX: x, worldY: y, nightMotePalette: palette}

	if r.spawnNightMote(1, &chosen, w) {
		t.Fatal("mote spawned after the global cap was reached")
	}
	if got := len(r.nightMotes); got != maxActive {
		t.Fatalf("active motes = %d, want cap %d", got, maxActive)
	}
}

func TestNightMotesDoNotBecomeWorldLights(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.frameCount = 60
	r := &Renderer{game: g, nightMotes: []nightMote{{
		startX: g.camera.X, startY: g.camera.Y,
		targetX: g.camera.X, targetY: g.camera.Y,
		bornTick: 0, dieTick: 600, sizeScale: 1,
	}}}
	r.updateActiveLights()
	if len(r.activeLights) != 0 {
		t.Fatalf("moving motes added %d world lights; want a visual-only effect", len(r.activeLights))
	}
}

func TestNightMoteTimingAndRadiusDeriveFromConfig(t *testing.T) {
	cfg := loadTestConfig(t)
	r := &Renderer{game: newTestGame(cfg, newTestWorldSized(cfg, 3, 3))}
	if want := cfg.Graphics.NightMotes.EmissionRadiusTiles * float64(cfg.GetTileSize()); r.nightMoteMaxDepth() != want {
		t.Fatalf("night mote max depth = %v, want %v", r.nightMoteMaxDepth(), want)
	}
	if want := int64(math.Round(cfg.Graphics.NightMotes.EmissionIntervalSeconds * float64(cfg.GetTPS()))); r.nightMoteSpawnInterval() != want {
		t.Fatalf("night mote spawn interval = %d ticks, want %d", r.nightMoteSpawnInterval(), want)
	}
}

func TestNightMoteOccludesBehindOpaqueTiles(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 5, 3)
	g := newTestGame(cfg, w)
	tileSize := float64(cfg.GetTileSize())
	g.camera.X, g.camera.Y = TileCenterFromTile(0, 1, tileSize)
	targetX, targetY := TileCenterFromTile(4, 1, tileSize)
	r := &Renderer{game: g}
	if !r.nightMoteHasLineOfSight(targetX, targetY) {
		t.Fatal("clear path unexpectedly occluded a night mote")
	}
	w.Tiles[1][2] = world.TileTree
	if r.nightMoteHasLineOfSight(targetX, targetY) {
		t.Fatal("tree failed to occlude a night mote")
	}
}

func TestNightMoteTargetRejectsBlockingTerrain(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	world.GlobalTileManager = world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}

	w := newTestWorldSized(cfg, 3, 3)
	for y := range w.Tiles {
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileWall
		}
	}
	g := newTestGame(cfg, w)
	r := &Renderer{game: g}
	tileSize := float64(cfg.GetTileSize())
	centerX, centerY := TileCenterFromTile(1, 1, tileSize)
	palette := nightMotePalette{glow: [3]int{1, 2, 3}, core: [3]int{4, 5, 6}}
	chosen := TransparentSpriteData{tileX: 1, tileY: 1, worldX: centerX, worldY: centerY, nightMotePalette: palette}
	if r.spawnNightMote(1, &chosen, w) {
		t.Fatal("mote spawned when every adjacent tile was blocking")
	}

	w.Tiles[0][0] = world.TileEmpty
	if !r.spawnNightMote(1, &chosen, w) {
		t.Fatal("mote did not use the only open adjacent tile")
	}
	mote := r.nightMotes[len(r.nightMotes)-1]
	if int(mote.targetX/tileSize) != 0 || int(mote.targetY/tileSize) != 0 {
		t.Fatalf("target = (%.1f, %.1f), want open tile (0,0)", mote.targetX, mote.targetY)
	}
}

func TestNightMotePaletteRequiresAuthoredConfig(t *testing.T) {
	if _, ok := nightMotePaletteForConfig(nil); ok {
		t.Fatal("nil tile unexpectedly emitted night motes")
	}
	tile := &config.TileData{NightMotes: &config.TileNightMoteConfig{
		GlowColor: [3]int{1, 2, 3},
		CoreColor: [3]int{4, 5, 6},
	}}
	got, ok := nightMotePaletteForConfig(tile)
	if !ok || got.glow != tile.NightMotes.GlowColor || got.core != tile.NightMotes.CoreColor {
		t.Fatalf("configured palette = %+v, %v", got, ok)
	}
}
