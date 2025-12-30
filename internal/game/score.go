package game

import (
	"time"
)

// ScoreData holds all the data needed for score calculation
type ScoreData struct {
	TotalExperience int
	Gold            int
	AverageLevel    int
	PlayTime        time.Duration
}

// CalculateScore computes the final score using a balanced formula
// BaseScore = (TotalExperience * 10) + (Gold * 5) + (AverageLevel * 1000)
// TimeBonus = max(0, 3600 - TimePlayedSeconds) * 10  // Bonus for finishing under 1 hour
// FinalScore = BaseScore + TimeBonus
func CalculateScore(data ScoreData) int {
	baseScore := (data.TotalExperience * 10) + (data.Gold * 5) + (data.AverageLevel * 1000)

	// Time bonus: reward finishing under 1 hour
	timePlayedSeconds := int(data.PlayTime.Seconds())
	timeBonus := 0
	if timePlayedSeconds < 3600 {
		timeBonus = (3600 - timePlayedSeconds) * 10
	}

	return baseScore + timeBonus
}

// GetScoreData collects score data from the game state
func (g *MMGame) GetScoreData() ScoreData {
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

	return ScoreData{
		TotalExperience: totalExp,
		Gold:            g.party.Gold,
		AverageLevel:    avgLevel,
		PlayTime:        playTime,
	}
}

// FormatPlayTime formats the play time as HH:MM:SS
func FormatPlayTime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return formatDuration(hours, minutes, seconds)
	}
	return formatMinutesSeconds(minutes, seconds)
}

func formatDuration(hours, minutes, seconds int) string {
	return padZero(hours) + ":" + padZero(minutes) + ":" + padZero(seconds)
}

func formatMinutesSeconds(minutes, seconds int) string {
	return padZero(minutes) + ":" + padZero(seconds)
}

func padZero(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
