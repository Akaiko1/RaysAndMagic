//go:build debug

package game

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_UIFrameGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_ui_frame_runtime_qa")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	type resolution struct {
		w, h int
	}
	resolutions := []resolution{
		{1024, 768},
		{1280, 720},
		{1366, 768},
		{1920, 1080},
		{2560, 1440},
		{3440, 1440},
		{3840, 2160},
	}

	render := func(name string, physical resolution, draw func(*ebiten.Image)) {
		logicalW, logicalH := g.gameLoop.Layout(physical.w, physical.h)
		logical := ebiten.NewImage(logicalW, logicalH)
		output := ebiten.NewImage(physical.w, physical.h)
		runOnDrawFrame(func(_ *ebiten.Image) {
			logical.Fill(color.RGBA{18, 18, 22, 255})
			draw(logical)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(float64(physical.w)/float64(logicalW), float64(physical.h)/float64(logicalH))
			op.Filter = ebiten.FilterNearest
			output.DrawImage(logical, op)
		})
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, output); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for _, resolution := range resolutions {
		suffix := fmt.Sprintf("%dx%d", resolution.w, resolution.h)
		g.currentTab = TabCharacters
		render("characters_tab_"+suffix, resolution, g.gameLoop.ui.drawTabbedMenu)

		g.partyCreate = newPartyCreateState(g.config)
		render("party_create_"+suffix, resolution, g.gameLoop.ui.drawPartyCreateScreen)
	}

	t.Logf("UI frame gallery -> %s", out)
}
