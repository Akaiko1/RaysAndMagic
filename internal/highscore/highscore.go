// Package highscore handles the score formula, play-time formatting, and the
// top-N leaderboard persisted as JSON next to the app's save files.
package highscore

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"ugataima/internal/storage"
)

const (
	fileName = "highscores.json"
	maxKeep  = 10
)

// Entry is a single high-score record.
type Entry struct {
	PlayerName string    `json:"player_name"`
	Score      int       `json:"score"`
	Gold       int       `json:"gold"`
	Experience int       `json:"experience"`
	AvgLevel   int       `json:"avg_level"`
	PlayTime   string    `json:"play_time"`
	Date       time.Time `json:"date"`
}

// Board is the persisted top-N leaderboard.
type Board struct {
	Entries []Entry `json:"entries"`
}

// ScoreData is the per-run input used to compute a final score.
type ScoreData struct {
	TotalExperience int
	Gold            int
	AverageLevel    int
	PlayTime        time.Duration
}

// Calculate returns the final score for a run.
//
// BaseScore  = experience*10 + gold*5 + avgLevel*1000
// TimeBonus  = max(0, 7200 - secondsPlayed)*5 + max(0, 3600 - secondsPlayed)*10
//
//	(two tiers: one for finishing under 2h, another under 1h)
//
// FinalScore = BaseScore + TimeBonus
func Calculate(d ScoreData) int {
	base := d.TotalExperience*10 + d.Gold*5 + d.AverageLevel*1000
	bonus := 0
	if secs := int(d.PlayTime.Seconds()); secs < 7200 {
		bonus += (7200 - secs) * 5
		if secs < 3600 {
			bonus += (3600 - secs) * 10
		}
	}
	return base + bonus
}

// FormatPlayTime renders a duration as HH:MM:SS, omitting hours when zero.
func FormatPlayTime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func path() string {
	return storage.AppSavePath(fileName)
}

// Load reads the leaderboard from disk. A missing or unparseable file yields
// an empty board rather than an error so the UI can render unconditionally.
func Load() (*Board, error) {
	data, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Board{Entries: []Entry{}}, nil
		}
		return nil, err
	}
	var b Board
	if err := json.Unmarshal(data, &b); err != nil {
		return &Board{Entries: []Entry{}}, nil
	}
	return &b, nil
}

// Save writes the leaderboard to disk.
func Save(b *Board) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0644)
}

// Add inserts entry into the board, keeps it sorted by score desc, and trims
// to the top maxKeep records.
func Add(b *Board, entry Entry) {
	b.Entries = append(b.Entries, entry)
	sort.Slice(b.Entries, func(i, j int) bool {
		return b.Entries[i].Score > b.Entries[j].Score
	})
	if len(b.Entries) > maxKeep {
		b.Entries = b.Entries[:maxKeep]
	}
}
