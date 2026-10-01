package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func crateTestGame(t *testing.T) *MMGame {
	t.Helper()
	game, _, _ := tbBehaviorGame(t, 12, 12)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	return game
}

// The box pile is stationary scenery; other loot crates keep their default spin.
func TestLootCratesUseAuthoredSpin(t *testing.T) {
	crateTestGame(t)
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc.Type != character.NPCTypeLootCrate {
			continue
		}
		if want := key == "pile_of_old_boxes"; npc.NoSpin != want {
			t.Errorf("loot crate %q no_spin = %v, want %v", key, npc.NoSpin, want)
		}
		cat := resolveNPCRenderCat(npc.RenderCategory)
		if cat != catScenery && cat != catLandmark {
			t.Errorf("loot crate %q render_category = %q, want rotating scenery or landmark", key, npc.RenderCategory)
		}
	}
}

func TestBoxPileSceneryMovement(t *testing.T) {
	// Authored category and pose survive both visit states and NPC restoration.
	// Walking and Fly must both cross the box pile after collision registration.
	for _, state := range []struct {
		name              string
		visited, restored bool
	}{
		{name: "fresh"},
		{name: "searched", visited: true},
		{name: "loaded_fresh", restored: true},
		{name: "loaded_searched", visited: true, restored: true},
	} {
		for _, mode := range []string{"walking", "flying"} {
			t.Run(state.name+"/"+mode, func(t *testing.T) {
				g := crateTestGame(t)
				ts := float64(g.config.GetTileSize())
				placePlayerAtTile(g, 4, 4, ts)
				boxes := spawnCrate(t, g, "pile_of_old_boxes", g.camera.X+ts, g.camera.Y)
				if state.restored {
					wm := &world.WorldManager{LoadedMaps: map[string]*world.World3D{"test": g.world}}
					g.restoreSavedNPCs(wm, &GameSave{NPCStates: []NPCSave{{
						MapKey: "test", Name: boxes.Name, X: boxes.X, Y: boxes.Y, Visited: state.visited,
					}}})
				} else {
					if state.visited {
						g.useLootCrate(boxes)
					}
					g.registerMapStaticCollision()
				}
				g.refreshLandmarkCollision()
				if boxes.Visited != state.visited {
					t.Fatalf("visited = %v, want %v", boxes.Visited, state.visited)
				}
				if npcRenderCatOf(boxes) != catScenery || !boxes.NoSpin {
					t.Fatal("box pile must load as stationary scenery")
				}
				g.world.SetTerrainPassageActive(mode == "flying")
				if !g.collisionSystem.CanMoveTo("player", boxes.X, boxes.Y) {
					t.Fatal("box pile blocks party movement")
				}
			})
		}
	}
}

func TestCrateInteractionSoundsAreAuthoredByProp(t *testing.T) {
	crateTestGame(t)
	tests := []struct {
		key  string
		want string
	}{
		{key: "pile_of_old_boxes"},
		{key: "campfire"},
		{key: "barrel_red"},
		{key: "barrel_green"},
		{key: "barrel_blue"},
		{key: "chest_wooden", want: "chest_open"},
		{key: "chest_iron", want: "chest_open"},
		{key: "chest_golden", want: "chest_open"},
		{key: "chest_gearwood", want: "chest_open"},
		{key: "chest_chrono", want: "chest_open"},
		{key: "chest_regal", want: "chest_open"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			crate := config.GetCrateConfig(test.key)
			if crate == nil {
				t.Fatalf("crate %q is missing", test.key)
			}
			if got := crate.InteractionSound; got != test.want {
				t.Fatalf("interaction sound = %q, want %q", got, test.want)
			}
		})
	}
}

// inventoryUnitsByName snapshots unit counts per item name - the merge-proof
// way to diff "what did this chest actually grant" now that AddItem folds
// stackable rewards into existing stacks.
func inventoryUnitsByName(p *character.Party) map[string]int {
	m := map[string]int{}
	for _, it := range p.Inventory {
		m[it.Name] += it.Count()
	}
	return m
}

// crateRewards is the party's reward state before an opening.
type crateRewards struct {
	units        map[string]int
	gold, points int
}

func snapshotCrateRewards(g *MMGame) crateRewards {
	return crateRewards{units: inventoryUnitsByName(g.party), gold: g.party.Gold, points: g.party.ArenaPoints}
}

