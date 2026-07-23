//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Headless render-walk diagnostic - a DEBUG MODULE, not a regression test.
//
// Unlike debug_standee_cost_sim_test.go (a pure-math cost proxy for TREES
// only), this drives the REAL renderer frame path (RenderFirstPersonView:
// floor shader, wall raycast, the full unified sprite pass with env sprites,
// FIREFLY swarms, monsters, NPCs, ground containers and the depth sort) on
// the REAL forest map, and reads the SAME timers the in-game perf overlay
// shows (statFloorMs / statWallsMs / statSpritesMs). It needs a live ebiten
// context - see TestMain in main_test.go (RAM_DEBUG_SIM=1 wraps the run in
// ebiten.RunGame).
//
// The walk: every tile along both map-grid sections of the forest river,
// smoothly (half-tile steps), spinning through 8 angles at each stop; plus a
// far-from-river control sweep. Each is measured TWICE: on the freshly loaded
// map, and in the "aftermath" state the FPS reports actually came from - every
// monster dead, a real loot bag dropped at each corpse (the real
// addLootBagDrop path). At each stop's worst angle the sprite-pass cost is attributed by
// ABLATION: re-render the same pose with one category hidden (trees /
// fireflies / other env sprites / monsters / NPCs / loot bags) and subtract -
// real timings, no formulas.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_RenderWalk -v
// One-pose profile: RAM_DEBUG_SIM=1 RAM_WALK_POSE=13,36,45
// RAM_WALK_MAP=deep_jungle selects another map for a one-pose profile.
// RAM_WALK_OFFSET_PX=0,30 moves that pose within the selected tile.
// RAM_WALK_SCREENSHOT=/tmp/river.png saves the final rendered frame.
// RAM_WALK_TREES_ONLY=1 removes every other unified-sprite category.
// RAM_WALK_REPS=1 exposes cold per-pose costs instead of taking a warm minimum.
// RAM_WALK_POSE_REPS=300 go test -tags debug ./internal/game \
// -run TestDebugSim_RenderWalk -cpuprofile /tmp/river.pprof

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
	"sort"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// walkFrame is one measured (position, angle) render.
type walkFrame struct {
	tx, ty    int
	x, y      float64
	angleDeg  float64
	riverDist int

	spritesMs float64 // min over reps - the overlay's "sprites:" number
	floorMs   float64
	wallsMs   float64
	trees     int // statTreesDrawn
	standeeDC int // statStandeeCalls
}

// walkHarness owns the real game + offscreen target and measures poses.
type walkHarness struct {
	g      *MMGame
	r      *Renderer
	w      *world.World3D
	screen *ebiten.Image
	reps   int
}

// measure renders the pose reps times - each render inside its OWN live Draw
// frame (see runOnDrawFrame in main_test.go; frame boundaries let ebiten
// reclaim per-frame internals) - and keeps the MINIMUM sprite-pass time (the
// first rep doubles as the lazy-cache warmer; min discards GC/OS noise).
func (h *walkHarness) measure(x, y, angleRad float64) walkFrame {
	h.g.camera.X, h.g.camera.Y, h.g.camera.Angle = x, y, angleRad
	best := walkFrame{spritesMs: math.MaxFloat64}
	for i := 0; i < h.reps; i++ {
		runOnDrawFrame(func(_ *ebiten.Image) {
			h.screen.Clear()
			h.r.RenderFirstPersonView(h.screen)
		})
		if h.r.statSpritesMs < best.spritesMs {
			best.spritesMs = h.r.statSpritesMs
			best.floorMs = h.r.statFloorMs
			best.wallsMs = h.r.statWallsMs
			best.trees = h.r.statTreesDrawn
			best.standeeDC = h.r.statStandeeCalls
		}
	}
	best.x, best.y, best.angleDeg = x, y, angleRad*180/math.Pi
	return best
}

// ablate re-measures the pose with one sprite category hidden and returns the
// cost DELTA vs base (clamped at 0 - measurement noise can go slightly negative).
func (h *walkHarness) ablate(x, y, angleRad, baseMs float64, hide func() (restore func())) float64 {
	restore := hide()
	got := h.measure(x, y, angleRad)
	restore()
	d := baseMs - got.spritesMs
	if d < 0 {
		d = 0
	}
	return d
}

