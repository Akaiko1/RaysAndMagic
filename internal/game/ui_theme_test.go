package game

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

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
