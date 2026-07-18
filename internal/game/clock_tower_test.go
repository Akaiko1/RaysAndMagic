package game

import (
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
	good := map[string]*character.NPCData{
		"a": {Currency: ""},
		"b": {Currency: "arena_points"},
		"c": {Currency: "item:clock_hand"},
		"d": {GridSpanTiles: 2, GridSpanDir: "e"},
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
		{"x": {GridSpanTiles: 2, GridSpanDir: "e", SizeTiles: 2}},
		{"x": {GridSpanTiles: 2, GridSpanDir: "e", SizeClass: "large"}},
		{"x": {GridSpanTiles: 2, GridSpanDir: "e", NoSpin: true}},
	}
	for i, m := range bad {
		if err := ValidateNPCCommerce(m); err == nil {
			t.Errorf("bad set %d accepted", i)
		}
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

// The tower monsters exist with attack sheets on disk (the generic
// AttackAnimFrames path plays <sprite>_attacking_r for ANY monster that has
// one), and every Clockmaker stock entry resolves to a real item.
func TestClockTowerContentIntegrity(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	if monsterPkg.MonsterConfig == nil {
		monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	}
	for _, key := range []string{"dust_slime", "possessed_tome", "alarm_clock", "grandfather_clock"} {
		def, ok := monsterPkg.MonsterConfig.Monsters[key]
		if !ok {
			t.Errorf("monster %s missing", key)
			continue
		}
		if def.Type != "construct" {
			t.Errorf("%s type = %q, want construct", key, def.Type)
		}
	}
	for _, k := range []string{"clock_hand", "clock_oil",
		"cogleather_helm", "cogleather_jacket", "cogleather_pauldrons", "cogleather_gloves", "cogleather_boots",
		"chainwork_helm", "chainwork_hauberk", "chainwork_pauldrons", "chainwork_gauntlets", "chainwork_greaves",
		"clockplate_helm", "clockplate_cuirass", "clockplate_pauldrons", "clockplate_gauntlets", "clockplate_greaves", "chrono_cape"} {
		if _, err := items.TryCreateItemFromYAML(k); err != nil {
			t.Errorf("item %s: %v", k, err)
		}
	}
	for _, k := range []string{"cogfang_blade", "chime_maul", "minute_hand", "mainspring_pike",
		"escapement_mace", "clockwork_pistol"} {
		def, ok := config.GetWeaponDefinition(k)
		if !ok || def == nil {
			t.Errorf("weapon %s missing", k)
			continue
		}
		if !def.NoLoot {
			t.Errorf("weapon %s must be no_loot (Clockmaker-only stock)", k)
		}
	}
}

// Each tower set must be wearable as a complete five-piece kit: a duplicate
// slot, wrong armor category, or wrong set key would otherwise make the
// Clockmaker's advertised set impossible to use in play.
func TestClockTowerArmorSetsEquipAndActivate(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cases := []struct {
		name, set, category string
		skill               character.SkillType
		pieces              []string
		might               int
		endurance           int
		accuracy            int
		speed               int
		luck                int
		stunDurationPct     int
	}{
		{
			name: "cogleather", set: "cogleather", category: "leather", skill: character.SkillLeather,
			pieces:   []string{"cogleather_helm", "cogleather_jacket", "cogleather_pauldrons", "cogleather_gloves", "cogleather_boots"},
			accuracy: 3, speed: 21, luck: 9,
		},
		{
			name: "chainwork", set: "chainwork", category: "chain", skill: character.SkillChain,
			pieces: []string{"chainwork_helm", "chainwork_hauberk", "chainwork_pauldrons", "chainwork_gauntlets", "chainwork_greaves"},
			might:  2, endurance: 13, accuracy: 17,
		},
		{
			name: "clockplate", set: "clockplate", category: "plate", skill: character.SkillPlate,
			pieces: []string{"clockplate_helm", "clockplate_cuirass", "clockplate_pauldrons", "clockplate_gauntlets", "clockplate_greaves"},
			might:  23, endurance: 3, stunDurationPct: -50,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := config.GetItemSet(tc.set)
			if set == nil {
				t.Fatalf("set %q is missing", tc.set)
			}
			if got := set.PiecesRequired; got != len(tc.pieces) {
				t.Fatalf("set %q pieces_required = %d, want %d", tc.set, got, len(tc.pieces))
			}

			ch := &character.MMCharacter{
				Name:      "Tower Gear Tester",
				Might:     20,
				Endurance: 20,
				Accuracy:  20,
				Speed:     20,
				Luck:      20,
				Skills: map[character.SkillType]*character.Skill{
					tc.skill: {},
				},
				Equipment: make(map[items.EquipSlot]items.Item),
			}
			baseMight, _, _, baseEndurance, baseAccuracy, baseSpeed, baseLuck := ch.GetEffectiveStats()

			for _, key := range tc.pieces {
				piece, err := items.TryCreateItemFromYAML(key)
				if err != nil {
					t.Fatalf("create %s: %v", key, err)
				}
				if piece.Type != items.ItemArmor || piece.ArmorCategory != tc.category || piece.Set != tc.set {
					t.Fatalf("%s = type %v, category %q, set %q; want armor, %q, %q", key, piece.Type, piece.ArmorCategory, piece.Set, tc.category, tc.set)
				}
				if _, hadPrevious, ok := ch.EquipItem(piece); !ok || hadPrevious {
					t.Fatalf("%s should occupy an unused set slot, ok=%v hadPrevious=%v", key, ok, hadPrevious)
				}
			}

			if got := len(ch.Equipment); got != len(tc.pieces) {
				t.Fatalf("equipped slots = %d, want %d; tower set has colliding slots", got, len(tc.pieces))
			}
			wantAC := 0
			for _, piece := range ch.Equipment {
				wantAC += cs.CalculateArmorClassContribution(piece, ch)
			}
			if got := cs.CalculateTotalArmorClass(ch); got != wantAC {
				t.Fatalf("total AC = %d, want %d from all five equipped pieces", got, wantAC)
			}
			might, _, _, endurance, accuracy, speed, luck := ch.GetEffectiveStats()
			if got := might - baseMight; got != tc.might {
				t.Errorf("Might bonus = %d, want %d", got, tc.might)
			}
			if got := endurance - baseEndurance; got != tc.endurance {
				t.Errorf("Endurance bonus = %d, want %d", got, tc.endurance)
			}
			if got := accuracy - baseAccuracy; got != tc.accuracy {
				t.Errorf("Accuracy bonus = %d, want %d", got, tc.accuracy)
			}
			if got := speed - baseSpeed; got != tc.speed {
				t.Errorf("Speed bonus = %d, want %d", got, tc.speed)
			}
			if got := luck - baseLuck; got != tc.luck {
				t.Errorf("Luck bonus = %d, want %d", got, tc.luck)
			}
			if got := ch.SetStunDurationPct(); got != tc.stunDurationPct {
				t.Errorf("stun duration bonus = %d, want %d", got, tc.stunDurationPct)
			}
		})
	}
}

func TestChronoCapeContributesFlatCloakAC(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	ch.Equipment = make(map[items.EquipSlot]items.Item)
	before := cs.CalculateTotalArmorClass(ch)

	cape, err := items.TryCreateItemFromYAML("chrono_cape")
	if err != nil {
		t.Fatalf("create Chrono Cape: %v", err)
	}
	if _, _, ok := ch.EquipItem(cape); !ok {
		t.Fatal("Chrono Cape should equip")
	}
	if got := cs.CalculateTotalArmorClass(ch) - before; got != 4 {
		t.Errorf("Chrono Cape AC contribution = %d, want 4", got)
	}
}

// Shop tabs: labels come from authored stock order, the visible slice filters
// by the active tab (the same slice indexes draw AND clicks), and mixed
// tabbed/untabbed stock fails validation.
func TestMerchantShopTabs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	if character.NPCConfigInstance == nil {
		if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
			t.Fatalf("npcs: %v", err)
		}
	}
	odile, err := character.CreateNPCFromConfig("clockmaker", 0, 0)
	if err != nil {
		t.Fatalf("clockmaker: %v", err)
	}
	g.dialogNPC = odile

	tabs := g.merchantShopTabs()
	want := []string{"Leather", "Chainmail", "Plate", "Arms&Stuff"}
	if len(tabs) != len(want) {
		t.Fatalf("tabs = %v, want %v", tabs, want)
	}
	for i := range want {
		if tabs[i] != want[i] {
			t.Fatalf("tabs = %v, want %v", tabs, want)
		}
	}
	for ti, label := range want {
		g.dialogTab = ti
		vis := g.merchantVisibleStock()
		if len(vis) == 0 {
			t.Fatalf("tab %s: empty stock", label)
		}
		for _, m := range vis {
			if m.Tab != label {
				t.Fatalf("tab %s leaked entry %q from tab %q", label, m.Item.Name, m.Tab)
			}
		}
	}
	g.dialogTab = 3
	if len(g.merchantVisibleStock()) != 8 { // 6 weapons + cape + oil
		t.Fatalf("Arms tab = %d entries, want 8", len(g.merchantVisibleStock()))
	}

	// Untabbed merchants keep the whole stock and no tabs.
	g.dialogTab = 2
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
