package game

import (
	"strings"
	"testing"

	"ugataima/internal/monster"
)

func TestKillQuotaKeepsFutureBossesAndStatueSources(t *testing.T) {
	for _, merged := range []bool{false, true} {
		t.Run(map[bool]string{false: "split", true: "merged"}[merged], func(t *testing.T) {
			t.Chdir("../..")
			g, wm, _ := bootOpenWorldGame(t, merged)
			g.combat = NewCombatSystem(g)
			g.reconcileKillQuests()
			final := g.questManager.GetQuest("endgame_triad")
			if final.Completed || final.Target() != 3 {
				t.Fatalf("unspawned final bosses changed target: %+v", final)
			}
			// A killed boss must not remove the two bosses still behind unlocks.
			def := g.questManager.Definitions()["broodmother_nests"]
			sp := def.OnCompleteSpawns[0]
			g.questSpawnsDone = map[string]bool{"broodmother_nests#" + sp.ID: true}
			w := wm.WorldByKey(sp.Map)
			tx, ty := wm.ProjectTile(sp.Map, sp.X, sp.Y)
			ts := g.config.GetTileSize()
			boss := monster.NewMonster3DFromConfig((float64(tx)+.5)*ts, (float64(ty)+.5)*ts, sp.Monster, g.config)
			boss.HitPoints = 0
			g.combat.updateQuestProgress(boss)
			if final.Completed || final.Target() != 3 || final.CurrentCount != 1 {
				t.Fatalf("first boss prematurely resolved finale: %+v", final)
			}
			// The spawn queue counts until its actor joins the world roster.
			boss.HitPoints = boss.MaxHitPoints
			g.pendingQuestSpawns = []pendingQuestSpawn{{world: w, monster: boss}}
			final.CurrentCount = 0
			g.reconcileKillQuests()
			if final.Target() != 3 {
				t.Fatal("queued boss vanished from census")
			}
			g.flushPendingQuestSpawns()
			if count, ok := g.availableKillQuestTargets(final, false); !ok || count != 3 {
				t.Fatalf("flushed boss census=%d valid=%v", count, ok)
			}

			ih := &InputHandler{game: g}
			ih.handleGiveQuest("dragon_slayer")
			hunt := g.questManager.GetQuest("dragon_slayer")
			if hunt.Completed || hunt.Target() != 4 {
				t.Fatalf("unspent seals lost: %+v", hunt)
			}
			// One spent seal without a living dragon represents an old cleared source.
			for _, npc := range g.allLoadedNPCs() {
				if len(npc.Summons) > 0 && npc.Summons[0].QuestID == hunt.ID {
					npc.Visited = true
					break
				}
			}
			g.reconcileKillQuests()
			if hunt.Completed || hunt.Target() != 3 || !strings.Contains(hunt.Description(), "slay 3 Elder Dragons") {
				t.Fatalf("remaining seal quota/copy: %+v", hunt)
			}
			save := g.buildSave(wm)
			g.restoreSavedQuests(&save)
			hunt = g.questManager.GetQuest(hunt.ID)
			if hunt.Target() != 3 {
				t.Fatal("restored seal quota changed")
			}
			// Ordinary same-name dragons never count toward a source-bound hunt.
			plain := monster.NewMonster3DFromConfig(2.5*ts, 2.5*ts, "elder_dragon", g.config)
			plain.HitPoints = 0
			g.combat.updateQuestProgress(plain)
			if hunt.CurrentCount != 0 || hunt.Completed {
				t.Fatal("ordinary dragon advanced statue hunt")
			}
		})
	}
}

func TestRepeatableKillQuotaStartsFresh(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	forest := wm.WorldByKey("forest")
	forest.Monsters = nil
	ts := g.config.GetTileSize()
	tx, ty := wm.ProjectTile("forest", 2, 2)
	add := func(n int) {
		for i := 0; i < n; i++ {
			forest.Monsters = append(forest.Monsters, monster.NewMonster3DFromConfig((float64(tx)+.5)*ts, (float64(ty)+.5)*ts, "forest_spider", g.config))
		}
	}
	ih := &InputHandler{game: g}
	ih.handleGiveQuest("lake_spiders")
	q := g.questManager.GetQuest("lake_spiders")
	if g.creditQuestIfCleared(q.ID) || q.Completed {
		t.Fatal("absent seasonal pack granted free quest rewards")
	}
	add(2)
	g.reconcileKillQuests()
	if q.Target() != 5 {
		t.Fatalf("first night target=%d", q.Target())
	}
	g.questManager.MarkCompleted(q.ID)
	if _, err := g.questManager.ClaimRewards(q.ID); err != nil {
		t.Fatal(err)
	}
	g.refreshRepeatableQuests("night")
	add(8)
	ih.handleGiveQuest(q.ID)
	q = g.questManager.GetQuest(q.ID)
	if q.Target() != 5 || q.CurrentCount != 0 || q.Completed {
		t.Fatalf("new night retained reduced quota: %+v", q)
	}
}
