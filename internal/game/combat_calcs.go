package game

import (
	"math"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// spellScalesWithPersonality reports whether a school's spell DAMAGE scales with
// Personality (self magic: Body/Mind/Spirit) instead of Intellect. Single source
// of truth for both the damage formula (CalculateSpellDamage) and the tooltip's
// stat-bonus label (spellDamageStatLabel), so they can never disagree. The school
// classification + label themselves live in the spells package (shared SSoT with
// EffectLines / the map editor); these thin wrappers keep the combat call sites.
func spellScalesWithPersonality(school string) bool {
	return spells.SchoolScalesWithPersonality(school)
}

func spellDamageStatLabel(school string, scalesWithPersonality bool) string {
	return spells.DamageStatLabel(school, scalesWithPersonality)
}

// CalculateSpellDamage returns base/stat/total damage for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual projectile damage.
func (cs *CombatSystem) CalculateSpellDamage(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	// Self magic (Body/Mind/Spirit) scales with Personality; all other schools
	// (elemental, Light, Dark) scale with Intellect. The math is stat-agnostic -
	// CalculateSpellDamageByID just divides the passed stat by SpellIntellectDivisor.
	def, defErr := spells.GetSpellDefinitionByID(spellID)
	selfMagic := defErr == nil && spellScalesWithPersonality(def.School)
	scalingStat := char.GetEffectiveIntellect()
	if selfMagic {
		scalingStat = char.GetEffectivePersonality()
	}
	baseDamage, intellectBonus, totalDamage := spells.CalculateSpellDamageByID(spellID, scalingStat)
	// Spells flagged scales_with_personality (e.g. ray_of_light) add a SECOND
	// Personality/divisor term on top of the primary term - but ONLY for non-self
	// magic, else Personality (already the primary stat for self magic) is counted
	// twice. The tooltip applies the same guard so the displayed number matches.
	if defErr == nil && def.ScalesWithPersonality && !selfMagic {
		perBonus := char.GetEffectivePersonality() / spells.SpellIntellectDivisor
		intellectBonus += perBonus
		totalDamage += perBonus
	}
	masteryBonus := cs.spellMasteryBonus(char, spellID)
	if masteryBonus > 0 {
		baseDamage += masteryBonus
		totalDamage += masteryBonus
	}
	return baseDamage, intellectBonus, totalDamage
}

// rollSpellCritDamage rolls Luck-based spell crit (no base crit) and returns the
// possibly-boosted damage plus whether it crit. No-damage spells (Disintegrate)
// never crit - the ONE place that rule lives, shared by every cast path (player
// projectile, champion cast, mortar) so none can drift.
func (cs *CombatSystem) rollSpellCritDamage(spellID spells.SpellID, caster *character.MMCharacter, base int) (int, bool) {
	if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.DealsNoDamage {
		return base, false
	}
	if crit, _ := cs.RollCriticalChance(0, caster); crit {
		return base * CritDamageMultiplier, true
	}
	return base, false
}

// CalculateSpellHealing returns base/stat/total healing for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual healing.
func (cs *CombatSystem) CalculateSpellHealing(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	effectivePersonality := char.GetEffectivePersonality()
	baseHeal, personalityBonus, totalHeal := spells.CalculateHealingAmountByID(spellID, effectivePersonality)
	masteryBonus := cs.spellMasteryBonus(char, spellID)
	if masteryBonus > 0 {
		baseHeal += masteryBonus
		totalHeal += masteryBonus
	}
	return baseHeal, personalityBonus, totalHeal
}

// CalculateSpellDurationSeconds returns duration in seconds with mastery bonus applied.
func (cs *CombatSystem) CalculateSpellDurationSeconds(spellID spells.SpellID, char *character.MMCharacter) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	if def.Duration <= 0 {
		return 0
	}
	seconds := def.Duration
	if char != nil && def.School != "" {
		school := character.MagicSchoolID(def.School)
		if skill, exists := char.MagicSchools[school]; exists && skill != nil {
			bonusPct := int(skill.Mastery) * SpellMasteryDurationBonusPct
			seconds = seconds * (100 + bonusPct) / 100
		}
	}
	return seconds
}

// CalculateSpellDurationFrames returns duration in frames with mastery bonus applied.
func (cs *CombatSystem) CalculateSpellDurationFrames(spellID spells.SpellID, char *character.MMCharacter) int {
	if cs == nil || cs.game == nil {
		return 0
	}
	seconds := cs.CalculateSpellDurationSeconds(spellID, char)
	if seconds <= 0 {
		return 0
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = config.GetTargetTPS()
	}
	return seconds * tps
}

