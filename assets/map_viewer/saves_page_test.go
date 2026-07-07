package main

import (
	"strings"
	"testing"

	"ugataima/internal/game"
)

func TestBuildSaveListEntries(t *testing.T) {
	rows := make([]game.SaveSummary, game.SaveRowsTotal())
	rows[1] = game.SaveSummary{Exists: true, Name: "Hero Run"}
	rows[2] = game.SaveSummary{Exists: true, MapKey: "forest"}
	archived := []game.ArchivedSave{
		{Path: "/saves/archive/Hero_Run_20260707-120000.json", Summary: game.SaveSummary{Exists: true, Name: "Hero Run", MapKey: "city"}},
		{Path: "/saves/archive/broken.json"},
	}

	entries := buildSaveListEntries(rows, archived)

	var stash, slots, archives, headers int
	for _, e := range entries {
		switch e.kind {
		case saveEntryStash:
			stash++
		case saveEntrySlot:
			slots++
		case saveEntryArchive:
			archives++
		case saveEntryHeader:
			headers++
		}
	}
	if stash != 1 || slots != game.SaveRowsTotal() || archives != len(archived) || headers != 3 {
		t.Fatalf("entry counts stash=%d slots=%d archives=%d headers=%d", stash, slots, archives, headers)
	}

	byLabel := func(sub string) *saveListEntry {
		for i := range entries {
			if strings.Contains(entries[i].label, sub) {
				return &entries[i]
			}
		}
		return nil
	}
	if e := byLabel("Hero Run"); e == nil || e.row != 1 || e.dim {
		t.Errorf("named slot entry wrong: %+v", e)
	}
	if e := byLabel("Slot 2"); e == nil || !strings.Contains(e.label, "forest") {
		t.Errorf("map-key fallback label wrong: %+v", e)
	}
	if e := byLabel("Slot 3"); e == nil || !e.dim || !strings.Contains(e.label, "empty") {
		t.Errorf("empty slot should be dimmed with an empty marker: %+v", e)
	}
	if e := byLabel("Autosave"); e == nil || e.row != 0 {
		t.Errorf("autosave row missing: %+v", e)
	}
}

func TestArchiveDisplayName(t *testing.T) {
	named := game.ArchivedSave{Path: "/a/Hero_Run_1.json", Summary: game.SaveSummary{Exists: true, Name: "Hero Run", MapKey: "city"}}
	if got := archiveDisplayName(named); got != "Hero Run  (city)" {
		t.Errorf("named archive display = %q", got)
	}
	broken := game.ArchivedSave{Path: "/a/broken.json"}
	if got := archiveDisplayName(broken); got != "broken" {
		t.Errorf("unreadable archive should fall back to filename, got %q", got)
	}
}
