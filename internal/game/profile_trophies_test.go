package game

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/playerprofile"
)

func TestProfileTrophyLootBoundaries(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			recorded bool
			act      func(*MMGame, []items.Item)
		}{
			{"drop then pickup", true, func(g *MMGame, loot []items.Item) {
				g.addLootBagDrop(g.camera.X, g.camera.Y, loot, 0)
				g.pickupGroundContainerAt(0)
				g.pickupGroundContainerAt(0)
			}},
			{"sealed chest", false, func(g *MMGame, loot []items.Item) {
				g.addGroundContainer(GroundContainer{Kind: ContainerKindTreasureChest, Items: loot})
			}},
			{"opened chest", true, func(g *MMGame, loot []items.Item) {
				g.addGroundContainer(GroundContainer{Kind: ContainerKindTreasureChest, Items: loot})
				g.pickupGroundContainerAt(0)
				g.pickupGroundContainerAt(0)
			}},
			{"crate", true, func(g *MMGame, loot []items.Item) { g.grantCrateLoot(&character.NPC{Name: "Crate"}, loot, 0, 0) }},
			{"restored bag", false, func(g *MMGame, loot []items.Item) {
				g.addGroundContainer(GroundContainer{Kind: ContainerKindLootBag, Items: loot})
				g.pickupGroundContainerAt(0)
			}},
			{"inventory transfer", false, func(g *MMGame, loot []items.Item) {
				for _, it := range loot {
					g.party.AddItem(it)
				}
			}},
		} {
			t.Run(fmt.Sprintf("%s/TB=%v", tc.name, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				attachTestProfile(t, g)
				g.turnBasedMode = tb
				tc.act(g, []items.Item{
					{Name: "Legendary Armor", Attributes: map[string]int{"value": 1000}, Type: items.ItemArmor, Rarity: "Legendary"},
					{Name: "Legendary Trophy", Attributes: map[string]int{"value": 500}, Type: items.ItemTrinket, Rarity: "legendary", Quantity: 3},
					{Name: "Legendary Card", Attributes: map[string]int{"value": 2000}, Type: items.ItemCard, Rarity: "legendary", Quantity: 4},
					{Name: "Common Card", Attributes: map[string]int{"value": 50}, Type: items.ItemCard, Rarity: "common", Quantity: 2},
					{Name: "Rare Weapon", Attributes: map[string]int{"value": 700}, Type: items.ItemWeapon, Rarity: "rare"},
				})
				for metric, units := range map[string]int64{"loot": 11, "legendary_loot": 4, "cards_found": 6} {
					if !tc.recorded {
						units = 0
					}
					if got := g.playerProfile.Data.Counters[metric]; got != units {
						t.Fatalf("%s=%d want %d", metric, got, units)
					}
					var total int64
					for _, e := range g.playerProfile.Data.Top(metric) {
						total += e.Count
					}
					if total != units {
						t.Fatalf("%s ranking=%d want %d", metric, total, units)
					}
				}
				spec := profilePages[2].rankings[2]
				if spec.title != "Most valuable finds" || spec.duration {
					t.Fatal("Discoveries does not display the valuable finds ranking")
				}
				valuable := h.ui.profileRankingEntries(spec, &g.playerProfile.Data)
				if !tc.recorded {
					if len(valuable) != 0 {
						t.Fatal("uncommitted loot entered valuable finds")
					}
				} else {
					if len(valuable) != 5 || valuable[0].Name != "Legendary Card" || valuable[0].BaseValue != 2000 || valuable[0].Count != 4 {
						t.Fatalf("incorrect value ranking: %+v", valuable)
					}
					if spec.score(valuable[0]) != 2000 || spec.entryValue(valuable[0]) != "2,000g" {
						t.Fatal("ranking presentation uses quantity instead of unit value")
					}
				}
				for _, e := range g.playerProfile.Data.Top("legendary_loot") {
					if e.Name == "Legendary Card" {
						t.Fatal("card counted as ordinary legendary loot")
					}
				}
			})
		}
	}
}

func TestProfileBossTrophies(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			mob  monster.Monster3D
			want int64
		}{
			{"boss", monster.Monster3D{Boss: true}, 1},
			{"normal", monster.Monster3D{}, 0},
			{"bound boss", monster.Monster3D{Boss: true, Bound: true}, 0},
			{"charmed boss", monster.Monster3D{Boss: true, CharmedByParty: true}, 0},
		} {
			t.Run(fmt.Sprintf("%s/TB=%v", tc.name, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				attachTestProfile(t, g)
				g.turnBasedMode = tb
				tc.mob.ID, tc.mob.Key, tc.mob.Name = "trophy-victim", "old_samurai", "Samurai Warlord"
				g.combat.finishMonsterKill(&tc.mob)
				g.combat.finishMonsterKill(&tc.mob)
				if got := g.playerProfile.Data.Counters["bosses"]; got != tc.want {
					t.Fatalf("boss total=%d want %d", got, tc.want)
				}
				if got := g.playerProfile.Data.Rankings["bosses"][tc.mob.Key].Count; got != tc.want {
					t.Fatalf("boss ranking=%d want %d", got, tc.want)
				}
			})
		}
	}
}

