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

func TestDebugSim_ReviewTooltipGallery(t *testing.T) {
	requireStandeeGPU(t)
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
	ch := gmReferenceChar(g.config)
	g.party.Members[0] = ch
	ui := g.gameLoop.ui
	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic-review-fixes")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tanegashima", "broodspike", "tonbogiri", "golden_armor"} {
		var it items.Item
		if key == "golden_armor" {
			it = items.CreateItemFromYAML(key)
			sword := items.CreateWeaponFromYAML("gold_sword")
			items.EnsureInstanceID(&sword)
			ch.Equipment[items.SlotMainHand] = sword
		} else {
			it = items.CreateWeaponFromYAML(key)
		}
		items.EnsureInstanceID(&it)
		ch.Equipment[it.PreferredSlot(items.SlotMainHand)] = it
		for _, size := range [][2]int{{800, 600}, {1280, 800}} {
			for _, full := range []bool{false, true} {
				text := GetItemTooltip(it, ch, g.combat, full)
				var shot *image.RGBA
				runOnDrawFrame(func(*ebiten.Image) {
					dst := ebiten.NewImage(size[0], size[1])
					defer dst.Deallocate()
					dst.Fill(color.RGBA{18, 20, 25, 255})
					ui.queueItemTooltip(strings.Split(text, "\n"), it, ch, size[0]-10, size[1]-10)
					ui.drawQueuedTooltips(dst)
					shot = snapshotUIImage(dst)
				})
				path := filepath.Join(out, fmt.Sprintf("%s-full%v-%dx%d.png", key, full, size[0], size[1]))
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, shot)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		}
	}
}
