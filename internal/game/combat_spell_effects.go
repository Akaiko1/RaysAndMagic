package game

import (
	"fmt"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// spellResistPierce returns the resistance-pierce percent for the given
// caster's spell. Elemental schools (including Light/Dark) use the separate
// Elemental Mastery skill; Body/Mind/Spirit retain their school-GM pierce.
func (cs *CombatSystem) spellResistPierce(caster *character.MMCharacter, spellType string) int {
	if caster == nil {
		return 0
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType))
	if err != nil {
		return 0
	}
	// One school decides BOTH the branch and the mastery: the school this caster
	// casts it with. A spell with no school at all resolves to no skill here and
	// answers 0 through the same path. Branching on the spell's primary while reading the caster's
	// filed skill let a Fire-filed page take the self-magic GM bonus.
	school := caster.SpellSchoolFor(def)
	if school.IsElemental() {
		if !caster.HasSkill(character.SkillElementalMastery) {
			return 0
		}
		return character.ElementalMasteryPiercePct(caster.SkillTier(character.SkillElementalMastery))
	}
	if ms := caster.SpellMasterySkill(def); ms != nil && ms.Mastery >= character.MasteryGrandMaster {
		return SelfMagicGMResistPiercePct
	}
	return 0
}

// effectiveSpellCost applies a Grandmaster meditator's flat percent spell-cost
// reduction. Single source used by every SP check/deduction site.
func (cs *CombatSystem) effectiveSpellCost(caster *character.MMCharacter, baseCost int) int {
	if caster != nil && caster.SkillTier(character.SkillMeditation) >= int(character.MasteryGrandMaster) {
		baseCost = baseCost * (100 - MeditationGMSpellCostReductionPct) / 100
	}
	return baseCost
}

// CalculatePersistentDamageZoneTickDamage is the per-tick damage of any
// persistent damage zone, including Hot Steam and Firewall. The YAML
// zone_tick_damage is the flat base, plus Intellect/divisor and the caster's
// school mastery. Single source of truth for the cast (tryCastPersistentDamageZone) and the
// tooltip, so the displayed number always matches the damage dealt.
func (cs *CombatSystem) CalculatePersistentDamageZoneTickDamage(def spells.SpellDefinition, char *character.MMCharacter) int {
	return character.SpellDamageBreakdown(def, char).Total
}

// CalculateInfernoDamage returns the whole normal-fire nova payload. Inferno
// has explicit YAML mastery scaling and never converts any part to true damage.
func (cs *CombatSystem) CalculateInfernoDamage(def spells.SpellDefinition, char *character.MMCharacter) int {
	return character.SpellDamageBreakdown(def, char).Total
}

// spellMasteryBonus returns +5 per mastery level for the spell's school.
func (cs *CombatSystem) spellMasteryBonus(char *character.MMCharacter, spellID spells.SpellID) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	return casterSpellMasteryTier(char, def) * MasterySpellEffectPerLevel
}

// tryCastSpecialEffect distinguishes an unrecognized effect from a handled
// no-op. Payment and successful-cast procs belong to executeSpellCast.
func (cs *CombatSystem) tryCastSpecialEffect(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	if cs.tryCastAoeStunBy(spellID, def, caster) {
		return castCommitted
	}
	if result := cs.tryCastJump(def, caster); result != castNotHandled {
		return result
	}
	if result := cs.tryCastSummon(def, caster); result != castNotHandled {
		return result
	}
	if cs.tryCastInferno(def, caster) {
		return castCommitted
	}
	if result := cs.tryCastPersistentDamageZone(spellID, def, caster); result != castNotHandled {
		return result
	}
	if cs.tryCastPartyBuff(spellID, def, caster) {
		return castCommitted
	}
	if result := cs.tryCastRaiseDead(def, caster); result != castNotHandled {
		return result
	}
	if result := cs.tryCastResurrect(def, caster); result != castNotHandled {
		return result
	}
	return cs.tryCastAwaken(def, caster)
}

