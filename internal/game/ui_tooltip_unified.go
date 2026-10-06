package game

import (
	"fmt"
	"strings"
	uitext "ugataima/assets/text"

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
//	  results, then base -> stat -> mastery calculation
//	...
// Empty sections and inapplicable lines are skipped. Exceptions stay with
// their mechanic; RT and TB values appear together.

// Shops and editor catalogs use these same builders with no character.
type ttSection = character.CardSection

// renderTooltipRows assembles the card without discarding semantic roles.
func renderTooltipRows(name, subtitle string, sections []ttSection, full bool) character.CardRows {
	var out character.CardRows
	out.Add(character.CardRowTitle, name)
	if subtitle != "" {
		out.Add(character.CardRowCategory, subtitle)
	}
	if body := character.RenderCardRows(sections, full); len(body) > 0 {
		out.Add(character.CardRowSpacer, "")
		out = append(out, body...)
	}
	if !full && character.SectionsHaveDetail(sections) {
		out.Add(character.CardRowSpacer, "")
		out.Add(character.CardRowHint, shiftDetailHint)
	}
	return out
}

const shiftDetailHint = "[Shift] full breakdown"

// hintRowsLast keeps the expansion hint after appended content by its role.
func hintRowsLast(rows character.CardRows) character.CardRows {
	for i, row := range rows {
		if row.Kind != character.CardRowHint {
			continue
		}
		start := i
		if i > 0 && rows[i-1].Kind == character.CardRowSpacer {
			start--
		}
		hint := append(character.CardRows(nil), rows[start:i+1]...)
		out := append(character.CardRows(nil), rows[:start]...)
		out = append(out, rows[i+1:]...)
		return append(out, hint...)
	}
	return rows
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
	return fmt.Sprintf("%.2fs", character.CardSeconds(float64(frames)/float64(tps)))
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
	return buildWeaponTooltipUnifiedRows(item, char, cs, full).String()
}

func buildWeaponTooltipUnifiedRows(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) character.CardRows {
	def := lookupWeaponConfigByName(item.Name)
	if def == nil {
		return character.CardRows{{Text: item.Name, Kind: character.CardRowTitle}}
	}
	subtitle := config.TitleWords(strings.ReplaceAll(def.Category, "_", " "))
	if def.Rarity != "" {
		subtitle += " - " + config.TitleWords(def.Rarity)
	}

	attack := ttSection{Title: "ATTACK"}
	addWeaponCooldown(&attack, char, cs, def)
	rangeTiles, speedTiles := character.EffectiveWeaponFlight(def, char)
	// Melee reach is the swing-arc line's job ("reaches 1 tile").
	if def.IsRanged() {
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
	if breakdown.FlowingStaff > 0 {
		dmg.AddDetail("Flowing Staff: +%d (%d charges)", breakdown.FlowingStaff, char.FlowingStaffCharges())
	}
	preview := cs.calculateWeaponDamagePreview(item, char)
	addDamageTotal(&dmg, "Total Damage", preview.Total, preview.True)
	dmg.Add("%s", damageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
	if character.WeaponStrikeCount(def) > 1 {
		dmg.Add("Strikes per attack: %d", character.WeaponStrikeCount(def))
		dmg.Add("Damage shown per strike")
	}
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
	isRanged := def.IsRanged()
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
	if preview.True > 0 {
		dmg.AddDetail("Normal Damage: %d", preview.Normal)
	}
	armorInteractionRules(&dmg, def.DamageType, def.Physics != nil, preview.True > 0)
	if skill, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
		if tier, _ := masteryTier(char, skill); tier >= int(character.MasteryGrandMaster) {
			dmg.AddDetail("Grandmaster: this strike ignores Perfect Dodge")
		}
	}
	totalCrit := def.CritChance
	if char != nil {
		totalCrit = cs.CalculateWeaponCritChance(item, char)
	}
	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		crit.Add("Chance: %d%%", totalCrit)
		crit.Add("Critical Damage: %d", preview.CriticalTotal)
		if char.HasChargedStaffAttack(def) {
			crit.AddDetail("Flowing Staff: guaranteed critical hit")
		} else if char != nil {
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
				skill := config.TitleWords(def.Category)
				if st, ok := character.WeaponSkillForCategory(def.Category); ok {
					skill = st.String()
				}
				parts = append(parts, fmt.Sprintf("%s Mastery - Grandmaster: +%d%%", skill, gmWeapon))
			}
			if gmArms > 0 {
				parts = append(parts, fmt.Sprintf("Arms Master - Grandmaster: +%d%%", gmArms))
			}
			crit.AddDetail("%s", strings.Join(parts, " - "))
			if rawCrit != totalCrit {
				crit.AddDetail("Capped at %d%%", totalCrit)
			}
		}
		if preview.True > 0 || preview.OutgoingBuff > 0 {
			crit.AddDetail("Critical hits double normal damage before party buffs; True damage is not doubled")
		}
	}
	if def.AoeRadiusTiles > 0 {
		crit.AddDetail("%s", character.WeaponSplashCritRule)
	}

	effects := ttSection{Title: "EFFECTS"}
	// Config-computable specials minus the lines this template renders itself.
	for _, ln := range def.CoreEffectLines() {
		effects.Add("%s", ln)
	}

	return renderTooltipRows(item.Name, subtitle, []ttSection{dmg, crit, attack, effects}, full)
}

