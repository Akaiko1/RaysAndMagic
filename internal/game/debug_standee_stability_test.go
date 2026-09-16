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
)

// Exercise the production crossed-slab draw, including shader compilation and
// pixel readback. Geometry and textures are real; scene lighting and movement
// jitter are excluded to isolate changes caused by the standee renderer.
func requireStandeeGPU(t *testing.T) {
	t.Helper()
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires the live GPU harness")
	}
	// Desktop focus changes must not suspend a diagnostic waiting for Draw.
	ebiten.SetRunnableOnUnfocused(true)
}

func TestStandeeApproachStability(t *testing.T) {
	requireStandeeGPU(t)
	for _, width := range []int{1024, 1920} {
		for _, art := range []string{"sakura_tree_large", "forest_oak"} {
			for _, angle := range []float64{0, math.Pi / 6} {
				t.Run(fmt.Sprintf("%d/%s/angle%.0f", width, art, angle*180/math.Pi), func(t *testing.T) {
					g, _, tile := tbBehaviorGame(t, 40, 40)
					height := width * 9 / 16
					g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = width, height
					g.camera.FOV = 2 * math.Atan(float64(width)/(2*float64(height)))
					g.renderHelper = NewRenderingHelper(g)
					r := &Renderer{game: g}
					f, err := os.Open("../../assets/sprites/environment/nature/" + art + ".png")
					if err != nil {
						t.Fatal(err)
					}
					cpu, err := png.Decode(f)
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
					var sprite, target *ebiten.Image
					var key standeeCoreKey
					runOnDrawFrame(func(_ *ebiten.Image) {
						sprite = ebiten.NewImageFromImage(cpu)
						target = ebiten.NewImage(width, height)
						key = makeStandeeCoreKey("stability:"+art, sprite, true)
						r.standeeCoreSilhouette(key, sprite)
					})
					prepare := func(distance float64) standeeSlab {
						g.camera.X, g.camera.Y = -distance*tile*math.Cos(angle), -distance*tile*math.Sin(angle)
						g.camera.Angle = angle
						size := float64(height) * 2 / distance
						slab, ok := r.prepareStandeeSlab(sprite, key, 0, 0, math.Pi/4,
							distance*tile, size, g.renderHelper.calculateFloorScreenYF(distance*tile),
							1, 1, 1, true, false, 2*tile, nil)
						if !ok {
							t.Fatal("tree disappeared during approach")
						}
						return slab
					}
					previous := prepare(15).sideFade
					for i := 1; i <= 1200; i++ {
						slab := prepare(15 - float64(i)*0.01)
						if math.Abs(float64(slab.sideFade-previous)) > 0.025 {
							t.Fatal("production slab opacity jumped during a small camera movement")
						}
						previous = slab.sideFade
					}
					var prev []byte
					maxDelta := 0.0
					for i := 0; i <= 100; i++ {
						distance := 10 - float64(i)*0.01
						slab := prepare(distance)
						pixels := make([]byte, 4*width*height)
						runOnDrawFrame(func(_ *ebiten.Image) {
							target.Clear()
							r.drawCrossedSlabs(target, sprite, key, 0, 0, math.Pi/4, 3*math.Pi/4,
								2*tile, distance*tile, slab.centerSize, slab.bottomY, 1, true)
							target.ReadPixels(pixels)
						})
						if prev != nil {
							sum, count := 0, 0
							for j := 0; j < len(pixels); j += 4 {
								if pixels[j+3] <= 8 && prev[j+3] <= 8 {
									continue
								}
								count++
								for c := 0; c < 4; c++ {
									sum += absInt(int(pixels[j+c]) - int(prev[j+c]))
								}
							}
							if count == 0 {
								t.Fatal("renderer produced no visible pixels")
							}
							maxDelta = math.Max(maxDelta, float64(sum)/float64(count*4))
						}
						prev = pixels
						if dir := os.Getenv("RAM_STANDEE_PREVIEW"); dir != "" && width == 1920 && art == "sakura_tree_large" && angle == 0 {
							if err := os.MkdirAll(dir, 0755); err != nil {
								t.Fatal(err)
							}
							out, err := os.Create(filepath.Join(dir, fmt.Sprintf("%03d.png", i)))
							if err != nil {
								t.Fatal(err)
							}
							img := &image.RGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
							err = png.Encode(out, img.SubImage(image.Rect(790, 270, 1130, 650)))
							out.Close()
							if err != nil {
								t.Fatal(err)
							}
						}
					}
					t.Logf("maximum adjacent-frame RGBA delta %.3f/255", maxDelta)
					// The old pixel-grid layer switch produced jumps above 16/255.
					// Allow normal resampling and perspective motion, reject flashes.
					if maxDelta > 6 {
						t.Fatalf("approach still flashes: mean channel delta %.3f", maxDelta)
					}
				})
			}
		}
	}
}

