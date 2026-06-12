package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// The unified card template, shared by the in-game tooltips and the map
// editor: UPPERCASE sections, base → scaling → total decomposition, RULES
// always spelling out armor/resistance interaction. The game renders it with
// the CASTER's numbers (internal/game/ui_tooltip_unified.go); the editor
// renders the character-independent variant below (formulas instead of
// personal values). Both share the section renderer and the rules logic, so
// the two views cannot disagree on mechanics.

// CardSection is one titled block of the template.
type CardSection struct {
	Title string
	Lines []string
}

// Add appends a formatted line.
func (s *CardSection) Add(format string, args ...interface{}) {
	s.Lines = append(s.Lines, fmt.Sprintf(format, args...))
}

// RenderCardLines flattens sections into display lines, hiding empty sections.
func RenderCardLines(sections []CardSection) []string {
	var out []string
	for _, sec := range sections {
		if len(sec.Lines) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, sec.Title)
		out = append(out, sec.Lines...)
	}
	return out
}

// DamageTypeAoELine composes "Fire Damage · 2-tile AoE" (any element).
func DamageTypeAoELine(damageType string, aoeTiles float64) string {
	dt := damageType
	if dt == "" {
		dt = "physical"
	}
	line := strings.Title(dt) + " Damage"
	if aoeTiles > 0 {
		line += fmt.Sprintf(" · %.0f-tile AoE", aoeTiles)
	}
	return line
}

// ArmorInteractionLines spells out how a damage type meets Armor Class —
// generic for EVERY element: physical is armor-reduced (ranged shots pierce
// 33% of the time), anything elemental skips armor and meets resistance.
func ArmorInteractionLines(sec *CardSection, damageType string, isRanged, hasTrueDmg bool, armorDivisor int) {
	dt := strings.ToLower(damageType)
	if dt == "" || dt == "physical" {
		if isRanged {
			sec.Add("%d%% of shots ignore Armor Class", ArmorPierceRangedChancePct)
			sec.Add("Other shots are reduced by Armor Class / %d", armorDivisor)
		} else {
			sec.Add("Reduced by Armor Class / %d", armorDivisor)
		}
	} else {
		sec.Add("%s damage ignores Armor Class", strings.Title(dt))
		sec.Add("%s Resistance affects normal damage", strings.Title(dt))
	}
	if hasTrueDmg {
		// Resistance still applies to the summed hit — true damage only
		// bypasses ARMOR and lands through Perfect Dodge.
		sec.Add("True Damage ignores armor and lands through dodges")
	}
}

