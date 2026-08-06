package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLoadConfigRejectsNonPositiveTileSize(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tileSizeLine := regexp.MustCompile(`(?m)^  tile_size:.*$`)
	if matches := tileSizeLine.FindAll(data, -1); len(matches) != 1 {
		t.Fatalf("shipped config contains %d world tile_size lines, want 1", len(matches))
	}

	for _, tileSize := range []int{0, -1} {
		t.Run(fmt.Sprintf("tile_size_%d", tileSize), func(t *testing.T) {
			contents := tileSizeLine.ReplaceAllString(string(data), fmt.Sprintf("  tile_size: %d", tileSize))
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "world.tile_size must be > 0") {
				t.Fatalf("LoadConfig(tile_size=%d) error = %v, want world.tile_size validation", tileSize, err)
			}
		})
	}
}

// The retired graphics.sprite block must fail the load: silently ignoring it
// would leave a content author believing the knobs still scale billboards.
func TestLoadConfigRejectsRetiredSpriteBlock(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tree_height_multiplier") || strings.Contains(string(data), "tree_width_multiplier") {
		t.Fatal("shipped config still authors the retired graphics.sprite knobs")
	}
	if _, err := LoadConfig("../../config.yaml"); err != nil {
		t.Fatalf("shipped config no longer loads: %v", err)
	}

	stale := strings.Replace(string(data), "  # Distance effects",
		"  sprite:\n    tree_height_multiplier: 1.5\n    tree_width_multiplier: 0.8\n\n  # Distance effects", 1)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "graphics.sprite is removed") {
		t.Fatalf("LoadConfig with a stale graphics.sprite block error = %v, want a removal error", err)
	}
}
