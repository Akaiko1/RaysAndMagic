package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
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
			bosses := final.Definition.TargetCount
			if final.Completed || final.Target() != bosses {
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
			if final.Completed || final.Target() != bosses || final.CurrentCount != 1 {
				t.Fatalf("first boss prematurely resolved finale: %+v", final)
			}
			// The spawn queue counts until its actor joins the world roster.
			boss.HitPoints = boss.MaxHitPoints
			g.pendingQuestSpawns = []pendingQuestSpawn{{world: w, monster: boss}}
			final.CurrentCount = 0
			g.reconcileKillQuests()
			if final.Target() != bosses {
				t.Fatal("queued boss vanished from census")
			}
			g.flushPendingQuestSpawns()
			if count, ok := g.availableKillQuestTargets(final, false); !ok || count != bosses {
				t.Fatalf("flushed boss census=%d valid=%v", count, ok)
			}

			ih := &InputHandler{game: g}
			ih.handleGiveQuest("dragon_slayer")
			hunt := g.questManager.GetQuest("dragon_slayer")
			var seals []*character.NPC
			for _, npc := range g.allLoadedNPCs() {
				if len(npc.Summons) > 0 && npc.Summons[0].QuestID == hunt.ID {
					seals = append(seals, npc)
				}
			}
			if len(seals) < 2 {
				t.Fatalf("%s has %d placed seals, want several", hunt.ID, len(seals))
			}
			if hunt.Completed || hunt.Target() != len(seals) {
				t.Fatalf("unspent seals lost: target %d, want %d seals", hunt.Target(), len(seals))
			}
			// One spent seal without a living dragon represents an old cleared source.
			seals[0].Visited = true
			g.reconcileKillQuests()
			remaining := len(seals) - 1
			if !strings.Contains(hunt.Definition.Description, "{target_count}") {
				t.Fatalf("%s description does not show its quota", hunt.ID)
			}
			wantCopy := strings.ReplaceAll(hunt.Definition.Description, "{target_count}", fmt.Sprint(remaining))
			if hunt.Completed || hunt.Target() != remaining || hunt.Description() != wantCopy {
				t.Fatalf("remaining seal quota/copy: %+v", hunt)
			}
			save := g.buildSave(wm)
			g.restoreSavedQuests(&save)
			hunt = g.questManager.GetQuest(hunt.ID)
			if hunt.Target() != remaining {
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
	quota := q.Definition.TargetCount
	if quota < 2 {
		t.Fatalf("lake_spiders quota %d leaves no smaller pack to test", quota)
	}
	if g.creditQuestIfCleared(q.ID) || q.Completed {
		t.Fatal("absent seasonal pack granted free quest rewards")
	}
	add(quota - 1)
	g.reconcileKillQuests()
	if q.Target() != quota {
		t.Fatalf("first night target=%d, want the fixed quota %d", q.Target(), quota)
	}
	g.questManager.MarkCompleted(q.ID)
	if _, err := g.questManager.ClaimRewards(q.ID); err != nil {
		t.Fatal(err)
	}
	g.refreshRepeatableQuests("night")
	add(quota + 3)
	ih.handleGiveQuest(q.ID)
	q = g.questManager.GetQuest(q.ID)
	if q.Target() != quota || q.CurrentCount != 0 || q.Completed {
		t.Fatalf("new night retained reduced quota: %+v", q)
	}
}
