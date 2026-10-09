package game

import (
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// The clock tower dungeon (2026-07-16): an item-backed merchant currency
// (clock hands), a grid-span building facade with a solid multi-tile footprint,
// and construct mobs with attack sheets.

func TestCurrencyItemKeyParsing(t *testing.T) {
	if k, ok := character.CurrencyItemKey("item:clock_hand"); !ok || k != "clock_hand" {
		t.Fatalf("item:clock_hand -> (%q,%v), want (clock_hand,true)", k, ok)
	}
	for _, cur := range []string{"", "arena_points", "item:", "clock_hand"} {
		if _, ok := character.CurrencyItemKey(cur); ok {
			t.Errorf("%q must not parse as an item currency", cur)
		}
	}
}

func TestValidateNPCCommerce(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	_ = cs
	weapon := ""
	for _, w := range config.GlobalWeapons.Weapons {
		weapon = w.Name
		break
	}
	good := map[string]*character.NPCData{
		"a": {Currency: ""},
		"b": {Currency: "arena_points"},
		"c": {Currency: "item:clock_hand"},
		"d": {GridSpanTiles: 2, GridSpanDir: "e"},
		"e": {Inventory: []*character.NPCItem{{Type: "item", Name: "Health Potion"}}, ShopDialogue: true, Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{Action: "leave"}}}},
		"f": {Inventory: []*character.NPCItem{{Type: "weapon", Name: weapon}}},
	}
	if err := ValidateNPCCommerce(good); err != nil {
		t.Fatalf("valid set rejected: %v", err)
	}
	bad := []map[string]*character.NPCData{
		{"x": {Currency: "gems"}},
		{"x": {Currency: "item:no_such_item"}},
		{"x": {GridSpanTiles: 1}},
		{"x": {GridSpanTiles: 2}}, // missing dir
		{"x": {GridSpanTiles: 5, GridSpanDir: "e"}},
		{"x": {GridSpanTiles: 2, GridSpanDir: "e", NoSpin: true}},
		{"x": {Inventory: []*character.NPCItem{{Type: "weapon", Name: "No Such Blade"}}}},
	}
	for i, m := range bad {
		if err := ValidateNPCCommerce(m); err == nil {
			t.Errorf("bad set %d accepted", i)
		}
	}
}

// Reward chests share one world-wide list keyed by ID: a reused ID hides the
// second chest, whichever field or file authors it.
func TestValidateRewardChestIDs(t *testing.T) {
	fight := func(single string, list ...string) *character.NPCData {
		r := &monsterPkg.EncounterRewards{}
		if single != "" {
			r.TreasureChest = &monsterPkg.TreasureChestReward{ID: single}
		}
		for _, id := range list {
			r.TreasureChests = append(r.TreasureChests, monsterPkg.TreasureChestReward{ID: id})
		}
		return &character.NPCData{Type: character.NPCTypeEncounter, Encounter: &character.NPCEncounter{Rewards: r}}
	}
	cleared := func(id string) *config.MapConfig {
		return &config.MapConfig{ClearEncounter: &config.MapClearEncounterConfig{Rewards: &config.MapEncounterRewardsConfig{
			TreasureChest: &config.MapTreasureChestRewardConfig{ID: id},
		}}}
	}
	for _, tc := range []struct {
		name string
		npcs map[string]*character.NPCData
		maps map[string]*config.MapConfig
		ok   bool
	}{
		{"distinct and unnamed chests", map[string]*character.NPCData{"a": fight("a_chest", "a_chest_2", ""), "b": fight("", "b_chest", "")}, map[string]*config.MapConfig{"m": cleared("m_chest")}, true},
		{"two chests of one fight", map[string]*character.NPCData{"a": fight("", "a_chest", "a_chest")}, nil, false},
		{"single and list chest fields", map[string]*character.NPCData{"a": fight("a_chest", "a_chest")}, nil, false},
		{"two fights", map[string]*character.NPCData{"a": fight("", "x"), "b": fight("", "x")}, nil, false},
		{"fight and map clear reward", map[string]*character.NPCData{"a": fight("", "x")}, map[string]*config.MapConfig{"m": cleared("x")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateRewardChestIDs(tc.npcs, tc.maps); (err == nil) != tc.ok {
				t.Fatalf("valid=%v, want %v: %v", err == nil, tc.ok, err)
			}
		})
	}
}

