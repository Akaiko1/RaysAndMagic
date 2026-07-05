package game

import (
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// Instance-id dedupe: the shared chest is the authority on ownership. Each
// deposited item carries a unique items.Item.InstanceID; the chest holds the
// item (and its id). At load time we strip from the reloaded bag any item whose
// id the chest already owns, so reloading a save (even an older one) can't
// re-deposit a copy the chest still holds — closing the save-scum dupe.

// ensureStashLoaded loads the shared chest into g.stash once, normalizing its
// items against current YAML and stamping any legacy (zero-InstanceID) entries
// so future deposits/withdrawals carry stable ids. Returns false when the file
// exists but is unreadable, in which case g.stash is left nil so the caller
// bails rather than risk clobbering a corrupt chest with an empty one.
func (g *MMGame) ensureStashLoaded() bool {
	if g.stash != nil {
		return true
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

// stampPartyInstanceIDs assigns a fresh id to every party item (bag, all
// members' equipment + quick slots) that lacks one — the lazy migration for
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

// reconcilePartyAgainstStash drops from the party any item whose InstanceID
// the chest already holds — the load-time defence against re-depositing a copy
// the chest still owns. The sweep covers every place a save can carry an item
// — the bag AND all rosters' equipment/quick slots (a save written while the
// item was still EQUIPPED would otherwise reload it past a bag-only check and
// re-open the dupe). Zero-id (untracked/legacy) items are never stripped.
func (g *MMGame) reconcilePartyAgainstStash() {
	if g.party == nil || !g.ensureStashLoaded() {
		return
	}
	owned := make(map[uint64]bool)
	for i := range g.stash.Slots {
		if id := g.stash.Slots[i].InstanceID; id != 0 {
			owned[id] = true
		}
	}
	for i := range g.stash.CardSlots {
		if id := g.stash.CardSlots[i].InstanceID; id != 0 {
			owned[id] = true
		}
	}
	if len(owned) == 0 {
		return
	}
	chestOwns := func(it items.Item) bool {
		return it.InstanceID != 0 && owned[it.InstanceID]
	}

	// Strip the bag: a slice deletes by rebuilding in place around the removed.
	bag := g.party.Inventory[:0]
	for _, it := range g.party.Inventory {
		if !chestOwns(it) {
			bag = append(bag, it)
		}
	}
	g.party.Inventory = bag

	stripMember := func(m *character.MMCharacter) {
		if m == nil {
			return
		}
		for slot, it := range m.Equipment {
			if chestOwns(it) {
				delete(m.Equipment, slot)
			}
		}
		for i, qs := range m.QuickSlots {
			if qs != nil && chestOwns(*qs) {
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
}
