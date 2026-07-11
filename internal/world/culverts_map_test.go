package world

import (
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
)

// TestCulvertsMapLoads parses the generated assets/culverts.map with the culverts
// biome and verifies it wires up: 50x50, a start, the 9 NPCs (exit, old man, 7
// valves), and a Golden Thief Bug boss among the spawns.
func TestCulvertsMapLoads(t *testing.T) {
	tm := NewTileManager()
	if err := tm.LoadTileConfig(filepath.Join("..", "..", "assets", "tiles.yaml")); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig(filepath.Join("..", "..", "assets", "special_tiles.yaml")); err != nil {
		t.Fatalf("load special tiles: %v", err)
	}
	GlobalTileManager = tm
	defer func() { GlobalTileManager = nil }()

	prevMC := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = prevMC }()

	ml := NewMapLoaderWithBiome(nil, "culverts")
	md, err := ml.LoadMap(filepath.Join("..", "..", "assets", "culverts.map"))
	if err != nil {
		t.Fatalf("load culverts.map: %v", err)
	}
	if md.Width != 50 || md.Height != 50 {
		t.Errorf("dimensions: got %dx%d, want 50x50", md.Width, md.Height)
	}
	if md.StartX < 0 || md.StartY < 0 {
		t.Errorf("no start position parsed (%d,%d)", md.StartX, md.StartY)
	}
	// Only the QUEST-REQUIRED NPCs are asserted (exit, old man, exactly 7
	// valves) - decorative extras (loot crates etc.) come and go with map
	// editing and must not break this test.
	valves, boss, exits, oldmen := 0, 0, 0, 0
	for _, n := range md.NPCSpawns {
		switch {
		case len(n.NPCKey) >= 12 && n.NPCKey[:12] == "culvert_valv":
			valves++
		case n.NPCKey == "culverts_exit":
			exits++
		case n.NPCKey == "culverts_oldman":
			oldmen++
		}
	}
	for _, m := range md.MonsterSpawns {
		if m.MonsterKey == "golden_thief_bug" {
			boss++
		}
	}
	if valves != 7 {
		t.Errorf("want 7 valve NPCs, got %d", valves)
	}
	if exits != 1 || oldmen != 1 {
		t.Errorf("want 1 exit + 1 old man, got %d/%d", exits, oldmen)
	}
	if boss != 1 {
		t.Errorf("want exactly 1 Golden Thief Bug, got %d", boss)
	}
}

// Every culverts-biome monster's collision box must be smaller than a tile (64):
// a box >= tile size spans a 2x2 footprint (half-open bounds) and can't traverse
// the maze's 1-wide corridors, freezing the monster. Guards the Golden Thief Bug
// (and any future culvert mob) against the "stands frozen, attacks only point
// blank" bug.
func TestCulvertsMonstersFitCorridors(t *testing.T) {
	prevMC := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = prevMC }()

	const tile = 64.0
	found := false
	for key, def := range monster.MonsterConfig.Monsters {
		culvert := false
		for _, b := range def.Biomes {
			if b == "culverts" {
				culvert = true
			}
		}
		if !culvert {
			continue
		}
		found = true
		if def.BoxW >= tile || def.BoxH >= tile {
			t.Errorf("%s collision box %gx%g >= tile %g - can't fit 1-wide maze corridors", key, def.BoxW, def.BoxH, tile)
		}
	}
	if !found {
		t.Fatal("no culverts-biome monsters found (config not loaded?)")
	}
}
