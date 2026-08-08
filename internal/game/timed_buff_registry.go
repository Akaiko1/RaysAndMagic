package game

import "ugataima/internal/spells"

// Generic plumbing shared by every spell-keyed timed buff list (combat buffs,
// stat buffs): find / upsert / remove / tick all live here once, so the typed
// registries can't drift in behaviour (replace-on-recast, HUD status refresh,
// expiry filtering).

// spellKeyedBuff is any timed buff entry keyed by its source spell id.
type spellKeyedBuff interface {
	buffSpellID() string
	buffSourceID() string
}

// upsertBuff replaces the existing entry from the same spell (recast refreshes
// duration/values rather than stacking duplicates) or appends a new one.
func upsertBuff[T spellKeyedBuff](list []T, b T) []T {
	for i := range list {
		if list[i].buffSpellID() == b.buffSpellID() {
			list[i] = b
			return list
		}
	}
	return append(list, b)
}

// buffRegistryID is the ownership key used for explicit removal. Ordinary
// casts are owned by their spell id. A system-granted cast can use a distinct
// source id, so later cleanup cannot remove a player's refreshed cast of the
// same spell.
func buffRegistryID[T spellKeyedBuff](b T) string {
	if sourceID := b.buffSourceID(); sourceID != "" {
		return sourceID
	}
	return b.buffSpellID()
}

// buffByID returns the active entry for a spell, if any.
func buffByID[T spellKeyedBuff](list []T, spellID string) (T, bool) {
	for i := range list {
		if list[i].buffSpellID() == spellID {
			return list[i], true
		}
	}
	var zero T
	return zero, false
}

// removeBuffByID drops the entry owned by buffID and clears the actual spell's
// HUD status. Reports whether anything was removed.
func removeBuffByID[T spellKeyedBuff](g *MMGame, list []T, buffID string) ([]T, bool) {
	for i := range list {
		if buffRegistryID(list[i]) == buffID {
			g.updateUtilityStatus(spells.SpellID(list[i].buffSpellID()), 0, false)
			return append(list[:i], list[i+1:]...), true
		}
	}
	return list, false
}

// tickBuffList decrements every entry's frames, refreshes its HUD status, and
// filters out the expired ones. Reports whether any entry expired so callers
// can re-derive aggregates.
func tickBuffList[T spellKeyedBuff](g *MMGame, list []T, frames func(*T) *int) ([]T, bool) {
	if len(list) == 0 {
		return list, false
	}
	w := 0
	expired := false
	for i := range list {
		*frames(&list[i])--
		b := list[i]
		left := *frames(&b)
		if left > 0 {
			g.updateUtilityStatus(spells.SpellID(b.buffSpellID()), left, true)
			list[w] = b
			w++
		} else {
			g.updateUtilityStatus(spells.SpellID(b.buffSpellID()), 0, false)
			expired = true
		}
	}
	return list[:w], expired
}

// resetTimedEffects drops every timed party effect family at once: stat buffs
// (re-deriving the aggregate), combat buffs, persistent damage zones, and the flag-based
// utility effects (torch / wizard eye / water - WITHOUT firing onExpire: a new
// game must not trigger the underwater return teleport). The ONE reset for
// new game; save load overwrites these via their restore* counterparts.
func (g *MMGame) resetTimedEffects() {
	g.statBuffs = nil
	g.recomputeStatBonuses()
	g.combatBuffs = nil
	g.celestialBuffSpellID = ""
	g.persistentDamageZones = nil
	g.nextPersistentDamageZoneFieldID = 0
	for _, b := range g.timedBuffs() {
		*b.active = false
		*b.duration = 0
	}
	g.torchLightRadius = 0
	g.underwaterReturnX = 0
	g.underwaterReturnY = 0
	g.underwaterReturnMap = ""
}
