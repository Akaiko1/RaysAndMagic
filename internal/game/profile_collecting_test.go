package game

import (
	"cmp"
	"maps"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/playerprofile"
	"ugataima/internal/quests"
)

// catalogLegendaryFinds returns n catalog items the legendary statistic
// counts (the Legendary Showcase's list), in key order.
func catalogLegendaryFinds(t *testing.T, n int) []items.Item {
	t.Helper()
	var out []items.Item
	for _, key := range sortedMapKeys(config.GlobalItems.Items) {
		if it, err := items.TryCreateItemFromYAML(key); err == nil && legendaryStampKey(it) != "" {
			if out = append(out, it); len(out) == n {
				return out
			}
		}
	}
	t.Fatalf("the catalog has fewer than %d legendary finds", n)
	return nil
}

// Chest loot counts only what a treasure chest yields. A crate counts by its
// treasure_chest flag alone (the counter reads nothing else of it), so one
// chest and one other prop from the catalog stand for every crate; combat
// mode is never read on this path.
func TestProfileCollectingChestSources(t *testing.T) {
	crateTestGame(t) // loads the crate and NPC catalogs once
	var chest, prop string
	for _, key := range sortedMapKeys(config.GlobalLoots.Crates) {
		if config.GlobalLoots.Crates[key].TreasureChest {
			chest = cmp.Or(chest, key)
		} else {
			prop = cmp.Or(prop, key)
		}
	}
	if chest == "" || prop == "" {
		t.Fatalf("crate catalog has chest %q and prop %q, want both kinds", chest, prop)
	}
	for _, source := range []string{chest, prop, "encounter", "monster", "sealed", "gold only"} {
		t.Run(source, func(t *testing.T) {
			g, _, _ := tbBehaviorGame(t, 12, 12)
			g.turnBasedMode = false
			profilePath := attachStatisticsProfile(t, g)
			relic := catalogLegendaryFinds(t, 1)[0]
			relic.Quantity = 3
			loot := []items.Item{relic, {Name: "Test Card", Type: items.ItemCard, Rarity: "legendary", Quantity: 2}}
			want := int64(5)
			switch source {
			case "encounter", "sealed", "gold only":
				payload := loot
				if source == "gold only" {
					payload = nil
					want = 0
				}
				g.addGroundContainer(GroundContainer{Kind: ContainerKindTreasureChest, Items: payload, Gold: 10})
				if source == "sealed" {
					want = 0
				} else {
					g.pickupGroundContainerAt(0)
					g.pickupGroundContainerAt(0)
				}
			case "monster":
				want = 0
				g.addLootBagDrop(g.camera.X, g.camera.Y, loot, 0)
				g.pickupGroundContainerAt(0)
			default:
				// Only a treasure chest's loot counts as chest loot.
				crate := config.GetCrateConfig(source)
				if !crate.TreasureChest {
					want = 0
				}
				// Force a deterministic catalog reward while retaining authored provenance
				// and driving the real one-shot interaction guard.
				copy := *crate
				copy.Rolls, copy.LootTable, copy.TrapDamage, copy.TrapIgnite, copy.FreeRest = 2, "", 0, false, false
				copy.RollSources = []config.CrateRollSource{{Pool: "catalog", ItemType: "consumable", Weight: 100}}
				config.GlobalLoots.Crates[source] = &copy
				t.Cleanup(func() { config.GlobalLoots.Crates[source] = crate })
				npc := spawnCrate(t, g, source, g.camera.X+64, g.camera.Y)
				g.useLootCrate(npc)
				g.useLootCrate(npc)
				if want > 0 {
					want = 2
				}
			}
			d := &g.playerProfile.Data
			if d.Counters["chest_loot"] != want {
				t.Fatalf("chest units=%d want %d", d.Counters["chest_loot"], want)
			}
			var ranked int64
			for _, e := range d.Top("chest_loot") {
				ranked += e.Count
			}
			if ranked != want {
				t.Fatalf("chest ranking=%d want %d", ranked, want)
			}
			if source == "encounter" && (d.Counters["cards_found"] != 2 || d.Counters["legendary_loot"] != 3 || d.Counters["loot"] != 5) {
				t.Fatal("chest provenance double-counted loot or mixed cards with legendaries")
			}
			assertProfileMetricsPersist(t, g.playerProfile, profilePath)
		})
	}
}

func attachStatisticsProfile(t *testing.T, g *MMGame) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	store, err := playerprofile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	g.playerProfile = store
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return path
}

func assertProfileMetricsPersist(t *testing.T, store *playerprofile.Store, path string) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := playerprofile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !reflect.DeepEqual(store.Data.Counters, restored.Data.Counters) || !reflect.DeepEqual(store.Data.Rankings, restored.Data.Rankings) {
		t.Fatal("statistics changed after profile restart")
	}
}