func TestProfileMerchantTradeUnits(t *testing.T) {
	for _, tc := range []struct {
		name, currency, override               string
		n, cost, items, gold, surcharge, stock int
		success                                bool
		want                                   int64
	}{
		{"shop currency", "item:clock_hand", "", 2, 3, 20, 20, 0, 2, true, 6},
		{"entry override", "item:clock_hand", "red_dragon_scale", 2, 3, 20, 20, 4, 2, true, 6},
		{"not enough items", "item:clock_hand", "", 2, 3, 5, 20, 0, 2, false, 0},
		{"not enough gold", "item:clock_hand", "", 2, 3, 20, 7, 4, 2, false, 0},
		{"not enough stock", "item:clock_hand", "", 2, 3, 20, 20, 0, 1, false, 0},
		{"invalid amount", "item:clock_hand", "", 0, 3, 20, 20, 0, 2, false, 0},
		{"free trade", "item:clock_hand", "", 2, 0, 20, 20, 0, 2, true, 0},
		{"gold", "", "", 2, 3, 20, 20, 0, 2, true, 0},
		{"arena", "arena_points", "", 2, 3, 20, 20, 0, 2, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			path := filepath.Join(t.TempDir(), "profile.json")
			store, err := playerprofile.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			g.playerProfile = store
			defer store.Close()
			key := "clock_hand"
			if tc.override != "" {
				key = tc.override
			}
			payment, err := items.TryCreateItemFromYAML(key)
			if err != nil {
				t.Fatal(err)
			}
			payment.Quantity = tc.items
			g.party.Inventory = nil
			g.party.AddItem(payment)
			g.party.Gold, g.party.ArenaPoints = tc.gold, 20
			g.dialogNPC = &character.NPC{Name: "Merchant", Currency: tc.currency}
			entry := &character.MerchantStockItem{Item: items.Item{Name: "Legendary Goods", Type: items.ItemWeapon, Rarity: "legendary"}, Cost: tc.cost, Quantity: tc.stock, CurrencyItem: tc.override, GoldCost: tc.surcharge}
			if got := g.buyMerchantUnits(entry, tc.n); got != tc.success {
				t.Fatalf("success=%v want %v", got, tc.success)
			}
			if store.Data.Counters["items_traded"] != tc.want || store.Data.Rankings["items_traded"][key].Count != tc.want {
				t.Fatalf("incorrect units: %+v", store.Data)
			}
			if store.Data.Counters["legendary_loot"] != 0 {
				t.Fatal("purchased goods counted as drops")
			}
			if !tc.success && g.party.CountItemsByName(payment.Name) != tc.items {
				t.Fatal("failed trade consumed items")
			}
			if tc.success && entry.Quantity == 0 {
				if g.buyMerchantUnits(entry, 1) {
					t.Fatal("empty stock sold twice")
				}
				if store.Data.Counters["items_traded"] != tc.want {
					t.Fatal("refused repeat counted")
				}
			}
			// New metrics share the normal store and remain independent of achievement resets.
			g.combat.finishMonsterKill(&monster.Monster3D{ID: "boss", Key: "old_samurai", Boss: true})
			g.grantCrateLoot(&character.NPC{Name: "Crate"}, []items.Item{{Name: "Card", Type: items.ItemCard, Rarity: "legendary"}, {Name: "Relic", Type: items.ItemTrinket, Rarity: "legendary"}}, 0, 0)
			counters, rankings := store.Data.Counters, store.Data.Rankings
			store.Data.ResetAchievements([]string{"kills"})
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			restored, err := playerprofile.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			if !reflect.DeepEqual(restored.Data.Counters, counters) || !reflect.DeepEqual(restored.Data.Rankings, rankings) {
				t.Fatal("trophy statistics changed on reset/restart")
			}
		})
	}
}

func TestProfileAllTabsUseDisplayedControls(t *testing.T) {
	for _, size := range [][2]int{{800, 680}, {1280, 720}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			attachTestProfile(t, g)
			fp := installFakePointer(t)
			g.appScreen = AppScreenMainMenu
			g.entryMenuMode = EntryMenuStatistics
			for tab, page := range profilePages {
				presentInputScreen(h)
				panel := profilePanelRect(size[0], size[1])
				r := profileTabRect(panel.x+menuFrameInset, panel.y+menuFrameInset, panel.w-2*menuFrameInset, tab)
				fp.moveTo(r.x+r.w/2, r.y+r.h/2)
				fp.press()
				updateInputScreen(h)
				if g.statisticsTab != tab {
					t.Fatalf("%s click selected tab %d", page.title, g.statisticsTab)
				}
				fp.release()
				presentInputScreen(h)
				updateInputScreen(h)
				layout := makeProfileStatsLayout(size[0], size[1], page)
				if layout.counterRows*layout.columns < len(page.counters) || layout.rankRows*layout.columns < len(page.rankings) {
					t.Fatalf("%s clips a metric", page.title)
				}
			}
		})
	}
}
