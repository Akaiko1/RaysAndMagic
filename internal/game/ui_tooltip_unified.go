package game

import (
	"fmt"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stats"
)

// The unified tooltip template (user-designed): every card renders as
//
//	=== Name ===
//	Category - Rarity/Level
//	SECTION
//	  base -> stat -> mastery -> total decomposition
//	...
//	RULES
//
// Empty sections and inapplicable lines are skipped; armor/resistance
// interaction is always spelled out; RT and TB values appear together.

// Shops and editor catalogs use these same builders with no character.
type ttSection = character.CardSection

// renderTooltip assembles the final text, dropping empty sections.
func renderTooltip(name, subtitle string, sections []ttSection, full bool) string {
	out := []string{name}
	if subtitle != "" {
		out = append(out, subtitle)
	}
	body := character.RenderCardLines(sections, full)
	if len(body) > 0 {
		out = append(out, "")
		out = append(out, body...)
	}
	// Compact view: tell the player a fuller breakdown exists (only if it does).
	if !full && character.SectionsHaveDetail(sections) {
		out = append(out, "", "[Shift] full breakdown")
	}
	return strings.Join(out, "\n")
}

// cooldownSeconds renders frames as "1.4s" (bare value, no label).
func cooldownSeconds(cs *CombatSystem, frames int) string {
	if cs == nil || cs.game == nil || frames <= 0 {
		return ""
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = config.DefaultTPS
	}
	return fmt.Sprintf("%.2fs", float64(frames)/float64(tps))
}

// cooldownLine renders the labeled RT/TB cooldown line for a frame count, or ""
// when there is no cooldown. Shares the wording with the editor cards.
func cooldownLine(cs *CombatSystem, frames int) string {
	if cs == nil || cs.game == nil || frames <= 0 {
		return ""
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = config.DefaultTPS
	}
	return character.CooldownLine(float64(frames) / float64(tps))
}

// masteryTier returns the tier and its display name for a skill ("Master").
func masteryTier(char *character.MMCharacter, skill character.SkillType) (int, string) {
	if char == nil {
		return 0, ""
	}
	sk, ok := char.Skills[skill]
	if !ok || sk == nil {
		return 0, ""
	}
	return int(sk.Mastery), sk.Mastery.String()
}

// spellMasteryTier is the magic-school analogue of masteryTier, for the school
// this character actually casts the spell with (SpellMasterySkill - the same
// lookup the damage, duration and pierce paths use).
func spellMasteryTier(char *character.MMCharacter, def spells.SpellDefinition) (int, string) {
	if char == nil {
		return 0, ""
	}
	ms := char.SpellMasterySkill(def)
	if ms == nil {
		return 0, ""
	}
	return int(ms.Mastery), ms.Mastery.String()
}

// statContribDetail renders "Accuracy (30 / 3): +10" into the full-only tier -
// the stat VALUE and the divisor the formula actually uses. A zero
// contribution still names the scaling stat (what to raise).
func statContribDetail(sec *ttSection, statName string, statValue, divisor int) {
	if statName == "" || divisor <= 0 {
		return
	}
	sec.AddDetail("%s (%d / %d): +%d", statName, statValue, divisor, statValue/divisor)
}

func statBreakdownDetails(sec *ttSection, result stats.Breakdown, char *character.MMCharacter) {
	for _, term := range result.Terms {
		if char == nil {
			sec.AddDetail("Scales with %s / %d", term.Stat, term.Divisor)
			continue
		}
		sec.AddDetail("%s (%d / %d): +%d", term.Stat, term.Value, term.Divisor, term.Bonus)
	}
}

// damageTypeAoELine / armorInteractionRules delegate to the shared template
// helpers so the editor's rules text is literally the same code (and lands in
// the full-only DETAIL tier - see character.ArmorInteractionLines).
func damageTypeAoELine(damageType string, aoeTiles float64) string {
	return character.DamageTypeAoELine(damageType, aoeTiles)
}

func armorInteractionRules(sec *ttSection, damageType string, isRanged, hasTrueDmg bool) {
	character.ArmorInteractionLines(sec, damageType, isRanged, hasTrueDmg)
}

