package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestSave(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveSaveFileMovesAndDeduplicatesNames(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "archive")

	src := filepath.Join(dir, "save1.json")
	writeTestSave(t, src, `{"map_key":"forest"}`)
	dst1, err := archiveSaveFile(src, arch, "hero")
	if err != nil {
		t.Fatalf("archiveSaveFile: %v", err)
	}
	if filepath.Base(dst1) != "hero.json" {
		t.Errorf("first archive name = %s, want hero.json", filepath.Base(dst1))
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still exists after archiving")
	}

	// Same base again: must not overwrite the first archived file.
	writeTestSave(t, src, `{"map_key":"desert"}`)
	dst2, err := archiveSaveFile(src, arch, "hero")
	if err != nil {
		t.Fatalf("second archiveSaveFile: %v", err)
	}
	if dst2 == dst1 {
		t.Fatalf("second archive reused the same path %s", dst1)
	}
	if data, _ := os.ReadFile(dst1); string(data) != `{"map_key":"forest"}` {
		t.Errorf("first archived file was clobbered: %s", data)
	}
}

func TestRestoreArchiveFileRefusesOccupiedSlot(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "archived.json")
	dst := filepath.Join(dir, "save3.json")
	writeTestSave(t, src, "archived-body")
	writeTestSave(t, dst, "occupied-body")

	err := restoreArchiveFile(src, dst, "Slot 3")
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("restore into occupied slot: err = %v, want occupied error", err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "occupied-body" {
		t.Errorf("occupied slot was overwritten: %s", data)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("archived file lost after refused restore: %v", err)
	}

	// Free the slot: restore must move the file.
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if err := restoreArchiveFile(src, dst, "Slot 3"); err != nil {
		t.Fatalf("restore into free slot: %v", err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "archived-body" {
		t.Errorf("restored slot body = %s", data)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("archived file still present after restore")
	}
}

func TestArchiveRestoreRowValidation(t *testing.T) {
	if _, err := ArchiveSaveRow(0); err == nil {
		t.Error("ArchiveSaveRow(0) accepted the autosave row")
	}
	if _, err := ArchiveSaveRow(-2); err == nil {
		t.Error("ArchiveSaveRow(-2) accepted a negative row")
	}
	if _, err := ArchiveSaveRow(saveRowCount + 1); err == nil {
		t.Error("ArchiveSaveRow past the last slot was accepted")
	}
	if err := RestoreArchivedSave("x.json", 0); err == nil {
		t.Error("RestoreArchivedSave into the autosave row was accepted")
	}
	if err := RestoreArchivedSave("x.json", saveRowCount+1); err == nil {
		t.Error("RestoreArchivedSave past the last slot was accepted")
	}
}

func TestSanitizeFileBase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"My Hero Run", "My_Hero_Run"},
		{"slash/..\\evil", "slashevil"},
		{"", ""},
		{"a-b_c9", "a-b_c9"},
	}
	for _, tt := range tests {
		if got := sanitizeFileBase(tt.in); got != tt.want {
			t.Errorf("sanitizeFileBase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
