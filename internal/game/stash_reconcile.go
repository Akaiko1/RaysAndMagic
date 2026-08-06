package game

import (
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// Stash-lineage dedupe: the shared chest is the authority on ownership. Each
// deposited item carries one or more items.StackLineage components; at load we
// strip only the components the chest owns, so reloading an older save cannot
// re-deposit units that still live in the cross-save stash.

// ensureStashLoaded loads the shared chest into g.stash once, normalizing its
// items against current YAML and stamping any legacy (zero-InstanceID) entries
// so future deposits/withdrawals carry stable ids. Returns false when the file
// exists but is unreadable, in which case g.stash is left nil so the caller
// bails rather than risk clobbering a corrupt chest with an empty one.
func (g *MMGame) ensureStashLoaded() bool {
	if g.stash != nil {
		return true
	}
	if !g.recoverPendingStashTransfer() {
		return false
	}
	s, err := stash.Load()
	if err != nil {
		return false
	}
	changed := false
	for i := range s.Slots {
		if !stash.IsEmpty(s.Slots[i]) {
			normalizeItemFromConfig(&s.Slots[i])
			if items.EnsureInstanceID(&s.Slots[i]) {
				changed = true
			}
		}
	}
	for i := range s.CardSlots {
		if !stash.IsEmpty(s.CardSlots[i]) {
			normalizeItemFromConfig(&s.CardSlots[i])
			if items.EnsureInstanceID(&s.CardSlots[i]) {
				changed = true
			}
		}
	}
	g.stash = s
	if changed {
		_ = stash.Save(g.stash) // one-time id migration of the chest file
	}
	return true
}

// recoverPendingStashTransfer completes a journal left by a crash or a failed
// write. A marker in any managed game save is the commit decision: if it carries
// the journal ID, the party side reached the new state; otherwise the stash
// returns to its pre-transfer snapshot before gameplay can read it.
func (g *MMGame) recoverPendingStashTransfer() bool {
	journal, err := stash.LoadTransferJournal()
	if err != nil {
		return false
	}
	if journal == nil {
		return true
	}
	selected := &journal.Before
	if stashTransferCommittedInAnySave(journal.ID) {
		selected = &journal.After
	}
	if err := stash.Save(selected); err != nil {
		return false
	}
	// A failed removal remains safe: the next recovery chooses the same snapshot
	// because no managed save's commit marker changed.
	return stash.ClearTransferJournal() == nil
}

// stashTransferCommittedInAnySave recognizes either the normal autosave or a
// manual save made after an autosave write failed. The journal ID is unique, so
// a marker from an already-finished older transfer cannot select this one.
func stashTransferCommittedInAnySave(journalID string) bool {
	if journalID == "" {
		return false
	}
	for row := 0; row <= saveRowCount; row++ {
		save, err := ReadGameSave(saveRowPath(row))
		if err == nil && save.StashTransferID == journalID {
			return true
		}
	}
	return false
}

// stampPartyInstanceIDs assigns a fresh id to every party item (bag, all
// members' equipment + quick slots) that lacks one - the lazy migration for
// items saved before instance ids existed. Returns whether anything was
// stamped, so the caller can persist the slot once and make the ids stick.
func (g *MMGame) stampPartyInstanceIDs() bool {
	if g.party == nil {
		return false
	}
	changed := false
	for i := range g.party.Inventory {
		if items.EnsureInstanceID(&g.party.Inventory[i]) {
			changed = true
		}
	}
	stampMember := func(m *character.MMCharacter) {
		if m == nil {
			return
		}
		for slot, it := range m.Equipment {
			if items.EnsureInstanceID(&it) {
				m.Equipment[slot] = it
				changed = true
			}
		}
		for i := range m.QuickSlots {
			if items.EnsureInstanceID(m.QuickSlots[i]) {
				changed = true
			}
		}
	}
	for _, m := range g.party.Members {
		stampMember(m)
	}
	for _, m := range g.party.Reserve {
		stampMember(m)
	}
	for _, m := range g.party.Captive {
		stampMember(m)
	}
	return changed
}

// reconcilePartyAgainstStash drops chest-owned units from every place a save
// can carry an item: bag, all rosters' equipment/quick slots, and the card
// collection. Stackable items can contain multiple origins after UI merges, so
// claims are consumed globally while walking the party. Zero-ID legacy items
// remain untracked until the normal ID migration stamps them.
func (g *MMGame) reconcilePartyAgainstStash() {
	if g.party == nil || !g.ensureStashLoaded() {
		return
	}
	// Chest ownership counts units by lineage. A merged stack can contain several
	// origins; retaining all of them avoids the old partial-withdrawal hole where
	// Party.AddItem collapsed the returned fragment's ID into a different stack.
	owned := make(map[uint64]int)
	claim := func(it items.Item) {
		if it.Stackable() {
			for _, part := range it.StackLineageParts() {
				owned[part.ID] += part.Quantity
			}
			return
		}
		if it.InstanceID != 0 {
			owned[it.InstanceID]++
		}
	}
	for i := range g.stash.Slots {
		claim(g.stash.Slots[i])
	}
	for i := range g.stash.CardSlots {
		claim(g.stash.CardSlots[i])
	}
	if len(owned) == 0 {
		return
	}
	// afterDedup returns the item minus chest-owned lineage units. A component
	// that survives a partial subtraction is rekeyed by items.Item so a later
	// reload cannot subtract the same stash claim again.
	afterDedup := func(it items.Item) (items.Item, bool) {
		kept, rekeyed := it.StripStashOwnedUnits(owned)
		if rekeyed {
			g.loadNeedsResave = true
		}
		return it, kept
	}

	// Strip the bag: a slice deletes by rebuilding in place around the removed.
	bag := g.party.Inventory[:0]
	for _, it := range g.party.Inventory {
		if kept, ok := afterDedup(it); ok {
			bag = append(bag, kept)
		}
	}
	g.party.Inventory = bag

	stripMember := func(m *character.MMCharacter) {
		if m == nil {
			return
		}
		for slot, it := range m.Equipment {
			if _, kept := afterDedup(it); !kept {
				delete(m.Equipment, slot)
			}
		}
		for i, qs := range m.QuickSlots {
			if qs == nil {
				continue
			}
			if kept, ok := afterDedup(*qs); ok {
				*m.QuickSlots[i] = kept
			} else {
				m.QuickSlots[i] = nil
			}
		}
	}
	for _, m := range g.party.Members {
		stripMember(m)
	}
	for _, m := range g.party.Reserve {
		stripMember(m)
	}
	for _, m := range g.party.Captive {
		stripMember(m)
	}
	for slot := 0; slot < MaxCardSlots; slot++ {
		if _, kept := afterDedup(g.cardSlots[slot].item); !kept {
			g.clearCardCollectionSlot(slot)
		}
	}
}