// categoryAblations builds the hide/restore closures for every sprite class
// the unified pass draws. Each swaps REAL renderer/world state - the render
// path itself stays untouched.
func (h *walkHarness) categoryAblations() []struct {
	name string
	hide func() func()
} {
	r, w, g := h.r, h.w, h.g
	return []struct {
		name string
		hide func() func()
	}{
		{"trees", func() func() {
			saved := r.treeTilesCache
			r.treeTilesCache = nil
			return func() { r.treeTilesCache = saved }
		}},
		{"fireflies", func() func() {
			saved := r.transparentSpritesCache
			kept := make([]TransparentSpriteData, 0, len(saved))
			for _, s := range saved {
				if !isFireflySwarmTile(s.tileType) {
					kept = append(kept, s)
				}
			}
			r.transparentSpritesCache = kept
			return func() { r.transparentSpritesCache = saved }
		}},
		{"env-other", func() func() { // ferns/mushrooms/etc, fireflies kept
			saved := r.transparentSpritesCache
			kept := make([]TransparentSpriteData, 0, len(saved))
			for _, s := range saved {
				if isFireflySwarmTile(s.tileType) {
					kept = append(kept, s)
				}
			}
			r.transparentSpritesCache = kept
			return func() { r.transparentSpritesCache = saved }
		}},
		{"monsters", func() func() {
			saved := w.Monsters
			w.Monsters = nil
			return func() { w.Monsters = saved }
		}},
		{"npcs", func() func() {
			saved := w.NPCs
			w.NPCs = nil
			return func() { w.NPCs = saved }
		}},
		{"lootbags", func() func() {
			saved := g.groundContainers
			g.groundContainers = nil
			return func() { g.groundContainers = saved }
		}},
	}
}

