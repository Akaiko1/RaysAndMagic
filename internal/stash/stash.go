// Package stash implements a cross-save shared chest. Unlike a party's
// inventory (which lives inside a single save slot), the stash is persisted to
// its own file in the app save dir, so rare drops deposited in one playthrough
// can be withdrawn in another. Mirrors the highscore package's global-file model.
package stash

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ugataima/internal/items"
	"ugataima/internal/storage"
)

// SlotCount is the number of general (any-item) stash cells.
const SlotCount = 8

// CardSlotCount is the number of extra cells that ONLY accept monster cards.
const CardSlotCount = 8

const fileName = "stash.json"

// Stash is the shared chest. An empty slot is the zero Item (Name == "").
// CardSlots are a separate bank restricted to monster cards (items.ItemCard);
// older stash.json files without the field load them empty (backward-compatible).
type Stash struct {
	Slots     [SlotCount]items.Item     `json:"slots"`
	CardSlots [CardSlotCount]items.Item `json:"card_slots"`
}

func path() string { return storage.AppSavePath(fileName) }

// IsEmpty reports whether a slot holds no item.
func IsEmpty(it items.Item) bool { return it.Name == "" }

// Load reads the stash from disk. A MISSING file yields an empty stash (first
// run). A present-but-unparseable file returns an error instead of an empty
// stash: a corrupted chest (e.g. an interrupted write) is surfaced to the player
// ("Could not open the stash") rather than silently emptied - which would let
// the next transfer overwrite the file with a blank state, permanently losing
// the deposited items.
func Load() (*Stash, error) {
	data, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Stash{}, nil
		}
		return nil, err
	}
	var s Stash
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the stash to disk atomically (temp file + rename), so a crash or
// interrupted write can't leave a half-written stash.json that Load would reject.
func Save(s *Stash) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
