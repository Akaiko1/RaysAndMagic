package items

import (
	cryptorand "crypto/rand"
	"encoding/binary"
)

// instanceIDGen produces a fresh unique item InstanceID. It is a package var so
// tests can swap in a deterministic counter; production uses 64 random bits,
// where a collision across any realistic item count is astronomically unlikely
// (no global counter to persist and keep in sync across saves + the stash).
var instanceIDGen = randomInstanceID

func randomInstanceID() uint64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// crypto/rand should never fail; if it does, a zero ID just means this
		// item stays "untracked" (never wrongly stripped) rather than crashing.
		return 0
	}
	id := binary.LittleEndian.Uint64(b[:])
	if id == 0 {
		id = 1 // 0 is the "untracked" sentinel - never hand it out
	}
	return id
}

// NewInstanceID returns a fresh unique item instance id.
func NewInstanceID() uint64 { return instanceIDGen() }

// EnsureInstanceID stamps a fresh id onto an item that has none (0). Used at the
// factories (new items) and on load (legacy save/stash items), so every live
// item ends up tracked. Returns true if it assigned one; no-op (false) for a nil
// item, an empty slot, or an already-identified item.
func EnsureInstanceID(it *Item) bool {
	if it == nil || it.Name == "" || it.InstanceID != 0 {
		return false
	}
	it.InstanceID = instanceIDGen()
	return true
}