// crateRewardSlots counts what one opening of crate key paid since before:
// every item unit, plus one slot per currency payout. A currency source pays
// its amount in each slot it wins, so 2000 gold from a 1000-gold source is
// two slots. currency is the currency share of the total.
func crateRewardSlots(t *testing.T, g *MMGame, key string, before crateRewards) (total, currency int) {
	t.Helper()
	for name, n := range inventoryUnitsByName(g.party) {
		if d := n - before.units[name]; d > 0 {
			total += d
		}
	}
	crate := config.GetCrateConfig(key)
	paid := func(pool string, gained int) int {
		if gained == 0 {
			return 0
		}
		amount := 0
		for _, s := range crate.RollSources {
			if s.Pool == pool {
				if amount != 0 && amount != s.Amount {
					t.Fatalf("%s has several %s amounts; slots are ambiguous", key, pool)
				}
				amount = s.Amount
			}
		}
		if amount <= 0 || gained%amount != 0 {
			t.Fatalf("%s paid %d %s, not a whole number of %d-slots", key, gained, pool, amount)
		}
		return gained / amount
	}
	currency = paid("gold", g.party.Gold-before.gold) + paid("arena_points", g.party.ArenaPoints-before.points)
	return total + currency, currency
}

func spawnCrate(t *testing.T, g *MMGame, key string, x, y float64) *character.NPC {
	t.Helper()
	npc, err := character.CreateNPCFromConfig(key, x, y)
	if err != nil {
		t.Fatalf("create %s: %v", key, err)
	}
	g.world.NPCs = append(g.world.NPCs, npc)
	return npc
}

// TestWoodenChest: 3 rolls from the drop pools of the monsters on the map.
func TestWoodenChest(t *testing.T) {
	g := crateTestGame(t)
	// Treants have a rich drop table (dead_branch/elven_bow/card).
	m := monster.NewMonster3DFromConfig(g.camera.X+300, g.camera.Y, "treant", g.config)
	g.world.Monsters = []*monster.Monster3D{m}

	chest := spawnCrate(t, g, "chest_wooden", g.camera.X+64, g.camera.Y)
	before := snapshotCrateRewards(g)
	g.useLootCrate(chest)
	if !chest.Visited {
		t.Fatal("chest not consumed")
	}
	if rewardSlots, _ := crateRewardSlots(t, g, "chest_wooden", before); rewardSlots != 3 {
		t.Fatalf("wooden chest produced %d reward slots, want 3", rewardSlots)
	}
	// Re-opening yields nothing.
	invAfter := g.party.GetTotalItems()
	g.useLootCrate(chest)
	if g.party.GetTotalItems() != invAfter {
		t.Fatal("an opened chest must stay empty")
	}
}

// TestWoodenChestRetainsInitialMapPoolAfterClear prevents map chests from
// turning into dust after the party has killed every fixed monster.
func TestWoodenChestRetainsInitialMapPoolAfterClear(t *testing.T) {
	g := crateTestGame(t)
	g.world.InitialMonsterKeys = map[string]struct{}{"treant": {}}
	g.world.Monsters = nil // The map has been completely cleared.

	chest := spawnCrate(t, g, "chest_wooden", g.camera.X+64, g.camera.Y)
	before := snapshotCrateRewards(g)
	g.useLootCrate(chest)
	if rewardSlots, _ := crateRewardSlots(t, g, "chest_wooden", before); rewardSlots != 3 {
		t.Fatalf("cleared-map wooden chest produced %d reward slots, want 3", rewardSlots)
	}
}

// TestIronChestFiltersCommons: the uncommon map gate drops every common entry
// from the map pool; the trap ignites the party unless disarmed.
func TestIronChestFiltersCommons(t *testing.T) {
	g := crateTestGame(t)
	m := monster.NewMonster3DFromConfig(g.camera.X+300, g.camera.Y, "treant", g.config)
	g.world.Monsters = []*monster.Monster3D{m}

	// The starting archer knows Disarm Trap (40% avoid) - strip it so the
	// trap outcome is deterministic.
	for _, member := range g.party.Members {
		delete(member.Skills, character.SkillDisarmTrap)
	}
	chest := spawnCrate(t, g, "chest_iron", g.camera.X+64, g.camera.Y)
	before := inventoryUnitsByName(g.party)
	g.useLootCrate(chest)
	after := inventoryUnitsByName(g.party)
	for name, n := range after {
		if n <= before[name] {
			continue
		}
		for _, it := range g.party.Inventory {
			if it.Name == name {
				if tier := rarityTier(it.Rarity); tier < 1 {
					t.Fatalf("iron chest dropped a common: %s (%s)", it.Name, it.Rarity)
				}
			}
		}
	}
	// Nobody has Disarm Trap in the bare fixture: the flame trap must have hit.
	burned := false
	for _, member := range g.party.Members {
		if member != nil && member.BurnFramesRemaining > 0 {
			burned = true
		}
	}
	if !burned {
		t.Fatal("undisarmed iron chest must ignite the party")
	}
}

