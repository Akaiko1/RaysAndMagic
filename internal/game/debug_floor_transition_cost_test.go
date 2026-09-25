//go:build debug

package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"path/filepath"
	"testing"
	"time"
)

// This diagnostic includes GPU readback to flush queued rendering. It compares
// the same atlas with blending enabled/disabled; it is not a full-scene FPS test.
func TestDebugSim_FloorTransitionCost(t *testing.T) {
	requireStandeeGPU(t)
	for _, biome := range []string{"forest", "desert", "dragon_cliffs"} {
		t.Run(biome, func(t *testing.T) {
			r, wm := terrainTestRenderer(t, biome)
			w, h := r.game.world.Width, r.game.world.Height
			start := time.Now()
			for i := 0; i < 100; i++ {
				r.prepareFloorMaps(w, h)
			}
			t.Logf("prepare %dx%d: %s/map", w, h, time.Since(start)/100)
			r.game.config.Display.ScreenWidth, r.game.config.Display.ScreenHeight = 1280, 720
			r.game.camera.X, r.game.camera.Y = 12*64, 8*64
			r.game.camera.Angle, r.game.camera.FOV = math.Pi/2, math.Pi/2
			r.game.camera.ViewDist = 40 * 64
			r.ambientLight = 1
			runOnDrawFrame(func(_ *ebiten.Image) {
				paths := map[string][]string{}
				for group, names := range wm.Biomes[biome].FloorTextureGroups {
					for _, name := range names {
						p, _ := filepath.Abs("../../assets/sprites/floor/" + name + ".png")
						paths[group] = append(paths[group], p)
					}
				}
				textures, groups := prepareFloorTextureGroups(paths)
				if len(textures) == 0 {
					t.Error("missing textures")
					return
				}
				r.buildFloorTexAtlas(textures)
				r.floorTexGroups = groups
				ground := "empty"
				if biome == "dragon_cliffs" {
					ground = "dragon_cliffs_floor"
				}
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						r.game.world.Tiles[y][x] = terrainTile(t, ground)
					}
				}
				r.game.world.Tiles[10][12] = terrainTile(t, "water")
				r.buildFloorColorMap(w, h)
				_, idx, _ := r.prepareFloorMaps(w, h)
				dst := ebiten.NewImage(1280, 720)
				pixels := make([]byte, 1280*720*4)
				for _, blend := range []bool{false, true} {
					state := append([]byte(nil), idx.Pix...)
					if !blend {
						for i := 2; i < len(state); i += 4 {
							state[i] = 0
						}
					}
					r.floorTextureIndexMap.WritePixels(state)
					for i := 0; i < 5; i++ {
						r.drawSimpleFloorCeiling(dst)
						dst.ReadPixels(pixels)
					}
					start := time.Now()
					for i := 0; i < 40; i++ {
						r.drawSimpleFloorCeiling(dst)
						dst.ReadPixels(pixels)
					}
					t.Logf("blend=%v 1280x720 draw+GPU-readback: %s/frame", blend, time.Since(start)/40)
				}
				dst.Deallocate()
				for _, img := range []*ebiten.Image{r.floorTexAtlas, r.floorColorMap, r.floorTextureIndexMap, r.floorShoreMap} {
					if img != nil {
						img.Deallocate()
					}
				}
				if r.floorShader != nil {
					r.floorShader.Deallocate()
				}
			})

		})
	}
}
