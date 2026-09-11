package game

import (
	"fmt"
	"strings"

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

	spell, hasSpell := caster.Equipment[items.SlotSpell]
	if !hasSpell {
		return false // No spell equipped
	}
	if spell.Type == items.ItemTrap {
		// Thief quick slot: F arms the slotted trap exactly like a quick
		// spell - explicit cast, so refusal messages stay on.
		_, placed := cs.tryPlaceQuickTrap(caster, true)
		return placed
	}
	if spell.Type != items.ItemBattleSpell && spell.Type != items.ItemUtilitySpell {
		return false // SlotSpell should only contain spells
	}

	spellID := spells.SpellID(spell.SpellEffect)
	spellDef, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
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
		cs.game.AddCombatMessage("Spell failed: " + err.Error())
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
		Cost: cs.effectiveSpellCost(caster, def.SpellPointsCost), Announce: announce,
		PlayerInitiated: true, ActionProcs: true}).handled()
}

// weaponSpellEchoPct is the strongest spell_echo_pct across the caster's
// equipped weapons (Verdant Eye scepter - either hand counts).
func weaponSpellEchoPct(caster *character.MMCharacter) int {
	if caster == nil {
		return 0
	}
	best := 0
	for _, def := range equippedWeaponDefinitions(caster) {
		if def != nil && def.SpellEchoPct > best {
			best = def.SpellEchoPct
		}
	}
	return best
}

func (cs *CombatSystem) applySpellEffect(spellID spells.SpellID, spellDef spells.SpellDefinition, caster *character.MMCharacter, announce bool) spellCastOutcome {
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
		projectile, err := castingSystem.CreateProjectile(spellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return castRejected
		}
		// CreateProjectile carries physics only; damage is authored HERE
		// (effective stats + mastery), once.
		_, _, totalDamage := cs.CalculateSpellDamage(spellID, caster)
		if spellDef.DealsNoDamage {
			totalDamage = 0 // Disintegrate: only the instakill roll matters
		}

		// Resolve spell config before spawning anything so a config error
		// can't leave a projectile without a collision entity.
		spellConfig, err := cs.game.config.GetSpellConfig(string(spellID))
		if err != nil {
			cs.game.AddCombatMessage("Spell config error: " + err.Error())
			return castRejected
		}
		disintegrateChance := 0.0
		if spellDefConfig, exists := config.GetSpellDefinition(string(spellID)); exists && spellDefConfig != nil {
			disintegrateChance = spellDefConfig.DisintegrateChance
		}
		disintegrateChance += float64(cs.game.cardDisintegratePct()) / 100

		// Luck-based spell crit doubles the normal and typed-true components
		// together. Elemental GM converts only the regular mastery bonus.
		parts := cs.spellDamageParts(spellID, caster, totalDamage)
		var isCrit bool
		parts, isCrit = cs.rollSpellCritParts(spellID, caster, parts)
		projectile.Damage = parts.Normal

		magicProjectile := MagicProjectile{
			ID:                 cs.game.GenerateProjectileID(string(spellID)),
			Attacker:           caster,
			X:                  projectile.X,
			Y:                  projectile.Y,
			VelX:               projectile.VelX,
			VelY:               projectile.VelY,
			Damage:             projectile.Damage,
			TrueDamage:         parts.True,
			LifeTime:           projectile.LifeTime,
			Active:             projectile.Active,
			SpellType:          string(spellID),
			Size:               projectile.Size,
			Crit:               isCrit,
			DisintegrateChance: disintegrateChance,
			Owner:              ProjectileOwnerPlayer,
		}
		cs.game.magicProjectiles = append(cs.game.magicProjectiles, magicProjectile)

		tileSize := cs.game.config.GetTileSize()
		collisionSize := spellConfig.GetCollisionSizePixels(tileSize)
		projectileEntity := collision.NewEntity(magicProjectile.ID, magicProjectile.X, magicProjectile.Y, collisionSize, collisionSize, collision.CollisionTypeProjectile, false)
		cs.game.collisionSystem.RegisterEntity(projectileEntity)

		if announce {
			cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", spellDef.Name))
		}
		return castCommitted
	}

	if spellDef.IsUtility {
		// ApplyUtilitySpell resolves flags + message only; every NUMBER
		// (heal total, duration, stat bonuses) is computed here, once.
		result, err := castingSystem.ApplyUtilitySpell(spellID)
		if err != nil {
			cs.game.AddCombatMessage("Spell failed: " + err.Error())
			return castRejected
		}
		if !result.Success {
			return castRejected
		}

		duration := 0
		if spellDef.Duration > 0 {
			duration = cs.CalculateSpellDurationFrames(spellID, caster)
		}
		timedBuffActivated := cs.activateUtilityTimedBuff(spellID, spellDef, result, duration) == timedBuffApplied

		// Stat-buff spells (Bless, ...) announce the ACTUAL granted bonus -
		// mastery-scaled and caster-dependent - so the chat can never drift from
		// the number really applied (e.g. Bless is +5 base, +10 only at GM).
		isStatBuff := spellDef.StatBonus > 0 || len(spellDef.StatBonuses) > 0
		var statBuff character.StatBonuses
		msg := result.Message
		if isStatBuff {
			statBuff = cs.spellStatBuffBonuses(spellID, caster)
			if suffix := statBuff.Summary(); suffix != "" {
				msg = fmt.Sprintf("%s (%s)", strings.TrimSpace(result.Message), suffix)
			}
		}
		cs.game.AddCombatMessage(msg)

		// Apply healing
		if spellDef.HealAmount > 0 {
			_, _, totalHeal := cs.CalculateSpellHealing(spellID, caster)
			if spellDef.HealParty {
				// Mass Heal: restore every party member.
				cs.healWholeParty(totalHeal)
			} else {
				// Fallback self-heal (mouse-targeted heals go via CastEquippedHealOnTarget).
				cs.healMember(cs.findCharacterIndex(caster), totalHeal)
			}
		}

		// Town Portal: open the visited-destination picker; the teleport happens
		// on confirm. The no-destination case was already refused before the SP spend.
		if spellDef.TownPortal {
			cs.game.townPortalPickerOpen = true
		}

		// Stat-buff spells, by DATA (stat_bonus / stat_bonuses), not by ID -
		// any spell authored with a bonus block applies it; different buff
		// spells stack, recasting one refreshes it. Reuses the block already
		// resolved for the announcement above (this cast only, not the aggregate).
		if isStatBuff {
			cs.applyStatBuffSpell(spellID, duration, statBuff)
		}

		if !timedBuffActivated {
			cs.game.setUtilityStatus(spellID, duration)
		}
		cs.playSpellBuffFx(spellID)
		return castCommitted
	}

	return castRejected
}

// playSpellBuffFx plays the spell's buff overlay animation if it defines one
// (buff_fx_sprite in spells.yaml). Called from every successful cast branch -
// special-effect AND utility - so any spell can be given the animation by data.
func (cs *CombatSystem) playSpellBuffFx(spellID spells.SpellID) {
	if cfgDef, ok := config.GetSpellDefinition(string(spellID)); ok && cfgDef != nil {
		cs.game.playBuffFx(cfgDef.BuffFxSprite)
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
	}
	if result.WaterBreathing {
		activate("water_breathing")
	}
	if result.VisionRadiusTiles > 0 {
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
		cs.game.AddCombatMessage("Failed to create spell item: " + err.Error())
		return
	}

	// Equip the spell item in the unified spell slot
	currentChar.Equipment[items.SlotSpell] = item
}
