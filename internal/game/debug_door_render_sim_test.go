//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Door-render diagnostic - renders the closed arena portcullis from a few
// distances and saves the frames to ~/Downloads for eyeballing the slab
// geometry against the flanking walls (width span, height, art aspect).
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_DoorRender -v

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

func TestDebugSim_DoorRender(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
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
	if err := wm.SwitchToMap("arena"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	defer g.Shutdown()
	g.appScreen = AppScreenInGame
	g.doorsClosed = true // render the portcullis without spawning a champion

	// First door NPC on the map.
	var door *character.NPC
	for _, n := range g.world.NPCs {
		if n != nil && g.npcIsDoor(n) {
			door = n
			break
		}
	}
	if door == nil {
		t.Fatal("no door NPC on the arena map")
	}
	_, _, yaw, ok := g.doorPose(door.X, door.Y)
	if !ok {
		t.Fatal("doorPose found no flanking wall pair")
	}

	outDir := filepath.Join(os.Getenv("HOME"), "Downloads", "door_render_check")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tile := float64(cfg.GetTileSize())
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())

	// The passage axis is PERPENDICULAR to the slab's long axis (yaw): stand
	// in the corridor, look straight at the door.
	axisX, axisY := math.Cos(yaw+math.Pi/2), math.Sin(yaw+math.Pi/2)
	for _, distTiles := range []float64{1.5, 3, 5} {
		side := 1.0
		camX := door.X + axisX*tile*distTiles*side
		camY := door.Y + axisY*tile*distTiles*side
		if g.collisionSystem != nil && !g.collisionSystem.CanMoveTo("player", camX, camY) {
			side = -1.0
			camX = door.X + axisX*tile*distTiles*side
			camY = door.Y + axisY*tile*distTiles*side
		}
		g.camera.X, g.camera.Y = camX, camY
		g.camera.Angle = math.Atan2(door.Y-camY, door.X-camX)
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			r.RenderFirstPersonView(screen)
		})
		path := filepath.Join(outDir, fmt.Sprintf("door_%.1ftiles.png", distTiles))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// ebiten.Image implements image.Image (At readback), png handles it.
		if err := png.Encode(f, screen); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Logf("wrote %s (cam at %.1f tiles)", path, distTiles)
	}
}
