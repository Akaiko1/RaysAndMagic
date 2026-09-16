//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
	"ugataima/internal/world"
)

// Drive both production builders across the managed/unmanaged boundary. Pixel
// contents, mip dimensions and source ownership must not depend on allocation.
func TestStandeeTextureStorageGPUParity(t *testing.T) {
	requireStandeeGPU(t)
	for _, size := range []image.Point{{16, 16}, {256, 256}, {512, 128}, {257, 255}} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/streaming=%v", size, streaming), func(t *testing.T) {
				cpu := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
				for y := 0; y < size.Y; y++ {
					for x := 0; x < size.X; x++ {
						cpu.SetRGBA(x, y, color.RGBA{uint8(x % 101), uint8(y % 127), 60, 180})
					}
				}
				prepared := prepareStandeePixels(cpu, 0.35, false)
				runOnDrawFrame(func(_ *ebiten.Image) {
					// A non-zero sheet origin exercises the owned normalized copy.
					sheet := ebiten.NewImage(size.X*2, size.Y)
					defer sheet.Deallocate()
					source := sheet.SubImage(image.Rect(size.X, 0, size.X*2, size.Y)).(*ebiten.Image)
					source.WritePixels(cpu.Pix)
					r := &Renderer{}
					key := makeStandeeCoreKey("storage-test", source, false)
					if streaming {
						commit := newMapRenderStandeeCommit(mapRenderPreparedStandee{key: key, source: source, prepared: prepared})
						for {
							_, _, done := commit.advance(r, 4096)
							if done {
								break
							}
							if r.standeeCoreCache[key] != nil {
								t.Error("partial texture published")
							}
						}
					} else {
						r.commitPreparedStandeePixels(key, source, prepared)
					}
					owned := make(map[*ebiten.Image]bool)
					for _, layer := range []standeeMipLayer{standeeMipSticker, standeeMipCore} {
						chain := r.standeeMipCache[standeeMipKey{frame: key, layer: layer}]
						want := prepared.stickerMips
						if layer == standeeMipCore {
							want = prepared.coreMips
						}
						if chain == nil || len(chain.levels) != len(want) {
							t.Error("mip levels missing")
							continue
						}
						for i, level := range chain.levels {
							pixels := make([]byte, len(want[i].Pix))
							level.ReadPixels(pixels)
							if level.Bounds() != want[i].Bounds() || !bytes.Equal(pixels, want[i].Pix) {
								t.Errorf("layer %d mip %d changed", layer, i)
							}
							owned[level] = true
						}
					}
					for img := range owned {
						img.Deallocate()
					}
					if !bytes.Equal(func() []byte { p := make([]byte, len(cpu.Pix)); source.ReadPixels(p); return p }(), cpu.Pix) {
						t.Error("derived release changed borrowed source")
					}
				})
			})
		}
	}
}

func TestFloorMipFastPathGPUParity(t *testing.T) {
	requireStandeeGPU(t)
	prefix, _, ok := strings.Cut(floorShaderSrc, "func Fragment(")
	if !ok {
		t.Fatal("missing production floor sampler")
	}
	fragment := `
var TestMip float
var Reference float
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
 p := (dstPos.xy-imageDstOrigin())/17.0
 if Reference > 0.0 {
  k0 := floor(TestMip)
  k1 := min(k0+1.0, MaxMip)
  return mix(sampleFloorMip(0.0,p.x,p.y,k0,2.0),sampleFloorMip(0.0,p.x,p.y,k1,2.0),fract(TestMip))
 }
 return sampleFloorTrilinear(0.0,p.x,p.y,TestMip,2.0)
}
`
	var shader *ebiten.Shader
	runOnDrawFrame(func(_ *ebiten.Image) {
		var err error
		shader, err = ebiten.NewShader([]byte(prefix + fragment))
		if err != nil {
			t.Error(err)
		}
	})
	if shader == nil {
		return
	}
	defer shader.Deallocate()
	for _, size := range []image.Point{{16, 16}, {32, 16}} {
		cpu := image.NewRGBA(image.Rect(0, 0, size.X, size.Y*2))
		for y := 0; y < cpu.Rect.Dy(); y++ {
			for x := 0; x < cpu.Rect.Dx(); x++ {
				cpu.SetRGBA(x, y, color.RGBA{R: uint8((x*37 + y*17) % 256), G: uint8((x*13 + y*71) % 256), B: 100, A: 255})
			}
		}
		var atlas, base, target *ebiten.Image
		runOnDrawFrame(func(_ *ebiten.Image) {
			atlas = ebiten.NewImageFromImage(cpu)
			base = ebiten.NewImage(4, 4)
			target = ebiten.NewImage(32, 32)
		})
		for _, mip := range []float32{0, 0.25, 1, 1.75, 3, 4} {
			t.Run(fmt.Sprintf("%v/mip%g", size, mip), func(t *testing.T) {
				render := func(reference float32) []byte {
					pixels := make([]byte, 32*32*4)
					runOnDrawFrame(func(_ *ebiten.Image) {
						opts := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{"TexTileSize": []float32{float32(size.X), float32(size.Y)}, "MaxMip": float32(4), "TestMip": mip, "Reference": reference}}
						opts.Images[0], opts.Images[1] = base, atlas
						target.Clear()
						target.DrawTrianglesShader([]ebiten.Vertex{{DstX: 0, DstY: 0}, {DstX: 32, DstY: 0}, {DstX: 0, DstY: 32}, {DstX: 32, DstY: 32}}, []uint16{0, 1, 2, 1, 3, 2}, shader, opts)
						target.ReadPixels(pixels)
					})
					return pixels
				}
				if !bytes.Equal(render(0), render(1)) {
					t.Fatal("floor sampling changed")
				}
			})
		}
		runOnDrawFrame(func(_ *ebiten.Image) { atlas.Deallocate(); base.Deallocate(); target.Deallocate() })
	}
}