// FilteredSpellEffectLines drops the EffectLines entries the unified template
// renders STRUCTURED elsewhere (the composed "X Damage · AoE" line and the
// decomposed DAMAGE/HEALING sections), so they don't appear twice.
func FilteredSpellEffectLines(sd spells.SpellDefinition) []string {
	var out []string
	for _, ln := range sd.EffectLines() {
		if strings.HasPrefix(ln, "AoE radius:") ||
			strings.HasPrefix(ln, "Damage scales with") ||
			strings.HasPrefix(ln, "Tick damage scales with") ||
			strings.HasPrefix(ln, "Healing scales with") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// --------------------- character-independent card builders (map editor) -----

// WeaponCardSections renders a weapon in template shape with formulas in
// place of caster numbers.
func WeaponCardSections(def *config.WeaponDefinitionConfig, armorDivisor int) []CardSection {
	attack := CardSection{Title: "ATTACK"}
	if def.Range > 0 {
		attack.Add("Range: %d tiles", def.Range)
	}
	if def.Physics != nil && def.Physics.SpeedTiles > 0 {
		attack.Add("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
	}
	if def.MaxProjectiles > 0 {
		attack.Add("Maximum Projectiles: %d", def.MaxProjectiles)
	}
	for _, ln := range WeaponCombatLines(def) {
		if strings.HasPrefix(ln, "Attack cooldown") {
			attack.Add("%s", ln)
		}
	}

	dmg := CardSection{Title: "DAMAGE"}
	dmg.Add("Base: %d", def.Damage)
	primary := def.BonusStat
	if primary == "" {
		primary = "Might"
	}
	dmg.Add("%s / %d: scales", primary, WeaponPrimaryStatDivisor)
	if def.BonusStatSecondary != "" {
		dmg.Add("%s / %d: scales", def.BonusStatSecondary, WeaponSecondaryStatDivisor)
	}
	_, hasWeaponSkill := WeaponSkillForCategory(strings.ToLower(def.Category))
	if hasWeaponSkill {
		dmg.Add("Weapon Mastery: +%d True per tier", MasteryWeaponTrueDamagePerTier)
	}

	crit := CardSection{Title: "CRITICAL"}
	if def.CritChance > 0 {
		crit.Add("Base Chance: %d%% (+Luck)", def.CritChance)
	}

	effects := CardSection{Title: "EFFECTS"}
	effects.Add("%s", DamageTypeAoELine(def.DamageType, def.AoeRadiusTiles))
	for _, ln := range def.EffectLines() {
		if strings.HasPrefix(ln, "Damage Type:") || strings.HasPrefix(ln, "AoE radius:") ||
			strings.HasPrefix(ln, "Max Airborne:") {
			continue
		}
		effects.Add("%s", ln)
	}

	rules := CardSection{Title: "RULES"}
	ArmorInteractionLines(&rules, def.DamageType, def.Physics != nil, hasWeaponSkill, armorDivisor)
	if hasWeaponSkill {
		rules.Add("Grandmaster: strikes ignore Perfect Dodge")
	}

	return []CardSection{attack, dmg, crit, effects, rules}
}

// SpellCardSections renders a spell in template shape (character-independent).
func SpellCardSections(key string, def *config.SpellDefinitionConfig, sd spells.SpellDefinition, armorDivisor int) []CardSection {
	casting := CardSection{Title: "CASTING"}
	casting.Add("Cost: %d SP", def.SpellPointsCost)
	cd := def.CooldownSeconds
	note := ""
	if cd <= 0 {
		cd = spells.SpellCooldownDefaultSecondsForLevel(def.Level)
		note = ", level default"
	}
	casting.Add("Cooldown: %.1fs (scales with Speed%s)", cd, note)
	if sd.IsProjectile && def.Physics != nil {
		if def.Physics.RangeTiles > 0 {
			casting.Add("Range: %.0f tiles", def.Physics.RangeTiles)
		}
		if def.Physics.SpeedTiles > 0 {
			casting.Add("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
		}
	}
	switch {
	case sd.HealParty || sd.StatBonus > 0 || len(sd.StatBonuses) > 0:
		casting.Add("Target: Entire Party")
	case sd.TargetSelf:
		casting.Add("Target: Self")
	}

	dmg := CardSection{Title: "DAMAGE"}
	if sd.IsProjectile && !sd.DealsNoDamage {
		mult := def.DamageCostMultiplier
		if mult <= 1 {
			dmg.Add("Base (%d SP × %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, def.SpellPointsCost*spells.SpellDamagePerSP)
		} else {
			dmg.Add("Base (%d SP × %d × %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, mult, def.SpellPointsCost*spells.SpellDamagePerSP*mult)
		}
		stat := "Intellect"
		if spells.SchoolScalesWithPersonality(def.School) {
			stat = "Personality"
		}
		dmg.Add("%s / %d: scales", stat, spells.SpellIntellectDivisor)
		dmg.Add("Mastery: +%d per tier", MasterySpellEffectPerLevel)
	}
	if sd.ZoneRadiusTiles > 0 {
		dmg.Title = "DAMAGE PER TICK"
		dmg.Add("Base: %d", sd.ZoneTickDamage)
		dmg.Add("Intellect / %d: scales", spells.SpellIntellectDivisor)
		dmg.Add("Mastery: +%d per tier", MasterySpellEffectPerLevel)
	}
	if sd.PartyAoeRadiusTiles > 0 {
		dmg.Title = "EFFECT"
		dmg.Add("Damage: %d", def.SpellPointsCost*spells.SpellDamagePerSP)
		dmg.Add("Radius: %.0f tiles", sd.PartyAoeRadiusTiles)
		dmg.Add("Targets: Monsters and Party")
	}

	heal := CardSection{Title: "HEALING"}
	if sd.HealAmount > 0 {
		heal.Add("Base: %d", sd.HealAmount)
		heal.Add("Personality / %d: scales", spells.HealingPersonalityDivisor)
		heal.Add("Mastery: +%d per tier", MasterySpellEffectPerLevel)
	}

	zone := CardSection{Title: "ZONE"}
	if sd.ZoneRadiusTiles > 0 {
		zone.Add("Radius: %.0f tiles", sd.ZoneRadiusTiles)
		zone.Add("RT: one tick every %.0fs", sd.ZoneTickSeconds)
		zone.Add("TB: one tick per monster turn")
	}

	effects := CardSection{Title: "EFFECTS"}
	if sd.IsProjectile && !sd.DealsNoDamage {
		effects.Add("%s", DamageTypeAoELine(def.School, sd.AoeRadiusTiles))
	}
	for _, ln := range FilteredSpellEffectLines(sd) {
		effects.Add("%s", ln)
	}
	if sd.Duration > 0 {
		effects.Add("Base Duration: %ds", sd.Duration)
		effects.Add("Mastery: +%d%% duration per tier", SpellMasteryDurationBonusPct)
	}

	rules := CardSection{Title: "RULES"}
	school := strings.Title(def.School)
	switch {
	case sd.PartyAoeRadiusTiles > 0:
		rules.Add("Fixed damage: no stat or mastery scaling")
		rules.Add("No GM resistance penetration")
		rules.Add("Enemy %s Resistance reduces damage", school)
		rules.Add("Party %s Resistance reduces self-damage", school)
		rules.Add("Cannot critically hit")
	case sd.DealsNoDamage:
		rules.Add("Deals no damage")
		rules.Add("Cannot critically hit")
	case sd.IsProjectile || sd.ZoneRadiusTiles > 0:
		rules.Add("%s Resistance reduces damage", school)
		rules.Add("Grandmaster: ignores %d%% of enemy %s Resistance", MagicGMResistPiercePct, school)
	}
	if sd.Pacify {
		rules.Add("Any received hit breaks the charm")
		rules.Add("No effect on undead")
	}
	if sd.StatBonus > 0 || len(sd.StatBonuses) > 0 {
		rules.Add("Mastery increases duration, not the bonus")
		rules.Add("Recasting refreshes the effect")
	}
	if sd.ZoneRadiusTiles > 0 {
		rules.Add("Overlapping zones of the same spell do not stack")
	}
	if def.MonsterOnly {
		rules.Add("Monster only — never offered to the party")
	}

	return []CardSection{casting, dmg, heal, zone, effects, rules}
}

// TrapCardSections renders a trap in template shape (character-independent).
func TrapCardSections(def *config.TrapDefinitionConfig, placeRangeTiles, maxPerOwner, armorDivisor int) []CardSection {
	placement := CardSection{Title: "PLACEMENT"}
	placement.Add("Cost: %d SP", def.SPCost)
	placement.Add("Cooldown: %.1fs", def.CooldownSeconds)
	placement.Add("Range: %d tiles", placeRangeTiles)
	placement.Add("Armed Lifetime: %ds", def.LifetimeSeconds)

	dmg := CardSection{Title: "DAMAGE"}
	if def.DamageBase > 0 {
		dmg.Add("Base: %d", def.DamageBase)
		dmg.Add("(Intellect + Accuracy) / %d: scales", TrapStatScalingDivisor)
		dmg.Add("Trapper: +%d per tier", TrapperDamagePerTier)
	}

	effect := CardSection{Title: "EFFECT"}
	if def.StunTurns > 0 {
		effect.Add("Base Stun: %d TB turns / %ds", def.StunTurns, def.StunSeconds)
		effect.Add("Trapper: +%d turn / +%ds per tier", TrapperTurnsPerTier, TrapperSecondsPerTier)
	}
	if def.RootTurns > 0 {
		effect.Add("Base Root: %d TB turns / %ds", def.RootTurns, def.RootSeconds)
		effect.Add("Trapper: +%d turn / +%ds per tier", TrapperTurnsPerTier, TrapperSecondsPerTier)
	}

	effects := CardSection{Title: "EFFECTS"}
	if def.DamageBase > 0 {
		effects.Add("%s", DamageTypeAoELine(def.Element, def.AoeRadiusTiles))
	}

	rules := CardSection{Title: "RULES"}
	if def.RootTurns > 0 {
		rules.Add("Prevents movement but not attacks")
	}
	if def.DamageBase > 0 {
		ArmorInteractionLines(&rules, def.Element, false, false, armorDivisor)
	}
	rules.Add("Triggers once, then disappears")
	rules.Add("Maximum %d armed traps per character on the map", maxPerOwner)

	return []CardSection{placement, dmg, effect, effects, rules}
}

// ItemCardSections renders a wearable/consumable/quest item in template shape.
func ItemCardSections(def *config.ItemDefinitionConfig, armorDivisor int) []CardSection {
	_, hasArmorSkill := ArmorSkillForCategory(strings.ToLower(def.ArmorType))
	defense := CardSection{Title: "DEFENSE"}
	if def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0 {
		if def.ArmorClassBase > 0 {
			defense.Add("Base Armor Class: %d", def.ArmorClassBase)
		}
		if def.EnduranceScalingDivisor > 0 {
			defense.Add("Endurance / %d: scales", def.EnduranceScalingDivisor)
		}
		// Cloth (and other skill-less categories) gains no mastery AC.
		if hasArmorSkill {
			defense.Add("Armor Mastery: +%d per tier", MasteryArmorACPerLevel)
		}
	}

	effects := CardSection{Title: "EFFECTS"}
	for _, ln := range def.StatBonusLines() {
		effects.Add("%s", ln)
	}
	for _, ln := range def.ResistLines() {
		effects.Add("%s", ln)
	}
	if def.ArmorClassBase > 0 || def.EnduranceScalingDivisor > 0 {
		effects.Add("Physical Damage Reduction: AC / %d", armorDivisor)
	}
	// Consumable / quest behavior shares the item formatter.
	for _, ln := range def.EffectLines() {
		if strings.HasPrefix(ln, "Armor class") || strings.HasPrefix(ln, "AC +Endurance") {
			continue // already decomposed in DEFENSE
		}
		if containsLine(effects.Lines, ln) {
			continue
		}
		effects.Add("%s", ln)
	}

	rules := CardSection{Title: "RULES"}
	if hasArmorSkill {
		rules.Add("Requires: %s Skill", strings.Title(def.ArmorType))
		rules.Add("Grandmaster: +%d%% Perfect Dodge while worn", ArmorGMDodgeBonus)
	}

	return []CardSection{defense, effects, rules}
}

func containsLine(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}
