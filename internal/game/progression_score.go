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

func earnedExperienceForCharacter(level, remaining int) int {
	if level < 1 {
		level = 1
	}
	if remaining < 0 {
		remaining = 0
	}
	spent := XPRequiredPerLevel * (level - 1) * level / 2
	return spent + remaining
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
