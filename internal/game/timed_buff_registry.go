package game

import "ugataima/internal/status"

// Generic plumbing shared by every timed buff list keyed by source id (combat
// buffs, stat buffs): find / upsert / remove / tick all live here once, so the typed
// registries can't drift in behaviour (replace-on-recast, HUD status refresh,
// expiry filtering).

// keyedBuff is any timed buff entry keyed by its source id: a spell,
// config.TechniqueID, config.CrateID or a draught item key.
type keyedBuff interface {
	buffKey() string
	buffSourceID() string
}

// upsertBuff replaces the existing entry from the same source (recast refreshes
// duration/values rather than stacking duplicates) or appends a new one.
func upsertBuff[T keyedBuff](list []T, b T) []T {
	for i := range list {
		if list[i].buffKey() == b.buffKey() {
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
func buffRegistryID[T keyedBuff](b T) string {
	if sourceID := b.buffSourceID(); sourceID != "" {
		return sourceID
	}
	return b.buffKey()
}

// buffByID returns the active entry for a spell, if any.
func buffByID[T keyedBuff](list []T, spellID string) (T, bool) {
	for i := range list {
		if list[i].buffKey() == spellID {
			return list[i], true
		}
	}
	var zero T
	return zero, false
}

// removeBuffByID drops the entry owned by buffID and clears the actual spell's
// HUD status. Reports whether anything was removed.
func removeBuffByID[T keyedBuff](g *MMGame, list []T, buffID string) ([]T, bool) {
	for i := range list {
		if buffRegistryID(list[i]) == buffID {
			g.updateBuffStatus(list[i].buffKey(), 0, false)
			return append(list[:i], list[i+1:]...), true
		}
	}
	return list, false
}

// tickBuffList spends one tick of every entry's clock, refreshes its HUD
// status, and filters out the expired ones. advance returns the frames left
// (0: expired). Reports whether any entry expired so callers can re-derive
// aggregates.
func tickBuffList[T keyedBuff](g *MMGame, list []T, turn bool, advance func(*T, bool) int) ([]T, bool) {
	if len(list) == 0 {
		return list, false
	}
	w := 0
	expired := false
	for i := range list {
		left := advance(&list[i], turn)
		b := list[i]
		if left != 0 {
			g.updateBuffStatus(b.buffKey(), left, true)
			list[w] = b
			w++
		} else {
			g.updateBuffStatus(b.buffKey(), 0, false)
			expired = true
		}
	}
	return list[:w], expired
}

func (g *MMGame) phaseOwnsBuff(id string) bool {
	if b, ok := g.statBuffByID(id); ok {
		return b.SourceID == celestialProvidenceSourceID
	}
	if b, ok := g.combatBuffByID(id); ok {
		return b.SourceID == celestialProvidenceSourceID
	}
	return false
}

// tickBuffClock spends one tick of a buff's clock. Providence belongs to the
// calendar phase, regardless of combat mode: its clock follows the phase. Its
// source already survives saves and is replaced by an ordinary same-spell cast.
func (g *MMGame) tickBuffClock(source string, clock *status.DualClock, turn bool) {
	if source == celestialProvidenceSourceID {
		*clock = status.FromFrames(g.celestialProvidenceFramesLeft(), g.config.GetTPS())
		return
	}
	clock.Tick(turn)
}

// tickPartyBuffsTurn spends a turn of the party's buffs and device recharges.
func (g *MMGame) tickPartyBuffsTurn() {
	g.tickDeviceCooldowns(true)
	g.advanceCombatBuffs(true)
	g.advanceStatBuffs(true)
	g.advanceUtilityBuffs(true)
}

// resetTimedEffects drops every timed party effect family at once: stat buffs
// (re-deriving the aggregate), combat buffs, persistent damage zones, and the flag-based
// utility effects (torch / wizard eye / water - WITHOUT firing onExpire: a new
// game must not trigger the underwater return teleport). The ONE reset for
// new game; save load overwrites these via their restore* counterparts.
func (g *MMGame) resetTimedEffects() {
	g.partyRoot = status.DualClock{}
	g.partyHinder = PartyHinderState{}
	g.alchemy = AlchemyState{}
	g.harvestRuntime = harvestRuntime{}
	g.spatialReuse = status.DualClock{}
	// Clock Tower: a fresh run starts out of any fight, standing, no clocks.
	g.partyWasInFight = false
	g.openerCooldown, g.partyMoveClock = status.DualClock{}, status.DualClock{}
	g.rareBookMessage = ""
	g.brewAnimation = nil
	g.statBuffs = nil
	g.combatBuffs = nil
	g.recomputeStatBonuses()
	g.celestialBuffSpellID = ""
	g.persistentDamageZones = nil
	g.replaceFrozenWater(nil)
	g.identifiedMonster = nil
	g.nextPersistentDamageZoneFieldID = 0
	for _, b := range g.timedBuffs() {
		*b.active = false
		*b.clock = status.DualClock{}
	}
	g.torchLightRadius = 0
	g.underwaterReturnX = 0
	g.underwaterReturnY = 0
	g.underwaterReturnMap = ""
}
