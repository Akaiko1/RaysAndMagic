package game

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/playerprofile"
	"ugataima/internal/quests"
)

func TestProfileCollectingChestSources(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, source := range []string{"chest_wooden", "chest_iron", "chest_golden", "chest_gearwood", "chest_chrono", "chest_regal", "pile_of_old_boxes", "campfire", "encounter", "monster", "sealed", "gold only"} {
			t.Run(fmt.Sprintf("%s/TB=%v", source, tb), func(t *testing.T) {
				g := crateTestGame(t)
				profilePath := attachStatisticsProfile(t, g)
				g.turnBasedMode = tb
				loot := []items.Item{{Name: "Test Relic", Type: items.ItemTrinket, Rarity: "legendary", Quantity: 3}, {Name: "Test Card", Type: items.ItemCard, Rarity: "legendary", Quantity: 2}}
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
					crate := config.GetCrateConfig(source)
					if source == "pile_of_old_boxes" || source == "campfire" {
						want = 0
						if crate.TreasureChest {
							t.Fatal("non-chest tagged as chest")
						}
					} else if !crate.TreasureChest {
						t.Fatal("authored chest lacks provenance")
					}
					// Force a deterministic catalog reward while retaining authored provenance
					// and driving the real one-shot interaction guard.
					copy := *crate
					copy.Rolls, copy.LootTable, copy.TrapDamage, copy.TrapIgnite, copy.FreeRest = 2, "", 0, false, false
					copy.RollSources = []config.CrateRollSource{{Pool: "catalog", ItemType: "consumable", Weight: 1}}
					copy.SpecialRolls = nil
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
					h.ui.claimQuestReward("test")
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
			if debugTextWidth("[ "+page.title+" ]") > r.w-8 {
				t.Fatalf("%v clips tab %s", res, page.title)
			}
		}
	}
}
