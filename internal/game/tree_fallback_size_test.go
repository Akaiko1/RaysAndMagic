package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestFlatTreeFallbackUsesClassAsWidthAndPreservesAspect(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 3, 3))
	g.renderHelper = NewRenderingHelper(g)
	r := &Renderer{game: g}
	source := ebiten.NewImage(512, 1024)
	distance := 4 * float64(cfg.GetTileSize())

	width, height := r.flatTreeFallbackSize(distance, 2, source)
	wantWidth := g.renderHelper.calculateSpriteSizeWithHeightMultiplier(distance, 2)
	if width != wantWidth {
		t.Fatalf("flat tree width = %d, want class-projected width %d", width, wantWidth)
	}
	if height != width*2 {
		t.Fatalf("flat tree size = %dx%d, want 1:2 source aspect", width, height)
	}
}
