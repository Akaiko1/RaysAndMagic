//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/world"
)

func TestDebugSim_TerrainTransitionBoundaries(t *testing.T) {
	requireStandeeGPU(t)
	for _, tc := range []struct {
		name, biome, a, b string
		blend, cliff      bool
	}{
		{"natural", "forest", "empty", "clearing", true, false},
		{"inactive_neighbor", "forest", "empty", "clearing", false, false},
		{"water", "highlands", "empty", "water", true, false},
		{"bridge_void", "dragon_cliffs", "dragon_cliffs_bridge", "dragon_cliffs_chasm_floor", false, false},
		{"ground_void", "dragon_cliffs", "dragon_cliffs_floor", "dragon_cliffs_chasm_floor", false, false},
		{"east_land", "dragon_cliffs", "dragon_cliffs_floor", "dragon_cliffs_chasm_edge", true, true},
		{"east_drop", "dragon_cliffs", "dragon_cliffs_chasm_edge", "dragon_cliffs_floor", false, false},
		{"west_land", "dragon_cliffs", "dragon_cliffs_chasm_edge_b", "dragon_cliffs_floor", true, true},
		{"west_drop", "dragon_cliffs", "dragon_cliffs_floor", "dragon_cliffs_chasm_edge_b", false, false},
		{"rim_void", "dragon_cliffs", "dragon_cliffs_chasm_edge", "dragon_cliffs_chasm_floor", false, false},
		{"void_variants", "dragon_cliffs", "dragon_cliffs_chasm_floor", "dragon_cliffs_chasm_floor_b", true, false},
		{"paving_wood", "japanese_castle", "japanese_castle_cobble", "japanese_castle_wood", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := terrainTestRenderer(t, tc.biome)
			a, b := terrainTile(t, tc.a), terrainTile(t, tc.b)
			for y := 0; y < r.game.world.Height; y++ {
				for x := 0; x < r.game.world.Width; x++ {
					first := x < 12
					r.game.world.Tiles[y][x] = b
					if first {
						r.game.world.Tiles[y][x] = a
					}
				}
			}
			const w, h = 512, 300
			r.game.config.Display.ScreenWidth, r.game.config.Display.ScreenHeight = w, h
			r.game.config.Graphics.BrightnessMin = 1
			r.game.camera.X, r.game.camera.Y = 12*64, 8*64
			r.game.camera.Angle = math.Pi / 2
			r.game.camera.FOV = math.Pi / 2
			r.game.camera.ViewDist = 1e9
			r.ambientLight = 1
			var actual, baseline *image.RGBA
			runOnDrawFrame(func(_ *ebiten.Image) {
				textures := make([]floorTexture, len(r.floorTexGroups))
				for group, g := range r.floorTexGroups {
					cpu := image.NewRGBA(image.Rect(0, 0, 32, 32))
					c := color.RGBA{20, 230, 20, 255}
					if group == r.floorTextureGroupForTile(12, 12, b) {
						c = color.RGBA{230, 20, 20, 255}
					}
					for y := 0; y < 32; y++ {
						for x := 0; x < 32; x++ {
							v := c
							if (group == "chasm_edge_0" && x >= 16) || (group == "chasm_edge_1" && x < 16) {
								v = color.RGBA{10, 10, 240, 255}
							}
							cpu.SetRGBA(x, y, v)
						}
					}
					textures[g.start] = floorTexture{width: 32, height: 32, pixels: cpu.Pix}
				}
				packed, tw, th, mip := prepareFloorAtlas(textures)
				r.floorTexAtlas = ebiten.NewImageFromImage(packed)
				r.floorTexTileW, r.floorTexTileH, r.floorTexMaxMip, r.floorTexCount = tw, th, mip, len(textures)
				r.buildFloorColorMap(r.game.world.Width, r.game.world.Height)
				_, indices, _ := r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
				for i := 2; i < len(indices.Pix); i += 4 {
					if tc.name == "inactive_neighbor" && int(indices.Pix[i-2]) == r.floorTexGroups["clearing"].start+1 {
						indices.Pix[i] &= ^floorBlendBoundary
					}
				}
				r.floorTextureIndexMap.WritePixels(indices.Pix)
				dst := ebiten.NewImage(w, h)
				r.drawSimpleFloorCeiling(dst)
				actual = snapshotUIImage(dst)
				for i := 2; i < len(indices.Pix); i += 4 {
					indices.Pix[i] = 0
				}
				r.floorTextureIndexMap.WritePixels(indices.Pix)
				dst.Clear()
				r.drawSimpleFloorCeiling(dst)
				baseline = snapshotUIImage(dst)
				dst.Deallocate()
				r.floorTexAtlas.Deallocate()
				r.floorColorMap.Deallocate()
				r.floorTextureIndexMap.Deallocate()
				r.floorShoreMap.Deallocate()
				r.floorShader.Deallocate()
			})
			change := 0
			for _, x := range []int{w/2 - 3, w/2 + 3} {
				got, want := actual.RGBAAt(x, h-20), baseline.RGBAAt(x, h-20)
				change += abs(int(got.R)-int(want.R)) + abs(int(got.G)-int(want.G)) + abs(int(got.B)-int(want.B))
				if tc.cliff && got.B > 90 {
					t.Fatalf("cliff's dark drop side leaked onto land: %v", got)
				}
			}
			if tc.blend && change < 20 {
				t.Fatalf("transition stayed hard: delta=%d", change)
			}
			if !tc.blend && change != 0 {
				t.Fatalf("protected edge blended: delta=%d", change)
			}
		})
	}
}

