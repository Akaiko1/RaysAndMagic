// Package highscore handles the score formula, play-time formatting, and the
// top-N leaderboard persisted as JSON next to the app's save files.
package highscore

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"ugataima/internal/items"
	"ugataima/internal/storage"
)

const (
	fileName = "highscores.json"
	maxKeep  = 10
)

// Entry is a single high-score record. Party and PlaySeconds arrived later;
// older records load without them.
type Entry struct {
	PlayerName  string    `json:"player_name"`
	Score       int       `json:"score"`
	Gold        int       `json:"gold"`
	Experience  int       `json:"experience"`
	AvgLevel    int       `json:"avg_level"`
	PlayTime    string    `json:"play_time"`
	PlaySeconds int       `json:"play_seconds,omitempty"`
	Date        time.Time `json:"date"`
	Party       []Hero    `json:"party,omitempty"`
}

// Hero is one party member as the run ended: who, and what they wore.
type Hero struct {
	Name     string       `json:"name"`
	Class    string       `json:"class"`              // class key
	Level    int          `json:"level"`              // 0 when unknown (a party restored without it)
	Portrait string       `json:"portrait,omitempty"` // sprite key
	Gear     []items.Item `json:"gear,omitempty"`
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

// Score points per unit, and the play-time bonus tiers: each second under a
// tier's limit earns its rate.
const (
	PointsPerExperience = 10
	PointsPerGold       = 5
	PointsPerLevel      = 1000
	SpeedBonusSeconds   = 7200
	SpeedBonusRate      = 5
	FastBonusSeconds    = 3600
	FastBonusRate       = 10
)

// Parts is a score split by what earned it.
type Parts struct {
	Experience, Gold, Levels, Speed int
}

// Total is the final score.
func (p Parts) Total() int { return p.Experience + p.Gold + p.Levels + p.Speed }

// Score splits a run's score:
//
//	experience*10 + gold*5 + avgLevel*1000, plus a speed bonus of
//	max(0, 7200 - secondsPlayed)*5 + max(0, 3600 - secondsPlayed)*10
//	(two tiers: one for finishing under 2h, another under 1h).
func Score(d ScoreData) Parts {
	p := Parts{
		Experience: d.TotalExperience * PointsPerExperience,
		Gold:       d.Gold * PointsPerGold,
		Levels:     d.AverageLevel * PointsPerLevel,
	}
	if secs := int(d.PlayTime.Seconds()); secs < SpeedBonusSeconds {
		p.Speed += (SpeedBonusSeconds - secs) * SpeedBonusRate
		if secs < FastBonusSeconds {
			p.Speed += (FastBonusSeconds - secs) * FastBonusRate
		}
	}
	return p
}

// Calculate returns the final score for a run.
func Calculate(d ScoreData) int { return Score(d).Total() }

// Seconds is the run's play time: PlaySeconds, or an older record's formatted
// time; false when neither reads.
func (e Entry) Seconds() (int, bool) {
	if e.PlaySeconds > 0 {
		return e.PlaySeconds, true
	}
	return parsePlayTime(e.PlayTime)
}

// Parts splits the stored score into what earned it. The speed bonus is what
// the score holds beyond the other parts: older records were scored under
// earlier speed tiers, so it is never recomputed.
func (e Entry) Parts() Parts {
	p := Score(ScoreData{TotalExperience: e.Experience, Gold: e.Gold, AverageLevel: e.AvgLevel})
	p.Speed = max(0, e.Score-p.Experience-p.Gold-p.Levels)
	return p
}

// parsePlayTime reads FormatPlayTime's "HH:MM:SS" or "MM:SS".
func parsePlayTime(s string) (int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	secs := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, false
		}
		secs = secs*60 + n
	}
	return secs, true
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

// Load returns an empty board only for a missing file. Read/decode errors
// must reach callers so a later write cannot erase existing records.
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
		return nil, err
	}
	return &b, nil
}

// Save writes the leaderboard to disk; the next LoadCached reads it again.
func Save(b *Board) error {
	cached.board, cached.err = nil, nil
	return storage.WriteJSONAtomic(path(), b, 0644)
}

// cached is the last board LoadCached read, keyed by the file and its stamp.
var cached struct {
	path    string
	modTime time.Time
	size    int64
	exists  bool
	board   *Board
	err     error
}

// LoadCached is Load for readers that run every frame: it reads the file
// again only after a Save or when its stamp changed. The board is shared;
// callers read it and never change it (Load gives a board to edit).
func LoadCached() (*Board, error) {
	p := path()
	info, err := os.Stat(p)
	exists := err == nil
	if (cached.board != nil || cached.err != nil) && cached.path == p && exists == cached.exists &&
		(!exists || info.ModTime().Equal(cached.modTime) && info.Size() == cached.size) {
		return cached.board, cached.err
	}
	cached.board, cached.err = Load()
	cached.path, cached.exists = p, exists
	if exists {
		cached.modTime, cached.size = info.ModTime(), info.Size()
	}
	return cached.board, cached.err
}

// Add files entry by score (below equal scores already kept), trims the board
// to the top maxKeep records and returns entry's rank, -1 when it fell off.
func Add(b *Board, entry Entry) int {
	sort.SliceStable(b.Entries, func(i, j int) bool {
		return b.Entries[i].Score > b.Entries[j].Score
	})
	rank := sort.Search(len(b.Entries), func(i int) bool { return b.Entries[i].Score < entry.Score })
	b.Entries = slices.Insert(b.Entries, rank, entry)
	if len(b.Entries) > maxKeep {
		b.Entries = b.Entries[:maxKeep]
	}
	if rank >= maxKeep {
		return -1
	}
	return rank
}