// tryCastInferno handles party-centered nova spells (Inferno): every monster AND
// every party member takes the spell's full damage (cost x SpellDamagePerSP).
// MapWide burns the ENTIRE current map - no radius; the party always burns too
// (fire resistance is the intended answer). Gated on either trigger field.
func (cs *CombatSystem) tryCastInferno(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.PartyAoeRadiusTiles <= 0 && !def.MapWide {
		return false
	}
	dmg := cs.CalculateInfernoDamage(def, caster)
	radius := math.Inf(1) // MapWide: every monster on the map
	if !def.MapWide {
		radius = def.PartyAoeRadiusTiles * float64(cs.game.config.GetTileSize())
	}
	cx, cy := cs.game.camera.X, cs.game.camera.Y
	damageTypeStr := normalizeDamageTypeStr(def.School)
	// The nova's packet goes through the SAME builder as projectiles, zones and
	// mortars (spellDamageParts), so Strong Magic and any future packet-level
	// modifier reach it - a direct CalculateInfernoDamage total would silently
	// bypass them while the cast still pays their price.
	monsterParts := cs.spellDamageParts(def.ID, caster, dmg)
	monsterParts, _ = cs.spellPartsWithOutgoingBuff(monsterParts, damageTypeStr)
	resistPierce := cs.spellResistPierce(caster, string(def.ID))

	cs.game.AddCombatMessage(fmt.Sprintf("%s erupts around the party!", def.Name))

	// Monsters in range. A sealed (dormant) boss is invulnerable and inert -
	// skip it so the nova neither damages nor wakes it. On the unified world
	// "the map" is the party's REGION - MapWide must not burn the other four.
	regionScoped := def.MapWide && cs.game.openWorldActive()
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || isPurePartySummon(m) || m.IsDamageInvulnerable() ||
			Distance(cx, cy, m.X, m.Y) > radius {
			continue
		}
		if regionScoped && cs.game.questKillMapKey(m) != currentMapKey() {
			continue
		}
		if cs.tryDarkElfBindInstead(caster, m) {
			continue
		}
		dealt := cs.applyMonsterDamagePacket(
			m,
			singleMonsterDamagePacket(monsterParts, damageTypeStr, resistPierce),
			monsterDamageOptions{},
		)
		cs.markMonsterHit(m)
		cs.spawnMonsterHitBurst(m, damageTypeStr)
		if !m.IsAlive() {
			cs.game.collisionSystem.UnregisterEntity(m.ID)
			xpAwarded := cs.finishMonsterKill(m)
			cs.game.AddCombatMessage(fmt.Sprintf("%s is consumed by %s! (+%d XP)", m.Name, def.Name, xpAwarded))
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s takes %d from %s! (HP: %d/%d)",
				m.Name, dealt.Total(), def.Name, m.HitPoints, m.MaxHitPoints))
		}
	}

	// Ground-shaking novas topple what stands on the shaken ground.
	if def.StandeeDestroyChance > 0 {
		cs.topplePropsInRadius(cx, cy, radius, def.StandeeDestroyChance)
	}

	// The nova's own ground FX (graphics.nova_fx). Without one the cast is only
	// the per-monster bursts above, which is nothing at all when it hits empty
	// ground. Map-wide spells paint out to a visible reach, not the whole map.
	fxRadiusTiles := def.PartyAoeRadiusTiles
	if def.MapWide {
		fxRadiusTiles = mapWideNovaFxRadiusTiles
	}
	cs.game.spawnNovaFx(string(def.ID), cx, cy, fxRadiusTiles)

	// Inferno catches the party in its own blast; a spell that spares the party
	// (Earthquake) says so in YAML rather than in a name check here.
	if def.SparesParty {
		return true
	}
	// The self-splash rides the same boosted packet rule as the monster side
	// (minus the party's own outgoing buff, which never targets the party).
	partySplash := cs.spellDamageParts(def.ID, caster, dmg).Total()
	cs.forEachDamageablePartyMember(func(idx int, member *character.MMCharacter) {
		// This is the party's own self-splash, not a hostile spell hit, so Spell
		// Absorption cannot turn Inferno's drawback into healing.
		dealt := cs.damagePartyMemberElement(idx, member, partySplash, damageTypeStr, false)
		cs.game.AddCombatMessage(fmt.Sprintf("%s is scorched for %d! (HP: %d/%d)",
			member.Name, dealt, member.HitPoints, member.MaxHitPoints))
		cs.game.TriggerPartyFlame(idx) // flame-particle overlay on the burned card
	})
	return true
}

