//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Town render check - Silverbough and Dunehold from the viewpoints a player
// actually arrives at: the gate you spawn beside, the avenue up to the central
// landmark, the plaza, the shop fronts. Saved to ~/Downloads/town_render_check.
//
// New town art (four wall variants, four floor variants, four landmarks and six
// props per town) cannot be judged from a contact sheet: seams, scale against
// the wall height, and whether a landmark reads as a building only show up in
// the first-person view.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_TownRender -v

import (
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

func TestDebugSim_TownRender(t *testing.T) {
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
	if err := wm.SwitchToMap("elf_city"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	defer g.Shutdown()
	g.appScreen = AppScreenInGame

	outDir := filepath.Join(os.Getenv("HOME"), "Downloads", "town_render_check")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := float64(cfg.GetTileSize())
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

	type shot struct {
		name           string
		cx, cy, tx, ty float64 // camera tile, look-at tile
	}
	tours := []struct {
		mapKey string
		shots  []shot
	}{
		{"elf_city", []shot{
			{"elf_1_exit_on_the_wall", 15.5, 18.5, 15.5, 21.5},
			{"elf_2_avenue_north", 15.5, 19.5, 15.5, 11.5},
			{"elf_3_plaza_pavilion", 15.5, 15.5, 15.5, 11.5},
			{"elf_4_archive_court", 15.5, 7.5, 15.5, 2.5},
			{"elf_5_apothecary_row", 10.5, 10.5, 2.5, 10.5},
			{"elf_6_ranger_hall", 20.5, 9.5, 25.5, 8.5},
		}},
		{"nomad_city", []shot{
			{"nomad_1_exit_on_the_wall", 15.5, 18.5, 15.5, 21.5},
			{"nomad_2_avenue_north", 15.5, 19.5, 15.5, 13.5},
			{"nomad_3_oasis_shrine", 15.5, 16.5, 15.5, 13.5},
			{"nomad_4_bazaar_street", 20.5, 7.5, 5.5, 8.5},
			{"nomad_5_caravan_yard", 10.5, 7.5, 10.5, 2.5},
			{"nomad_6_windwright", 20.5, 9.5, 25.5, 8.5},
		}},
		// The two gates as they stand on the overworld - a 2.3-tile landmark
		// dropped into pinewood or open sand can clip its neighbours.
		{"highlands", []shot{
			{"gate_1_silverbough_approach", 2.5, 14.5, 2.5, 10.5},
			{"gate_2_silverbough_angle", 6.5, 13.5, 2.5, 10.5},
		}},
		{"desert", []shot{
			{"gate_3_dunehold_approach", 43.5, 29.5, 43.5, 24.5},
			{"gate_4_dunehold_angle", 47.5, 28.5, 43.5, 24.5},
		}},
		// Seabright at the same focal lengths: the reference for how dense a
		// town of this size is supposed to read from the street.
		{"city", []shot{
			{"ref_1_exit_on_the_wall", 15.5, 18.5, 15.5, 21.5},
			{"ref_2_avenue_north", 14.5, 19.5, 14.5, 11.5},
			{"ref_3_fountain", 14.5, 15.5, 14.5, 11.5},
			{"ref_4_shop_row", 14.5, 7.5, 14.5, 2.5},
			{"ref_5_west_blocks", 10.5, 10.5, 2.5, 10.5},
			{"ref_6_east_blocks", 20.5, 9.5, 27.5, 8.5},
		}},
	}

	ih := &InputHandler{game: g}
	for _, tour := range tours {
		ih.switchToMap(tour.mapKey)
		for _, s := range tour.shots {
			g.camera.X, g.camera.Y = s.cx*ts, s.cy*ts
			g.camera.Angle = math.Atan2((s.ty-s.cy)*ts, (s.tx-s.cx)*ts)
			shoot(s.name)
		}
	}
	t.Logf("shots -> %s", outDir)
}
