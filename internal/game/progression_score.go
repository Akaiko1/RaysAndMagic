package game

import (
	"time"

	"ugataima/internal/highscore"
)

// GetScoreData collects score-input data from the current game state.
func (g *MMGame) GetScoreData() highscore.ScoreData {
	totalExp := 0
	totalLevel := 0
	for _, member := range g.party.Members {
		totalExp += member.Experience
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
		TotalExperience: totalExp,
		Gold:            g.party.Gold,
		AverageLevel:    avgLevel,
		PlayTime:        playTime,
	}
}