func TestCrateIgniteUsesWearerStatusDuration(t *testing.T) {
	g := crateTestGame(t)
	for _, member := range g.party.Members {
		delete(member.Skills, character.SkillDisarmTrap)
	}
	protected := g.party.Members[0]
	protected.Equipment[items.SlotOffHand] = items.CreateItemFromYAML("deathgod_aegis")

	const igniteSeconds = 10
	g.springCrateTrap(
		&character.NPC{Name: "Test Chest"},
		&config.CrateConfig{TrapIgnite: true, TrapIgniteSeconds: igniteSeconds},
	)

	wantProtected := igniteSeconds * g.config.GetTPS() / 2
	if protected.BurnFramesRemaining != wantProtected {
		t.Fatalf("protected burn = %d frames, want %d", protected.BurnFramesRemaining, wantProtected)
	}
	unprotected := g.party.Members[1]
	wantFull := igniteSeconds * g.config.GetTPS()
	if unprotected.BurnFramesRemaining != wantFull {
		t.Fatalf("unprotected burn = %d frames, want %d", unprotected.BurnFramesRemaining, wantFull)
	}
}

func exactRarity(r string) config.RarityRange {
	t := config.RarityTier(r)
	return config.RarityRange{Min: t, Max: t}
}

func TestCrateCatalogRollFilters(t *testing.T) {
	crateTestGame(t)
	anyRarity, _ := config.ParseRarityRange("")
	for _, tc := range []struct {
		itemType string
		tiers    config.RarityRange
		ok       func(it items.Item) bool
	}{
		{"consumable", anyRarity, func(it items.Item) bool { return it.Type == items.ItemConsumable }},
		{"armor", exactRarity("common"), func(it items.Item) bool { return it.Type == items.ItemArmor && it.Rarity == "common" }},
		{"accessory", exactRarity("uncommon"), func(it items.Item) bool { return it.Type == items.ItemAccessory && it.Rarity == "uncommon" }},
		{"weapon", exactRarity("rare"), func(it items.Item) bool { return it.Type == items.ItemWeapon && it.Rarity == "rare" }},
		{"any", exactRarity("legendary"), func(it items.Item) bool { return it.Rarity == "legendary" && it.Type != items.ItemQuest }},
	} {
		for i := 0; i < 50; i++ {
			it, ok := rollCatalogItem(tc.itemType, tc.tiers)
			if !ok || !tc.ok(it) {
				t.Fatalf("%s %+v catalog roll returned %s (%s/%s) ok=%v", tc.itemType, tc.tiers, it.Name, it.Type, it.Rarity, ok)
			}
		}
	}
}

// Every slot draws from exactly one source, and a map source with nothing in
// its rarity span on this map sits the chest out instead of emptying slots.
func TestCrateSlotsRollOneSourceEach(t *testing.T) {
	g := crateTestGame(t)
	g.world.Monsters = []*monster.Monster3D{monster.NewMonster3DFromConfig(g.camera.X+300, g.camera.Y, "treant", g.config)}
	for _, tc := range []struct {
		name      string
		sources   []config.CrateRollSource
		gold, pts int
	}{
		{"one source fills every slot", []config.CrateRollSource{{Pool: "gold", Amount: 10, Weight: 100}}, 30, 0},
		{"each slot one currency", []config.CrateRollSource{{Pool: "gold", Amount: 10, Weight: 50}, {Pool: "arena_points", Amount: 1, Weight: 50}}, -1, -1},
		{"empty map span sits out", []config.CrateRollSource{{Pool: "map", Rarity: "unique", Weight: 90}, {Pool: "gold", Amount: 10, Weight: 10}}, 30, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loot, gold, pts := g.rollCratePool(&config.CrateConfig{Rolls: 3, RollSources: tc.sources})
			if len(loot) != 0 {
				t.Fatalf("currency-only crate granted items %v", loot)
			}
			if tc.gold >= 0 && (gold != tc.gold || pts != tc.pts) {
				t.Fatalf("crate paid %d gold and %d points, want %d and %d", gold, pts, tc.gold, tc.pts)
			}
			if tc.gold < 0 && gold/10+pts != 3 {
				t.Fatalf("three slots paid %d gold and %d points; each slot must pay one source", gold, pts)
			}
		})
	}
}

