package game

import (
	"encoding/json"
	"os"
	"sort"
	"time"
)

const (
	highScoresFile = "highscores.json"
	maxHighScores  = 10
)

// HighScoreEntry represents a single high score entry
type HighScoreEntry struct {
	PlayerName string    `json:"player_name"`
	Score      int       `json:"score"`
	Gold       int       `json:"gold"`
	Experience int       `json:"experience"`
	AvgLevel   int       `json:"avg_level"`
	PlayTime   string    `json:"play_time"`
	Date       time.Time `json:"date"`
}

// HighScores holds the list of high score entries
type HighScores struct {
	Entries []HighScoreEntry `json:"entries"`
}

// getHighScoresPath returns the path to the high scores file
func getHighScoresPath() string {
	return getAppSavePath(highScoresFile)
}

// LoadHighScores reads high scores from the JSON file
func LoadHighScores() (*HighScores, error) {
	path := getHighScoresPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HighScores{Entries: []HighScoreEntry{}}, nil
		}
		return nil, err
	}

	var scores HighScores
	if err := json.Unmarshal(data, &scores); err != nil {
		return &HighScores{Entries: []HighScoreEntry{}}, nil
	}

	return &scores, nil
}

// SaveHighScores writes high scores to the JSON file
func SaveHighScores(scores *HighScores) error {
	path := getHighScoresPath()

	data, err := json.MarshalIndent(scores, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// AddHighScore adds a new entry to the high scores list, maintaining top 10
func AddHighScore(scores *HighScores, entry HighScoreEntry) {
	scores.Entries = append(scores.Entries, entry)

	// Sort by score descending
	sort.Slice(scores.Entries, func(i, j int) bool {
		return scores.Entries[i].Score > scores.Entries[j].Score
	})

	// Keep only top 10
	if len(scores.Entries) > maxHighScores {
		scores.Entries = scores.Entries[:maxHighScores]
	}
}

// IsHighScore checks if a score qualifies for the top 10
func IsHighScore(scores *HighScores, score int) bool {
	if len(scores.Entries) < maxHighScores {
		return true
	}
	return score > scores.Entries[len(scores.Entries)-1].Score
}

// GetRank returns the rank (1-based) that this score would achieve
func GetRank(scores *HighScores, score int) int {
	for i, entry := range scores.Entries {
		if score > entry.Score {
			return i + 1
		}
	}
	return len(scores.Entries) + 1
}
