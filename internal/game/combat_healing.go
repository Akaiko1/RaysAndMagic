package game

import (
	"fmt"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// healMember is the single source for applying a heal to ONE party member: add
// HP, clamp to max, clear Unconscious when revived above 0, and flash the green
// "+" overlay. Every single-target heal path funnels through here so the clamp /
// revive / VFX behaviour can't drift between them.
func (cs *CombatSystem) healMember(idx, amount int) {
	if idx < 0 || idx >= len(cs.game.party.Members) {
		return
	}
	m := cs.game.party.Members[idx]
	if m == nil {
		return
	}
	m.HitPoints += amount
	if m.HitPoints > m.MaxHitPoints {
		m.HitPoints = m.MaxHitPoints
	}
	if m.HitPoints > 0 {
		m.RemoveCondition(character.ConditionUnconscious)
	}
	cs.game.TriggerPartyHeal(idx) // rising green "+" overlay on the healed card
}

func (cs *CombatSystem) healWholeParty(amount int) int {
	healed := 0
	for i, m := range cs.game.party.Members {
		if m == nil || m.HitPoints <= 0 ||
			m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		cs.healMember(i, amount)
		healed++
	}
	return healed
}

func (cs *CombatSystem) CastEquippedHealOnTarget(targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Stunned characters cannot cast heals either.
	if !caster.CanUseCombatAction() {
		return false
	}

	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false
	}
	// Allow both heal-type spells for targeting
	if spell.SpellEffect != items.SpellEffectHealSelf && spell.SpellEffect != items.SpellEffectHealOther {
		return false
	}

	spellID := spells.SpellID(spell.SpellEffect)
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false
	}
	// SP gate, target resolution (self-only heals redirect to the caster),
	// 0-HP refusal, cost and messages all live in the ONE heal cast path.
	return cs.castKnownHealOn(spellID, def, targetIndex)
}

// bestKnownHealSpell returns the most powerful heal spell the caster knows
// across all their magic schools: party heals first (Mass Heal's 10-per-member
// outvalues a 20 single-target), then the larger HealAmount, then the pricier
// spell, then ID for a stable pick over map-ordered spellbooks. False if none.
// healRank orders known heals for bestKnownHealSpell. Party reach dominates raw
// magnitude, then magnitude, then cost; the ID tail keeps the choice stable.
type healRankKey struct {
	party  bool
	amount int
	cost   int
	id     string
}

func healRank(def spells.SpellDefinition, id spells.SpellID) healRankKey {
	return healRankKey{party: def.HealParty, amount: def.HealAmount, cost: def.SpellPointsCost, id: string(id)}
}

func (k healRankKey) beats(other healRankKey) bool {
	if k.party != other.party {
		return k.party
	}
	if k.amount != other.amount {
		return k.amount > other.amount
	}
	if k.cost != other.cost {
		return k.cost > other.cost
	}
	return k.id < other.id
}

func (cs *CombatSystem) bestKnownHealSpell(caster *character.MMCharacter) (spells.SpellID, bool) {
	var bestID spells.SpellID
	var best spells.SpellDefinition
	found := false
	for _, school := range caster.MagicSchools {
		if school == nil {
			continue
		}
		for _, id := range school.KnownSpells {
			def, err := spells.GetSpellDefinitionByID(id)
			if err != nil || !def.IsHeal() {
				continue
			}
			better := !found || healRank(def, id).beats(healRank(best, bestID))
			if better {
				bestID, best, found = id, def, true
			}
		}
	}
	return bestID, found
}

// CastBestHealOnTarget casts the selected character's strongest known heal
// (see bestKnownHealSpell) - bound to the C key. Party heals hit everyone; self-only heals (e.g.
// First Aid) ignore the requested target and heal the caster; other heals use
// targetIndex (resolved from the mouse by the caller). Returns whether a heal
// fired plus the spell used (for the real-time cooldown).
func (cs *CombatSystem) CastBestHealOnTarget(targetIndex int) (bool, spells.SpellID) {
	caster := cs.game.party.Members[cs.game.selectedChar]
	if !caster.CanUseCombatAction() {
		return false, ""
	}
	spellID, ok := cs.bestKnownHealSpell(caster)
	if !ok {
		// Silent no-op: the C-key cycle only ever targets known healers, so this
		// just guards stray callers - no chat spam.
		return false, ""
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return false, ""
	}
	if cs.castKnownHealOn(spellID, def, targetIndex) {
		return true, spellID
	}
	return false, ""
}

// castKnownHealOn pays for and casts a known (book) heal on a resolved party
// target. Party heals restore everyone; self-only heals always land on the
// caster; an out-of-range target falls back to the caster. Callers resolve def
// once and pass it in.
func (cs *CombatSystem) castKnownHealOn(spellID spells.SpellID, def spells.SpellDefinition, targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]
	req := spellCastRequest{ID: spellID, Definition: def, Caster: caster,
		Cost: cs.effectiveSpellCost(caster, def.SpellPointsCost), PlayerInitiated: true}
	outcome := cs.executeSpellCast(req, func() spellCastOutcome {
		_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)

		// Party heal: restore everyone, ignore the single target.
		if def.HealParty {
			n := cs.healWholeParty(healAmount)
			cs.game.AddCombatMessage(fmt.Sprintf("%s casts %s, healing %d allies for %d HP!",
				caster.Name, def.Name, n, healAmount))
			return castCommitted
		}

		// Single-target heal. Self-only heals (TargetSelf) always land on the caster.
		if def.TargetSelf {
			targetIndex = cs.game.selectedChar
		}
		if targetIndex < 0 || targetIndex >= len(cs.game.party.Members) {
			targetIndex = cs.game.selectedChar
		}
		target := cs.game.party.Members[targetIndex]
		if target.HitPoints <= 0 || target.HasCondition(character.ConditionDead) || target.HasCondition(character.ConditionEradicated) {
			cs.game.AddCombatMessage(fmt.Sprintf("%s cannot be healed from 0 HP.", target.Name))
			return castRejected
		}

		cs.healMember(targetIndex, healAmount)
		if targetIndex == cs.game.selectedChar {
			cs.game.AddCombatMessage(fmt.Sprintf("%s heals themselves for %d HP with %s!", caster.Name, healAmount, def.Name))
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s heals %s for %d HP with %s!", caster.Name, target.Name, healAmount, def.Name))
		}
		return castCommitted
	})
	if outcome == castCommitted {
		// Targeted heals retain their existing animal-only proc cadence.
		cs.tryAnimalBondingOnAction(caster)
		cs.game.playSpellSound(def)
	}
	return outcome.handled()
}