// Paying in clock hands consumes exactly the cost from the inventory and
// refuses when short.
func TestItemCurrencyPayment(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	p := cs.game.party
	p.Inventory = nil
	hand, err := items.TryCreateItemFromYAML("clock_hand")
	if err != nil {
		t.Fatalf("clock_hand item: %v", err)
	}
	for i := 0; i < 3; i++ {
		p.AddItem(hand)
	}
	if p.RemoveItemsByName(hand.Name, 4) {
		t.Fatal("payment of 4 must fail with 3 hands")
	}
	if p.CountItemsByName(hand.Name) != 3 {
		t.Fatal("failed payment must not consume anything")
	}
	if !p.RemoveItemsByName(hand.Name, 2) {
		t.Fatal("payment of 2 must succeed with 3 hands")
	}
	if p.CountItemsByName(hand.Name) != 1 {
		t.Fatalf("2 hands must be consumed, %d left", p.CountItemsByName(hand.Name))
	}
}

// A grid-span building really owns its two tiles: both are collision-blocked,
// and the pose spans the pair grid-aligned.
func TestGridSpanBuildingFootprint(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ts := float64(g.config.GetTileSize())
	g.world.NPCs = append(g.world.NPCs, &character.NPC{
		Name: "Tower", X: 10*ts + ts/2, Y: 10*ts + ts/2,
		RenderCategory: "landmark", GridSpanTiles: 2, GridSpanDir: "e",
	})
	g.registerBuildingFootprints()

	for i, tx := range []float64{10, 11} {
		x := tx*ts + ts/2
		y := 10*ts + ts/2
		if g.collisionSystem.CanMoveTo("player", x, y) {
			t.Errorf("footprint tile %d at (%.0f,%.0f) must be solid", i, x, y)
		}
	}
	// The tile past the span stays free.
	if !g.collisionSystem.CanMoveTo("player", 12*ts+ts/2, 10*ts+ts/2) {
		t.Error("tile beyond the span must stay walkable")
	}

	bx, by, yaw, ok := g.buildingPose(g.world.NPCs[len(g.world.NPCs)-1])
	if !ok {
		t.Fatal("buildingPose must resolve for a valid span")
	}
	if wantX := 11 * ts; bx != wantX || by != 10*ts+ts/2 || yaw != 0 {
		t.Errorf("pose = (%.0f,%.0f,%.2f), want (%.0f,%.0f,0) - midpoint of the pair, slab along east",
			bx, by, yaw, wantX, 10*ts+ts/2)
	}

	// Map switch cleanup frees the tiles again.
	g.clearBuildingEntities()
	if !g.collisionSystem.CanMoveTo("player", 10*ts+ts/2, 10*ts+ts/2) {
		t.Error("clearBuildingEntities must free the footprint")
	}
}

func TestSaveLoadRestoresFourTileBuildingFootprint(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "clock_tower_test"
	tile := float64(cfg.GetTileSize())
	newTowerWorld := func() *world.World3D {
		w := newTestWorldSized(cfg, 24, 24)
		w.NPCs = append(w.NPCs, &character.NPC{
			Name: "Tower", X: 10*tile + tile/2, Y: 10*tile + tile/2,
			RenderCategory: "landmark", GridSpanTiles: 4, GridSpanDir: "e",
		})
		return w
	}
	newManager := func(w *world.World3D) *world.WorldManager {
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		return wm
	}

	wmSave := newManager(newTowerWorld())
	gameSave := newTestGame(cfg, wmSave.GetCurrentWorld())
	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmSave
	defer func() { world.GlobalWorldManager = oldWorldManager }()
	save := gameSave.buildSave(wmSave)

	wmLoad := newManager(newTowerWorld())
	world.GlobalWorldManager = wmLoad
	gameLoad := newTestGame(cfg, wmLoad.GetCurrentWorld())
	if err := gameLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	for tx := 10; tx < 14; tx++ {
		x, y := TileCenterFromTile(tx, 10, tile)
		if gameLoad.collisionSystem.CanMoveTo("player", x, y) {
			t.Errorf("loaded four-tile facade left footprint tile (%d,10) walkable", tx)
		}
	}
	x, y := TileCenterFromTile(14, 10, tile)
	if !gameLoad.collisionSystem.CanMoveTo("player", x, y) {
		t.Error("tile after the loaded four-tile facade must remain walkable")
	}
}

