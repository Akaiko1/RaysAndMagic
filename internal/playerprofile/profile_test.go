package playerprofile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileSurvivesRestartAndCoalescesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		s.Data.Add("kills", 1)
		s.Data.Rank("kills", "wolf", "Wolf", "wolf", 1)
		s.Checkpoint()
	}
	stamp := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if !s.Data.Unlock("first_blood", []string{"kills"}, 1, stamp) {
		t.Fatal("did not unlock")
	}
	s.Data.ObserveRun("run-1", true, false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Data.Counters["kills"] != 120 || restored.Data.Top("kills")[0].Count != 120 {
		t.Fatal("latest snapshot lost")
	}
	if !restored.Data.Unlocked["first_blood"].Equal(stamp) {
		t.Fatal("unlock timestamp lost")
	}
	for range 3 {
		restored.Data.ObserveRun("run-1", true, true)
	}
	if restored.Data.Counters["adventures"] != 1 || restored.Data.Counters["victories"] != 1 || restored.Data.Counters["defeats"] != 1 {
		t.Fatal("reloading duplicated a run outcome")
	}
	if restored.Data.Unlock("first_blood", []string{"kills"}, 1, time.Now()) {
		t.Fatal("restart replayed achievement")
	}
}

func TestProfileRejectsUnreadableDataWithoutReplacingIt(t *testing.T) {
	for _, raw := range []string{"{truncated", `{"version":999}`, `{"version":1,"since":"invalid"}`} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.json")
			os.WriteFile(path, []byte(raw), 0600)
			if s, err := Open(path); err == nil || s != nil {
				t.Fatal("invalid profile accepted")
			}
			got, _ := os.ReadFile(path)
			if string(got) != raw {
				t.Fatal("original data overwritten")
			}
		})
	}
}

func TestRulesAndRankings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int64
		metric string
		want   bool
	}{
		{"below", 1, "a", false}, {"first alternative", 2, "a", true}, {"second alternative", 2, "b", true}, {"unrelated", 100, "c", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := New()
			d.Add(tc.metric, tc.count)
			if d.Unlock("test", []string{"a", "b"}, 2, time.Now()) != tc.want {
				t.Fatal("wrong rule result")
			}
		})
	}
	d := New()
	d.Rank("loot", "b", "Bee", "b", 2)
	d.Rank("loot", "a", "Ant", "a", 2)
	d.Rank("loot", "c", "Cat", "c", 0)
	if top := d.Top("loot"); len(top) != 2 || top[0].Name != "Ant" {
		t.Fatal("unstable ranking or zero entry")
	}
}

func TestAchievementResetPreservesStatisticsAcrossRestart(t *testing.T) {
	for _, metric := range []string{"kills", "departures:city", "boss:a", "boss:b", "roster"} {
		t.Run(metric, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.json")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			s.Data.Add(metric, 7)
			s.Data.Rank("kills", "goblin", "Goblin", "goblin", 7)
			s.Data.ObserveRun("old-run", true, false)
			stamp := s.Data.Since
			if !s.Data.Unlock("test", []string{metric}, 1, time.Now()) {
				t.Fatal("legacy lifetime progress lost")
			}
			s.Data.ResetAchievements([]string{"kills", "departures:city", "boss:a", "boss:b", "roster"})
			if s.Data.Counters[metric] != 7 || s.Data.Top("kills")[0].Count != 7 || s.Data.Counters["victories"] != 1 || s.Data.Since != stamp {
				t.Fatal("reset changed lifetime statistics")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			metrics := []string{metric}
			if metric == "boss:a" || metric == "boss:b" {
				metrics = []string{"boss:a", "boss:b"}
			}
			if s.Data.Unlock("test", metrics, 1, time.Now()) || s.Data.AchievementProgress(metrics) != 0 {
				t.Fatal("restart reused historical progress")
			}
			if metric == "roster" {
				s.Data.Observe(metric, 1)
			} else {
				s.Data.Add(metric, 1)
			}
			if !s.Data.Unlock("test", metrics, 1, time.Now()) || s.Data.Unlock("test", metrics, 1, time.Now()) {
				t.Fatal("new condition must unlock exactly once")
			}
			want := int64(8)
			if metric == "roster" {
				want = 7
			}
			if s.Data.Counters[metric] != want || s.Data.AchievementProgress(metrics) != 1 {
				t.Fatal("progress period drifted from committed events")
			}
		})
	}
}

func TestAchievementHeroHistorySurvivesRestartAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Data.AchievementHeroes = map[string]bool{"Gareth": true, "Grikka": true}
	s.Data.Add("play_ns", 123)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Data.AchievementHeroes) != 2 {
		t.Fatal("hero identities lost on restart")
	}
	s.Data.ResetAchievements([]string{"all_heroes_played"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Data.AchievementHeroes) != 0 || s.Data.Counters["play_ns"] != 123 {
		t.Fatal("reset/restart lost statistics or restored heroes")
	}
}

func TestVisitedCoordinatesSurviveRestartMapChangesAndAchievementReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		s.Data.VisitTile("forest", 1, 2)
		s.Data.VisitTile("forest", 7, 3)
	}
	s.Data.VisitTile("desert", 1, 2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Data.ResetAchievements([]string{"kills"})
	for _, tc := range []struct {
		w, h int
		want int64
	}{{8, 8, 2}, {16, 8, 2}, {4, 4, 1}, {8, 8, 2}} {
		if n := s.Data.VisitedTileCount("forest", tc.w, tc.h); n != tc.want {
			t.Fatalf("%dx%d: %d, want %d", tc.w, tc.h, n, tc.want)
		}
	}
	s.Data.VisitTile("forest", 1, 2)
	if len(s.Data.VisitedTiles["forest"]) != 2 || s.Data.VisitedTileCount("desert", 8, 8) != 1 {
		t.Fatal("coordinates duplicated or region histories mixed")
	}
}
