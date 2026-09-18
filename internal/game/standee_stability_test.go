package game

import (
	"math"
	"testing"
)

func TestStandeeSideFadeIsContinuous(t *testing.T) {
	for _, tc := range []struct {
		name     string
		parallax float64
		want     float32
	}{
		{"face on", 0, 1},
		{"subpixel", 0.5, 1},
		{"transition", 1.25, 0.5},
		{"full thickness", 2, 0},
		{"close", 20, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := standeeSideFade(tc.parallax); got != tc.want {
				t.Fatalf("fade = %g, want %g", got, tc.want)
			}
		})
	}
	prev := standeeSideFade(0)
	for i := 1; i <= 3000; i++ {
		next := standeeSideFade(float64(i) / 1000)
		if next > prev || prev-next > 0.00101 {
			t.Fatalf("opacity jumped at parallax %.3f: %g -> %g", float64(i)/1000, prev, next)
		}
		prev = next
	}
}

func TestStandeeFootprintBoundsDirectionalSampling(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		width, height, expect float64
	}{
		{"native", 512, 512, 1},
		{"magnified", 1024, 1024, 1},
		{"square", 64, 64, 8},
		{"horizontal compression", 16, 128, 4},
		{"vertical compression", 128, 16, 4},
		{"moderate angle", 64, 128, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := standeeProjectedFootprint(tc.width, tc.height, 512, 512); got != float32(tc.expect) {
				t.Fatalf("footprint = %g, want %g", got, tc.expect)
			}
		})
	}
}

// Horizontal collapse must not select a vertically blurred silhouette.
func TestStandeeEdgeOnFootprintPreservesVisibleAxis(t *testing.T) {
	for _, tc := range []struct {
		width, height float64
		level         int
	}{{0, 100, 2}, {100, 0, 2}, {0, 0, maxMipLevel}} {
		footprint := standeeProjectedFootprint(tc.width, tc.height, 512, 512)
		if level, _ := mipLevelBlend(footprint, maxMipLevel); level != tc.level {
			t.Fatalf("footprint at %gx%g selected level %d, want %d", tc.width, tc.height, level, tc.level)
		}
	}
}

func TestGrazingStandeeMipPreservesUncompressedAxis(t *testing.T) {
	for _, dims := range [][4]float64{{0.01, 300, 512, 1024}, {1, 300, 512, 1024}, {300, 1, 1024, 512}, {100, 200, 512, 1024}, {1024, 1024, 512, 512}} {
		footprint := standeeProjectedFootprint(dims[0], dims[1], dims[2], dims[3])
		minor := max(1, math.Min(dims[2]/dims[0], dims[3]/dims[1]))
		if float64(footprint) > minor+1e-5 {
			t.Fatalf("grazing width blurred the uncompressed silhouette: footprint=%g bound=%g", footprint, minor)
		}
	}
}
