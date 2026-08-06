//go:build debug

package game

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// compassGalleryViewpoint chooses a walkable 13x13 window with the greatest
// authored tile variety. This exposes each map's visual vocabulary in QA more
// reliably than a spawn point that may intentionally sit in an empty courtyard.
func compassGalleryViewpoint(wm *world.WorldManager, mapKey string, w *world.World3D) (int, int) {
	minX, minY, maxX, maxY := 0, 0, w.Width, w.Height
	if region := wm.OpenWorldRegionByKey(mapKey); region != nil {
		minX, minY = region.OffsetX, region.OffsetY
		maxX, maxY = minX+region.Width, minY+region.Height
	}

	bestX, bestY, bestScore := minX, minY, -1
	const viewRange = 6
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			tile := w.GetTileAtGrid(x, y)
			if world.GlobalTileManager == nil || !world.GlobalTileManager.IsWalkable(tile) {
				continue
			}
			seen := make(map[string]struct{})
			spriteCells := 0
			for dy := -viewRange; dy <= viewRange; dy++ {
				for dx := -viewRange; dx <= viewRange; dx++ {
					if dx*dx+dy*dy > viewRange*viewRange {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < minX || ny < minY || nx >= maxX || ny >= maxY {
						continue
					}
					near := w.GetTileAtGrid(nx, ny)
					seen[world.GlobalTileManager.GetTileKey(near)] = struct{}{}
					if data := world.GlobalTileManager.GetTileData(near); data != nil && data.Sprite != "" {
						spriteCells++
					}
				}
			}
			score := len(seen)*100 + min(spriteCells, 99)
			if score > bestScore {
				bestX, bestY, bestScore = x, y, score
			}
		}
	}
	return bestX, bestY
}

func TestDebugSim_CompassAllMapsGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()

	wm := world.GlobalWorldManager
	if wm == nil {
		t.Fatal("world manager missing")
	}
	keys := make([]string, 0, len(wm.MapConfigs))
	for key := range wm.MapConfigs {
		if wm.IsValidMap(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_compass_all_maps")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	ui := g.gameLoop.ui
	radius := ui.compassRadius()
	const pad = 22
	compassSide := 2 * (radius + pad)
	const columns = 4
	cellW, cellH := compassSide+12, compassSide+24
	rows := (len(keys) + columns - 1) / columns
	contact := ebiten.NewImage(columns*cellW, rows*cellH)
	contact.Fill(color.RGBA{8, 10, 14, 255})

	writePNG := func(path string, img *ebiten.Image) {
		t.Helper()
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for i, key := range keys {
		if err := wm.SwitchToMap(key); err != nil {
			t.Fatalf("switch %s: %v", key, err)
		}
		w := wm.GetCurrentWorld()
		if w == nil {
			t.Fatalf("map %s has no world", key)
		}
		tx, ty := compassGalleryViewpoint(wm, key, w)
		g.world = w
		tileSize := float64(g.config.GetTileSize())
		g.camera.X = (float64(tx) + 0.5) * tileSize
		g.camera.Y = (float64(ty) + 0.5) * tileSize
		ui.invalidateCompassTileLayer()

		crop := ebiten.NewImage(compassSide, compassSide)
		runOnDrawFrame(func(_ *ebiten.Image) {
			crop.Fill(color.RGBA{8, 10, 14, 255})
			ui.drawCompassAt(crop, compassSide/2, compassSide/2)
			col, row := i%columns, i/columns
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(col*cellW+6), float64(row*cellH))
			contact.DrawImage(crop, op)
			drawCenteredDebugText(contact, key, col*cellW, row*cellH+compassSide+2, cellW, 18)
		})
		writePNG(filepath.Join(out, key+".png"), crop)
	}
	writePNG(filepath.Join(out, "contact_sheet.png"), contact)
	t.Logf("compass gallery: %d maps -> %s", len(keys), out)
}
