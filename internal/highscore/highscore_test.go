package highscore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ugataima/internal/items"
	"ugataima/internal/storage"
)

func TestCalculate_UsesTwoTierTimeBonus(t *testing.T) {
	base := ScoreData{TotalExperience: 100, Gold: 10, AverageLevel: 2}

	tests := []struct {
		name string
		time time.Duration
		want int
	}{
		{name: "over two hours", time: 2*time.Hour + time.Second, want: 100*10 + 10*5 + 2*1000},
		{name: "under two hours", time: 90 * time.Minute, want: 100*10 + 10*5 + 2*1000 + 30*60*5},
		{name: "under one hour gets both tiers", time: 30 * time.Minute, want: 100*10 + 10*5 + 2*1000 + 90*60*5 + 30*60*10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Calculate(ScoreData{
				TotalExperience: base.TotalExperience,
				Gold:            base.Gold,
				AverageLevel:    base.AverageLevel,
				PlayTime:        tt.time,
			})
			if got != tt.want {
				t.Fatalf("Calculate() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Every record explains its whole stored score: the speed bonus is what the
// score holds beyond experience, gold and level, so records scored under the
// earlier one-tier speed bonus (under an hour only) still add up.
func TestEntryPartsAddUpToTheStoredScore(t *testing.T) {
	data := ScoreData{TotalExperience: 4010, Gold: 980, AverageLevel: 14, PlayTime: 90 * time.Minute}
	base := 40100 + 4900 + 14000
	for _, tc := range []struct {
		name     string
		score    int
		secs     int
		playTime string
		speed    int
	}{
		{"seconds", Calculate(data), 5400, "01:30:00", 30 * 60 * 5},
		{"two-tier record", Calculate(data), 0, "01:30:00", 30 * 60 * 5},
		{"one-tier record, 1:30", base, 0, "01:30:00", 0},
		{"one-tier record, 30:00", base + 1800*10, 0, "30:00", 1800 * 10},
		{"unreadable time", Calculate(data), 0, "1h30m", 30 * 60 * 5},
		{"no time", Calculate(data), 0, "", 30 * 60 * 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			score := tc.score
			e := Entry{Score: score, Experience: data.TotalExperience, Gold: data.Gold, AvgLevel: data.AverageLevel, PlayTime: tc.playTime, PlaySeconds: tc.secs}
			p := e.Parts()
			if p.Total() != score || p.Speed != tc.speed || p.Experience != 40100 || p.Gold != 4900 || p.Levels != 14000 {
				t.Fatalf("parts %+v (total %d), want the score %d with a %d speed bonus", p, p.Total(), score, tc.speed)
			}
		})
	}
}

// A board written before parties were kept still loads, and a recorded party
// keeps its heroes and their gear through the file.
func TestBoardKeepsOldRecordsAndParties(t *testing.T) {
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	legacy := `{"entries":[{"player_name":"Old Guard","score":473000,"gold":9800,"experience":40100,"avg_level":14,"play_time":"01:30:00","date":"2026-10-12T20:00:00Z"}]}`
	if err := os.MkdirAll(filepath.Dir(path()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadCached()
	if err != nil || len(b.Entries) != 1 || b.Entries[0].Party != nil || b.Entries[0].Parts().Total() != 473000 {
		t.Fatalf("legacy board = %+v, %v", b, err)
	}
	if b, err = Load(); err != nil { // a board to edit, never the shared cached one
		t.Fatal(err)
	}
	hero := Hero{Name: "Auberon", Class: "paladin", Level: 31, Portrait: "auberon", Gear: []items.Item{{Name: "Vibroblade", Type: items.ItemWeapon}}}
	if rank := Add(b, Entry{PlayerName: "Company", Score: 900000, Party: []Hero{hero}}); rank != 0 {
		t.Fatalf("the best run ranks %d", rank)
	}
	if err := Save(b); err != nil {
		t.Fatal(err)
	}
	b, err = LoadCached() // the file changed: the cache must read it again
	if err != nil || len(b.Entries) != 2 {
		t.Fatalf("board after save = %+v, %v", b, err)
	}
	got := b.Entries[0].Party
	if len(got) != 1 || got[0].Name != "Auberon" || got[0].Class != "paladin" || len(got[0].Gear) != 1 || got[0].Gear[0].Name != "Vibroblade" {
		t.Fatalf("party after reload = %+v", got)
	}

	storage.SetDataRootForTesting(t.TempDir()) // another data root holds no board
	if b, err := LoadCached(); err != nil || len(b.Entries) != 0 {
		t.Fatalf("a new data root served %+v, %v", b, err)
	}
}
