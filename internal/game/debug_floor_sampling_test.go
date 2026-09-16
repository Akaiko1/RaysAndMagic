//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Exercise the shipped floor shader on high-frequency ground while moving.
// A minified stripe texture must not turn into broad moving bright/dark bands.
func TestDebugSim_FloorMinificationStable(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, angle := range []float64{0, math.Pi / 4, math.Pi / 2} {
		t.Run(fmt.Sprint(angle), func(t *testing.T) {
			shader, err := ebiten.NewShader([]byte(floorShaderSrc))
			if err != nil {
				t.Fatal(err)
			}
			var dst, atlas, base, index *ebiten.Image
			const w, h = 640, 120
			runOnDrawFrame(func(_ *ebiten.Image) {
				cpu := image.NewRGBA(image.Rect(0, 0, 64, 64))
				for y := 0; y < 64; y++ {
					for x := 0; x < 64; x++ {
						v := uint8(0)
						axis := x
						if angle == math.Pi/2 {
							axis = y
						}
						if axis%2 == 0 {
							v = 255
						}
						cpu.SetRGBA(x, y, color.RGBA{v, v, v, 255})
					}
				}
				packed, _, _, _ := prepareFloorAtlas([]floorTexture{{pixels: cpu.Pix, width: 64, height: 64}})
				atlas = ebiten.NewImageFromImage(packed)
				base = ebiten.NewImage(32, 32)
				base.Fill(color.RGBA{128, 128, 128, 255})
				index = ebiten.NewImage(32, 32)
				index.Fill(color.RGBA{1, 0, 0, 255})
				dst = ebiten.NewImage(w, h)
			})
			defer shader.Deallocate()
			defer dst.Deallocate()
			defer atlas.Deallocate()
			defer base.Deallocate()
			defer index.Deallocate()
			worst := 0
			for step := 0; step < 12; step++ {
				runOnDrawFrame(func(_ *ebiten.Image) {
					op := &ebiten.DrawTrianglesShaderOptions{}
					op.Images[0], op.Images[1], op.Images[2] = base, atlas, index
					shift := float32(step) * 0.17
					op.Uniforms = map[string]interface{}{
						"CamPos": []float32{512 + shift, 512 + shift}, "DirCos": float32(math.Cos(angle)), "DirSin": float32(math.Sin(angle)),
						"PlaneCos": float32(-math.Sin(angle)), "PlaneSin": float32(math.Cos(angle)), "ScreenSize": []float32{w, 720},
						"Horizon": float32(0), "RowDistFactor": float32(23040), "TileSize": float32(64), "WorldSize": []float32{32, 32},
						"ViewDist": float32(99999), "MinBrightness": float32(1), "Ambient": float32(1), "ViewerAmbient": float32(1),
						"TexCount": float32(1), "TexTileSize": []float32{64, 64}, "MaxMip": float32(6), "LightCount": float32(0), "Lights": make([]float32, 128),
					}
					vertices := []ebiten.Vertex{{DstX: 0, DstY: 0}, {DstX: w, DstY: 0}, {DstX: 0, DstY: h}, {DstX: w, DstY: h}}
					dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, shader, op)
					pixels := snapshotUIImage(dst)
					for y := 50; y < 73; y++ {
						for x := 0; x < w; x += 7 {
							worst = max(worst, abs(int(pixels.RGBAAt(x, y).R)-128))
						}
					}
				})
			}
			t.Logf("worst minified stripe deviation from its average: %d", worst)
			if worst > 35 {
				t.Fatalf("minified floor aliases into broad bands: max deviation=%d", worst)
			}
		})
	}
}
