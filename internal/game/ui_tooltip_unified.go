package game

import (
	"fmt"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// The unified tooltip template (user-designed): every card renders as
//
//	=== Name ===
//	Category · Rarity/Level
//	SECTION
//	  base → stat → mastery → total decomposition
//	...
//	RULES
//
// Empty sections and inapplicable lines are skipped; armor/resistance
// interaction is always spelled out; RT and TB values appear together.

// ttSection aliases the SHARED template engine (character/cardtemplate.go) —
// the map editor renders the same sections in their character-independent
// form, so the two views cannot diverge structurally.
type ttSection = character.CardSection

// renderTooltip assembles the final text, dropping empty sections.
func renderTooltip(name, subtitle string, sections []ttSection, full bool) string {
	out := []string{fmt.Sprintf("=== %s ===", name)}
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
		tps = 60
	}
	return fmt.Sprintf("%.1fs", float64(frames)/float64(tps))
}

// cooldownLine renders the labeled RT/TB cooldown line for a frame count, or ""
// when there is no cooldown. Shares the wording with the editor cards.
func cooldownLine(cs *CombatSystem, frames int) string {
	if cs == nil || cs.game == nil || frames <= 0 {
		return ""
	}
	tps := cs.game.config.GetTPS()
	if tps <= 0 {
		tps = 60
	}
	return character.CooldownLine(float64(frames) / float64(tps))
}