// tryCastRaiseDead handles Raise Dead: revives the first fallen ally that is
// Unconscious or Dead (NOT eradicated - that's Resurrect's domain) to
// ReviveHpPct% of max HP, clearing both conditions. No eligible target returns
// castNoEffect. Gated on ReviveHpPct > 0 so it never collides with Resurrect.
func (cs *CombatSystem) tryCastRaiseDead(def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	if def.ReviveHpPct <= 0 {
		return castNotHandled
	}
	var target *character.MMCharacter
	for _, m := range cs.game.party.Members {
		if m == nil || m.HasCondition(character.ConditionEradicated) {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) || m.HasCondition(character.ConditionDead) || m.HitPoints <= 0 {
			target = m
			break
		}
	}
	if target == nil {
		cs.game.AddCombatMessage("There is no fallen ally to raise.")
		return castNoEffect
	}
	target.RemoveCondition(character.ConditionUnconscious)
	target.RemoveCondition(character.ConditionDead)
	hp := target.MaxHitPoints * def.ReviveHpPct / 100
	if hp < 1 {
		hp = 1
	}
	target.HitPoints = hp
	cs.game.AddCombatMessage(fmt.Sprintf("%s is raised to %d HP!", target.Name, hp))
	return castCommitted
}

// tryCastAwaken handles the Awaken spell: rouses EVERY unconscious party member
// back to 1 HP (does not touch the truly dead/eradicated - that's Resurrect).
// Shared by both cast paths; returns castNoEffect when nobody needs awakening.
func (cs *CombatSystem) tryCastAwaken(def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	if !def.Awaken {
		return castNotHandled
	}
	revived := 0
	for _, m := range cs.game.party.Members {
		if m == nil || !m.HasCondition(character.ConditionUnconscious) {
			continue
		}
		m.RemoveCondition(character.ConditionUnconscious)
		if m.HitPoints < 1 {
			m.HitPoints = 1
		}
		revived++
	}
	if revived == 0 {
		cs.game.AddCombatMessage("No one is unconscious to awaken.")
		return castNoEffect
	}
	cs.game.AddCombatMessage(fmt.Sprintf("Awakening rouses %d fallen ally(s) back to 1 HP!", revived))
	return castCommitted
}

