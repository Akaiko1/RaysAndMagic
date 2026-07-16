//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Clock-tower render check - the 2:1 grid-span facade on the highlands and an
// interior shot of each floor, saved to ~/Downloads/clock_tower_render_check.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_ClockTowerRender -v

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

func TestDebugSim_ClockTowerRender(t *testing.T) {
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
	if err := ValidateNPCCommerce(character.NPCConfigInstance.NPCs); err != nil {
		t.Fatalf("commerce: %v", err)
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
	if err := wm.SwitchToMap("highlands"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	defer g.Shutdown()
	g.appScreen = AppScreenInGame

	outDir := filepath.Join(os.Getenv("HOME"), "Downloads", "clock_tower_render_check")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	shoot := func(name string) {
		runOnDrawFrame(func(_ *ebiten.Image) {
			screen.Clear()
			r.RenderFirstPersonView(screen)
		})
		f, err := os.Create(filepath.Join(outDir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Logf("wrote %s", name)
	}

	// The facade from the highlands meadow, straight on and angled.
	var tower *character.NPC
	for _, n := range g.world.NPCs {
		if n != nil && n.GridSpanTiles >= 2 {
			tower = n
			break
		}
	}
	if tower == nil {
		t.Fatal("no grid-span tower NPC on highlands")
	}
	bx, by, _, _ := g.buildingPose(tower)
	ts := float64(cfg.GetTileSize())
	for _, shot := range []struct {
		name string
		dx   float64
		dy   float64
	}{{"facade_front_3t", 0, 3.2}, {"facade_front_6t", 0, 6}, {"facade_angle", -3.5, 3.5}} {
		g.camera.X, g.camera.Y = bx+shot.dx*ts, by+shot.dy*ts
		g.camera.Angle = math.Atan2(by-g.camera.Y, bx-g.camera.X)
		shoot(shot.name)
	}

	// Interiors: spawn-point view of each floor.
	ih := &InputHandler{game: g}
	for i, mapKey := range []string{"clock_tower_1", "clock_tower_2", "clock_tower_3"} {
		ih.switchToMap(mapKey)
		sx, sy := g.world.GetStartingPosition()
		g.camera.X, g.camera.Y = sx, sy
		for a := 0; a < 3; a++ {
			g.camera.Angle = -math.Pi/2 + float64(a-1)*0.7 // face north-ish fan
			shoot(fmt.Sprintf("floor%d_view%d", i+1, a))
		}
	}
}