// The one-table crates keep the odds the old per-chest jackpot rolls gave: a
// w%-a-slot source over n slots shows up in 1-(1-w)^n of the chests, and the
// ordinary sources keep their old shares of the rest. The regal chest's
// Clockmaker gear is a later 10%-a-chest jackpot.
func TestCrateOddsMatchTheirFormerPerChestRolls(t *testing.T) {
	crateTestGame(t)
	is := func(pool, itemType, rarity string, amount int) func(config.CrateRollSource) bool {
		return func(s config.CrateRollSource) bool {
			return s.Pool == pool && s.ItemType == itemType && s.Rarity == rarity && s.Amount == amount
		}
	}
	type share struct {
		match func(config.CrateRollSource) bool
		old   float64 // former weight among the ordinary sources
	}
	for _, tc := range []struct {
		key      string
		jackpots map[string]float64 // per-chest chance by label
		matchers map[string]func(config.CrateRollSource) bool
		ordinary []share
	}{
		{"chest_wooden", map[string]float64{"rare": .07, "legendary": .03, "gold": .05},
			map[string]func(config.CrateRollSource) bool{"rare": is("map", "", "rare", 0), "legendary": is("map", "", "legendary", 0), "gold": is("gold", "", "", 1000)},
			[]share{{is("map", "", "common-uncommon", 0), .70}, {is("catalog", "consumable", "", 0), .20}, {is("catalog", "armor", "common", 0), .10}}},
		{"chest_iron", map[string]float64{"rare": .25, "legendary": .05, "gold": .05},
			map[string]func(config.CrateRollSource) bool{"rare": is("map", "", "rare", 0), "legendary": is("map", "", "legendary", 0), "gold": is("gold", "", "", 1500)},
			[]share{{is("map", "", "uncommon", 0), .60}, {is("catalog", "accessory", "uncommon", 0), .20}, {is("catalog", "armor", "uncommon", 0), .20}}},
		{"chest_golden", map[string]float64{"arena": .05},
			map[string]func(config.CrateRollSource) bool{"arena": is("arena_points", "", "", 5000)},
			[]share{{is("catalog", "any", "rare", 0), .85}, {is("catalog", "any", "legendary", 0), .15}}},
		{"chest_gearwood", nil, nil,
			[]share{{is("map", "", "common-uncommon", 0), .60}, {is("gold", "", "", 120), .25}, {is("catalog", "consumable", "", 0), .15}}},
		{"chest_chrono", map[string]float64{"rare": .20},
			map[string]func(config.CrateRollSource) bool{"rare": is("map", "", "rare", 0)},
			[]share{{is("map", "", "uncommon+", 0), .60}, {is("gold", "", "", 250), .25}, {is("catalog", "trinket", "common-rare", 0), .15}}},
		{"chest_regal", map[string]float64{"rare": .30, "gold": .10, "gear": .10},
			map[string]func(config.CrateRollSource) bool{"rare": is("map", "", "rare", 0), "gold": is("gold", "", "", 800), "gear": isClockGear},
			[]share{{is("map", "", "uncommon+", 0), .55}, {is("catalog", "trinket", "common-rare", 0), .25}, {is("gold", "", "", 400), .20}}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			crate := config.GetCrateConfig(tc.key)
			if crate == nil {
				t.Fatal("crate missing")
			}
			weight := func(match func(config.CrateRollSource) bool) float64 {
				w := 0.0
				for _, s := range crate.RollSources {
					if match(s) {
						w += s.Weight / 100
					}
				}
				return w
			}
			for label, want := range tc.jackpots {
				if got := 1 - math.Pow(1-weight(tc.matchers[label]), float64(crate.Rolls)); math.Abs(got-want) > 0.01 {
					t.Fatalf("%s comes up in %.1f%% of chests, was %.0f%%", label, got*100, want*100)
				}
			}
			total := 0.0
			for _, o := range tc.ordinary {
				total += weight(o.match)
			}
			for i, o := range tc.ordinary {
				if got := weight(o.match) / total; math.Abs(got-o.old) > 0.005 {
					t.Fatalf("ordinary source %d holds %.1f%% of the ordinary slots, was %.0f%%", i, got*100, o.old*100)
				}
			}
		})
	}
}

