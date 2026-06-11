package game

import (
	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// TimedStatBuff is one active, timed party STAT buff (Bless, and any spell
// authored with stat_bonus / stat_bonuses). Different spells STACK additively —
// with each other AND with equipment bonuses (effective stat = base + gear +
// the sum of all active stat buffs). Recasting the same spell refreshes its
// entry instead of stacking a duplicate. This replaces the old single-slot
// blessActive/blessBonuses fields.
type TimedStatBuff struct {
	SpellID string // spell id (HUD status icon + replace-on-recast key)
	Frames  int    // frames remaining
	Bonuses character.StatBonuses
}

// addStatBuff activates a stat buff (same-spell recast refreshes) and pushes
// the new aggregate onto the party.
func (g *MMGame) addStatBuff(b TimedStatBuff) {
	for i := range g.statBuffs {
		if g.statBuffs[i].SpellID == b.SpellID {
			g.statBuffs[i] = b
			g.recomputeStatBonuses()
			return
		}
	}
	g.statBuffs = append(g.statBuffs, b)
	g.recomputeStatBonuses()
}

// recomputeStatBonuses derives g.statBonuses as the SUM of active stat buffs
// and pushes it onto every member — the only legal way the aggregate changes,
// so it can never drift from its sources (the old save-format bless bug).
func (g *MMGame) recomputeStatBonuses() {
	sum := character.StatBonuses{}
	for i := range g.statBuffs {
		sum = sum.Add(g.statBuffs[i].Bonuses)
	}
	g.statBonuses = sum
	g.applyPartyStatBonuses()
}

// tickStatBuffs decrements every active stat buff, refreshes its HUD status,
// and drops expired ones (re-deriving the aggregate). Called once per frame
// next to tickCombatBuffs.
func (g *MMGame) tickStatBuffs() {
	if len(g.statBuffs) == 0 {
		return
	}
	w := 0
	expired := false
	for i := range g.statBuffs {
		g.statBuffs[i].Frames--
		b := g.statBuffs[i]
		if b.Frames > 0 {
			g.updateUtilityStatus(spells.SpellID(b.SpellID), b.Frames, true)
			g.statBuffs[w] = b
			w++
		} else {
			g.updateUtilityStatus(spells.SpellID(b.SpellID), 0, false)
			expired = true
		}
	}
	g.statBuffs = g.statBuffs[:w]
	if expired {
		g.recomputeStatBonuses()
	}
}

// removeStatBuff drops a stat buff by spell id (dispel) and re-derives the
// aggregate. No-op if absent.
func (g *MMGame) removeStatBuff(spellID string) {
	for i := range g.statBuffs {
		if g.statBuffs[i].SpellID == spellID {
			g.statBuffs = append(g.statBuffs[:i], g.statBuffs[i+1:]...)
			g.updateUtilityStatus(spells.SpellID(spellID), 0, false)
			g.recomputeStatBonuses()
			return
		}
	}
}

// statBuffByID returns the active stat buff for a spell, if any (tests/UI).
func (g *MMGame) statBuffByID(spellID string) (TimedStatBuff, bool) {
	for i := range g.statBuffs {
		if g.statBuffs[i].SpellID == spellID {
			return g.statBuffs[i], true
		}
	}
	return TimedStatBuff{}, false
}

// StatBuffSave is the JSON form of a TimedStatBuff for save files.
type StatBuffSave struct {
	SpellID string         `json:"spell_id"`
	Frames  int            `json:"frames"`
	Bonuses map[string]int `json:"bonuses"`
}

// buildStatBuffSaves serializes the active stat-buff list for saving.
func buildStatBuffSaves(buffs []TimedStatBuff) []StatBuffSave {
	if len(buffs) == 0 {
		return nil
	}
	out := make([]StatBuffSave, len(buffs))
	for i, b := range buffs {
		out[i] = StatBuffSave{b.SpellID, b.Frames, statBonusesToMap(b.Bonuses)}
	}
	return out
}

// restoreStatBuffs rebuilds the stat-buff list from a save. Legacy saves
// (pre-registry) carry bless_* fields instead — the caller converts those to
// a single "bless" entry.
func restoreStatBuffs(saves []StatBuffSave) []TimedStatBuff {
	if len(saves) == 0 {
		return nil
	}
	out := make([]TimedStatBuff, len(saves))
	for i, s := range saves {
		out[i] = TimedStatBuff{s.SpellID, s.Frames, character.StatBonusesFromMap(s.Bonuses)}
	}
	return out
}
