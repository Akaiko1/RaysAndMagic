package game

import (
	"math"
	"testing"
)

func TestStatusFxSpanVisibleChecksEveryCoveredDepthColumn(t *testing.T) {
	const width = 12
	clearDepth := make([]float64, width)
	clearActors := make([]float64, width)
	for i := range width {
		clearDepth[i] = math.Inf(1)
		clearActors[i] = math.Inf(1)
	}
	g := &MMGame{depthBuffer: clearDepth, actorDepthBuffer: clearActors}
	r := &Renderer{game: g}
	depth := 20.0

	if !r.statusFxSpanVisible(depth, 5, 4) {
		t.Fatal("unoccluded status primitive should remain visible")
	}
	g.depthBuffer[6] = 10 // rightmost covered column, not the primitive centre
	if r.statusFxSpanVisible(depth, 5, 4) {
		t.Fatal("wall covering the primitive edge must occlude that primitive")
	}
	g.depthBuffer[6] = math.Inf(1)
	g.actorDepthBuffer[3] = 10 // left edge of the primitive
	if r.statusFxSpanVisible(depth, 5, 4) {
		t.Fatal("actor covering the primitive edge must occlude that primitive")
	}
	g.actorDepthBuffer[3] = math.Inf(1)
	g.depthBuffer[7] = 10 // immediately beside the half-open [3,7) span
	if !r.statusFxSpanVisible(depth, 5, 4) {
		t.Fatal("an occluder beside the primitive must not hide it")
	}
	if !r.statusFxSpanVisible(depth, 9, 1) {
		t.Fatal("an occluder outside the primitive span must not hide it")
	}
}

func TestStatusFxSpanVisibleAllowsSyntheticGalleryWithoutDepthBuffers(t *testing.T) {
	r := &Renderer{game: &MMGame{}}
	if !r.statusFxSpanVisible(20, 50, 10) {
		t.Fatal("gallery status effects should render without world depth buffers")
	}
}
