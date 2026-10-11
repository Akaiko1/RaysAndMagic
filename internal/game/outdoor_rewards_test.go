package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Exercise new reward families through use, not just their YAML descriptions.
func TestOutdoorRewardLifecycle(t *testing.T) {
	for _, key := range []string{"sakura_collectible_leaves", "castle_armor_stand", "forest_hunters_bag", "highlands_mist_collector", "desert_solar_condenser"} {
		t.Run(key, func(t *testing.T) {
			g := crateTestGame(t)
			setTestWorldManager(t, nil)
			npc := spawnCrate(t, g, key, g.camera.X+64, g.camera.Y)
			before := snapshotCrateRewards(g)
			g.useLootCrate(npc)
			if !npc.Visited {
				t.Fatal("object was not consumed")
			}
			crate := config.GetCrateConfig(key)
			if crate.Buff != nil {
				b, ok := g.combatBuffByID("crate:" + key)
				if !ok || b.Frames != crate.Buff.DurationSeconds*g.config.GetTPS() {
					t.Fatal("buff duration missing")
				}
				for _, c := range g.party.Members {
					if c.BuffHPRegenPct != crate.Buff.HPRegenPct || c.BuffManaRegenPct != crate.Buff.ManaRegenPct {
						t.Fatal("party regeneration missing")
					}
				}
			} else {
				after := inventoryUnitsByName(g.party)
				count := 0
				for name, n := range after {
					count += n - before.units[name]
				}
				if key == "forest_hunters_bag" && g.party.Gold > before.gold {
					count = 1
				}
				maxCount := 1
				if key == "forest_hunters_bag" {
					maxCount = 5
				}
				if count < 1 || count > maxCount {
					t.Fatalf("reward units=%d", count)
				}
			}
			after := snapshotCrateRewards(g)
			wm := &world.WorldManager{LoadedMaps: map[string]*world.World3D{"test": g.world}}
			g.restoreSavedNPCs(wm, &GameSave{NPCStates: []NPCSave{{MapKey: "test", Name: npc.Name, X: npc.X, Y: npc.Y, Visited: true}}})
			g.useLootCrate(npc)
			if fmt.Sprint(snapshotCrateRewards(g)) != fmt.Sprint(after) {
				t.Fatal("loaded object paid twice")
			}
		})
	}
}

func TestAncientCalendarUse(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _ := summonTileWorld(t)
			monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
			wm := world.NewWorldManager(g.config)
			wm.CurrentMapKey = "test"
			wm.LoadedMaps = map[string]*world.World3D{"test": g.world}
			setTestWorldManager(t, wm)
			g.turnBasedMode, g.currentTurn = tb, 0
			hero := g.party.Members[0]
			hero.ActionsRemaining = 1
			hero.RTCooldown = 0
			carryInQuickSlot(t, g, 0, 0, items.CreateItemFromYAML("ancient_calendar"))
			it := hero.QuickSlots[0]
			recharge := it.Attributes["use_cooldown_seconds"] * g.config.GetTPS()
			if recharge <= 0 {
				t.Fatal("fixture: calendar has no authored recharge")
			}
			n := len(g.world.Monsters)
			g.useQuickSlot(0, 0)
			if len(g.world.Monsters) != n+1 || hero.QuickSlots[0] != it || it.Count() != 1 || it.DeviceCooldownFrames != recharge {
				t.Fatal("calendar did not summon once and recharge intact")
			}
			if hero.RTCooldown <= 0 || tb && hero.ActionsRemaining != 0 {
				t.Fatal("summon did not spend hero action")
			}
			if !g.world.Monsters[n].QuestProgressIgnored {
				t.Fatal("summon can advance authored quest kills")
			}
			if g.world.Monsters[n].IsAmbient() {
				t.Fatalf("calendar summoned ambient %s", g.world.Monsters[n].Key)
			}
			hero.RTCooldown, hero.ActionsRemaining = 0, 1
			g.currentTurn = 0
			g.useQuickSlot(0, 0)
			if len(g.world.Monsters) != n+1 {
				t.Fatal("cooldown bypassed")
			}
			g.tickDeviceCooldowns(recharge - 1)
			if it.DeviceCooldownFrames != 1 {
				t.Fatal("calendar recharge did not count down frame by frame")
			}
			g.tickDeviceCooldowns(1)
			if it.DeviceCooldownFrames != 0 {
				t.Fatal("calendar failed to recharge")
			}
			// Same failure path as Dead Branch: no world context must not spend the item or action.
			setTestWorldManager(t, nil)
			if g.useDevice(it, 0) || it.DeviceCooldownFrames != 0 || hero.RTCooldown != 0 || hero.ActionsRemaining != 1 {
				t.Fatal("failed summon spent a cost")
			}
		})
	}
	// An ordinary mob and a boss may answer; a fish, wildlife, the caravan and
	// a champion never do.
	pool := summonableMonsterKeys()
	if !monster.MonsterConfig.Monsters["old_samurai"].Boss {
		t.Fatal("fixture: old_samurai is no longer an authored boss")
	}
	for key, want := range map[string]bool{
		"goblin": true, "old_samurai": true,
		"common_carp": false, "desert_rabbit": false, "desert_caravan": false, "hobbit_archer": false,
	} {
		if _, ok := monster.MonsterConfig.Monsters[key]; !ok {
			t.Fatalf("fixture: %s missing from monsters.yaml", key)
		}
		if got := slices.Contains(pool, key); got != want {
			t.Errorf("summon pool holds %s=%v, want %v", key, got, want)
		}
	}
}

