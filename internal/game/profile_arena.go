package game

import (
	"fmt"
	"strings"

	"ugataima/internal/arena"
	"ugataima/internal/playerprofile"
)

// The arena board already owns lifetime victory history, including old runs.
// Derive this read-only page from it instead of crediting a second ledger.
func arenaProfileData(board *arena.Board) playerprofile.Data {
	d := playerprofile.New()
	if board == nil {
		return d
	}
	for i, e := range board.Entries {
		wins := e.TotalKills()
		if wins == 0 {
			continue
		}
		d.Add("arena_wins", int64(wins))
		d.Add("arena_points", int64(e.TotalPoints))
		d.Add("arena_parties", 1)
		var names []string
		for _, m := range e.Members {
			names = append(names, m.Name)
		}
		label := strings.Join(names, ", ")
		if label == "" {
			label = fmt.Sprintf("Party %d", i+1)
		}
		key := e.RunID
		if key == "" {
			key = fmt.Sprintf("legacy:%d", i)
		}
		d.Rank("arena_parties", key, label, "icon_achievement_full_roster", int64(e.TotalPoints))
		for champion, tiers := range e.Kills {
			for tier, count := range tiers {
				d.Rank("arena_champions", champion, champion, "icon_achievement_warlord", int64(count))
				d.Rank("arena_tiers", tier, tier, "icon_achievement_victory", int64(count))
			}
		}
	}
	return d
}

func (ui *UISystem) ensureProfileArena() {
	if ui.profileArena != nil {
		return
	}
	board, err := arena.LoadChecked()
	if err != nil {
		ui.profileArenaError = "Arena records could not be read. The saved leaderboard has not been changed."
	}
	d := arenaProfileData(board)
	ui.profileArena = &d
}
