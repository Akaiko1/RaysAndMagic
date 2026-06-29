package world

import (
	"os"
	"path/filepath"
	"testing"
)

// hasNPCKey reports whether a map spawned an NPC with the given key. Map tests
// assert key NPCs are PRESENT by name rather than counting spawns, so adding
// service NPCs (taverns, etc.) doesn't break unrelated tests.
func hasNPCKey(spawns []NPCSpawn, key string) bool {
	for _, s := range spawns {
		if s.NPCKey == key {
			return true
		}
	}
	return false
}

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

// TestMapLoader_UnderEntityFloorDominantNeighbour: the auto-default floor under a
// placed entity ('@') matches the dominant FLOOR variant around it, not the bare
// biome '.' default — and stays the '.' default when neighbours are uniform.
func TestMapLoader_UnderEntityFloorDominantNeighbour(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	// japanese_castle has two walkable floor variants: '.'=cobble (default), ','=wood.
	wood, ok := tm.GetTileTypeFromLetterForBiome(",", "japanese_castle")
	if !ok {
		t.Fatalf("wood floor tile not found")
	}
	cobble, ok := tm.GetTileTypeFromLetterForBiome(".", "japanese_castle")
	if !ok {
		t.Fatalf("cobble floor tile not found")
	}
	if wood == cobble {
		t.Fatalf("test needs two distinct floor variants")
	}

	load := func(t *testing.T, content string) *MapData {
		t.Helper()
		p := filepath.Join(t.TempDir(), "t.map")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write map: %v", err)
		}
		ml := NewMapLoaderWithBiome(nil, "japanese_castle")
		md, err := ml.LoadMap(p)
		if err != nil {
			t.Fatalf("load map: %v", err)
		}
		return md
	}

	// '@' ringed by wood → under-tile becomes wood (dominant), not the '.' cobble default.
	md := load(t, ",,,\n,@,\n,,,\n")
	if got := md.Tiles[1][1]; got != wood {
		t.Fatalf("under-entity tile = %v, want dominant wood %v", got, wood)
	}

	// '@' ringed by the default '.' floor → unchanged (cobble).
	md = load(t, "...\n.@.\n...\n")
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want default cobble %v", got, cobble)
	}

	// 'W' (water) is render_type "floor_only" but NOT walkable: it must never be
	// voted as floor. '@' ringed only by water → no floor neighbour → biome '.'
	// fallback (cobble), never the impassable water tile.
	water, ok := tm.GetTileTypeFromLetterForBiome("W", "japanese_castle")
	if !ok {
		t.Fatalf("water tile not found")
	}
	md = load(t, "WWW\nW@W\nWWW\n")
	if got := md.Tiles[1][1]; got == water {
		t.Fatalf("under-entity tile became impassable water %v", water)
	}
	if got := md.Tiles[1][1]; got != cobble {
		t.Fatalf("under-entity tile = %v, want '.' fallback cobble %v (water excluded)", got, cobble)
	}
}