// Shared standee behavior also covers rectangular art, reversed faces and
// partial wall occlusion. Auto volume selection must match material rendering
// on both sides of the side-fade and shell-count boundaries.
func TestStandeeCompositorStabilityParity(t *testing.T) {
	requireStandeeGPU(t)
	for _, size := range []image.Point{{512, 512}, {256, 512}} {
		for _, angle := range []float64{0, math.Pi / 6, math.Pi / 2} {
			for _, clipped := range []bool{false, true} {
				t.Run(fmt.Sprintf("%v/angle%.0f/clipped%v", size, angle*180/math.Pi, clipped), func(t *testing.T) {
					g, _, tile := tbBehaviorGame(t, 40, 40)
					w, h := 640, 480
					g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = w, h
					g.camera.FOV = 2 * math.Atan(float64(w)/(2*float64(h)))
					g.renderHelper = NewRenderingHelper(g)
					r := &Renderer{game: g}
					cpu := image.NewRGBA(image.Rectangle{Max: size})
					for y := 0; y < size.Y; y++ {
						for x := 0; x < size.X; x++ {
							if x%23 < 12 || y%31 < 16 {
								cpu.SetRGBA(x, y, color.RGBA{R: 160, G: 100, B: 60, A: 192})
							}
						}
					}
					var sprite, target *ebiten.Image
					var key standeeCoreKey
					runOnDrawFrame(func(_ *ebiten.Image) {
						sprite, target = ebiten.NewImageFromImage(cpu), ebiten.NewImage(w, h)
						key = makeStandeeCoreKey("parity", sprite, true)
						r.standeeCoreSilhouette(key, sprite)
					})
					g.depthBuffer = make([]float64, w)
					g.wallTopBuffer = make([]int, w)
					for _, distance := range []float64{0.8, 1.2, 2.5} {
						g.camera.X, g.camera.Y = -distance*tile*math.Cos(angle), -distance*tile*math.Sin(angle)
						g.camera.Angle, g.camera.ViewDist = angle, 50*tile
						for x := range g.depthBuffer {
							g.depthBuffer[x] = g.camera.ViewDist
							if clipped && x > w/3 && x < 2*w/3 {
								g.depthBuffer[x], g.wallTopBuffer[x] = distance*tile/2, h/2
							}
						}
						width := float64(h) / distance
						height := standeeHeightForWidth(width, size.X, size.Y)
						draw := func(volume bool) []byte {
							pixels := make([]byte, 4*w*h)
							runOnDrawFrame(func(_ *ebiten.Image) {
								target.Clear()
								r.drawCrossedSlabs(target, sprite, key, 0, 0, math.Pi/4, 3*math.Pi/4,
									tile, distance*tile, height, g.renderHelper.calculateFloorScreenYF(distance*tile), 1, volume)
								target.ReadPixels(pixels)
							})
							return pixels
						}
						a, b := draw(true), draw(false)
						sum, covered := 0, 0
						for i := 0; i < len(a); i += 4 {
							if a[i+3] > 8 || b[i+3] > 8 {
								covered++
								for c := 0; c < 4; c++ {
									sum += absInt(int(a[i+c]) - int(b[i+c]))
								}
							}
						}
						if covered == 0 || float64(sum)/float64(covered*4) > 3 {
							t.Fatalf("volume/material mismatch at %.1f tiles: delta=%d coverage=%d", distance, sum, covered)
						}
					}
				})
			}
		}
	}
}