func TestSaveLoadPreservesAlarmRallyDone(t *testing.T) {
	cfg := loadTestConfig(t)
	const mapKey = "clock_tower_test"
	newManager := func(w *world.World3D) *world.WorldManager {
		wm := world.NewWorldManager(cfg)
		wm.CurrentMapKey = mapKey
		wm.LoadedMaps = map[string]*world.World3D{mapKey: w}
		return wm
	}

	wSave := newTestWorldSized(cfg, 24, 24)
	alarm := monsterPkg.NewMonster3DFromConfig(8.5*float64(cfg.GetTileSize()), 8.5*float64(cfg.GetTileSize()), "alarm_clock", cfg)
	if alarm == nil {
		t.Fatal("alarm_clock is missing from monsters.yaml")
	}
	alarm.ID = "saved-alarm"
	alarm.RallyDone = true
	wSave.Monsters = append(wSave.Monsters, alarm)
	wmSave := newManager(wSave)

	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = wmSave
	defer func() { world.GlobalWorldManager = oldWorldManager }()
	save := newTestGame(cfg, wSave).buildSave(wmSave)

	wLoad := newTestWorldSized(cfg, 24, 24)
	wmLoad := newManager(wLoad)
	world.GlobalWorldManager = wmLoad
	gameLoad := newTestGame(cfg, wLoad)
	if err := gameLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if len(wLoad.Monsters) != 1 || wLoad.Monsters[0].ID != alarm.ID || !wLoad.Monsters[0].RallyDone {
		t.Fatalf("loaded alarm did not retain its one-shot rally state: %+v", wLoad.Monsters)
	}
}

func TestRespawnAuthoredMonstersPreservesPartyCharms(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 24, 24)
	tile := float64(cfg.GetTileSize())
	w.MonsterSpawns = []world.MonsterSpawn{{X: 2, Y: 2, MonsterKey: "goblin"}}

	charmed := monsterPkg.NewMonster3DFromConfig(8.5*tile, 8.5*tile, "goblin", cfg)
	bound := monsterPkg.NewMonster3DFromConfig(9.5*tile, 8.5*tile, "skeleton", cfg)
	if charmed == nil || bound == nil {
		t.Fatal("test monsters are missing from monsters.yaml")
	}
	charmed.ID = "party-charm"
	charmed.CharmedByParty = true
	bound.ID = "bound-undead"
	bound.Bound = true
	w.Monsters = []*monsterPkg.Monster3D{charmed, bound}

	w.RespawnAuthoredMonsters()
	if len(w.Monsters) != 2 {
		t.Fatalf("respawn roster has %d monsters, want preserved charm plus authored spawn", len(w.Monsters))
	}
	var foundCharm, foundBound, foundFresh bool
	for _, m := range w.Monsters {
		switch m {
		case charmed:
			foundCharm = true
		case bound:
			foundBound = true
		default:
			foundFresh = m.Key == "goblin" && m.ID != charmed.ID
		}
	}
	if !foundCharm || foundBound || !foundFresh {
		t.Fatalf("respawn preservation mismatch: charm=%v bound=%v fresh=%v", foundCharm, foundBound, foundFresh)
	}
}

// clockmaker loads the shipped NPC catalog (restored afterwards) and returns
// the Clockmaker's authored data and her live merchant.
func clockmaker(t *testing.T) (*character.NPCData, *character.NPC) {
	t.Helper()
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	data, ok := character.NPCConfigInstance.GetNPCData("clockmaker")
	if !ok {
		t.Fatal("clockmaker NPC missing")
	}
	npc, err := character.CreateNPCFromConfig("clockmaker", 0, 0)
	if err != nil {
		t.Fatalf("clockmaker: %v", err)
	}
	return data, npc
}