// CalculateSpellStatBonus returns the spell's uniform stat bonus (e.g. Bless),
// including optional spell-school mastery scaling.
func (cs *CombatSystem) CalculateSpellStatBonus(spellID spells.SpellID, char *character.MMCharacter) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	return scaledSpellMasteryValue(def, char, def.StatBonus, def.StatBonusGrandmaster)
}

// CalculateWeaponCritChance returns total crit chance (weapon base + luck bonus), clamped to [0,100].
// WeaponCritBreakdown decomposes the weapon crit chance into its components -
// the SAME pieces CalculateWeaponCritChance sums, so the tooltip's breakdown
// can't drift from the rolled total.
func (cs *CombatSystem) WeaponCritBreakdown(weapon items.Item, char *character.MMCharacter) (baseCrit, luck, gmWeapon, gmArms int) {
	if def, _, ok := config.GetWeaponDefinitionByName(weapon.Name); ok && def != nil {
		baseCrit = def.CritChance
		// Grandmaster in this weapon's category: extra crit with it.
		if st, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok &&
			char != nil && char.SkillTier(st) >= int(character.MasteryGrandMaster) {
			gmWeapon = WeaponGMCritBonus
		}
	}
	// Grandmaster Arms Master: extra crit with ANY weapon.
	if char != nil && char.SkillTier(character.SkillArmsMaster) >= int(character.MasteryGrandMaster) {
		gmArms = ArmsMasterGMCritBonus
	}
	return baseCrit, cs.CalculateCriticalChance(char), gmWeapon, gmArms
}

func (cs *CombatSystem) CalculateWeaponCritChance(weapon items.Item, char *character.MMCharacter) int {
	baseCrit, luck, gmWeapon, gmArms := cs.WeaponCritBreakdown(weapon, char)
	total := baseCrit + luck + gmWeapon + gmArms
	if total < 0 {
		return 0
	}
	if total > 100 {
		return 100
	}
	return total
}

// CalculateArmorClassContribution returns this item's AC contribution based on endurance scaling and mastery.
func (cs *CombatSystem) CalculateArmorClassContribution(item items.Item, char *character.MMCharacter) int {
	if cs == nil || cs.game == nil || char == nil {
		return 0
	}
	return cs.armorClassContributionWithEnd(item, char, char.GetEffectiveEndurance())
}

// armorClassContributionWithEnd is the precomputed-endurance variant: the
// per-hit total loops every slot, and effective Endurance (a full equipment
// scan) is identical for all of them - compute it once, not per piece.
func (cs *CombatSystem) armorClassContributionWithEnd(item items.Item, char *character.MMCharacter, effectiveEndurance int) int {
	baseArmor := item.Attributes["armor_class_base"]
	baseArmor += cs.armorMasteryBonus(char, item)
	if enduranceDiv, ok := armorEnduranceScalingDivisor(item); ok {
		baseArmor += effectiveEndurance / enduranceDiv
	}
	return baseArmor
}

// armorEnduranceScalingDivisor is the design rule for armor AC scaling.
// Keep this aligned with assets/items.yaml:
//   - leather scales as END/10
//   - chain scales as END/7
//   - plate scales as END/5
//
// Cloth, shields, accessories, and any future armor category not explicitly
// listed here are flat AC: no Endurance contribution, even if stale YAML data
// accidentally contains endurance_scaling_divisor.
func armorEnduranceScalingDivisor(item items.Item) (int, bool) {
	div := item.Attributes["endurance_scaling_divisor"]
	if div <= 0 {
		return 0, false
	}
	switch strings.ToLower(item.ArmorCategory) {
	case "leather", "chain", "plate":
		return div, true
	default:
		return 0, false
	}
}

