package graphics

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/config"
)

// Case table: edge-only/excluded x base/sheet x static/every directional action x
// synchronous/background decoding. Similar prefixes and non-animation names
// remain independent. An art family (edge-only or excluded) keeps key-close art
// inside the body (a glowing gem) with its surroundings, while key joined to the
// image border, or diagonally to a hole, is background for every sprite.
// Save/load is N/A: this is a load-time asset policy.
func TestDespillFamilyResourceLoading(t *testing.T) {
	purple := color.NRGBA{120, 40, 160, 255}
	gray := color.NRGBA{40, 40, 80, 255}
	gem := color.NRGBA{200, 50, 245, 255} // within the key tolerance
	src := image.NewNRGBA(image.Rect(0, 0, 64, 16))
	draw.Draw(src, src.Bounds(), image.NewUniform(purple), image.Point{}, draw.Src)
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 255, 255})
	src.SetNRGBA(40, 8, gem)
	for x := 54; x < 64; x++ { // a key run from the border deep into the art
		src.SetNRGBA(x, 4, color.NRGBA{255, 0, 255, 255})
	}
	src.SetNRGBA(20, 8, color.NRGBA{}) // a hole; key residue touches it diagonally
	src.SetNRGBA(21, 9, color.NRGBA{250, 50, 230, 255})
	path := filepath.Join(t.TempDir(), "sheet.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, src)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	requests := []SpriteResourceRequest{{Name: "lich"}, {Name: "lich_king"}, {Name: "lich_full"}, {Name: "lich_walking_extra_r"}}
	for _, action := range []string{"walking", "attacking", "dying", "climbing", "descending", "jumping", "perched", "leaping"} {
		for _, direction := range []string{"r", "l"} {
			requests = append(requests, SpriteResourceRequest{Name: "lich", AnimationType: action + "_" + direction})
		}
	}
	for _, excluded := range []bool{false, true} {
		for _, exemption := range []string{"lich", "lich_walking_r", "lich_dying_l"} {
			for _, request := range requests {
				for _, background := range []bool{false, true} {
					name := request.Name
					if request.AnimationType != "" {
						name += "_" + request.AnimationType
					}
					mode := "immediate"
					if background {
						mode = "background"
					}
					t.Run(fmt.Sprintf("excluded=%v/%s/%s/%s", excluded, exemption, name, mode), func(t *testing.T) {
						sm := NewSpriteManager()
						sm.spritePaths = map[string]string{name: path}
						sm.SetColorKey(true, 255, 0, 255, 60, true)
						sm.SetDespillEdgeOnly([]string{exemption}, 3)
						if excluded {
							sm.SetDespillExclusions([]string{exemption})
						}
						var prepared PreparedSpriteResource
						if background {
							for result := range sm.PrepareResources(context.Background(), []SpriteResourceRequest{request}) {
								prepared = result
							}
						} else {
							prepared = sm.decodePreparedResource(request)
						}
						defer prepared.QueueLease.Release()
						if !prepared.Found || prepared.Image == nil {
							t.Fatal("source was not decoded")
						}
						want := gray
						if request.Name == "lich" {
							want = purple
						}
						if got := color.NRGBAModel.Convert(prepared.Image.At(8, 8)); got != want {
							t.Fatalf("interior: got %v want %v", got, want)
						}
						if got := color.NRGBAModel.Convert(prepared.Image.At(41, 8)); got != want {
							t.Fatalf("beside the gem: got %v want %v", got, want)
						}
						gemAlpha := uint32(0)
						if request.Name == "lich" {
							gemAlpha = 0xffff
						}
						if _, _, _, alpha := prepared.Image.At(40, 8).RGBA(); alpha != gemAlpha {
							t.Fatalf("key-close art inside the body: alpha %#x want %#x", alpha, gemAlpha)
						}
						if _, _, _, alpha := prepared.Image.At(54, 4).RGBA(); alpha != 0 {
							t.Fatal("key joined to the border survived")
						}
						if _, _, _, alpha := prepared.Image.At(21, 9).RGBA(); alpha != 0 {
							t.Fatal("key joined diagonally to a hole survived")
						}
						fringe := gray
						if excluded && request.Name == "lich" {
							fringe = purple
						}
						if got := color.NRGBAModel.Convert(prepared.Image.At(1, 1)); got != fringe {
							t.Fatalf("fringe: got %v want %v", got, fringe)
						}
						if _, _, _, alpha := prepared.Image.At(0, 0).RGBA(); alpha != 0 {
							t.Fatal("key core survived")
						}
					})
				}
			}
		}
	}
}

