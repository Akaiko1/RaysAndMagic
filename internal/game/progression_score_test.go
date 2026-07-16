package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/world"
)

func TestScoreDataUsesEarnedTotalsNotCurrentRemainders(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	g.party.Members = g.party.Members[:1]
	g.party.Reserve = nil
	g.party.Captive = nil

	member.Level = 1
	member.Experience = XPRequiredPerLevel - 10
	member.HitPoints = 1
	delete(member.Skills, character.SkillLearning)
	g.totalExperienceEarned = 0
	g.grantSharedXP(20)
	if member.Level != 2 || member.Experience != 10 {
		t.Fatalf("test setup expected level-up to leave 10 XP, got level=%d xp=%d", member.Level, member.Experience)
	}

	g.party.Gold = g.config.Characters.StartingGold
	g.totalGoldEarned = 0
	g.awardGold(100)
	g.party.Gold -= 80

	score := g.GetScoreData()
	if score.TotalExperience != 20 {
		t.Fatalf("score XP = %d, want earned 20 rather than current remainder 10", score.TotalExperience)
	}
	if score.Gold != 100 {
		t.Fatalf("score gold = %d, want earned 100 rather than current wallet delta", score.Gold)
	}
}

func TestEarnedExperienceForCharacterReconstructsSpentLevelXP(t *testing.T) {
	got := earnedExperienceForCharacter(4, 25)
	want := XPRequiredPerLevel*(1+2+3) + 25
	if got != want {
		t.Fatalf("earnedExperienceForCharacter = %d, want %d", got, want)
	}
}

// TestXPStepCostCurve pins the level-cost curve: linear (unchanged) through
// L12, quadratic from L13 so kills-per-level stays roughly FLAT instead of
// collapsing as mob XP (~2.2 x L^2) outruns a linear cost.
func TestXPStepCostCurve(t *testing.T) {
	for _, tc := range []struct{ level, want int }{
		{1, 100}, {5, 500}, {12, 1200}, // linear branch, exactly as before
		{13, 1352}, // crossover: 8*13^2 > 100*13
		{20, 3200}, {30, 7200}, {49, 19208},
	} {
		if got := xpStepCost(tc.level); got != tc.want {
			t.Errorf("xpStepCost(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
	// The design property itself: cost/L^2 (kills-per-level against
	// level-appropriate mobs) must never DECREASE from L13 on.
	prev := 0.0
	for l := 13; l < 50; l++ {
		ratio := float64(xpStepCost(l)) / float64(l*l)
		if ratio < prev {
			t.Fatalf("kills-per-level proxy falls at L%d (%.3f < %.3f) - farming accelerates again", l, ratio, prev)
		}
		prev = ratio
	}
	if got, want := xpSpentToReach(50), 326000; got != want {
		t.Errorf("xpSpentToReach(50) = %d, want %d", got, want)
	}
}

func TestScoreTotalsAndVictoryAcknowledgementPersistThroughSaveLoad(t *testing.T) {
	cfg := loadTestConfig(t)
	wSave := newTestWorld(cfg)
	wmSave := world.NewWorldManager(cfg)
	wmSave.LoadedMaps = map[string]*world.World3D{"forest": wSave}
	wmSave.CurrentMapKey = "forest"

	gSave := newTestGame(cfg, wSave)
	gSave.totalGoldEarned = 1234
	gSave.totalExperienceEarned = 5678
	gSave.victoryAcknowledged = true
	save := gSave.buildSave(wmSave)

	wLoad := newTestWorld(cfg)
	wmLoad := world.NewWorldManager(cfg)
	wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
	wmLoad.CurrentMapKey = "forest"
	gLoad := newTestGame(cfg, wLoad)
	if err := gLoad.applySave(wmLoad, &save); err != nil {
		t.Fatalf("applySave: %v", err)
	}
	if gLoad.totalGoldEarned != 1234 || gLoad.totalExperienceEarned != 5678 {
		t.Fatalf("totals after load = gold %d xp %d, want 1234/5678", gLoad.totalGoldEarned, gLoad.totalExperienceEarned)
	}
	if !gLoad.victoryAcknowledged {
		t.Fatal("victory acknowledgement did not persist")
	}
}