// CalculateTotalArmorClass returns total AC from all equipped armor slots.
func (cs *CombatSystem) CalculateTotalArmorClass(char *character.MMCharacter) int {
	if cs == nil || cs.game == nil || char == nil {
		return 0
	}
	total := 0
	effEnd := char.GetEffectiveEndurance() // one equipment scan for all slots
	armorSlots := []items.EquipSlot{
		items.SlotArmor,
		items.SlotHelmet,
		items.SlotBoots,
		items.SlotCloak,
		items.SlotGauntlets,
		items.SlotBelt,
		items.SlotOffHand, // shields carry armor_class_base too
	}
	for _, slot := range armorSlots {
		if armorPiece, hasArmor := char.Equipment[slot]; hasArmor {
			total += cs.armorClassContributionWithEnd(armorPiece, char, effEnd)
		}
	}
	// Hasta: an equipped weapon can grant its bearer flat AC (either hand).
	for _, slot := range []items.EquipSlot{items.SlotMainHand, items.SlotOffHand} {
		if w, ok := char.Equipment[slot]; ok && w.Type == items.ItemWeapon {
			if def := lookupWeaponConfigByName(w.Name); def != nil {
				total += def.ArmorClassBonus
			}
		}
	}
	total += cs.game.cardArmorBonus()             // Treant Card: flat party Armor Class
	total += cs.game.partyArmorAuraBonusFor(char) // Parma shield wall: aura from OTHER members' gear
	if char.HasSkill(character.SkillIronBody) {
		// Iron Body: flat AC per tier, Novice included - a Monk's only AC
		// source besides Endurance, since they wear no armor at all.
		total += (char.SkillTier(character.SkillIronBody) + 1) * character.IronBodyACPerTier
	}
	return total
}

// partyArmorAuraBonusFor sums party_armor_bonus from every OTHER member's
// equipped items (the Parma's shield wall: the bearer shelters the line, not
// themselves - the shield's own armor_class_base already covers them). Only
// PARTY MEMBERS stand in the wall: champion templates and other non-party
// characters run through CalculateTotalArmorClass too and must never borrow
// the party's shields.
func (g *MMGame) partyArmorAuraBonusFor(char *character.MMCharacter) int {
	if g == nil || g.party == nil {
		return 0
	}
	inParty := false
	for _, member := range g.party.Members {
		if member == char {
			inParty = true
			break
		}
	}
	if !inParty {
		return 0
	}
	total := 0
	for _, member := range g.party.Members {
		if member == nil || member == char {
			continue
		}
		for _, it := range member.Equipment {
			total += it.Attributes["party_armor_bonus"]
		}
	}
	return total
}

// CalculateSpellRangeTiles returns the configured range in tiles for a spell.
func (cs *CombatSystem) CalculateSpellRangeTiles(spellID spells.SpellID) (float64, bool) {
	def, ok := config.GetSpellDefinition(string(spellID))
	if !ok || def == nil || def.Physics == nil || def.Physics.RangeTiles <= 0 {
		return 0, false
	}
	return def.Physics.RangeTiles, true
}

// CalculateActionCooldownFrames returns the shared action cooldown used by input handling and tooltips.
func (cs *CombatSystem) CalculateActionCooldownFrames(char *character.MMCharacter) int {
	if cs == nil || cs.game == nil || char == nil {
		return 0
	}
	if cs.game.turnBasedMode {
		return inputDebounceCooldown
	}
	speed := char.GetEffectiveSpeed()
	return calculateSpeedActionCooldownFrames(speed)
}

func calculateSpeedActionCooldownFrames(speed int) int {
	frames := AttackCooldownIntercept - AttackCooldownSpeedSlope*float64(speed)
	cd := int(math.Round(frames))
	if cd < AttackCooldownMinFrames {
		return AttackCooldownMinFrames
	}
	if cd > AttackCooldownMaxFrames {
		return AttackCooldownMaxFrames
	}
	return cd
}

// clampRTCooldown clamps a real-time per-character cooldown to the sane range.
func clampRTCooldown(frames int) int {
	if frames < RTCooldownMinFrames {
		return RTCooldownMinFrames
	}
	if frames > RTCooldownMaxFrames {
		return RTCooldownMaxFrames
	}
	return frames
}

// WeaponCooldownFrames is the real-time cooldown after a weapon attack: the
// doubled Speed curve scaled by the weapon's category multiplier (or a
// per-weapon `cooldown_multiplier` override for legendaries). Unarmed = sword
// baseline. Speed still drives the underlying curve.
func (cs *CombatSystem) WeaponCooldownFrames(char *character.MMCharacter) int {
	weaponName := ""
	if char != nil {
		if weapon, ok := char.Equipment[items.SlotMainHand]; ok {
			weaponName = weapon.Name
		}
	}
	return cs.WeaponCooldownFramesFor(char, weaponName)
}

// OffHandWeaponCooldownFrames mirrors WeaponCooldownFrames for the off-hand
// weapon (Dual Wielding only) - the two hands cool down independently.
func (cs *CombatSystem) OffHandWeaponCooldownFrames(char *character.MMCharacter) int {
	weaponName := ""
	if char != nil {
		if weapon, ok := char.Equipment[items.SlotOffHand]; ok {
			weaponName = weapon.Name
		}
	}
	return cs.WeaponCooldownFramesFor(char, weaponName)
}