func TestProfileQuestTurnInDetails(t *testing.T) {
	for _, source := range []string{"journal", "npc", "incomplete", "missing"} {
		t.Run(source, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			profilePath := attachStatisticsProfile(t, g)
			g.playerProfile.Data.Add("quest_rewards", 7) // Keep historical totals intact.
			g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"test": {Name: "Test Errand", Type: quests.QuestTypeKill, Rewards: quests.QuestRewards{Gold: 100, Experience: 200, ArenaPoints: 3}}}})
			if source != "missing" {
				if err := g.questManager.ActivateQuest("test"); err != nil {
					t.Fatal(err)
				}
				if source != "incomplete" {
					g.questManager.MarkCompleted("test")
				}
			}
			for i := 0; i < 2; i++ {
				if source == "npc" {
					(&InputHandler{game: g}).handleTurnInQuest("test")
				} else {
					h.ui.game.claimQuestReward("test", nil)
				}
			}
			want := int64(1)
			if source == "missing" || source == "incomplete" {
				want = 0
			}
			d := &g.playerProfile.Data
			d.ResetAchievements([]string{"kills"})
			for key, n := range map[string]int64{"quest_rewards": 7 + want, "quest_gold": 100 * want, "quest_xp": 200 * want, "quest_arena_points": 3 * want} {
				if d.Counters[key] != n {
					t.Fatalf("%s=%d want %d", key, d.Counters[key], n)
				}
			}
			entry := d.Rankings["quest_rewards"]["test"]
			if entry.Count != want || (want > 0 && entry.Name != "Test Errand") {
				t.Fatalf("quest detail=%+v", entry)
			}
			assertProfileMetricsPersist(t, g.playerProfile, profilePath)
		})
	}
}

func TestProfileSectionsStayWithinThree(t *testing.T) {
	for _, page := range profilePages {
		if len(page.counters) > 3 || len(page.rankings) > 3 {
			t.Fatalf("%s exceeds three sections", page.title)
		}
	}
	for _, res := range campHUDResolutions {
		panel := profilePanelRect(res[0], res[1])
		for i, page := range profilePages {
			r := profileTabRect(panel.x+menuFrameInset, panel.y+menuFrameInset, panel.w-2*menuFrameInset, i)
			if uiTextWidth("[ "+page.title+" ]") > r.w-8 {
				t.Fatalf("%v clips tab %s", res, page.title)
			}
		}
	}
}

// The collecting statistics follow the current catalog: when the profile
// opens they are rebuilt from the complete loot ranking, so an item demoted
// since it was found and a quest item an older rule counted (a dragon
// statuette) leave Legendary loot with their units, an item that is a
// legendary find now joins it, total loot stays, the repair persists, and a
// newly found statuette never counts.
func TestProfileLegendaryLootFollowsCatalogRarity(t *testing.T) {
	g := crateTestGame(t)
	demoted, err := items.TryCreateItemFromYAML("diamond")
	if err != nil || demoted.Rarity == "legendary" {
		t.Fatalf("fixture: diamond must exist below legendary (%v, %q)", err, demoted.Rarity)
	}
	statuette, err := items.TryCreateItemFromYAML("black_dragon_statuette")
	if err != nil || statuette.Type != items.ItemQuest || statuette.Rarity != "legendary" {
		t.Fatalf("fixture: the black dragon statuette must be a legendary quest item (%v)", err)
	}
	finds := catalogLegendaryFinds(t, 2)
	still, promoted := finds[0], finds[1]
	card := items.Item{Name: "Test Card", Type: items.ItemCard}
	path := filepath.Join(t.TempDir(), "profile.json")
	legacy, err := playerprofile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	g.playerProfile = legacy
	still.Quantity = 1
	g.recordProfileLoot([]items.Item{still})
	// As older rules recorded them, each with its loot ranking entry: the
	// diamond before its rebalance and the statuette as legendary loot, the
	// promoted find as ordinary loot.
	for _, old := range []struct {
		it        items.Item
		n         int64
		legendary bool
	}{{demoted, 2, true}, {statuette, 1, true}, {promoted, 3, false}, {card, 2, false}} {
		key, icon := profileItemKey(old.it), itemTooltipIconName(old.it)
		legacy.Data.Add("loot", old.n)
		legacy.Data.Rank("loot", key, old.it.Name, icon, old.n)
		if old.legendary {
			legacy.Data.Add("legendary_loot", old.n)
			legacy.Data.Rank("legendary_loot", key, old.it.Name, icon, old.n)
		}
		if old.it.Type == items.ItemCard {
			legacy.Data.Add("cards_found", old.n)
			legacy.Data.Rank("cards_found", key, old.it.Name, icon, old.n)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	g.playerProfile = nil
	g.openPlayerProfile(path)
	t.Cleanup(func() { g.playerProfile.Close() })
	d := &g.playerProfile.Data
	ranked := d.Rankings["legendary_loot"]
	if d.Counters["legendary_loot"] != 4 || len(ranked) != 2 || ranked[profileItemKey(still)].Count != 1 || ranked[profileItemKey(promoted)].Count != 3 {
		t.Fatalf("legendary loot after the catalog change: %d units, ranking %v", d.Counters["legendary_loot"], ranked)
	}
	if d.Counters["loot"] != 9 {
		t.Fatalf("the repair must leave total loot alone, got %d", d.Counters["loot"])
	}
	if d.Counters["cards_found"] != 2 || d.Rankings["cards_found"][profileItemKey(card)].Count != 2 {
		t.Fatalf("the rebuild lost the cards found: %d units, ranking %v", d.Counters["cards_found"], d.Rankings["cards_found"])
	}
	repaired := playerprofile.Data{Counters: maps.Clone(d.Counters), Rankings: maps.Clone(d.Rankings)}
	assertProfileMetricsPersist(t, g.playerProfile, path)
	g.playerProfile = nil
	g.openPlayerProfile(path) // a second open finds nothing to repair
	d = &g.playerProfile.Data
	if !reflect.DeepEqual(d.Counters, repaired.Counters) || !reflect.DeepEqual(d.Rankings, repaired.Rankings) {
		t.Fatal("a second profile open changed the repaired statistics")
	}
	g.recordProfileLoot([]items.Item{statuette})
	if d.Counters["legendary_loot"] != 4 || d.Counters["loot"] != 10 {
		t.Fatalf("a found statuette: legendary loot %d, loot %d; want 4 and 10", d.Counters["legendary_loot"], d.Counters["loot"])
	}
}