// ---------------------------------------------------------------- weapons ---

func buildWeaponTooltipUnified(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	def := lookupWeaponConfigByName(item.Name)
	if def == nil {
		return item.Name
	}
	subtitle := config.TitleWords(strings.ReplaceAll(def.Category, "_", " "))
	if def.Rarity != "" {
		subtitle += " - " + config.TitleWords(def.Rarity)
	}

	attack := ttSection{Title: "ATTACK"}
	rangeTiles, speedTiles := character.EffectiveWeaponFlight(def, char)
	if def.Range > 0 {
		attack.Add("Range: %.0f tiles", rangeTiles)
		if rangeTiles != float64(def.Range) {
			attack.AddDetail("Base range: %d; Ballistics: %+.0f tiles", def.Range, rangeTiles-float64(def.Range))
		}
	}
	if def.Physics != nil && speedTiles > 0 {
		attack.AddDetail("Projectile Speed: %.1f tiles/s", speedTiles)
		if speedTiles != def.Physics.SpeedTiles {
			attack.AddDetail("Base projectile speed: %.1f; Ballistics: +%.0f%%", def.Physics.SpeedTiles, (speedTiles/def.Physics.SpeedTiles-1)*100)
		}
	}
	if arc := character.MeleeSwingArcLine(def); arc != "" {
		attack.Add("%s", arc)
	}
	addWeaponCooldown(&attack, char, cs, def)
	if character.WeaponStrikeCount(def) > 1 {
		attack.Add("Strikes per attack: %d", character.WeaponStrikeCount(def))
	}
	if def.MaxProjectiles > 0 {
		attack.AddDetail("Maximum Projectiles: %d", def.MaxProjectiles)
	}
	if def.Volley > 1 {
		attack.AddDetail("Volley: %d per shot", def.Volley)
	}

	dmg := ttSection{Title: "DAMAGE"}
	formula := character.WeaponDamageFormula(def)
	breakdown := character.WeaponDamageBreakdown(def, char)
	armsBonus, furyBonus := breakdown.ArmsMaster, breakdown.OrcishFury
	preview := cs.calculateWeaponDamagePreview(item, char)
	dmg.AddDetail("Base: %d", breakdown.Base)
	if char != nil {
		statBreakdownDetails(&dmg, breakdown.Breakdown, char)
	} else {
		for i, term := range formula.Terms {
			label := "Scales with"
			if i > 0 {
				label = "Also scales with"
			}
			dmg.AddDetail("%s %s / %d", label, term.Stat, term.Divisor)
		}
	}
	if armsBonus > 0 {
		_, tierName := masteryTier(char, character.SkillArmsMaster)
		dmg.AddDetail("Arms Master - %s: +%d", tierName, armsBonus)
	}
	if furyBonus > 0 {
		_, tierName := masteryTier(char, character.SkillOrcishFury)
		dmg.AddDetail("Orcish Fury - %s: +%d", tierName, furyBonus)
	}
	if line := character.WeaponStrikeFormulaLine(def); line != "" {
		dmg.AddDetail("%s", line)
	}
	isRanged := def.Range > 3
	if !isRanged && preview.OutgoingBuff > 0 {
		dmg.AddDetail("Active party buff: +%d", preview.OutgoingBuff)
	}
	if preview.CardDamagePct != 0 {
		mode := "melee"
		if isRanged {
			mode = "ranged"
		}
		dmg.AddDetail("Cards: +%d%% %s damage", preview.CardDamagePct, mode)
	}
	if isRanged && preview.OutgoingBuff > 0 {
		dmg.AddDetail("Active party buff: +%d", preview.OutgoingBuff)
	}
	if preview.AuthoredTrue > 0 {
		dmg.AddDetail("Weapon: +%d True", preview.AuthoredTrue)
	}
	masteryTrue := preview.True - preview.AuthoredTrue - preview.CardTrue
	if masteryTrue > 0 {
		if skill, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
			_, tierName := masteryTier(char, skill)
			dmg.AddDetail("%s Mastery - %s: +%d True", skill.String(), tierName, masteryTrue)
		}
	}
	if preview.CardTrue > 0 {
		dmg.AddDetail("Cards: +%d True", preview.CardTrue)
	}
	if character.WeaponStrikeCount(def) > 1 {
		dmg.Add("Damage shown per strike")
	}
	if preview.True > 0 {
		dmg.AddDetail("Normal Damage: %d", preview.Normal)
	}
	addDamageTotal(&dmg, "Total Damage", preview.Total, preview.True)
	totalCrit := def.CritChance
	if char != nil {
		totalCrit = cs.CalculateWeaponCritChance(item, char)
	}
	if totalCrit > 0 {
		dmg.Add("Critical Damage: %d", preview.CriticalTotal)
		if preview.True > 0 || preview.OutgoingBuff > 0 {
			dmg.AddDetail("Critical hits double normal damage before party buffs; True damage is not doubled")
		}
	}

	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		if char != nil {
			baseCrit, luck, cardCrit, setCrit, gmWeapon, gmArms, ballistics := cs.WeaponCritBreakdown(item, char)
			rawCrit := baseCrit + luck + cardCrit + setCrit + gmWeapon + gmArms + ballistics
			parts := []string{fmt.Sprintf("Base: %d%%", baseCrit), fmt.Sprintf("Luck: +%d%%", luck)}
			if cardCrit > 0 {
				parts = append(parts, fmt.Sprintf("Cards: +%d%%", cardCrit))
			}
			if setCrit > 0 {
				parts = append(parts, fmt.Sprintf("Set: +%d%%", setCrit))
			}
			if ballistics > 0 {
				parts = append(parts, fmt.Sprintf("Ballistics: +%d%%", ballistics))
			}
			if gmWeapon > 0 {
				parts = append(parts, fmt.Sprintf("%s Mastery - Grandmaster: +%d%%", config.TitleWords(def.Category), gmWeapon))
			}
			if gmArms > 0 {
				parts = append(parts, fmt.Sprintf("Arms Master - Grandmaster: +%d%%", gmArms))
			}
			crit.AddDetail("%s", strings.Join(parts, " - "))
			if rawCrit != totalCrit {
				crit.AddDetail("Capped at %d%%", totalCrit)
			}
		}
		crit.Add("Chance: %d%%", totalCrit)
	}

	effects := ttSection{Title: "EFFECTS"}
	effects.Add("%s", damageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
	// Config-computable specials minus the lines this template renders itself.
	for _, ln := range def.CoreEffectLines() {
		effects.Add("%s", ln)
	}

	rules := ttSection{Title: "RULES"}
	armorInteractionRules(&rules, def.DamageType, def.Physics != nil, preview.True > 0)
	if def.AoeRadiusTiles > 0 {
		rules.AddDetail("%s", character.WeaponSplashCritRule)
	}
	if skill, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
		if tier, _ := masteryTier(char, skill); tier >= int(character.MasteryGrandMaster) {
			rules.AddDetail("Grandmaster: this strike ignores Perfect Dodge")
		}
	}

	return renderTooltip(item.Name, subtitle, []ttSection{attack, dmg, crit, effects, rules}, full)
}