// ----------------------------------------------------------------- armor ----

func buildArmorTooltipUnified(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	return buildArmorTooltipUnifiedRows(item, char, cs, full).String()
}

func buildArmorTooltipUnifiedRows(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) character.CardRows {
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

	requirements := ttSection{Title: "REQUIREMENTS"}
	if ok && def != nil && (def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0) {
		defense.AddDetail("Typed true damage and damage over time bypass Armor Class")
	}
	if cat, catOK := armorMasterySkill(item); catOK {
		if line := getArmorRequirementLine(item, char); line != "" {
			requirements.Add("%s", line)
		}
		if tier, _ := masteryTier(char, cat); tier >= int(character.MasteryGrandMaster) {
			defense.AddDetail("Grandmaster: %s", character.ArmorGMDodgeRule())
		}
	}

	return renderTooltipRows(item.Name, subtitle, []ttSection{defense, effects, requirements}, full)
}

// armorMasterySkill maps an armor piece to its mastery skill by its category
// (leather/chain/plate/shield). Combat and the tooltip both read it.
func armorMasterySkill(item items.Item) (character.SkillType, bool) {
	return character.ArmorSkillForCategory(strings.ToLower(item.ArmorCategory))
}

// ----------------------------------------------------------------- spells ---

// spellDamageTail is how one kind of spell damage words the rows every damage
// section shares: a hit, a zone's tick and a nova around the party differ only
// here.
type spellDamageTail struct {
	label       string // the result row: "Total Damage", "Total per tick", "Damage"
	splitTrue   bool   // the result shows its Normal + True split
	enemiesOnly bool   // the party is struck too: pierce and buffs reach enemies only
	masteryRow  bool   // the mastery bonus gets a row (an authored ladder is the whole payload)
	bareMastery bool   // a nova's mastery adds raw points: "+N", not "+N Damage"
}

func buildSpellTooltipUnified(def spells.SpellDefinition, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	return buildSpellTooltipUnifiedRows(def, char, cs, full).String()
}

