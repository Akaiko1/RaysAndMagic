//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Weapon-projectile gallery - renders the flying signature of every weapon that
// authors graphics.projectile_fx, at the REAL in-game sizes, with the same base
// silhouette the game draws under it (bullet tracer for blasters, arrow quad
// for bows). Melee flourishes have debug_melee_fx_gallery_test.go; this is the
// ranged half, so a bow or blaster overlay can be judged without firing it.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_WeaponProjectileFxGallery -v
// RAM_FX_WEAPONS=wyrmspine_bow,longlance_rifle narrows the set.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_WeaponProjectileFxGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()

	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("RAM_FX_WEAPONS"), ",") {
		if s != "" {
			only[s] = true
		}
	}

	var keys []string
	for key, def := range config.GlobalWeapons.Weapons {
		if def == nil || def.Graphics == nil {
			continue
		}
		if len(only) > 0 && !only[key] {
			continue
		}
		if def.Graphics.ProjectileFx == "" && !only[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("no weapon authors projectile_fx")
	}

	outRoot := filepath.Join(os.Getenv("HOME"), "Downloads", "weapon_projectile_fx_gallery")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	const cell, frames = 200, 4
	screen := ebiten.NewImage(cell, cell)
	for _, key := range keys {
		def := config.GlobalWeapons.Weapons[key]
		style := def.Graphics.ProjectileFx
		col := [3]int{220, 220, 235}
		if len(def.Graphics.Color) == 3 {
			col = [3]int{def.Graphics.Color[0], def.Graphics.Color[1], def.Graphics.Color[2]}
		}
		blaster := strings.EqualFold(def.Category, "blaster")
		magic := strings.EqualFold(def.Category, "staff") || strings.EqualFold(def.Category, "book")
		magicProfile := r.weaponFxProfile(def)
		// Judge at the sizes the projectile actually takes on screen: point
		// blank (max_size), mid flight (base_size) and near its cap (min_size),
		// side-on, right-hand release, one-tile and 1.5-tile convergence,
		// outgoing head-on and incoming head-on for each size.
		sizes := []float64{float64(def.Graphics.MaxSize), float64(def.Graphics.BaseSize), math.Max(4, float64(def.Graphics.MinSize))}
		const views = 6
		sheet := image.NewNRGBA(image.Rect(0, 0, cell*frames, cell*len(sizes)*views))
		for si, size := range sizes {
			for view := 0; view < views; view++ {
				for f := 0; f < frames; f++ {
					g.frameCount += 7 // spread the animation clock across the strip
					runOnDrawFrame(func(_ *ebiten.Image) {
						screen.Fill(color.RGBA{18, 16, 22, 255})
						if view == 0 {
							r.drawWeaponProjectileFx(style, screen, cell/2, cell/2, size, 1, 0, 1.0, 3)
							if blaster {
								r.drawBulletTracer(screen, cell/2, cell/2, size, 0, 1, col, 1.0, 3)
							} else if magic {
								r.drawSpellProjectileFx(screen, cell/2, cell/2, size, 1, 0, col, magicProfile, 1.0, 3)
							} else {
								r.drawArrowQuad(screen, cell/2, cell/2, size, 0, col, 1.0)
							}
						} else if !blaster && !magic && view >= 1 && view <= 3 {
							convergence := 1.0
							if view == 2 {
								convergence = bowHandConvergence(g.config.GetTileSize(), g.config.GetTileSize())
							} else if view == 3 {
								convergence = bowHandConvergence(1.5*g.config.GetTileSize(), g.config.GetTileSize())
							}
							r.drawOutgoingBowFromHand(style, screen, cell/2, cell/2, size, col, 1.0, 3, convergence)
						} else {
							incoming := view == 5
							if blaster {
								r.drawWeaponProjectileFxHeadOn(style, screen, cell/2, cell/2, size, 1.0, 3)
								r.drawBulletTracer(screen, cell/2, cell/2, size, 1, 0, col, 1.0, 3)
							} else if magic {
								r.drawWeaponProjectileFxHeadOn(style, screen, cell/2, cell/2, size, 1.0, 3)
								r.drawSpellProjectileFxHeadOn(screen, cell/2, cell/2, size, col, magicProfile, 1.0, 3)
							} else {
								r.drawBowWeaponProjectileFxHeadOn(style, screen, cell/2, cell/2, size, 1.0, 3, incoming)
								r.drawArrowHeadOn(screen, cell/2, cell/2, size, col, 1.0, incoming)
							}
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
		t.Logf("%-20s style=%-18s sizes=%v blaster=%v", key, style, sizes, blaster)
	}
	t.Logf("sheets -> %s", outRoot)
}
