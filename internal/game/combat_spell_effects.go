package game

import (
	"math"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
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

// CalculatePersistentDamageZoneTickDamage is the per-tick damage of any
// persistent damage zone, including Hot Steam and Firewall. The YAML
// zone_tick_damage is the flat base, plus Intellect/divisor and the caster's
// school mastery. Single source of truth for the cast (tryCastPersistentDamageZone) and the
// tooltip, so the displayed number always matches the damage dealt.
func (cs *CombatSystem) CalculatePersistentDamageZoneTickDamage(def spells.SpellDefinition, char *character.MMCharacter) int {
	return character.SpellDamageBreakdown(def, char).Total
}

// CalculatePartyNovaDamage returns a party nova's whole payload (Inferno,
// Earthquake): its YAML mastery scaling, never converted to true damage.
func (cs *CombatSystem) CalculatePartyNovaDamage(def spells.SpellDefinition, char *character.MMCharacter) int {
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
	if cs.tryCastPartyNova(def, caster) {
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

// tryCastPartyNova handles the novas centered on the party (Inferno,
// Earthquake): every monster in reach takes the spell's full damage, and the
// party does too unless the spell spares it (Earthquake). MapWide reaches the
// whole current region - no radius. Gated on either trigger field.
func (cs *CombatSystem) tryCastPartyNova(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.PartyAoeRadiusTiles <= 0 && !def.MapWide {
		return false
	}
	dmg := cs.CalculatePartyNovaDamage(def, caster)
	radius := math.Inf(1) // MapWide: every monster on the map
	if !def.MapWide {
		radius = def.PartyAoeRadiusTiles * float64(cs.game.config.GetTileSize())
	}
	cx, cy := cs.game.camera.X, cs.game.camera.Y
	damageTypeStr := def.DamageType()
	// The nova's packet goes through the SAME builders as projectiles and
	// mortars (spellDamageParts, partySpellAttack), so Strong Magic and any
	// future packet-level modifier reach it - a direct CalculatePartyNovaDamage
	// total would silently bypass them while the cast still pays their price.
	attack := cs.partySpellAttack(caster, string(def.ID), cs.spellDamageParts(def.ID, caster, dmg), false)

	cs.game.logCombatLine(logToneGood, uitext.Text("combat.erupts_around_the_party", logSlot), logSchoolWord(def.School, def.Name))

	// Monsters in range. A sealed (dormant) boss is invulnerable and inert -
	// skip it so the nova neither damages nor wakes it. On the unified world
	// "the map" is the party's REGION - MapWide must not burn the other four.
	// It burns what stands there now, wherever the monster was born; a
	// corridor counts to its side.
	regionScoped := def.MapWide && cs.game.openWorldActive()
	ts := float64(cs.game.config.GetTileSize())
	hurts := func(m *monsterPkg.Monster3D) bool {
		return partyAreaHurts(m) && (!regionScoped || cs.game.mapKeyAtWorldTile(cs.game.world, TileIndex(m.X, ts), TileIndex(m.Y, ts)) == currentMapKey())
	}
	// A nova fills the ground around the party: walls do not shield from it.
	nova := areaReach{x: cx, y: cy, radius: radius, throughWalls: true}
	blow := partyBlow{caster: caster, attack: attack, name: def.Name, school: damageTypeStr, kind: damageLogIndirect}
	cs.forEachAreaVictim(nova, hurts, func(m *monsterPkg.Monster3D) {
		cs.landPartyHit(m, blow)
	})

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
		cs.game.reportDamage(logToneBad, damageLog{
			Source: logSchoolWord(damageTypeStr, def.Name),
			Target: heroLogTarget(member),
			Amount: dealt,
			School: damageTypeStr,
			Kind:   damageLogScorch,
		})
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
		cs.game.AddCombatMessage(uitext.Text("combat.there_is_no_fallen_ally_to_2"))
		return castNoEffect
	}
	target.Revive(target.MaxHitPoints*def.ReviveHpPct/100, false)
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.is_raised_to_hp", logSlot, logSlot), logHeroName(target), logHealed(target.HitPoints))
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
		m.Revive(1, false)
		revived++
	}
	if revived == 0 {
		cs.game.AddCombatMessage(uitext.Text("combat.no_one_is_unconscious_to_awaken"))
		return castNoEffect
	}
	cs.game.AddCombatMessage(uitext.Text("combat.awakening_rouses_fallen_ally_s_back", revived))
	return castCommitted
}

// tryCastResurrect handles the Resurrect spell: restores a fallen party member
// (unconscious, dead, or even eradicated) - to full HP if FullHeal. The
// eradicated go first in slot order, since nothing else can bring them back.
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
		if m.HasCondition(character.ConditionEradicated) {
			target = m
			break
		}
		if target == nil && (m.HasCondition(character.ConditionUnconscious) || m.HasCondition(character.ConditionDead) || m.HitPoints <= 0) {
			target = m
		}
	}
	if target == nil {
		cs.game.AddCombatMessage(uitext.Text("combat.there_is_no_fallen_ally_to"))
		return castNoEffect
	}
	hp := 1
	if def.FullHeal {
		hp = target.MaxHitPoints
	}
	target.Revive(hp, true)
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.status.poisoned", logSlot, logSlot), logHeroName(target), logKeyword("heal", "restored to life"))
	return castCommitted
}

