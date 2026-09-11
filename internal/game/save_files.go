package game

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"time"

	"ugataima/internal/highscore"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// DefaultSavePath is the default file used for saving/loading
const DefaultSavePath = "savegame.json"

// saveRowFileName maps a global save-row index to its bare file name: the ONE
// place the autosave.json / save%d.json naming lives, so anything enumerating
// slot files can tell a save apart from a sibling runtime artifact in the same
// directory like arena_leaderboard.json. Name only, NO directory resolution -
// AppSaveDir picks between the bundle data root, the exe folder and the cwd, and
// creates the directory as a side effect.
func saveRowFileName(row int) string {
	if row == 0 {
		return autosaveFile
	}
	return fmt.Sprintf("save%d.json", row)
}

// saveRowPath maps a global save-row index to its file. Row 0 is the autosave.
func saveRowPath(row int) string {
	return storage.AppSavePath(saveRowFileName(row))
}

// GetSaveRowSummary reads minimal display info for a global save-row index.
func GetSaveRowSummary(row int) SaveSummary {
	return summaryFromPath(saveRowPath(row))
}

// saveSummaryCache memoizes summaries by path + (mtime, size): the save/load
// menus redraw every frame, and a cold summary is a FULL GameSave decode -
// without the cache the open menu re-parsed up to 8 save files per frame.
var saveSummaryCache = map[string]cachedSaveSummary{}

type cachedSaveSummary struct {
	modTime int64
	size    int64
	sum     SaveSummary
}

// Autosave writes the shared autosave slot (row 0). Best-effort and silent: a
// failure must never interrupt play. No-op outside live gameplay so it can't fire
// mid-load or before the world exists.
func (g *MMGame) Autosave() {
	_ = g.autosaveErr()
}

// autosaveErr is Autosave that reports a real write failure, so callers needing
// the bag/game state committed in step with another store (the stash) can detect
// it and roll back. A guard miss returns nil: there is no in-game state to
// autosave (the stash only opens in play, so this is the unit-test / not-in-game
// case), which is "nothing to commit", not a failure.
func (g *MMGame) autosaveErr() error {
	if g.appScreen != AppScreenInGame || world.GlobalWorldManager == nil || g.party == nil {
		return nil
	}
	return g.SaveGameToFile(saveRowPath(0))
}

// SaveSummary is lightweight info used for menu display
type SaveSummary struct {
	Exists    bool
	SavedAt   string
	MapKey    string
	TurnBased bool
	Name      string
	RunID     string           // playthrough id (GameSave.ArenaRunID) - names collide across runs (default roster), the run id doesn't
	PlayTime  string           // formatted elapsed play time ("" if unknown)
	Party     []SavePartyBrief // active roster for the hover tooltip
}

// SavePartyBrief is the per-member info shown in the save hover tooltip.
type SavePartyBrief struct {
	Name  string
	Level int
	Class int
}

// summaryFromPath reads minimal display info from a save file path.
// mintPlaythroughID mints the identity of a new run (random; persisted in
// every save as arena_run_id). See legacyRunID for pre-feature saves.
func mintPlaythroughID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// adoptPlaythroughID resolves the run identity when a save is loaded: a saved
// id is adopted VERBATIM; a pre-run-id save derives the DETERMINISTIC legacy
// id from its roster. Determinism here is load-bearing - an interim build that
// minted a RANDOM id on legacy load poisoned sibling saves of one run with
// divergent ids (the save-glow bug). Never mint randomness on this path.
func adoptPlaythroughID(saved string, memberNames []string) string {
	if saved != "" {
		return saved
	}
	return legacyRunID(memberNames)
}

// legacyRunID is the deterministic playthrough id for saves written BEFORE
// run ids existed: a hash of the sorted member names. Two legacy saves of the
// same roster map to the SAME id (so they highlight together and keep doing so
// after either is loaded); the default-roster name collision this reintroduces
// is confined to legacy saves, which carry no better signal.
func legacyRunID(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	h := fnv.New64a()
	for _, n := range sorted {
		h.Write([]byte(n))
		h.Write([]byte{'|'})
	}
	return fmt.Sprintf("legacy-%x", h.Sum64())
}

func summaryFromPath(path string) SaveSummary {
	st, err := os.Stat(path)
	if err != nil {
		delete(saveSummaryCache, path)
		return SaveSummary{Exists: false}
	}
	if c, ok := saveSummaryCache[path]; ok && c.modTime == st.ModTime().UnixNano() && c.size == st.Size() {
		return c.sum
	}
	f, err := os.Open(path)
	if err != nil {
		return SaveSummary{Exists: false}
	}
	defer f.Close()
	var s GameSave
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return SaveSummary{Exists: false}
	}
	sum := SaveSummary{Exists: true, SavedAt: s.SavedAt, MapKey: s.MapKey, TurnBased: s.TurnBased, Name: s.SaveName, RunID: s.ArenaRunID}
	if sum.RunID == "" { // pre-run-id save: derive the deterministic legacy id
		names := make([]string, 0, len(s.Party.Members))
		for _, m := range s.Party.Members {
			names = append(names, m.Name)
		}
		sum.RunID = legacyRunID(names)
	}
	if s.PlayedTimeNs > 0 {
		sum.PlayTime = highscore.FormatPlayTime(time.Duration(s.PlayedTimeNs))
	}
	for _, m := range s.Party.Members {
		sum.Party = append(sum.Party, SavePartyBrief{Name: m.Name, Level: m.Level, Class: m.Class})
	}
	saveSummaryCache[path] = cachedSaveSummary{modTime: st.ModTime().UnixNano(), size: st.Size(), sum: sum}
	return sum
}

// SaveGameToFile writes the current game state to a JSON file
func (g *MMGame) SaveGameToFile(path string) error {
	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	save := g.buildSave(wm)
	if f, err := os.Open(path); err == nil {
		var prev GameSave
		if err := json.NewDecoder(f).Decode(&prev); err == nil && prev.SaveName != "" {
			save.SaveName = prev.SaveName
		}
		_ = f.Close()
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(&save)
}

// RenameSaveSlot updates the stored save name for an existing slot, identified
// by its global save-row index (rows 1..; the autosave row is never renamed).
func RenameSaveSlot(row int, name string) error {
	path := saveRowPath(row)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()
	save.SaveName = name
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(&save)
}

// LoadGameFromFile loads state from a JSON file and applies it
func (g *MMGame) LoadGameFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		return err
	}

	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	g.loadNeedsResave = false
	if err := g.applySave(wm, &save); err != nil {
		return err
	}
	// One-time migration write: a load that stamped legacy items with instance
	// ids re-saves the slot so the ids stick (SaveGameToFile preserves the slot's
	// name). Without this the reloaded slot would revert to id-less items and stay
	// dupe-able. Best-effort - a write failure just defers the migration.
	if g.loadNeedsResave {
		g.loadNeedsResave = false
		_ = g.SaveGameToFile(path)
	}
	return nil
}