func buildSpellTooltipUnifiedRows(def spells.SpellDefinition, char *character.MMCharacter, cs *CombatSystem, full bool) character.CardRows {
	if char == nil {
		cs = nil
	}
	subtitle := fmt.Sprintf("%s Magic", spellSchoolsLabel(def))

	casting := ttSection{Title: "CASTING"}
	cost := addCastingCost(&casting, def.SpellPointsCost, char, cs)
	if burn := character.StrongMagicHPCost(cost, strongMagicPct(char, def)); burn > 0 {
		casting.Add("Strong Magic: also burns %d HP (never the last)", burn)
	}
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

	// A catalog card (no caster) quotes what mastery alone adds at every tier,
	// from the same formula the cast evaluates.
	hero := char != nil
	catalog := character.SpellFormulaTiers(formula.Formula)
	catalogLadder := func(tiers [4]stats.Breakdown, pick func(stats.Breakdown) int) [4]int {
		return tierValues(func(t int) int { return pick(tiers[t]) })
	}
	baseOf := func(b stats.Breakdown) int { return b.Base }
	totalOf := func(b stats.Breakdown) int { return b.Total }
	addCatalogMastery := func(sec *ttSection, tiers [4]stats.Breakdown, unit string) {
		if values := catalogLadder(tiers, func(b stats.Breakdown) int { return b.Mastery }); !hero && values != ([4]int{}) {
			sec.AddDetail("%s Mastery: +%s%s", formatSchoolName(masterySchool), tierLadder(values), unit)
		}
	}

	// addDamageTail is what every damage section shares after its base rows:
	// the mastery bonus, pierce, Strong Magic, the active party buff and the
	// result, all quoted from the packet the cast fires (parts, before the
	// buff). It returns that packet with the buff.
	addDamageTail := func(dmg *ttSection, k spellDamageTail, parts damagecalc.Parts, masteryBonus int) damagecalc.Parts {
		if k.masteryRow && masteryBonus > 0 {
			switch {
			case k.bareMastery:
				dmg.AddDetail("%s Mastery - %s: +%d", formatSchoolName(masterySchool), tierName, masteryBonus)
			case parts.True > 0:
				// Mastery is the CASTER's school; the true damage it converts to is
				// typed by the SPELL's own element (spellDamageParts), so the two
				// words differ for a dual-school page.
				dmg.AddDetail("%s Mastery - %s: +%d %s True Damage",
					formatSchoolName(masterySchool), tierName, masteryBonus, formatSchoolName(def.School))
			default:
				dmg.AddDetail("%s Mastery - %s: +%d Damage", formatSchoolName(masterySchool), tierName, masteryBonus)
			}
		}
		reach := ""
		if k.enemiesOnly {
			reach = " (enemies only)"
		}
		if pierce := cs.spellResistPierce(char, string(def.ID)); pierce > 0 {
			dmg.AddDetail("Current Resistance Pierce: %d%%%s", pierce, reach)
		}
		addStrongMagicDetail(dmg)
		// Active party buffs add a flat bonus after crit doubling; Heroism is
		// physical-only, so spell schools get only all-damage buffs like Hour of Power.
		total, outBonus := cs.spellPartsWithOutgoingBuff(parts, def.School)
		if outBonus > 0 {
			dmg.AddDetail("Active party buff: +%d%s", outBonus, reach)
		}
		switch {
		case !hero:
			dmg.Add("%s: %s", k.label, tierLadder(catalogLadder(catalog, totalOf)))
		case k.splitTrue:
			addDamageTotal(dmg, k.label, total.Total(), total.True)
		default:
			dmg.Add("%s: %d", k.label, total.Total())
		}
		return total
	}

	dmg := ttSection{Title: "DAMAGE"}
	totalCrit, criticalDamage := 0, 0
	if formula.Kind == spells.DamageProjectile {
		spellParts = cs.spellDamageParts(def.ID, char, breakdown.Total)
		mult, base := formula.CostMultiplier, breakdown.Base
		if mult > 1 {
			dmg.AddDetail("Base (%d SP x %d x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, mult, base)
		} else if len(formula.MasteryLadder) == 4 {
			dmg.AddDetail("Base: %s", tierValueText(hero, tier, catalogLadder(catalog, baseOf)))
		} else {
			dmg.AddDetail("Base (%d SP x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, base)
		}
		statBreakdownDetails(&dmg, breakdown, char)
		addCatalogMastery(&dmg, catalog, " Damage")
		mastery = breakdown.Mastery
		addDamageTail(&dmg, spellDamageTail{label: "Total Damage", splitTrue: true, masteryRow: true}, spellParts, mastery)
		totalCrit = cs.totalCriticalChance(0, char)
		if totalCrit > 0 {
			critParts := spellCriticalParts(spellParts)
			critParts, _ = cs.spellPartsWithOutgoingBuff(critParts, def.School)
			criticalDamage = critParts.Total()
		}
	}
	// Party/map nova (Inferno): explicit mastery scaling, all normal damage.
	if formula.Kind == spells.DamageNova {
		// The card quotes the packet the nova actually fires (tryCastPartyNova
		// routes it through spellDamageParts too).
		novaParts := cs.spellDamageParts(def.ID, char, breakdown.Total)
		if len(def.DamageByMastery) == 4 && char != nil {
			dmg.AddDetail("Base (%s): %d", tierName, breakdown.Base)
		} else {
			dmg.AddDetail("Base: %s", tierValueText(hero, tier, catalogLadder(catalog, baseOf)))
		}
		addCatalogMastery(&dmg, catalog, "")
		// The party is struck too: pierce and buffs reach the enemies only.
		addDamageTail(&dmg, spellDamageTail{label: "Damage", enemiesOnly: true, masteryRow: true, bareMastery: true}, novaParts, breakdown.Mastery)
		for _, line := range spellAreaLines(def) {
			dmg.Add("%s", line)
		}
	}

	heal := ttSection{Title: "HEALING"}
	if def.HealAmount > 0 {
		healing := character.SpellHealingBreakdown(def, char)
		mastery = healing.Mastery
		healingTiers := character.SpellFormulaTiers(def.HealingFormula())
		heal.AddDetail("Base: %d", healing.Base)
		statBreakdownDetails(&heal, healing.Breakdown, char)
		addCatalogMastery(&heal, healingTiers, "")
		if mastery > 0 {
			heal.AddDetail("%s Mastery - %s: +%d", formatSchoolName(masterySchool), tierName, mastery)
		}
		if char != nil && char.HasSkill(character.SkillNaturalHealer) {
			heal.AddDetail("Natural Healer: +%d%%", healing.HealerPercent)
		}
		if hero {
			heal.Add("Total Healing: %d", healing.Total)
		} else {
			heal.Add("Total Healing: %s", tierLadder(catalogLadder(healingTiers, totalOf)))
		}
	}

	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		crit.Add("Chance: %d%%", totalCrit)
		crit.Add("Critical Damage: %d", criticalDamage)
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
			dmg.AddDetail("Base: %s", tierValueText(hero, tier, catalogLadder(catalog, baseOf)))
			statBreakdownDetails(&dmg, breakdown, char)
			addCatalogMastery(&dmg, catalog, " Damage")
		}
		mastery = breakdown.Mastery
		// Same packet the zone ticks with (combat_zones builds TickDamage from
		// spellDamageParts) - the card can never understate a boosted tick.
		tickParts := cs.spellDamageParts(def.ID, char, breakdown.Total)
		addDamageTail(&dmg, spellDamageTail{label: "Total per tick", splitTrue: true, masteryRow: !ladder}, tickParts, mastery)
		dmg.Title = "DAMAGE PER TICK"
	}

	if def.IsProjectile && !def.DealsNoDamage {
		dmg.Add("%s", damageTypeAoELine(def.School, def.AoeRadiusTiles))
	}
	// Party projectiles add the card collection's disintegrate bonus at launch.
	shown, cardDisintegrate := def, 0
	if cs != nil && cs.game != nil && def.DisintegrateChance > 0 && def.IsProjectile {
		cardDisintegrate = cs.game.cardDisintegratePct()
		shown.DisintegrateChance = cs.game.partyDisintegrateChance(def.DisintegrateChance)
	}
	effects := spellCurrentEffects(shown, char, false)
	if cardDisintegrate > 0 {
		effects.AddDetail("Cards: +%d%% disintegrate chance", cardDisintegrate)
	}
	durationSection := ttSection{Title: "DURATION"}
	if def.Duration > 0 {
		duration := character.SpellDurationBreakdown(def, char)
		if char != nil {
			durationSection.AddDetail("Base Duration: %ds", duration.Base)
		}
		if tier > 0 {
			durationSection.AddDetail("%s Mastery - %s: +%d%%", formatSchoolName(masterySchool), tierName, duration.MasteryPct)
		}
		// A resolved TB round spends TurnBasedTurnSeconds of every timer.
		seconds := tierValues(func(t int) int { return character.SpellDurationAtTier(def, t).Seconds })
		rounds := tierValues(func(t int) int { return tbRoundsForSeconds(seconds[t]) })
		turns := tierCountText(rounds, "round", "rounds")
		if hero {
			turns = pluralizeCount(config.TierValue(rounds, tier), "round", "rounds")
		}
		durationSection.Add("%s: %s", tierLabel(hero, "Duration", seconds), cardDurationText(tierValueText(hero, tier, seconds), turns))
	}

	for _, rule := range character.SpellRules(def) {
		switch rule.Kind {
		case character.SpellRuleMasteryPolicy:
			// A live card shows the caster's active contribution instead.
			if !hero {
				dmg.AddDetail("%s", rule.Text)
			}
		case character.SpellRuleDodge:
			section := &dmg
			if formula.Kind == spells.DamageNone {
				section = &effects
			}
			if spellParts.True > 0 {
				section.AddDetail("Perfect Dodge avoids normal damage; typed true damage still lands")
			} else {
				section.AddDetail("Can be evaded by Perfect Dodge")
			}
		case character.SpellRuleDamage:
			dmg.AddDetail("%s", rule.Text)
		case character.SpellRuleCritical:
			crit.AddDetail("%s", rule.Text)
		case character.SpellRuleZone:
			zone.AddDetail("%s", rule.Text)
		case character.SpellRuleDuration:
			durationSection.AddDetail("%s", rule.Text)
		default:
			effects.AddDetail("%s", rule.Text)
		}
	}

	return renderTooltipRows(def.Name, subtitle, []ttSection{dmg, heal, crit, effects, durationSection, zone, casting}, full)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ----------------------------------------------------------------- traps ----

func buildTrapTooltipUnified(key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	return buildTrapTooltipUnifiedRows(key, def, char, cs, full).String()
}

func buildTrapTooltipUnifiedRows(key string, def *config.TrapDefinitionConfig, char *character.MMCharacter, cs *CombatSystem, full bool) character.CardRows {
	subtitle := fmt.Sprintf("Trap - Level %d", def.Level)

	placement := ttSection{Title: "PLACEMENT"}
	addCastingCost(&placement, def.SPCost, char, cs)
	addSpellCooldown(&placement, spells.SpellID(key), char, cs)
	placement.Add("Range: %d tiles", TrapPlaceRangeTiles)
	// A trap ages one round's worth of seconds per turn-based round.
	rounds := (def.LifetimeSeconds + character.TurnBasedTurnSeconds - 1) / character.TurnBasedTurnSeconds
	placement.AddDetail("Armed Lifetime: %ds RT / %s TB", def.LifetimeSeconds, pluralizeCount(rounds, "round", "rounds"))

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
		if char != nil {
			dmg.Add("Total Damage: %d", trapDamage(def, char))
		} else {
			// Catalog card: what the Trapper tier alone adds.
			dmg.AddDetail("Trapper: +%s", tierLadder(tierValues(func(t int) int { return t * character.TrapperDamagePerTier })))
			dmg.Add("Total Damage: %s", tierLadder(tierValues(func(t int) int { return trapDamageAtTier(def, t) })))
		}
		dmg.Add("%s", damageTypeAoELine(def.Element, def.AoeRadiusTiles))
	}

	effect := ttSection{Title: "CONTROL"}
	turns := func(n int) string { return pluralizeCount(n, "turn", "turns") }
	for _, c := range []struct {
		name             string
		turnsTB, seconds int
	}{{"Stun", def.StunTurns, def.StunSeconds}, {"Root", def.RootTurns, def.RootSeconds}} {
		if c.turnsTB <= 0 {
			continue
		}
		effect.AddDetail("Base %s: %ds RT / %s TB", c.name, c.seconds, turns(c.turnsTB))
		if char == nil {
			at := func(t int) (int, int) { return trapControlDurationAtTier(c.turnsTB, c.seconds, t) }
			secs := tierValues(func(t int) int { _, s := at(t); return s })
			tb := tierValues(func(t int) int { n, _ := at(t); return n })
			effect.Add("Total %s: %ss RT / %s TB", c.name, tierLadder(secs), tierCountText(tb, "turn", "turns"))
			continue
		}
		t, sec := trapControlDuration(c.turnsTB, c.seconds, char)
		if tier > 0 {
			effect.AddDetail("Trapper - %s: +%ds RT / +%s TB", tierName, sec-c.seconds, turns(t-c.turnsTB))
		}
		effect.Add("Total %s: %ds RT / %s TB", c.name, sec, turns(t))
	}
	if def.StunTurns > 0 {
		effect.AddDetail("%s", uitext.Text("spell.repeated_stuns_wear_off_diminishing_returns_then"))
	}

	requirements := ttSection{Title: "REQUIREMENTS"}
	if char != nil && char.Level < def.Level {
		requirements.Add("LOCKED: requires level %d", def.Level)
	}
	if def.RootTurns > 0 {
		effect.Add("Prevents movement but not attacks")
	}
	if def.DamageBase > 0 {
		armorInteractionRules(&dmg, def.Element, false, false)
	}
	placement.AddDetail("Triggers once, then disappears")
	placement.AddDetail("Maximum %d armed traps per character on the map", MaxTrapsPerOwner)

	return renderTooltipRows(def.Name, subtitle, []ttSection{dmg, effect, placement, requirements}, full)
}

