//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Zone gallery - one spawn-point screenshot per map at an arbitrary logical
// resolution (RAM_WALK_RES=WxH), for eyeballing the projection: with the
// square-projection FOV every zone's tiles must read square at ANY aspect.
//
// Run with:  RAM_DEBUG_SIM=1 RAM_WALK_RES=1920x1080 go test -tags debug ./internal/game/ -run TestDebugSim_ZoneGallery -v

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_ZoneGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if res := os.Getenv("RAM_WALK_RES"); res != "" {
		var rw, rh int
		if _, err := fmt.Sscanf(res, "%dx%d", &rw, &rh); err == nil && rw > 0 && rh > 0 {
			cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = rw, rh
		}
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
	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	if _, err := config.LoadChampionConfig("assets/champions.yaml"); err != nil {
		t.Fatalf("champions: %v", err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("maps: %v", err)
	}
	if err := wm.SwitchToMap("forest"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	defer g.Shutdown()
	g.appScreen = AppScreenInGame

	outDir := filepath.Join(os.Getenv("HOME"), "Downloads", "zone_gallery")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	suffix := fmt.Sprintf("_%dx%d", cfg.GetScreenWidth(), cfg.GetScreenHeight())

	maps := []string{"forest", "city", "arena", "pyramid_1", "japanese_castle", "highlands"}
	if v := os.Getenv("RAM_ZONE_MAPS"); v != "" {
		maps = strings.Split(v, ",")
	}
	// RAM_FOV=<radians> pins the camera FOV (e.g. the pre-square-projection
	// constant 1.176005207) to compare projections at the same resolution.
	fovOverride := 0.0
	if v := os.Getenv("RAM_FOV"); v != "" {
		fmt.Sscanf(v, "%f", &fovOverride)
		suffix += "_fov" + v
	}

	ih := &InputHandler{game: g}
	for _, mapKey := range maps {
		ih.switchToMap(mapKey)
		if fovOverride > 0 {
			g.camera.FOV = fovOverride
		}
		// Teleport-only zones (desert) have no '+' start; shoot from map center.
		ts := float64(g.config.GetTileSize())
		sx, sy := float64(g.world.Width)/2*ts, float64(g.world.Height)/2*ts
		if g.world.StartX != -1 && g.world.StartY != -1 {
			sx, sy = g.world.GetStartingPosition()
		}
		g.camera.X, g.camera.Y = sx, sy
		g.camera.Angle = -1.2 // a consistent oblique look
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			r.RenderFirstPersonView(screen)
		})
		f, err := os.Create(filepath.Join(outDir, mapKey+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Logf("wrote %s%s", mapKey, suffix)
	}
}
