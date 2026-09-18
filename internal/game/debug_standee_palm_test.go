//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Palm diagnostic uses authored sprites and the production crossed-slab draw.
// It isolates surface geometry from scene occlusion; full-game captures remain
// necessary to verify a reported scene. No images are resized for inspection.
func TestStandeePalmGrazing(t *testing.T) {
	requireStandeeGPU(t)
	g, _, tile := tbBehaviorGame(t, 40, 40)
	const w, h = 1024, 768
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = w, h
	g.camera.FOV = squareProjectionFOV(w, h)
	g.camera.ViewDist = 10000
	g.renderHelper = NewRenderingHelper(g)
	r := &Renderer{game: g}
	out := os.Getenv("RAM_PALM_PREVIEW")
	if out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, art := range []string{"palm", "three_intertwined_palms", "tropical_vine_tree", "cutout_fixture"} {
		var cpu image.Image
		if art == "cutout_fixture" {
			im := image.NewRGBA(image.Rect(0, 0, 512, 1024))
			draw.Draw(im, image.Rect(0, 256, 512, 768), image.NewUniform(color.RGBA{100, 190, 30, 255}), image.Point{}, draw.Src)
			cpu = im
		} else {
			f, err := os.Open("../../assets/sprites/environment/nature/" + art + ".png")
			if err != nil {
				t.Fatal(err)
			}
			cpu, err = png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		var src, dst *ebiten.Image
		runOnDrawFrame(func(_ *ebiten.Image) { src = ebiten.NewImageFromImage(cpu); dst = ebiten.NewImage(w, h) })
		key := makeStandeeCoreKey("palm-regression:"+art, src, true)
		for _, distance := range []float64{3, 10, 20} {
			for _, angle := range []float64{-0.02, 0, 0.0002, 0.002, 0.02, 0.2} {
				for _, volume := range []bool{false, true} {
					yaw := math.Pi / 4
					g.camera.Angle = yaw + angle
					g.camera.X, g.camera.Y = -distance*tile*math.Cos(g.camera.Angle), -distance*tile*math.Sin(g.camera.Angle)
					// A half-pixel offset makes one grazing surface cross a pixel center.
					g.camera.Y += 0.37
					_, depth, ok := g.renderHelper.projectToScreenXF(0, 0)
					if !ok {
						t.Fatal("bad camera")
					}
					width := 2 * tile * h / depth
					height := width * float64(cpu.Bounds().Dy()) / float64(cpu.Bounds().Dx())
					bottom := g.renderHelper.calculateFloorScreenYF(depth)
					pixels := make([]byte, w*h*4)
					runOnDrawFrame(func(_ *ebiten.Image) {
						dst.Clear()
						r.drawCrossedSlabs(dst, src, key, 0, 0, yaw, yaw+math.Pi/2, 2*tile, depth, height, bottom, 1, volume)
						dst.ReadPixels(pixels)
					})
					if art == "cutout_fixture" {
						var surfaces []standeeSurface
						for _, armYaw := range []float64{yaw, yaw + math.Pi/2} {
							slab, ok := r.prepareStandeeSlab(src, key, 0, 0, armYaw, depth, height, bottom, 1, 1, 1, true, false, 2*tile, nil)
							if ok {
								surfaces = append(surfaces, slab.surfaces...)
							}
						}
						basis := r.cameraBasis()
						for x := 0; x < w; x++ {
							rx, ry := standeeRayAtScreenX(float64(x)+0.5, w, basis.dirX, basis.dirY, basis.planeX, basis.planeY)
							minTop, maxBottom := math.Inf(1), math.Inf(-1)
							for _, sf := range surfaces {
								d, _, hit := standeeColumnHit(g.camera.X, g.camera.Y, rx, ry, sf.p0x, sf.p0y, sf.dx, sf.dy)
								if !hit {
									continue
								}
								hh := height * depth / d
								bb := h/2 + (bottom-h/2)*depth/d
								minTop = math.Min(minTop, bb-0.75*hh)
								maxBottom = math.Max(maxBottom, bb-0.25*hh)
							}
							for y := 0; y < h; y++ {
								if pixels[(y*w+x)*4+3] > 64 && (float64(y) < minTop-3 || float64(y) > maxBottom+3) {
									t.Fatalf("grazing filter stretched the cutout: distance=%g angle=%g volume=%v pixel=(%d,%d) physical alpha span=[%.2f,%.2f]", distance, angle, volume, x, y, minTop, maxBottom)
								}
							}
						}
					}
					minY, maxY := h, 0
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							if pixels[(y*w+x)*4+3] > 32 {
								minY = min(minY, y)
								maxY = max(maxY, y)
							}
						}
					}
					t.Logf("%s distance=%g angle=%g volume=%v top=%d expectedFrontTop=%.1f bottom=%d", art, distance, angle, volume, minY, bottom-height, maxY)
					if out != "" && distance == 10 {
						name := fmt.Sprintf("%s_%g_%v.png", art, angle, volume)
						f, err := os.Create(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(f, &image.RGBA{Pix: pixels, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)})
						f.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
		src.Deallocate()
		dst.Deallocate()
	}
}
