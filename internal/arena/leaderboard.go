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
	// LastCredit: tier -> highest in-game phase credited for this run.
	// Equal or older saves cannot replay that tier for another board credit.
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

// Board is the whole leaderboard, sorted by TotalPoints descending.
type Board struct {
	Entries []Entry `json:"entries"`
}

func filePath() string { return storage.AppSavePath(fileName) }

// Load is the read-only UI view. Mutation paths use LoadChecked and never
// replace unreadable records with this empty display fallback.
func Load() *Board {
	b, err := LoadChecked()
	if err != nil {
		return &Board{}
	}
	return b
}

func LoadChecked() (*Board, error) {
	data, err := os.ReadFile(filePath())
	if os.IsNotExist(err) {
		return &Board{}, nil
	}
	if err != nil {
		return nil, err
	}
	var b Board
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// Save writes the board.
func Save(b *Board) error {
	return storage.WriteJSONAtomic(filePath(), b, 0644)
}

// RecordVictory upserts the RUN's entry: member snapshot refreshed, the
// (champion, tier) kill counted, points accumulated; re-sorted by points.
// day is a high-water mark: replaying the same or any earlier phase of a run
// cannot earn another board credit. Returns
// whether the victory was recorded.
func RecordVictory(runID string, members []Member, championName, tier string, points, day int) bool {
	b, err := LoadChecked()
	if err != nil {
		return false
	}
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
	if credited, ok := entry.LastCredit[tier]; ok && day <= credited {
		return false // This phase or a later one already earned credit.
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
	return Save(b) == nil
}
