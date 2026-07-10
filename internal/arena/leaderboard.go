// Package arena keeps the GLOBAL arena leaderboard: one entry per party
// (identified by its member-name set), accumulating defeated champions per
// difficulty tier. Same JSON-in-save-dir pattern as internal/highscore -
// survives across saves and runs.
package arena

import (
	"encoding/json"
	"os"
	"sort"
	"time"

	"ugataima/internal/storage"
)

const fileName = "arena_leaderboard.json"

// Member is a party fighter snapshot (refreshed on every recorded victory).
type Member struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	Level int    `json:"level"`
}

// Entry is one playthrough's arena record. Keyed by RunID: member names
// collide across runs (every new game starts with the same default roster)
// and shift within one (tavern swaps), so the run - not the name set - is
// the identity. Members is a display snapshot, refreshed on each victory.
type Entry struct {
	RunID   string   `json:"run_id,omitempty"`
	Members []Member `json:"members"`
	// Kills: champion display name -> tier -> victories.
	Kills       map[string]map[string]int `json:"kills"`
	TotalPoints int                       `json:"total_points"`
	LastVictory time.Time                 `json:"last_victory"`
	// LastCredit: tier -> the in-game day of the last COUNTED victory (the run
	// is this entry's key). A reloaded save keeps the same day, so save-scumming
	// can never farm the board; a new day counts again.
	LastCredit map[string]int `json:"last_credit,omitempty"`
}

// TotalKills sums every recorded victory.
func (e *Entry) TotalKills() int {
	total := 0
	for _, tiers := range e.Kills {
		for _, n := range tiers {
			total += n
		}
	}
	return total
}

// KillsByTier folds the per-champion table into tier -> victories.
func (e *Entry) KillsByTier() map[string]int {
	out := map[string]int{}
	for _, tiers := range e.Kills {
		for tier, n := range tiers {
			out[tier] += n
		}
	}
	return out
}

// Board is the whole leaderboard, sorted by TotalPoints descending.
type Board struct {
	Entries []Entry `json:"entries"`
}

func filePath() string { return storage.AppSavePath(fileName) }

// Load reads the board; a missing or corrupt file yields an empty board.
func Load() *Board {
	data, err := os.ReadFile(filePath())
	if err != nil {
		return &Board{}
	}
	var b Board
	if json.Unmarshal(data, &b) != nil {
		return &Board{}
	}
	return &b
}

// Save writes the board.
func Save(b *Board) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath(), data, 0644)
}

// RecordVictory upserts the RUN's entry: member snapshot refreshed, the
// (champion, tier) kill counted, points accumulated; re-sorted by points.
// day guards against save-scum farming: a tier already credited on the SAME
// in-game day of this run (a save-load replay) is NOT counted again. Returns
// whether the victory was recorded.
func RecordVictory(runID string, members []Member, championName, tier string, points, day int) bool {
	b := Load()
	var entry *Entry
	for i := range b.Entries {
		if b.Entries[i].RunID == runID {
			entry = &b.Entries[i]
			break
		}
	}
	if entry == nil {
		b.Entries = append(b.Entries, Entry{RunID: runID, Kills: map[string]map[string]int{}})
		entry = &b.Entries[len(b.Entries)-1]
	}
	if credited, ok := entry.LastCredit[tier]; ok && credited == day {
		return false // same run, same in-game day: the board already honors it
	}
	entry.Members = members
	if entry.Kills == nil {
		entry.Kills = map[string]map[string]int{}
	}
	if entry.Kills[championName] == nil {
		entry.Kills[championName] = map[string]int{}
	}
	if entry.LastCredit == nil {
		entry.LastCredit = map[string]int{}
	}
	entry.LastCredit[tier] = day
	entry.Kills[championName][tier]++
	entry.TotalPoints += points
	entry.LastVictory = time.Now()
	sort.Slice(b.Entries, func(i, j int) bool { return b.Entries[i].TotalPoints > b.Entries[j].TotalPoints })
	_ = Save(b)
	return true
}
