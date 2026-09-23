//go:build debug

package game

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/items"
)

func TestDebugSim_ActiveSetTooltip(t *testing.T) {
	requireStandeeGPU(t)
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ch := g.party.Members[0]
	armor, err := items.TryCreateItemFromYAML("golden_armor")
	if err != nil {
		t.Fatal(err)
	}
	sword := items.CreateWeaponFromYAML("gold_sword")
	items.EnsureInstanceID(&armor)
	items.EnsureInstanceID(&sword)
	ch.Equipment[items.SlotArmor], ch.Equipment[items.SlotMainHand] = armor, sword
	ui := g.gameLoop.ui
	for _, size := range [][2]int{{1024, 768}, {1280, 800}} {
		lines := strings.Split(GetItemTooltip(armor, ch, g.combat, false), "\n")
		ui.queueItemTooltip(lines, armor, ch, 10, 10)
		runOnDrawFrame(func(_ *ebiten.Image) {
			dst := ebiten.NewImage(size[0], size[1])
			defer dst.Deallocate()
			dst.Fill(color.RGBA{20, 20, 20, 255})
			drawTooltip(dst, ui.tooltipLines, ui.tooltipColors, ui.tooltipTitleColor, nil, "", 10, 10, size[0]-10, g.sprites)
			pixels := snapshotUIImage(dst)
			green := 0
			for y := 0; y < size[1]; y++ {
				for x := 0; x < size[0]; x++ {
					if pixels.RGBAAt(x, y) == (color.RGBA{120, 225, 135, 255}) {
						green++
					}
				}
			}
			if green < 50 {
				t.Errorf("active set text was not rendered green: %d pixels", green)
			}
			f, err := os.Create(fmt.Sprintf("/tmp/rays-active-set-%d.png", size[0]))
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if err := png.Encode(f, pixels); err != nil {
				t.Error(err)
			}
		})
	}
}
