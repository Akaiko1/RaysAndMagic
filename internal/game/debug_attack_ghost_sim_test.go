//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Headless attack-animation ghost repro - a DEBUG MODULE, not a regression
// test. Renders a lone weapon_master through every attack-sweep art frame via
// the REAL frame path (standee slab included) and dumps one PNG per frame, so
// a reported "piece of the previous frame stays on screen" can be confirmed
// or ruled out by fact.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_AttackGhost -v

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_AttackGhost(t *testing.T) {
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
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
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
	if err := wm.SwitchToMap("arena"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm
	w := wm.GetCurrentWorld()

	g := NewMMGame(cfg)
	r := g.gameLoop.renderer
	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())

	// The arena's own duel ground (map_configs duel block): party at [11,4]
	// looking east, the weapon master 2.5 tiles dead ahead, token axis square
	// to the camera; nothing else in the world to muddy the capture.
	ts := float64(cfg.GetTileSize())
	g.camera.X, g.camera.Y, g.camera.Angle = 11.5*ts, 4.5*ts, 0
	m := monster.NewMonster3DFromConfig(g.camera.X+2.5*ts, g.camera.Y, "weapon_master", cfg)
	m.StandeeYaw = 3.14159 / 2 // slab long axis perpendicular to the view
	m.Direction = 3.14159      // walking left, so _l art is unmirrored
	w.Monsters = []*monster.Monster3D{m}
	w.NPCs = nil
	g.groundContainers = nil

	outDir := os.Getenv("RAM_GHOST_OUT")
	if outDir == "" {
		outDir = "."
	}

	// TEMPORAL sweep, like live play: AttackAnimFrames ticks down one per draw
	// frame while the renderer runs its real yaw/mirror dynamics. Per frame,
	// count "slash-violet" pixels (the wide arc's color signature: bright,
	// blue>green, red>green - sand/sky/walls never match). Art frame 3 carries
	// almost no violet, so thousands of violet pixels while idx==3 renders =
	// the reported ghost, caught by fact.
	total := int(MonsterAttackAnimFrames)
	bw, bh := screen.Bounds().Dx(), screen.Bounds().Dy()
	buf := make([]byte, 4*bw*bh)
	for _, mode := range []struct {
		name    string
		standee bool
	}{{"standee", true}, {"billboard", false}} {
		cfg.Graphics.Standee.Enabled = mode.standee
		violetAt := make(map[int][]int) // artIdx -> counts per draw
		for aaf := total; aaf >= 1; aaf-- {
			m.AttackAnimFrames = aaf
			runOnDrawFrame(func(_ *ebiten.Image) {
				screen.Clear()
				r.RenderFirstPersonView(screen)
			})
			screen.ReadPixels(buf)
			violet := 0
			for i := 0; i+3 < len(buf); i += 4 {
				rr, gg, bb := int(buf[i]), int(buf[i+1]), int(buf[i+2])
				if bb > 170 && bb > gg+15 && rr > gg+5 {
					violet++
				}
			}
			artIdx := int(int64(total-aaf) * 4 / int64(total))
			violetAt[artIdx] = append(violetAt[artIdx], violet)

			// Dump the suspicious composites: violet arc while the recovery
			// frame (idx 3) should be on screen.
			if artIdx == 3 && violet > 100 {
				x0, x1 := bw/2-bh/3, bw/2+bh/3
				sub := screen.SubImage(image.Rect(x0, 0, x1, bh))
				path := fmt.Sprintf("%s/ghost_%s_idx3_aaf%d_violet%d.png", outDir, mode.name, aaf, violet)
				if f, err := os.Create(path); err == nil {
					_ = png.Encode(f, sub)
					f.Close()
					t.Logf("GHOST CANDIDATE -> %s", path)
				}
			}
		}
		for idx := 0; idx < 4; idx++ {
			t.Logf("%s: artIdx %d violet counts: %v", mode.name, idx, violetAt[idx])
		}
	}
	cfg.Graphics.Standee.Enabled = true

	// Hypothesis probe: TWO weapon masters overlapping at different sweep
	// phases - the rear one's wide slash (frame 2) pokes out around the front
	// one showing frame 3, reading as "a piece of the slash stays into frame 4".
	m2 := monster.NewMonster3DFromConfig(m.X+0.4*ts, m.Y+0.3*ts, "weapon_master", cfg)
	m2.StandeeYaw = m.StandeeYaw
	m2.Direction = m.Direction
	w.Monsters = []*monster.Monster3D{m, m2}
	m.AttackAnimFrames = 2  // front: art frame 3 (recovery)
	m2.AttackAnimFrames = 7 // rear: art frame 2 (wide slash)
	runOnDrawFrame(func(_ *ebiten.Image) {
		screen.Clear()
		r.RenderFirstPersonView(screen)
	})
	screen.ReadPixels(buf)
	violet := 0
	for i := 0; i+3 < len(buf); i += 4 {
		rr, gg, bb := int(buf[i]), int(buf[i+1]), int(buf[i+2])
		if bb > 170 && bb > gg+15 && rr > gg+5 {
			violet++
		}
	}
	x0, x1 := bw/2-bh/3, bw/2+bh/3
	sub := screen.SubImage(image.Rect(x0, 0, x1, bh))
	path := fmt.Sprintf("%s/two_wm_overlap_violet%d.png", outDir, violet)
	if f, err := os.Create(path); err == nil {
		_ = png.Encode(f, sub)
		f.Close()
	}
	t.Logf("two overlapping WMs (front idx3, rear idx2): violet=%d -> %s", violet, path)
}
