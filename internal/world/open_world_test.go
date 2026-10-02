package world

import (
	"math"
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// bootOpenWorldTest loads the real content configs and builds the unified
// world from assets/open_world.yaml. Globals are restored via t.Cleanup.
func bootOpenWorldTest(t *testing.T) (*WorldManager, *config.OpenWorldConfig) {
	t.Helper()
	return bootWorldManagerTest(t, true)
}

// bootWorldManagerTest boots every shipped map through LoadAllMaps, stitched
// or as classic split maps.
func bootWorldManagerTest(t *testing.T, unified bool) (*WorldManager, *config.OpenWorldConfig) {
	t.Helper()
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	// EVERY process global this boot touches is restored: NPC and spell configs
	// included, or a shuffled run would leak them and make the next build's NPC
	// population (and its ground painting) depend on test order.
	prevTM, prevWM, prevMC := GlobalTileManager, GlobalWorldManager, monster.MonsterConfig
	prevNPC, prevSpells := character.NPCConfigInstance, config.GlobalSpells
	t.Cleanup(func() {
		GlobalTileManager, GlobalWorldManager, monster.MonsterConfig = prevTM, prevWM, prevMC
		character.NPCConfigInstance, config.GlobalSpells = prevNPC, prevSpells
	})

	GlobalTileManager = NewTileManager(testTileSizeClasses())
	if err := GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	// NPCs are part of the world: ground_tile painting and the removal of
	// travel devices only happen for NPCs that actually get created, so a boot
	// without them would build a world every NPC assertion passes vacuously in.
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}

	owc, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
	if err != nil {
		t.Fatalf("open world config: %v", err)
	}

	wm := NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if unified {
		wm.SetOpenWorldConfig(owc)
	}
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

	// The split-world travel devices are stripped: no removed NPC stands in
	// its region (a removal naming an NPC the map lacks fails the boot), and
	// no teleporters register under any merged region key.
	for region, removal := range owc.Removals {
		for _, key := range removal.NPCs {
			for _, npc := range wm.OpenWorld.NPCs {
				if npc.Key == key && wm.MapKeyAt(wm.OpenWorld, int(npc.X/wm.config.GetTileSize()), int(npc.Y/wm.config.GetTileSize())) == region {
					t.Errorf("removed NPC %q still stands in region %q", key, region)
				}
			}
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

	// Every region round-trips position and heading local -> unified -> local
	// (oriented placements included), and its '+' start lands on walkable ground.
	ts := wm.config.GetTileSize()
	for i := range wm.OpenWorldRegions {
		r := &wm.OpenWorldRegions[i]
		lx, ly := (float64(r.LocalWidth/2)+0.5)*ts, (float64(r.LocalHeight/2)+0.5)*ts
		gx, gy := wm.ProjectWorldPos(r.MapKey, lx, ly)
		if key, backX, backY, ok := wm.LocalizeWorldPos(gx, gy); !ok || key != r.MapKey ||
			math.Abs(backX-lx) > 1e-6 || math.Abs(backY-ly) > 1e-6 {
			t.Errorf("%s (orient %q) round-trip: got (%q, %.1f, %.1f, %v), want (%.1f, %.1f)", r.MapKey, r.Orient, key, backX, backY, ok, lx, ly)
		}
		a := 1.234
		back := wm.LocalizeAngle(r.MapKey, wm.ProjectAngle(r.MapKey, a))
		if math.Abs(math.Mod(back-a+3*math.Pi, 2*math.Pi)-math.Pi) > 1e-9 {
			t.Errorf("%s (orient %q) angle round-trip: got %.4f, want %.4f", r.MapKey, r.Orient, back, a)
		}
		if r.StartX < 0 {
			continue
		}
		if sx, sy, ok := wm.OpenWorldRegionStart(r.MapKey); !ok ||
			!GlobalTileManager.IsWalkable(wm.OpenWorld.Tiles[int(sy/ts)][int(sx/ts)]) {
			t.Errorf("%s (orient %q) start did not project onto a walkable tile", r.MapKey, r.Orient)
		}
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

// A per-placement ground override belongs to the PLACEMENT, so it must survive
// the unified-world transform: every [npc:key@tile] on every merged map stands
// on that tile in the merged world exactly as on its standalone map.
func TestOpenWorldKeepsPerPlacementGroundOverride(t *testing.T) {
	wm, owc := bootOpenWorldTest(t)
	merged := wm.OpenWorld
	if merged == nil {
		t.Fatal("unified world missing")
	}
	checked := 0
	for i := range wm.OpenWorldRegions {
		region := wm.OpenWorldRegions[i].MapKey
		mc := wm.MapConfigs[region]
		data, err := NewMapLoaderWithBiome(wm.config, mc.Biome).LoadMap("assets/" + mc.File)
		if err != nil {
			t.Fatalf("load %s: %v", region, err)
		}
		for _, spawn := range data.NPCSpawns {
			if spawn.GroundTile == "" || slices.Contains(owc.Removals[region].NPCs, spawn.NPCKey) {
				continue // removed travel devices give their ground back
			}
			ground, ok := GlobalTileManager.GetTileTypeFromKey(spawn.GroundTile)
			if !ok {
				t.Fatalf("%s: %s@%s names an unknown tile", region, spawn.NPCKey, spawn.GroundTile)
			}
			checked++
			// An identity projection is legal (a region may sit at 0,0 unrotated),
			// so only the GROUND at the projected tile is asserted.
			tx, ty := wm.ProjectTile(region, spawn.X, spawn.Y)
			if got := merged.Tiles[ty][tx]; got != ground {
				t.Errorf("%s: unified ground under %s = %q, want %q", region, spawn.NPCKey,
					GlobalTileManager.GetTileKey(got), spawn.GroundTile)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no merged map authors a per-placement ground override (positive control)")
	}
}
