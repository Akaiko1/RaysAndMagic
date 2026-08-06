package game

import (
	"math"
	"testing"
)

func TestTrapFxSpanVisibleChecksEveryCoveredDepthColumn(t *testing.T) {
	const width = 12
	clearDepth := make([]float64, width)
	clearActors := make([]float64, width)
	for i := range width {
		clearDepth[i] = math.Inf(1)
		clearActors[i] = math.Inf(1)
	}
	g := &MMGame{depthBuffer: clearDepth, actorDepthBuffer: clearActors}
	r := &Renderer{game: g}
	a := trapAnchor{depth: 20}

	if !r.trapFxSpanVisible(a, 5, 4) {
		t.Fatal("unoccluded trap primitive should remain visible")
	}
	g.depthBuffer[6] = 10 // rightmost covered column, not the primitive centre
	if r.trapFxSpanVisible(a, 5, 4) {
		t.Fatal("wall covering the primitive edge must occlude that primitive")
	}
	g.depthBuffer[6] = math.Inf(1)
	g.actorDepthBuffer[3] = 10 // left edge of the primitive
	if r.trapFxSpanVisible(a, 5, 4) {
		t.Fatal("actor covering the primitive edge must occlude that primitive")
	}
	g.actorDepthBuffer[3] = math.Inf(1)
	g.depthBuffer[7] = 10 // immediately beside the half-open [3,7) span
	if !r.trapFxSpanVisible(a, 5, 4) {
		t.Fatal("an occluder beside the primitive must not hide it")
	}
	if !r.trapFxSpanVisible(a, 9, 1) {
		t.Fatal("an occluder outside the primitive span must not hide it")
	}
}

func TestTrapFxSpanVisibleAllowsSyntheticGalleryWithoutDepthBuffers(t *testing.T) {
	r := &Renderer{game: &MMGame{}}
	if !r.trapFxSpanVisible(trapAnchor{depth: 20}, 50, 10) {
		t.Fatal("gallery trap anchors should render without world depth buffers")
	}
}