// Every monster that drops the Clockmaker's currency is one of the tower's
// constructs, and every weapon she sells is no_loot (her stock and the regal
// chest only). Her other stock is checked by ValidateNPCCommerce at boot.
func TestClockTowerContentIntegrity(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	previousMonsters := monsterPkg.MonsterConfig
	t.Cleanup(func() { monsterPkg.MonsterConfig = previousMonsters })
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("loots: %v", err)
	}
	data, _ := clockmaker(t)
	currency, ok := character.CurrencyItemKey(data.Currency)
	if !ok {
		t.Fatalf("clockmaker currency %q is not an item", data.Currency)
	}
	droppers := 0
	for key, def := range monsterPkg.MonsterConfig.Monsters {
		drops := false
		for _, e := range config.GetLootTable(key, def.Boss) {
			drops = drops || e.Key == currency
		}
		if !drops {
			continue
		}
		droppers++
		if def.Type != "construct" {
			t.Errorf("%s drops %s but is type %q, want construct", key, currency, def.Type)
		}
	}
	if droppers == 0 {
		t.Fatalf("no monster drops the %s currency", currency)
	}
	weapons := 0
	for _, entry := range data.Inventory {
		if entry == nil || entry.Type != "weapon" {
			continue
		}
		weapons++
		def, _, ok := config.GetWeaponDefinitionByName(entry.Name)
		if !ok {
			t.Errorf("stock weapon %q missing", entry.Name)
			continue
		}
		if !def.NoLoot {
			t.Errorf("weapon %q must be no_loot (Clockmaker stock and the regal chest only)", entry.Name)
		}
	}
	if weapons == 0 {
		t.Fatal("the Clockmaker sells no weapons (positive control)")
	}
}

// Each set the Clockmaker sells must be wearable as a complete kit: a
// duplicate slot, mixed armor category, or wrong set key would otherwise make
// the advertised set impossible to use in play. Completing it grants exactly
// the set's authored bonuses on top of the pieces' own.
func TestClockTowerArmorSetsEquipAndActivate(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	_, odile := clockmaker(t)
	kits := map[string][]items.Item{}
	for _, m := range odile.MerchantStock {
		if m != nil && m.Item.Type == items.ItemArmor && m.Item.Set != "" {
			kits[m.Item.Set] = append(kits[m.Item.Set], m.Item)
		}
	}
	if len(kits) == 0 {
		t.Fatal("the Clockmaker sells no armor set (positive control)")
	}

	for setKey, pieces := range kits {
		t.Run(setKey, func(t *testing.T) {
			set := config.GetItemSet(setKey)
			if set == nil {
				t.Fatalf("set %q is missing", setKey)
			}
			if got := set.RequiredPieceCount(); got != len(pieces) {
				t.Fatalf("set %q needs %d pieces, the shop sells %d", setKey, got, len(pieces))
			}
			category := pieces[0].ArmorCategory
			skill, ok := character.SkillTypeFromKey(category)
			if !ok {
				t.Fatalf("armor category %q has no skill", category)
			}

			ch := &character.MMCharacter{
				Name:      "Tower Gear Tester",
				Might:     20,
				Endurance: 20,
				Accuracy:  20,
				Speed:     20,
				Luck:      20,
				Skills: map[character.SkillType]*character.Skill{
					skill: {},
				},
				Equipment: make(map[items.EquipSlot]items.Item),
			}
			baseMight, _, _, baseEndurance, baseAccuracy, baseSpeed, baseLuck := ch.GetEffectiveStats()

			// The set's bonus on top of every piece's own flat bonus.
			want := struct{ might, endurance, accuracy, speed, luck int }{
				set.BonusMight, set.BonusEndurance, set.BonusAccuracy, set.BonusSpeed, set.BonusLuck,
			}
			for _, piece := range pieces {
				if piece.ArmorCategory != category {
					t.Fatalf("%s category %q, want the set's %q", piece.Name, piece.ArmorCategory, category)
				}
				if _, hadPrevious, ok := ch.EquipItem(piece); !ok || hadPrevious {
					t.Fatalf("%s should occupy an unused set slot, ok=%v hadPrevious=%v", piece.Name, ok, hadPrevious)
				}
				def, _, _ := config.GetItemDefinitionByName(piece.Name)
				want.might += def.BonusMight
				want.endurance += def.BonusEndurance
				want.accuracy += def.BonusAccuracy
				want.speed += def.BonusSpeed
				want.luck += def.BonusLuck
			}

			if got := len(ch.Equipment); got != len(pieces) {
				t.Fatalf("equipped slots = %d, want %d; tower set has colliding slots", got, len(pieces))
			}
			wantAC := 0
			for _, piece := range ch.Equipment {
				wantAC += cs.CalculateArmorClassContribution(piece, ch)
			}
			if got := cs.CalculateTotalArmorClass(ch); got != wantAC {
				t.Fatalf("total AC = %d, want %d from all equipped pieces", got, wantAC)
			}
			might, _, _, endurance, accuracy, speed, luck := ch.GetEffectiveStats()
			for _, row := range []struct {
				stat      string
				got, want int
			}{
				{"Might", might - baseMight, want.might},
				{"Endurance", endurance - baseEndurance, want.endurance},
				{"Accuracy", accuracy - baseAccuracy, want.accuracy},
				{"Speed", speed - baseSpeed, want.speed},
				{"Luck", luck - baseLuck, want.luck},
			} {
				if row.got != row.want {
					t.Errorf("%s bonus = %d, want %d", row.stat, row.got, row.want)
				}
			}
			if got := ch.SetStunDurationPct(); got != set.StunDurationPct {
				t.Errorf("stun duration bonus = %d, want %d", got, set.StunDurationPct)
			}
		})
	}
}

