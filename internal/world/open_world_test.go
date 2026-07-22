package world

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// bootOpenWorldTest loads the real content configs and builds the unified
// world from assets/open_world.yaml. Globals are restored via t.Cleanup.
func bootOpenWorldTest(t *testing.T) (*WorldManager, *config.OpenWorldConfig) {
	t.Helper()
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	prevTM, prevWM, prevMC := GlobalTileManager, GlobalWorldManager, monster.MonsterConfig
	t.Cleanup(func() {
		GlobalTileManager, GlobalWorldManager, monster.MonsterConfig = prevTM, prevWM, prevMC
	})

	GlobalTileManager = NewTileManager()
	if err := GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	owc, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
	if err != nil {
		t.Fatalf("open world config: %v", err)
	}

	wm := NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	wm.SetOpenWorldConfig(owc)
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	GlobalWorldManager = wm
	return wm, owc
}

// TestOpenWorldStitchGeometry verifies the unified world builds from the real
// assets: regions land at their placements, every carved passage is walkable
// end to end, removed travel devices are gone, and coordinate projection
// round-trips.
func TestOpenWorldStitchGeometry(t *testing.T) {
	wm, owc := bootOpenWorldTest(t)

	if wm.OpenWorld == nil {
		t.Fatal("open world not built")
	}
	if len(wm.OpenWorldRegions) != len(owc.Placements) {
		t.Fatalf("regions = %d, placements = %d", len(wm.OpenWorldRegions), len(owc.Placements))
	}
	for key := range owc.Placements {
		if _, loaded := wm.LoadedMaps[key]; loaded {
			t.Errorf("merged map %q must not stay in LoadedMaps", key)
		}
		if !wm.IsValidMap(key) {
			t.Errorf("merged map %q must remain a valid map key", key)
		}
		if wm.WorldByKey(key) != wm.OpenWorld {
			t.Errorf("WorldByKey(%q) must resolve to the unified world", key)
		}
	}

	// Every connection must be traversable: BFS over walkable tiles from just
	// behind the from-side opening to just behind the to-side opening, using
	// the REAL placed-opening math (orientation included). Shape-agnostic, so
	// straight corridors and bent canyons validate alike.
	for ci, conn := range owc.Connections {
		fromR, toR := wm.OpenWorldRegionByKey(conn.From.Map), wm.OpenWorldRegionByKey(conn.To.Map)
		if fromR == nil || toR == nil {
			t.Fatalf("connection %d: missing regions", ci)
		}
		starts := fromR.OpeningInteriorCells(conn.From, conn.Width)
		goals := toR.OpeningInteriorCells(conn.To, conn.Width)
		for i := 0; i < conn.Width; i++ {
			if !tilesReachable(wm.OpenWorld, starts[i], goals[i]) {
				t.Errorf("connection %d (%s->%s) span %d: no walkable path from %v to %v",
					ci, conn.From.Map, conn.To.Map, i, starts[i], goals[i])
			}
		}
	}

	// The split-world travel devices are stripped: no gate NPCs, and no
	// teleporters registered under any merged region key.
	for _, npc := range wm.OpenWorld.NPCs {
		if npc.Key == "portal_gate_highlands" || npc.Key == "portal_gate_forest" {
			t.Errorf("gate NPC %q must be removed from the unified world", npc.Key)
		}
	}
	for _, tp := range wm.GlobalTeleporterRegistry.Teleporters {
		if wm.IsOpenWorldRegion(tp.MapKey) {
			t.Errorf("teleporter %q must be removed from merged map %q", tp.Label, tp.MapKey)
		}
	}

	// Monster conservation: the merged world holds exactly the authored spawns
	// of its source maps (relative check - no balance counts pinned).
	want := 0
	for key := range owc.Placements {
		mc := wm.MapConfigs[key]
		loader := NewMapLoaderWithBiome(wm.config, mc.Biome)
		data, err := loader.LoadMap("assets/" + mc.File)
		if err != nil {
			t.Fatalf("reload %q: %v", key, err)
		}
		want += len(data.MonsterSpawns)
	}
	if len(wm.OpenWorld.Monsters) != want {
		t.Errorf("merged monsters = %d, authored spawns = %d", len(wm.OpenWorld.Monsters), want)
	}

	// Clear-encounter rewards keep LOCAL coordinates (the game projects them
	// at spawn time) - the desert oasis chests must match their authored tiles.
	desertCfg := wm.MapConfigs["desert"]
	if len(desertCfg.ClearEncounters) > 0 && desertCfg.ClearEncounters[0].Rewards != nil &&
		desertCfg.ClearEncounters[0].Rewards.TreasureChest != nil {
		authored := desertCfg.ClearEncounters[0].Rewards.TreasureChest
		found := false
		for _, m := range wm.OpenWorld.Monsters {
			if m.IsEncounterMonster && m.EncounterRewards != nil && m.EncounterRewards.TreasureChest != nil &&
				m.EncounterRewards.TreasureChest.TileX == authored.TileX &&
				m.EncounterRewards.TreasureChest.TileY == authored.TileY {
				found = true
				break
			}
		}
		if !found {
			t.Error("desert clear-encounter chest lost its authored local coordinates")
		}
	}

	// Projection round-trip on a region interior point.
	ts := wm.config.GetTileSize()
	desert := wm.OpenWorldRegionByKey("desert")
	lx, ly := 10.5*ts, 20.5*ts
	gx, gy := wm.ProjectWorldPos("desert", lx, ly)
	if key, backX, backY, ok := wm.LocalizeWorldPos(gx, gy); !ok || key != "desert" || backX != lx || backY != ly {
		t.Errorf("projection round-trip: got (%q, %.1f, %.1f, %v), want (desert, %.1f, %.1f)", key, backX, backY, ok, lx, ly)
	}

	// Same round-trip through an ORIENTED region (highlands is placed rot270):
	// position and heading must survive local -> unified -> local exactly.
	if r := wm.OpenWorldRegionByKey("highlands"); r != nil && r.Orient != "" && r.Orient != "none" {
		hx, hy := wm.ProjectWorldPos("highlands", lx, ly)
		if key, backX, backY, ok := wm.LocalizeWorldPos(hx, hy); !ok || key != "highlands" ||
			math.Abs(backX-lx) > 1e-6 || math.Abs(backY-ly) > 1e-6 {
			t.Errorf("oriented round-trip: got (%q, %.1f, %.1f, %v), want (highlands, %.1f, %.1f)", key, backX, backY, ok, lx, ly)
		}
		a := 1.234
		back := wm.LocalizeAngle("highlands", wm.ProjectAngle("highlands", a))
		if math.Abs(math.Mod(back-a+3*math.Pi, 2*math.Pi)-math.Pi) > 1e-9 {
			t.Errorf("oriented angle round-trip: got %.4f, want %.4f", back, a)
		}
		// The '+' start must land on a walkable unified tile.
		if sx, sy, ok := wm.OpenWorldRegionStart("highlands"); !ok ||
			!GlobalTileManager.IsWalkable(wm.OpenWorld.Tiles[int(sy/ts)][int(sx/ts)]) {
			t.Error("oriented region start did not project onto a walkable tile")
		}
	} else {
		t.Error("expected highlands to be placed with an orientation (layout regression?)")
	}

	// A corridor position localizes to a connected region's INTERIOR (never a
	// border tile a split map would trap the party in).
	conn := owc.Connections[0]
	fromR := wm.OpenWorldRegionByKey(conn.From.Map)
	corrX := float64(fromR.OffsetX+fromR.Width) + 0.5 // first corridor column east of the from-map
	corrY := float64(fromR.OffsetY+conn.From.At) + 0.5
	key, lpx, lpy, ok := wm.LocalizeWorldPos(corrX*ts, corrY*ts)
	if !ok || (key != conn.From.Map && key != conn.To.Map) {
		t.Fatalf("corridor localize: got (%q, ok=%v)", key, ok)
	}
	r := wm.OpenWorldRegionByKey(key)
	ltx, lty := int(lpx/ts), int(lpy/ts)
	if ltx < 1 || ltx > r.Width-2 || lty < 1 || lty > r.Height-2 {
		t.Errorf("corridor snap left a border tile: local (%d,%d) on %q", ltx, lty, key)
	}

	// The unified world anchors its spawn on the default start map's '+'.
	forest := wm.OpenWorldRegionByKey("forest")
	if wm.OpenWorld.StartX != forest.StartX || wm.OpenWorld.StartY != forest.StartY {
		t.Errorf("unified start = (%d,%d), want forest region start (%d,%d)",
			wm.OpenWorld.StartX, wm.OpenWorld.StartY, forest.StartX, forest.StartY)
	}
	_ = desert
}