func TestOutdoorPassiveOwnership(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _, wm := clockSaveFixture(t)
			g.turnBasedMode = tb
			a, b := g.party.Members[0], g.party.Members[1]
			kit := items.CreateItemFromYAML("personal_medical_kit")
			g.party.Inventory = []items.Item{kit}
			a.Inventory = nil
			b.Inventory = nil
			pulse := func() {
				g.turnBasedSpRegenCount = TurnBasedSpRegenEveryNRounds - 1
				for _, c := range g.party.Members {
					c.MaxHitPoints, c.HitPoints = 1000, 500
					c.RestoreRealtimeRegenProgress(character.ManaRegenIntervalFrames-1, character.ManaRegenIntervalFrames-1)
				}
				if tb {
					g.endPartyTurn()
				} else {
					g.updatePartyClocks()
				}
			}
			pulse()
			if a.HitPoints != 500 || b.HitPoints != 500 {
				t.Fatal("shared bag healed")
			}
			a.Inventory = []items.Item{kit, kit}
			pulse()
			if a.HitPoints != 520 || b.HitPoints != 500 {
				t.Fatal("personal kit ownership or duplicate cap failed")
			}
			a.Inventory = nil
			b.Inventory = []items.Item{kit}
			pulse()
			if a.HitPoints != 500 || b.HitPoints != 520 {
				t.Fatal("transfer retained cached ownership")
			}
			// Save real derived maxima, then check the passive survives item normalization.
			for _, c := range g.party.Members {
				c.RecalculateMaxStatsKeepingCurrent(g.config)
			}
			calendar := items.CreateItemFromYAML("ancient_calendar")
			calendar.DeviceCooldownFrames = 123
			g.party.Inventory = append(g.party.Inventory, calendar)
			saved := auditSaveJSON(t, g.buildSave(wm))
			if err := g.applySave(wm, &saved); err != nil {
				t.Fatal(err)
			}
			if g.party.Members[0].PersonalInventoryRegenPct() != 0 || g.party.Members[1].PersonalInventoryRegenPct() != 2 {
				t.Fatal("save lost personal ownership")
			}
			restored := &g.party.Inventory[len(g.party.Inventory)-1]
			if restored.UseAction != "summon" || restored.DeviceCooldownFrames != 123 || restored.Attributes["summon_distance_tiles"] != calendar.Attributes["summon_distance_tiles"] {
				t.Fatal("save lost calendar state")
			}
			g.tickDeviceCooldowns(123)
			if restored.DeviceCooldownFrames != 0 {
				t.Fatal("restored calendar clock did not wake")
			}

		})
	}
}

func TestAmberTalismanBaseDamage(t *testing.T) {
	g := crateTestGame(t)
	c := g.party.Members[0]
	talisman := items.CreateItemFromYAML("amber_talisman")
	bonus := talismanBaseDamageBonus(t)
	for _, key := range []string{"iron_sword", "magic_dagger", "alien_blaster"} {
		def, ok := config.GetWeaponDefinition(key)
		if !ok {
			t.Fatal(key)
		}
		delete(c.Equipment, items.SlotAmulet)
		before := character.WeaponDamageAtNight(def, c, false)
		c.Equipment[items.SlotAmulet] = talisman
		after := character.WeaponDamageAtNight(def, c, false)
		if after.Base != before.Base+bonus || after.Total != before.Total+bonus {
			t.Fatalf("%s: %v -> %v", key, before, after)
		}
	}
	delete(c.Equipment, items.SlotAmulet)
	c.Inventory = []items.Item{talisman}
	if c.WeaponBaseDamageBonus() != 0 {
		t.Fatal("unequipped talisman still buffs weapons")
	}
}

