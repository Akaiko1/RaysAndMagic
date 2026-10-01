package game

import (
	"fmt"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// spellAreaLines keeps the delivery geometry identical in cards and comparisons.
func spellAreaLines(def spells.SpellDefinition) []string {
	var lines []string
	switch def.DamageFormula().Kind {
	case spells.DamageNova:
		if def.MapWide {
			lines = append(lines, "Radius: Current map")
		} else {
			lines = append(lines, fmt.Sprintf("Radius: %.0f tiles", def.PartyAoeRadiusTiles))
		}
		if def.SparesParty {
			lines = append(lines, "Targets: Monsters only")
		} else {
			lines = append(lines, "Targets: Monsters and Party")
		}
		if def.StandeeDestroyChance > 0 {
			lines = append(lines, fmt.Sprintf("Topples trees, dunes and rocks: %.0f%% each", def.StandeeDestroyChance*100))
		}
	case spells.DamageZone:
		if def.ZoneWidthTiles > 1 {
			lines = append(lines, fmt.Sprintf("Wall: %d tiles across, %.0f tiles ahead", def.ZoneWidthTiles, def.ZoneAheadTiles))
		} else {
			lines = append(lines, fmt.Sprintf("Radius: %.0f tiles", def.ZoneRadiusTiles))
		}
		if def.ZoneEdgeTiles > 0 {
			lines = append(lines, fmt.Sprintf("Around the wall (%s): %d%% damage", pluralizeCount(def.ZoneEdgeTiles, "tile", "tiles"), def.ZoneEdgeDamagePercent))
		}
		if def.ZoneBurnSeconds > 0 {
			lines = append(lines, fmt.Sprintf("Sets monsters it touches burning for %.0fs: %d%% of max HP per second (%d%% on bosses), ignoring fire resistance",
				def.ZoneBurnSeconds, config.BurnPercent(false), config.BurnPercent(true)))
		}
		lines = append(lines, "A monster that walks in is hit at once")
		lines = append(lines, fmt.Sprintf("RT: one tick every %.0fs", def.ZoneTickSeconds))
		if ticks := int(float64(TurnBasedPeriodicEffectSeconds) / def.ZoneTickSeconds); def.ZoneTickSeconds > 0 && ticks > 1 {
			lines = append(lines, fmt.Sprintf("TB: %d ticks per monster turn", ticks))
		} else {
			lines = append(lines, "TB: one tick per monster turn")
		}
	}
	return lines
}

func addWeaponCooldown(sec *ttSection, ch *character.MMCharacter, cs *CombatSystem, def *config.WeaponDefinitionConfig) {
	tps := float64(config.GetTargetTPS())
	if cs != nil && cs.game != nil {
		tps = float64(cs.game.config.GetTPS())
	}
	reference := math.Round(float64(calculateSpeedActionCooldownFrames(0)) * RTBaseCooldownMult * character.WeaponCooldownMultiplier(def))
	if ch == nil {
		sec.Add("Base weapon cooldown: %.2fs", character.CardSeconds(reference/tps))
		sec.AddDetail("Scales with wielder Speed - TB: 1 action")
		return
	}
	cd := cs.weaponCooldownBreakdown(ch, def.Name)
	sec.Add("%s", cooldownLine(cs, cd.TotalFrames))
	sec.AddDetail("Base weapon cooldown: %.2fs", character.CardSeconds(reference/tps))
	atSpeed := math.Round(cd.BaseFrames * cd.WeaponMultiplier)
	// Subtract displayed stages so their two-decimal values add up on the card.
	delta := math.Round(atSpeed*100/tps)/100 - math.Round(reference*100/tps)/100
	sec.AddDetail("Speed (%d): %+.2fs", cd.Speed, delta)
	if cd.DualWieldingReductionPct > 0 {
		_, tier := masteryTier(ch, character.SkillDualWielding)
		sec.AddDetail("Dual Wielding - %s: -%d%% cooldown", tier, cd.DualWieldingReductionPct)
	}
	addCooldownAdjustments(sec, cs, cd.RawFrames, cd.ClampedFrames, cd.TotalFrames, cd.QuickenPct)
}

// addCooldownAdjustments names the stages after the formula: the RT cooldown
// limits, then Quickening's recovery cut.
func addCooldownAdjustments(sec *ttSection, cs *CombatSystem, raw, clamped, total, quickenPct int) {
	if clamped != raw {
		sec.AddDetail("Cooldown limit: %s", cooldownSeconds(cs, clamped))
	}
	if total != clamped {
		sec.AddDetail("Quickening: -%d%% recovery", quickenPct)
	}
}

func addCastingCost(sec *ttSection, base int, ch *character.MMCharacter, cs *CombatSystem) {
	cost := base
	if cs != nil {
		cost = cs.effectiveSpellCost(ch, base)
	}
	if cost != base {
		sec.AddDetail("Base Cost: %d SP", base)
		sec.AddDetail("Meditation - Grandmaster: -%d%%", MeditationGMSpellCostReductionPct)
	}
	sec.Add("Cost: %d SP", cost)
}

func addSpellCooldown(sec *ttSection, id spells.SpellID, ch *character.MMCharacter, cs *CombatSystem) {
	tb := "1 action"
	def, err := spells.GetSpellDefinitionByID(id)
	isSpell := err == nil
	if isSpell {
		tb = character.SpellTBCost(def)
	}
	if ch == nil || cs == nil {
		if base, ok := baseCastCooldownSeconds(id); ok && base > 0 {
			sec.Add("Base cooldown: %.2fs", base)
			sec.AddDetail("Scales with caster Speed - TB: %s", tb)
		} else if isSpell && def.IsBuff() {
			sec.Add("No RT cooldown - TB: %s", tb)
		}
		return
	}
	cd := cs.spellCooldownBreakdown(ch, id)
	if cd.TotalFrames <= 0 {
		// Buffs have no personal RT cooldown; a TB cast still spends the action.
		if isSpell && def.IsBuff() {
			sec.Add("No RT cooldown - TB: %s", tb)
		}
		return
	}
	sec.AddDetail("Base cooldown: %.2fs", character.CardSeconds(cd.BaseSeconds))
	tps := float64(cs.game.config.GetTPS())
	atSpeed := math.Round(cd.BaseSeconds*cd.SpeedFactor*tps) / tps
	delta := math.Round(atSpeed*100)/100 - math.Round(cd.BaseSeconds*100)/100
	sec.AddDetail("Speed (%d): %+.2fs", cd.Speed, delta)
	if cd.WeaponMultiplier != 1 {
		sec.AddDetail("%s: x%.2f cooldown", cd.WeaponName, cd.WeaponMultiplier)
	}
	addCooldownAdjustments(sec, cs, cd.RawFrames, cd.ClampedFrames, cd.TotalFrames, cd.QuickenPct)
	sec.Add("%s", character.CooldownLineTB(float64(cd.TotalFrames)/tps, tb))
}

func addItemEffects(sec *ttSection, def *config.ItemDefinitionConfig, item items.Item, ch *character.MMCharacter) {
	if ch == nil {
		for _, line := range character.FilteredItemEffectLines(def) {
			sec.Add("%s", line)
		}
		return
	}
	for _, line := range def.FixedEffectLines() {
		sec.Add("%s", line)
	}
	intellect, personality := ch.ItemAttributeScalingBonuses(item)
	for _, row := range []struct {
		stat              string
		value, div, bonus int
	}{
		{"Intellect", ch.Intellect, def.IntellectScalingDivisor, intellect},
		{"Personality", ch.Personality, def.PersonalityScalingDivisor, personality},
	} {
		if row.div <= 0 {
			continue
		}
		sec.AddDetail("Base %s (%d / %d): +%d", row.stat, row.value, row.div, row.bonus)
		sec.Add("%s: +%d", row.stat, row.bonus)
	}
}

func spellCurrentEffects(def spells.SpellDefinition, char *character.MMCharacter, includeDamageType bool) ttSection {
	tier, tierName := spellMasteryTier(char, def)
	effects := ttSection{Title: "EFFECTS"}
	if includeDamageType && def.IsProjectile && !def.DealsNoDamage {
		effects.Add("%s", damageTypeAoELine(def.School, def.AoeRadiusTiles))
	}
	for _, ln := range def.CoreEffectLines() {
		effects.Add("%s", ln)
	}
	if def.SummonMonster != "" {
		if tierName != "" {
			effects.AddDetail("%s Mastery - %s", formatSchoolName(spellSchoolForChar(char, def)), tierName)
		}
		if hp := masteryLadderValue(def.SummonHPByMastery, tier); hp > 0 {
			effects.Add("Summon HP: %d", hp)
		}
		if damage := masteryLadderValue(def.SummonDamageByMastery, tier); damage > 0 {
			effects.Add("Summon Damage: %d", damage)
		}
		if monsterPkg.MonsterConfig != nil {
			if m, err := monsterPkg.MonsterConfig.GetMonsterByKey(def.SummonMonster); err == nil && m.ProjectileSpell != "" {
				if shot, err := spells.GetSpellDefinitionByID(spells.SpellID(m.ProjectileSpell)); err == nil {
					effects.AddDetail("Attacks with %s (%s, range %.0f tiles)", shot.Name, config.TitleWords(shot.School), m.RangedAttackRange)
				}
			}
		}
	}
	if def.StatBonus > 0 {
		current := scaledSpellMasteryValue(def, char, def.StatBonus, def.StatBonusGrandmaster)
		effects.Add("%s bonus to all stats: +%d", tooltipValuePrefix(char), current)
	}
	if def.ResistBuffPct > 0 {
		current := scaledSpellMasteryValue(def, char, def.ResistBuffPct, def.ResistBuffPctGrandmaster)
		effects.Add("%s incoming damage: -%d%%", tooltipValuePrefix(char), current)
	}
	if def.OutgoingDamageBonus > 0 {
		current := scaledSpellMasteryValue(def, char, def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster)
		target := "damage"
		if damageType, err := damagecalc.ParseType(def.OutgoingDamageType); err == nil && damageType == damagecalc.Physical {
			target = "physical damage"
		}
		effects.Add("%s %s bonus: +%d", tooltipValuePrefix(char), target, current)
	}
	if def.IncomingDamageReduction > 0 {
		current := scaledIncomingDamageReduction(def, char)
		effects.Add("%s reduction: -%d per hit", tooltipValuePrefix(char), current)
		effects.AddDetail("Cuts normal damage after armor and resistance; true damage and damage over time pass")
	}
	return effects
}

func tooltipValuePrefix(char *character.MMCharacter) string {
	if char == nil {
		return "Base"
	}
	return "Current"
}

func addDamageTotal(sec *ttSection, label string, total, trueDamage int) {
	if trueDamage > 0 {
		sec.Add("%s: %d (%d Normal + %d True)", label, total, total-trueDamage, trueDamage)
		sec.AddDetail("True Damage ignores armor and dodge")
	} else {
		sec.Add("%s: %d", label, total)
	}
}