// tilesReachable BFS-walks the merged grid's walkable tiles from start to goal.
func tilesReachable(w *World3D, start, goal [2]int) bool {
	if GlobalTileManager == nil {
		return false
	}
	walkable := func(c [2]int) bool {
		if c[0] < 0 || c[0] >= w.Width || c[1] < 0 || c[1] >= w.Height {
			return false
		}
		return GlobalTileManager.IsWalkable(w.Tiles[c[1]][c[0]])
	}
	if !walkable(start) || !walkable(goal) {
		return false
	}
	visited := make(map[[2]int]bool, 4096)
	queue := [][2]int{start}
	visited[start] = true
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == goal {
			return true
		}
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if !visited[n] && walkable(n) {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
	return false
}

// TestOpenWorldStitchRejectsBadPlacement proves geometry validation is
// fail-fast: two placements overlapping must be a load-time error, not a
// silently broken world. (Misaligned spans are legal - they auto-route.)
func TestOpenWorldStitchRejectsBadPlacement(t *testing.T) {
	wm, owc := bootOpenWorldTest(t)
	_ = wm

	bad := *owc
	bad.Placements = make(map[string]config.OpenWorldPlacement, len(owc.Placements))
	for k, v := range owc.Placements {
		bad.Placements[k] = v
	}
	p := bad.Placements["desert"]
	p.X = bad.Placements["forest"].X + 10 // desert lands on top of the forest
	p.Y = bad.Placements["forest"].Y
	bad.Placements["desert"] = p

	wm2 := NewWorldManager(wm.config)
	if err := wm2.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	wm2.SetOpenWorldConfig(&bad)
	if err := wm2.LoadAllMaps(); err == nil {
		t.Fatal("overlapping placements must fail the stitch at load time")
	}
}

// TestOpenWorldRegionMonsterPools: each region keeps its OWN authored monster
// kinds (the region-scoped loot pool for "map" crate rolls), the merged world
// carries NO flattened union, and biomes do not share one pool.
func TestOpenWorldRegionMonsterPools(t *testing.T) {
	wm, _ := bootOpenWorldTest(t)

	if n := len(wm.OpenWorld.InitialMonsterKeys); n != 0 {
		t.Errorf("merged world carries a flattened pool of %d kinds - nothing may roll cross-biome loot", n)
	}
	for i := range wm.OpenWorldRegions {
		r := &wm.OpenWorldRegions[i]
		if len(r.InitialMonsterKeys) == 0 {
			t.Errorf("region %q has an empty monster-kind pool", r.MapKey)
		}
	}

	forest := wm.OpenWorldRegionByKey("forest")
	desert := wm.OpenWorldRegionByKey("desert")
	if forest == nil || desert == nil {
		t.Fatal("forest/desert regions missing")
	}
	shared := true
	for key := range desert.InitialMonsterKeys {
		if _, ok := forest.InitialMonsterKeys[key]; !ok {
			shared = false
			break
		}
	}
	if shared {
		t.Fatal("forest and desert resolve identical monster pools - region scoping is not in effect")
	}
}
