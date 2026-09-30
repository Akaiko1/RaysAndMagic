package game

import (
	"fmt"
	"image"
	"math"
	"sort"
	"testing"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

type wantInterfaceFrame struct {
	w, h        int
	sharp, same bool
}

// interfaceTestDisplays are the screens the interface is checked on: window
// size in window pixels, device scale, and what Normal/Large/Largest become.
var interfaceTestDisplays = []struct {
	name       string
	outW, outH int
	scale      float64
	want       [3]wantInterfaceFrame
}{
	{"1366x768", 1366, 768, 1, [3]wantInterfaceFrame{{1366, 768, true, false}, {1209, 680, false, false}, {1209, 680, false, true}}},
	{"1920x1080", 1920, 1080, 1, [3]wantInterfaceFrame{{1920, 1080, true, false}, {1536, 864, false, false}, {1280, 720, false, false}}},
	{"1920x1200", 1920, 1200, 1, [3]wantInterfaceFrame{{1728, 1080, false, false}, {1382, 864, false, false}, {1152, 720, false, false}}},
	{"2560x1440", 2560, 1440, 1, [3]wantInterfaceFrame{{1920, 1080, false, false}, {1280, 720, true, false}, {1280, 720, true, true}}},
	{"3440x1440", 3440, 1440, 1, [3]wantInterfaceFrame{{2580, 1080, false, false}, {1720, 720, true, false}, {1720, 720, true, true}}},
	{"3840x2160", 3840, 2160, 1, [3]wantInterfaceFrame{{1920, 1080, true, false}, {1280, 720, true, false}, {1280, 720, true, true}}},
	{"4K HiDPI", 1920, 1080, 2, [3]wantInterfaceFrame{{1920, 1080, true, false}, {1280, 720, true, false}, {1280, 720, true, true}}},
	{"MacBook 14", 1512, 982, 2, [3]wantInterfaceFrame{{1512, 982, true, false}, {1210, 786, false, false}, {1047, 680, false, false}}},
	{"MacBook 16", 1728, 1117, 2, [3]wantInterfaceFrame{{1671, 1080, false, false}, {1152, 744, true, false}, {1152, 744, true, true}}},
	{"5K HiDPI", 2560, 1440, 2, [3]wantInterfaceFrame{{1920, 1080, false, false}, {1706, 960, true, false}, {1280, 720, true, false}}},
	{"window HiDPI", 1500, 900, 2, [3]wantInterfaceFrame{{1500, 900, true, false}, {1200, 720, false, false}, {1133, 680, false, false}}},
	{"small window", 640, 480, 1, [3]wantInterfaceFrame{{907, 680, false, false}, {907, 680, false, true}, {907, 680, false, true}}},
	{"portrait", 1080, 1920, 1, [3]wantInterfaceFrame{{800, 1422, false, false}, {800, 1422, false, true}, {800, 1422, false, true}}},
}

// Larger presets take the whole-number steps above Normal while the UI
// minimum fits; only screens without such a step fall back to a soft scale,
// and a preset that cannot grow on a screen is flagged as the same size.
func TestResolveInterfaceFrames(t *testing.T) {
	sizes := loadTestConfig(t).Display.InterfaceSizes
	if len(sizes) != 3 {
		t.Fatalf("rows below are written for three presets, config has %d", len(sizes))
	}
	minW, minH := MinimumWindowSize()
	for _, d := range interfaceTestDisplays {
		frames := resolveInterfaceFrames(sizes, d.outW, d.outH, d.scale)
		for i, want := range d.want {
			t.Run(fmt.Sprintf("%s/%s", d.name, sizes[i].Key), func(t *testing.T) {
				got := frames[i]
				if got.w != want.w || got.h != want.h || got.sharp != want.sharp || got.same != want.same {
					t.Fatalf("frame %dx%d sharp=%v same=%v (x%.3f), want %dx%d sharp=%v same=%v",
						got.w, got.h, got.sharp, got.same, got.scale, want.w, want.h, want.sharp, want.same)
				}
				if got.w < minW || got.h < minH {
					t.Fatalf("frame %dx%d below the UI minimum %dx%d", got.w, got.h, minW, minH)
				}
			})
		}
	}
}

// interfaceFrameSizes is every logical frame the presets produce on the test
// displays, plus the UI minimum: the sizes every menu and tooltip must read at.
func interfaceFrameSizes(t testing.TB) [][2]int {
	t.Helper()
	// Galleries run from the repository root, where the test config path no
	// longer resolves; any loaded config carries the same shipped presets.
	cfg := config.GlobalConfig
	if cfg == nil {
		cfg = loadTestConfig(t)
	}
	sizes := cfg.Display.InterfaceSizes
	seen := map[[2]int]bool{}
	minW, minH := MinimumWindowSize()
	seen[[2]int{minW, minH}] = true
	for _, d := range interfaceTestDisplays {
		for _, f := range resolveInterfaceFrames(sizes, d.outW, d.outH, d.scale) {
			seen[[2]int{f.w, f.h}] = true
		}
	}
	out := make([][2]int, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// withInterfaceFrames adds every interface frame to a test's own list of
// logical sizes, so layout checks cover each size a preset can produce.
func withInterfaceFrames(t testing.TB, logical [][2]int) [][2]int {
	t.Helper()
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, s := range append(append([][2]int(nil), logical...), interfaceFrameSizes(t)...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Whole scales are drawn exactly (nearest-neighbour, whole-pixel offsets);
// anything else keeps Ebitengine's own transform.
func TestSnapFinalScreenGeoM(t *testing.T) {
	screen := image.Rect(0, 0, 3840, 2160)
	letterbox := func(frame image.Rectangle) ebiten.GeoM {
		scale := min(float64(screen.Dx())/float64(frame.Dx()), float64(screen.Dy())/float64(frame.Dy()))
		var m ebiten.GeoM
		m.Scale(scale, scale)
		m.Translate((float64(screen.Dx())-float64(frame.Dx())*scale)/2, (float64(screen.Dy())-float64(frame.Dy())*scale)/2)
		return m
	}
	for _, tc := range []struct {
		name       string
		frame      image.Rectangle
		wantScale  float64
		wantOffset [2]float64
		snapped    bool
	}{
		{"exact x3", image.Rect(0, 0, 1280, 720), 3, [2]float64{0, 0}, true},
		{"exact x2", image.Rect(0, 0, 1920, 1080), 2, [2]float64{0, 0}, true},
		{"a hair above x3", image.Rect(0, 0, 1279, 719), 3, [2]float64{1, 1}, true},
		{"fractional", image.Rect(0, 0, 1536, 864), 2.5, [2]float64{0, 0}, false},
		{"downscale", image.Rect(0, 0, 4000, 2250), 0.96, [2]float64{0, 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := letterbox(tc.frame)
			got := snapFinalScreenGeoM(screen, tc.frame, in)
			if !tc.snapped {
				if got != in {
					t.Fatal("a non-whole scale was changed")
				}
				return
			}
			if s := got.Element(0, 0); s != tc.wantScale || got.Element(1, 1) != tc.wantScale {
				t.Fatalf("scale %.4f, want %.0f", s, tc.wantScale)
			}
			if ox, oy := got.Element(0, 2), got.Element(1, 2); ox != tc.wantOffset[0] || oy != tc.wantOffset[1] || ox != math.Floor(ox) || oy != math.Floor(oy) {
				t.Fatalf("offset (%.2f, %.2f), want whole pixels %v", ox, oy, tc.wantOffset)
			}
		})
	}
}
