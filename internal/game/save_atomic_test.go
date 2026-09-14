package game

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"testing"

	"ugataima/internal/arena"
	"ugataima/internal/highscore"
	"ugataima/internal/items"
	"ugataima/internal/stash"
	"ugataima/internal/storage"
)

func TestSaveReplacementEntryPoints(t *testing.T) {
	for _, entry := range []string{"manual", "autosave", "rename", "stash_transfer"} {
		for _, fail := range []bool{false, true} {
			t.Run(entry+map[bool]string{false: "/success", true: "/failure"}[fail], func(t *testing.T) {
				g, _, _ := travelFixture(t)
				row := 1
				if entry == "autosave" || entry == "stash_transfer" {
					row = 0
				}
				path := saveRowPath(row)
				g.stash = &stash.Stash{}
				if err := g.SaveGameToFile(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				g.party.Gold += 17
				if fail && entry != "rename" {
					g.camera.X = math.NaN()
				}
				if entry == "rename" && fail {
					// A malformed existing slot must not be overwritten by a rename attempt.
					before = []byte("invalid previous slot")
					if err = os.WriteFile(path, before, 0600); err != nil {
						t.Fatal(err)
					}
				}
				switch entry {
				case "manual":
					err = g.SaveGameToFile(path)
				case "autosave":
					err = g.autosaveErr()
				case "rename":
					err = RenameSaveSlot(row, "Renamed slot")
				case "stash_transfer":
					snapshot := g.stashSnapshot()
					g.stash.Slots[0] = items.Item{Name: "Transfer fixture", Type: items.ItemAccessory}
					ok := g.commitStashTransfer(snapshot)
					if ok == fail {
						t.Fatalf("transfer success=%v, failure case=%v", ok, fail)
					}
					if fail && g.stash.Slots[0].Name != "" {
						t.Fatal("failed autosave did not restore stash")
					}
				}
				if entry != "stash_transfer" && (err != nil) != fail {
					t.Fatalf("operation error=%v, failure case=%v", err, fail)
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if fail {
					if !bytes.Equal(before, after) {
						t.Fatal("failed operation destroyed previous save")
					}
					return
				}
				var saved GameSave
				if err = json.Unmarshal(after, &saved); err != nil {
					t.Fatal(err)
				}
				if entry == "rename" {
					if saved.SaveName != "Renamed slot" {
						t.Fatal("rename was not persisted")
					}
				} else if saved.Party.Gold != g.party.Gold {
					t.Fatal("replacement did not persist current gold")
				}
				if entry == "stash_transfer" {
					loaded, err := stash.Load()
					if err != nil || loaded.Slots[0].Name != "Transfer fixture" {
						t.Fatalf("stash roundtrip=%+v, %v", loaded, err)
					}
					if saved.StashTransferID == "" {
						t.Fatal("autosave lost transfer commit marker")
					}
				}
			})
		}
	}
}

func TestSharedStoreReplacementEntryPoints(t *testing.T) {
	for _, kind := range []string{"arena", "highscore", "stash", "journal"} {
		t.Run(kind, func(t *testing.T) {
			storage.SetDataRootForTesting(t.TempDir())
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			var path string
			var save func(string) error
			var load func() string
			switch kind {
			case "arena":
				path = storage.AppSavePath("arena_leaderboard.json")
				save = func(name string) error { return arena.Save(&arena.Board{Entries: []arena.Entry{{RunID: name}}}) }
				load = func() string { return arena.Load().Entries[0].RunID }
			case "highscore":
				path = storage.AppSavePath("highscores.json")
				save = func(name string) error {
					return highscore.Save(&highscore.Board{Entries: []highscore.Entry{{PlayerName: name}}})
				}
				load = func() string {
					b, err := highscore.Load()
					if err != nil {
						t.Fatal(err)
					}
					return b.Entries[0].PlayerName
				}
			case "stash":
				path = storage.AppSavePath("stash.json")
				save = func(name string) error {
					s := &stash.Stash{}
					s.Slots[0].Name = name
					return stash.Save(s)
				}
				load = func() string {
					s, err := stash.Load()
					if err != nil {
						t.Fatal(err)
					}
					return s.Slots[0].Name
				}
			case "journal":
				path = storage.AppSavePath("stash-transfer.json")
				save = func(name string) error { return stash.SaveTransferJournal(&stash.TransferJournal{ID: name}) }
				load = func() string {
					s, err := stash.LoadTransferJournal()
					if err != nil {
						t.Fatal(err)
					}
					return s.ID
				}
			}
			for _, name := range []string{"first", "replacement"} {
				if err := save(name); err != nil {
					t.Fatal(err)
				}
				if got := load(); got != name {
					t.Fatalf("roundtrip=%q", got)
				}
			}
			// Force replacement to fail against a directory, retaining its sentinel.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := path + "/previous"
			if err := os.WriteFile(sentinel, []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := save("failure"); err == nil {
				t.Fatal("invalid target accepted")
			}
			if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserved" {
				t.Fatal("failure changed existing target")
			}
		})
	}
}