// ----------------------------------------------------------------- armor ----

func buildArmorTooltipUnified(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	subtitle := item.DisplayKind()
	if ok && def != nil && def.Rarity != "" {
		subtitle += " - " + config.TitleWords(def.Rarity)
	}

	defense := ttSection{Title: "DEFENSE"}
	totalAC := 0
	if ok && def != nil && (def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0) {
		totalAC = def.ArmorClassBase
		if char != nil {
			totalAC = cs.CalculateArmorClassContribution(item, char)
		}
		defense.AddDetail("Base Armor Class: %d", def.ArmorClassBase)
		if enduranceDiv, scalingOK := armorEnduranceScalingDivisor(item); scalingOK && char != nil {
			statContribDetail(&defense, "Endurance", char.GetEffectiveEndurance(), enduranceDiv)
		}
		if div, ok := armorEnduranceScalingDivisor(item); char == nil && ok {
			defense.AddDetail("Scales with Endurance / %d", div)
		}
		if cat, catOK := armorMasterySkill(item); catOK && char != nil {
			if tier, tierName := masteryTier(char, cat); tier > 0 {
				defense.AddDetail("%s Mastery - %s: +%d", cat.String(), tierName, tier*character.MasteryArmorACPerLevel)
			}
		}
		defense.Add("Item Armor Class: %d", totalAC)
	}

	effects := ttSection{Title: "EFFECTS"}
	if ok && def != nil {
		addItemEffects(&effects, def, item, char)
	}

	rules := ttSection{Title: "RULES"}
	if ok && def != nil && (def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0) {
		rules.AddDetail("Typed true damage and damage over time bypass Armor Class")
	}
	if cat, catOK := armorMasterySkill(item); catOK {
		if line := getArmorRequirementLine(item, char); line != "" {
			rules.Add("%s", line) // equip requirement is per-item state, not a formula
		}
		if tier, _ := masteryTier(char, cat); tier >= int(character.MasteryGrandMaster) {
			rules.AddDetail("Grandmaster: +%d%% Perfect Dodge while worn", character.ArmorGMDodgeBonus)
		}
	}

	return renderTooltip(item.Name, subtitle, []ttSection{defense, effects, rules}, full)
}