// talismanBaseDamageBonus is the Amber Talisman's authored weapon bonus.
func talismanBaseDamageBonus(t *testing.T) int {
	t.Helper()
	d, ok := config.GetItemDefinition("amber_talisman")
	if !ok || d.WeaponBaseDamageBonus <= 0 {
		t.Fatal("fixture: amber_talisman has no authored weapon base damage bonus")
	}
	return d.WeaponBaseDamageBonus
}

// The castle armour sets drop from exactly one crate source with a real
// chance and never from a generated loot pool.
func TestOutdoorExclusiveSources(t *testing.T) {
	g := crateTestGame(t)
	for _, key := range []string{"scarlet_crane_do", "night_heron_do", "jade_tortoise_do", "white_fox_do"} {
		d, ok := config.GetItemDefinition(key)
		if !ok || !d.NoLoot {
			t.Fatal("exclusive armour escaped into generated loot pools", key)
		}
		count := 0
		for _, c := range config.GlobalLoots.Crates {
			for _, s := range c.RollSources {
				if s.ItemKey == key {
					count++
					if s.PerMille() <= 0 {
						t.Fatal("exclusive source can never pay", key)
					}
				}
			}
		}
		if count != 1 {
			t.Fatal("exclusive reward has no single crate source", key)
		}
	}
	rings := config.CrateEquipmentSlotKeys("ring", (config.CrateRollSource{}).RarityTiers())
	for key, d := range config.GlobalItems.Items {
		if d.EquipSlot == "ring" && !d.CraftedOnly && !slices.Contains(rings, key) {
			t.Fatal("ring omitted", key)
		}
	}
	// One stack per item source: an unauthored quantity is a single item,
	// a fixed quantity is exactly that stack.
	for _, key := range []string{"health_potion", "mana_potion", "sake_flask"} {
		for _, q := range []struct{ min, max, want int }{{0, 0, 1}, {5, 5, 5}} {
			r := g.rollCrateSource(config.CrateRollSource{Pool: "item", ItemKey: key, QuantityMin: q.min, QuantityMax: q.max})
			if !r.ok || len(r.items) != 1 || r.items[0].Count() != q.want {
				t.Fatalf("%s quantity %d-%d: stack %+v, want one of %d", key, q.min, q.max, r, q.want)
			}
		}
	}
}

func TestRandomSummonedQuestBossIsActive(t *testing.T) {
	g, wm, _ := travelFixture(t)
	original := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = original })
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	catalog := monster.MonsterConfig
	old := catalog.Monsters
	catalog.Monsters = maps.Clone(old)
	t.Cleanup(func() { catalog.Monsters = old })
	// Restrict the roll, retaining the real Samurai definition and spawn path.
	for key := range catalog.Monsters {
		if key != "old_samurai" {
			delete(catalog.Monsters, key)
		}
	}
	if !g.SummonRandomMonsterNearPlayer(2) {
		t.Fatal("random summon failed")
	}
	m := g.world.Monsters[len(g.world.Monsters)-1]
	g.refreshMonsterAIState()
	if m.BossDormant || m.BossEvasive || g.combat.bossEvasive(m) {
		t.Fatal("summoned Samurai inherited the map quest gate")
	}
	snapshot := g.buildSave(wm)
	w := newTestWorldSized(g.config, g.world.Width, g.world.Height)
	wm.LoadedMaps["forest"] = w
	loaded := newTestGame(g.config, w)
	if err := loaded.applySave(wm, &snapshot); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, saved := range w.Monsters {
		if saved.ID == m.ID {
			found = true
			if saved.BossDormant || saved.BossEvasive {
				t.Fatal("load sealed the summoned boss")
			}
		}
	}
	if !found {
		t.Fatal("summoned Samurai disappeared on load")
	}
	m.QuestProgressIgnored = false
	if !g.combat.bossEvasive(m) {
		t.Fatal("authored Samurai lost its quest gate")
	}
}

