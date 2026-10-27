package status

import (
	"reflect"
	"testing"

	"ugataima/internal/config"
)

// Save data read at half the rate it was written at keeps each frame count's
// seconds and nothing else moves: a clock scales itself once, a struct that
// embeds one still scales its tagged fields beside it, untagged numbers stay.
func TestRescaleSavedKeepsSeconds(t *testing.T) {
	type marked struct {
		DualClock
		Delay int `ticks:"frames"`
		Count int
	}
	type saved struct {
		Clock  DualClock
		Marked marked
		Marks  map[string]marked
		Tagged map[string]int `ticks:"frames"`
		Plain  map[string]int
		Counts []int
		Unset  *DualClock
	}
	clock := DualClock{Frames: 200, Turns: 2, Rate: 360}
	data := saved{
		Clock:  clock,
		Marked: marked{DualClock: clock, Delay: 240, Count: 7},
		Marks:  map[string]marked{"fire": {DualClock: clock, Delay: 240, Count: 7}},
		Tagged: map[string]int{"wolf": 240},
		Plain:  map[string]int{"wolf": 240},
		Counts: []int{240},
	}
	live := config.GetTargetTPS()
	if got := RescaleSaved(&data, 2*live); got != live {
		t.Fatalf("stamp after the rescale %d, want the live %d", got, live)
	}
	half := DualClock{Frames: 100, Turns: 2, Rate: 180}
	want := saved{
		Clock:  half,
		Marked: marked{DualClock: half, Delay: 120, Count: 7},
		Marks:  map[string]marked{"fire": {DualClock: half, Delay: 120, Count: 7}},
		Tagged: map[string]int{"wolf": 120},
		Plain:  map[string]int{"wolf": 240},
		Counts: []int{240},
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("rescaled\n %+v\nwant\n %+v", data, want)
	}
	// No stamp (0) and a corrupt one (below 0) read as written at the default rate.
	for _, stamp := range []int{0, -config.DefaultTPS} {
		got, atDefault := clock, clock
		RescaleSaved(&atDefault, config.DefaultTPS)
		if RescaleSaved(&got, stamp); got != atDefault {
			t.Errorf("stamp %d: %+v, want %+v as at the default rate", stamp, got, atDefault)
		}
	}
}
