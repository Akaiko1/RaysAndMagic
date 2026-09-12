package game

import (
	"fmt"
	"math"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// CalculateSpellDamage returns base/stat/total damage for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual projectile damage.
func (cs *CombatSystem) CalculateSpellDamage(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0, 0, 0
	}
	result := character.SpellDamageBreakdown(def, char)
	return result.Base + result.Mastery, result.StatBonus, result.Total
}

// strongMagicPct is the caster's Strong Magic exchange percent for the given
// spell: the share of the SP cost burned as HP at cast, and the share added to
// the spell's damage. Zero when the passive does not apply (no skill, no
// caster, or a non-offensive spell).
func strongMagicPct(caster *character.MMCharacter, def spells.SpellDefinition) int {
	if caster == nil || !def.IsOffensive() || !caster.HasSkill(character.SkillStrongMagic) {
		return 0
	}
	return character.StrongMagicPct(caster.SkillTier(character.SkillStrongMagic))
}

// applyStrongMagicBurn is Strong Magic's HP price, paid at the SAME site the
// SP cost is paid (castResolvedSpell - the one payment point for offensive
// casts): pct% of the paid cost, clamped so the passive never takes the last
// hit point. The matching damage boost lives in spellDamageParts, so tooltips
// and combat read one number.
func (cs *CombatSystem) applyStrongMagicBurn(caster *character.MMCharacter, def spells.SpellDefinition, paidCost int) {
	pct := strongMagicPct(caster, def)
	if pct <= 0 || paidCost <= 0 {
		return
	}
	burn := paidCost * pct / 100
	if burn >= caster.HitPoints {
		burn = caster.HitPoints - 1
	}
	if burn <= 0 {
		return
	}
	caster.HitPoints -= burn
	cs.game.AddCombatMessage(fmt.Sprintf("%s's Strong Magic burns %d HP for power!", caster.Name, burn))
}

// spellDamageParts is the ONE damage builder for every party/champion cast
// (projectiles, zones, mortars, and both tooltips read it): the mastery
// true-damage split below, then the Strong Magic boost on the final packet.
func (cs *CombatSystem) spellDamageParts(spellID spells.SpellID, caster *character.MMCharacter, total int) damagecalc.Parts {
	parts := cs.spellMasteryDamageParts(spellID, caster, total)
	if def, err := spells.GetSpellDefinitionByID(spellID); err == nil {
		if pct := strongMagicPct(caster, def); pct > 0 {
			parts.Normal += parts.Normal * pct / 100
			parts.True += parts.True * pct / 100
		}
	}
	return parts
}

// spellPartsWithOutgoingBuff applies the party's flat outgoing-damage bonus
// after every spell-owned packet modifier (mastery, Strong Magic, and crit).
// The bonus always joins the Normal component and is never multiplied by those
// modifiers. Runtime spell forms and their tooltips share this final step.
func (cs *CombatSystem) spellPartsWithOutgoingBuff(parts damagecalc.Parts, damageType string) (damagecalc.Parts, int) {
	if cs == nil || cs.game == nil || parts.Normal <= 0 {
		return parts, 0
	}
	bonus := cs.game.combatBuffOutBonusForDamageType(damageType)
	parts.Normal += bonus
	return parts, bonus
}

// spellMasteryDamageParts converts only an elemental school's regular +5/tier mastery
// bonus to typed true damage at Grandmaster. A spell with its own explicit
// mastery step (currently Inferno's 45-90 scaling) remains entirely Normal.
func (cs *CombatSystem) spellMasteryDamageParts(spellID spells.SpellID, caster *character.MMCharacter, total int) damagecalc.Parts {
	parts := damagecalc.Parts{Normal: total}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || def.MasteryDamagePerTier > 0 || len(def.DamageByMastery) == 4 || caster == nil {
		return parts
	}
	// The school this caster holds the spell under decides BOTH the branch and
	// the mastery, exactly as in spellResistPierce: a dual-school page learned
	// through Air is scored against Air, and a page authored with only `schools:`
	// has no primary school to test at all.
	if !caster.SpellSchoolFor(def).IsElemental() {
		return parts
	}
	school := caster.SpellMasterySkill(def)
	if school == nil || school.Mastery < character.MasteryGrandMaster {
		return parts
	}
	bonus := cs.spellMasteryBonus(caster, spellID)
	if bonus > parts.Normal {
		bonus = parts.Normal
	}
	parts.Normal -= bonus
	parts.True = bonus
	return parts
}

// rollSpellCritParts rolls the universal player crit chance for a spell and
// doubles both components together. No-damage spells never crit.
func (cs *CombatSystem) rollSpellCritParts(spellID spells.SpellID, caster *character.MMCharacter, parts damagecalc.Parts) (damagecalc.Parts, bool) {
	if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.DealsNoDamage {
		return parts, false
	}
	if crit, _ := cs.RollCriticalChance(0, caster); crit {
		return spellCriticalParts(parts), true
	}
	return parts, false
}

func spellCriticalParts(parts damagecalc.Parts) damagecalc.Parts {
	parts.Normal *= CritDamageMultiplier
	parts.True *= CritDamageMultiplier
	return parts
}

// CalculateSpellHealing returns base/stat/total healing for a spell using the same formulas as combat.
// Base and total include mastery bonus to match tooltip display and actual healing.
func (cs *CombatSystem) CalculateSpellHealing(spellID spells.SpellID, char *character.MMCharacter) (int, int, int) {
	if cs == nil || cs.game == nil || char == nil {
		return 0, 0, 0
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0, 0, 0
	}
	result := character.SpellHealingBreakdown(def, char)
	return result.Base + result.Mastery, result.StatBonus, result.Total
}

