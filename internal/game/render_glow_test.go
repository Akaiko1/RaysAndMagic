package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDrawBubbleSpritePinsItsFilter(t *testing.T) {
	r := &Renderer{}
	screen := ebiten.NewImage(16, 16)
	// Simulate a solid glow having used the shared options immediately before
	// the bubble. The bubble must not inherit that path's nearest sampler.
	r.glowOpts.Filter = ebiten.FilterNearest
	r.drawBubbleSprite(screen, 8, 8, 4, [3]int{255, 255, 255}, 1, ebiten.BlendSourceOver)
	if r.glowOpts.Filter != ebiten.FilterLinear {
		t.Fatalf("bubble filter = %v, want linear", r.glowOpts.Filter)
	}
}
