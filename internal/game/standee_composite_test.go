package game

import "testing"

// Crosses and untinted fading tokens take the volume compositor; other tokens,
// tinted ones, single-face LOD and subpixel thickness retain the material
// compositor.
// Shell counts change geometry cost only, never the number of visible layers.
func TestStandeeVolumeEligibility(t *testing.T) {
	for _, tc := range []struct {
		name           string
		shells         int
		cross          bool
		tinted         bool
		first          int
		sideFade, fade float32
		want           bool
	}{
		{name: "minimum cross", shells: 2, cross: true, want: true},
		{name: "medium cross", shells: 5, cross: true, want: true},
		{name: "close cross", shells: 16, cross: true, want: true},
		{name: "single shell", shells: 1, cross: true},
		{name: "ordinary token", shells: 6},
		{name: "single face", shells: 6, cross: true, first: 7},
		{name: "subpixel thickness", shells: 6, cross: true, sideFade: 0.25},
		{name: "fading cross", shells: 6, cross: true, fade: 0.25, want: true},
		{name: "fading token", shells: 6, fade: 0.25, want: true},
		{name: "fading single shell", shells: 1, fade: 0.25},
		{name: "tinted fading token", shells: 6, fade: 0.25, tinted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slab := standeeSlab{surfaces: make([]standeeSurface, tc.shells+2), volumeComposite: tc.cross, firstSurface: tc.first, sideFade: tc.sideFade, fade: tc.fade, rr: 1, gg: 1, bb: 1}
			if tc.tinted {
				slab.gg, slab.bb = 0.7, 0.4
			}
			if got := canUseStandeeVolume(slab); got != tc.want {
				t.Fatalf("volume eligibility=%v want %v", got, tc.want)
			}
		})
	}
}