// CalculateSpellDurationSeconds returns duration in seconds with mastery bonus applied.
func (cs *CombatSystem) CalculateSpellDurationSeconds(spellID spells.SpellID, char *character.MMCharacter) int {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return 0
	}
	return character.SpellDurationBreakdown(def, char).Seconds
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

// CalculateWeaponCritChance returns total weapon crit chance, clamped to [0,100].
// WeaponCritBreakdown decomposes the weapon crit chance into its components -
// the SAME pieces CalculateWeaponCritChance sums, so the tooltip's breakdown
// can't drift from the rolled total.
func (cs *CombatSystem) WeaponCritBreakdown(weapon items.Item, char *character.MMCharacter) (baseCrit, luck, cardCrit, setCrit, gmWeapon, gmArms int) {
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
	luck, cardCrit, setCrit = cs.CriticalChanceBreakdown(char)
	return baseCrit, luck, cardCrit, setCrit, gmWeapon, gmArms
}

func (cs *CombatSystem) CalculateWeaponCritChance(weapon items.Item, char *character.MMCharacter) int {
	baseCrit, luck, cardCrit, setCrit, gmWeapon, gmArms := cs.WeaponCritBreakdown(weapon, char)
	total := baseCrit + luck + cardCrit + setCrit + gmWeapon + gmArms
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
	for _, def := range equippedWeaponDefinitions(char) {
		if def != nil {
			total += def.ArmorClassBonus
		}
	}
	if cs.game.isPartyMember(char) {
		total += cs.game.cardArmorBonus()       // Treant Card: flat PARTY Armor Class
		total += cs.game.combatBuffArmorBonus() // stoneskin draught: timed flat AC
	}
	total += cs.game.partyArmorAuraBonusFor(char) // Parma shield wall: aura from OTHER members' gear
	if char.HasSkill(character.SkillIronBody) {
		// Iron Body: flat AC per tier, Novice included - a Monk's only AC
		// source besides Endurance, since they wear no armor at all.
		total += (char.SkillTier(character.SkillIronBody) + 1) * character.IronBodyACPerTier
	}
	// Drakehide Gauntlets: scales grown by taking hits this combat.
	if char.ScaleStacks > 0 {
		if per, capMax := char.ScaleStackParams(); per > 0 {
			stacks := char.ScaleStacks
			if stacks > capMax {
				stacks = capMax
			}
			total += stacks * per
		}
	}
	return total
}

func (g *MMGame) isPartyMember(char *character.MMCharacter) bool {
	if g == nil || g.party == nil || char == nil {
		return false
	}
	for _, member := range g.party.Members {
		if member == char {
			return true
		}
	}
	return false
}

// partyArmorAuraBonusFor sums party_armor_bonus from every OTHER member's
// equipped items (the Parma's shield wall: the bearer shelters the line, not
// themselves - the shield's own armor_class_base already covers them). Only
// PARTY MEMBERS stand in the wall: champion templates and other non-party
// characters run through CalculateTotalArmorClass too and must never borrow
// the party's shields.
func (g *MMGame) partyArmorAuraBonusFor(char *character.MMCharacter) int {
	if !g.isPartyMember(char) {
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
	return cs.weaponCooldownBreakdown(char, weaponName).TotalFrames
}

type weaponCooldownBreakdown struct {
	Speed                    int
	BaseFrames               float64
	WeaponMultiplier         float64
	DualWieldingReductionPct int
	RawFrames                int
	TotalFrames              int
}

func (cs *CombatSystem) weaponCooldownBreakdown(char *character.MMCharacter, weaponName string) weaponCooldownBreakdown {
	result := weaponCooldownBreakdown{WeaponMultiplier: 1, TotalFrames: RTCooldownMinFrames}
	if cs == nil || cs.game == nil || char == nil {
		return result
	}
	result.Speed = char.GetEffectiveSpeed()
	result.BaseFrames = float64(calculateSpeedActionCooldownFrames(result.Speed)) * RTBaseCooldownMult
	if weaponName != "" {
		if def, _, found := config.GetWeaponDefinitionByName(weaponName); found && def != nil {
			result.WeaponMultiplier = character.WeaponCooldownMultiplier(def)
		}
	}
	// Dual Wielding: -10%/tier ABOVE Novice on cooldown, either hand (Novice
	// itself only unlocks the off-hand weapon slot, no reduction yet).
	if tier := char.SkillTier(character.SkillDualWielding); char.HasSkill(character.SkillDualWielding) && tier > 0 {
		result.DualWieldingReductionPct = tier * character.DualWieldingCDReductionPerTier
	}
	dualWieldingMultiplier := 1.0 - float64(result.DualWieldingReductionPct)/100.0
	result.RawFrames = int(math.Round(result.BaseFrames * result.WeaponMultiplier * dualWieldingMultiplier))
	result.TotalFrames = clampRTCooldown(result.RawFrames)
	return result
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
// spell's authored cooldown_seconds at reference Speed, scaled by the caster's
// Speed and any equipped weapon's
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
	} else {
		// Unresolvable cast ID: there is no authored cooldown to honor, so only
		// the global floor applies (spells.yaml validation keeps this unreachable
		// for real content).
		return RTCooldownMinFrames
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
