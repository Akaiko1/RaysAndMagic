// Package stash implements a cross-save shared chest. Unlike a party's
// inventory (which lives inside a single save slot), the stash is persisted to
// its own file in the app save dir, so rare drops deposited in one playthrough
// can be withdrawn in another. Mirrors the highscore package's global-file model.
package stash

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"

	"ugataima/internal/items"
	"ugataima/internal/storage"
)

// SlotCount is the number of general (any-item) chest cells.
const SlotCount = 64

// AlbumCopies is how many copies of one card a card-album slot holds.
const AlbumCopies = 4

const fileName = "stash.json"

const transferJournalFileName = "stash-transfer.json"

// Stash is everything stored across saves. An empty cell is the zero Item
// (Name == ""). Slots are the tavern chest; Collection (the Storage Clerk's
// legendary stamps) holds one item per item key and Album up to AlbumCopies
// cards per card key.
type Stash struct {
	Slots      [SlotCount]items.Item   `json:"slots"`
	Collection map[string]items.Item   `json:"collection,omitempty"`
	Album      map[string][]items.Item `json:"album,omitempty"`
}

// UnmarshalJSON reads the current layout and moves the old card vault
// (card_slots) into free general cells, for the chest file and the transfer
// journal alike, so no reader ever sees the retired bank.
func (s *Stash) UnmarshalJSON(data []byte) error {
	type current Stash
	var wire struct {
		current
		CardSlots []items.Item `json:"card_slots"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*s = Stash(wire.current)
	for _, card := range wire.CardSlots {
		if IsEmpty(card) {
			continue
		}
		i := slices.IndexFunc(s.Slots[:], IsEmpty)
		if i < 0 {
			return fmt.Errorf("stash: no free cell for a card from the old card vault")
		}
		s.Slots[i] = card
	}
	return nil
}

// Place lands an item on chest cell idx under items.DropOnto, as Bag.Place
// does in a bag: onto a matching stack it merges, an empty cell takes it, onto
// any other item it takes the cell and returns the item it displaced. Callers
// refuse part of a stack onto another item before taking anything.
func (s *Stash) Place(idx int, it items.Item) (displaced items.Item) {
	cell := &s.Slots[idx]
	if items.DropOnto(it, *cell, true) == items.DropMerge {
		cell.MergeStack(it)
		return items.Item{}
	}
	displaced, *cell = *cell, it
	return displaced
}

// Each visits every stored item - chest cells, collection, album - in a fixed
// order and keeps whatever fn changes. It is the one walk over the stash, so
// anti-dupe claims, load normalization and every reader cover every container.
func (s *Stash) Each(fn func(*items.Item)) {
	for i := range s.Slots {
		if !IsEmpty(s.Slots[i]) {
			fn(&s.Slots[i])
		}
	}
	for _, key := range slices.Sorted(maps.Keys(s.Collection)) {
		it := s.Collection[key]
		fn(&it)
		s.Collection[key] = it
	}
	for _, key := range slices.Sorted(maps.Keys(s.Album)) {
		for i := range s.Album[key] {
			fn(&s.Album[key][i])
		}
	}
}

// Clone is a deep copy: a snapshot's collection and album pages never alias
// the live stash.
func (s Stash) Clone() Stash {
	out := s
	out.Collection = maps.Clone(s.Collection)
	if s.Album != nil {
		out.Album = make(map[string][]items.Item, len(s.Album))
		for key, page := range s.Album {
			out.Album[key] = slices.Clone(page)
		}
	}
	return out
}

func path() string { return storage.AppSavePath(fileName) }

func transferJournalPath() string { return storage.AppSavePath(transferJournalFileName) }

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
	return storage.WriteJSONAtomic(path(), s, 0644)
}

// TransferJournal makes a stash transfer recoverable across the independent
// stash and game-save files. The game writes it before either side changes; on
// the next stash access it chooses Before or After by checking a managed save's
// transaction marker.
type TransferJournal struct {
	ID     string `json:"id"`
	Before Stash  `json:"before"`
	After  Stash  `json:"after"`
}

// LoadTransferJournal returns nil, nil when no transaction needs recovery.
func LoadTransferJournal() (*TransferJournal, error) {
	data, err := os.ReadFile(transferJournalPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var journal TransferJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, err
	}
	if journal.ID == "" {
		return nil, os.ErrInvalid
	}
	return &journal, nil
}

// SaveTransferJournal persists a prepared transfer before stash.json or the
// party autosave changes. It uses the same atomic write contract as the stash.
func SaveTransferJournal(journal *TransferJournal) error {
	if journal == nil || journal.ID == "" {
		return os.ErrInvalid
	}
	return storage.WriteJSONAtomic(transferJournalPath(), journal, 0644)
}

// ClearTransferJournal acknowledges a completed or recovered transaction.
// A failed removal is safe: recovery will write the same selected state again.
func ClearTransferJournal() error {
	err := os.Remove(transferJournalPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
