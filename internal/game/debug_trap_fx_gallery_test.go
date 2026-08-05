//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Trap-FX gallery - renders every bespoke armed_fx style from traps.yaml on a
// dark floor backdrop and dumps per-style frame strips plus one contact sheet,
// so armed-trap effects can be eyeballed and iterated without booting the game
// and walking a thief into position.
//
// The trap renderers take an already-projected trapAnchor, so the gallery can
// synthesise the anchor directly - no world, camera or depth buffers needed.
//
// Run with:  RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_TrapFxGallery -v

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	trapGalleryCellW  = 260
	trapGalleryCellH  = 210
	trapGalleryUnit   = 88.0 // perspective unit: a trap a couple of tiles away
	trapGalleryFrames = 4    // sampled frames per style on the contact sheet
	trapGalleryStride = 17   // frames skipped between samples (catches the cycles)
	trapGalleryStrip  = 48   // frames per style in the strip dump
)

func TestDebugSim_TrapFxGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatalf("traps: %v", err)
	}
	validateTrapFxStyles() // the gallery is also the typo check

	// Minimal game: these renderers only need the frame clock, the screen size
	// and the lazy glow sprite - no party, world content or bridges.
	g := &MMGame{config: cfg, world: newTestWorld(cfg)}
	r := NewRenderer(g)

	// Style -> the trap that declares it, so colours come from live content.
	type entry struct {
		style string
		trap  string
		rgb   [3]int
	}
	var entries []entry
	for key, def := range config.GlobalTrapConfig.Traps {
		if def == nil || def.ArmedFx == "" {
			continue
		}
		entries = append(entries, entry{def.ArmedFx, key, [3]int{
			clampColor(def.BorderColor[0]),
			clampColor(def.BorderColor[1]),
			clampColor(def.BorderColor[2]),
		}})
	}
	if len(entries) == 0 {
		t.Skip("no trap declares armed_fx")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].trap < entries[j].trap })

	outRoot := filepath.Join(os.Getenv("HOME"), "Downloads", "trap_fx_gallery")
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	backdrop := color.RGBA{38, 36, 34, 255} // dim dungeon floor: dark AND bright particles both read
	cell := ebiten.NewImage(trapGalleryCellW, trapGalleryCellH)
	anchor := trapAnchor{
		cx:   float64(trapGalleryCellW) / 2,
		fy:   float64(trapGalleryCellH) * 0.60,
		unit: trapGalleryUnit,
		fade: 1,
	}

	sheet := ebiten.NewImage(trapGalleryCellW*trapGalleryFrames, trapGalleryCellH*len(entries))
	sheet.Fill(backdrop)

	dump := func(img *ebiten.Image, path string) {
		fp, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer fp.Close()
		if err := png.Encode(fp, img); err != nil {
			t.Fatal(err)
		}
	}

	for row, e := range entries {
		draw, ok := trapFxStyleDraw[e.style]
		if !ok {
			t.Fatalf("trap %q: no renderer for armed_fx %q", e.trap, e.style)
		}
		dir := filepath.Join(outRoot, e.style)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Deterministic hash id, same shape as the live call site (tile-derived).
		id := row*73 + 131

		for f := 0; f < trapGalleryStrip; f++ {
			g.frameCount = int64(f)
			runOnDrawFrame(func(_ *ebiten.Image) {
				cell.Fill(backdrop)
				draw(r, cell, anchor, e.rgb, id)
			})
			dump(cell, filepath.Join(dir, fmt.Sprintf("f%02d.png", f)))
		}

		// Contact-sheet row: frames spread across the animation cycles.
		for col := 0; col < trapGalleryFrames; col++ {
			g.frameCount = int64(col * trapGalleryStride)
			runOnDrawFrame(func(_ *ebiten.Image) {
				cell.Fill(backdrop)
				draw(r, cell, anchor, e.rgb, id)
			})
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(col*trapGalleryCellW), float64(row*trapGalleryCellH))
			sheet.DrawImage(cell, op)
		}
		t.Logf("row %d: %-14s (%s) rgb=%v -> %s", row, e.style, e.trap, e.rgb, dir)
	}

	dump(sheet, filepath.Join(outRoot, "contact.png"))
	t.Logf("contact sheet: %s", filepath.Join(outRoot, "contact.png"))
}
