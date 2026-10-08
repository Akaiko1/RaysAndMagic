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

// Wide spans must preserve the exact column path's perspective, alpha and wall
// clipping. Drawing one-column clips selects the production fallback without a
// second rasterizer or a test-only switch in the renderer.
func TestStandeeSpansPreserveColumnRendering(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires the live GPU harness")
	}
	cfg := loadTestConfig(t)
	const w, h = 640, 480
	cfg.Display.ScreenWidth, cfg.Display.ScreenHeight = w, h
	g := newTestGame(cfg, newTestWorld(cfg))
	g.camera.X, g.camera.Y, g.camera.Angle = 0, 0, 0
	g.camera.FOV, g.camera.ViewDist = squareProjectionFOV(w, h), 3200
	g.renderHelper = NewRenderingHelper(g)
	g.depthBuffer, g.wallTopBuffer = make([]float64, w), make([]float64, w)
	r := &Renderer{game: g}

	// Non-zero source bounds exercise animated-sheet normalization. Asymmetric
	// colours and transparent holes expose UV distortion, mirroring and alpha.
	pixels := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if (x/13+y/19)%4 == 0 {
				continue
			}
			pixels.SetRGBA(x, y, color.RGBA{uint8(40 + x), uint8(30 + y), 70, 190})
		}
	}
	sheet := ebiten.NewImage(256, 256)
	defer sheet.Deallocate()
	sprite := sheet.SubImage(image.Rect(32, 48, 160, 176)).(*ebiten.Image)
	sprite.WritePixels(pixels.Pix)
	key := makeStandeeCoreKey("span-parity", sprite, false)
	r.standeeCoreSilhouetteFromCPU(key, sprite, pixels)
	spanImage, columnImage := ebiten.NewImage(w, h), ebiten.NewImage(w, h)
	defer spanImage.Deallocate()
	defer columnImage.Deallocate()
	spanPixels, columnPixels := make([]byte, 4*w*h), make([]byte, 4*w*h)

	type spanCase struct {
		name                   string
		depth, size, yaw       float64
		fade                   float32
		mirror, single, hidden bool
		walls, mounted         bool
		slope                  float64
		volume, brokenWall     bool
		panini                 bool
	}
	cases := []spanCase{
		{name: "angled transparent stack", depth: 100, size: 350, yaw: 0.8},
		{name: "mirrored fading stack", depth: 100, size: 350, yaw: 2.1, mirror: true, fade: 0.4},
		{name: "minified single face", depth: 600, size: 45, yaw: 1.2, single: true},
		{name: "stepped foreground walls", depth: 100, size: 350, yaw: 0.8, walls: true},
		{name: "backing wall and foreground wall", depth: 100, size: 350, yaw: math.Pi / 2, walls: true, mounted: true},
		{name: "fully hidden", depth: 100, size: 350, yaw: 0.8, hidden: true},
		{name: "crossing viewport edges", depth: 30, size: 1100, yaw: 0.8},
		{name: "near plane crossing", depth: 4.5, size: 2000, yaw: 0.6},
		{name: "grazing face", depth: 100, size: 350, yaw: 0.001},
	}
	for _, volume := range []bool{false, true} {
		for _, slope := range []float64{0.03, 0.12, 0.5, -0.5} {
			cases = append(cases, spanCase{name: fmt.Sprintf("sloped wall %g volume=%v", slope, volume), depth: 100, size: 350, yaw: 0.8, slope: slope, volume: volume})
		}
		cases = append(cases, spanCase{name: fmt.Sprintf("slopes with corners and gaps volume=%v", volume), depth: 100, size: 350, yaw: 0.8, slope: 0.5, volume: volume, brokenWall: true})
	}
	for _, volume := range []bool{false, true} {
		cases = append(cases, spanCase{name: fmt.Sprintf("Panini sloped wall volume=%v", volume), depth: 100, size: 350, yaw: 0.8, slope: 0.5, volume: volume, panini: true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g.camera.FOV = squareProjectionFOV(w, h)
			g.combatPreferences.WideView = tc.panini
			if tc.panini {
				g.camera.FOV = 108 * math.Pi / 180
			}
			bottom := float64(h)/2 + tc.size/2
			slab, ok := r.prepareStandeeSlab(sprite, key, tc.depth, 0, tc.yaw, tc.depth, tc.size, bottom, 1, 0.7, 0.4, false, tc.mirror, 0, nil)
			if !ok {
				t.Fatal("fixture did not project")
			}
			slab.fade = tc.fade
			slab.volumeComposite = tc.volume
			if tc.volume && !canUseStandeeVolume(slab) {
				t.Fatal("fixture did not engage the volume compositor")
			}
			if tc.single {
				slab.firstSurface = len(slab.surfaces) - 1
			}
			r.standeeWallOcclusion = standeeWallOcclusion{}
			if tc.mounted {
				r.standeeWallOcclusion = standeeWallOcclusion{hasBackingWall: true, backingX: tc.depth - 2, backingYaw: tc.yaw}
			}
			for x := range g.depthBuffer {
				g.depthBuffer[x], g.wallTopBuffer[x] = g.camera.ViewDist, 0
				if tc.walls {
					g.depthBuffer[x] = tc.depth - 2
					g.wallTopBuffer[x] = float64(h/3+(x/31)%4*30) + 0.375
					if x > w/2 && x < 3*w/4 {
						g.depthBuffer[x] = tc.depth / 2
					}
				}
				if tc.slope != 0 {
					offset := float64(x-w/2) + 0.5
					// Both inputs come from one planar wall: reciprocal depth and
					// its projected top vary linearly, not depth itself.
					inv := 1/(tc.depth/2) + offset*tc.slope*1e-5
					g.depthBuffer[x] = 1 / inv
					g.wallTopBuffer[x] = float64(h)/2 + 0.375 + offset*tc.slope
					if tc.brokenWall {
						switch {
						case x%79 == 0: // One isolated, fully hiding column.
							g.wallTopBuffer[x] = 0
						case x%79 < 13: // A gap in the wall.
							g.depthBuffer[x] = g.camera.ViewDist
						case x%79 < 40: // Opposite slope meeting a step.
							g.wallTopBuffer[x] = float64(h)/2 - offset*tc.slope
						}
					}
				}
				if tc.hidden {
					g.depthBuffer[x] = tc.depth / 10
				}
			}
			var spanVertices, columnVertices int
			runOnDrawFrame(func(_ *ebiten.Image) {
				previous := activeWorldRaster
				defer func() { activeWorldRaster = previous }()
				var raster paniniRasterizer
				if tc.panini {
					raster.configure(spanImage, g.widePaniniProjection())
					activeWorldRaster = &raster
				}
				spanImage.Clear()
				r.statStandeeVertices = 0
				r.drawStandeeSlabColumns(spanImage, slab, -1, -1)
				spanVertices = r.statStandeeVertices
				if tc.panini {
					raster.configure(columnImage, g.widePaniniProjection())
				}
				columnImage.Clear()
				r.statStandeeVertices = 0
				for x := slab.minX; x <= slab.maxX; x++ {
					r.drawStandeeSlabColumns(columnImage, slab, x, x)
				}
				columnVertices = r.statStandeeVertices
				spanImage.ReadPixels(spanPixels)
				columnImage.ReadPixels(columnPixels)
			})
			var covered, coverageMismatch, delta int
			for i := 0; i < len(spanPixels); i += 4 {
				a, b := spanPixels[i+3] > 8, columnPixels[i+3] > 8
				if a || b {
					covered++
					if a != b {
						coverageMismatch++
					}
					for c := 0; c < 4; c++ {
						delta += abs(int(spanPixels[i+c]) - int(columnPixels[i+c]))
					}
				}
			}
			if tc.hidden {
				if covered != 0 || spanVertices != 0 {
					t.Fatalf("hidden surface rendered: pixels=%d vertices=%d", covered, spanVertices)
				}
				return
			}
			if covered == 0 {
				t.Fatal("fixture drew nothing")
			}
			mismatch := float64(coverageMismatch) / float64(covered)
			meanDelta := float64(delta) / float64(4*covered)
			t.Logf("coverage mismatch %.5f; mean RGBA delta %.3f/255; vertices %d -> %d", mismatch, meanDelta, columnVertices, spanVertices)
			coverageLimit, colorLimit := 0.002, 0.5
			if tc.panini {
				// The column fallback holds U constant across each source column;
				// spans sample continuously. Panini magnifies that subpixel
				// reference error, so permit one colour level in the mean.
				coverageLimit, colorLimit = 0.007, 1
			}
			if mismatch > coverageLimit || meanDelta > colorLimit {
				t.Fatalf("span rendering changed coverage or sampling: mismatch %.5f delta %.3f", mismatch, meanDelta)
			}
			if !tc.walls && !tc.brokenWall && tc.depth >= 100 && tc.yaw > 0.01 && spanVertices > columnVertices/4 {
				t.Fatalf("affine geometry was not reduced: %d -> %d vertices", columnVertices, spanVertices)
			}
		})
	}
	t.Run("packed flags tolerate interpolation rounding", func(t *testing.T) {
		clear(g.depthBuffer)
		for i := range g.depthBuffer {
			g.depthBuffer[i] = math.Inf(1)
		}
		r.standeeWallOcclusion = standeeWallOcclusion{}
		opts := &ebiten.DrawTrianglesShaderOptions{}
		for i, c := range []color.RGBA{{255, 255, 0, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}} {
			opts.Images[i] = ebiten.NewImage(8, 8)
			opts.Images[i].Fill(c)
			defer opts.Images[i].Deallocate()
		}
		shader, err := r.ensureStandeeTrilinearShader()
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			layer, filtered, blend float32
			want                   color.RGBA
		}{
			{0, 0, 0, color.RGBA{255, 255, 0, 255}},
			{0, 1, 0, color.RGBA{255, 0, 0, 255}},
			{0, 1, 1, color.RGBA{0, 0, 255, 255}},
			{1, 0, 0, color.RGBA{0, 255, 0, 255}},
			{2, 0, 0, color.RGBA{255, 0, 0, 255}},
			{2, 1, 0.5, color.RGBA{128, 0, 128, 255}},
			{2, 1, 1, color.RGBA{0, 0, 255, 255}},
		} {
			for _, direction := range []float32{float32(math.Inf(-1)), float32(math.Inf(1))} {
				material := ebiten.Vertex{ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1, Custom0: tc.blend, Custom1: tc.layer, Custom2: tc.filtered}
				slab := standeeSlab{centerDepth: 100, centerSize: 300, bottomY: 400}
				surface := standeeSurface{p0x: 100, p0y: -100, dy: 200}
				vertices, indices, ok := r.appendStandeeSurfaceSpans(nil, nil, slab, surface, 200, 440, material, image.Point{})
				if !ok || len(vertices) == 0 {
					t.Fatal("flag fixture did not project")
				}
				for i := range vertices {
					vertices[i].Custom2 = math.Nextafter32(vertices[i].Custom2, direction)
				}
				runOnDrawFrame(func(_ *ebiten.Image) {
					spanImage.Clear()
					spanImage.DrawTrianglesShader32(vertices, indices, shader, opts)
					got := color.RGBAModel.Convert(spanImage.At(320, 250)).(color.RGBA)
					if abs(int(got.R)-int(tc.want.R)) > 1 || abs(int(got.G)-int(tc.want.G)) > 1 || abs(int(got.B)-int(tc.want.B)) > 1 || got.A != tc.want.A {
						t.Errorf("layer=%g filtered=%g blend=%g rounding=%g: got %v want %v", tc.layer, tc.filtered, tc.blend, direction, got, tc.want)
					}
				})
			}
		}
	})

}