// spellCooldownWeaponLine names the equipped main-hand's spell-cooldown perk
// (e.g. Archmage Staff ×0.80), or "" when none — SpellCooldownFrames factors it.
func spellCooldownWeaponLine(char *character.MMCharacter) string {
	if char == nil {
		return ""
	}
	weapon, ok := char.Equipment[items.SlotMainHand]
	if !ok {
		return ""
	}
	if def, _, found := config.GetWeaponDefinitionByName(weapon.Name); found && def != nil && def.SpellCooldownMultiplier > 0 {
		return fmt.Sprintf("%s: ×%.2f spell cooldown", weapon.Name, def.SpellCooldownMultiplier)
	}
	return ""
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

// schoolMasteryTier is the magic-school analogue of masteryTier.
func schoolMasteryTier(char *character.MMCharacter, school string) (int, string) {
	if char == nil || school == "" {
		return 0, ""
	}
	ms := char.MagicSchools[character.MagicSchoolID(school)]
	if ms == nil {
		return 0, ""
	}
	return int(ms.Mastery), ms.Mastery.String()
}

// statContribDetail renders "Accuracy (30 / 3): +10" into the full-only tier —
// the stat VALUE and the divisor the formula actually uses. A zero
// contribution still names the scaling stat (what to raise).
func statContribDetail(sec *ttSection, statName string, statValue, divisor int) {
	if statName == "" || divisor <= 0 {
		return
	}
	sec.AddDetail("%s (%d / %d): +%d", statName, statValue, divisor, statValue/divisor)
}

// damageTypeAoELine / armorInteractionRules delegate to the shared template
// helpers so the editor's rules text is literally the same code (and lands in
// the full-only DETAIL tier — see character.ArmorInteractionLines).
func damageTypeAoELine(damageType string, aoeTiles float64) string {
	return character.DamageTypeAoELine(damageType, aoeTiles)
}

func armorInteractionRules(sec *ttSection, damageType string, isRanged, hasTrueDmg bool) {
	character.ArmorInteractionLines(sec, damageType, isRanged, hasTrueDmg, ArmorPhysicalReductionDivisor)
}

// ---------------------------------------------------------------- weapons ---

func buildWeaponTooltipUnified(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	def := lookupWeaponConfigByName(item.Name)
	if def == nil || cs == nil {
		return fmt.Sprintf("=== %s ===", item.Name)
	}
	subtitle := strings.Title(def.Category)
	if def.Rarity != "" {
		subtitle += " · " + strings.Title(def.Rarity)
	}

	attack := ttSection{Title: "ATTACK"}
	if def.Range > 0 {
		attack.Add("Range: %d tiles", def.Range)
	}
	if arc := character.MeleeSwingArcLine(def); arc != "" {
		attack.Add("%s", arc)
	}
	if cd := cooldownLine(cs, cs.WeaponCooldownFramesFor(char, item.Name)); cd != "" {
		attack.Add("%s", cd)
	}
	// Explain WHY the cooldown differs from the bare Speed curve (category
	// multiplier or a legendary override) — the editor shows the same line.
	for _, ln := range character.WeaponCombatLines(def) {
		if strings.HasPrefix(ln, "Attack cooldown") {
			attack.AddDetail("%s", ln)
		}
	}
	if def.Physics != nil && def.Physics.SpeedTiles > 0 {
		attack.AddDetail("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
	}
	if hb := character.ProjectileHitboxLine(def.Physics); hb != "" {
		attack.AddDetail("%s", hb)
	}
	if def.MaxProjectiles > 0 {
		attack.AddDetail("Maximum Projectiles: %d", def.MaxProjectiles)
	}

	dmg := ttSection{Title: "DAMAGE"}
	armsBonus := 0
	if char != nil {
		armsBonus = char.ArmsMasterTier() * ArmsMasterDamagePerTier
	}
	_, _, normal := cs.CalculateWeaponDamage(item, char)
	trueDmg, _ := cs.weaponMasteryStrike(char, def)
	dmg.AddDetail("Base: %d", def.Damage)
	primaryStat := def.BonusStat
	if primaryStat == "" {
		primaryStat = "Might"
	}
	statContribDetail(&dmg, primaryStat, getEffectiveStatValue(primaryStat, char), WeaponPrimaryStatDivisor)
	if def.BonusStatSecondary != "" {
		statContribDetail(&dmg, def.BonusStatSecondary, getEffectiveStatValue(def.BonusStatSecondary, char), WeaponSecondaryStatDivisor)
	}
	if armsBonus > 0 {
		_, tierName := masteryTier(char, character.SkillArmsMaster)
		dmg.AddDetail("Arms Master — %s: +%d", tierName, armsBonus)
	}
	dmg.AddDetail("Normal Damage: %d", normal)
	if trueDmg > 0 {
		if skill, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
			_, tierName := masteryTier(char, skill)
			dmg.AddDetail("%s Mastery — %s: +%d True", skill.String(), tierName, trueDmg)
		}
	}
	// Active party buffs (Heroism, Hour of Power, …) add a flat bonus to every
	// outgoing hit AFTER crit doubling (combat.go applyProjectileDamage), so the
	// live starting damage is higher than the gear/stat total alone.
	outBonus := cs.game.combatBuffOutBonus()
	if outBonus > 0 {
		dmg.AddDetail("Active party buff: +%d", outBonus)
	}
	dmg.Add("Total Damage: %d", normal+trueDmg+outBonus)
	totalCrit := cs.CalculateWeaponCritChance(item, char)
	if totalCrit > 0 {
		dmg.Add("Critical Damage: %d", normal*CritDamageMultiplier+trueDmg+outBonus)
	}

	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		baseCrit, luck, gmWeapon, gmArms := cs.WeaponCritBreakdown(item, char)
		crit.Add("Chance: %d%%", totalCrit)
		parts := []string{fmt.Sprintf("Base: %d%%", baseCrit), fmt.Sprintf("Luck: +%d%%", luck)}
		if gmWeapon > 0 {
			parts = append(parts, fmt.Sprintf("GM weapon: +%d%%", gmWeapon))
		}
		if gmArms > 0 {
			parts = append(parts, fmt.Sprintf("GM Arms Master: +%d%%", gmArms))
		}
		crit.AddDetail("%s", strings.Join(parts, " · "))
	}

	effects := ttSection{Title: "EFFECTS"}
	effects.Add("%s", damageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
	// Config-computable specials minus the lines this template renders itself.
	for _, ln := range def.EffectLines() {
		if strings.HasPrefix(ln, "Damage Type:") || strings.HasPrefix(ln, "AoE radius:") ||
			strings.HasPrefix(ln, "Max Airborne:") {
			continue
		}
		effects.Add("%s", ln)
	}

	rules := ttSection{Title: "RULES"}
	armorInteractionRules(&rules, def.DamageType, def.Physics != nil, trueDmg > 0)
	if def.AoeRadiusTiles > 0 {
		rules.AddDetail("%s", character.SplashCritRule)
	}
	if skill, ok := character.WeaponSkillForCategory(strings.ToLower(def.Category)); ok {
		if tier, _ := masteryTier(char, skill); tier >= int(character.MasteryGrandMaster) {
			rules.AddDetail("Grandmaster: this strike ignores Perfect Dodge")
		} else {
			rules.AddDetail("Grandmaster %s: strikes ignore Perfect Dodge", skill.String())
		}
	}

	return renderTooltip(item.Name, subtitle, []ttSection{attack, dmg, crit, effects, rules}, full)
}

// ----------------------------------------------------------------- armor ----

func buildArmorTooltipUnified(item items.Item, char *character.MMCharacter, cs *CombatSystem, full bool) string {
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	subtitle := itemKindLabel(item)
	if ok && def != nil && def.Rarity != "" {
		subtitle += " · " + strings.Title(def.Rarity)
	}

	defense := ttSection{Title: "DEFENSE"}
	totalAC := 0
	if ok && def != nil && cs != nil && (def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0) {
		totalAC = cs.CalculateArmorClassContribution(item, char)
		defense.AddDetail("Base Armor Class: %d", def.ArmorClassBase)
		if def.EnduranceScalingDivisor > 0 && char != nil {
			statContribDetail(&defense, "Endurance", char.GetEffectiveEndurance(), def.EnduranceScalingDivisor)
		}
		if cat, catOK := armorMasterySkill(item); catOK && char != nil {
			if tier, tierName := masteryTier(char, cat); tier > 0 {
				defense.AddDetail("%s Mastery — %s: +%d", cat.String(), tierName, tier*character.MasteryArmorACPerLevel)
			}
		}
		defense.Add("Total Armor Class: %d", totalAC)
	}

	effects := ttSection{Title: "EFFECTS"}
	if ok && def != nil {
		for _, ln := range def.StatBonusLines() {
			effects.Add("%s", ln)
		}
		for _, ln := range def.ResistLines() {
			effects.Add("%s", ln)
		}
	}
	if totalAC > 0 {
		effects.AddDetail("Physical Damage Reduction: AC / %d", ArmorPhysicalReductionDivisor)
		effects.Add("Current Reduction: %d per physical hit", totalAC/ArmorPhysicalReductionDivisor)
	}

	rules := ttSection{Title: "RULES"}
	if cat, catOK := armorMasterySkill(item); catOK {
		if line := getArmorRequirementLine(item, char); line != "" {
			rules.Add("%s", line) // equip requirement is per-item state, not a formula
		}
		if tier, _ := masteryTier(char, cat); tier >= int(character.MasteryGrandMaster) {
			rules.AddDetail("Grandmaster: +%d%% Perfect Dodge while worn", character.ArmorGMDodgeBonus)
		} else {
			rules.AddDetail("Grandmaster %s: +%d%% Perfect Dodge while worn", cat.String(), character.ArmorGMDodgeBonus)
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
	subtitle := fmt.Sprintf("%s Magic · Level %d", formatSchoolName(def.School), def.Level)

	casting := ttSection{Title: "CASTING"}
	cost := def.SpellPointsCost
	if cs != nil {
		cost = cs.effectiveSpellCost(char, def.SpellPointsCost)
	}
	casting.Add("Cost: %d SP", cost)
	// A GM meditator pays less — show the discount so the reduced Cost isn't a
	// mystery (Base → −% → Current).
	if cost < def.SpellPointsCost {
		casting.AddDetail("Base Cost: %d SP", def.SpellPointsCost)
		casting.AddDetail("GM Meditation: -%d%%", MeditationGMSpellCostReductionPct)
	}
	if cs != nil {
		if cd := cooldownLine(cs, cs.SpellCooldownFrames(char, def.ID)); cd != "" {
			casting.Add("%s", cd)
			casting.AddDetail("Scales with caster Speed")
			if wl := spellCooldownWeaponLine(char); wl != "" {
				casting.AddDetail("%s", wl)
			}
		}
	}
	if def.IsProjectile && cs != nil {
		if rng, okRng := cs.CalculateSpellRangeTiles(def.ID); okRng {
			casting.Add("Range: %.0f tiles", rng)
		}
		if phys, err := cs.game.config.GetSpellConfig(string(def.ID)); err == nil && phys.SpeedTiles > 0 {
			casting.AddDetail("Projectile Speed: %.0f tiles/s", phys.SpeedTiles)
			if hb := character.ProjectileHitboxLine(phys); hb != "" {
				casting.AddDetail("%s", hb)
			}
		}
	}
	switch {
	case def.HealParty || def.StatBonus > 0 || len(def.StatBonuses) > 0:
		casting.Add("Target: Entire Party")
	case def.TargetSelf:
		casting.Add("Target: Self")
	}

	tier, tierName := schoolMasteryTier(char, def.School)
	mastery := 0
	if cs != nil {
		mastery = cs.spellMasteryBonus(char, def.ID)
	}

	dmg := ttSection{Title: "DAMAGE"}
	totalCrit := 0
	if def.IsProjectile && !def.DealsNoDamage && cs != nil {
		_, _, total := cs.CalculateSpellDamage(def.ID, char)
		mult := maxInt(1, def.DamageCostMultiplier)
		base := def.SpellPointsCost * spells.SpellDamagePerSP * mult
		if mult > 1 {
			dmg.AddDetail("Base (%d SP × %d × %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, mult, base)
		} else {
			dmg.AddDetail("Base (%d SP × %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, base)
		}
		// The same stat the damage formula divides (self magic → Personality).
		primaryStat, primaryValue := "Intellect", 0
		if char != nil {
			primaryValue = char.GetEffectiveIntellect()
			if spellScalesWithPersonality(def.School) {
				primaryStat, primaryValue = "Personality", char.GetEffectivePersonality()
			}
		}
		statContribDetail(&dmg, primaryStat, primaryValue, spells.SpellIntellectDivisor)
		// Non-self magic only: self magic already shows Personality as its primary
		// stat above (mirrors the guard in CalculateSpellDamage).
		if def.ScalesWithPersonality && char != nil && !spellScalesWithPersonality(def.School) {
			statContribDetail(&dmg, "Personality", char.GetEffectivePersonality(), spells.SpellIntellectDivisor)
		}
		if mastery > 0 {
			dmg.AddDetail("%s Mastery — %s: +%d", formatSchoolName(def.School), tierName, mastery)
		}
		// Active party buffs (Heroism, …) add a flat bonus after crit doubling.
		outBonus := cs.game.combatBuffOutBonus()
		if outBonus > 0 {
			dmg.AddDetail("Active party buff: +%d", outBonus)
		}
		dmg.Add("Total Damage: %d", total+outBonus)
		totalCrit = cs.CalculateCriticalChance(char)
		if totalCrit > 0 {
			dmg.Add("Critical Damage: %d", total*CritDamageMultiplier+outBonus)
		}
	}
	// Party nova (Inferno): fixed damage, own EFFECT block.
	if def.PartyAoeRadiusTiles > 0 {
		dmg.Title = "EFFECT"
		dmg.Add("Damage: %d", def.SpellPointsCost*spells.SpellDamagePerSP)
		dmg.Add("Radius: %.0f tiles", def.PartyAoeRadiusTiles)
		dmg.Add("Targets: Monsters and Party")
	}

	heal := ttSection{Title: "HEALING"}
	if def.HealAmount > 0 && cs != nil {
		baseHeal, persBonus, totalHeal := cs.CalculateSpellHealing(def.ID, char)
		_ = persBonus
		heal.AddDetail("Base: %d", baseHeal-mastery)
		if char != nil {
			statContribDetail(&heal, "Personality", char.GetEffectivePersonality(), spells.HealingPersonalityDivisor)
		}
		if mastery > 0 {
			heal.AddDetail("%s Mastery — %s: +%d", formatSchoolName(def.School), tierName, mastery)
		}
		heal.Add("Total Healing: %d", totalHeal)
	}

	crit := ttSection{Title: "CRITICAL"}
	if totalCrit > 0 {
		crit.Add("Chance: %d%%", totalCrit)
		crit.AddDetail("Luck: +%d%%", totalCrit)
	}

	zone := ttSection{Title: "ZONE"}
	if def.ZoneRadiusTiles > 0 && cs != nil {
		zone.Add("Radius: %.0f tiles", def.ZoneRadiusTiles)
		zone.Add("RT: one tick every %.0fs", def.ZoneTickSeconds)
		zone.Add("TB: one tick per monster turn")
	}
	if def.ZoneRadiusTiles > 0 && cs != nil {
		// Tick damage decomposed exactly as CalculateSteamZoneTickDamage.
		intBonus := 0
		if char != nil {
			intBonus = char.GetEffectiveIntellect() / spells.SpellIntellectDivisor
		}
		dmg.AddDetail("Base: %d", def.ZoneTickDamage)
		if char != nil {
			statContribDetail(&dmg, "Intellect", char.GetEffectiveIntellect(), spells.SpellIntellectDivisor)
		}
		if mastery > 0 {
			dmg.AddDetail("%s Mastery — %s: +%d", formatSchoolName(def.School), tierName, mastery)
		}
		dmg.Add("Total per tick: %d", def.ZoneTickDamage+intBonus+mastery)
		dmg.Title = "DAMAGE PER TICK"
	}

	effects := ttSection{Title: "EFFECTS"}
	if def.IsProjectile && !def.DealsNoDamage {
		effects.Add("%s", damageTypeAoELine(def.School, def.AoeRadiusTiles))
	}
	// Filtered: the composed type·AoE line and the DAMAGE/HEALING sections
	// already cover what these EffectLines entries would repeat.
	for _, ln := range character.FilteredSpellEffectLines(def) {
		effects.Add("%s", ln)
	}
	if def.IncomingDamageReductionGrandmaster > def.IncomingDamageReduction && def.IncomingDamageReduction > 0 {
		current := scaledIncomingDamageReduction(def, char)
		effects.Add("Current reduction: -%d per hit", current)
	}
	// Duration decomposed: base → mastery % → current.
	if def.Duration > 0 && cs != nil {
		current := cs.CalculateSpellDurationSeconds(def.ID, char)
		effects.AddDetail("Base Duration: %ds", def.Duration)
		if tier > 0 {
			effects.AddDetail("%s Mastery — %s: +%d%%", formatSchoolName(def.School), tierName, tier*SpellMasteryDurationBonusPct)
		}
		effects.Add("Current Duration: %ds", current)
	}

	rules := ttSection{Title: "RULES"}
	switch {
	case def.PartyAoeRadiusTiles > 0:
		rules.AddDetail("Fixed damage: no stat or mastery scaling")
		rules.AddDetail("%s: no GM resistance penetration", def.Name)
		rules.AddDetail("Enemy %s Resistance reduces damage", formatSchoolName(def.School))
		rules.AddDetail("Party %s Resistance reduces self-damage", formatSchoolName(def.School))
		rules.AddDetail("Cannot critically hit")
	case def.DealsNoDamage:
		rules.AddDetail("Deals no damage")
		rules.AddDetail("Cannot critically hit")
	case def.IsProjectile || def.ZoneRadiusTiles > 0:
		rules.AddDetail("%s Resistance reduces damage", formatSchoolName(def.School))
		if line := spellGMPierceLine(def, char); line != "" {
			rules.AddDetail("GM: ignores %d%% of enemy %s Resistance", MagicGMResistPiercePct, formatSchoolName(def.School))
		}
	}
	if def.AoeRadiusTiles > 0 {
		rules.AddDetail("%s", character.SplashCritRule)
	}
	if def.IsProjectile {
		rules.AddDetail("Can be evaded by Perfect Dodge (magic mastery never pierces it)")
	}
	if def.Pacify {
		rules.AddDetail("Any received hit breaks the charm")
		rules.AddDetail("No effect on undead")
	}
	if def.StatBonus > 0 || len(def.StatBonuses) > 0 {
		rules.AddDetail("Mastery increases duration, not the bonus")
		rules.AddDetail("Recasting refreshes the effect")
	}
	if def.ZoneRadiusTiles > 0 {
		rules.AddDetail("Overlapping zones of the same spell do not stack")
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
	subtitle := fmt.Sprintf("Trap · Level %d", def.Level)

	placement := ttSection{Title: "PLACEMENT"}
	cost := def.SPCost
	if cs != nil {
		cost = cs.effectiveSpellCost(char, def.SPCost)
	}
	placement.Add("Cost: %d SP", cost)
	placement.Add("%s", character.CooldownLine(def.CooldownSeconds))
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
		}
		if tier > 0 {
			dmg.AddDetail("Trapper — %s: +%d", tierName, tier*character.TrapperDamagePerTier)
		}
		dmg.Add("Total Damage: %d", trapDamage(def, char))
	}

	effect := ttSection{Title: "EFFECT"}
	if def.StunTurns > 0 {
		t, s := trapControlDuration(def.StunTurns, def.StunSeconds, char)
		effect.AddDetail("Base Stun: %d TB turns / %ds", def.StunTurns, def.StunSeconds)
		if tier > 0 {
			effect.AddDetail("Trapper — %s: +%d turns / +%ds", tierName, t-def.StunTurns, s-def.StunSeconds)
		}
		effect.Add("Total Stun: %d TB turns / %ds", t, s)
	}
	if def.RootTurns > 0 {
		t, s := trapControlDuration(def.RootTurns, def.RootSeconds, char)
		effect.AddDetail("Base Root: %d TB turns / %ds", def.RootTurns, def.RootSeconds)
		if tier > 0 {
			effect.AddDetail("Trapper — %s: +%d turns / +%ds", tierName, t-def.RootTurns, s-def.RootSeconds)
		}
		effect.Add("Total Root: %d TB turns / %ds", t, s)
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

func buildSimpleItemTooltipUnified(item items.Item, title string, usage []string, full bool) string {
	def, _, ok := config.GetItemDefinitionByName(item.Name)
	subtitle := itemKindLabel(item)
	if ok && def != nil && def.Rarity != "" {
		subtitle += " · " + strings.Title(def.Rarity)
	}
	effect := ttSection{Title: title}
	if ok && def != nil {
		for _, ln := range def.EffectLines() {
			effect.Add("%s", ln)
		}
	}
	use := ttSection{Title: "USAGE"}
	for _, u := range usage {
		use.Add("%s", u)
	}
	return renderTooltip(item.Name, subtitle, []ttSection{effect, use}, full)
}
