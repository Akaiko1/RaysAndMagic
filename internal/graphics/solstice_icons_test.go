package graphics

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"ugataima/internal/config"
)

// Every Solstice source is complete, centered and frame-safe. Content cells:
// 24 resources, 8 equipment items, 4 weapons, 12 potions and 8 brewed variants.
// Entry cells: HUD, synchronous framing and worker framing. Persistence: N/A.
func TestSolsticeIconRegistration(t *testing.T) {
	t.Chdir("../..")
	oldItems, oldWeapons, oldFrames := config.GlobalItems, config.GlobalWeapons, config.GlobalIconFrames
	t.Cleanup(func() {
		config.GlobalItems, config.GlobalWeapons, config.GlobalIconFrames = oldItems, oldWeapons, oldFrames
	})
	readYAML := func(path string, dst any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, dst); err != nil {
			t.Fatal(err)
		}
	}
	config.GlobalItems = &config.ItemSystemConfig{}
	config.GlobalWeapons = &config.WeaponSystemConfig{}
	readYAML("assets/items.yaml", config.GlobalItems)
	readYAML("assets/weapons.yaml", config.GlobalWeapons)
	cfg := &config.IconFramesConfig{}
	readYAML("assets/icon_frames.yaml", cfg)
	for name := range cfg.Icons {
		if !strings.Contains(name, "solstice_") {
			delete(cfg.Icons, name)
		}
	}
	if len(cfg.Icons) != 56 {
		t.Fatalf("expected all 56 Solstice icons, got %d", len(cfg.Icons))
	}
	config.GlobalIconFrames = cfg
	sm := NewSpriteManager()
	sm.spritePaths = map[string]string{}
	sm.spriteDirType = map[string]string{}
	for name := range cfg.Icons {
		sm.spritePaths[name] = filepath.Join("assets/sprites/interface/items", name+".png")
		sm.spriteDirType[name] = "interface"
	}
	sm.indexHUDIcons()
	for name := range cfg.Icons {
		t.Run(name, func(t *testing.T) {
			hud := sm.decodePreparedResource(SpriteResourceRequest{Name: sm.HUDIconName(name)})
			if !hud.Found {
				t.Fatal("missing source")
			}
			art := hud.CPU
			if art.Bounds() != image.Rect(0, 0, 128, 128) {
				t.Fatal("non-square source")
			}
			bounds := image.Rectangle{}
			for y := range 128 {
				for x := range 128 {
					c := art.RGBAAt(x, y)
					if c.A != 255 {
						t.Fatal("non-opaque backing")
					}
					if max(c.R, c.G, c.B) > 16 {
						bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
					}
				}
			}
			if bounds.Empty() {
				t.Fatal("empty icon")
			}
			abs := func(n int) int {
				if n < 0 {
					return -n
				}
				return n
			}
			if abs(bounds.Min.X+bounds.Max.X-128) > 1 || abs(bounds.Min.Y+bounds.Max.Y-128) > 1 {
				t.Fatalf("source is not centered: %v", bounds)
			}
			if bounds.Min.X < 8 || bounds.Min.Y < 8 || bounds.Max.X > 120 || bounds.Max.Y > 120 {
				t.Fatalf("unsafe margins: %v", bounds)
			}
			if strings.HasPrefix(name, "icon_item_brewed_") {
				base := strings.Replace(name, "icon_item_brewed_", "icon_item_", 1)
				other := sm.decodePreparedResource(SpriteResourceRequest{Name: sm.HUDIconName(base)})
				if !other.Found || !bytes.Equal(art.Pix, other.CPU.Pix) {
					t.Fatal("brewed variant differs from corrected base")
				}
			}
			req := SpriteResourceRequest{Name: name}
			framed := sm.decodePreparedResource(req)
			if !framed.Found {
				t.Fatal("missing framed icon")
			}
			// The visible frame must not cover the subject; faint mask antialiasing is allowed.
			for y := range 128 {
				for x := range 128 {
					c := art.RGBAAt(x, y)
					if max(c.R, c.G, c.B) > 32 && sm.iconFrames[name].mask.RGBAAt(x, y).A > 32 && c != framed.CPU.RGBAAt(x, y) {
						t.Fatalf("frame covers silhouette at %d,%d", x, y)
					}
				}
			}
			for result := range sm.PrepareResources(context.Background(), []SpriteResourceRequest{req}) {
				if !result.Found || !bytes.Equal(framed.CPU.Pix, result.CPU.Pix) {
					t.Fatal("worker registration differs")
				}
				result.QueueLease.Release()
			}
			if dir := os.Getenv("RAM_ICON_GALLERY"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, framed.CPU)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
