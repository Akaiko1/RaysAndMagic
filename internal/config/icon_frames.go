package config

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// IconFramesConfig separates reusable frame art from the migrated icon art.
// Unlisted sources keep their legacy presentation until their art is replaced.
type IconFramesConfig struct {
	Frames map[string]string `yaml:"frames"`
	Icons  map[string]string `yaml:"icons"`
}

var GlobalIconFrames *IconFramesConfig

func LoadIconFrames(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg IconFramesConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return err
	}
	for _, style := range []string{"basic", "asian", "boss"} {
		if cfg.Frames[style] == "" {
			return fmt.Errorf("missing icon frame %q", style)
		}
	}
	for style := range cfg.Frames {
		if style != "basic" && style != "asian" && style != "boss" {
			return fmt.Errorf("unknown icon frame %q", style)
		}
	}
	for icon, style := range cfg.Icons {
		if cfg.Frames[style] == "" {
			return fmt.Errorf("icon %q has unknown frame %q", icon, style)
		}
		if _, ok := IconFrameColor(icon); !ok {
			return fmt.Errorf("icon frame references unknown content %q", icon)
		}
	}
	GlobalIconFrames = &cfg
	return nil
}

func IsUnframedIcon(name string) bool {
	return GlobalIconFrames != nil && GlobalIconFrames.Icons[name] != ""
}

// IconFrameColor resolves semantic content metadata, never an icon's pixels.
// Common, uncommon and other non-rare tiers use the default silver frame.
func IconFrameColor(icon string) (color.RGBA, bool) {
	rarity := ""
	switch {
	case strings.HasPrefix(icon, "icon_weapon_"):
		d, ok := GetWeaponDefinition(strings.TrimPrefix(icon, "icon_weapon_"))
		if !ok {
			return color.RGBA{}, false
		}
		rarity = d.Rarity
	case strings.HasPrefix(icon, "icon_item_"):
		d, ok := GetItemDefinition(strings.TrimPrefix(icon, "icon_item_"))
		if !ok {
			return color.RGBA{}, false
		}
		rarity = d.Rarity
	case strings.HasPrefix(icon, "icon_spell_"):
		d, ok := GetSpellDefinition(strings.TrimPrefix(icon, "icon_spell_"))
		if !ok {
			return color.RGBA{}, false
		}
		return SchoolRGBA(d.School), true
	case strings.HasPrefix(icon, "icon_trap_"):
		d, ok := GetTrapDefinition(strings.TrimPrefix(icon, "icon_trap_"))
		if !ok || d.Icon != icon {
			return color.RGBA{}, false
		}
		return SchoolRGBA(d.Element), true
	default:
		return color.RGBA{}, false
	}
	rarity = strings.ToLower(rarity)
	if rarity == "rare" || rarity == "legendary" {
		return RarityRGBA(rarity), true
	}
	return RaritySilver, true
}
