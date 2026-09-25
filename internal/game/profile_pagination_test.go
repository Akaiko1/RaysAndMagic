package game

import (
	"fmt"
	"path/filepath"
	"testing"

	"ugataima/internal/playerprofile"
)

func TestStatisticsNavigationWrapsDisplayedButtons(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		entries, start, want int
		next                 bool
	}{
		{"previous wraps", 11, 0, 2, false},
		{"next wraps", 11, 2, 0, true},
		{"previous middle", 11, 1, 0, false},
		{"next middle", 11, 1, 2, true},
		{"empty previous", 0, 0, 0, false},
		{"single next", 5, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g, fp := h.g, installFakePointer(t)
			g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuStatistics
			store, err := playerprofile.Open(filepath.Join(t.TempDir(), "profile.json"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			g.playerProfile = store
			entries := make(map[string]playerprofile.Entry)
			for i := 0; i < tc.entries; i++ {
				entries[fmt.Sprint(i)] = playerprofile.Entry{Name: "Test", Count: int64(i + 1)}
			}
			g.playerProfile.Data.Rankings["spells"] = entries
			g.statisticsPage, g.statisticsScroll = tc.start, 48
			presentInputScreen(h)
			l := makeProfileStatsLayout(1024, 768, profilePages[0])
			x := l.panel.right() - menuFrameInset - 184 + 4
			if tc.next {
				x = l.panel.right() - menuFrameInset - 90 + 4
			}
			fp.moveTo(x, l.footerY+4)
			fp.press()
			updateInputScreen(h)
			if g.statisticsPage != tc.want {
				t.Fatalf("page=%d, want %d", g.statisticsPage, tc.want)
			}
			if tc.entries > profileRanksPerPage && g.statisticsScroll != 0 {
				t.Fatal("page change retained scroll")
			}
		})
	}
}
