package world

import (
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/monster"
)

// TestJapaneseCastleMapLoads parses assets/japanese_castle.map with the
// japanese_castle biome and verifies it wires up: 50x50, a start, the 7 NPCs
// (exit, old retainer, 5 sword racks), and exactly one dormant Samurai Warlord
// among the spawns. Catches unresolved tile letters / NPC keys end-to-end.
func TestJapaneseCastleMapLoads(t *testing.T) {
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

	ml := NewMapLoaderWithBiome(nil, "japanese_castle")
	md, err := ml.LoadMap(filepath.Join("..", "..", "assets", "japanese_castle.map"))
	if err != nil {
		t.Fatalf("load japanese_castle.map: %v", err)
	}
	if md.Width != 50 || md.Height != 50 {
		t.Errorf("dimensions: got %dx%d, want 50x50", md.Width, md.Height)
	}
	if md.StartX < 0 || md.StartY < 0 {
		t.Errorf("no start position parsed (%d,%d)", md.StartX, md.StartY)
	}
	if len(md.NPCSpawns) != 7 {
		t.Errorf("want 7 NPC spawns (exit, retainer, 5 racks), got %d", len(md.NPCSpawns))
	}
	racks, boss := 0, 0
	for _, n := range md.NPCSpawns {
		if strings.HasPrefix(n.NPCKey, "sword_rack_") {
			racks++
		}
	}
	for _, m := range md.MonsterSpawns {
		if m.MonsterKey == "old_samurai" {
			boss++
		}
	}
	if racks != 5 {
		t.Errorf("want 5 sword-rack NPCs, got %d", racks)
	}
	if boss != 1 {
		t.Errorf("want exactly 1 Samurai Warlord, got %d", boss)
	}
}

// Castle monsters must fit the keep's 1-wide doorways: a collision box >= tile
// size (64) spans a 2x2 footprint (half-open bounds) and can't pass a single-tile
// door, freezing the mob. Guards the same class of bug as the culverts corridors.
func TestJapaneseCastleMonstersFitDoorways(t *testing.T) {
	prevMC := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = prevMC }()

	const tile = 64.0
	found := false
	for key, def := range monster.MonsterConfig.Monsters {
		castle := false
		for _, b := range def.Biomes {
			if b == "japanese_castle" {
				castle = true
			}
		}
		if !castle {
			continue
		}
		found = true
		if def.BoxW >= tile || def.BoxH >= tile {
			t.Errorf("%s collision box %gx%g >= tile %g — can't fit 1-wide doorways", key, def.BoxW, def.BoxH, tile)
		}
	}
	if !found {
		t.Fatal("no japanese_castle-biome monsters found (config not loaded?)")
	}
}
