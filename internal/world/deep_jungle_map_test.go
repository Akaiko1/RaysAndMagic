package world

import (
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
)

// TestDeepJungleMapLoads parses assets/deep_jungle.map with the jungle biome and
// verifies it wires up end-to-end: 50x50, a landing start, exactly the one exit
// NPC (pure-wilderness zone), one Orc Warlord, and the four warding idols.
// Catches unresolved tile letters / NPC keys.
func TestDeepJungleMapLoads(t *testing.T) {
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

	ml := NewMapLoaderWithBiome(nil, "jungle")
	md, err := ml.LoadMap(filepath.Join("..", "..", "assets", "deep_jungle.map"))
	if err != nil {
		t.Fatalf("load deep_jungle.map: %v", err)
	}
	if md.Width != 50 || md.Height != 50 {
		t.Errorf("dimensions: got %dx%d, want 50x50", md.Width, md.Height)
	}
	if md.StartX < 0 || md.StartY < 0 {
		t.Errorf("no start position parsed (%d,%d)", md.StartX, md.StartY)
	}
	if !hasNPCKey(md.NPCSpawns, "deep_jungle_exit") {
		t.Errorf("deep_jungle_exit NPC missing: %+v", md.NPCSpawns)
	}
	boss, idols := 0, 0
	for _, m := range md.MonsterSpawns {
		switch m.MonsterKey {
		case "orc_hero_boss":
			boss++
		case "jungle_idol":
			idols++
		}
	}
	if boss != 1 {
		t.Errorf("want exactly 1 Orc Warlord, got %d", boss)
	}
	if idols != 4 {
		t.Errorf("want 4 warding idols, got %d", idols)
	}
}

// Jungle monsters must fit 1-wide gaps between foliage: a collision box >= tile
// size (64) spans a 2x2 footprint and can wedge. Same guard as the castle/culverts.
func TestDeepJungleMonstersFitGaps(t *testing.T) {
	prevMC := monster.MonsterConfig
	monster.MustLoadMonsterConfig(filepath.Join("..", "..", "assets", "monsters.yaml"))
	defer func() { monster.MonsterConfig = prevMC }()

	const tile = 64.0
	found := false
	for key, def := range monster.MonsterConfig.Monsters {
		jungle := false
		for _, b := range def.Biomes {
			if b == "jungle" {
				jungle = true
			}
		}
		if !jungle {
			continue
		}
		found = true
		if def.BoxW >= tile || def.BoxH >= tile {
			t.Errorf("%s collision box %gx%g >= tile %g — can wedge in 1-wide gaps", key, def.BoxW, def.BoxH, tile)
		}
	}
	if !found {
		t.Fatal("no jungle-biome monsters found (config not loaded?)")
	}
}
