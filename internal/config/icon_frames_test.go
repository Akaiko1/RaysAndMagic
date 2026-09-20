package config

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
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
