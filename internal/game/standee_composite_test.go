package game

import "testing"

// Non-cross tokens, fades and single-face LOD retain the material compositor.
// Shell counts change geometry cost only, never the number of visible layers.
func TestStandeeVolumeEligibility(t *testing.T) {
	for _, tc := range []struct {
		name           string
		shells         int
		cross          bool
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
		{name: "transient opacity", shells: 6, cross: true, fade: 0.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slab := standeeSlab{surfaces: make([]standeeSurface, tc.shells+2), volumeComposite: tc.cross, firstSurface: tc.first, sideFade: tc.sideFade, fade: tc.fade}
			if got := canUseStandeeVolume(slab); got != tc.want {
				t.Fatalf("volume eligibility=%v want %v", got, tc.want)
			}
		})
	}
}
