package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/quests"
)

func TestLakeSpiderNightPackAdvancesRepeatableQuest(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, questManager := loadRealWorldForTest(t, cfg, "forest")
	game := newTestGame(cfg, wm.GetCurrentWorld())
	game.combat = NewCombatSystem(game)
	game.questManager = questManager
	quests.GlobalQuestManager = questManager

	game.syncDayNightPacks(true)

	var spiders []*monster.Monster3D
	for _, m := range game.world.Monsters {
		if m != nil && m.PackKey == dayNightPackTag("forest", true) && m.Key == "forest_spider" {
			spiders = append(spiders, m)
		}
	}
	if len(spiders) < 5 {
		t.Fatalf("forest night pack spawned %d spiders, want at least 5", len(spiders))
	}
	for _, spider := range spiders {
		if spider.QuestProgressIgnored {
			t.Fatal("forest night-pack spider is excluded from quest progress")
		}
	}

	input := &InputHandler{game: game}
	input.handleGiveQuest("lake_spiders")
	quest := questManager.GetQuest("lake_spiders")
	if quest == nil || quest.Completed || quest.CurrentCount != 0 {
		t.Fatalf("new lake_spiders quest = %+v, want active at 0/5", quest)
	}

	for i := 0; i < 5; i++ {
		spiders[i].HitPoints = 0
		game.combat.updateQuestProgress(spiders[i])
	}
	if !quest.Completed || quest.CurrentCount != 5 {
		t.Fatalf("lake_spiders after five night-pack kills = completed %v, progress %d", quest.Completed, quest.CurrentCount)
	}
	if !game.claimQuestReward("lake_spiders") {
		t.Fatal("claim first nightly lake_spiders reward")
	}

	game.refreshRepeatableQuests()
	if questManager.GetQuest("lake_spiders") != nil {
		t.Fatal("claimed repeatable quest was not cleared for the next night")
	}
	input.handleGiveQuest("lake_spiders")
	next := questManager.GetQuest("lake_spiders")
	if next == nil || next.Completed || next.CurrentCount != 0 {
		t.Fatalf("next-night lake_spiders quest = %+v, want active at 0/5", next)
	}
}
