package game

import (
	"fmt"
	"slices"

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

// healReach is a heal's place in the triage chain: a heal that mends every
// member, the shared pool (Shared Life), one ally, then the caster alone.
type healReach uint8

const (
	healReachParty healReach = iota
	healReachPool
	healReachAlly
	healReachSelf
)

func healReachOf(def spells.SpellDefinition) healReach {
	switch {
	case def.Mechanic != nil && def.Mechanic.Kind == "shared_life":
		return healReachPool
	case def.HealParty:
		return healReachParty
	case def.TargetSelf:
		return healReachSelf
	default:
		return healReachAlly
	}
}

// healChainBefore orders two heals in the chain: by reach, then the larger
// HealAmount, then the pricier spell; the ID keeps map-ordered spellbooks
// in one stable order.
func healChainBefore(a, b spells.SpellDefinition) bool {
	if ra, rb := healReachOf(a), healReachOf(b); ra != rb {
		return ra < rb
	}
	if a.HealAmount != b.HealAmount {
		return a.HealAmount > b.HealAmount
	}
	if a.SpellPointsCost != b.SpellPointsCost {
		return a.SpellPointsCost > b.SpellPointsCost
	}
	return a.ID < b.ID
}

// knownHealChain is every heal the caster knows, in the order triage (C key,
// Space) tries them. A self-only heal joins only when it is the caster's one
// heal: whoever has another heals the party with it.
func knownHealChain(caster *character.MMCharacter) []spells.SpellDefinition {
	var chain []spells.SpellDefinition
	for _, school := range caster.MagicSchools {
		if school == nil {
			continue
		}
		for _, id := range school.KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(id); err == nil && def.IsHeal() &&
				!slices.ContainsFunc(chain, func(d spells.SpellDefinition) bool { return d.ID == def.ID }) {
				chain = append(chain, def)
			}
		}
	}
	if slices.ContainsFunc(chain, func(d spells.SpellDefinition) bool { return healReachOf(d) != healReachSelf }) {
		chain = slices.DeleteFunc(chain, func(d spells.SpellDefinition) bool { return healReachOf(d) == healReachSelf })
	}
	slices.SortFunc(chain, func(a, b spells.SpellDefinition) int {
		if healChainBefore(a, b) {
			return -1
		}
		if healChainBefore(b, a) {
			return 1
		}
		return 0
	})
	return chain
}

// healCastableNow: not under its personal reuse lockout, and payable.
func (cs *CombatSystem) healCastableNow(caster *character.MMCharacter, def spells.SpellDefinition) bool {
	return caster.SpellState.Reusable(string(def.ID)) && caster.SpellPoints >= cs.effectiveSpellCost(caster, def.SpellPointsCost)
}

// bestKnownHealSpell returns the first heal of the caster's chain it can cast
// now; a heal under its reuse lockout or past the purse hands over to the
// next. False if none.
func (cs *CombatSystem) bestKnownHealSpell(caster *character.MMCharacter) (spells.SpellID, bool) {
	for _, def := range knownHealChain(caster) {
		if cs.healCastableNow(caster, def) {
			return def.ID, true
		}
	}
	return "", false
}

// CastBestHealOnTarget casts the first castable heal of the selected
// character's chain (see bestKnownHealSpell) - bound to the C key. Party heals hit everyone; self-only heals (e.g.
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
		if def.Mechanic != nil {
			return cs.applySpellEffect(spellID, def, caster, true)
		}
		_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)

		// Party heal: restore everyone, ignore the single target.
		if def.HealParty {
			n := cs.healWholeParty(healAmount)
			cs.game.logCombat(logToneGood, "%s casts %s, healing %d allies for %s HP!",
				logHeroName(caster), logKeyword("heal", def.Name), n, logHealed(healAmount))
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
		if !target.IsLiving() {
			cs.game.AddCombatMessage(fmt.Sprintf("%s cannot be healed from 0 HP.", target.Name))
			return castRejected
		}

		cs.healMember(targetIndex, healAmount)
		if targetIndex == cs.game.selectedChar {
			cs.game.logCombat(logToneGood, "%s heals themselves for %s HP with %s!", logHeroName(caster), logHealed(healAmount), logKeyword("heal", def.Name))
		} else {
			cs.game.logCombat(logToneGood, "%s heals %s for %s HP with %s!", logHeroName(caster), logHeroName(target), logHealed(healAmount), logKeyword("heal", def.Name))
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
