package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapLoader_SpecialTileByKey(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("load special tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	mapDir := t.TempDir()
	mapPath := filepath.Join(mapDir, "test.map")
	content := "@..  >[stile:spike_trap]\n"
	if err := os.WriteFile(mapPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}

	ml := NewMapLoaderWithBiome(nil, "forest")
	mapData, err := ml.LoadMap(mapPath)
	if err != nil {
		t.Fatalf("load map: %v", err)
	}

	trapType, ok := tm.GetTileTypeFromKey("spike_trap")
	if !ok {
		t.Fatalf("spike_trap tile key not found")
	}
	if got := mapData.Tiles[0][0]; got != trapType {
		t.Fatalf("expected spike_trap at (0,0), got %v", got)
	}
}
