package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Resolution paths cross live/restored state and both combat modes. Completed
// snapshots are silent; retries are no-ops; reactivated repeatables count anew.
// Profile reopening checks persistence independently of the game's save slot.
func TestProfileQuestResolutionPaths(t *testing.T) {
	for _, source := range []string{"journal", "npc", "encounter", "auto kill", "auto census", "auto prop", "auto arena", "archmage", "archmage picker"} {
		for _, restored := range []bool{false, true} {
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/restored=%v/TB=%v", source, restored, tb), func(t *testing.T) {
					h := newDisplayedModalHarness(t, 1024, 768)
					g := h.g
					path := attachStatisticsProfile(t, g)
					g.turnBasedMode = tb
					g.playerProfile.Data.Add("quest_rewards", 7)
					id := "profile_resolution"
					def := &quests.QuestDefinition{Name: "Test Resolution", Type: quests.QuestTypeInteract,
						TargetMonster: "test_target", TargetCount: 1}
					needsClaim := source == "journal" || source == "npc"
					promotion := source == "archmage" || source == "archmage picker"
					if needsClaim || source == "encounter" {
						def.Rewards = quests.QuestRewards{Gold: 100, Experience: 200, ArenaPoints: 3}
					}
					if source == "encounter" {
						def.Type = quests.QuestTypeEncounter
						def.Rewards.ArenaPoints = 0
					}
					if source == "auto kill" || source == "auto census" {
						def.Type = quests.QuestTypeKill
						def.AutoClaim = true
						def.Exterminate = source == "auto census"
					}
					if source == "auto prop" {
						def.AutoClaim = true
					}
					if source == "auto arena" {
						def.AutoClaim, def.TargetMonster = true, quests.ArenaDuelTag
						world.GlobalWorldManager.MapConfigs = map[string]*config.MapConfig{
							"forest": {Duel: &config.MapDuelConfig{}},
						}
					}
					if promotion {
						id = "archmage_trial"
						hero := g.party.Members[sorcererIndex(t, g)]
						g.party.Members = []*character.MMCharacter{hero}
						if source == "archmage picker" {
							second := *hero
							g.party.Members = append(g.party.Members, &second)
						}
						t.Chdir("../..") // Promotion eligibility uses shipped portrait assets.
						if !g.canPromoteArchmage(hero) {
							t.Fatal("fixture lacks an eligible Archmage portrait")
						}
					}
					g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{id: def}})
					previous := quests.GlobalQuestManager
					quests.GlobalQuestManager = g.questManager
					t.Cleanup(func() { quests.GlobalQuestManager = previous })
					target := &monster.Monster3D{Name: "Test Target", HitPoints: 1}
					g.world.Monsters = []*monster.Monster3D{target}
					ready := func() {
						t.Helper()
						if restored {
							status := quests.QuestStatusActive
							if needsClaim || promotion {
								status = quests.QuestStatusCompleted
							}
							g.restoreSavedQuests(&GameSave{Quests: []QuestSave{{ID: id, Status: string(status)}}})
						} else if source == "encounter" {
							g.questManager.CreateEncounterQuest(id, def.Name, "", def.Rewards.Gold, def.Rewards.Experience)
						} else {
							if err := g.questManager.ActivateQuest(id); err != nil {
								t.Fatal(err)
							}
							if needsClaim || promotion {
								g.questManager.MarkCompleted(id)
							}
						}
					}
					action := func() {
						switch source {
						case "journal":
							h.ui.claimQuestReward(id)
						case "npc", "archmage", "archmage picker":
							h.loop.inputHandler.handleTurnInQuest(id)
						case "encounter":
							h.loop.awardEncounterRewards(&monster.EncounterRewards{QuestID: id})
						case "auto kill":
							target.HitPoints = 0
							g.combat.updateQuestProgress(target)
						case "auto census":
							target.HitPoints = 0
							g.reconcileKillQuests()
						case "auto prop":
							g.dialogNPC = &character.NPC{}
							h.loop.inputHandler.handleQuestPropInteract(id, &character.NPCPropCopy{Tag: def.TargetMonster})
						case "auto arena":
							g.creditArenaDuelWin(&monster.Monster3D{ChampionTier: "novice"})
						}
					}
					assertCount := func(n int64) {
						t.Helper()
						d := &g.playerProfile.Data
						for key, want := range map[string]int64{"quest_rewards": 7 + n, "quest_gold": n * int64(def.Rewards.Gold), "quest_xp": n * int64(def.Rewards.Experience), "quest_arena_points": n * int64(def.Rewards.ArenaPoints)} {
							if got := d.Counters[key]; got != want {
								t.Fatalf("%s=%d, want %d", key, got, want)
							}
						}
						entry := d.Rankings["quest_rewards"][id]
						if entry.Count != n || (n > 0 && entry.Name != def.Name) {
							t.Fatalf("quest ranking=%+v, want count=%d", entry, n)
						}
					}
					ready()
					assertCount(0)
					action()
					action()
					assertCount(1)
					if source == "archmage" && !g.party.Members[0].IsArchmage() {
						t.Fatal("turn-in did not promote the hero")
					}
					if source == "archmage picker" && !g.promotionPickerOpen {
						t.Fatal("turn-in did not open the promotion picker")
					}
					if !promotion {
						q := g.questManager.GetQuest(id)
						if !q.Completed || !q.RewardsClaimed {
							t.Fatalf("quest did not resolve: %+v", q)
						}
						if g.party.Gold != def.Rewards.Gold || g.party.ArenaPoints != def.Rewards.ArenaPoints {
							t.Fatal("quest payout missing or duplicated")
						}
						// The saved completed state must neither emit statistics on load
						// nor become claimable again. No profile totals are in the save.
						save := g.buildSave(world.GlobalWorldManager)
						g.restoreSavedQuests(&save)
						action()
						assertCount(1)
						// A new quest instance is a legitimate repeatable resolution.
						g.questManager.RemoveQuest(id)
						target.HitPoints = 1
						ready()
						action()
						action()
						assertCount(2)
					} else {
						save := g.buildSave(world.GlobalWorldManager)
						g.restoreSavedQuests(&save)
						action()
						assertCount(1)
					}
					assertProfileMetricsPersist(t, g.playerProfile, path)
				})
			}
		}
	}
}

func TestProfileRejectedArchmageTurnIn(t *testing.T) {
	for _, reason := range []string{"incomplete", "ineligible", "lich"} {
		t.Run(reason, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			path := attachStatisticsProfile(t, g)
			g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
				"archmage_trial": {Name: "Trial", Type: quests.QuestTypeInteract},
			}})
			if err := g.questManager.ActivateQuest("archmage_trial"); err != nil {
				t.Fatal(err)
			}
			if reason != "incomplete" {
				g.questManager.MarkCompleted("archmage_trial")
			}
			for _, hero := range g.party.Members {
				hero.Promotion = character.PromotionArchmage
				if reason == "lich" {
					hero.Promotion = character.PromotionLich
				}
			}
			for i := 0; i < 2; i++ {
				h.loop.inputHandler.handleTurnInQuest("archmage_trial")
			}
			if g.questManager.GetQuest("archmage_trial") == nil || g.playerProfile.Data.Counters["quest_rewards"] != 0 || len(g.playerProfile.Data.Top("quest_rewards")) != 0 {
				t.Fatal("rejected promotion consumed or counted the quest")
			}
			assertProfileMetricsPersist(t, g.playerProfile, path)
		})
	}
}
