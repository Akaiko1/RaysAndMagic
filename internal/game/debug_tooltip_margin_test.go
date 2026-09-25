//go:build debug

package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_TooltipViewportMargins(t *testing.T) {
	requireStandeeGPU(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ui := g.gameLoop.ui
	for _, res := range [][2]int{{800, 600}, {1280, 720}, {1920, 1080}} {
		for _, point := range [][2]int{{0, 0}, {res[0] + 12, 0}, {0, res[1] + 8}, {res[0] + 12, res[1] + 8}} {
			for _, kind := range []string{"plain", "icon", "comparison", "save row"} {
				t.Run(fmt.Sprintf("%dx%d/%v/%s", res[0], res[1], point, kind), func(t *testing.T) {
					var pixels []byte
					runOnDrawFrame(func(_ *ebiten.Image) {
						screen := ebiten.NewImage(res[0], res[1])
						defer screen.Deallocate()
						lines := []string{"Camp", "Restores living party members' HP and SP.", "Make camp (1 food)"}
						ui.tooltipCompareLines = nil
						if kind == "save row" {
							ui.drawTooltipLines(screen, point[0], point[1], lines)
						} else {
							if kind == "plain" {
								ui.queueTooltip(lines, point[0], point[1])
							} else {
								ui.queueTooltipIcon(lines, campHUDSprite, point[0], point[1])
							}
							if kind == "comparison" {
								ui.queueTooltipComparison(lines, nil)
							}
							ui.drawQueuedTooltips(screen)
						}
						pixels = make([]byte, res[0]*res[1]*4)
						screen.ReadPixels(pixels)
					})
					painted := 0
					for y := 0; y < res[1]; y++ {
						for x := 0; x < res[0]; x++ {
							if pixels[(y*res[0]+x)*4+3] == 0 {
								continue
							}
							painted++
							if x < tooltipScreenMargin || y < tooltipScreenMargin || x >= res[0]-tooltipScreenMargin || y >= res[1]-tooltipScreenMargin {
								t.Fatalf("paint crossed viewport inset at %d,%d", x, y)
							}
						}
					}
					if painted == 0 {
						t.Fatal("tooltip was not drawn")
					}
				})
			}
		}
	}
}