// Export actual perspective frames for visual review, using the same uploaded
// maps and draw entry point as the game, not a reimplementation of the shader.
func TestDebugSim_TerrainTransitionGallery(t *testing.T) {
	requireStandeeGPU(t)
	for _, biome := range []string{"forest", "desert", "dragon_cliffs"} {
		t.Run(biome, func(t *testing.T) {
			r, _ := terrainTestRenderer(t, biome)
			keys := []string{"empty", "clearing", "water"}
			if biome == "dragon_cliffs" {
				keys = []string{"dragon_cliffs_floor", "dragon_cliffs_basalt_floor", "dragon_cliffs_chasm_edge", "dragon_cliffs_chasm_floor", "dragon_cliffs_chasm_floor_b", "dragon_cliffs_chasm_edge_b", "dragon_cliffs_bridge"}
			}
			for y := 0; y < r.game.world.Height; y++ {
				for x := 0; x < r.game.world.Width; x++ {
					idx := 0
					if biome != "dragon_cliffs" {
						if x >= 12 {
							idx = 2
						}
						if biome == "forest" && x < 9 && y > 13 {
							idx = 1
						}
					} else {
						switch {
						case x < 9:
							idx = 0
						case x < 11:
							idx = 1
						case x == 11:
							idx = 2
						case x < 15:
							idx = 3
						case x < 17:
							idx = 4
						case x == 17:
							idx = 5
						default:
							idx = 0
						}
						if y == 14 && x >= 11 && x <= 17 {
							idx = 6
						}
					}
					r.game.world.Tiles[y][x] = terrainTile(t, keys[idx])
				}
			}
			paths := map[string][]string{}
			for group, files := range world.GlobalWorldManager.Biomes[r.floorBiomeKeyAt(0, 0)].FloorTextureGroups {
				for _, name := range files {
					paths[group] = append(paths[group], filepath.Join("../../assets/sprites/floor", name+".png"))
				}
			}
			textures, groups := prepareFloorTextureGroups(paths)
			r.floorTexGroups = groups
			const w, h = 1024, 640
			r.game.config.Display.ScreenWidth, r.game.config.Display.ScreenHeight = w, h
			r.game.camera.X, r.game.camera.Y = 10.4*64, 9*64
			r.game.camera.Angle = math.Pi / 2
			r.game.camera.FOV = squareProjectionFOV(w, h)
			r.game.camera.ViewDist = 40 * 64
			r.ambientLight = 1
			runOnDrawFrame(func(_ *ebiten.Image) {
				packed, tw, th, mip := prepareFloorAtlas(textures)
				r.floorTexAtlas = ebiten.NewImageFromImage(packed)
				r.floorTexTileW, r.floorTexTileH, r.floorTexMaxMip, r.floorTexCount = tw, th, mip, len(textures)
				r.buildFloorColorMap(r.game.world.Width, r.game.world.Height)
				dst := ebiten.NewImage(w, h)
				r.drawSimpleFloorCeiling(dst)
				if dir := os.Getenv("RAM_TERRAIN_GALLERY"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Error(err)
					}
					f, err := os.Create(filepath.Join(dir, biome+".png"))
					if err != nil {
						t.Error(err)
					} else {
						if err := png.Encode(f, snapshotUIImage(dst)); err != nil {
							t.Error(err)
						}
						f.Close()
					}
				}
				dst.Deallocate()
				r.floorTexAtlas.Deallocate()
				r.floorColorMap.Deallocate()
				r.floorTextureIndexMap.Deallocate()
				r.floorShoreMap.Deallocate()
				r.floorShader.Deallocate()
			})
		})
	}
}

