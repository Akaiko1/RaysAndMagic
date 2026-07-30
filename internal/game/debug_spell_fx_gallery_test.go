//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Spell-projectile gallery - renders the flying body of every projectile spell
// on a dark backdrop and dumps a contact sheet per spell, so a bolt can be
// eyeballed without booting the game and casting it.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_SpellFxGallery -v
// RAM_FX_SPELLS=fireball,ice_bolt narrows the set.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// bootFxGalleryGame boots the real game on the arena map - the gallery draws
// through the live renderer, not a stub.
func bootFxGalleryGame(t *testing.T) (*MMGame, *Renderer) {
	t.Helper()
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	// Ordered: npcs.yaml validates its spell traders against spells.yaml.
	for _, step := range []struct {
		path string
		load func(string) error
	}{
		{"assets/spells.yaml", func(p string) error { _, e := config.LoadSpellConfig(p); return e }},
		{"assets/weapons.yaml", func(p string) error { _, e := config.LoadWeaponConfig(p); return e }},
		{"assets/items.yaml", func(p string) error { _, e := config.LoadItemConfig(p); return e }},
		{"assets/traps.yaml", func(p string) error { _, e := config.LoadTrapConfig(p); return e }},
		{"assets/npcs.yaml", character.LoadNPCConfig},
	} {
		if err := step.load(step.path); err != nil {
			t.Fatalf("%s: %v", step.path, err)
		}
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM })
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
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
	g.appScreen = AppScreenInGame
	return g, g.gameLoop.renderer
}

func TestDebugSim_SpellFxGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()

	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("RAM_FX_SPELLS"), ",") {
		if s != "" {
			only[s] = true
		}
	}

	var keys []string
	for key, def := range config.GlobalSpells.Spells {
		if def == nil || !def.IsProjectile {
			continue
		}
		if len(only) > 0 && !only[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	outRoot := filepath.Join(os.Getenv("HOME"), "Downloads", "spell_fx_gallery")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// Real in-game scales in both projections: side-on first, then head-on for
	// each size. Judging only the side profile misses fake horizontal trails on
	// shots travelling directly away from the camera.
	sizes := []float64{spellParticleMaxSize, 24, spellFxMinClusterSize}
	const cell, frames, views = 180, 4, 2
	sheetW, sheetH := cell*frames, cell*len(sizes)*views
	screen := ebiten.NewImage(cell, cell)
	for _, key := range keys {
		def := config.GlobalSpells.Spells[key]
		base := [3]int{200, 200, 255}
		if def.Graphics != nil && len(def.Graphics.Color) == 3 {
			base = [3]int{def.Graphics.Color[0], def.Graphics.Color[1], def.Graphics.Color[2]}
		}
		profile := r.spellFxProfile(key, base)
		sheet := image.NewNRGBA(image.Rect(0, 0, sheetW, sheetH))
		for si, size := range sizes {
			for view := 0; view < views; view++ {
				for f := 0; f < frames; f++ {
					g.frameCount += 7 // spread the animation clock across the strip
					runOnDrawFrame(func(_ *ebiten.Image) {
						screen.Fill(color.RGBA{18, 16, 22, 255})
						if view == 0 {
							r.drawSpellProjectileFx(screen, cell/2, cell/2, size, 1, 0, base, profile, 1.0, 3)
						} else {
							r.drawSpellProjectileFxHeadOn(screen, cell/2, cell/2, size, base, profile, 1.0, 3)
						}
					})
					row := si*views + view
					for y := 0; y < cell; y++ {
						for x := 0; x < cell; x++ {
							sheet.Set(f*cell+x, row*cell+y, screen.At(x, y))
						}
					}
				}
			}
		}
		fp, err := os.Create(filepath.Join(outRoot, fmt.Sprintf("%s.png", key)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(fp, sheet); err != nil {
			t.Fatal(err)
		}
		fp.Close()
		style := profile.style
		if style == "" {
			style = "(school default orb)"
		}
		t.Logf("%-20s style=%s", key, style)
	}
	t.Logf("sheets -> %s", outRoot)
}
