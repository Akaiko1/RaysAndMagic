package game

import (
	"time"

	"ugataima/internal/character"
	"ugataima/internal/highscore"
)

// GetScoreData collects score-input data from the current game state.
func (g *MMGame) GetScoreData() highscore.ScoreData {
	totalLevel := 0
	for _, member := range g.party.Members {
		totalLevel += member.Level
	}
	avgLevel := 1
	if len(g.party.Members) > 0 {
		avgLevel = totalLevel / len(g.party.Members)
	}

	playTime := time.Since(g.sessionStartTime)
	if !g.victoryTime.IsZero() {
		playTime = g.victoryTime.Sub(g.sessionStartTime)
	}

	return highscore.ScoreData{
		TotalExperience: g.totalExperienceEarned,
		Gold:            g.totalGoldEarned,
		AverageLevel:    avgLevel,
		PlayTime:        playTime,
	}
}

func (g *MMGame) awardGold(amount int) {
	if amount <= 0 || g.party == nil {
		return
	}
	g.party.Gold += amount
	g.totalGoldEarned += amount
}

// awardArenaPoints is the single crediting path for the arena victory currency
// (crate jackpots, champion kills) - the counterpart to awardGold, so any future
// telemetry or guard lives in one place instead of scattered raw increments.
func (g *MMGame) awardArenaPoints(amount int) {
	if amount <= 0 || g.party == nil {
		return
	}
	g.party.ArenaPoints += amount
}

// xpStepCost is the experience required to advance FROM `level` to level+1:
// the classic linear 100 x L early, the quadratic branch from L13 up (see
// XPQuadPerLevel for why). Single source for the level gate and the score
// reconstruction below.
func xpStepCost(level int) int {
	linear := XPRequiredPerLevel * level
	if quad := XPQuadPerLevel * level * level; quad > linear {
		return quad
	}
	return linear
}

// xpSpentToReach sums the step costs from level 1 up to `level` - the XP a
// character has already paid into level-ups (their Experience field holds only
// the remainder toward the next level).
func xpSpentToReach(level int) int {
	spent := 0
	for l := 1; l < level; l++ {
		spent += xpStepCost(l)
	}
	return spent
}

func earnedExperienceForCharacter(level, remaining int) int {
	if level < 1 {
		level = 1
	}
	if remaining < 0 {
		remaining = 0
	}
	return xpSpentToReach(level) + remaining
}

func earnedExperienceForParty(p *character.Party) int {
	if p == nil {
		return 0
	}
	total := 0
	add := func(m *character.MMCharacter) {
		if m != nil {
			total += earnedExperienceForCharacter(m.Level, m.Experience)
		}
	}
	for _, m := range p.Members {
		add(m)
	}
	for _, m := range p.Reserve {
		add(m)
	}
	for _, m := range p.Captive {
		add(m)
	}
	return total
}
