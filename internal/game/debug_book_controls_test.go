//go:build debug

package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_BookControlsGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	out := os.Getenv("RAM_BOOK_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-book-controls")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, trap := range []bool{false, true} {
		name := "spellbook"
		if trap {
			name = "trap-book"
		}
		t.Run(name, func(t *testing.T) {
			h, _, _, _ := bookControlsHarness(t, trap, false)
			for _, size := range [][2]int{{800, 680}, {1920, 1080}} {
				var problem error
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.g.config.Display.ScreenWidth, h.g.config.Display.ScreenHeight = size[0], size[1]
					screen := ebiten.NewImage(size[0], size[1])
					defer screen.Deallocate()
					h.ui.Draw(screen)
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("%s-%dx%d.png", name, size[0], size[1])))
					if err != nil {
						problem = err
						return
					}
					defer f.Close()
					problem = png.Encode(f, screen)
				})
				if problem != nil {
					t.Fatal(problem)
				}
			}
		})
	}
}
