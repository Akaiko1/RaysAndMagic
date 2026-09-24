//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/items"
	"ugataima/internal/storage"
)

// Set RAM_WEAPON_TOOLTIP_OUTPUT to capture the actual queued tooltip renderer.
func TestDebugSim_WeaponTooltipGroupingGallery(t *testing.T) {
	requireStandeeGPU(t)
	out := os.Getenv("RAM_WEAPON_TOOLTIP_OUTPUT")
	if out == "" {
		t.Skip("set RAM_WEAPON_TOOLTIP_OUTPUT for gallery output")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ch := gmReferenceChar(g.config)
	g.party.Members[0] = ch
	ui := g.gameLoop.ui
	for _, key := range []string{"iron_sword", "tanegashima", "broodspike", "tonbogiri"} {
		it := items.CreateWeaponFromYAML(key)
		items.EnsureInstanceID(&it)
		for _, compare := range []bool{false, true} {
			ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
			if compare {
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
			}
			for _, full := range []bool{false, true} {
				card := GetItemTooltip(it, ch, g.combat, full)
				for _, size := range [][2]int{{800, 600}, {1024, 768}} {
					var shot *image.RGBA
					runOnDrawFrame(func(*ebiten.Image) {
						dst := ebiten.NewImage(size[0], size[1])
						defer dst.Deallocate()
						dst.Fill(color.RGBA{18, 20, 25, 255})
						ui.tooltipCompareLines = nil
						ui.queueItemTooltip(strings.Split(card, "\n"), it, ch, 16, 16)
						if compare {
							lines := strings.Split(GetItemComparisonTooltip(it, ch, g.combat), "\n")
							plate, title := ui.itemTitleColors(it)
							ui.queueTitledTooltipComparison(lines, equipmentComparisonColors(lines, nil), plate, title)
						}
						ui.drawQueuedTooltips(dst)
						shot = snapshotUIImage(dst)
					})
					for y := 0; y < size[1]; y++ {
						for x := 0; x < size[0]; x++ {
							if x >= tooltipScreenMargin && x < size[0]-tooltipScreenMargin && y >= tooltipScreenMargin && y < size[1]-tooltipScreenMargin {
								continue
							}
							if shot.RGBAAt(x, y) != (color.RGBA{18, 20, 25, 255}) {
								t.Fatalf("%s/full%v/compare%v/%v: tooltip paints outside viewport margin at %d,%d", key, full, compare, size, x, y)
							}
						}
					}
					name := fmt.Sprintf("%s-full%v-compare%v-%dx%d", key, full, compare, size[0], size[1])
					f, err := os.Create(filepath.Join(out, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, shot)
					closeErr := f.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("write PNG: %v; close: %v", err, closeErr)
					}
					if err := os.WriteFile(filepath.Join(out, name+".txt"), []byte(card), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}