// The collection and equipment feed the same base, including live previews,
// night scaling, roster changes and restoration. Inventory alone grants nothing.
func TestCardWeaponBaseDamageLifecycle(t *testing.T) {
	g, _, wm := clockSaveFixture(t)
	g.resetCardCollection()
	for _, c := range g.party.Members {
		c.Equipment = map[items.EquipSlot]items.Item{}
	}
	card := items.CreateItemFromYAML("kitsune_onmyoji_card")
	g.party.Inventory = []items.Item{card}
	check := func(wantCard int) {
		t.Helper()
		for _, c := range g.party.Members {
			if c.BonusWeaponBaseDamage != wantCard {
				t.Fatalf("%s card bonus=%d, want %d", c.Name, c.BonusWeaponBaseDamage, wantCard)
			}
			for _, key := range []string{"iron_sword", "magic_dagger", "alien_blaster", "duskneedle"} {
				def, _ := config.GetWeaponDefinition(key)
				weapon := items.CreateWeaponFromYAML(key)
				for _, night := range []bool{false, true} {
					g.dayNightIsNight = night
					want := def.Damage + wantCard
					if _, ok := c.Equipment[items.SlotAmulet]; ok {
						want += talismanBaseDamageBonus(t)
					}
					if night && key == "duskneedle" {
						want *= 2
					}
					breakdown := character.WeaponDamageAtNight(def, c, night)
					if breakdown.Base != want {
						t.Fatalf("%s/%s/night=%v base=%d, want %d", c.Name, key, night, breakdown.Base, want)
					}
					_, _, actual := g.combat.CalculateWeaponDamage(weapon, c)
					if actual != breakdown.Total || g.combat.calculateWeaponDamagePreview(weapon, c).FormulaNormal != actual {
						t.Fatalf("%s combat and live preview diverged", key)
					}
				}
			}
		}
		g.dayNightIsNight = false
	}
	check(0)
	if !g.placeCardFromInventory(0) {
		t.Fatal("collect card")
	}
	check(15)
	if g.cardResistBonusFor("mind") != 15 || g.cardResistBonusFor("spirit") != 15 {
		t.Fatal("resistances changed")
	}
	def, _ := config.GetItemDefinition("kitsune_onmyoji_card")
	for _, lines := range [][]string{def.CardCollectionLines(), g.cardCollectionEffectLines()} {
		if !strings.Contains(strings.Join(lines, "\n"), "Party weapons: +15 base damage") {
			t.Fatalf("missing bonus in tooltip: %v", lines)
		}
	}
	oldBonus := def.WeaponBaseDamageBonus
	t.Cleanup(func() { def.WeaponBaseDamageBonus = oldBonus })
	// Authored values reach the runtime; there is no Kitsune-specific constant.
	def.WeaponBaseDamageBonus = 17
	g.recomputeStatBonuses()
	check(17)
	def.WeaponBaseDamageBonus = 15
	g.recomputeStatBonuses()
	g.party.Members[0].Equipment[items.SlotAmulet] = items.CreateItemFromYAML("amber_talisman")
	check(15)
	g.party.Inventory = []items.Item{items.CreateItemFromYAML("kitsune_onmyoji_card")}
	if !g.placeCardFromInventory(0) {
		t.Fatal("collect second card")
	}
	check(30)
	if !strings.Contains(strings.Join(g.cardCollectionEffectLines(), "\n"), "Party weapons: +30 base damage") {
		t.Fatal("aggregate tooltip failed")
	}
	saved := auditSaveJSON(t, g.buildSave(wm))
	if err := g.applySave(wm, &saved); err != nil {
		t.Fatal(err)
	}
	check(30)
	bench := character.CreateCharacter("Reserve", character.ClassKnight, g.config)
	g.party.Recruit(bench)
	outgoing := g.party.Members[0]
	if !g.swapRosterMember(0, len(g.party.Reserve)-1) {
		t.Fatal("roster swap")
	}
	if outgoing.BonusWeaponBaseDamage != 0 {
		t.Fatal("benched member kept party bonus")
	}
	check(30)
	if !g.removeCardToInventory(1) {
		t.Fatal("remove second card")
	}
	check(15)
	if !g.removeCardToInventory(0) {
		t.Fatal("remove first card")
	}
	check(0)
}

// The runtime must forward authored rarity to the same pool as validation.
// The empty unique gate catches a dropped filter deterministically.
func TestEquipmentSlotRollUsesRarity(t *testing.T) {
	g := crateTestGame(t)
	old := config.GlobalItems
	t.Cleanup(func() { config.GlobalItems = old })
	config.GlobalItems = &config.ItemSystemConfig{Items: map[string]*config.ItemDefinitionConfig{
		"test_ring": {Name: "Test Ring", Type: "accessory", EquipSlot: "ring", Rarity: "rare"},
	}}
	for _, tc := range []struct {
		rarity string
		want   bool
	}{{"", true}, {"rare", true}, {"uncommon+", true}, {"unique", false}} {
		r := g.rollCrateSource(config.CrateRollSource{Pool: "equipment_slot", EquipSlot: "ring", Rarity: tc.rarity})
		if r.ok != tc.want {
			t.Fatalf("rarity=%q: roll=%+v", tc.rarity, r)
		}
		if r.ok && (len(r.items) != 1 || r.items[0].Name != "Test Ring") {
			t.Fatalf("wrong slot reward: %+v", r)
		}
	}
}