// ------------------------------------------------------------- techniques ---

// buildTechniqueTooltipUnified is a Pilgrim technique card. With a hero it
// shows that hero's cost, real recovery and magnitude; without one (catalog,
// editor) the base values and every tier.
func buildTechniqueTooltipUnified(d *config.TechniqueDefinition, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	return buildTechniqueTooltipUnifiedRows(d, char, cs, full).String()
}

func buildTechniqueTooltipUnifiedRows(d *config.TechniqueDefinition, char *character.MMCharacter, cs *CombatSystem, full bool) character.CardRows {
	if char == nil {
		cs = nil
	}
	tier, _ := masteryTier(char, character.SkillTranslocation)
	casting := ttSection{Title: "CASTING"}
	addCastingCost(&casting, config.TierValue(d.SPCost, tier), char, cs)
	switch {
	case d.FreeStep:
		casting.Add("No action cost")
	case cs != nil && cs.game.techniqueCooldown(char, d) > 0:
		casting.Add("%s", cooldownLine(cs, cs.game.techniqueCooldown(char, d)))
	case d.CooldownSeconds > 0:
		casting.Add("%s", character.CooldownLine(d.CooldownSeconds))
	default:
		casting.Add("No RT cooldown - TB: 1 action")
	}
	if d.ReuseSeconds > 0 {
		if d.FreeStep {
			casting.Add("Reuse: %ds, shared by %s (TB: once per turn)", d.ReuseSeconds, strings.Join(techniqueStepNames(), " and "))
		} else {
			casting.Add("Reuse: %s", cardDurationText(fmt.Sprint(d.ReuseSeconds), pluralizeCount(tbRoundsForSeconds(d.ReuseSeconds), "round", "rounds")))
		}
	}
	effect := ttSection{Title: "EFFECTS"}
	tps := config.DefaultTPS
	if cs != nil {
		tps = cs.game.config.GetTPS()
	}
	if line := techniqueMagnitude(char, d, tps); line != "" {
		effect.Add("%s", line)
	}
	trigger := ttSection{Title: "TRIGGER"}
	if d.Automatic {
		trigger.Add("Autocast (switch on in the technique book): %s", d.Trigger)
	}
	requirements := ttSection{Title: "REQUIREMENTS"}
	if char != nil && char.Level < d.Level {
		requirements.Add("LOCKED: requires level %d", d.Level)
	}
	card := renderTooltipRows(d.Name, fmt.Sprintf("Technique - Level %d", d.Level), []ttSection{casting, effect, trigger, requirements}, full)
	if d.Description != "" {
		card.Add(character.CardRowSpacer, "")
		card.Add(character.CardRowDescription, d.Description)
	}
	return card
}