func TestChronoCapeContributesFlatCloakAC(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	ch.Equipment = make(map[items.EquipSlot]items.Item)
	before := cs.CalculateTotalArmorClass(ch)

	def, ok := config.GetItemDefinition("chrono_cape")
	if !ok || def.ArmorClassBase <= 0 {
		t.Fatalf("chrono_cape must author armor_class_base: %+v", def)
	}
	cape, err := items.TryCreateItemFromYAML("chrono_cape")
	if err != nil {
		t.Fatalf("create Chrono Cape: %v", err)
	}
	if _, _, ok := ch.EquipItem(cape); !ok {
		t.Fatal("Chrono Cape should equip")
	}
	if got := cs.CalculateTotalArmorClass(ch) - before; got != def.ArmorClassBase {
		t.Errorf("Chrono Cape AC contribution = %d, want its flat armor_class_base %d", got, def.ArmorClassBase)
	}
}

// Shop tabs: labels come from authored stock order, the visible slice filters
// by the active tab (the same slice indexes draw AND clicks), and mixed
// tabbed/untabbed stock fails validation.
func TestMerchantShopTabs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	data, odile := clockmaker(t)
	g.dialogNPC = odile

	var want []string
	perTab := map[string]int{}
	for _, entry := range data.Inventory {
		if perTab[entry.Tab] == 0 {
			want = append(want, entry.Tab)
		}
		perTab[entry.Tab]++
	}
	if len(want) < 2 {
		t.Fatalf("the Clockmaker authors %d tabs, the test needs a tabbed shop", len(want))
	}
	tabs := g.merchantShopTabs()
	if !slices.Equal(tabs, want) {
		t.Fatalf("tabs = %v, want authored order %v", tabs, want)
	}
	for ti, label := range want {
		g.dialogTab = ti
		vis := g.merchantVisibleStock()
		if len(vis) != perTab[label] {
			t.Fatalf("tab %s: %d entries, want the %d authored", label, len(vis), perTab[label])
		}
		for _, m := range vis {
			if m.Tab != label {
				t.Fatalf("tab %s leaked entry %q from tab %q", label, m.Item.Name, m.Tab)
			}
		}
	}

	// Untabbed merchants keep the whole stock and no tabs.
	g.dialogTab = len(want) - 1
	plain := &character.NPC{MerchantStock: []*character.MerchantStockItem{{Cost: 1}, {Cost: 2}}}
	g.dialogNPC = plain
	if got := g.merchantShopTabs(); len(got) != 0 {
		t.Fatalf("untabbed merchant grew tabs: %v", got)
	}
	if len(g.merchantVisibleStock()) != 2 {
		t.Fatal("untabbed merchant must show full stock")
	}

	// Mixed authoring fails fast.
	bad := map[string]*character.NPCData{"x": {Inventory: []*character.NPCItem{{Tab: "A"}, {}}}}
	if err := ValidateNPCCommerce(bad); err == nil {
		t.Fatal("mixed tabbed/untabbed stock must fail validation")
	}
}
