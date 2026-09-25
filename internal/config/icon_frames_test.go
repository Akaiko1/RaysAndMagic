package config

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/assetmanifest"
)

func TestIconFrameSemanticColours(t *testing.T) {
	oldItems, oldWeapons, oldSpells, oldTraps := GlobalItems, GlobalWeapons, GlobalSpells, GlobalTrapConfig
	t.Cleanup(func() {
		GlobalItems, GlobalWeapons, GlobalSpells, GlobalTrapConfig = oldItems, oldWeapons, oldSpells, oldTraps
	})
	GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": {}}}
	GlobalWeapons = &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": {}}}
	GlobalSpells = &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"test": {}}}
	GlobalTrapConfig = &TrapSystemConfig{Traps: map[string]*TrapDefinitionConfig{"test": {Icon: "icon_trap_test"}}}
	for _, kind := range []string{"item", "weapon"} {
		for _, tc := range []struct {
			rarity string
			want   color.RGBA
		}{{"", RaritySilver}, {"common", RaritySilver}, {"uncommon", RaritySilver}, {"rare", RarityGold}, {"legendary", RarityLegendary}, {"unique", RaritySilver}, {"Rare", RarityGold}, {"LEGENDARY", RarityLegendary}} {
			t.Run(kind+"/"+tc.rarity, func(t *testing.T) {
				GlobalItems.Items["test"].Rarity = tc.rarity
				GlobalWeapons.Weapons["test"].Rarity = tc.rarity
				got, ok := IconFrameColor("icon_" + kind + "_test")
				if !ok || got != tc.want {
					t.Fatalf("got %v/%v, want %v", got, ok, tc.want)
				}
			})
		}
	}
	for _, school := range []string{"physical", "fire", "air", "water", "earth", "body", "mind", "spirit", "light", "dark"} {
		for _, kind := range []string{"spell", "trap"} {
			t.Run(kind+"/"+school, func(t *testing.T) {
				GlobalSpells.Spells["test"].School = school
				GlobalTrapConfig.Traps["test"].Element = school
				got, ok := IconFrameColor("icon_" + kind + "_test")
				if !ok || got != SchoolRGBA(school) {
					t.Fatalf("school colour drift: %v", got)
				}
			})
		}
	}
	for _, unknown := range []string{"portrait_test", "icon_item_missing", "icon_spell_missing", "icon_weapon_missing", "icon_trap_missing"} {
		if _, ok := IconFrameColor(unknown); ok {
			t.Fatalf("accepted %s", unknown)
		}
	}
}

func TestIconFrameCatalogValidation(t *testing.T) {
	oldItems, oldFrames := GlobalItems, GlobalIconFrames
	t.Cleanup(func() { GlobalItems, GlobalIconFrames = oldItems, oldFrames })
	GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": {}}}
	for _, tc := range []struct {
		name, frames, icons string
		valid               bool
	}{
		{"valid", "{basic: b, asian: a, boss: z}", "{icon_item_test: asian}", true},
		{"missing_style", "{basic: b, asian: a}", "{}", false},
		{"unknown_style", "{basic: b, asian: a, boss: z, extra: q}", "{}", false},
		{"unknown_icon", "{basic: b, asian: a, boss: z}", "{icon_item_missing: basic}", false},
		{"bad_assignment", "{basic: b, asian: a, boss: z}", "{icon_item_test: unknown}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for _, path := range []string{"b", "a", "z", "assets/sprites/interface/nested/icon_item_test.png"} {
				writeIconTestPNG(t, path, 128, 128)
			}
			previous := GlobalIconFrames
			p := filepath.Join(t.TempDir(), "frames.yaml")
			os.WriteFile(p, []byte(fmt.Sprintf("frames: %s\nicons: %s\n", tc.frames, tc.icons)), 0600)
			err := LoadIconFrames(p)
			if (err == nil) != tc.valid {
				t.Fatalf("validation: %v", err)
			}
			if err != nil && GlobalIconFrames != previous {
				t.Fatal("invalid catalog partially published")
			}
		})
	}
}

func writeIconTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, width, height)))
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestIconFrameSourceValidationAtLoad(t *testing.T) {
	oldItems, oldFrames := GlobalItems, GlobalIconFrames
	t.Cleanup(func() { GlobalItems, GlobalIconFrames = oldItems, oldFrames })
	GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": {}}}
	for _, role := range []string{"frame", "icon"} {
		for _, state := range []string{"valid", "narrow", "short", "large", "missing", "corrupt"} {
			t.Run(role+"/"+state, func(t *testing.T) {
				t.Chdir(t.TempDir())
				frame, art := "mask.png", "assets/sprites/interface/nested/icon_item_test.png"
				writeIconTestPNG(t, frame, 128, 128)
				writeIconTestPNG(t, art, 128, 128)
				path := art
				if role == "frame" {
					path = frame
				}
				switch state {
				case "narrow":
					writeIconTestPNG(t, path, 64, 128)
				case "short":
					writeIconTestPNG(t, path, 128, 64)
				case "large":
					writeIconTestPNG(t, path, 256, 256)
				case "missing":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "corrupt":
					if err := os.WriteFile(path, []byte("bad PNG"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile("frames.yaml", []byte("frames: {basic: mask.png, asian: mask.png, boss: mask.png}\nicons: {icon_item_test: basic}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				previous := GlobalIconFrames
				err := LoadIconFrames("frames.yaml")
				if (err == nil) != (state == "valid") {
					t.Fatalf("load-time validation: %v", err)
				}
				if err != nil {
					if GlobalIconFrames != previous {
						t.Fatal("failed load published partial catalog")
					}
					label := "icon_item_test"
					if role == "frame" {
						label = "icon frame"
					}
					if !strings.Contains(err.Error(), label) {
						t.Fatalf("error lacks source identity: %v", err)
					}
				}
			})
		}
	}
}

func TestIconValidationUsesRuntimeSpriteResolution(t *testing.T) {
	oldItems, oldFrames := GlobalItems, GlobalIconFrames
	t.Cleanup(func() { GlobalItems, GlobalIconFrames = oldItems, oldFrames })
	GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": {}}}
	for _, state := range []string{"ignored_only", "legacy_first", "shipped_override"} {
		t.Run(state, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeIconTestPNG(t, "mask.png", 128, 128)
			for _, dir := range []string{"archive", "_old", ".hidden"} {
				writeIconTestPNG(t, "assets/sprites/interface/"+dir+"/icon_item_test.png", 128, 128)
			}
			if state != "ignored_only" {
				writeIconTestPNG(t, "assets/sprites/interface/a/icon_item_test.png", 64, 64)
				writeIconTestPNG(t, "assets/sprites/interface/z/icon_item_test.png", 128, 128)
			}
			if state == "shipped_override" {
				if err := (assetmanifest.Manifest{"sprites/interface/z/icon_item_test.png": "current"}).Save("."); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile("frames.yaml", []byte("frames: {basic: mask.png, asian: mask.png, boss: mask.png}\nicons: {icon_item_test: basic}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			err := LoadIconFrames("frames.yaml")
			if (err == nil) != (state == "shipped_override") {
				t.Fatalf("runtime source policy: %v", err)
			}
		})
	}
}