func TestDebugSim_RenderWalk(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	fast := os.Getenv("RAM_WALK_FAST") != ""
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	// NPC config AFTER spells (loader validation order) - with it loaded the
	// river NPCs (traders, gates, shipwreck) exist and render like in the game.
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	mapKey := os.Getenv("RAM_WALK_MAP")
	if mapKey == "" {
		mapKey = "forest"
	}
	if mapKey != "forest" && os.Getenv("RAM_WALK_POSE") == "" {
		t.Fatal("RAM_WALK_MAP is only supported with RAM_WALK_POSE")
	}
	if err := wm.SwitchToMap(mapKey); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm
	w := wm.GetCurrentWorld()

	// RAM_WALK_RES=WxH overrides the render resolution - the per-column costs
	// scale with screen width, so 3840x2160 reproduces a fullscreen retina
	// session instead of the config's windowed size.
	if res := os.Getenv("RAM_WALK_RES"); res != "" {
		var rw, rhh int
		if _, err := fmt.Sscanf(res, "%dx%d", &rw, &rhh); err == nil && rw > 0 && rhh > 0 {
			cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = rw, rhh
		}
	}

	var poseSet bool
	var poseTileX, poseTileY int
	var poseAngleDeg, poseOffsetX, poseOffsetY float64
	if pose := os.Getenv("RAM_WALK_POSE"); pose != "" {
		poseSet = true
		if _, err := fmt.Sscanf(pose, "%d,%d,%f", &poseTileX, &poseTileY, &poseAngleDeg); err != nil {
			t.Fatalf("RAM_WALK_POSE must be tileX,tileY,angleDeg: %v", err)
		}
		if offset := os.Getenv("RAM_WALK_OFFSET_PX"); offset != "" {
			if _, err := fmt.Sscanf(offset, "%f,%f", &poseOffsetX, &poseOffsetY); err != nil {
				t.Fatalf("RAM_WALK_OFFSET_PX must be x,y: %v", err)
			}
		}
	}

	g := NewMMGame(cfg)
	if poseSet {
		g.camera.X, g.camera.Y = TileCenterFromTile(poseTileX, poseTileY, float64(cfg.GetTileSize()))
		g.camera.X += poseOffsetX
		g.camera.Y += poseOffsetY
		g.camera.Angle = poseAngleDeg * math.Pi / 180
	}
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	if os.Getenv("RAM_SKIP_TREE_PREWARM") == "" {
		g.gameLoop.renderer.prewarmPendingTreeStandeeResources()
		if g.gameLoop.renderer.treeStandeeResourcePrewarmPending {
			t.Fatal("tree standee resource prewarm remained pending")
		}
		seenTreeSprites := make(map[string]struct{})
		for i := range g.gameLoop.renderer.treeTilesCache {
			td := &g.gameLoop.renderer.treeTilesCache[i]
			spriteName := td.spriteName
			if spriteName == "" {
				spriteName = treeStandeeSpriteName(td.tileType)
			}
			if _, seen := seenTreeSprites[spriteName]; seen {
				continue
			}
			seenTreeSprites[spriteName] = struct{}{}
			sprite := g.sprites.GetSprite(spriteName)
			key := standeeCoreKey{name: "tree:" + spriteName, bounds: sprite.Bounds(), img: sprite}
			if g.gameLoop.renderer.standeeCoreCache[key] == nil {
				t.Fatalf("tree %q has no prewarmed standee core", spriteName)
			}
			for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
				if chain := g.gameLoop.renderer.standeeMipCache[standeeMipKey{frame: key, layer: layer}]; chain == nil || len(chain.levels) == 0 {
					t.Fatalf("tree %q layer %d has no prewarmed mip chain", spriteName, layer)
				}
			}
		}
	}
	h := &walkHarness{
		g: g, r: g.gameLoop.renderer, w: w,
		screen: screen,
		// Fast mode limits the route, not samples per pose: the first sample
		// warms Ebitengine's lazy GPU resources and is intentionally discarded.
		reps: 3,
	}
	if reps := os.Getenv("RAM_WALK_REPS"); reps != "" {
		if _, err := fmt.Sscanf(reps, "%d", &h.reps); err != nil || h.reps <= 0 {
			t.Fatalf("RAM_WALK_REPS must be a positive integer")
		}
	}
	if os.Getenv("RAM_WALK_TREES_ONLY") != "" {
		h.r.transparentSpritesCache = nil
		h.r.wallTorches = nil
		h.w.Monsters = nil
		h.w.NPCs = nil
		h.g.groundContainers = nil
		trees := h.r.treeTilesCache
		h.r.treeTilesCache = nil
		h.measure(h.g.camera.X, h.g.camera.Y, h.g.camera.Angle)
		h.r.treeTilesCache = trees
	} else {
		// The live game screen has already been allocated and submitted before a
		// player can reach the river. Warm only this diagnostic's fresh offscreen
		// destination so its first atlas allocation is not misattributed to sprites.
		runOnDrawFrame(func(_ *ebiten.Image) {
			h.screen.Clear()
		})
	}
	if poseSet {
		if reps := os.Getenv("RAM_WALK_POSE_REPS"); reps != "" {
			if _, err := fmt.Sscanf(reps, "%d", &h.reps); err != nil || h.reps <= 0 {
				t.Fatalf("RAM_WALK_POSE_REPS must be a positive integer")
			}
		}
		got := h.measure(g.camera.X, g.camera.Y, g.camera.Angle)
		if path := os.Getenv("RAM_WALK_SCREENSHOT"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				t.Fatalf("create screenshot: %v", err)
			}
			if err := png.Encode(f, h.screen); err != nil {
				f.Close()
				t.Fatalf("encode screenshot: %v", err)
			}
			if err := f.Close(); err != nil {
				t.Fatalf("close screenshot: %v", err)
			}
		}
		t.Logf("single pose tile=(%d,%d) world=(%.1f,%.1f) angle=%.1f reps=%d: sprites=%.2fms floor=%.2fms walls=%.2fms trees=%d standeeDC=%d",
			poseTileX, poseTileY, g.camera.X, g.camera.Y, poseAngleDeg, h.reps, got.spritesMs, got.floorMs, got.wallsMs, got.trees, got.standeeDC)
		return
	}
	tileSize := float64(cfg.GetTileSize())
	t.Logf("render target %dx%d, reps=%d (min taken), %d monsters, %d NPCs, %d env sprites, %d tree tiles",
		cfg.GetScreenWidth(), cfg.GetScreenHeight(), h.reps,
		len(w.Monsters), len(w.NPCs), len(h.r.transparentSpritesCache), len(h.r.treeTilesCache))

	// River tiles from the actual grid, ordered stream-by-stream then west to
	// east - the walk follows the water like a player would.
	streamType, ok := world.GlobalTileManager.GetTileTypeFromKey("forest_stream")
	if !ok {
		t.Fatal("forest_stream tile type not registered")
	}
	var riverTiles [][2]int
	for ty := 0; ty < w.Height; ty++ {
		for tx := 0; tx < w.Width; tx++ {
			if w.Tiles[ty][tx] == streamType {
				riverTiles = append(riverTiles, [2]int{tx, ty})
			}
		}
	}
	if len(riverTiles) == 0 {
		t.Fatal("no forest_stream tiles on the forest map")
	}
	sort.Slice(riverTiles, func(i, j int) bool {
		if riverTiles[i][1] != riverTiles[j][1] {
			return riverTiles[i][1] < riverTiles[j][1]
		}
		return riverTiles[i][0] < riverTiles[j][0]
	})
	allRiverTiles := riverTiles
	if fast && len(riverTiles) > 2 {
		riverTiles = riverTiles[:2]
	}
	chebyshevToRiver := func(tx, ty int) int {
		best := 1 << 30
		for _, rt := range allRiverTiles {
			d := absInt(rt[0] - tx)
			if dy := absInt(rt[1] - ty); dy > d {
				d = dy
			}
			if d < best {
				best = d
			}
		}
		return best
	}

	const angles = 8
	measured := 0
	// sweep measures every pose of a route and returns per-pose worst frames.
	sweep := func(route [][2]int, subSteps int) []walkFrame {
		var worst []walkFrame
		for ti, tile := range route {
			if ti%20 == 0 {
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				fmt.Printf("[walk] tile %d/%d, %d poses measured, heap=%dMB sys=%dMB\n",
					ti, len(route), measured, ms.HeapAlloc/(1<<20), ms.Sys/(1<<20))
			}
			cx, cy := TileCenterFromTile(tile[0], tile[1], tileSize)
			for s := 0; s < subSteps; s++ {
				x := cx + float64(s)*tileSize/float64(subSteps)
				y := cy
				var w0 walkFrame
				w0.spritesMs = -1
				for a := 0; a < angles; a++ {
					f := h.measure(x, y, float64(a)*2*math.Pi/float64(angles))
					measured++
					f.tx, f.ty = tile[0], tile[1]
					f.riverDist = chebyshevToRiver(tile[0], tile[1])
					if f.spritesMs > w0.spritesMs {
						w0 = f
					}
				}
				worst = append(worst, w0)
			}
		}
		return worst
	}

	report := func(label string, frames []walkFrame) {
		if len(frames) == 0 {
			return
		}
		var sum, max float64
		var maxF walkFrame
		for _, f := range frames {
			sum += f.spritesMs
			if f.spritesMs > max {
				max, maxF = f.spritesMs, f
			}
		}
		t.Logf("[%s] poses=%d  sprites ms: mean=%.2f max=%.2f  (worst at tile=(%d,%d) angle=%.0f: floor=%.2f walls=%.2f trees=%d standeeDC=%d)",
			label, len(frames), sum/float64(len(frames)), max,
			maxF.tx, maxF.ty, maxF.angleDeg, maxF.floorMs, maxF.wallsMs, maxF.trees, maxF.standeeDC)
	}

	// attribution ablates the top-N worst frames and prints the breakdown.
	attribution := func(label string, frames []walkFrame, topN int) {
		sort.Slice(frames, func(i, j int) bool { return frames[i].spritesMs > frames[j].spritesMs })
		if topN > len(frames) {
			topN = len(frames)
		}
		t.Logf("[%s] --- ablation breakdown of the %d worst poses (ms shaved off by hiding each category) ---", label, topN)
		cats := h.categoryAblations()
		for i := 0; i < topN; i++ {
			f := frames[i]
			rad := f.angleDeg * math.Pi / 180
			base := h.measure(f.x, f.y, rad)
			line := fmt.Sprintf("  #%d tile=(%d,%d) angle=%3.0f total=%.2fms:", i+1, f.tx, f.ty, f.angleDeg, base.spritesMs)
			for _, c := range cats {
				d := h.ablate(f.x, f.y, rad, base.spritesMs, c.hide)
				line += fmt.Sprintf("  %s=%.2f", c.name, d)
			}
			t.Log(line)
		}
	}

	// --- Phase A: map as loaded (all monsters alive, no loot bags) ----------
	riverFrames := sweep(riverTiles, 2)
	report("A river, mobs alive", riverFrames)

	// Far-from-river control: same measurement, tiles >= 12 tiles from water.
	var farRoute [][2]int
	for ty := 0; ty < w.Height; ty += 2 {
		for tx := 0; tx < w.Width; tx += 2 {
			if world.GlobalTileManager.IsWalkable(w.Tiles[ty][tx]) && chebyshevToRiver(tx, ty) >= 12 {
				farRoute = append(farRoute, [2]int{tx, ty})
			}
		}
	}
	if fast && len(farRoute) > 2 {
		farRoute = farRoute[:2]
	}
	farFrames := sweep(farRoute, 1)
	report("A far from river   ", farFrames)
	if !fast {
		attribution("A river", riverFrames, 8)
	}

	// --- Phase B: the aftermath state from the FPS reports - every monster
	// dead, a REAL loot bag at each corpse (the exact path monster kills use).
	deadAt := make([][2]float64, 0, len(w.Monsters))
	for _, m := range w.Monsters {
		deadAt = append(deadAt, [2]float64{m.X, m.Y})
	}
	w.Monsters = nil
	for _, p := range deadAt {
		g.addLootBagDrop(p[0], p[1], nil, 5)
	}
	t.Logf("phase B: %d monsters removed, %d loot bags dropped", len(deadAt), len(g.groundContainers))

	riverFramesB := sweep(riverTiles, 2)
	report("B river, aftermath ", riverFramesB)
	farFramesB := sweep(farRoute, 1)
	report("B far, aftermath   ", farFramesB)
	if !fast {
		attribution("B river", riverFramesB, 8)
	}
}
