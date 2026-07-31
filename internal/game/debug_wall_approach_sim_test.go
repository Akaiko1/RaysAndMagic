//go:build debug

package game

// Headless temporal-artifact diagnostic - a DEBUG MODULE, not a regression
// test. Renders a smooth camera run (default: the city plaza approach) and
// dumps every frame as PNG. Static screenshots cannot show temporal render
// artifacts - a shadow crawling along a wall, texture jumping sideways on a
// mip-level switch, standee mip pop - so verify those on a frame sequence
// from this sim (stack crops into strips, or assemble a GIF).
//
// Run:
//   RAM_DEBUG_SIM=1 RAM_APPROACH_DIR=/path/frames \
//   go test -tags debug ./internal/game/ -run TestDebugSim_WallApproach -count=1

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_WallApproach(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	outDir := os.Getenv("RAM_APPROACH_DIR")
	if outDir == "" {
		t.Skip("set RAM_APPROACH_DIR to the frame output directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
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
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
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
	if err := wm.SwitchToMap("city"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	g.appScreen = AppScreenInGame
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	runOnDrawFrame(func(_ *ebiten.Image) {
		g.gameLoop.renderer.prewarmPendingMapRenderResources()
	})

	// Default: march from tile (16,21) straight north to (16,2) - the full
	// 19-tile plaza run, looking dead ahead. RAM_APPROACH_PATH overrides with
	// "x1,y1,x2,y2,angleDeg" in tile coords (e.g. a grazing wall-follow).
	tileSize := float64(cfg.GetTileSize())
	x1, y1, x2, y2, angleDeg := 16, 21, 16, 2, -90.0
	if path := os.Getenv("RAM_APPROACH_PATH"); path != "" {
		if _, err := fmt.Sscanf(path, "%d,%d,%d,%d,%f", &x1, &y1, &x2, &y2, &angleDeg); err != nil {
			t.Fatalf("RAM_APPROACH_PATH must be x1,y1,x2,y2,angleDeg: %v", err)
		}
	}
	startX, startY := TileCenterFromTile(x1, y1, tileSize)
	endX, endY := TileCenterFromTile(x2, y2, tileSize)
	angle := angleDeg * math.Pi / 180

	const stepPx = 4.0
	span := math.Hypot(endX-startX, endY-startY)
	steps := int(span / stepPx)
	frame := 0
	for i := 0; i <= steps; i++ {
		along := float64(i) / float64(steps)
		g.camera.X = startX + (endX-startX)*along
		g.camera.Y = startY + (endY-startY)*along
		g.camera.Angle = angle
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			g.gameLoop.renderer.RenderFirstPersonView(screen)
		})
		path := filepath.Join(outDir, fmt.Sprintf("frame_%04d.png", frame))
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create frame: %v", err)
		}
		if err := png.Encode(f, screen); err != nil {
			f.Close()
			t.Fatalf("encode frame: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close frame: %v", err)
		}
		frame++
	}
	t.Logf("wrote %d frames to %s", frame, outDir)
}
