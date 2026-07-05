package world

import (
	"path/filepath"
	"testing"

	"ugataima/internal/monster"
)

// TestDragonCliffsMapLoads parses the real assets/dragon_cliffs.map with the
// dragon_cliffs biome and verifies it wires up: a start, the violet teleporter,
// the four area NPCs, and the 6 green + 3 gold dragons.
func TestDragonCliffsMapLoads(t *testing.T) {
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

	ml := NewMapLoaderWithBiome(nil, "dragon_cliffs")
	md, err := ml.LoadMap(filepath.Join("..", "..", "assets", "dragon_cliffs.map"))
	if err != nil {
		t.Fatalf("load dragon_cliffs.map: %v", err)
	}

	if md.Width != 45 || md.Height != 33 {
		t.Errorf("dimensions: got %dx%d, want 45x33", md.Width, md.Height)
	}
	if md.StartX < 0 {
		t.Errorf("no start position parsed (StartX=%d)", md.StartX)
	}
	for _, key := range []string{
		"dragon_cliffs_ranger", "dragon_cliffs_bone_hermit",
		"dragon_cliffs_ember_lair", "dragon_cliffs_bone_lair",
	} {
		if !hasNPCKey(md.NPCSpawns, key) {
			t.Errorf("key NPC %q missing: %+v", key, md.NPCSpawns)
		}
	}
	if len(md.SpecialTileSpawns) != 1 || md.SpecialTileSpawns[0].TileKey != "vteleporter" {
		t.Errorf("want 1 vteleporter special tile, got %+v", md.SpecialTileSpawns)
	}

	counts := map[string]int{}
	for _, ms := range md.MonsterSpawns {
		counts[ms.MonsterKey]++
	}
	if counts["dragon_green"] != 6 || counts["dragon_gold"] != 3 {
		t.Errorf("dragons: got %d green, %d gold; want 6 green, 3 gold", counts["dragon_green"], counts["dragon_gold"])
	}
	// Quest targets must actually spawn here: the ranger's Troll Cull needs ≥3
	// mountain_troll and the hermit's Ember Rites needs ≥2 archmage.
	if counts["mountain_troll"] < 3 {
		t.Errorf("mountain_troll: got %d, want >=3 for the Troll Cull quest", counts["mountain_troll"])
	}
	if counts["archmage"] < 2 {
		t.Errorf("archmage: got %d, want >=2 for the Ember Rites quest", counts["archmage"])
	}
}
