//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Melee-FX gallery - renders every bespoke slash_fx style frame-by-frame on a
// dark backdrop (the exact drawSlashEffects camera anchor) and dumps PNG frame
// strips for GIF assembly, so swing flourishes can be eyeballed and iterated
// without booting the game.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_MeleeFxGallery -v

import (
	"fmt"
	"image"
	"image/color"
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

func TestDebugSim_MeleeFxGallery(t *testing.T) {
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
	r := g.gameLoop.renderer

	// RAM_FX_STYLES=comma,list narrows the set (fast iteration on one style).
	only := map[string]bool{}
	if v := os.Getenv("RAM_FX_STYLES"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s != "" {
				only[s] = true
			}
		}
	}

	// One weapon per bespoke style, from the live catalog - crit variant on
	// request (RAM_FX_CRIT=1).
	styleWeapon := map[string]*config.WeaponDefinitionConfig{}
	for _, def := range config.GlobalWeapons.Weapons {
		if def != nil && def.Graphics != nil && def.Graphics.SlashFx != "" {
			styleWeapon[def.Graphics.SlashFx] = def
		}
	}
	crit := os.Getenv("RAM_FX_CRIT") == "1"

	outRoot := filepath.Join(os.Getenv("HOME"), "Downloads", "melee_fx_gallery")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	sw, sh := cfg.GetScreenWidth(), cfg.GetScreenHeight()
	// Crop window around the swing anchor keeps the GIFs tight.
	cropW, cropH := sw*2/3, sh*3/4
	cropX, cropY := (sw-cropW)/2, sh-cropH
	screen := ebiten.NewImage(sw, sh)

	for style, def := range styleWeapon {
		if len(only) > 0 && !only[style] {
			continue
		}
		maxFrames := def.Melee.AnimationFrames
		if maxFrames < meleeFxStyledLingerFrames {
			maxFrames = meleeFxStyledLingerFrames
		}
		s := SlashEffect{
			ID: "gallery_" + style, Width: def.Graphics.SlashWidth, Length: def.Graphics.SlashLength,
			Color: def.Graphics.SlashColor, MaxFrames: maxFrames, Active: true,
			Kind: meleeFxKind(def), Style: style, Crit: crit,
		}
		dir := filepath.Join(outRoot, style)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f := 0; f < maxFrames; f++ {
			s.AnimationFrame = f
			g.frameCount++ // styles animate off the global frame clock too
			runOnDrawFrame(func(_ *ebiten.Image) {
				screen.Fill(color.RGBA{24, 22, 28, 255}) // dark arena-night backdrop
				cx := float64(sw) / 2
				cy := float64(sh) * meleeAnchorYFrac
				r.drawMeleeParticles(screen, s, cx, cy, float64(sh))
			})
			sub := screen.SubImage(image.Rect(cropX, cropY, cropX+cropW, cropY+cropH)).(*ebiten.Image)
			fp, err := os.Create(filepath.Join(dir, fmt.Sprintf("f%02d.png", f)))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(fp, sub); err != nil {
				t.Fatal(err)
			}
			fp.Close()
		}
		t.Logf("style %-18s %d frames -> %s", style, maxFrames, dir)
	}
}
