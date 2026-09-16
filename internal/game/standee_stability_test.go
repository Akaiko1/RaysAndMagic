package game

import (
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
		{"horizontal compression", 16, 128, 8},
		{"vertical compression", 128, 16, 8},
		{"moderate angle", 64, 128, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := standeeProjectedFootprint(tc.width, tc.height, 512, 512); got != float32(tc.expect) {
				t.Fatalf("footprint = %g, want %g", got, tc.expect)
			}
		})
	}
}

func TestStandeeEdgeOnFootprintSelectsCoarsestMip(t *testing.T) {
	for _, size := range [][2]float64{{0, 100}, {100, 0}, {0, 0}} {
		footprint := standeeProjectedFootprint(size[0], size[1], 512, 512)
		if level, _ := mipLevelBlend(footprint, maxMipLevel); level != maxMipLevel {
			t.Fatalf("edge-on footprint %v selected level %d, want %d", size, level, maxMipLevel)
		}
	}
}
