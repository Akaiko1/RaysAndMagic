package game

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// Invariant: every destination pixel is covered once, source reads stay inside
// the master, and all four corner ornaments retain the same square scale.
// Cells: three metals x normal/wide/tall/small/odd/tiny/empty destinations.
// Persistence is N/A: plans are transient and borrow the sprite's GPU storage.
func TestInterfaceFrameResizeContract(t *testing.T) {
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	ui.game.validateInterfaceArt()
	screen := ebiten.NewImage(1200, 800)
	defer screen.Deallocate()
	for style, spec := range interfaceFrames {
		for _, size := range [][2]int{{800, 600}, {1000, 24}, {24, 700}, {16, 16}, {37, 23}, {1, 1}, {0, 20}} {
			t.Run(fmt.Sprintf("%s/%dx%d", spec.name, size[0], size[1]), func(t *testing.T) {
				ui.drawThemeFrame(screen, interfaceFrame(style), 7, 9, size[0], size[1])
				if size[0] == 0 {
					return
				}
				var plan *patternPlan
				for _, p := range ui.patternPlans.entries {
					if p.name == spec.name && p.w == size[0] && p.h == size[1] {
						plan = p
						break
					}
				}
				if plan == nil || !plan.ok {
					t.Fatal("production draw did not use a shared frame plan")
				}
				coverage := make([]uint8, size[0]*size[1])
				for _, op := range plan.ops {
					if !op.part.Bounds().In(plan.source.Bounds()) {
						t.Fatal("source cut leaves master")
					}
					if !image.Rect(op.dx, op.dy, op.dx+op.dw, op.dy+op.dh).In(image.Rect(0, 0, size[0], size[1])) {
						t.Fatal("frame paints outside its layout rectangle")
					}
					b := op.part.Bounds()
					if b.Dx() == spec.pattern.slice && b.Dy() == spec.pattern.slice && op.dw != op.dh {
						t.Fatal("corner was stretched")
					}
					for y := op.dy; y < op.dy+op.dh; y++ {
						for x := op.dx; x < op.dx+op.dw; x++ {
							coverage[y*size[0]+x]++
						}
					}
				}
				for _, n := range coverage {
					if n != 1 {
						t.Fatalf("destination coverage = %d, want exactly one", n)
					}
				}
				ui.drawThemeFrame(screen, interfaceFrame(style), 17, 19, size[0], size[1])
				found := false
				for _, p := range ui.patternPlans.entries {
					found = found || p == plan
				}
				if !found {
					t.Fatal("moving a frame rebuilt its plan")
				}
			})
		}
	}
}

func TestFramePlanUsesSubimageOrigin(t *testing.T) {
	source := ebiten.NewImage(600, 600)
	defer source.Deallocate()
	sub := source.SubImage(image.Rect(31, 47, 543, 559)).(*ebiten.Image)
	var cache patternPlanCache
	spec := &interfaceFrames[frameGold]
	plan := cache.get("offset", sub, &spec.pattern, 300, 150, spec.scale)
	if !plan.ok || len(plan.ops) != 9 {
		t.Fatal("missing frame slices")
	}
	for _, op := range plan.ops {
		if !op.part.Bounds().In(sub.Bounds()) {
			t.Fatal("subimage origin was lost")
		}
	}
}

// Hover must change color only, retaining the same texture and source cuts.
func TestButtonHoverKeepsFrameSource(t *testing.T) {
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	screen := ebiten.NewImage(800, 600)
	defer screen.Deallocate()
	for _, size := range [][2]int{{117, 32}, {200, 40}, {500, 52}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			ui.patternPlans = patternPlanCache{}
			ui.drawButtonFrame(screen, 10, 10, size[0], size[1], false)
			if len(ui.patternPlans.entries) != 1 {
				t.Fatal("missing button plan")
			}
			plan := ui.patternPlans.entries[0]
			for _, active := range []bool{true, false, true} {
				ui.drawButtonFrame(screen, 10, 10, size[0], size[1], active)
				if len(ui.patternPlans.entries) != 1 || ui.patternPlans.entries[0] != plan {
					t.Fatal("hover replaced the button artwork or source cuts")
				}
			}
		})
	}
}
