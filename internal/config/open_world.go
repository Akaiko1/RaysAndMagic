package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// OpenWorldConfig is the data-driven rule set for stitching the outdoor maps
// into one unified world (assets/open_world.yaml). The source .map files stay
// untouched for the editor; every unified-world difference (carved openings,
// corridors, removed portals/teleporters) is authored here and applied at
// load time by the world stitcher.
type OpenWorldConfig struct {
	// VoidTile fills the space between placed maps (must be a solid tile key).
	VoidTile string                  `yaml:"void_tile"`
	Corridor OpenWorldCorridorConfig `yaml:"corridor"`
	// Placements pin each merged map's origin in unified tile coordinates.
	// Explicit offsets (not auto-layout): geometry stays predictable and the
	// stitcher fail-fast validates overlaps and corridor alignment instead.
	Placements  map[string]OpenWorldPlacement `yaml:"placements"`
	Connections []OpenWorldConnection         `yaml:"connections"`
	// Removals strip split-world travel devices (gate NPCs, teleporter tiles)
	// that the carved passages replace. Keys are per merged map.
	Removals map[string]OpenWorldRemoval `yaml:"removals"`
}

type OpenWorldCorridorConfig struct {
	// Length is the gap in tiles between two connected map borders. Placements
	// must leave exactly this gap along every connection (validated).
	Length int `yaml:"length"`
	// FloorTile paves corridor cells (default "empty" - renders with the
	// attributed neighbour region's biome floor).
	FloorTile string `yaml:"floor_tile"`
}

type OpenWorldPlacement struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
	// Orient places the map rotated/mirrored in the unified world:
	// "" | none | rot90 | rot180 | rot270 (clockwise) | mirror_x | mirror_y.
	// The map's LOCAL coordinates (saves, quests, encounters) are unaffected -
	// the projection layer applies the transform both ways.
	Orient string `yaml:"orient,omitempty"`
}

type OpenWorldConnection struct {
	From  OpenWorldPortalSide `yaml:"from"`
	To    OpenWorldPortalSide `yaml:"to"`
	Width int                 `yaml:"width"`
	// FloorTile overrides the global corridor floor for this connection.
	FloorTile string `yaml:"floor_tile,omitempty"`
}

type OpenWorldPortalSide struct {
	Map  string `yaml:"map"`
	Edge string `yaml:"edge"` // north|south|east|west
	// At is the span start along the edge in the map's local tiles (x for
	// north/south, y for east/west). The opening covers [At, At+Width).
	At int `yaml:"at"`
	// Depth is how many wall layers to pierce inward from the border (default
	// 1; e.g. the jungle's foliage+cliff double ring needs 2).
	Depth int `yaml:"depth,omitempty"`
}

type OpenWorldRemoval struct {
	NPCs         []string `yaml:"npcs,omitempty"`
	SpecialTiles []string `yaml:"special_tiles,omitempty"`
}

// GlobalOpenWorldConfig is set at boot when the unified-world flag is on.
var GlobalOpenWorldConfig *OpenWorldConfig

var validOpenWorldEdges = map[string]bool{"north": true, "south": true, "east": true, "west": true}

// OpposingEdge returns the edge facing the given one (north<->south,
// east<->west). Opposing pairs get a straight corridor; any other pair is
// joined by an auto-routed bent corridor in the world stitcher.
func OpposingEdge(edge string) string {
	switch edge {
	case "north":
		return "south"
	case "south":
		return "north"
	case "east":
		return "west"
	case "west":
		return "east"
	}
	return ""
}

// LoadOpenWorldConfig reads and structurally validates open_world.yaml.
// Geometry (overlaps, corridor alignment, opening walkability) needs loaded
// maps and is validated by the world stitcher.
func LoadOpenWorldConfig(path string) (*OpenWorldConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read open world config: %w", err)
	}
	var cfg OpenWorldConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse open world config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("open world config: %w", err)
	}
	return &cfg, nil
}

// MustLoadOpenWorldConfig is LoadOpenWorldConfig with a panic on error and
// sets GlobalOpenWorldConfig.
func MustLoadOpenWorldConfig(path string) *OpenWorldConfig {
	cfg, err := LoadOpenWorldConfig(path)
	if err != nil {
		panic(err)
	}
	GlobalOpenWorldConfig = cfg
	return cfg
}

func (c *OpenWorldConfig) validate() error {
	if c.VoidTile == "" {
		c.VoidTile = "oob_cliff"
	}
	if c.Corridor.Length <= 0 {
		c.Corridor.Length = 2
	}
	if c.Corridor.FloorTile == "" {
		c.Corridor.FloorTile = "empty"
	}
	if len(c.Placements) == 0 {
		return fmt.Errorf("no placements defined")
	}
	validOrients := map[string]bool{"": true, "none": true, "rot90": true, "rot180": true, "rot270": true, "mirror_x": true, "mirror_y": true}
	for key, p := range c.Placements {
		if p.X < 0 || p.Y < 0 {
			return fmt.Errorf("placement %q has negative offset (%d,%d)", key, p.X, p.Y)
		}
		if !validOrients[p.Orient] {
			return fmt.Errorf("placement %q: invalid orient %q (none|rot90|rot180|rot270|mirror_x|mirror_y)", key, p.Orient)
		}
	}
	for i := range c.Connections {
		conn := &c.Connections[i]
		if conn.Width <= 0 {
			return fmt.Errorf("connection %d (%s->%s): width must be >= 1", i, conn.From.Map, conn.To.Map)
		}
		for _, side := range []*OpenWorldPortalSide{&conn.From, &conn.To} {
			if _, ok := c.Placements[side.Map]; !ok {
				return fmt.Errorf("connection %d references map %q with no placement", i, side.Map)
			}
			if !validOpenWorldEdges[side.Edge] {
				return fmt.Errorf("connection %d map %q: invalid edge %q (north|south|east|west)", i, side.Map, side.Edge)
			}
			if side.Depth == 0 {
				side.Depth = 1
			}
			if side.Depth < 0 {
				return fmt.Errorf("connection %d map %q: negative depth", i, side.Map)
			}
			if side.At < 0 {
				return fmt.Errorf("connection %d map %q: negative at", i, side.Map)
			}
		}
		if conn.From.Map == conn.To.Map {
			return fmt.Errorf("connection %d: cannot connect map %q to itself", i, conn.From.Map)
		}
		// Edges need not face each other: non-opposing pairs are joined by an
		// auto-routed bent corridor (see the world stitcher).
	}
	for key := range c.Removals {
		if _, ok := c.Placements[key]; !ok {
			return fmt.Errorf("removals reference map %q with no placement", key)
		}
	}
	return nil
}
