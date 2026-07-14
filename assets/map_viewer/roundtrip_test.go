package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/world"
)

// The editor writes .map files with its own encoder (encodeMapLines) while the
// game reads them with world.MapLoader - two independent implementations of
// the same text format. This test pins their contract: every shipped map,
// re-encoded by the editor and re-parsed by the game loader, must produce the
// exact same MapData (tiles, spawns, start position). A format change on
// either side that breaks the other fails here.
func TestMapEncodeRoundTrip(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cfg, _ := boot.LoadGameData()
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatalf("load maps: %v", err)
	}
	if len(maps) == 0 {
		t.Fatal("no maps found")
	}

	tmpDir := t.TempDir()
	for _, m := range maps {
		m := m
		t.Run(m.Key, func(t *testing.T) {
			if m.Err != nil || m.Data == nil {
				t.Fatalf("shipped map failed to load: %v", m.Err)
			}
			lines, err := encodeMapLines(&m, world.GlobalTileManager)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			path := filepath.Join(tmpDir, m.Key+".map")
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			loader := world.NewMapLoaderWithBiome(cfg, m.Config.Biome)
			got, err := loader.LoadMap(path)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			compareMapData(t, m.Data, got)
		})
	}
}

func compareMapData(t *testing.T, want, got *world.MapData) {
	t.Helper()
	if got.Width != want.Width || got.Height != want.Height {
		t.Fatalf("size drifted: want %dx%d, got %dx%d", want.Width, want.Height, got.Width, got.Height)
	}
	if got.StartX != want.StartX || got.StartY != want.StartY {
		t.Errorf("start drifted: want (%d,%d), got (%d,%d)", want.StartX, want.StartY, got.StartX, got.StartY)
	}
	for y := 0; y < want.Height; y++ {
		for x := 0; x < want.Width; x++ {
			if want.Tiles[y][x] != got.Tiles[y][x] {
				t.Errorf("tile (%d,%d) drifted: want %q, got %q", x, y,
					world.GlobalTileManager.GetTileKey(want.Tiles[y][x]),
					world.GlobalTileManager.GetTileKey(got.Tiles[y][x]))
			}
		}
	}
	compareSpawns(t, "monster", spawnStrings(want.MonsterSpawns, func(s world.MonsterSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.MonsterKey)
	}), spawnStrings(got.MonsterSpawns, func(s world.MonsterSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.MonsterKey)
	}))
	compareSpawns(t, "npc", spawnStrings(want.NPCSpawns, func(s world.NPCSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.NPCKey)
	}), spawnStrings(got.NPCSpawns, func(s world.NPCSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.NPCKey)
	}))
	compareSpawns(t, "special tile", spawnStrings(want.SpecialTileSpawns, func(s world.SpecialTileSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.TileKey)
	}), spawnStrings(got.SpecialTileSpawns, func(s world.SpecialTileSpawn) string {
		return fmt.Sprintf("(%d,%d) %s", s.X, s.Y, s.TileKey)
	}))
}

func spawnStrings[T any](spawns []T, format func(T) string) []string {
	out := make([]string, len(spawns))
	for i, s := range spawns {
		out[i] = format(s)
	}
	sort.Strings(out)
	return out
}

func compareSpawns(t *testing.T, kind string, want, got []string) {
	t.Helper()
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Errorf("%s spawns drifted:\nwant:\n%s\ngot:\n%s", kind, strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}