func TestDebugSim_TerrainCornersAndWorldAnchoring(t *testing.T) {
	requireStandeeGPU(t)
	for _, count := range []int{3, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			r, _ := terrainTestRenderer(t, "forest")
			const w, h = 512, 300
			r.game.config.Display.ScreenWidth, r.game.config.Display.ScreenHeight = w, h
			r.game.config.Graphics.BrightnessMin = 1
			r.game.camera.X, r.game.camera.Y = 12*64, 10.5*64
			r.game.camera.Angle = math.Pi / 2
			r.game.camera.FOV = math.Pi / 2
			r.game.camera.ViewDist = 1e9
			r.ambientLight = 1
			var first, shifted *image.RGBA
			runOnDrawFrame(func(_ *ebiten.Image) {
				textures := []floorTexture{}
				for _, c := range []color.RGBA{{240, 20, 20, 255}, {20, 240, 20, 255}, {20, 20, 240, 255}, {240, 240, 240, 255}} {
					cpu := image.NewRGBA(image.Rect(0, 0, 32, 32))
					for y := 0; y < 32; y++ {
						for x := 0; x < 32; x++ {
							cpu.SetRGBA(x, y, c)
						}
					}
					textures = append(textures, floorTexture{width: 32, height: 32, pixels: cpu.Pix})
				}
				packed, tw, th, mip := prepareFloorAtlas(textures)
				r.floorTexAtlas = ebiten.NewImageFromImage(packed)
				r.floorTexTileW, r.floorTexTileH, r.floorTexMaxMip, r.floorTexCount = tw, th, mip, 4
				r.buildFloorColorMap(r.game.world.Width, r.game.world.Height)
				idx := image.NewRGBA(image.Rect(0, 0, r.game.world.Width, r.game.world.Height))
				for y := 0; y < idx.Bounds().Dy(); y++ {
					for x := 0; x < idx.Bounds().Dx(); x++ {
						i := 1
						if x >= 12 {
							i++
						}
						if y >= 12 {
							i += 2
						}
						if i > count {
							i = 1
						}
						idx.SetRGBA(x, y, color.RGBA{uint8(i), 0, floorBlendNatural | floorBlendBoundary, 255})
					}
				}
				r.floorTextureIndexMap.WritePixels(idx.Pix)
				dst := ebiten.NewImage(w, h)
				r.drawSimpleFloorCeiling(dst)
				first = snapshotUIImage(dst)
				// Account for the pixel center at row 250.5 when translating by
				// exactly 64 pixels. The same world points must keep their mask/noise.
				r.game.camera.X += (150.0 / (250.5 - 150.0)) * 128.0 / 512.0 * 64.0
				dst.Clear()
				r.drawSimpleFloorCeiling(dst)
				shifted = snapshotUIImage(dst)
				dst.Deallocate()
				r.floorTexAtlas.Deallocate()
				r.floorColorMap.Deallocate()
				r.floorTextureIndexMap.Deallocate()
				r.floorShoreMap.Deallocate()
				r.floorShader.Deallocate()
			})
			for x := w/2 - 12; x <= w/2+12; x++ {
				c, d := first.RGBAAt(x, 250), shifted.RGBAAt(x+64, 250)
				if c.A != 255 || int(c.R)+int(c.G)+int(c.B) < 190 {
					t.Fatalf("corner weight hole at %d: %v", x, c)
				}
				if abs(int(c.R)-int(d.R))+abs(int(c.G)-int(d.G))+abs(int(c.B)-int(d.B)) > 4 {
					t.Fatalf("world-space mask moved with camera: %v -> %v", c, d)
				}
			}
			c := first.RGBAAt(w/2, 250)
			if c.R < 55 || c.G < 55 || c.B < 55 {
				t.Fatalf("corner omitted a material: %v", c)
			}
		})
	}
}