func isClockGear(s config.CrateRollSource) bool {
	return s.Pool == "loot_table" && s.LootTable == "clock_tower_gear"
}

// The regal chest's gear slot opens the Clockmaker's arsenal: the table names
// every weapon and wearable Odile sells and nothing else, and a chest whose
// slots all land on it grants exactly one such piece per slot, no coin.
func TestRegalChestRollsClockmakerGear(t *testing.T) {
	g := crateTestGame(t)
	odile, err := character.CreateNPCFromConfig("clockmaker", 0, 0)
	if err != nil {
		t.Fatalf("clockmaker: %v", err)
	}
	sold := map[string]bool{}
	for _, m := range odile.MerchantStock {
		switch m.Item.Type {
		case items.ItemWeapon, items.ItemArmor, items.ItemAccessory:
			sold[m.Item.Name] = true
		}
	}
	table, ok := config.GetWeightedLootTable("clock_tower_gear")
	if !ok {
		t.Fatal("clock_tower_gear table missing")
	}
	listed := map[string]bool{}
	for _, e := range table.Entries {
		it, err := createLootItem(e.Type, e.Key)
		if err != nil {
			t.Fatalf("%s: %v", e.Key, err)
		}
		if !sold[it.Name] {
			t.Errorf("table lists %s, which the Clockmaker does not sell", it.Name)
		}
		listed[it.Name] = true
	}
	for name := range sold {
		if !listed[name] {
			t.Errorf("Clockmaker sells %s, but the table never rolls it", name)
		}
	}

	crate := config.GetCrateConfig("chest_regal")
	var gear *config.CrateRollSource
	for i := range crate.RollSources {
		if isClockGear(crate.RollSources[i]) {
			gear = &crate.RollSources[i]
		}
	}
	if gear == nil {
		t.Fatal("regal chest has no clock_tower_gear source")
	}
	forced := *crate
	forced.TrapDamage = 0
	only := *gear
	only.Weight = 100
	forced.RollSources = []config.CrateRollSource{only}
	config.GlobalLoots.Crates["chest_regal"] = &forced
	t.Cleanup(func() { config.GlobalLoots.Crates["chest_regal"] = crate })
	chest := spawnCrate(t, g, "chest_regal", g.camera.X+64, g.camera.Y)
	before, goldBefore := inventoryUnitsByName(g.party), g.party.Gold
	g.useLootCrate(chest)
	gained := 0
	for name, n := range inventoryUnitsByName(g.party) {
		if d := n - before[name]; d > 0 {
			if !sold[name] {
				t.Fatalf("gear slot granted %s", name)
			}
			gained += d
		}
	}
	if gained != crate.Rolls || g.party.Gold != goldBefore {
		t.Fatalf("regal chest granted %d pieces and %d gold, want %d pieces and no gold", gained, g.party.Gold-goldBefore, crate.Rolls)
	}
}

func TestGoldenChestTrapDamageTypesComeFromYAML(t *testing.T) {
	crateTestGame(t)
	crate := config.GetCrateConfig("chest_golden")
	if crate == nil {
		t.Fatal("golden chest config missing")
	}
	if crate.TrapDamage != 150 {
		t.Fatalf("golden chest trap damage = %d, want 150", crate.TrapDamage)
	}
	if got := crate.TrapDamageTypes; len(got) != 2 || got[0] != "physical" || got[1] != "fire" {
		t.Fatalf("golden chest trap damage types = %v, want [physical fire]", got)
	}
}

