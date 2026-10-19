package game

import (
	"math"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// CastEquippedSpell performs a magic attack using equipped spell (unified F key casting).
// Returns true for a handled attempt, including a refundable no-effect cast.
func (cs *CombatSystem) CastEquippedSpell() bool {
	caster := cs.game.party.Members[cs.game.selectedChar]

	// Stunned characters cannot start a combat action either.
	if !caster.CanUseCombatAction() {
		return false
	}

	spell, hasSpell := caster.Equipment[items.SlotQuickAction]
	if !hasSpell {
		return false // No spell equipped
	}
	if spell.Type != items.ItemBattleSpell && spell.Type != items.ItemUtilitySpell {
		return false // a technique, trap or flask: not cast here
	}

	spellID := spells.SpellID(spell.SpellEffect)
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		cs.game.AddCombatMessage(uitext.Text("combat.spell_failed", err.Error()))
		return false
	}

	// Quick-cast stays quiet on launch; the hit itself reports (anti-spam).
	return cs.castPlayerSpell(spellID, spellDef, caster, false)
}

// CastSelectedSpell casts the currently selected spell from the spellbook.
// Handled attempts, including no-effect casts, return the spell ID so callers
// consume a TB action and retain its RT cooldown on a later mode switch.
func (cs *CombatSystem) CastSelectedSpell() (bool, spells.SpellID) {
	currentChar := cs.game.party.Members[cs.game.selectedChar]

	// Prevent casting while down or stunned; utility healing cannot act as a revive.
	if !currentChar.CanUseCombatAction() {
		return false, ""
	}
	// SAME filtered list the spellbook UI numbers (schools with spells only) -
	// indexing the full school list desynced selection when a school was empty.
	schools := spellbookSchoolsWithSpells(currentChar)

	if cs.game.selectedSchool < 0 || cs.game.selectedSchool >= len(schools) {
		return false, ""
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell < 0 || cs.game.selectedSpell >= len(availableSpells) {
		return false, ""
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]
	selectedSpellDef, err := spells.GetSpellDefinitionByID(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage(uitext.Text("combat.spell_failed", err.Error()))
		return false, ""
	}

	if !cs.castPlayerSpell(selectedSpellID, selectedSpellDef, currentChar, true) {
		return false, ""
	}
	return true, selectedSpellID
}

// castResolvedSpell executes an internal cast, including explicit free procs.
// Player input must use castPlayerSpell to validate the caster's spellbook.
func (cs *CombatSystem) castResolvedSpell(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter, cost int, announce, countsAsAction bool) bool {
	return cs.castSpell(spellCastRequest{ID: spellID, Definition: def, Caster: caster, Cost: cost, Announce: announce, ActionProcs: countsAsAction}).handled()
}

func (cs *CombatSystem) castPlayerSpell(spellID spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter, announce bool) bool {
	return cs.castSpell(spellCastRequest{ID: spellID, Definition: def, Caster: caster,
		Cost: caster.MeditatedSPCost(def.SpellPointsCost), Announce: announce,
		PlayerInitiated: true, ActionProcs: true}).handled()
}

// weaponSpellEchoPct is the caster's spell_echo_pct (Verdant Eye scepter).
func weaponSpellEchoPct(caster *character.MMCharacter) int {
	if def := strongestWeaponPerk(caster, func(d *config.WeaponDefinitionConfig) float64 { return float64(d.SpellEchoPct) }); def != nil {
		return def.SpellEchoPct
	}
	return 0
}

func (cs *CombatSystem) applySpellEffect(spellID spells.SpellID, spellDef spells.SpellDefinition, caster *character.MMCharacter, announce bool) spellCastOutcome {
	if result := cs.tryCastAdvancedSupport(spellID, spellDef, caster); result != castNotHandled {
		return result
	}
	if result := cs.tryCastAdvancedOffense(spellID, spellDef, caster); result != castNotHandled {
		if result == castCommitted {
			cs.playSpellBuffFx(spellID)
		}
		return result
	}
	if result := cs.tryCastSpecialEffect(spellID, spellDef, caster); result != castNotHandled {
		if result == castCommitted {
			cs.playSpellBuffFx(spellID)
		}
		return result
	}

	castingSystem := spells.NewCastingSystem(cs.game.config)

	// Mortar spells (Stone Blossom): the arc ignores everything in flight and
	// detonates at a fixed distance - never the straight-line projectile path.
	if spellDef.IsProjectile && spellDef.MortarRangeTiles > 0 {
		if cs.castMortarSpell(spellID, spellDef, caster, announce) {
			return castCommitted
		}
		return castRejected
	}

	if spellDef.IsProjectile {
		mp, err := cs.newSpellProjectile(spellID, "", cs.game.camera.X, cs.game.camera.Y, cs.partyAttackAngle())
		if err != nil {
			cs.game.AddCombatMessage(uitext.Text("combat.spell_failed", err.Error()))
			return castRejected
		}
		speed := math.Hypot(mp.VelX, mp.VelY)
		mp.Launch = cs.partyProjectileLaunch(cs.partyAttackAngle(), speed*float64(mp.LifeTime), false)
		mp.VelX, mp.VelY = mp.Launch.dx*speed, mp.Launch.dy*speed
		// The bolt carries physics only; damage is authored HERE (effective
		// stats + mastery), once. Luck-based spell crit doubles the normal and
		// typed-true components together; Elemental GM converts only the
		// regular mastery bonus.
		_, _, totalDamage := cs.CalculateSpellDamage(spellID, caster)
		parts, isCrit := cs.rollSpellCritParts(spellID, caster, cs.spellDamageParts(spellID, caster, totalDamage))
		mp.Attacker, mp.Owner = caster, ProjectileOwnerPlayer
		mp.Damage, mp.TrueDamage, mp.Crit = parts.Normal, parts.True, isCrit
		mp.DisintegrateChance = cs.game.partyDisintegrateChance(mp.DisintegrateChance)
		if err := cs.launchSpellProjectile(mp); err != nil {
			cs.game.AddCombatMessage(uitext.Text("combat.spell_config_error", err.Error()))
			return castRejected
		}

		if announce {
			cs.game.AddCombatMessage(uitext.Text("combat.casting", spellDef.Name))
		}
		return castCommitted
	}

	if spellDef.IsUtility {
		// ApplyUtilitySpell resolves flags + message only; every NUMBER
		// (heal total, duration, stat bonuses) is computed here, once.
		result, err := castingSystem.ApplyUtilitySpell(spellID)
		if err != nil {
			cs.game.AddCombatMessage(uitext.Text("combat.spell_failed", err.Error()))
			return castRejected
		}
		if !result.Success {
			return castRejected
		}

		duration := cs.CalculateSpellDurationFrames(spellID, caster)
		timedBuffActivated := cs.activateUtilityTimedBuff(spellID, spellDef, result, duration) == timedBuffApplied

		// Stat-buff spells (Bless, ...) announce the ACTUAL granted bonus -
		// mastery-scaled and caster-dependent - so the chat can never drift from
		// the number really applied (e.g. Bless is +5 base, +10 only at GM).
		isStatBuff := spellDef.GrantsStatBuff()
		msg, summary := result.Message, logStyled{"", keywordColors["buff"]}
		if isStatBuff {
			if suffix := cs.grantSpellPartyBuff(spellID, spellDef, caster, duration, "").Summary(); suffix != "" {
				msg, summary.text = strings.TrimSpace(result.Message), " ("+suffix+")"
			}
		}
		cs.game.logCombat(logToneGood, "%s%s", msg, summary)

		// Apply healing
		if spellDef.HealAmount > 0 {
			_, _, totalHeal := cs.CalculateSpellHealing(spellID, caster)
			if spellDef.HealParty {
				// Mass Heal: restore every party member.
				cs.healWholeParty(totalHeal)
			} else {
				// The quick bar and the book: handHealTarget, skipping a hero
				// at 0 HP for the most wounded, else the caster; a self-only
				// heal stays on the caster.
				target := cs.handHealTarget(spellDef, cs.castHovered)
				if target >= 0 && !cs.game.party.Members[target].IsLiving() {
					target = cs.mostWoundedHealTarget(spellDef, 1)
				}
				if spellDef.TargetSelf || target < 0 {
					target = cs.findCharacterIndex(caster)
				}
				cs.healMember(target, totalHeal)
			}
		}

		// Stat-buff spells, by DATA (stat_bonus / stat_bonuses), not by ID -
		// any spell authored with a bonus block applies it; different buff
		// spells stack, recasting one refreshes it. Reuses the block already
		// resolved for the announcement above (this cast only, not the aggregate).
		if !isStatBuff && !timedBuffActivated {
			cs.game.setBuffStatus(string(spellID), duration)
		}
		cs.playSpellBuffFx(spellID)
		return castCommitted
	}

	return castRejected
}

// newSpellProjectile is the one spell-bolt constructor for every caster:
// spells.yaml physics from (x, y) heading angle, the blast radius and the
// disintegrate chance snapshotted. Callers add damage, owner and launch, then
// launchSpellProjectile.
func (cs *CombatSystem) newSpellProjectile(spellID spells.SpellID, idPrefix string, x, y, angle float64) (MagicProjectile, error) {
	p, err := spells.NewCastingSystem(cs.game.config).CreateProjectile(spellID, x, y, angle)
	if err != nil {
		return MagicProjectile{}, err
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return MagicProjectile{}, err
	}
	return MagicProjectile{
		ID: cs.game.GenerateProjectileID(idPrefix + string(spellID)),
		X:  p.X, Y: p.Y, VelX: p.VelX, VelY: p.VelY,
		LifeTime: p.LifeTime, Active: p.Active, SpellType: string(spellID), Size: p.Size,
		AoeTiles:           def.AoeRadiusTiles,
		DisintegrateChance: def.DisintegrateChance,
	}, nil
}

// launchSpellProjectile puts a built bolt in flight: listed, its hitbox
// registered at the spell's authored size.
func (cs *CombatSystem) launchSpellProjectile(mp MagicProjectile) error {
	physics, err := cs.game.config.GetSpellConfig(mp.SpellType)
	if err != nil {
		return err
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, mp)
	size := cs.game.config.ProjectileHitboxTiles(physics) * cs.game.config.GetTileSize()
	cs.game.collisionSystem.RegisterEntity(collision.NewEntity(mp.ID, mp.X, mp.Y, size, size, collision.CollisionTypeProjectile, false))
	return nil
}

// playSpellBuffFx plays the spell's buff overlay animation if it defines one
// (buff_fx_sprite in spells.yaml). Called from every successful cast branch -
// special-effect AND utility - so any spell can be given the animation by data.
func (cs *CombatSystem) playSpellBuffFx(spellID spells.SpellID) {
	if cfgDef, ok := config.GetSpellDefinition(string(spellID)); ok && cfgDef != nil {
		cs.game.playSpellCastFx(cfgDef)
	}
}

// activateUtilityTimedBuff maps authored utility-effect flags to the runtime
// state they control. The YAML flags decide whether an effect exists; the timed
// buff registry owns only its active flag, duration, and lifecycle hooks.
func (cs *CombatSystem) activateUtilityTimedBuff(
	spellID spells.SpellID,
	spellDef spells.SpellDefinition,
	result spells.UtilitySpellResult,
	duration int,
) timedBuffActivation {
	activation := timedBuffNotHandled
	activate := func(id spells.SpellID) {
		if cs.game.activateTimedBuffFrames(id, duration, false) == timedBuffApplied {
			activation = timedBuffApplied
		}
	}
	if result.WaterWalk {
		activate("walk_on_water")
	}
	if spellDef.Fly {
		activate("fly")
	} else if spellDef.TerrainPassage {
		// Other passage spells own their timer and do not become Fly aliases.
		activate(spellID)
	}
	if result.WaterBreathing {
		activate("water_breathing")
	}
	if result.Vision {
		// Torch Light and Wizard Eye have distinct runtime state, so their
		// authored spell ID selects the matching registry entry.
		activate(spellID)
	}
	return activation
}

// EquipSelectedSpell equips the selected spell as an item in a battle or utility slot
func (cs *CombatSystem) EquipSelectedSpell() {
	currentChar := cs.game.party.Members[cs.game.selectedChar]
	// Same filtered list the spellbook UI numbers - see CastSelectedSpell.
	schools := spellbookSchoolsWithSpells(currentChar)

	if cs.game.selectedSchool < 0 || cs.game.selectedSchool >= len(schools) {
		return
	}

	selectedSchool := schools[cs.game.selectedSchool]
	availableSpells := currentChar.GetSpellsForSchool(selectedSchool)

	if cs.game.selectedSpell < 0 || cs.game.selectedSpell >= len(availableSpells) {
		return
	}

	selectedSpellID := availableSpells[cs.game.selectedSpell]

	// Use centralized spell item creation - no fallbacks, no hardcoded mappings
	item, err := spells.CreateSpellItem(selectedSpellID)
	if err != nil {
		cs.game.AddCombatMessage(uitext.Text("combat.failed_to_create_spell_item", err.Error()))
		return
	}

	// Ready the spell as the quick action
	currentChar.Equipment[items.SlotQuickAction] = item
}
