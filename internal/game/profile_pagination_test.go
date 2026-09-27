package game

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"testing"
	"ugataima/internal/arena"
	"ugataima/internal/config"
)

func TestProfileIndependentScroll(t *testing.T) {
	for _, size := range [][2]int{{800, 600}, {1920, 1080}} {
		for _, count := range []int{0, 2, 14} {
			t.Run(fmt.Sprintf("%dx%d/count%d", size[0], size[1], count), func(t *testing.T) {
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				fp := installFakePointer(t)
				g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuStatistics
				attachTestProfile(t, g)
				for _, group := range []string{"classes", "spells", "regions"} {
					for i := 0; i < count; i++ {
						g.playerProfile.Data.Rank(group, fmt.Sprint(i), "Entry", "", int64(i+1))
					}
				}
				priorWheel := pointerWheel
				t.Cleanup(func() { pointerWheel = priorWheel })
				wheel := -0.2
				pointerWheel = func() (float64, float64) { return 0, wheel }
				noKeys := func(ebiten.Key) bool { return false }
				l := makeProfileStatsLayout(size[0], size[1], profilePages[0])
				for i, spec := range profilePages[0].rankings {
					g.statisticsScroll = min(max(0, l.contentH-l.body.h), l.rankY+(i/l.columns)*(l.rankH+14))
					presentInputScreen(h)
					r := profileRankingBody(l.rankingRect(i, g.statisticsScroll))
					fp.moveTo(r.x+10, max(r.y, l.body.y)+10)
					beforeOuter := g.statisticsScroll
					g.updatePlayerStatisticsKeys(noKeys)
					want := 0
					if count > 4 {
						want = 9
					}
					wantOuter := beforeOuter
					if count <= 4 {
						wantOuter = min(beforeOuter+9, max(0, l.contentH-l.body.h))
					}
					if g.statisticsRankingScroll[spec.group] != want || g.statisticsScroll != wantOuter {
						t.Fatalf("%s scroll=%d outer=%d; want %d outer=%d", spec.group, g.statisticsRankingScroll[spec.group], g.statisticsScroll, want, wantOuter)
					}
					for _, other := range profilePages[0].rankings[i+1:] {
						if g.statisticsRankingScroll[other.group] != 0 {
							t.Fatal("wheel changed another list")
						}
					}
					wheel = -100
					g.updatePlayerStatisticsKeys(noKeys)
					if g.statisticsRankingScroll[spec.group] != g.statisticsRankingLimits[spec.group] {
						t.Fatal("list did not clamp at end")
					}
					wheel = -0.2
				}
				before := g.statisticsScroll
				g.setStatisticsTab(1)
				g.statisticsScroll = 34
				g.setStatisticsTab(0)
				if g.statisticsScroll != before {
					t.Fatal("tab switch lost its own outer position")
				}
				fp.moveTo(l.body.x+3, l.body.y+3)
				wheel = 0
				g.updatePlayerStatisticsKeys(func(k ebiten.Key) bool { return k == ebiten.KeyEnd })
				if g.statisticsScroll != max(0, l.contentH-l.body.h) {
					t.Fatal("outer keyboard scroll failed")
				}
				g.entryMenuMode = EntryMenuAchievements
				presentInputScreen(h)
				a := makeProfileAchievementsLayout(size[0], size[1], len(config.GetAchievements()))
				fp.moveTo(a.body.x+10, a.body.y+10)
				wheel = -0.2
				g.updateAchievementsKeys(noKeys)
				if g.achievementsScroll != min(9, max(0, a.contentH-a.body.h)) {
					t.Fatal("achievements wheel failed")
				}
			})
		}
	}
}

func TestArenaProfileUsesRecordedBoard(t *testing.T) {
	d := arenaProfileData(&arena.Board{Entries: []arena.Entry{
		{RunID: "one", TotalPoints: 80, Members: []arena.Member{{Name: "Ada"}}, Kills: map[string]map[string]int{"Knight": {"Novice": 2, "Master": 1}}},
		{TotalPoints: 120, Kills: map[string]map[string]int{"Knight": {"Master": 1}, "Monk": {"Expert": 3}}},
	}})
	if d.Counters["arena_wins"] != 7 || d.Counters["arena_points"] != 200 || d.Counters["arena_parties"] != 2 {
		t.Fatalf("arena totals: %v", d.Counters)
	}
	if d.Rankings["arena_champions"]["Knight"].Count != 4 || d.Rankings["arena_tiers"]["Master"].Count != 2 {
		t.Fatal("champion/tier breakdown lost history")
	}
	if len(d.Top("arena_parties")) != 2 {
		t.Fatal("legacy run discarded")
	}
}