// TestGoldenChestPool: catalog rares (or upgraded legendaries) only. Cards are
// valid collectible loot, while quest items and arena uniques stay excluded.
func TestGoldenChestPool(t *testing.T) {
	g := crateTestGame(t)
	chest := spawnCrate(t, g, "chest_golden", g.camera.X+64, g.camera.Y)
	// Diff unit counts by NAME: AddItem merges stackable rewards (possibly into
	// a pre-held stack), so slicing appended entries under-counts and can skip
	// a merged drop's rarity check.
	before := snapshotCrateRewards(g)
	g.useLootCrate(chest)
	for name, n := range inventoryUnitsByName(g.party) {
		if n <= before.units[name] {
			continue
		}
		for _, it := range g.party.Inventory {
			if it.Name == name && it.Rarity != "rare" && it.Rarity != "legendary" {
				t.Fatalf("golden chest dropped %s (%s), want rare/legendary", it.Name, it.Rarity)
			}
		}
	}
	if rewardSlots, _ := crateRewardSlots(t, g, "chest_golden", before); rewardSlots != 3 {
		t.Fatalf("golden chest produced %d reward slots, want 3", rewardSlots)
	}
}

// A currency source pays once per slot it wins, so one opening can pay the
// same coin twice or three times. Half of each forced crate's slots pay
// currency: every opening still counts exactly its rolls, and some openings
// pay the currency repeatedly (each of 64 misses that with p = 1/2).
func TestCrateCurrencyPaysPerSlot(t *testing.T) {
	g := crateTestGame(t)
	for _, tc := range []struct {
		key     string
		sources []config.CrateRollSource
	}{
		{"chest_wooden", []config.CrateRollSource{{Pool: "gold", Amount: 1000, Weight: 50}, {Pool: "catalog", ItemType: "consumable", Weight: 50}}},
		{"chest_golden", []config.CrateRollSource{{Pool: "arena_points", Amount: 5000, Weight: 50}, {Pool: "catalog", ItemType: "any", Rarity: "rare", Weight: 50}}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			crate := config.GetCrateConfig(tc.key)
			forced := *crate
			forced.TrapDamage, forced.TrapIgnite = 0, false
			forced.RollSources = tc.sources
			config.GlobalLoots.Crates[tc.key] = &forced
			t.Cleanup(func() { config.GlobalLoots.Crates[tc.key] = crate })
			repeated := false
			for i := 0; i < 64; i++ {
				before := snapshotCrateRewards(g)
				g.useLootCrate(spawnCrate(t, g, tc.key, g.camera.X+64, g.camera.Y))
				total, currency := crateRewardSlots(t, g, tc.key, before)
				if total != forced.Rolls {
					t.Fatalf("opening %d counted %d rewards (%d of them currency), want %d", i, total, currency, forced.Rolls)
				}
				repeated = repeated || currency >= 2
			}
			if !repeated {
				t.Fatal("no opening paid its currency in two slots")
			}
		})
	}
}

// TestGoldenChestTrapDisarm: a Grandmaster Disarm Trap hand avoids the trap
// with certainty (40/60/80/100 by tier).
func TestGoldenChestTrapDisarm(t *testing.T) {
	g := crateTestGame(t)
	thief := g.party.Members[0]
	thief.Skills[character.SkillDisarmTrap] = &character.Skill{Mastery: character.MasteryGrandMaster}
	if chance, hand := g.partyTrapAvoidChancePct(); chance != 100 || hand != thief {
		t.Fatalf("GM disarm chance = %d (hand %v), want 100", chance, hand)
	}
	hpBefore := make([]int, len(g.party.Members))
	for i, m := range g.party.Members {
		hpBefore[i] = m.HitPoints
	}
	chest := spawnCrate(t, g, "chest_golden", g.camera.X+64, g.camera.Y)
	g.useLootCrate(chest)
	for i, m := range g.party.Members {
		if m.HitPoints != hpBefore[i] {
			t.Fatalf("member %d took trap damage despite a certain disarm", i)
		}
	}
}

func TestPartyTrapAvoidChanceUsesBestActiveDisarmer(t *testing.T) {
	g := crateTestGame(t)
	for _, member := range g.party.Members {
		delete(member.Skills, character.SkillDisarmTrap)
	}
	first, second := g.party.Members[0], g.party.Members[1]
	first.Skills[character.SkillDisarmTrap] = &character.Skill{Mastery: character.MasteryNovice}
	second.Skills[character.SkillDisarmTrap] = &character.Skill{Mastery: character.MasteryMaster}

	if chance, hand := g.partyTrapAvoidChancePct(); chance != 80 || hand != second {
		t.Fatalf("best active disarmer = %d%% (%v), want 80%% (%v)", chance, hand, second)
	}

	second.HitPoints = 0
	if chance, hand := g.partyTrapAvoidChancePct(); chance != 40 || hand != first {
		t.Fatalf("with best disarmer incapacitated = %d%% (%v), want 40%% (%v)", chance, hand, first)
	}
}

