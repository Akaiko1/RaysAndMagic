package game

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// Animated person NPCs use four distinct square cells through the existing
// renderer at the configured TPS. Persistence is N/A: sheets are static assets.
func TestDesertNPCIdleContent(t *testing.T) {
	t.Chdir("../..")
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	npc, err := character.CreateNPCFromConfig("nomad_city_safiya", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{npcSpriteName(npc), "nomad_caravan_merchant", "desert_pilgrim"} {
		t.Run(name, func(t *testing.T) {
			path, ok := graphics.ResolveSpritePath(name)
			if !ok {
				t.Fatalf("missing NPC sprite %s", name)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			cpu, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			h := cpu.Bounds().Dy()
			if h != 512 || cpu.Bounds().Dx() != h*SpriteSheetFrameCount {
				t.Fatalf("NPC idle needs four 512x512 cells, got %v", cpu.Bounds())
			}
			seen := map[[32]byte]bool{}
			var planted [2][2]float64
			for i := 0; i < SpriteSheetFrameCount; i++ {
				cell := image.NewRGBA(image.Rect(0, 0, h, h))
				draw.Draw(cell, cell.Bounds(), cpu, image.Pt(i*h, 0), draw.Src)
				hash := sha256.Sum256(cell.Pix)
				if seen[hash] {
					t.Fatalf("idle frame %d duplicates another pose", i)
				}
				seen[hash] = true
				if name == npcSpriteName(npc) {
					for foot := 0; foot < 2; foot++ {
						x0, x1 := foot*h/2, (foot+1)*h/2
						bottom := -1
						for y := 9 * h / 10; y < h; y++ {
							for x := x0; x < x1; x++ {
								if cell.RGBAAt(x, y).A > 127 {
									bottom = y
								}
							}
						}
						if bottom < 0 {
							t.Fatalf("frame %d foot %d missing ground contact", i, foot)
						}
						left, right := h, -1
						for y := bottom - 1; y <= bottom; y++ {
							for x := x0; x < x1; x++ {
								if cell.RGBAAt(x, y).A > 127 {
									left, right = min(left, x), max(right, x)
								}
							}
						}
						contact := [2]float64{float64(left+right) / 2, float64(bottom)}
						if i == 0 {
							planted[foot] = contact
						} else if math.Abs(contact[0]-planted[foot][0]) > 1 || contact[1] != planted[foot][1] {
							t.Fatalf("frame %d foot %d slides: contact %v, neutral %v", i, foot, contact, planted[foot])
						}
					}
				}
			}
			sheet := graphics.NewSpriteManager().GetSprite(name)
			for _, tps := range []int{60, 120} {
				t.Run(fmt.Sprintf("TPS%d", tps), func(t *testing.T) {
					r := &Renderer{game: &MMGame{config: &config.Config{Engine: config.EngineConfig{TPS: tps}}}}
					for step := 0; step <= SpriteSheetFrameCount; step++ {
						frame, w, fh := r.selectNPCIdleSpriteFrame(sheet, int64(step*tps/NPCIdleAnimationFPS))
						want := image.Rect((step%SpriteSheetFrameCount)*h, 0, (step%SpriteSheetFrameCount+1)*h, h)
						if w != h || fh != h || frame.Bounds() != want {
							t.Fatalf("step %d: frame %v size %dx%d, want %v", step, frame.Bounds(), w, fh, want)
						}
					}
				})
			}
		})
	}
}
