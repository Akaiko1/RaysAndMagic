package game

import (
	"math"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// bootOpenWorldGame loads the full real content set (mirrors main.go) and
// returns a headless game on the requested world mode. Globals restore via
// t.Cleanup.
func bootOpenWorldGame(t *testing.T, unified bool) (*MMGame, *world.WorldManager, *config.Config) {
	t.Helper()

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	if _, err := config.LoadLootTables("assets/loots.yaml"); err != nil {
		t.Fatalf("loots: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	t.Cleanup(func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	})
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatalf("traps: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	if _, err := config.LoadChampionConfig("assets/champions.yaml"); err != nil {
		t.Fatalf("champions: %v", err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("assets/level_up.yaml"); err != nil {
		t.Fatalf("level-up: %v", err)
	}
	monster.MustLoadHatesConfig("assets/hates.yaml")
	questConfig, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	quests.GlobalQuestManager = quests.NewQuestManager(questConfig)
	quests.GlobalQuestManager.InitializeStartingQuests()

	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if unified {
		owc, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
		if err != nil {
			t.Fatalf("open world config: %v", err)
		}
		wm.SetOpenWorldConfig(owc)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	world.GlobalWorldManager = wm

	g := NewMMGame(cfg)
	t.Cleanup(g.Shutdown)
	g.appScreen = AppScreenInGame
	return g, wm, cfg
}

// TestOpenWorldSaveRoundTrip proves the "local canon" save contract both
// ways: a save written on the unified world is keyed by REGION with map-local
// coordinates, loads back into the unified world at the same spot, loads into
// SPLIT mode on the right map at the same local spot, and a split save loads
// back into the unified world - no converter step, no format change.
func TestOpenWorldSaveRoundTrip(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := cfg.GetTileSize()

	// Park the party in the highlands region and let region tracking catch up.
	startX, startY, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("highlands region has no start")
	}
	g.camera.X, g.camera.Y = startX, startY
	g.syncOpenWorldRegion()
	if wm.CurrentMapKey != "highlands" {
		t.Fatalf("region tracking: CurrentMapKey = %q, want highlands", wm.CurrentMapKey)
	}

	save := g.buildSave(wm)

	// Save format: region key + map-local coordinates, no merged-world keys.
	if save.MapKey != "highlands" {
		t.Fatalf("save.MapKey = %q, want highlands", save.MapKey)
	}
	// Local canon: the saved position is map-local - projecting it through the
	// current layout (offset AND orientation) must land exactly back on the
	// runtime position.
	if px, py := wm.ProjectWorldPos("highlands", save.PlayerX, save.PlayerY); math.Abs(px-startX) > 0.01 || math.Abs(py-startY) > 0.01 {
		t.Fatalf("saved local (%.1f,%.1f) projects to (%.1f,%.1f), want (%.1f,%.1f)",
			save.PlayerX, save.PlayerY, px, py, startX, startY)
	}
	highlands := wm.OpenWorldRegionByKey("highlands")
	if save.PlayerX < 0 || save.PlayerY < 0 ||
		save.PlayerX > float64(highlands.LocalWidth)*ts || save.PlayerY > float64(highlands.LocalHeight)*ts {
		t.Fatalf("saved player (%.1f,%.1f) outside highlands local bounds", save.PlayerX, save.PlayerY)
	}
	wantLX, wantLY := save.PlayerX, save.PlayerY
	if _, ok := save.MapMonsters[world.OpenWorldKey]; ok {
		t.Fatal("save must never contain the merged-world key")
	}
	for _, regionKey := range []string{"forest", "desert", "highlands", "dragon_cliffs", "deep_jungle"} {
		bucket, ok := save.MapMonsters[regionKey]
		if !ok || len(bucket) == 0 {
			t.Fatalf("region %q missing from MapMonsters", regionKey)
		}
		r := wm.OpenWorldRegionByKey(regionKey)
		for _, ms := range bucket {
			if ms.X < 0 || ms.Y < 0 || ms.X > float64(r.LocalWidth)*ts || ms.Y > float64(r.LocalHeight)*ts {
				t.Fatalf("region %q monster %q at (%.0f,%.0f) is outside local bounds", regionKey, ms.Key, ms.X, ms.Y)
			}
		}
	}
	unifiedTotal := len(wm.OpenWorld.Monsters)

	// Reload into the SAME unified world: player returns to the global spot,
	// the roster survives. Force the current key AWAY from the save's region
	// first so the load exercises the real SwitchToMap path (a fresh boot
	// starts on "forest" while the save may point at any region).
	if err := wm.SwitchToMap("forest"); err != nil {
		t.Fatalf("switch to forest region: %v", err)
	}
	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("unified reload: %v", err)
	}
	if wm.CurrentMapKey != "highlands" {
		t.Fatalf("unified reload map = %q, want highlands", wm.CurrentMapKey)
	}
	if math.Abs(g.camera.X-startX) > 0.01 || math.Abs(g.camera.Y-startY) > 0.01 {
		t.Fatalf("unified reload player = (%.1f,%.1f), want (%.1f,%.1f)", g.camera.X, g.camera.Y, startX, startY)
	}
	if got := len(wm.OpenWorld.Monsters); got != unifiedTotal {
		t.Fatalf("unified reload monsters = %d, want %d", got, unifiedTotal)
	}

	// Load the SAME save into SPLIT mode: the party stands on the highlands
	// map at the local coordinates, the forest map holds exactly its bucket.
	gSplit, wmSplit, _ := bootOpenWorldGame(t, false)
	if err := gSplit.applySave(wmSplit, &save); err != nil {
		t.Fatalf("split load: %v", err)
	}
	if wmSplit.CurrentMapKey != "highlands" {
		t.Fatalf("split load map = %q, want highlands", wmSplit.CurrentMapKey)
	}
	if math.Abs(gSplit.camera.X-wantLX) > 0.01 || math.Abs(gSplit.camera.Y-wantLY) > 0.01 {
		t.Fatalf("split load player = (%.1f,%.1f), want (%.1f,%.1f)", gSplit.camera.X, gSplit.camera.Y, wantLX, wantLY)
	}
	if got, want := len(wmSplit.LoadedMaps["forest"].Monsters), len(save.MapMonsters["forest"]); got != want {
		t.Fatalf("split forest monsters = %d, want %d", got, want)
	}

	// And back: a save written in SPLIT mode loads into the unified world at
	// the projected position.
	splitSave := gSplit.buildSave(wmSplit)
	if splitSave.MapKey != "highlands" || splitSave.PlayerX != wantLX || splitSave.PlayerY != wantLY {
		t.Fatalf("split save = (%q, %.1f, %.1f), want (highlands, %.1f, %.1f)",
			splitSave.MapKey, splitSave.PlayerX, splitSave.PlayerY, wantLX, wantLY)
	}
	world.GlobalWorldManager = wm
	if err := g.applySave(wm, &splitSave); err != nil {
		t.Fatalf("unified load of split save: %v", err)
	}
	if math.Abs(g.camera.X-startX) > 0.01 || math.Abs(g.camera.Y-startY) > 0.01 {
		t.Fatalf("unified load of split save player = (%.1f,%.1f), want (%.1f,%.1f)", g.camera.X, g.camera.Y, startX, startY)
	}
}

// TestOpenWorldTavernRegionScoping: Town Portal must resolve "this map's
// tavern" per REGION on the unified world - each merged map owns exactly its
// own tavern, never the first tavern of the combined NPC list.
func TestOpenWorldTavernRegionScoping(t *testing.T) {
	t.Chdir("../..")

	g, wm, _ := bootOpenWorldGame(t, true)
	for _, key := range []string{"forest", "desert", "highlands", "dragon_cliffs", "deep_jungle"} {
		count := 0
		for _, npc := range wm.OpenWorld.NPCs {
			if npcOffersTavernRest(npc) && g.npcOnMapRegion(npc, key) {
				count++
			}
		}
		if count != 1 {
			t.Errorf("region %q resolves %d taverns, want exactly its own 1", key, count)
		}
	}

	// A rotated region's multi-tile facade must rotate its span direction too
	// (highlands is rot270: the clock tower's authored east span turns north).
	for _, npc := range wm.OpenWorld.NPCs {
		if npc.Key != "clock_tower_entrance" {
			continue
		}
		if r := wm.OpenWorldRegionByKey("highlands"); r != nil && r.Orient == "rot270" {
			if npc.GridSpanDir != "n" {
				t.Errorf("clock tower span dir = %q under rot270, want n", npc.GridSpanDir)
			}
		}
	}
}

// TestOpenWorldClearedRegionStaysCleared: a fully cleared region must
// serialize as an EMPTY bucket. A missing key means "legacy save, keep the
// fresh authored roster" to the loader, so absence would resurrect every
// monster the party permanently killed.
func TestOpenWorldClearedRegionStaysCleared(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := cfg.GetTileSize()
	regionOf := func(m *monster.Monster3D) string {
		if r := wm.OpenWorldRegionAtTile(int(m.X/ts), int(m.Y/ts)); r != nil {
			return r.MapKey
		}
		return ""
	}

	// The party wipes the desert clean.
	kept := wm.OpenWorld.Monsters[:0]
	removed := 0
	for _, m := range wm.OpenWorld.Monsters {
		if m != nil && regionOf(m) == "desert" {
			removed++
			continue
		}
		kept = append(kept, m)
	}
	wm.OpenWorld.Monsters = kept
	if removed == 0 {
		t.Fatal("desert region has no authored monsters to clear")
	}

	save := g.buildSave(wm)
	bucket, ok := save.MapMonsters["desert"]
	if !ok {
		t.Fatal("cleared desert region missing from MapMonsters - loader would refill it as a legacy save")
	}
	if len(bucket) != 0 {
		t.Fatalf("cleared desert bucket holds %d monsters, want 0", len(bucket))
	}

	forestBefore := len(save.MapMonsters["forest"])
	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("reload: %v", err)
	}
	desertAfter, forestAfter := 0, 0
	for _, m := range wm.OpenWorld.Monsters {
		if m == nil {
			continue
		}
		switch regionOf(m) {
		case "desert":
			desertAfter++
		case "forest":
			forestAfter++
		}
	}
	if desertAfter != 0 {
		t.Fatalf("cleared desert region refilled with %d monsters after reload", desertAfter)
	}
	if forestAfter != forestBefore {
		t.Fatalf("forest roster changed on reload: %d, want %d", forestAfter, forestBefore)
	}
}

// TestOpenWorldRegionCrossCrumblesBoundAllies: crossing a region seam IS a
// map departure - a bound undead crumbles and pays its XP, a card ally
// vanishes yielding nothing, and a charmed (pacified) monster stays.
func TestOpenWorldRegionCrossCrumblesBoundAllies(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)

	fx, fy, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("forest region has no start")
	}
	g.camera.X, g.camera.Y = fx, fy
	g.syncOpenWorldRegion()
	if wm.CurrentMapKey != "forest" {
		t.Fatalf("CurrentMapKey = %q, want forest", wm.CurrentMapKey)
	}

	bound := monster.NewMonster3DFromConfig(fx+64, fy, "skeleton", cfg)
	ally := monster.NewMonster3DFromConfig(fx-64, fy, "wolf", cfg)
	charmed := monster.NewMonster3DFromConfig(fx, fy+64, "goblin", cfg)
	if bound == nil || ally == nil || charmed == nil {
		t.Fatal("failed to spawn test monsters")
	}
	bound.Bound = true // spell-bound former enemy: crumbles WITH an XP payout
	markCardAlly(ally) // pure summon: crumbles yielding nothing
	charmed.Pacified = true
	wm.OpenWorld.Monsters = append(wm.OpenWorld.Monsters, bound, ally, charmed)

	xpBefore := 0
	for _, m := range g.party.Members {
		xpBefore += m.Experience
	}

	hx, hy, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("highlands region has no start")
	}
	g.camera.X, g.camera.Y = hx, hy
	g.syncOpenWorldRegion()
	if wm.CurrentMapKey != "highlands" {
		t.Fatalf("CurrentMapKey = %q, want highlands", wm.CurrentMapKey)
	}

	alive := map[*monster.Monster3D]bool{}
	for _, m := range wm.OpenWorld.Monsters {
		alive[m] = true
	}
	if alive[bound] {
		t.Error("bound undead survived the region cross")
	}
	if alive[ally] {
		t.Error("card ally survived the region cross")
	}
	if !alive[charmed] {
		t.Error("charmed monster was removed on region cross - it must stay")
	}
	xpAfter := 0
	for _, m := range g.party.Members {
		xpAfter += m.Experience
	}
	if xpAfter <= xpBefore {
		t.Error("bound undead crumbled without its XP payout")
	}
}