// The authored case table comes from YAML and the real sprite index, rather
// than repeating the loader's animation classifier. Every listed asset is
// checked, including every authored motion of a listed monster. A listed sprite
// loses exactly its outside and keeps its interior as authored (an excluded one
// every kept pixel); async loading must match immediate load.
func TestDespillConfiguredAssetCatalog(t *testing.T) {
	t.Chdir("../..")
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("assets/monsters.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Monsters map[string]struct{ Sprite string }
	}
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	// A candidate belongs to a monster only if it is that exact base or an
	// authored <base>_<motion>_<direction> sheet. Prefix relatives are separate.
	motion := func(name, base string) (string, bool) {
		suffix, ok := strings.CutPrefix(name, base+"_")
		parts := strings.Split(suffix, "_")
		return suffix, ok && len(parts) == 2 && parts[0] != "" && (parts[1] == "r" || parts[1] == "l")
	}
	sm := NewSpriteManager()
	sm.ensureIndex()
	ck := cfg.Graphics.ColorKey
	sm.SetColorKey(ck.Enabled, ck.Color[0], ck.Color[1], ck.Color[2], ck.Tolerance, ck.Despill)
	sm.SetIllustrations(ck.Illustrations)
	sm.SetDespillEdgeOnly(ck.EdgeOnlyDespill, ck.EdgeDespillRadius)
	sm.SetDespillExclusions(ck.DespillExclusions)
	requests := map[string]SpriteResourceRequest{}
	families := map[string]int{}
	entries := slices.Concat(ck.EdgeOnlyDespill, ck.DespillExclusions)
	for _, entry := range entries {
		if sm.spritePaths[entry] == "" {
			t.Errorf("configured exception %q has no asset", entry)
			continue
		}
		requests[entry] = SpriteResourceRequest{Name: entry}
		for _, def := range catalog.Monsters {
			_, animated := motion(entry, def.Sprite)
			if entry != def.Sprite && !animated {
				continue
			}
			families[def.Sprite] = 0
			for name := range sm.spritePaths {
				if name == def.Sprite {
					requests[name] = SpriteResourceRequest{Name: def.Sprite}
				} else if anim, ok := motion(name, def.Sprite); ok {
					requests[name] = SpriteResourceRequest{Name: def.Sprite, AnimationType: anim}
				}
			}
		}
	}
	names := make([]string, 0, len(requests))
	for name := range requests {
		names = append(names, name)
	}
	sort.Strings(names)
	var report strings.Builder
	fmt.Fprint(&report, "# Configured color-preservation audit\n\n")
	fmt.Fprintln(&report, "| Asset | Protected interior pixels | Immediate/background |\n|---|---:|---|")
	for _, name := range names {
		request := requests[name]
		if _, ok := families[request.Name]; ok {
			families[request.Name]++
		}
		t.Run(name, func(t *testing.T) {
			src := loadPNGForColorKeyTest(t, sm.spritePaths[name])
			prepared := sm.decodePreparedResource(request)
			if !prepared.Found {
				t.Fatal("authored resource did not load")
			}
			// A listed sprite's outside is its transparency plus the key joined to it
			// or to the image border; the loader erases exactly that. An excluded
			// sprite (same family rule as runtime) keeps every other pixel; an
			// edge-only one every pixel beyond the despill band.
			excluded := sm.keyDespillExcluded[spriteDespillFamily(request.Name)]
			b := src.Bounds()
			w, h, stride := b.Dx(), b.Dy(), b.Dx()+1
			near := func(v uint8, channel int) bool { return max(int(v)-channel, channel-int(v)) <= ck.Tolerance }
			outside := colorKeyOutside(src, func(p color.NRGBA) bool {
				return near(p.R, ck.Color[0]) && near(p.G, ck.Color[1]) && near(p.B, ck.Color[2])
			})
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					got := prepared.CPU.RGBAAt(x, y)
					if erased := got.A == 0; erased != outside[y*w+x] {
						t.Fatalf("(%d,%d): erased %v, outside %v", x, y, erased, outside[y*w+x])
					}
					if excluded && !outside[y*w+x] && got != color.RGBAModel.Convert(src.At(x+b.Min.X, y+b.Min.Y)) {
						t.Fatalf("excluded sprite colour changed at (%d,%d)", x, y)
					}
				}
			}
			// Summed-area mask lets us reject any pixel whose fringe neighborhood
			// touches the outside in constant time.
			invalid := make([]int, stride*(h+1))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					v := 0
					if outside[y*w+x] {
						v = 1
					}
					invalid[(y+1)*stride+x+1] = v + invalid[y*stride+x+1] + invalid[(y+1)*stride+x] - invalid[y*stride+x]
				}
			}
			radius := ck.EdgeDespillRadius
			if radius <= 0 {
				radius = despillEdgeRadiusDefault
			}
			checked, purple := 0, 0
			for y := radius; y < h-radius; y++ {
				for x := radius; x < w-radius; x++ {
					left, right, top, bottom := x-radius, x+radius+1, y-radius, y+radius+1
					if invalid[bottom*stride+right]-invalid[top*stride+right]-invalid[bottom*stride+left]+invalid[top*stride+left] != 0 {
						continue
					}
					checked++
					p := src.NRGBAAt(x+b.Min.X, y+b.Min.Y)
					if int(min(p.R, p.B))-int(p.G) > despillHueFloor {
						purple++
					}
					if !excluded && prepared.CPU.RGBAAt(x, y) != color.RGBAModel.Convert(src.At(x+b.Min.X, y+b.Min.Y)) {
						t.Fatalf("interior color changed at (%d,%d)", x, y)
					}
				}
			}
			if checked == 0 {
				t.Fatal("no interior pixels checked")
			}
			for result := range sm.PrepareResources(context.Background(), []SpriteResourceRequest{request}) {
				result.QueueLease.Release()
				if !result.Found || !bytes.Equal(prepared.CPU.Pix, result.CPU.Pix) {
					t.Fatal("immediate/background pixels differ")
				}
			}
			fmt.Fprintf(&report, "| %s | %d (%d purple) | identical |\n", name, checked, purple)
		})
	}
	t.Logf("audited %d configured entries, %d monster families, %d PNG resources", len(entries), len(families), len(names))
	if path := os.Getenv("RAM_DESPILL_AUDIT_REPORT"); path != "" && !t.Failed() {
		if err := os.WriteFile(path, []byte(report.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// colorKeyOutside marks src's transparent pixels and the key pixels joined to
// them or to the image border, 8-connected.
func colorKeyOutside(src *image.NRGBA, key func(color.NRGBA) bool) []bool {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	outside := make([]bool, w*h)
	var stack [][2]int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := src.NRGBAAt(x+b.Min.X, y+b.Min.Y)
			if p.A == 0 || (key(p) && (x == 0 || y == 0 || x == w-1 || y == h-1)) {
				outside[y*w+x] = true
				stack = append(stack, [2]int{x, y})
			}
		}
	}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
			x, y := c[0]+d[0], c[1]+d[1]
			if x < 0 || y < 0 || x >= w || y >= h || outside[y*w+x] {
				continue
			}
			if p := src.NRGBAAt(x+b.Min.X, y+b.Min.Y); p.A != 0 && key(p) {
				outside[y*w+x] = true
				stack = append(stack, [2]int{x, y})
			}
		}
	}
	return outside
}
