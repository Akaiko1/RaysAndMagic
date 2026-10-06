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

// The isotropic mip follows the LESS compressed axis: a grazing or edge-on face
// must not select a silhouette blurred along its visible axis.
func TestStandeeFootprintBoundsDirectionalSampling(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		width, height, texW, texH float64
		wantFootprint             float64
		wantLevel                 int
	}{
		{"native", 512, 512, 512, 512, 1, 0},
		{"magnified", 1024, 1024, 512, 512, 1, 0},
		{"square", 64, 64, 512, 512, 8, 3},
		{"horizontal compression", 16, 128, 512, 512, 4, 2},
		{"vertical compression", 128, 16, 512, 512, 4, 2},
		{"moderate angle", 64, 128, 512, 512, 4, 2},
		{"anisotropic small texture", 128, 64, 256, 256, 2, 1},
		{"magnified small texture", 512, 512, 256, 256, 1, 0},
		{"edge-on width", 0, 100, 512, 512, 5.12, 2},
		{"edge-on height", 100, 0, 512, 512, 5.12, 2},
		{"collapsed both axes", 0, 0, 512, 512, 512 / 1e-6, maxMipLevel},
		{"grazing subpixel width", 0.01, 300, 512, 1024, 1024.0 / 300, 2},
		{"grazing one pixel width", 1, 300, 512, 1024, 1024.0 / 300, 2},
		{"grazing one pixel height", 300, 1, 1024, 512, 1024.0 / 300, 2},
		{"tall texture moderate angle", 100, 200, 512, 1024, 5.12, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := standeeProjectedFootprint(tc.width, tc.height, tc.texW, tc.texH)
			if math.Abs(float64(got)-tc.wantFootprint) > 1e-5*max(1, tc.wantFootprint) {
				t.Fatalf("footprint = %g, want %g", got, tc.wantFootprint)
			}
			if level, _ := mipLevelBlend(got, maxMipLevel); level != tc.wantLevel {
				t.Fatalf("footprint %g selected level %d, want %d", got, level, tc.wantLevel)
			}
		})
	}
}
