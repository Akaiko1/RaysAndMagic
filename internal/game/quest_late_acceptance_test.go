package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// A fixed quota never strands a late acceptance: reach the quota or clear its
// eligible roster. Shipped regional quests and global quests share that rule.
func TestLateKillQuestAcceptance(t *testing.T) {
	for _, id := range []string{"goblin_hunt", "wolf_pack", "global_hunt"} {
		for _, merged := range []bool{false, true} {
			for _, entry := range []string{"empty", "last_kill", "quota", "restored_last_kill", "restored_dialogue"} {
				t.Run(fmt.Sprintf("%s/merged=%v/%s", id, merged, entry), func(t *testing.T) {
					t.Chdir("../..")
					g, wm, cfg := bootOpenWorldGame(t, merged)
					catalog := g.questManager
					sourceID := id
					if id == "global_hunt" {
						sourceID = "goblin_hunt"
					}
					def := *catalog.Definitions()[sourceID]
					if id == "global_hunt" {
						def.TargetMap = ""
					} else if def.TargetMap != "forest" {
						t.Fatal("Wenna's hunt must target the forest")
					}
					qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{id: &def}})
					previous := quests.GlobalQuestManager
					quests.GlobalQuestManager = qm
					t.Cleanup(func() { quests.GlobalQuestManager = previous })
					wm.CurrentMapKey = "forest"
					w := wm.WorldByKey("forest")
					other := wm.WorldByKey("highlands")
					wm.EachWorld(func(_ string, live *world.World3D) { live.Monsters = nil })
					g.world = w

					g.questManager = qm
					g.combat = NewCombatSystem(g)
					ih := &InputHandler{game: g}
					ts := cfg.GetTileSize()
					makeTarget := func(mapKey string) *monster.Monster3D {
						tx, ty := wm.ProjectTile(mapKey, 2, 2)
						m := monster.NewMonster3DFromConfig((float64(tx)+0.5)*ts, (float64(ty)+0.5)*ts, def.TargetMonster, cfg)
						m.Gold, m.Experience = 0, 0
						return m
					}
					// Same species outside the region must not block regional completion.
					// A global quest must wait for targets in both regions/worlds.
					outsider := makeTarget("highlands")
					if merged {
						w.Monsters = append(w.Monsters, outsider)
					} else {
						other.Monsters = append(other.Monsters, outsider)
					}
					if id == "global_hunt" && (entry == "empty" || entry == "restored_dialogue") {
						outsider.HitPoints = 0
					}
					ignored := makeTarget("forest")
					ignored.QuestProgressIgnored = true
					w.Monsters = append(w.Monsters, ignored)
					remaining := def.TargetCount - 2
					if entry == "empty" || entry == "restored_dialogue" {
						remaining = 0
					}
					if entry == "quota" {
						remaining = def.TargetCount + 2
					}
					var targets []*monster.Monster3D
					for i := 0; i < remaining; i++ {
						m := makeTarget("forest")
						targets = append(targets, m)
						w.Monsters = append(w.Monsters, m)
					}
					if entry == "restored_last_kill" || entry == "restored_dialogue" {
						// Old saves store an ordinary active counter, not a special fallback flag.
						g.restoreSavedQuests(&GameSave{Quests: []QuestSave{{ID: id, Status: string(quests.QuestStatusActive), CurrentCount: 1}}})
					} else {
						ih.handleGiveQuest(id)
					}
					q := qm.GetQuest(id)
					if q == nil {
						t.Fatal("quest missing")
					}
					wantTarget := remaining
					if id == "global_hunt" && outsider.IsAlive() {
						wantTarget++
					}
					if entry == "restored_last_kill" || entry == "restored_dialogue" {
						wantTarget++
					}
					wantTarget = min(wantTarget, def.TargetCount)
					if q.Target() != wantTarget {
						t.Fatalf("display target=%d want=%d", q.Target(), wantTarget)
					}
					if strings.Contains(q.Description(), "{target_count}") || !strings.Contains(q.GetProgressString(), fmt.Sprintf("/%d ", wantTarget)) {
						t.Fatal("quest copy diverged from resolved count")
					}
					if remaining > 0 && q.Completed {
						t.Fatal("accepted quest completed while eligible targets remain")
					}
					if entry == "restored_dialogue" {
						for _, nested := range []bool{true, false} {
							choices := []*character.NPCDialogueChoice{{Action: "turn_in_quest", QuestID: id}}
							if nested {
								choices = []*character.NPCDialogueChoice{{Action: "info", Choices: choices}}
							}
							ih.openNPCInteraction(&character.NPC{DialogueData: &character.NPCDialogue{Choices: choices}})
							if !q.Completed {
								t.Fatalf("cleared quest not credited by dialogue: nested=%v", nested)
							}
						}
					} else {
						for i, m := range targets {
							if q.Completed {
								break
							}
							m.HitPoints = 0
							g.combat.finishMonsterKill(m)
							if entry == "quota" && q.Completed != (i+1 >= def.TargetCount) {
								t.Fatalf("quota completion after %d kills = %v", i+1, q.Completed)
							}
							if entry != "quota" && i < len(targets)-1 && q.Completed {
								t.Fatal("completed while eligible targets remain below quota")
							}
						}
						if id == "global_hunt" && entry != "quota" && outsider.IsAlive() {
							if q.Completed {
								t.Fatal("global census ignored a living target in another region")
							}
							outsider.HitPoints = 0
							g.combat.finishMonsterKill(outsider)
						}
					}
					if !q.Completed || q.CurrentCount != q.Target() {
						t.Fatalf("quest stranded: count=%d target=%d completed=%v", q.CurrentCount, q.Target(), q.Completed)
					}
					save := g.buildSave(wm)
					g.restoreSavedQuests(&save)
					q = qm.GetQuest(id)
					if q.Target() != wantTarget || !q.Completed {
						t.Fatal("save/load lost the resolved target, including zero")
					}
					if _, err := qm.ClaimRewards(id); err != nil {
						t.Fatalf("cannot claim reward: %v", err)
					}
					if _, err := qm.ClaimRewards(id); err == nil {
						t.Fatal("reward paid twice")
					}
				})
			}
		}
	}
}
