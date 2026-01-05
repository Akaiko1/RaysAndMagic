package world

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisterTeleportersFromMapData_UsesProperties(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("load special tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	vType, ok := tm.GetTileTypeFromKey("vteleporter")
	if !ok {
		t.Fatalf("vteleporter tile key not found")
	}
	rType, ok := tm.GetTileTypeFromKey("rteleporter")
	if !ok {
		t.Fatalf("rteleporter tile key not found")
	}

	tiles := [][]TileType3D{{vType, rType}}
	reg := &TeleporterRegistry{
		Teleporters:     make([]TeleporterLocation, 0),
		LastUsedByGroup: make(map[string]time.Time),
	}
	RegisterTeleportersFromMapData(nil, "forest", reg, tiles)

	if len(reg.Teleporters) != 2 {
		t.Fatalf("expected 2 teleporters, got %d", len(reg.Teleporters))
	}

	byKey := make(map[string]TeleporterLocation)
	for _, tel := range reg.Teleporters {
		byKey[tel.TileKey] = tel
	}

	violet := byKey["vteleporter"]
	if violet.Group != "violet" {
		t.Fatalf("expected vteleporter group 'violet', got %q", violet.Group)
	}
	if math.Abs(violet.CooldownSeconds-5) > 0.001 {
		t.Fatalf("expected vteleporter cooldown 5, got %v", violet.CooldownSeconds)
	}

	red := byKey["rteleporter"]
	if red.Group != "red" {
		t.Fatalf("expected rteleporter group 'red', got %q", red.Group)
	}
}
