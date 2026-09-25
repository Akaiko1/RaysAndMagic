//go:build debug

package game

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pixel assertions exercise the shared renderer through every queue and the
// direct menu path, including the separate comparison card.
func TestDebugSim_TooltipSectionPresentation(t *testing.T) {
	requireStandeeGPU(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ui := g.gameLoop.ui
	lines := []string{"Tooltip", "", "EFFECTS", "Current recovery: 42 HP", "Base recovery: 10 HP", "", "USAGE", "Double-click to use"}
	for _, kind := range []string{"plain", "icon", "titled", "comparison", "menu"} {
		for _, size := range [][2]int{{800, 600}, {1024, 768}} {
			t.Run(kind, func(t *testing.T) {
				var failed string
				runOnDrawFrame(func(*ebiten.Image) {
					screen := ebiten.NewImage(size[0], size[1])
					defer screen.Deallocate()
					ui.tooltipCompareLines = nil
					x, y := 16, 16
					cap := tooltipColumnWidth(size[0], 1)
					hasIcon := false
					switch kind {
					case "plain", "menu":
						ui.queueTooltip(lines, x, y)
					case "icon":
						ui.queueTooltipIcon(lines, campHUDSprite, x, y)
						hasIcon = ui.tooltipIcon != ""
					case "titled":
						colors := make([]color.Color, len(lines))
						colors[3] = equipmentBenefitColor
						ui.queueTitledTooltipIcon(lines, colors, woodPlateColor, nil, campHUDSprite, x, y)
						hasIcon = ui.tooltipIcon != ""
					case "comparison":
						ui.queueTooltip([]string{"Hovered item"}, x, y)
						ui.queueTitledTooltipComparison(lines, nil, woodPlateColor, nil)
						pair := ui.queuedTooltipPairLayout(size[0], size[1])
						_, x = tooltipPairX(x, pair.mainW, pair.compareW, tooltipCompareGap, size[0])
						cap = pair.compareCap
					}
					if kind == "menu" {
						ui.drawTooltipLines(screen, x, y, lines)
					} else {
						ui.drawQueuedTooltips(screen)
					}
					layout := layoutTooltip(lines, hasIcon, cap, size[1])
					pixels := snapshotUIImage(screen)
					for _, row := range layout.rows {
						if row.source == 2 || row.source == 6 {
							if pixels.RGBAAt(x+row.x+row.w-4, y+row.y+layout.lineHeight/2) != (color.RGBA{48, 49, 76, 255}) {
								failed = "section band missing from production renderer"
							}
						}
						if kind == "titled" && row.source == 3 {
							green := false
							for py := y + row.y; py < y+row.y+layout.lineHeight; py++ {
								for px := x + row.x; px < x+row.x+row.w; px++ {
									green = green || pixels.RGBAAt(px, py) == equipmentBenefitColor
								}
							}
							if !green {
								failed = "semantic gain/set color was overwritten"
							}
						}
					}
				})
				if failed != "" {
					t.Fatal(failed)
				}
			})
		}
	}
}