func TestDebugSim_TerrainWithinGroupVariants(t *testing.T) {
	requireStandeeGPU(t)
	for _, tc := range []struct {
		name, biome, tile, group string
		shore, blend             bool
	}{
		{"sand", "desert", "empty", "default", false, true},
		{"water", "forest", "water", "water", false, true},
		{"void", "dragon_cliffs", "dragon_cliffs_chasm_floor", "chasm_floor_0", false, true},
		{"beach", "forest", "empty", "beach", true, true},
		{"hard", "japanese_castle", "japanese_castle_wood", "wood", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := terrainTestRenderer(t, tc.biome)
			r.floorTexGroups[tc.group] = floorTextureGroup{start: len(r.floorTexGroups), count: 2}
			for y := range r.game.world.Tiles {
				for x := range r.game.world.Tiles[y] {
					r.game.world.Tiles[y][x] = terrainTile(t, tc.tile)
					if tc.shore && x >= 12 {
						r.game.world.Tiles[y][x] = terrainTile(t, "water")
					}
				}
			}
			const w, h = 512, 300
			r.game.config.Display.ScreenWidth, r.game.config.Display.ScreenHeight = w, h
			r.game.config.Graphics.BrightnessMin = 1
			r.game.camera.X, r.game.camera.Y = 11.5*64, 8*64
			r.game.camera.Angle, r.game.camera.FOV, r.game.camera.ViewDist = math.Pi/2, math.Pi/2, 1e9
			r.ambientLight = 1
			runOnDrawFrame(func(_ *ebiten.Image) {
				textures := make([]floorTexture, len(r.floorTexGroups)+2)
				for i := range textures {
					cpu := image.NewRGBA(image.Rect(0, 0, 32, 32))
					c := color.RGBA{20, 230, 20, 255}
					if i == len(textures)-1 {
						c = color.RGBA{230, 20, 20, 255}
					}
					for y := 0; y < 32; y++ {
						for x := 0; x < 32; x++ {
							cpu.SetRGBA(x, y, c)
						}
					}
					textures[i] = floorTexture{width: 32, height: 32, pixels: cpu.Pix}
				}
				packed, tw, th, mip := prepareFloorAtlas(textures)
				r.floorTexAtlas = ebiten.NewImageFromImage(packed)
				r.floorTexTileW, r.floorTexTileH, r.floorTexMaxMip, r.floorTexCount = tw, th, mip, len(textures)
				r.buildFloorColorMap(r.game.world.Width, r.game.world.Height)
				dst := ebiten.NewImage(w, h)
				r.drawSimpleFloorCeiling(dst)
				actual := snapshotUIImage(dst)
				_, indices, _ := r.prepareFloorMaps(r.game.world.Width, r.game.world.Height)
				// Reference keeps the identical variant/shore samples but disables
				// neighbor blending, isolating seams within this single group.
				for i := 2; i < len(indices.Pix); i += 4 {
					indices.Pix[i] &^= floorBlendBoundary
				}
				r.floorTextureIndexMap.WritePixels(indices.Pix)
				dst.Clear()
				r.drawSimpleFloorCeiling(dst)
				baseline := snapshotUIImage(dst)
				changed := 0
				for y := 200; y < h; y++ {
					for x := w / 2; x < w-10; x++ {
						a, b := actual.RGBAAt(x, y), baseline.RGBAAt(x, y)
						if abs(int(a.R)-int(b.R))+abs(int(a.G)-int(b.G)) > 20 {
							changed++
						}
					}
				}
				if tc.blend && changed < 100 || !tc.blend && changed != 0 {
					t.Errorf("within-group seam: changed=%d, blend=%v", changed, tc.blend)
				}
				for _, img := range []*ebiten.Image{dst, r.floorTexAtlas, r.floorColorMap, r.floorTextureIndexMap, r.floorShoreMap} {
					img.Deallocate()
				}
				r.floorShader.Deallocate()
			})
		})
	}
}