// armorMasterySkill maps an armor piece to its mastery skill (leather/chain/
// plate via category; shields via the off-hand slot).
func armorMasterySkill(item items.Item) (character.SkillType, bool) {
	cat := strings.ToLower(item.ArmorCategory)
	if cat != "" {
		if st, ok := character.ArmorSkillForCategory(cat); ok {
			return st, true
		}
	}
	if slotCode, ok := item.Attributes["equip_slot"]; ok && items.EquipSlot(slotCode) == items.SlotOffHand {
		return character.SkillShield, true
	}
	return 0, false
}

// ----------------------------------------------------------------- spells ---

func buildSpellTooltipUnified(def spells.SpellDefinition, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	if char == nil {
		cs = nil
	}
	subtitle := fmt.Sprintf("%s Magic", spellSchoolsLabel(def))

	casting := ttSection{Title: "CASTING"}
	addCastingCost(&casting, def.SpellPointsCost, char, cs)
	addSpellCooldown(&casting, def.ID, char, cs)
	if def.IsProjectile {
		if rng, okRng := cs.CalculateSpellRangeTiles(def.ID); okRng {
			casting.Add("Range: %.0f tiles", rng)
		}
		if phys, err := config.GlobalConfig.GetSpellConfig(string(def.ID)); err == nil && phys.SpeedTiles > 0 {
			casting.AddDetail("Projectile Speed: %.0f tiles/s", phys.SpeedTiles)

		}
	}
	switch {
	case def.HealParty || def.IsBuff() || def.StatBonus > 0 || len(def.StatBonuses) > 0:
		casting.Add("Target: Entire Party")
	case def.TargetSelf:
		casting.Add("Target: Self")
	}

	// Mastery belongs to the school the CHARACTER casts this spell with (the very
	// skill the fight scores by); the DAMAGE TYPE below stays the spell's own.
	masterySchool := spellSchoolForChar(char, def)
	tier, tierName := spellMasteryTier(char, def)
	mastery := 0
	formula := def.DamageFormula()
	breakdown := character.SpellDamageBreakdown(def, char)
	spellParts := damagecalc.Parts{}

	// addStrongMagicDetail is the one ACTIVE Strong Magic line for every damage
	// section (projectile, zone tick, nova). It quotes the same predicate the
	// packet builder uses (strongMagicPct), so the label and the actual boost
	// can never disagree.
	addStrongMagicDetail := func(dmg *ttSection) {
		if pct := strongMagicPct(char, def); pct > 0 {
			_, tierName := masteryTier(char, character.SkillStrongMagic)
			dmg.AddDetail("Strong Magic - %s: +%d%% damage", tierName, pct)
		}
	}

	dmg := ttSection{Title: "DAMAGE"}
	totalCrit := 0
	if formula.Kind == spells.DamageProjectile {
		spellParts = cs.spellDamageParts(def.ID, char, breakdown.Total)
		mult, base := formula.CostMultiplier, breakdown.Base
		if mult > 1 {
			dmg.AddDetail("Base (%d SP x %d x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, mult, base)
		} else if len(formula.MasteryLadder) == 4 {
			dmg.AddDetail("Base: %d", base)
		} else {
			dmg.AddDetail("Base (%d SP x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, base)
		}
		statBreakdownDetails(&dmg, breakdown, char)
		mastery = breakdown.Mastery
		if mastery > 0 {
			if spellParts.True > 0 {
				// Mastery is the CASTER's school; the true damage it converts to is
				// typed by the SPELL's own element (spellDamageParts), so the two
				// words differ for a dual-school page.
				dmg.AddDetail("%s Mastery - %s: +%d %s True Damage",
					formatSchoolName(masterySchool), tierName, mastery, formatSchoolName(def.School))
			} else {
				dmg.AddDetail("%s Mastery - %s: +%d Damage", formatSchoolName(masterySchool), tierName, mastery)
			}
		}
		if pierce := cs.spellResistPierce(char, string(def.ID)); pierce > 0 {
			dmg.AddDetail("Current Resistance Pierce: %d%%", pierce)
		}
		addStrongMagicDetail(&dmg)
		// Active party buffs add a flat bonus after crit doubling; Heroism is
		// physical-only, so spell schools get only all-damage buffs like Hour of Power.
		totalParts, outBonus := cs.spellPartsWithOutgoingBuff(spellParts, def.School)
		if outBonus > 0 {
			dmg.AddDetail("Active party buff: +%d", outBonus)
		}
		// Totals come from the same source and outgoing-buff stages as combat.
		addDamageTotal(&dmg, "Total Damage", totalParts.Total(), totalParts.True)
		totalCrit = cs.totalCriticalChance(0, char)
		if totalCrit > 0 {
			critParts := spellCriticalParts(spellParts)
			critParts, _ = cs.spellPartsWithOutgoingBuff(critParts, def.School)
			dmg.Add("Critical Damage: %d", critParts.Total())
		}
	}
	// Party/map nova (Inferno): explicit mastery scaling, all normal damage.
	if formula.Kind == spells.DamageNova {
		// The card quotes the packet the nova actually fires (tryCastInferno
		// routes it through spellDamageParts too).
		novaParts := cs.spellDamageParts(def.ID, char, breakdown.Total)
		novaParts, outBonus := cs.spellPartsWithOutgoingBuff(novaParts, def.School)
		dmg.AddDetail("Base: %d", breakdown.Base)
		if breakdown.Mastery > 0 {
			dmg.AddDetail("%s Mastery - %s: +%d", formatSchoolName(masterySchool), tierName, breakdown.Mastery)
		}
		if pierce := cs.spellResistPierce(char, string(def.ID)); pierce > 0 {
			dmg.AddDetail("Current Resistance Pierce: %d%% (enemies only)", pierce)
		}
		addStrongMagicDetail(&dmg)
		if outBonus > 0 {
			dmg.AddDetail("Active party buff: +%d (enemies only)", outBonus)
		}
		dmg.Add("Damage: %d", novaParts.Total())
		for _, line := range spellAreaLines(def) {
			dmg.Add("%s", line)
		}
	}

	heal := ttSection{Title: "HEALING"}
	if def.HealAmount > 0 {
		healing := character.SpellHealingBreakdown(def, char)
		mastery = healing.Mastery
		heal.AddDetail("Base: %d", healing.Base)
		statBreakdownDetails(&heal, healing.Breakdown, char)
		if mastery > 0 {
			heal.AddDetail("%s Mastery - %s: +%d", formatSchoolName(masterySchool), tierName, mastery)
		}
		if char != nil && char.HasSkill(character.SkillNaturalHealer) {
			heal.AddDetail("Natural Healer: +%d%%", healing.HealerPercent)
		}
		heal.Add("Total Healing: %d", healing.Total)
	}

	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		if char != nil {
			luck, cardCrit, setCrit := cs.CriticalChanceBreakdown(char)
			parts := []string{fmt.Sprintf("Luck: +%d%%", luck)}
			if cardCrit > 0 {
				parts = append(parts, fmt.Sprintf("Cards: +%d%%", cardCrit))
			}
			if setCrit > 0 {
				parts = append(parts, fmt.Sprintf("Set: +%d%%", setCrit))
			}
			crit.AddDetail("%s", strings.Join(parts, " - "))
		}
		crit.Add("Chance: %d%%", totalCrit)
	}

	zone := ttSection{Title: "ZONE"}
	if formula.Kind == spells.DamageZone {
		for _, line := range spellAreaLines(def) {
			zone.Add("%s", line)
		}
	}
	if formula.Kind == spells.DamageZone {
		// Tick damage uses the cast snapshot plus the same live outgoing buff
		// damagePersistentDamageZoneOnce reads on every tick.
		ladder := len(def.DamageByMastery) == 4
		if ladder && char != nil {
			// An authored ladder IS the payload: no Intellect, no per-tier bonus and
			// no Grandmaster true-damage split (Inferno's rule, same reason).
			dmg.AddDetail("Base (%s): %d", tierName, breakdown.Base)
		} else {
			dmg.AddDetail("Base: %d", breakdown.Base)
			statBreakdownDetails(&dmg, breakdown, char)
		}
		tickTotal := breakdown.Total
		mastery = breakdown.Mastery
		tickParts := cs.spellDamageParts(def.ID, char, tickTotal)
		if mastery > 0 && !ladder {
			if tickParts.True > 0 {
				// Mastery is the CASTER's school; the true damage it converts to is
				// typed by the SPELL's own element (spellDamageParts), so the two
				// words differ for a dual-school page.
				dmg.AddDetail("%s Mastery - %s: +%d %s True Damage",
					formatSchoolName(masterySchool), tierName, mastery, formatSchoolName(def.School))
			} else {
				dmg.AddDetail("%s Mastery - %s: +%d Damage", formatSchoolName(masterySchool), tierName, mastery)
			}
		}
		if pierce := cs.spellResistPierce(char, string(def.ID)); pierce > 0 {
			dmg.AddDetail("Current Resistance Pierce: %d%%", pierce)
		}
		addStrongMagicDetail(&dmg)
		tickParts, outBonus := cs.spellPartsWithOutgoingBuff(tickParts, def.School)
		if outBonus > 0 {
			dmg.AddDetail("Active party buff: +%d", outBonus)
		}
		// Same packet the zone ticks with (combat_zones builds TickDamage from
		// spellDamageParts) - the card can never understate a boosted tick.
		addDamageTotal(&dmg, "Total per tick", tickParts.Total(), tickParts.True)
		dmg.Title = "DAMAGE PER TICK"
	}

	effects := spellCurrentEffects(def, char)
	// Duration decomposed: base -> mastery % -> current.
	if def.Duration > 0 {
		duration := character.SpellDurationBreakdown(def, char)
		if char != nil {
			effects.AddDetail("Base Duration: %ds", duration.Base)
		}
		if tier > 0 {
			effects.AddDetail("%s Mastery - %s: +%d%%", formatSchoolName(masterySchool), tierName, duration.MasteryPct)
		}
		effects.Add("%s Duration: %ds", tooltipValuePrefix(char), duration.Seconds)
	}

	rules := ttSection{Title: "RULES"}
	for _, rule := range character.SpellRules(def) {
		switch rule.Kind {
		case character.SpellRuleMasteryPolicy:
			continue
		case character.SpellRuleDodge:
			if spellParts.True > 0 {
				rules.AddDetail("Perfect Dodge avoids normal damage; typed true damage still lands")
			} else {
				rules.AddDetail("Can be evaded by Perfect Dodge")
			}
		default:
			rules.AddDetail("%s", rule.Text)
		}
	}

	return renderTooltip(def.Name, subtitle, []ttSection{casting, dmg, heal, crit, zone, effects, rules}, full)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ----------------------------------------------------------------- traps ----

func buildTrapTooltipUnified(key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	subtitle := fmt.Sprintf("Trap - Level %d", def.Level)

	placement := ttSection{Title: "PLACEMENT"}
	addCastingCost(&placement, def.SPCost, char, cs)
	addSpellCooldown(&placement, spells.SpellID(key), char, cs)
	placement.Add("Range: %d tiles", TrapPlaceRangeTiles)
	placement.AddDetail("Armed Lifetime: %ds", def.LifetimeSeconds)

	tier, tierName := masteryTier(char, character.SkillTrapper)

	dmg := ttSection{Title: "DAMAGE"}
	if def.DamageBase > 0 {
		dmg.AddDetail("Base: %d", def.DamageBase)
		if char != nil {
			intAcc := char.GetEffectiveIntellect() + char.GetEffectiveAccuracy()
			dmg.AddDetail("Intellect + Accuracy ((%d + %d) / %d): +%d",
				char.GetEffectiveIntellect(), char.GetEffectiveAccuracy(),
				character.TrapStatScalingDivisor, intAcc/character.TrapStatScalingDivisor)
		} else {
			dmg.AddDetail("Scales with (Intellect + Accuracy) / %d", character.TrapStatScalingDivisor)
		}
		if tier > 0 {
			dmg.AddDetail("Trapper - %s: +%d", tierName, tier*character.TrapperDamagePerTier)
		}
		dmg.Add("Total Damage: %d", trapDamage(def, char))
	}

	effect := ttSection{Title: "CONTROL"}
	if def.StunTurns > 0 {
		t, s := trapControlDuration(def.StunTurns, def.StunSeconds, char)
		effect.AddDetail("Base Stun: %ds RT / %d turns TB", def.StunSeconds, def.StunTurns)
		if tier > 0 {
			effect.AddDetail("Trapper - %s: +%ds RT / +%d turns TB", tierName, s-def.StunSeconds, t-def.StunTurns)
		}
		effect.Add("Total Stun: %ds RT / %d turns TB", s, t)
	}
	if def.RootTurns > 0 {
		t, s := trapControlDuration(def.RootTurns, def.RootSeconds, char)
		effect.AddDetail("Base Root: %ds RT / %d turns TB", def.RootSeconds, def.RootTurns)
		if tier > 0 {
			effect.AddDetail("Trapper - %s: +%ds RT / +%d turns TB", tierName, s-def.RootSeconds, t-def.RootTurns)
		}
		effect.Add("Total Root: %ds RT / %d turns TB", s, t)
	}

	effects := ttSection{Title: "EFFECTS"}
	if def.DamageBase > 0 {
		effects.Add("%s", damageTypeAoELine(def.Element, def.AoeRadiusTiles))
	}

	rules := ttSection{Title: "RULES"}
	if char != nil && char.Level < def.Level {
		rules.Add("LOCKED: requires level %d", def.Level) // must be visible compact
	}
	if def.RootTurns > 0 {
		rules.Add("Prevents movement but not attacks") // the root's key caveat
	}
	if def.DamageBase > 0 {
		armorInteractionRules(&rules, def.Element, false, false)
	}
	rules.AddDetail("Triggers once, then disappears")
	rules.AddDetail("Maximum %d armed traps per character on the map", MaxTrapsPerOwner)

	return renderTooltip(def.Name, subtitle, []ttSection{placement, dmg, effect, effects, rules}, full)
}

// -------------------------------------------------- misc item categories ----

func buildSimpleItemTooltipUnified(item items.Item, full bool, bearers ...*character.MMCharacter) string {
	var bearer *character.MMCharacter
	if len(bearers) > 0 {
		bearer = bearers[0]
	}
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	subtitle := item.DisplayKind()
	if ok && def != nil && def.Rarity != "" {
		subtitle += " - " + config.TitleWords(def.Rarity)
	}
	effect := ttSection{Title: "EFFECTS"}
	use := ttSection{Title: "USAGE"}
	if ok && def != nil {
		for _, ln := range def.EffectLinesWithoutRecovery() {
			effect.Add("%s", ln)
		}
		character.AddConsumableDetails(&effect, def, bearer)
		character.AddConsumableUsage(&use, def)
		for _, ln := range def.TooltipUsageLines() {
			use.Add("%s", ln)
		}
	}
	return renderTooltip(item.Name, subtitle, []ttSection{effect, use}, full)
}