func (cs *CombatSystem) tryCastAoeStunBy(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.StunRadiusTiles <= 0 {
		return false
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radius := def.StunRadiusTiles * tileSize
	frames, rounds := config.EffectClock(def.StunDurationSeconds, cs.game.config.GetTPS())
	stunned := 0
	// A sealed or warded boss is immune to control as to damage.
	cs.forEachAreaVictim(cs.partyNova(radius), partyAreaHurts, func(m *monsterPkg.Monster3D) {
		if !cs.tryDarkElfBindInstead(caster, m) && cs.applyStunDR(m, rounds, false) { // per-target DR; summary printed below
			stunned++
		}
	})
	// Flavor lead comes from the spell's own `message:` (Darkness engulfs, a
	// shockwave rips...); the count suffix is shared.
	lead := def.Message
	if lead == "" {
		lead = def.Name
	}
	cs.game.AddCombatMessage(uitext.Text("combat.foe_s_stunned", lead, stunned))
	cs.game.setBuffStatus(string(spellID), frames)
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
	return scaledMasteryValueAt(base, max, casterSpellMasteryTier(caster, def))
}

// scaledMasteryValueAt interpolates base..max (the *_grandmaster cap) by tier.
func scaledMasteryValueAt(base, max, tier int) int {
	if base <= 0 || max <= base {
		return base
	}
	gmTier := int(character.MasteryGrandMaster)
	if tier <= 0 {
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
	if !def.GrantsCombatBuff() {
		return false
	}
	// Party-buff magnitudes may opt into mastery scaling with *_grandmaster
	// caps; spells without a cap stay flat at their authored base value.
	cs.grantSpellPartyBuff(spellID, def, caster, cs.CalculateSpellDurationFrames(spellID, caster), "")
	cs.game.logCombatLine(logToneGood, uitext.Text("combat.empowers_the_party", logSlot), logKeyword("buff", def.Name))
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

// grantSpellPartyBuff is the one path a buff spell's party payload takes, a
// cast (sourceID "") and a system grant (Celestial Providence) alike: its
// stat block into the stat registry, else its combat buff, at the caster's
// mastery. Different spells stack; the same one refreshes. It returns the
// stat block granted, empty for a combat buff.
func (cs *CombatSystem) grantSpellPartyBuff(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter, frames int, sourceID string) character.StatBonuses {
	if def.GrantsStatBuff() {
		bonuses := cs.spellStatBuffBonuses(spellID, caster)
		buff := TimedStatBuff{SpellID: string(spellID), SourceID: sourceID, Bonuses: bonuses}
		buff.Clock.ArmFrames(frames, cs.game.config.GetTPS())
		cs.game.addStatBuff(buff)
		return bonuses
	}
	cs.game.addCombatBuff(timedCombatBuffFromSpell(spellID, def, caster, frames, sourceID))
	return character.StatBonuses{}
}
