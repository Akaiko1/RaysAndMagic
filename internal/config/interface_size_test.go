package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterfaceSizesValidation(t *testing.T) {
	valid := func() DisplayConfig {
		return DisplayConfig{
			InterfaceSizes: []InterfaceSize{
				{Key: "normal", Label: "Normal", Scale: 1},
				{Key: "large", Label: "Large", Scale: 1.25},
			},
			DefaultInterfaceSize: "normal",
		}
	}
	cases := []struct {
		name    string
		mutate  func(d *DisplayConfig)
		wantErr string
	}{
		{"valid", func(*DisplayConfig) {}, ""},
		{"missing list", func(d *DisplayConfig) { d.InterfaceSizes = nil }, "interface_sizes is required"},
		{"empty key", func(d *DisplayConfig) { d.InterfaceSizes[1].Key = "" }, "key is required"},
		{"duplicate key", func(d *DisplayConfig) { d.InterfaceSizes[1].Key = "normal" }, `duplicate key "normal"`},
		{"empty label", func(d *DisplayConfig) { d.InterfaceSizes[1].Label = "" }, "label is required"},
		{"non-ASCII label", func(d *DisplayConfig) { d.InterfaceSizes[1].Label = "Gro\u00df" }, "label must be ASCII"},
		{"below native", func(d *DisplayConfig) { d.InterfaceSizes[0].Scale = 0.9 }, "must be within 1..3"},
		{"too large", func(d *DisplayConfig) { d.InterfaceSizes[1].Scale = 3.5 }, "must be within 1..3"},
		{"not ascending", func(d *DisplayConfig) { d.InterfaceSizes[1].Scale = 1 }, "larger than the previous preset"},
		{"unknown default", func(d *DisplayConfig) { d.DefaultInterfaceSize = "huge" }, `default_interface_size "huge"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			tc.mutate(&d)
			err := d.validateInterfaceSizes()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// LoadConfig runs the check: the shipped presets load, a broken default fails
// at boot and is never published.
func TestInterfaceSizesLoadConfig(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	cfg, err := LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Display.InterfaceSizes) != 3 {
		t.Fatalf("shipped presets %+v, want three", cfg.Display.InterfaceSizes)
	}
	if size, ok := cfg.Display.InterfaceSize(cfg.Display.DefaultInterfaceSize); !ok || size.Scale != 1 {
		t.Fatalf("default preset %+v must be the native size", size)
	}
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), "default_interface_size: normal", "default_interface_size: huge", 1)
	if broken == string(data) {
		t.Fatal("setup: shipped config has no default_interface_size line")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(broken), 0600); err != nil {
		t.Fatal(err)
	}
	before := GlobalConfig
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "default_interface_size") {
		t.Fatalf("broken default loaded: %v", err)
	}
	if GlobalConfig != before {
		t.Fatal("invalid config was published")
	}
}

func TestUIFontsValidation(t *testing.T) {
	valid := func() DisplayConfig {
		return DisplayConfig{
			Fonts: []UIFont{
				{Key: "classic", Label: "Classic"},
				{Key: "romulus", Label: "Romulus", File: "assets/fonts/romulus/Romulus.ttf", SizePx: 16, Credit: "Romulus by Hewett Tsoi"},
			},
			DefaultFont: "classic",
		}
	}
	cases := []struct {
		name    string
		mutate  func(d *DisplayConfig)
		wantErr string
	}{
		{"valid", func(*DisplayConfig) {}, ""},
		{"missing list", func(d *DisplayConfig) { d.Fonts = nil }, "display.fonts is required"},
		{"empty key", func(d *DisplayConfig) { d.Fonts[1].Key = "" }, "key is required"},
		{"duplicate key", func(d *DisplayConfig) { d.Fonts[1].Key = "classic" }, `duplicate key "classic"`},
		{"empty label", func(d *DisplayConfig) { d.Fonts[1].Label = "" }, "label is required"},
		{"non-ASCII label", func(d *DisplayConfig) { d.Fonts[1].Label = "Römulus" }, "must be ASCII"},
		{"non-ASCII credit", func(d *DisplayConfig) { d.Fonts[1].Credit = "by Tsöi" }, "must be ASCII"},
		{"size on built-in", func(d *DisplayConfig) { d.Fonts[0].SizePx = 16 }, "font file only"},
		{"file without size", func(d *DisplayConfig) { d.Fonts[1].SizePx = 0 }, "must be within 6..16"},
		{"taller than a line", func(d *DisplayConfig) { d.Fonts[1].SizePx = 18 }, "must be within 6..16"},
		{"unknown default", func(d *DisplayConfig) { d.DefaultFont = "papyrus" }, `default_font "papyrus"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			tc.mutate(&d)
			err := d.resolveFonts("root")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if d.Fonts[0].Path != "" || d.Fonts[1].Path != filepath.Join("root", "assets/fonts/romulus/Romulus.ttf") {
					t.Fatalf("paths %q %q", d.Fonts[0].Path, d.Fonts[1].Path)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}
