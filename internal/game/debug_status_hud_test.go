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
	"ugataima/internal/spells"
)

func TestDebugSim_StatusHUD(t *testing.T) {
	requireStandeeGPU(t)
	g := bootGameplayPreviewGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.showPartyStats = true
	keys := statusHUDCatalog()
	for _, key := range keys {
		g.setUtilityStatus(spells.SpellID(key), 1200)
	}
	ui := g.gameLoop.ui
	runOnDrawFrame(func(*ebiten.Image) {
		cell := ebiten.NewImage(24, 24)
		defer cell.Deallocate()
		for _, key := range keys {
			status := g.utilitySpellStatuses[spells.SpellID(key)]
			for _, tc := range []struct {
				remaining int
				fill      int
				ink       color.RGBA
			}{
				{100, 24, color.RGBA{0, 200, 0, 255}},
				{50, 12, color.RGBA{200, 200, 0, 255}},
				{20, 4, color.RGBA{200, 100, 0, 255}},
			} {
				cell.Clear()
				x, y, w, h := ui.drawSpellIcon(cell, 0, 0, 24, status.Icon, status.Fallback, tc.remaining, 100)
				if x != 0 || y != 0 || w != 24 || h != 24 {
					t.Fatalf("%s changed click bounds", key)
				}
				shot := snapshotUIImage(cell)
				for px := 0; px < tc.fill; px++ {
					if got := shot.RGBAAt(px, 22); got != tc.ink {
						t.Fatalf("%s duration %d: bar clipped at x=%d: %v", key, tc.remaining, px, got)
					}
				}
			}
		}
	})
	if out := os.Getenv("RAM_STATUS_HUD_DIR"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		for _, res := range [][2]int{{800, 600}, {1920, 1080}} {
			w, h := g.gameLoop.Layout(res[0], res[1])
			shot := captureGameplayPreviewFrame(t, g, w, h)
			path := filepath.Join(out, fmt.Sprintf("status-hud-native-%dx%d.png", w, h))
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, shot)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("save screenshot: %v / %v", err, closeErr)
			}
		}
	}
}
