package game

import (
	"slices"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
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
	before := m.HitPoints
	m.HitPoints += amount
	if m.HitPoints > m.MaxHitPoints {
		m.HitPoints = m.MaxHitPoints
	}
	cs.queueOverflow(m, amount-(m.HitPoints-before))
	if m.HitPoints > 0 {
		m.RemoveCondition(character.ConditionUnconscious)
	}
	cs.game.TriggerPartyHeal(idx) // rising green "+" overlay on the healed card
}

func (cs *CombatSystem) healWholeParty(amount int) int {
	healed := 0
	for i, m := range cs.game.party.Members {
		if !m.IsLiving() {
			continue
		}
		cs.healMember(i, amount)
		healed++
	}
	return healed
}

// CastEquippedHeal casts the slotted heal (F) on handHealTarget's hero.
func (cs *CombatSystem) CastEquippedHeal() bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Stunned characters cannot cast heals either.
	if !caster.CanUseCombatAction() {
		return false
	}

	spell, hasSpell := caster.Equipment[items.SlotQuickAction]
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
	return cs.castHandHeal(spellID, def)
}

// castHandHeal casts a known heal by hand (F, C) on handHealTarget's hero.
// With nobody to heal it refuses before paying anything, like Space. The SP
// gate, 0-HP refusal, cost and messages live in castKnownHealOn.
func (cs *CombatSystem) castHandHeal(spellID spells.SpellID, def spells.SpellDefinition) bool {
	target := cs.handHealTarget(def, cs.game.heroUnderCursor())
	return target >= 0 && cs.castKnownHealOn(spellID, def, target)
}

// handHealTarget is the hero a heal cast by hand (F, C, the quick bar, the
// book) mends: the caster for a self-only heal, else hovered (the hero under
// the cursor), else the most wounded living hero; -1 when the heal has nobody
// hurt to mend. Space and a smart click ignore the cursor (smartHealPlan).
func (cs *CombatSystem) handHealTarget(def spells.SpellDefinition, hovered *character.MMCharacter) int {
	if def.TargetSelf {
		return cs.mostWoundedHealTarget(def, 1)
	}
	if i := cs.findCharacterIndex(hovered); hovered != nil && i >= 0 {
		return i
	}
	return cs.mostWoundedHealTarget(def, 1)
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
	case def.Mechanic != nil && def.Mechanic.Kind == config.MechanicSharedLife:
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

// bestKnownHealSpell returns the first heal of the caster's chain it can cast
// now; a heal under its reuse lockout or past the purse hands over to the
// next. False if none.
func (cs *CombatSystem) bestKnownHealSpell(caster *character.MMCharacter) (spells.SpellID, bool) {
	for _, def := range knownHealChain(caster) {
		if cs.spellCastableNow(caster, def, caster.MeditatedSPCost(def.SpellPointsCost)) {
			return def.ID, true
		}
	}
	return "", false
}

// CastBestHeal casts the first castable heal of the selected character's
// chain (see bestKnownHealSpell) - bound to the C key - on handHealTarget's
// hero. Returns whether a heal fired plus the spell used (for the real-time
// cooldown).
func (cs *CombatSystem) CastBestHeal() (bool, spells.SpellID) {
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
	if cs.castHandHeal(spellID, def) {
		return true, spellID
	}
	return false, ""
}

// castKnownHealOn pays for and casts a known (book) heal on a resolved party
// target. Party heals restore everyone; self-only heals always land on the
// caster; an out-of-range target refuses. Callers resolve def once and pass
// it in.
func (cs *CombatSystem) castKnownHealOn(spellID spells.SpellID, def spells.SpellDefinition, targetIndex int) bool {
	caster := cs.game.party.Members[cs.game.selectedChar]
	req := spellCastRequest{ID: spellID, Definition: def, Caster: caster,
		Cost: caster.MeditatedSPCost(def.SpellPointsCost), PlayerInitiated: true}
	outcome := cs.executeSpellCast(req, func() spellCastOutcome {
		if def.Mechanic != nil {
			return cs.applySpellEffect(spellID, def, caster, true)
		}
		_, _, healAmount := cs.CalculateSpellHealing(spellID, caster)

		// Party heal: restore everyone, ignore the single target.
		if def.HealParty {
			n := cs.healWholeParty(healAmount)
			cs.game.logCombatLine(logToneGood, uitext.Text("combat.casts_healing_allies_for_hp", logSlot, logSlot, n, logSlot), logHeroName(caster), logKeyword("heal", def.Name), logHealed(healAmount))
			return castCommitted
		}

		// Single-target heal. Self-only heals (TargetSelf) always land on the caster.
		if def.TargetSelf {
			targetIndex = cs.game.selectedChar
		}
		if targetIndex < 0 || targetIndex >= len(cs.game.party.Members) {
			return castRejected
		}
		target := cs.game.party.Members[targetIndex]
		if !target.IsLiving() {
			cs.game.AddCombatMessage(uitext.Text("combat.cannot_be_healed_from_0_hp", target.Name))
			return castRejected
		}

		cs.healMember(targetIndex, healAmount)
		if targetIndex == cs.game.selectedChar {
			cs.game.logCombatLine(logToneGood, uitext.Text("combat.heals_themselves_for_hp_with", logSlot, logSlot, logSlot), logHeroName(caster), logHealed(healAmount), logKeyword("heal", def.Name))
		} else {
			cs.game.logCombatLine(logToneGood, uitext.Text("combat.heals_for_hp_with", logSlot, logSlot, logSlot, logSlot), logHeroName(caster), logHeroName(target), logHealed(healAmount), logKeyword("heal", def.Name))
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