// techniqueStepNames lists the steps that share the spatial reuse lock.
func techniqueStepNames() []string {
	var names []string
	if config.GlobalTechniques != nil {
		for _, t := range config.GlobalTechniques.Techniques {
			if t.FreeStep {
				names = append(names, t.Name)
			}
		}
	}
	return names
}

// -------------------------------------------------- misc item categories ----

func buildSimpleItemTooltipWithParty(item items.Item, full bool, bearer *character.MMCharacter, party *character.Party, cs *CombatSystem, usage ...string) string {
	return buildSimpleItemTooltipWithPartyRows(item, full, bearer, party, cs, usage...).String()
}

func buildSimpleItemTooltipWithPartyRows(item items.Item, full bool, bearer *character.MMCharacter, party *character.Party, cs *CombatSystem, usage ...string) character.CardRows {
	def, itemKey, ok := config.GetItemDefinitionByName(item.Name)
	subtitle := item.DisplayKind()
	if ok && def != nil && def.Rarity != "" {
		subtitle += " - " + config.TitleWords(def.Rarity)
	}
	effect := ttSection{Title: "EFFECTS"}
	recovery := ttSection{Title: "RECOVERY"}
	dmg := ttSection{Title: "DAMAGE"}
	use := ttSection{Title: "USAGE"}
	if ok && def != nil {
		for _, ln := range def.EffectLinesWithoutRecovery() {
			effect.Add("%s", ln)
		}
		var hpBonus, spBonus character.PotionSupport
		if party != nil && bearer != nil {
			hpBonus = party.PotionSupport(bearer, def.HealBase, def.HealEnduranceDivisor, false)
			spBonus = party.PotionSupport(bearer, def.ManaBase, def.ManaPersonalityDivisor, true)
		}
		character.AddConsumableDetails(&recovery, def, bearer, hpBonus, spBonus)
		if f := def.Flask; f != nil {
			addFlaskSections(&dmg, &effect, &use, itemKey, f, bearer, cs)
		}
		character.AddConsumableUsage(&use, def)
		for _, ln := range def.TooltipUsageLines() {
			use.Add("%s", ln)
		}
	}
	if item.Type == items.ItemDevice && item.DeviceCooldownFrames > 0 {
		recovery.Add("%s", uitext.Text("item.device_cooldown", float64(item.DeviceCooldownFrames)/float64(config.GetTargetTPS()), deviceCooldownTurns(item)))
	}
	for _, ln := range usage {
		use.Add("%s", ln)
	}
	return renderTooltipRows(item.Name, subtitle, []ttSection{recovery, dmg, effect, use}, full)
}

