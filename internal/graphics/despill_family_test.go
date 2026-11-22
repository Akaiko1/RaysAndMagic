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

// Case table: mode (default, edge-only, full) x listed name (base or a sheet)
// x base/sheet request x static/every directional action x synchronous/
// background decoding. Similar prefixes and non-animation names stay default.
// Key joined to the image border, or diagonally to a hole, is background in
// every mode; key-close art inside the body (a glowing gem) and the body's
// colour survive unless the family is full; the rim is despilled in both
// despill modes. Save/load is N/A: this is a load-time asset policy.
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
	listings := []struct{ mode, listed string }{{"default", ""}}
	for _, mode := range []string{"edge", "full"} {
		for _, listed := range []string{"lich", "lich_walking_r", "lich_dying_l"} {
			listings = append(listings, struct{ mode, listed string }{mode, listed})
		}
	}
	for _, listing := range listings {
		for _, request := range requests {
			for _, background := range []bool{false, true} {
				name := request.Name
				if request.AnimationType != "" {
					name += "_" + request.AnimationType
				}
				decode := "immediate"
				if background {
					decode = "background"
				}
				t.Run(fmt.Sprintf("%s/%s/%s/%s", listing.mode, listing.listed, name, decode), func(t *testing.T) {
					sm := NewSpriteManager()
					sm.spritePaths = map[string]string{name: path}
					sm.SetColorKey(true, 255, 0, 255, 60, true)
					switch listing.mode {
					case "edge":
						sm.SetDespillEdgeOnly([]string{listing.listed}, 3)
					case "full":
						sm.SetDespillFull([]string{listing.listed})
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
					mode := "default"
					if request.Name == "lich" {
						mode = listing.mode
					}
					body, rim, gemAlpha := purple, purple, uint32(0xffff)
					switch mode {
					case "edge":
						rim = gray
					case "full":
						body, rim, gemAlpha = gray, gray, 0
					}
					if got := color.NRGBAModel.Convert(prepared.Image.At(8, 8)); got != body {
						t.Fatalf("interior: got %v want %v", got, body)
					}
					if got := color.NRGBAModel.Convert(prepared.Image.At(41, 8)); got != body {
						t.Fatalf("beside the gem: got %v want %v", got, body)
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
					if got := color.NRGBAModel.Convert(prepared.Image.At(1, 1)); got != rim {
						t.Fatalf("rim: got %v want %v", got, rim)
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
// than repeating the loader's animation classifier. Every edge-only asset is
// checked, including every authored motion of a listed monster. An edge-only
// sprite loses exactly its outside and keeps its interior as authored, and
// async loading must match immediate load.
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
	sm.ConfigureColorKey(ck)
	requests := map[string]SpriteResourceRequest{}
	families := map[string]int{}
	entries := ck.EdgeOnlyDespill
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
	rows := make([]string, len(names))
	// Assets are independent: check them in parallel, one report row each.
	t.Run("assets", func(t *testing.T) {
		for i, name := range names {
			request := requests[name]
			if _, ok := families[request.Name]; ok {
				families[request.Name]++
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				src := loadPNGForColorKeyTest(t, sm.spritePaths[name])
				prepared := sm.decodePreparedResource(request)
				if !prepared.Found {
					t.Fatal("authored resource did not load")
				}
				// An edge-only sprite's outside is its transparency plus the key joined
				// to it or to the image border; the loader erases exactly that and keeps
				// every pixel beyond the despill band.
				b := src.Bounds()
				w, h, stride := b.Dx(), b.Dy(), b.Dx()+1
				outside := colorKeyOutside(src, sm.isKeyColor)
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						got := prepared.CPU.RGBAAt(x, y)
						if erased := got.A == 0; erased != outside[y*w+x] {
							t.Fatalf("(%d,%d): erased %v, outside %v", x, y, erased, outside[y*w+x])
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
						if prepared.CPU.RGBAAt(x, y) != color.RGBAModel.Convert(src.At(x+b.Min.X, y+b.Min.Y)) {
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
				rows[i] = fmt.Sprintf("| %s | %d (%d purple) | identical |", name, checked, purple)
			})
		}
	})
	t.Logf("audited %d configured entries, %d monster families, %d PNG resources", len(entries), len(families), len(names))
	if path := os.Getenv("RAM_DESPILL_AUDIT_REPORT"); path != "" && !t.Failed() {
		report := "# Configured color-preservation audit\n\n| Asset | Protected interior pixels | Immediate/background |\n|---|---:|---|\n" + strings.Join(rows, "\n") + "\n"
		if err := os.WriteFile(path, []byte(report), 0644); err != nil {
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