func TestCrossedFrameGeometryGPUParity(t *testing.T) {
	requireStandeeGPU(t)
	g, tile := summonTileWorld(t)
	t.Chdir("../..")
	g.sprites = graphics.NewSpriteManager()
	g.renderHelper = NewRenderingHelper(g)
	r := &Renderer{game: g, ambientLight: 1}
	tree, _ := world.GlobalTileManager.GetTileTypeFromKey("tree")
	for _, width := range []int{640, 1280} {
		g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = width, width*9/16
		g.camera.FOV = squareProjectionFOV(width, width*9/16)
		g.depthBuffer = make([]float64, width)
		g.wallTopBuffer = make([]int, width)
		g.camera.ViewDist = 50 * tile
		for i := range g.depthBuffer {
			g.depthBuffer[i] = g.camera.ViewDist
		}
		var target *ebiten.Image
		runOnDrawFrame(func(_ *ebiten.Image) { target = ebiten.NewImage(width, width*9/16) })
		for _, distance := range []float64{1.5, 10, 30} {
			for _, angle := range []float64{0, 0.4} {
				g.camera.Angle = angle
				g.camera.X, g.camera.Y = tile/2-distance*tile*math.Cos(angle), tile/2-distance*tile*math.Sin(angle)
				td := TransparentSpriteData{worldX: tile / 2, worldY: tile / 2, tileType: tree, spriteName: "forest_oak"}
				s, ok := r.crossedTreeRenderData(&td, g.camera.X, g.camera.Y, math.Cos(angle), math.Sin(angle), g.camera.ViewDist*g.camera.ViewDist)
				if !ok {
					t.Fatal("fixture invisible")
				}
				parts := r.splitCrossedTreesForPainterOrder([]UnifiedSpriteRenderData{s}, 0, 1)
				sort.Slice(parts, func(i, j int) bool { return compareUnifiedSprites(parts[i], parts[j]) < 0 })
				render := func(cached bool) []byte {
					pixels := make([]byte, width*(width*9/16)*4)
					runOnDrawFrame(func(_ *ebiten.Image) {
						target.Clear()
						if cached {
							r.crossedGeometry.begin()
						}
						for _, part := range parts {
							r.drawCrossedTreeStandees(target, part)
						}
						if cached {
							r.crossedGeometry.end()
						}
						target.ReadPixels(pixels)
					})
					return pixels
				}
				a, b := render(false), render(true)
				if !bytes.Equal(a, b) {
					t.Fatalf("cached cross changed: width=%d distance=%g angle=%g", width, distance, angle)
				}
				for _, entry := range r.crossedGeometry.slabs {
					for _, sf := range entry.slab.surfaces[:cap(entry.slab.surfaces)] {
						if sf.img != nil || sf.mipKey.frame.img != nil {
							t.Fatal("frame cache retained GPU image")
						}
					}
				}
			}
		}
		runOnDrawFrame(func(_ *ebiten.Image) { target.Deallocate() })
	}
}

func TestRenderTimingPercentiles(t *testing.T) {
	for _, tc := range []struct {
		values []float64
		want   [4]float64
	}{{nil, [4]float64{}}, {[]float64{5}, [4]float64{5, 5, 5, 5}}, {[]float64{100, 1, 1, 1, 1, 1, 1, 1, 1, 1}, [4]float64{1, 100, 100, 100}}} {
		a, b, c, d := renderTimingPercentiles(tc.values)
		if [4]float64{a, b, c, d} != tc.want {
			t.Fatal("cold outlier lost")
		}
	}
}
