package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The torch and gold settings name the key that is wrong, and the defaults
// and the shipped config.yaml load.
func TestWallTorchAndGoldSettings(t *testing.T) {
	if err := DefaultWallTorchRenderConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := DefaultGoldReflectionRenderConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	torch := []struct {
		key    string
		change func(*WallTorchRenderConfig)
	}{
		{"light_radius_tiles", func(c *WallTorchRenderConfig) { c.LightRadiusTiles = 0 }},
		{"light_radius_tiles", func(c *WallTorchRenderConfig) { c.LightRadiusTiles = WallTorchMaxLightRadiusTiles + 1 }},
		{"light_intensity", func(c *WallTorchRenderConfig) { c.LightIntensity = 1.5 }},
		{"self_light", func(c *WallTorchRenderConfig) { c.SelfLight = math.NaN() }},
		{"body_tiles", func(c *WallTorchRenderConfig) { c.BodyTiles = 0 }},
		{"body_tiles", func(c *WallTorchRenderConfig) { c.BodyTiles = math.NaN() }},
		{"detail_tiles", func(c *WallTorchRenderConfig) { c.DetailTiles = 0 }},
		{"detail_tiles", func(c *WallTorchRenderConfig) { c.DetailTiles = c.BodyTiles + 1 }},
		{"wood", func(c *WallTorchRenderConfig) { c.Wood[1] = 256 }},
		{"coals", func(c *WallTorchRenderConfig) { c.Coals[2] = -1 }},
		{"torch_halo.size", func(c *WallTorchRenderConfig) { c.TorchHalo.Size = 0 }},
		{"lantern_halo.size", func(c *WallTorchRenderConfig) { c.LanternHalo.Size = WallTorchMaxHaloSize + 1 }},
		{"lantern_halo.opacity", func(c *WallTorchRenderConfig) { c.LanternHalo.Opacity = 1.5 }},
		{"torch_halo.color", func(c *WallTorchRenderConfig) { c.TorchHalo.Color[0] = 300 }},
		{"spark", func(c *WallTorchRenderConfig) { c.Spark[1] = -3 }},
	}
	for _, tt := range torch {
		c := DefaultWallTorchRenderConfig()
		tt.change(&c)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "graphics.wall_torches."+tt.key) {
			t.Fatalf("%+v: error %v, want one naming %s", c, err, tt.key)
		}
	}
	for _, deg := range []float64{-1, StandeeCrossFlipBlendMaxDeg + 1, math.NaN()} {
		c := StandeeConfig{CrossFlipBlendDeg: deg}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "graphics.standee.cross_flip_blend_deg") {
			t.Fatalf("cross_flip_blend_deg %v: error %v", deg, err)
		}
	}
	for _, passes := range []float64{0, -1, math.NaN()} {
		c := GoldReflectionRenderConfig{TilesPerPass: passes}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "graphics.gold_reflection.tiles_per_pass") {
			t.Fatalf("tiles_per_pass %v: error %v", passes, err)
		}
	}

	if _, err := LoadConfig("../../config.yaml"); err != nil {
		t.Fatal(err)
	}

	// A wrong value in the file is reported with the file and the key.
	shipped, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ good, bad, key string }{
		{"body_tiles: 12", "body_tiles: 0", "graphics.wall_torches.body_tiles"},
		{"tiles_per_pass: 1.5", "tiles_per_pass: 0", "graphics.gold_reflection.tiles_per_pass"},
		{"width_px: 5", "width_px: 0", "graphics.monster.hover_halo.width_px"},
		{"from_tiles: 2.0", "from_tiles: 9.0", "graphics.walkable_prop_fade.from_tiles"},
		{"cross_flip_blend_deg: 3", "cross_flip_blend_deg: 31", "graphics.standee.cross_flip_blend_deg"},
	} {
		if !strings.Contains(string(shipped), tt.good) {
			t.Fatalf("config.yaml no longer has %q", tt.good)
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(strings.Replace(string(shipped), tt.good, tt.bad, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tt.key) {
			t.Fatalf("%s: error %v, want the file and %s", tt.bad, err, tt.key)
		}
	}
}

// Without cross_flip_blend_deg, config.yaml keeps the default blend.
func TestStandeeCrossFlipBlendDefault(t *testing.T) {
	shipped, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(shipped), "\n") {
		if !strings.Contains(line, "cross_flip_blend_deg:") {
			kept = append(kept, line)
		}
	}
	if len(kept) == len(strings.Split(string(shipped), "\n")) {
		t.Fatal("config.yaml no longer sets cross_flip_blend_deg")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Graphics.Standee.CrossFlipBlendDeg; got != DefaultStandeeCrossFlipBlendDeg {
		t.Fatalf("cross_flip_blend_deg without the key = %v, want %v", got, DefaultStandeeCrossFlipBlendDeg)
	}
}

// A map's wall_torch_style is one of the known lights and needs wall_torches.
func TestWallTorchStyleValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		m    MapConfig
		ok   bool
	}{
		{"default torch", MapConfig{WallTorches: true}, true},
		{"paper lantern", MapConfig{WallTorches: true, WallTorchStyle: WallTorchStylePaperLantern}, true},
		{"unknown style", MapConfig{WallTorches: true, WallTorchStyle: "brazier"}, false},
		{"style without lights", MapConfig{WallTorchStyle: WallTorchStylePaperLantern}, false},
	} {
		err := MapConfigs{Maps: map[string]MapConfig{"keep": tt.m}}.ValidateWallTorches()
		if tt.ok != (err == nil) || err != nil && !strings.Contains(err.Error(), `map "keep"`) {
			t.Fatalf("%s: error %v", tt.name, err)
		}
	}
	if got := (MapConfig{WallTorches: true}).WallTorchModel(); got != WallTorchStyleTorch {
		t.Fatalf("a map without a style hangs %q", got)
	}
}
