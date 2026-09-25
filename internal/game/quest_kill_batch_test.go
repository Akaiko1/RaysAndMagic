package game

import (
	"fmt"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Cross quota semantics, source filtering, map representation and finalization
// entry points. Partial batches keep their goal; a full batch completes once.
// Both modes' autonomous damage shares finalizeIndirectKills after HP updates.
func TestKillQuestBatchCredit(t *testing.T) {
	for _, merged := range []bool{false, true} {
		for _, exterminate := range []bool{false, true} {
			for _, sourceOnly := range []bool{false, true} {
				for _, entry := range []string{"direct", "indirect", "mixed"} {
					for _, killed := range []int{3, 5} {
						t.Run(fmt.Sprintf("merged=%v/all=%v/source=%v/%s/kills=%d", merged, exterminate, sourceOnly, entry, killed), func(t *testing.T) {
							t.Chdir("../..")
							g, wm, cfg := bootOpenWorldGame(t, merged)
							const id = "batch_hunt"
							def := *g.questManager.Definitions()["goblin_hunt"]
							def.Exterminate, def.EncounterOnly = exterminate, sourceOnly
							g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{id: &def}})
							previous := quests.GlobalQuestManager
							quests.GlobalQuestManager = g.questManager
							t.Cleanup(func() { quests.GlobalQuestManager = previous })
							wm.CurrentMapKey = "forest"
							g.world = wm.WorldByKey("forest")
							wm.EachWorld(func(_ string, w *world.World3D) { w.Monsters, w.NPCs = nil, nil })
							g.combat = NewCombatSystem(g)
							ts := cfg.GetTileSize()
							add := func(region string) *monster.Monster3D {
								tx, ty := wm.ProjectTile(region, 2, 2)
								m := monster.NewMonster3DFromConfig((float64(tx)+.5)*ts, (float64(ty)+.5)*ts, "goblin", cfg)
								m.Gold, m.Experience = 0, 0
								if sourceOnly {
									m.IsEncounterMonster = true
									m.EncounterRewards = &monster.EncounterRewards{QuestID: id}
								}
								w := wm.WorldByKey(region)
								w.Monsters = append(w.Monsters, m)
								return m
							}
							var targets []*monster.Monster3D
							for i := 0; i < 5; i++ {
								targets = append(targets, add("forest"))
							}
							add("highlands") // Same name, wrong region.
							ignored := add("forest")
							ignored.QuestProgressIgnored = true
							if sourceOnly {
								wrongSource := add("forest")
								wrongSource.EncounterRewards.QuestID = "other_hunt"
							}
							(&InputHandler{game: g}).handleGiveQuest(id)
							q := g.questManager.GetQuest(id)
							if q == nil || q.Target() != 5 {
								t.Fatalf("acceptance quota: %+v", q)
							}
							for _, m := range targets[:killed] {
								m.HitPoints = 0
							}
							// An ignored death never contributes pending credit either.
							ignored.HitPoints = 0
							gl := &GameLoop{game: g}
							g.reusableDeadSet = make(map[string]bool)
							switch entry {
							case "direct":
								for i, m := range targets[:killed] {
									g.combat.finishMonsterKill(m)
									if q.Target() != 5 || q.CurrentCount != i+1 || q.Completed != (i+1 == 5) {
										t.Fatalf("after credit %d: %+v", i+1, q)
									}
								}
							case "mixed":
								g.combat.finishMonsterKillImmediately(targets[0])
								gl.finalizeIndirectKills()
							case "indirect":
								gl.finalizeIndirectKills()
							}
							gl.finalizeIndirectKills() // Must not credit queued deaths again.
							assertProgress := func() {
								t.Helper()
								q = g.questManager.GetQuest(id)
								if q.Target() != 5 || q.CurrentCount != killed || q.Completed != (killed == 5) {
									t.Fatalf("batch progress=%d/%d completed=%v; want %d/5 completed=%v", q.CurrentCount, q.Target(), q.Completed, killed, killed == 5)
								}
							}
							assertProgress()
							wantAnnouncements := 0
							if killed == 5 {
								wantAnnouncements = 1
							}
							if got := countCombatLog(g, "completed!"); got != wantAnnouncements {
								t.Fatalf("completion announcements=%d want=%d", got, wantAnnouncements)
							}
							save := g.buildSave(wm)
							g.restoreSavedQuests(&save)
							assertProgress()
						})
					}
				}
			}
		}
	}
}
