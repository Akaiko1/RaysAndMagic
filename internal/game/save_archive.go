package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"ugataima/internal/storage"
)

// Save archive: a sibling folder of the save slots holding saves parked by the
// editor's Saves page. Archived files keep the GameSave format but live outside
// the save%d.json / autosave.json naming, so the game's slot menus never see
// them; restoring is a rename back into a free slot file.

const archiveDirName = "archive"

// ArchiveDirPath is the on-disk folder holding archived saves.
func ArchiveDirPath() string { return storage.AppSavePath(archiveDirName) }

// SaveRowsTotal is the number of save-menu rows (autosave row 0 + manual slots).
func SaveRowsTotal() int { return saveRowCount + 1 }

// SaveRowFilePath is the save file backing a global save-row index.
func SaveRowFilePath(row int) string { return saveRowPath(row) }

// SaveRowDisplayName is the slot's display name ("Autosave" or "Slot N").
func SaveRowDisplayName(row int) string { return saveRowLabel(row) }

// IsAutosaveRow reports whether a row is the load-only autosave slot.
func IsAutosaveRow(row int) bool { return saveRowIsAutosave(row) }

// ReadGameSave decodes a full save file (slot or archive).
func ReadGameSave(path string) (*GameSave, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		return nil, err
	}
	return &save, nil
}

// ArchivedSave is one parked save file with its display summary.
type ArchivedSave struct {
	Path    string
	Summary SaveSummary
}

// ListArchivedSaves returns the archived saves, newest first. A missing
// archive folder is simply an empty list.
func ListArchivedSaves() []ArchivedSave {
	dir := ArchiveDirPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var files []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, candidate{path: filepath.Join(dir, e.Name()), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	out := make([]ArchivedSave, 0, len(files))
	for _, f := range files {
		out = append(out, ArchivedSave{Path: f.path, Summary: summaryFromPath(f.path)})
	}
	return out
}

// ArchiveSaveRow moves a manual slot's save file into the archive, freeing the
// slot. The autosave row is refused: it regenerates on the next map change, so
// archiving it frees nothing. Returns the archived file's path.
func ArchiveSaveRow(row int) (string, error) {
	if saveRowIsAutosave(row) {
		return "", errors.New("the Autosave slot regenerates and cannot be archived")
	}
	if row < 0 || row > saveRowCount {
		return "", fmt.Errorf("row %d is not a save slot", row)
	}
	src := saveRowPath(row)
	sum := summaryFromPath(src)
	if !sum.Exists {
		return "", fmt.Errorf("%s is empty", saveRowLabel(row))
	}
	return archiveSaveFile(src, ArchiveDirPath(), archiveBaseName(sum.Name, row))
}

// RestoreArchivedSave moves an archived save into a FREE manual slot. An
// occupied slot (and the load-only autosave row) is refused - archive it first.
func RestoreArchivedSave(archivePath string, row int) error {
	if saveRowIsAutosave(row) {
		return errors.New("the Autosave slot is load-only")
	}
	if row < 0 || row > saveRowCount {
		return fmt.Errorf("row %d is not a save slot", row)
	}
	return restoreArchiveFile(archivePath, saveRowPath(row), saveRowLabel(row))
}

// archiveBaseName is "<save name or slotN>_<timestamp>", sanitized to a safe
// filename base (no extension).
func archiveBaseName(name string, row int) string {
	base := sanitizeFileBase(name)
	if base == "" {
		base = fmt.Sprintf("slot%d", row)
	}
	return base + "_" + time.Now().Format("20060102-150405")
}

// sanitizeFileBase keeps [A-Za-z0-9_-], mapping spaces to underscores.
func sanitizeFileBase(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	return b.String()
}

// archiveSaveFile moves src into archiveDir as base.json, appending a counter
// on name collision. Returns the destination path.
func archiveSaveFile(src, archiveDir, base string) (string, error) {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(archiveDir, base+".json")
	for n := 2; ; n++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(archiveDir, fmt.Sprintf("%s_%d.json", base, n))
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// restoreArchiveFile moves src to dst, refusing when dst already exists.
func restoreArchiveFile(src, dst, dstLabel string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("archived save not found: %v", err)
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s is occupied - archive it first", dstLabel)
	}
	return os.Rename(src, dst)
}
