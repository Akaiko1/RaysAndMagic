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
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/config"
)

// Case table: base/sheet exemption x static/every directional action x
// synchronous/background decoding. Similar prefixes and non-animation names
// remain independent. Save/load is N/A: this is a load-time asset policy.
func TestDespillFamilyResourceLoading(t *testing.T) {
	purple := color.NRGBA{120, 40, 160, 255}
	gray := color.NRGBA{40, 40, 80, 255}
	src := image.NewNRGBA(image.Rect(0, 0, 64, 16))
	draw.Draw(src, src.Bounds(), image.NewUniform(purple), image.Point{}, draw.Src)
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 255, 255})
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
				t.Run(exemption+"/"+name+"/"+mode, func(t *testing.T) {
					sm := NewSpriteManager()
					sm.spritePaths = map[string]string{name: path}
					sm.SetColorKey(true, 255, 0, 255, 60, true)
					sm.SetDespillEdgeOnly([]string{exemption}, 3)
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
					if got := color.NRGBAModel.Convert(prepared.Image.At(1, 1)); got != gray {
						t.Fatalf("fringe was not cleaned: %v", got)
					}
					if _, _, _, alpha := prepared.Image.At(0, 0).RGBA(); alpha != 0 {
						t.Fatal("key core survived")
					}
				})
			}
		}
	}
}

// The authored case table comes from YAML and the real sprite index, rather
// than repeating the loader's animation classifier. Every listed asset is
// checked, including every authored motion of a listed monster. Interior color
// is compared to key-only processing; async loading must match immediate load.
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
	sm.SetDespillEdgeOnly(ck.EdgeOnlyDespill, ck.EdgeDespillRadius)
	keyOnly := NewSpriteManager()
	keyOnly.SetColorKey(ck.Enabled, ck.Color[0], ck.Color[1], ck.Color[2], ck.Tolerance, false)
	requests := map[string]SpriteResourceRequest{}
	families := map[string]int{}
	for _, entry := range ck.EdgeOnlyDespill {
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
			before := keyOnly.prepareSpritePixels(name, src)
			prepared := sm.decodePreparedResource(request)
			if !prepared.Found {
				t.Fatal("authored resource did not load")
			}
			// Summed-area mask lets us reject any pixel whose fringe neighborhood
			// touches transparency or the color-key core in constant time.
			b := src.Bounds()
			w, h, stride := b.Dx(), b.Dy(), b.Dx()+1
			invalid := make([]int, stride*(h+1))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					p := src.NRGBAAt(x+b.Min.X, y+b.Min.Y)
					v := 0
					near := func(v uint8, channel int) bool { return max(int(v)-channel, channel-int(v)) <= ck.Tolerance }
					if p.A == 0 || (near(p.R, ck.Color[0]) && near(p.G, ck.Color[1]) && near(p.B, ck.Color[2])) {
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
					if prepared.CPU.RGBAAt(x, y) != color.RGBAModel.Convert(before.At(x, y)) {
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
	t.Logf("audited %d configured entries, %d monster families, %d PNG resources", len(ck.EdgeOnlyDespill), len(families), len(names))
	if path := os.Getenv("RAM_DESPILL_AUDIT_REPORT"); path != "" && !t.Failed() {
		if err := os.WriteFile(path, []byte(report.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