// tryCastResurrect handles the Resurrect spell: restores the first fallen party
// member (unconscious, dead, or even eradicated) - to full HP if FullHeal.
// Shared by both cast paths; returns castNoEffect when nobody needs revival.
func (cs *CombatSystem) tryCastResurrect(def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	if !def.Revive {
		return castNotHandled
	}
	var target *character.MMCharacter
	for _, m := range cs.game.party.Members {
		if m == nil {
			continue
		}
		if m.HasCondition(character.ConditionUnconscious) ||
			m.HasCondition(character.ConditionDead) ||
			m.HasCondition(character.ConditionEradicated) ||
			m.HitPoints <= 0 {
			target = m
			break
		}
	}
	if target == nil {
		cs.game.AddCombatMessage("There is no fallen ally to resurrect.")
		return castNoEffect
	}
	target.RemoveCondition(character.ConditionUnconscious)
	target.RemoveCondition(character.ConditionDead)
	target.RemoveCondition(character.ConditionEradicated)
	if def.FullHeal {
		target.HitPoints = target.MaxHitPoints
	} else if target.HitPoints <= 0 {
		target.HitPoints = 1
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s is restored to life!", target.Name))
	return castCommitted
}

// tryCastAoeStun handles AoE-stun effect spells (e.g. Darkness): if the spell
// has StunRadiusTiles > 0, every alive monster within that radius of the caster
// is stunned (RT frames + TB turns), no damage dealt. Shared by both cast
// paths. Returns true if it handled the spell (caller should stop).
func (cs *CombatSystem) tryCastAoeStun(spellID spells.SpellID, def spells.SpellDefinition) bool {
	return cs.tryCastAoeStunBy(spellID, def, nil)
}

func (cs *CombatSystem) tryCastAoeStunBy(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.StunRadiusTiles <= 0 {
		return false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radius := def.StunRadiusTiles * tileSize
	frames := def.StunDurationSeconds * cs.game.config.GetTPS()
	turns := def.StunDurationTurns
	stunned := 0
	for _, m := range cs.game.world.Monsters {
		if m == nil || !m.IsAlive() || isPurePartySummon(m) {
			continue
		}
		if Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) > radius {
			continue
		}
		if cs.tryDarkElfBindInstead(caster, m) {
			continue
		}
		if cs.applyStunDR(m, turns, frames, false) { // per-target DR; summary printed below
			stunned++
		}
	}
	// Flavor lead comes from the spell's own `message:` (Darkness engulfs, a
	// shockwave rips...); the count suffix is shared.
	lead := def.Message
	if lead == "" {
		lead = def.Name
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s - %d foe(s) stunned!", lead, stunned))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// casterSpellMasteryTier is THE spell-mastery tier: the mastery of the school
// this caster actually casts the spell with (SpellMasterySkill). Every ladder,
// bonus and tooltip line resolves through here - keyed off the spell's primary
// school instead, a dual-school page filed under the caster's other school
// scores 0 in the fight while the card shows the real tier.
func casterSpellMasteryTier(caster *character.MMCharacter, def spells.SpellDefinition) int {
	return character.SpellMasteryTier(caster, def)
}

func scaledSpellMasteryValue(def spells.SpellDefinition, caster *character.MMCharacter, base, max int) int {
	if base <= 0 || max <= base {
		return base
	}
	tier := casterSpellMasteryTier(caster, def)
	gmTier := int(character.MasteryGrandMaster)
	if tier <= 0 || gmTier <= 0 {
		return base
	}
	if tier > gmTier {
		tier = gmTier
	}
	return base + (max-base)*tier/gmTier
}

func scaledIncomingDamageReduction(def spells.SpellDefinition, caster *character.MMCharacter) int {
	return scaledSpellMasteryValue(def, caster, def.IncomingDamageReduction, def.IncomingDamageReductionGrandmaster)
}

// tryCastPartyBuff handles party combat-buff spells (Day of the Gods, Hour of
// Power, Stone Skin). If the spell carries any party-buff field it activates the
// buff for `duration` seconds and returns true. Shared by both cast paths.
func (cs *CombatSystem) tryCastPartyBuff(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.ResistBuffPct <= 0 && def.OutgoingDamageBonus <= 0 && def.IncomingDamageReduction <= 0 && def.ResistBuffSchoolPct <= 0 {
		return false
	}
	// Party-buff magnitudes may opt into mastery scaling with *_grandmaster
	// caps; spells without a cap stay flat at their authored base value.
	frames := cs.CalculateSpellDurationFrames(spellID, caster)
	cs.game.addCombatBuff(TimedCombatBuff{
		SpellID:         string(spellID),
		Frames:          frames,
		OutBonus:        scaledSpellMasteryValue(def, caster, def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster),
		OutDamageType:   def.OutgoingDamageType,
		InReduce:        scaledIncomingDamageReduction(def, caster),
		ResistPct:       scaledSpellMasteryValue(def, caster, def.ResistBuffPct, def.ResistBuffPctGrandmaster),
		ResistSchool:    def.ResistBuffSchool,
		ResistSchoolPct: def.ResistBuffSchoolPct,
	})
	cs.game.AddCombatMessage(fmt.Sprintf("%s empowers the party!", def.Name))
	cs.game.setUtilityStatus(spellID, frames)
	return true
}

// spellStatBuffBonuses resolves the stat-buff block a cast of spellID grants:
// per-stat `stat_bonuses:` maps are authored absolute; uniform `stat_bonus`
// may opt into mastery scaling with stat_bonus_grandmaster.
func (cs *CombatSystem) spellStatBuffBonuses(spellID spells.SpellID, caster *character.MMCharacter) character.StatBonuses {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return character.StatBonuses{}
	}
	if len(def.StatBonuses) > 0 {
		return character.StatBonusesFromMap(def.StatBonuses)
	}
	return character.UniformStatBonuses(cs.CalculateSpellStatBonus(spellID, caster))
}

// applyStatBuffSpell registers a stat-buff spell in the timed registry:
// different spells stack, recasting the same one refreshes it.
func (cs *CombatSystem) applyStatBuffSpell(spellID spells.SpellID, duration int, bonuses character.StatBonuses) {
	cs.applyStatBuffSpellFromSource(spellID, "", duration, bonuses)
}

func (cs *CombatSystem) applyStatBuffSpellFromSource(spellID spells.SpellID, sourceID string, duration int, bonuses character.StatBonuses) {
	cs.game.addStatBuff(TimedStatBuff{SpellID: string(spellID), SourceID: sourceID, Frames: duration, Bonuses: bonuses})
}
