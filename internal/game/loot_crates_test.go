package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
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
	invBefore := len(g.party.Inventory)
	goldBefore := g.party.Gold
	g.useLootCrate(chest)
	if !chest.Visited {
		t.Fatal("chest not consumed")
	}
	rewardSlots := len(g.party.Inventory) - invBefore
	if g.party.Gold > goldBefore {
		rewardSlots++ // A special gold cache replaces one item slot.
	}
	if rewardSlots != 3 {
		t.Fatalf("wooden chest produced %d reward slots, want 3", rewardSlots)
	}
	// Re-opening yields nothing.
	invAfter := len(g.party.Inventory)
	g.useLootCrate(chest)
	if len(g.party.Inventory) != invAfter {
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
	invBefore := len(g.party.Inventory)
	goldBefore := g.party.Gold
	g.useLootCrate(chest)
	rewardSlots := len(g.party.Inventory) - invBefore
	if g.party.Gold > goldBefore {
		rewardSlots++
	}
	if rewardSlots != 3 {
		t.Fatalf("cleared-map wooden chest produced %d reward slots, want 3", rewardSlots)
	}
}

// TestIronChestFiltersCommons: min_rarity uncommon drops every common entry
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
	invBefore := len(g.party.Inventory)
	g.useLootCrate(chest)
	for _, it := range g.party.Inventory[invBefore:] {
		if tier := rarityTier(it.Rarity); tier < 1 {
			t.Fatalf("iron chest dropped a common: %s (%s)", it.Name, it.Rarity)
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

func TestCrateCatalogRollFilters(t *testing.T) {
	crateTestGame(t)

	for i := 0; i < 50; i++ {
		it, ok := rollCatalogItem("consumable", "", "", "")
		if !ok {
			t.Fatal("consumable catalog roll failed")
		}
		if it.Type != items.ItemConsumable {
			t.Fatalf("consumable catalog roll returned %s (%s)", it.Name, it.Type)
		}
	}

	for i := 0; i < 50; i++ {
		it, ok := rollCatalogItem("armor", "common", "", "")
		if !ok {
			t.Fatal("common armor catalog roll failed")
		}
		if it.Type != items.ItemArmor || it.Rarity != "common" {
			t.Fatalf("common armor roll returned %s (%s/%s)", it.Name, it.Type, it.Rarity)
		}
	}

	for i := 0; i < 50; i++ {
		it, ok := rollCatalogItem("accessory", "uncommon", "", "")
		if !ok {
			t.Fatal("uncommon accessory catalog roll failed")
		}
		if it.Type != items.ItemAccessory || it.Rarity != "uncommon" {
			t.Fatalf("uncommon accessory roll returned %s (%s/%s)", it.Name, it.Type, it.Rarity)
		}
	}
}

func TestCrateSpecialRollReplacesOneBaseSlot(t *testing.T) {
	g := crateTestGame(t)
	crate := &config.CrateConfig{
		Rolls: 3,
		RollSources: []config.CrateRollSource{
			{Pool: "catalog", ItemType: "consumable", Weight: 1},
		},
		SpecialRolls: []config.CrateRollSource{
			{Pool: "gold", Amount: 1000, ChancePct: 100},
			{Pool: "arena_points", Amount: 5000, ChancePct: 100},
		},
	}

	loot, gold, arenaPoints := g.rollCratePool(crate)
	if gold != 1000 || arenaPoints != 5000 {
		t.Fatalf("special rewards = %d gold, %d arena points; want 1000, 5000", gold, arenaPoints)
	}
	if len(loot) != 1 {
		t.Fatalf("special rolls must replace two of three base slots; got %d item slots", len(loot))
	}
}

func TestCrateRarityAndCurrencyRulesComeFromYAML(t *testing.T) {
	crateTestGame(t)
	for _, tc := range []struct {
		key                   string
		rarePct, legendaryPct int
		currencyPool          string
		currencyAmount        int
	}{
		{"chest_wooden", 7, 3, "gold", 1000},
		{"chest_iron", 25, 5, "gold", 1500},
		{"chest_golden", 0, 0, "arena_points", 5000},
	} {
		crate := config.GetCrateConfig(tc.key)
		if crate == nil {
			t.Fatalf("%s config missing", tc.key)
		}
		var rare, legendary, currency *config.CrateRollSource
		for i := range crate.SpecialRolls {
			src := &crate.SpecialRolls[i]
			switch {
			case src.Pool == "map" && src.Rarity == "rare":
				rare = src
			case src.Pool == "map" && src.Rarity == "legendary":
				legendary = src
			case src.Pool == tc.currencyPool:
				currency = src
			}
		}
		if tc.rarePct > 0 && (rare == nil || rare.ChancePct != tc.rarePct) {
			t.Fatalf("%s rare map roll = %+v, want %d%%", tc.key, rare, tc.rarePct)
		}
		if tc.legendaryPct > 0 && (legendary == nil || legendary.ChancePct != tc.legendaryPct) {
			t.Fatalf("%s legendary map roll = %+v, want %d%%", tc.key, legendary, tc.legendaryPct)
		}
		if currency == nil || currency.Amount != tc.currencyAmount || currency.ChancePct != 5 {
			t.Fatalf("%s currency roll = %+v, want 5%% for %d %s", tc.key, currency, tc.currencyAmount, tc.currencyPool)
		}
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
	invBefore := len(g.party.Inventory)
	arenaBefore := g.party.ArenaPoints
	g.useLootCrate(chest)
	drops := g.party.Inventory[invBefore:]
	rewardSlots := len(drops)
	if g.party.ArenaPoints > arenaBefore {
		rewardSlots++ // The arena jackpot replaces a rare/legendary item.
	}
	if rewardSlots != 3 {
		t.Fatalf("golden chest produced %d reward slots, want 3", rewardSlots)
	}
	for _, it := range drops {
		if it.Rarity != "rare" && it.Rarity != "legendary" {
			t.Fatalf("golden chest dropped %s (%s), want rare/legendary", it.Name, it.Rarity)
		}
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
	learned := false
	for _, id := range []string{"fly", "town_portal"} { // the air-reachable pool spells
		if reader.KnowsSpell(spells.SpellID(id)) {
			learned = true
		}
	}
	if !learned {
		t.Fatal("reader learned nothing from the lectern")
	}
}
