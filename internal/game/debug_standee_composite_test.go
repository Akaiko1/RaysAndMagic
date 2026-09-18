//go:build debug

package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared rule: compositing a shell stack must preserve its cutout, shade and
// wall clipping while submitting only one column mesh. Persistence is N/A:
// the mesh is rebuilt for the current camera on every draw.
func TestStandeeSmallStackComposite(t *testing.T) {
	requireStandeeGPU(t)
	g, _, tile := tbBehaviorGame(t, 40, 40)
	const w, h = 800, 600
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = w, h
	g.camera.FOV = squareProjectionFOV(w, h)
	g.camera.ViewDist = 10000
	g.renderHelper = NewRenderingHelper(g)
	g.depthBuffer = make([]float64, w)
	g.wallTopBuffer = make([]int, w)
	r := &Renderer{game: g}
	cpu := image.NewRGBA(image.Rect(0, 0, 128, 256))
	for y := 16; y < 240; y++ {
		for x := 12; x < 116; x++ {
			if x > 45 && x < 78 && y > 60 && y < 190 {
				continue
			}
			a := uint8(255)
			if x < 16 || x > 111 {
				a = 128
			}
			cpu.SetRGBA(x, y, color.RGBA{R: a / 2, G: a / 3, B: a / 4, A: a})
		}
	}
	var sprite, target *ebiten.Image
	runOnDrawFrame(func(_ *ebiten.Image) { sprite = ebiten.NewImageFromImage(cpu); target = ebiten.NewImage(w, h) })
	defer runOnDrawFrame(func(_ *ebiten.Image) { sprite.Deallocate(); target.Deallocate() })
	key := makeStandeeCoreKey("small-stack", sprite, true)
	for _, shells := range []int{2, 3, 5, 6, 16} {
		for _, mirrored := range []bool{false, true} {
			for _, state := range []string{"clear", "wall", "side-fade", "fade"} {
				t.Run(fmt.Sprintf("shells%d/mirror%v/%s", shells, mirrored, state), func(t *testing.T) {
					depth := 4 * tile
					g.camera.X, g.camera.Y, g.camera.Angle = -depth, 0, 0
					// Choose physical thickness through the production shell-count rule.
					g.config.Graphics.Standee.ThicknessTiles = (float64(shells) - 0.5) * standeeShellSpacingPx * 2 * math.Tan(g.camera.FOV/2) * depth / (tile * w)
					for x := range g.depthBuffer {
						g.depthBuffer[x] = g.camera.ViewDist
						g.wallTopBuffer[x] = 0
						if state == "wall" && x > w/3 && x < 2*w/3 {
							g.depthBuffer[x] = depth / 2
							g.wallTopBuffer[x] = h / 2
						}
					}
					slab, ok := r.prepareStandeeSlab(sprite, key, 0, 0, math.Pi/4, depth, 300, 450, 1, 1, 1, false, mirrored, 2*tile, nil)
					if !ok || len(slab.surfaces) != shells+2 {
						t.Fatalf("bad production slab: ok=%v surfaces=%d", ok, len(slab.surfaces))
					}
					// Exercise the full stack even if this fixture's physical parallax would
					// otherwise fade it; the fade rows separately cover that fallback.
					slab.sideFade = 0
					slab.firstSurface = 0
					if state == "side-fade" {
						slab.sideFade = 0.5
					}
					if state == "fade" {
						slab.fade = 0.5
					}
					draw := func(volume bool) ([]byte, int) {
						p := make([]byte, 4*w*h)
						runOnDrawFrame(func(_ *ebiten.Image) {
							target.Clear()
							r.statStandeeVertices = 0
							slab.volumeComposite = volume
							r.drawStandeeSlabColumns(target, slab, -1, -1)
							target.ReadPixels(p)
						})
						return p, r.statStandeeVertices
					}
					fast, nFast := draw(true)
					ref, nRef := draw(false)
					if nFast == 0 || nRef == 0 {
						t.Fatal("fixture rendered no geometry")
					}
					if state == "clear" || state == "wall" {
						if nFast >= nRef/2 {
							t.Fatalf("production failed to use one mesh: composite=%d material=%d", nFast, nRef)
						}
					} else if nFast != nRef {
						t.Fatal("fading stack bypassed material path")
					}
					sum, union, shared := 0, 0, 0
					for i := 0; i < len(fast); i += 4 {
						a, b := fast[i+3] > 8, ref[i+3] > 8
						if a || b {
							union++
							for c := 0; c < 4; c++ {
								sum += absInt(int(fast[i+c]) - int(ref[i+c]))
							}
						}
						if a && b {
							shared++
						}
					}
					if union == 0 || float64(shared)/float64(union) < 0.97 || float64(sum)/float64(4*union) > 3 {
						t.Fatalf("compositing changed silhouette/shade: shared=%d union=%d delta=%d", shared, union, sum)
					}
					// The central opening is deliberately wide enough to stay clear through
					// every shell. A bounding-box occluder must never fill this hole.
					if fast[(300*w+400)*4+3] != 0 {
						t.Fatal("transparent opening filled")
					}
				})
			}
		}
	}
}