// TestChestNeedsLineOfSight: a wall between party and chest refuses the open
// and leaves the chest intact.
func TestChestNeedsLineOfSight(t *testing.T) {
	g := crateTestGame(t)
	ts := float64(g.config.GetTileSize())
	placePlayerAtTile(g, 2, 2, ts)
	// Wall the chest off completely: it sits behind a solid tile.
	g.world.Tiles[2][3] = 1 // TileWall
	chest := spawnCrate(t, g, "chest_wooden", (4.0+0.5)*ts, (2.0+0.5)*ts)
	invBefore := len(g.party.Inventory)
	g.useLootCrate(chest)
	if chest.Visited {
		t.Fatal("chest behind a wall must not open")
	}
	if len(g.party.Inventory) != invBefore {
		t.Fatal("no loot through walls")
	}
}

// TestSpellLectern: teaches the first member with the school open; a lectern
// nobody can read is NOT consumed.
func TestSpellLectern(t *testing.T) {
	g := crateTestGame(t)
	// Nobody has any school open: the tome must refuse and survive.
	for _, m := range g.party.Members {
		m.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
	}
	lectern := spawnCrate(t, g, "spell_lectern", g.camera.X+64, g.camera.Y)
	g.useSpellLectern(lectern)
	if lectern.Visited {
		t.Fatal("a lectern nobody can read must not be consumed")
	}

	// Open Air on one member: the pool must teach them one of its spells.
	reader := g.party.Members[1]
	reader.MagicSchools[character.MagicSchoolAir] = &character.MagicSkill{}
	g.useSpellLectern(lectern)
	if !lectern.Visited {
		t.Fatal("lectern not consumed after teaching")
	}
	// The pool is shuffled and authored in npcs.yaml, so the reachable set is
	// derived from it: any pool spell Air can learn counts.
	learned := ""
	for _, id := range lectern.Lectern.Pool {
		spellID := spells.SpellID(id)
		if reader.KnowsSpell(spellID) {
			learned = id
		}
	}
	if learned == "" {
		t.Fatalf("reader learned nothing from the lectern (pool: %v)", lectern.Lectern.Pool)
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(learned))
	if err != nil {
		t.Fatalf("learned spell %q has no definition: %v", learned, err)
	}
	airReachable := false
	for _, school := range def.SchoolList() {
		if school == string(character.MagicSchoolAir) {
			airReachable = true
		}
	}
	if !airReachable {
		t.Errorf("lectern taught %q (schools %v) to an Air-only reader", learned, def.SchoolList())
	}
}

// The lake-chest contract: a crate standing on deep water opens across water
// (water tiles are transparent, so the LOS gate passes) while a wall on the
// same ray still refuses the interaction.
func TestCrateOpensAcrossWaterButNotThroughWalls(t *testing.T) {
	g := crateTestGame(t)
	tile := float64(g.config.GetTileSize())
	g.camera.X, g.camera.Y = TileCenterFromTile(1, 1, tile)
	g.collisionSystem = collision.NewCollisionSystem(g.world, tile)

	stageCrate := func(between world.TileType3D) *character.NPC {
		g.world.Tiles[1][2] = between
		g.world.Tiles[1][3] = world.TileDeepWater
		x, y := TileCenterFromTile(3, 1, tile)
		npc, err := character.CreateNPCFromConfig("chest_iron", x, y)
		if err != nil {
			t.Fatalf("create chest: %v", err)
		}
		return npc
	}

	chest := stageCrate(world.TileDeepWater)
	g.useLootCrate(chest)
	if !chest.Visited {
		t.Fatal("chest across deep water refused to open - water must not block the LOS gate")
	}

	walled := stageCrate(world.TileThicket)
	g.useLootCrate(walled)
	if walled.Visited {
		t.Fatal("chest behind a thicket opened - the through-wall guard is gone")
	}

	treed := stageCrate(world.TileTree)
	g.useLootCrate(treed)
	if treed.Visited {
		t.Fatal("chest behind a tree opened - the through-tree guard is gone")
	}
}
