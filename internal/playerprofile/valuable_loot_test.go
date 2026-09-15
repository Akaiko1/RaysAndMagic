package playerprofile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValuableLootUsesUnitValueAndPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	legacy := `{"version":1,"rankings":{"loot":{"old":{"name":"Old find","count":8}}}}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if len(s.Data.MostValuableLoot()) != 0 {
		t.Fatal("invented value for an old record")
	}
	for _, tc := range []struct {
		key, name    string
		units, value int64
	}{
		{"cheap", "Cheap", 1000, 5},
		{"rare", "Rare", 1, 100},
		{"old", "Old find", 2, 50},
		{"rare", "Rare", 2, 90},
		{"zero", "Free", 3, 0},
		{"negative", "Invalid", 1, -1},
		{"ignored", "Ignored", 0, 9999},
		{"tie", "Another", 1, 100},
	} {
		s.Data.RankValued("loot", tc.key, tc.name, "icon_"+tc.key, tc.units, tc.value)
	}
	want := []Entry{
		{Name: "Another", Icon: "icon_tie", Count: 1, BaseValue: 100},
		{Name: "Rare", Icon: "icon_rare", Count: 3, BaseValue: 100},
		{Name: "Old find", Icon: "icon_old", Count: 10, BaseValue: 50},
		{Name: "Cheap", Icon: "icon_cheap", Count: 1000, BaseValue: 5},
	}
	if got := s.Data.MostValuableLoot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ranking=%+v want %+v", got, want)
	}
	if s.Data.Top("loot")[0].Name != "Cheap" {
		t.Fatal("value ranking changed frequency order")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got := restored.Data.MostValuableLoot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("restart changed ranking: %+v", got)
	}
}