// WeaponCooldownFramesFor computes the real-time cooldown for a SPECIFIC
// weapon (tooltips hover unequipped weapons too) - the ONE formula combat and
// every tooltip share. Empty name = unarmed (sword baseline).
func (cs *CombatSystem) WeaponCooldownFramesFor(char *character.MMCharacter, weaponName string) int {
	if cs == nil || cs.game == nil || char == nil {
		return RTCooldownMinFrames
	}
	speed := char.GetEffectiveSpeed()
	base := float64(calculateSpeedActionCooldownFrames(speed)) * RTBaseCooldownMult
	mult := 1.0
	if weaponName != "" {
		if def, _, found := config.GetWeaponDefinitionByName(weaponName); found && def != nil {
			switch {
			case def.CooldownMultiplier > 0:
				mult = def.CooldownMultiplier // legendary / per-weapon override
			default:
				// Resolve the weapon's category to its canonical weapon SKILL
				// (so "throwing" -> dagger) and read that type's multiplier from
				// weapons.yaml. Categories with no skill (e.g. blaster) stay 1.0.
				if skill, ok := character.WeaponSkillForCategory(def.Category); ok {
					mult = config.WeaponCooldownMultiplierForSkill(skill.WeaponNoun())
				}
			}
		}
	}
	// Dual Wielding: -10%/tier ABOVE Novice on cooldown, either hand (Novice
	// itself only unlocks the off-hand weapon slot, no reduction yet).
	if tier := char.SkillTier(character.SkillDualWielding); char.HasSkill(character.SkillDualWielding) && tier > 0 {
		mult *= 1.0 - float64(tier*character.DualWieldingCDReductionPerTier)/100.0
	}
	return clampRTCooldown(int(math.Round(base * mult)))
}

// spellCooldownSpeedFactor scales a spell's authored cooldown_seconds by Speed,
// reusing the same Speed curve as weapons so faster casters also cast faster.
// 1.0 at the reference Speed, clamped to [Min, Max].
func spellCooldownSpeedFactor(speed int) float64 {
	ref := float64(calculateSpeedActionCooldownFrames(SpellCooldownSpeedRefSpeed))
	cur := float64(calculateSpeedActionCooldownFrames(speed))
	factor := cur / ref
	if factor < SpellCooldownSpeedFactorMin {
		return SpellCooldownSpeedFactorMin
	}
	if factor > SpellCooldownSpeedFactorMax {
		return SpellCooldownSpeedFactorMax
	}
	return factor
}

// SpellCooldownFrames is the real-time cooldown after casting spellID: the
// spell's authored cooldown_seconds (or a level-based default) at reference
// Speed, scaled by the caster's Speed and any equipped weapon's
// spell_cooldown_multiplier (e.g. Archmage Staff -20%). YAML category "buff"
// is the explicit exception: it has no personal RT cooldown.
func (cs *CombatSystem) SpellCooldownFrames(char *character.MMCharacter, spellID spells.SpellID) int {
	if cs == nil || cs.game == nil || char == nil {
		return RTCooldownMinFrames
	}
	seconds := 0.0
	if trapDef, ok := config.GetTrapDefinition(string(spellID)); ok {
		// SmartAttack returns trap keys through the same cast-ID channel.
		seconds = trapDef.CooldownSeconds
	} else if def, err := spells.GetSpellDefinitionByID(spellID); err == nil {
		if def.IsBuff() {
			return 0
		}
		seconds = def.CooldownSeconds
		if seconds <= 0 {
			seconds = SpellCooldownDefaultSecondsForLevel(def.Level)
		}
	} else {
		seconds = SpellCooldownDefaultSecondsForLevel(1)
	}
	speed := char.GetEffectiveSpeed()
	frames := seconds * float64(cs.game.config.GetTPS()) * spellCooldownSpeedFactor(speed)
	// Equipped-weapon spell-cooldown modifier (caster staff perk).
	if weapon, ok := char.Equipment[items.SlotMainHand]; ok {
		if def, _, found := config.GetWeaponDefinitionByName(weapon.Name); found && def != nil && def.SpellCooldownMultiplier > 0 {
			frames *= def.SpellCooldownMultiplier
		}
	}
	return clampRTCooldown(int(math.Round(frames)))
}

func (cs *CombatSystem) TrapCooldownFrames(char *character.MMCharacter, trapKey string) int {
	return cs.SpellCooldownFrames(char, spells.SpellID(trapKey))
}
