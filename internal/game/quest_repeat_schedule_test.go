package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

// Invariant: only a completed AND claimed quest resets when its authored event
// or full elapsed interval is due. Cases cross RT/TB, natural/paid boundaries,
// active/unclaimed/claimed state, once/day/night/Nd, and save/load below.
func TestRepeatQuestPhaseSchedules(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, paid := range []bool{false, true} {
			for _, schedule := range []quests.RepeatSchedule{"", "day", "night", "3d"} {
				for _, state := range []string{"active", "unclaimed", "claimed"} {
					t.Run(fmt.Sprintf("TB=%v/paid=%v/%s/%s", tb, paid, schedule, state), func(t *testing.T) {
						g, wm := dailyTimerTestGame(t)
						g.turnBasedMode = tb
						def := &quests.QuestDefinition{Name: "Errand", Type: quests.QuestTypeKill, TargetMonster: "wolf", TargetCount: 5, Repeatable: schedule}
						g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{"errand": def}})
						if err := g.questManager.ActivateQuest("errand"); err != nil {
							t.Fatal(err)
						}
						if state != "active" {
							g.questManager.MarkCompleted("errand")
						}
						if state == "claimed" {
							if !g.claimQuestReward("errand") {
								t.Fatal("claim failed")
							}
						}
						npc := &character.NPC{Visited: true, DialogueData: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{Action: "give_quest", QuestID: "errand"}}}}
						wm.GetCurrentWorld().NPCs = append(wm.GetCurrentWorld().NPCs, npc)
						for _, night := range []bool{true, false} {
							if paid {
								g.advanceDayNightToPhase(night)
								g.finishDayNightSkipImmediately()
							} else {
								g.dayNightFrames = g.dayNightCycleFrames() / 4
								if !night {
									g.dayNightFrames *= 3
								}
								g.updateDayNight()
							}
							wantReset := state == "claimed" && (schedule == "night" || (schedule == "day" && !night))
							if (g.questManager.GetQuest("errand") == nil) != wantReset {
								t.Fatalf("night=%v: reset differs", night)
							}
							if wantReset && npc.Visited {
								t.Fatal("giver remained concluded")
							}
						}
					})
				}
			}
		}
	}
}

func TestRepeatQuestFullDaysSurviveSaveLoad(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/legacy=%v", tb, legacy), func(t *testing.T) {
				g, wm := dailyTimerTestGame(t)
				g.turnBasedMode = tb
				g.questManager = loadTestQuestManager(t)
				if err := g.questManager.ActivateQuest("toll_of_blades"); err != nil {
					t.Fatal(err)
				}
				g.questManager.MarkCompleted("toll_of_blades")
				if !g.claimQuestReward("toll_of_blades") {
					t.Fatal("claim failed")
				}
				if g.claimQuestReward("toll_of_blades") {
					t.Fatal("duplicate claim succeeded")
				}
				claim := g.questManager.GetQuest("toll_of_blades").ClaimedAtDay
				if claim <= 0 {
					t.Fatal("claim timestamp missing")
				}
				save := g.buildSave(wm)
				if legacy {
					for i := range save.Quests {
						save.Quests[i].ClaimedAtDay = 0
					}
				}
				encoded, err := json.Marshal(save)
				if err != nil {
					t.Fatal(err)
				}
				var loaded GameSave
				if err = json.Unmarshal(encoded, &loaded); err != nil {
					t.Fatal(err)
				}
				g.restoreSavedQuests(&loaded)
				if got := g.questManager.GetQuest("toll_of_blades").ClaimedAtDay; got != claim {
					t.Fatalf("restored claim=%v, want %v", got, claim)
				}
				// Three dawns are less than three full days after a noon claim.
				for i := 0; i < 3; i++ {
					g.advanceDayNightToPhase(true)
					g.finishDayNightSkipImmediately()
					g.advanceDayNightToPhase(false)
					g.finishDayNightSkipImmediately()
				}
				if g.questManager.GetQuest("toll_of_blades") == nil {
					t.Fatal("reset early at third dawn")
				}
				// One tick before the original noon, then exactly the same time of day.
				g.dayNightFrames = g.dayNightCycleFrames() - 2
				g.updateDayNight()
				if g.questManager.GetQuest("toll_of_blades") == nil {
					t.Fatal("reset before full interval")
				}
				g.updateDayNight()
				if g.questManager.GetQuest("toll_of_blades") != nil {
					t.Fatal("not reset after three complete cycles")
				}
			})
		}
	}
}
