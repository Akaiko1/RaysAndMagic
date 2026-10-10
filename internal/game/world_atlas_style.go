package game

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/world"

	"gopkg.in/yaml.v3"
)

// Terrain symbols are authored independently of the first-person render class:
// trees, dunes and boulders all use crossed standees in the world.
type worldAtlasStyle struct {
	Tiles            map[string]string `yaml:"tiles"`
	NPCs             map[string]string `yaml:"npcs"`
	BossIcon         string            `yaml:"boss_icon"`
	PortalIcon       string            `yaml:"portal_icon"`
	PropShapes       map[string]string `yaml:"prop_shapes"`
	NPCTypes         map[string]string `yaml:"npc_types"`
	RenderCategories map[string]string `yaml:"render_categories"`
	LegendIcons      map[string]string `yaml:"legend_icons"`
}

var loadedAtlasStyle worldAtlasStyle

// LoadWorldAtlas validates the shared catalog once at startup, not per game.
// The caller supplies its sprite index so validation never depends on a second CWD.
func LoadWorldAtlas(path string, spritePaths map[string]string) error {
	style, err := loadAtlasStyle(path, world.GlobalTileManager, character.NPCConfigInstance, spritePaths)
	if err == nil {
		loadedAtlasStyle = style
	}
	return err
}

func loadAtlasStyle(path string, tiles *world.TileManager, npcs *character.NPCConfig, paths map[string]string) (worldAtlasStyle, error) {
	var style worldAtlasStyle
	data, err := os.ReadFile(path)
	if err != nil {
		return style, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&style); err != nil {
		return style, fmt.Errorf("world atlas %s: %w", path, err)
	}
	if tiles == nil || style.Tiles == nil {
		return style, fmt.Errorf("world atlas %s requires tiles and a loaded tile catalog", path)
	}
	checkIcon := func(owner, icon string) error {
		if !strings.HasPrefix(icon, "atlas_") || paths[icon] == "" {
			return fmt.Errorf("world atlas %s references missing atlas sprite %q", owner, icon)
		}
		return nil
	}
	for key, icon := range style.Tiles {
		if tiles.GetTileDataByKey(key) == nil {
			return style, fmt.Errorf("world atlas %s references unknown tile %q", path, key)
		}
		if err := checkIcon("tile "+key, icon); err != nil {
			return style, err
		}
	}
	for key, icon := range style.NPCs {
		if npcs == nil || npcs.NPCs[key] == nil {
			return style, fmt.Errorf("world atlas %s references unknown NPC %q", path, key)
		}
		if err := checkIcon("NPC "+key, icon); err != nil {
			return style, err
		}
	}
	if err := checkIcon("boss_icon", style.BossIcon); err != nil {
		return style, err
	}
	if err := checkIcon("portal_icon", style.PortalIcon); err != nil {
		return style, err
	}
	for key, icon := range style.PropShapes {
		if !character.IsNPCPropShape(key) {
			return style, fmt.Errorf("unknown atlas prop shape %q", key)
		}
		if err := checkIcon("prop shape "+key, icon); err != nil {
			return style, err
		}
	}
	for key, icon := range style.NPCTypes {
		if !character.ValidNPCTypes[key] {
			return style, fmt.Errorf("unknown atlas NPC type %q", key)
		}
		if err := checkIcon("NPC type "+key, icon); err != nil {
			return style, err
		}
	}
	for key, icon := range style.RenderCategories {
		if !ValidNPCRenderCategories[key] {
			return style, fmt.Errorf("unknown atlas render category %q", key)
		}
		if err := checkIcon("render category "+key, icon); err != nil {
			return style, err
		}
	}
	for key := range ValidNPCRenderCategories {
		if style.RenderCategories[key] == "" {
			return style, fmt.Errorf("missing atlas render category %q", key)
		}
	}
	for _, key := range []string{"town", "people", "objects"} {
		if err := checkIcon("legend "+key, style.LegendIcons[key]); err != nil {
			return style, err
		}
	}
	for key := range style.LegendIcons {
		if key != "town" && key != "people" && key != "objects" {
			return style, fmt.Errorf("unknown atlas legend %q", key)
		}
	}
	return style, nil
}
