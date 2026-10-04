package game

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// Both feet must stay registered, even when arms and coat hems move. Test the
// imported pixels and the production frame selector at both supported rates.
// Persistence is N/A: this is immutable sprite data.
func TestSolsticeNPCPlantedFeet(t *testing.T) {
	t.Chdir("../..")
	for _, name := range []string{"solstice_velna", "solstice_tovan", "solstice_sera"} {
		t.Run(name, func(t *testing.T) {
			path, ok := graphics.ResolveSpritePath(name)
			if !ok {
				t.Fatal("missing NPC sprite")
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			source, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if source.Bounds().Size() != image.Pt(2048, 512) {
				t.Fatal("incorrect NPC sheet layout")
			}
			frames := make([]*image.RGBA, 4)
			for i := range frames {
				frames[i] = image.NewRGBA(image.Rect(0, 0, 512, 512))
				draw.Draw(frames[i], frames[i].Bounds(), source, image.Pt(i*512, 0), draw.Src)
			}
			for i, frame := range frames[1:] {
				for foot, span := range [][2]int{{140, 255}, {256, 370}} {
					best := math.Inf(1)
					var shift image.Point
					for dy := -2; dy <= 2; dy++ {
						for dx := -8; dx <= 8; dx++ {
							score, count := 0.0, 0
							for y := 455; y < 508; y++ {
								for x := span[0]; x < span[1]; x++ {
									a := frames[0].RGBAAt(x, y)
									if a.A < 240 {
										continue
									}
									b := frame.RGBAAt(x-dx, y-dy)
									for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
										score += float64(d * d)
									}
									count++
								}
							}
							if count < 100 {
								t.Fatal("missing planted foot reference")
							}
							score /= float64(count)
							if score < best {
								best = score
								shift = image.Pt(dx, dy)
							}
						}
					}
					if math.Abs(float64(shift.X)) > 1 || math.Abs(float64(shift.Y)) > 1 {
						t.Fatalf("frame %d foot %d slides by %v", i+1, foot, shift)
					}
				}
			}
			sheet := graphics.NewSpriteManager().GetSprite(name)
			for _, tps := range []int{60, 120} {
				t.Run(fmt.Sprintf("TPS%d", tps), func(t *testing.T) {
					r := &Renderer{game: &MMGame{config: &config.Config{Engine: config.EngineConfig{TPS: tps}}}}
					for step := 0; step <= 4; step++ {
						frame, w, h := r.selectNPCIdleSpriteFrame(sheet, int64(step*tps/NPCIdleAnimationFPS))
						want := image.Rect((step%4)*512, 0, (step%4+1)*512, 512)
						if frame.Bounds() != want || w != 512 || h != 512 {
							t.Fatal("renderer changed registered frame geometry")
						}
					}
				})
			}
		})
	}
}