// addFlaskSections states a flask the way a throw resolves it: with a bearer
// the damage breakdown and the real recovery (Speed, staff, Quickening); with
// none (catalog) the per-tier base plus the Intellect term.
func addFlaskSections(dmg, effect, use *ttSection, key string, f *config.FlaskDefinition, bearer *character.MMCharacter, cs *CombatSystem) {
	tier, tierName := masteryTier(bearer, character.SkillBombThrowing)
	perTier := func(values [4]int) string { return tierValueText(bearer != nil, tier, values) }
	if bearer != nil {
		dmg.AddDetail("Base (Bomb Throwing - %s): %d", tierName, config.TierValue(f.Damage, tier))
		intellect := bearer.GetEffectiveIntellect()
		dmg.AddDetail("Intellect (%d / %d): +%d", intellect, character.BombThrowingIntellectDivisor, intellect/character.BombThrowingIntellectDivisor)
		dmg.Add("Total Damage: %d to each victim", flaskDamage(bearer, f))
	} else {
		dmg.Add("Damage: %s + Intellect / %d to each victim", tierLadder(f.Damage), character.BombThrowingIntellectDivisor)
	}
	dmg.Add("%s", damageTypeAoELine(f.Element, float64(f.RadiusTiles)))
	effect.Add("Range: %d tiles", f.RangeTiles)
	if f.PoisonSeconds != [4]int{} {
		effect.Add("Poison: %s seconds", perTier(f.PoisonSeconds))
	}
	if f.BurnSeconds != [4]int{} {
		effect.Add("Burning: %s seconds", perTier(f.BurnSeconds))
	}
	use.Add("Requires Bomb Throwing. Uses own bag, then shared bag.")
	if cs != nil && bearer != nil {
		use.Add("%s", cooldownLine(cs, cs.SpellCooldownFrames(bearer, spells.SpellID(key))))
	} else {
		use.Add("%s", character.CooldownLine(f.CooldownSeconds))
	}
	use.Add("No critical hits.")
	use.Add("Equip as a quick action, then use F or SmartAttack.")
}
