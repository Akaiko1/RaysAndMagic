package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"ugataima/internal/boot"
	"ugataima/internal/items"
	"ugataima/internal/storage"

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

func TestSaveTooltipBrowseKeepsFileAndNormalizesItems(t *testing.T) {
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	t.Chdir("../..")
	boot.LoadGameData()
	for _, kind := range []string{"legacy", "physical", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			it := items.CreateItemFromYAML("leather_armor")
			it.Attributes["armor_class"] = 9999
			gs := game.GameSave{Party: game.PartySave{Inventory: []items.Item{it}}}
			troll := items.CreateItemFromYAML("troll_card")
			switch kind {
			case "legacy":
				gs.Party.CardCollection = []string{"troll_card"}
			case "physical":
				gs.Party.CardCollectionItems = []items.Item{troll}
			case "mixed":
				gs.Party.CardCollectionItems = []items.Item{{}, troll}
				gs.Party.CardCollection = []string{"troll_card", "troll_card"}
			}
			before, err := json.Marshal(gs)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "fixture.json")
			if err = os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			_, loot := buildSaveDetail(path, "Fixture", game.SaveSummary{})
			cards := 0
			for _, row := range loot {
				if row.item == nil {
					continue
				}
				if row.item.Type == items.ItemCard {
					cards++
				}
				card := cardForSavedItem(*row.item)
				text := strings.Join(card.tooltipRows, "\n")
				if text == "" || strings.Contains(text, "9999") {
					t.Fatalf("stale or empty tooltip: %s", text)
				}
			}
			want := 1
			if kind == "mixed" {
				want = 2
			}
			if cards != want {
				t.Fatalf("card hover count %d, want %d", cards, want)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("read-only save browsing changed the source file")
			}
		})
	}
}