// TestOpenWorldInfernoRegionScoped: a MapWide nova on the unified world burns
// the party's REGION only - a monster across the seam takes nothing.
func TestOpenWorldInfernoRegionScoped(t *testing.T) {
	t.Chdir("../..")

	g, wm, cfg := bootOpenWorldGame(t, true)

	fx, fy, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("forest region has no start")
	}
	g.camera.X, g.camera.Y = fx, fy
	g.syncOpenWorldRegion()

	hx, hy, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("highlands region has no start")
	}
	near := monster.NewMonster3DFromConfig(fx+128, fy, "goblin", cfg)
	far := monster.NewMonster3DFromConfig(hx, hy, "goblin", cfg)
	if near == nil || far == nil {
		t.Fatal("failed to spawn test monsters")
	}
	wm.OpenWorld.Monsters = append(wm.OpenWorld.Monsters, near, far)
	nearBefore, farBefore := near.HitPoints, far.HitPoints

	def := spells.SpellDefinition{Name: "Test Nova", School: "fire", SpellPointsCost: 20, MapWide: true}
	if !g.combat.tryCastInferno(def) {
		t.Fatal("MapWide nova was not handled")
	}
	if near.HitPoints >= nearBefore {
		t.Errorf("same-region monster untouched by MapWide nova (HP %d -> %d)", nearBefore, near.HitPoints)
	}
	if far.HitPoints != farBefore {
		t.Errorf("cross-region monster burned by MapWide nova (HP %d -> %d)", farBefore, far.HitPoints)
	}
}
