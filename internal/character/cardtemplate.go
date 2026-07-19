package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// The unified card template, shared by the in-game tooltips and the map
// editor: UPPERCASE sections, base -> scaling -> total decomposition, RULES
// always spelling out armor/resistance interaction. The game renders it with
// the CASTER's numbers (internal/game/ui_tooltip_unified.go); the editor
// renders the character-independent variant below (formulas instead of
// personal values). Both share the section renderer and the rules logic, so
// the two views cannot disagree on mechanics.

// cardLine is one rendered line plus whether it's full-view-only.
type cardLine struct {
	text   string
	detail bool
}

// CardSection is one titled block of the template. Lines keep INSERTION ORDER
// with a per-line compact/detail flag: Add() lines show always (totals, costs,
// the item's special effects + must-see flags), AddDetail() lines only in the
// full view (Shift held in-game) - the Base->Stat->Mastery decomposition and the
// universal armor/resistance RULES. Keeping one ordered list (not two arrays)
// means the full view renders in the SAME order the builder added them, so a
// decomposition added before its Total reads "Base -> ... -> Total", not reversed.
type CardSection struct {
	Title string
	lines []cardLine
}

// Add appends an always-visible line (compact + full).
func (s *CardSection) Add(format string, args ...interface{}) {
	s.lines = append(s.lines, cardLine{fmt.Sprintf(format, args...), false})
}

// AddDetail appends a full-view-only line (hidden in compact).
func (s *CardSection) AddDetail(format string, args ...interface{}) {
	s.lines = append(s.lines, cardLine{fmt.Sprintf(format, args...), true})
}

func (s *CardSection) hasDetail() bool {
	for _, l := range s.lines {
		if l.detail {
			return true
		}
	}
	return false
}

// SectionsHaveDetail reports whether any section carries full-only lines (so
// the caller knows whether to show a "[Shift] full breakdown" hint).
func SectionsHaveDetail(sections []CardSection) bool {
	for i := range sections {
		if sections[i].hasDetail() {
			return true
		}
	}
	return false
}

// RenderCardLines flattens sections into display lines, hiding empty sections.
// Lines render in insertion order; compact (full=false) skips detail lines.
// The map editor always passes true - it's a reference panel, not a tooltip.
func RenderCardLines(sections []CardSection, full bool) []string {
	var out []string
	for _, sec := range sections {
		var lines []string
		for _, l := range sec.lines {
			if l.detail && !full {
				continue
			}
			lines = append(lines, l.text)
		}
		if len(lines) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, sec.Title)
		out = append(out, lines...)
	}
	return out
}

// DamageTypeAoELine composes "Fire Damage - 2-tile AoE" (any element).
func DamageTypeAoELine(damageType string, aoeTiles float64) string {
	dt := damageType
	if dt == "" {
		dt = "physical"
	}
	line := strings.Title(dt) + " Damage"
	if aoeTiles > 0 {
		line += fmt.Sprintf(" - %.0f-tile AoE", aoeTiles)
	}
	return line
}

// MeleeSwingArcLine describes a melee weapon's swing shape and reach. A swing
// strikes EVERY enemy inside the cone, so the arc width is as decision-relevant
// as the reach. Returns "" for projectile weapons (Physics set) or no melee.
func MeleeSwingArcLine(def *config.WeaponDefinitionConfig) string {
	if def == nil || def.Physics != nil || def.Melee == nil || def.Melee.ArcType <= 0 {
		return ""
	}
	var shape string
	switch def.Melee.ArcType {
	case 1:
		shape = "Strikes straight ahead"
	case 2:
		shape = "Strikes the front and one flank"
	case 3:
		shape = "Strikes the front and both diagonals"
	case 4:
		shape = "Strikes the front, both diagonals and both sides"
	default:
		return ""
	}
	// Reach depth only: which directions get hit is the shape sentence's job,
	// and depth counts diagonals as one step for EVERY weapon, so a per-item
	// "diagonals included" note carried no information.
	reach := "reaches 1 tile"
	if def.Range >= 2 {
		reach = fmt.Sprintf("reaches %d tiles deep in the cone", def.Range)
	}
	return fmt.Sprintf("%s; %s", shape, reach)
}

// MeleeArcShortLabel is the compact arc descriptor used in comparison tooltips
// (e.g. "front+diagonals"). Returns "" for projectile weapons or no melee.
func MeleeArcShortLabel(def *config.WeaponDefinitionConfig) string {
	if def == nil || def.Physics != nil || def.Melee == nil || def.Melee.ArcType <= 0 {
		return ""
	}
	switch def.Melee.ArcType {
	case 1:
		return "front"
	case 2:
		return "front+flank"
	case 3:
		return "front+diagonals"
	case 4:
		return "front+diagonals+sides"
	default:
		return ""
	}
}

// ProjectileHitboxLine reports a projectile's collision footprint in tiles - its
// accuracy size, a wider box is easier to land. Mirrors GetCollisionSizePixels'
// 0.5-tile floor. Returns "" when there is no projectile physics.
func ProjectileHitboxLine(physics *config.ProjectilePhysicsConfig) string {
	if physics == nil || physics.SpeedTiles <= 0 {
		return ""
	}
	size := physics.CollisionSizeTiles
	if size < 0.5 {
		size = 0.5
	}
	return fmt.Sprintf("Hitbox: %.1f tiles", size)
}

// SplashCritRule is the universal AoE caveat: the splash deals the PRIMARY hit's
// damage, so when the primary crits the whole splash is crit-boosted too (it
// rolls no separate crit/disintegrate/stun of its own).
const SplashCritRule = "A critical hit on the primary target boosts the splash damage too"

// CooldownLine formats a real-time cooldown, noting that turn-based combat
// ignores the seconds and spends the actor's single action for the turn instead.
func CooldownLine(seconds float64) string {
	return fmt.Sprintf("RT Cooldown: %.1fs - TB: 1 action", seconds)
}

// ArmorInteractionLines spells out how a damage type meets the target's defenses
// under the percentage armor model: armor mitigates physical up to its cap and
// elemental up to a lower cap (diminishing returns), elemental also meets
// Resistance, ranged physical shots can pierce armor. Universal/educational RULES
// -> DETAIL tier (full view only); the map editor renders full and still shows them.
func ArmorInteractionLines(sec *CardSection, damageType string, isRanged, hasTrueDmg bool) {
	dt := strings.ToLower(damageType)
	if dt == "" || dt == "physical" {
		sec.AddDetail("Reduced by target Armor (up to %d%%, diminishing)", ArmorPhysicalMitigationCap)
		if isRanged {
			sec.AddDetail("%d%% of shots pierce armor entirely", ArmorPierceRangedChancePct)
		}
	} else {
		sec.AddDetail("Reduced by target Armor (up to %d%%) and %s Resistance", ArmorElementalMitigationCap, strings.Title(dt))
	}
	if hasTrueDmg {
		// Resistance still applies to the summed hit - true damage only
		// bypasses ARMOR and lands through Perfect Dodge.
		sec.AddDetail("True Damage ignores armor and lands through dodges")
	}
}

// FilteredSpellEffectLines drops the EffectLines entries the unified template
// renders STRUCTURED elsewhere (the composed "X Damage - AoE" line and the
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
//
// Why these exist as a SECOND set of builders, parallel to the game's
// build*TooltipUnified (internal/game/ui_tooltip_unified.go), instead of one
// unified builder:
//
//  1. Dependency direction. Every live number on the game card - the damage
//     decomposition, crit breakdown, the cooldown computed from the caster's
//     Speed, effective stats, mastery bonuses, the active-buff bonus, the
//     Meditation discount - is produced by CombatSystem, which lives in package
//     `game`. The map editor (assets/map_viewer) imports `character` but MUST
//     NOT import `game`. A single builder living here would therefore have to
//     pull ~14 combat operations back through an injected interface that
//     `character` defines and `game` implements - which makes the tooltip a
//     second, full API surface over the entire combat system. That coupling is
//     worse than the duplication it removes.
//
//  2. The two cards are not "same lines, different numbers". The game card
//     decomposes into per-term VALUES and prints a numeric Total/Critical; the
//     editor has no character and prints FORMULAS with no totals at all
//     ("Intellect / 3: scales" vs "Intellect (20 / 3): +6"). A unified builder
//     would need a value-or-formula term model whose own complexity exceeds the
//     duplication, or it degrades into `if character != nil` branches everywhere.
//
// So the two builders are kept deliberately parallel: SAME section order, SHARED
// EffectLines / rule helpers / constants, but each owns its own term rendering.
// The risk of that arrangement is drift (one builder forgetting a term the other
// has - exactly how Ray of Light lost its Personality term and the editor lost
// its Arms Master and crit lines). TestCardParity_* (internal/game) re-derives a
// normalized mechanic skeleton from BOTH rendered cards and fails the suite if
// they disagree, turning this convention into an enforced contract. The eventual
// clean unification is "spec + precomputed CardFacts" (the game computes facts
// once via CombatSystem; the editor builds the same facts with nil values; one
// renderer picks value-or-formula) - NOT an injected CombatSystem.

// WeaponCardSections renders a weapon in template shape with formulas in
// place of caster numbers.
func WeaponCardSections(def *config.WeaponDefinitionConfig) []CardSection {
	attack := CardSection{Title: "ATTACK"}
	if def.Range > 0 {
		attack.Add("Range: %d tiles", def.Range)
	}
	if arc := MeleeSwingArcLine(def); arc != "" {
		attack.Add("%s", arc)
	}
	if def.Physics != nil && def.Physics.SpeedTiles > 0 {
		attack.Add("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
	}
	if hb := ProjectileHitboxLine(def.Physics); hb != "" {
		attack.AddDetail("%s", hb)
	}
	if def.MaxProjectiles > 0 {
		attack.Add("Maximum Projectiles: %d", def.MaxProjectiles)
	}
	if def.Volley > 1 {
		attack.Add("Volley: %d per shot", def.Volley)
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
	dmg.Add("Arms Master: +%d Normal per tier", ArmsMasterDamagePerTier)
	_, hasWeaponSkill := WeaponSkillForCategory(strings.ToLower(def.Category))
	if hasWeaponSkill {
		dmg.Add("Weapon Mastery: +%d True per tier", MasteryWeaponTrueDamagePerTier)
	}

	crit := CardSection{Title: "CRITICAL"}
	crit.Add("Base Chance: %d%%", def.CritChance)
	crit.Add("Luck / %d: adds to chance", LuckToCritDivisor)
	if hasWeaponSkill {
		crit.Add("Grandmaster weapon: +%d%%", WeaponGMCritBonus)
	}
	crit.Add("Grandmaster Arms Master: +%d%%", ArmsMasterGMCritBonus)
	crit.Add("Critical hits deal x%d damage", CritDamageMultiplier)

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
	ArmorInteractionLines(&rules, def.DamageType, def.Physics != nil, hasWeaponSkill)
	if def.AoeRadiusTiles > 0 {
		rules.Add("%s", SplashCritRule)
	}
	if hasWeaponSkill {
		rules.Add("Grandmaster: strikes ignore Perfect Dodge")
	}

	return []CardSection{attack, dmg, crit, effects, rules}
}

// SpellCardSections renders a spell in template shape (character-independent).
func SpellCardSections(key string, def *config.SpellDefinitionConfig, sd spells.SpellDefinition) []CardSection {
	casting := CardSection{Title: "CASTING"}
	casting.Add("Cost: %d SP", def.SpellPointsCost)
	// Buffs have no personal RT cooldown (but still spend a TB action). Match the
	// game card by omitting a cooldown line entirely instead of displaying the
	// authored fallback value as if it were active.
	if !sd.IsBuff() {
		cd := def.CooldownSeconds
		note := ""
		if cd <= 0 {
			cd = spells.SpellCooldownDefaultSecondsForLevel(def.Level)
			note = " (level default)"
		}
		casting.Add("%s", CooldownLine(cd))
		casting.Add("Scales with caster Speed%s", note)
	}
	if sd.IsProjectile && def.Physics != nil {
		if def.Physics.RangeTiles > 0 {
			casting.Add("Range: %.0f tiles", def.Physics.RangeTiles)
		}
		if def.Physics.SpeedTiles > 0 {
			casting.Add("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
		}
		if hb := ProjectileHitboxLine(def.Physics); hb != "" {
			casting.AddDetail("%s", hb)
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
			dmg.Add("Base (%d SP x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, def.SpellPointsCost*spells.SpellDamagePerSP)
		} else {
			dmg.Add("Base (%d SP x %d x %d): %d", def.SpellPointsCost, spells.SpellDamagePerSP, mult, def.SpellPointsCost*spells.SpellDamagePerSP*mult)
		}
		stat := "Intellect"
		if spells.SchoolScalesWithPersonality(def.School) {
			stat = "Personality"
		}
		dmg.Add("%s / %d: scales", stat, spells.SpellIntellectDivisor)
		if sd.ScalesWithPersonality {
			dmg.Add("Personality / %d: scales", spells.SpellIntellectDivisor)
		}
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

	// Damage projectiles crit on a Luck-based roll (no base crit for spells) for
	// xCritDamageMultiplier - the in-game card shows the same block, so it must
	// appear here too (character-independent form).
	crit := CardSection{Title: "CRITICAL"}
	if sd.IsProjectile && !sd.DealsNoDamage {
		crit.Add("Chance: Luck / %d", LuckToCritDivisor)
		crit.Add("Critical hits deal x%d damage", CritDamageMultiplier)
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
		rules.Add("%s: no GM resistance penetration", def.Name)
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
	if sd.AoeRadiusTiles > 0 {
		rules.Add("%s", SplashCritRule)
	}
	if sd.IsProjectile {
		rules.Add("Can be evaded by Perfect Dodge (magic mastery never pierces it)")
	}
	if sd.Pacify {
		rules.Add("Any received hit breaks the charm")
		rules.Add("No effect on undead")
	}
	if sd.StatBonus > 0 || len(sd.StatBonuses) > 0 {
		if sd.StatBonusGrandmaster > sd.StatBonus {
			rules.Add("Mastery increases duration and the bonus")
		} else {
			rules.Add("Mastery increases duration, not the bonus")
		}
		rules.Add("Recasting refreshes the effect")
	}
	if sd.ZoneRadiusTiles > 0 {
		rules.Add("Overlapping zones of the same spell do not stack")
	}
	if def.MonsterOnly {
		rules.Add("Monster only - never offered to the party")
	}

	return []CardSection{casting, dmg, heal, crit, zone, effects, rules}
}

// MonsterSpellCardSections renders a MONSTER-ONLY spell. Monsters cast these
// with their OWN attack damage (combat.go spawnMonsterSpellProjectile) - no SP
// cost, no Intellect/mastery scaling, no crit - so the player-formula card would
// be a fiction. Disintegrate / AoE / stun riders still fire, so the EffectLines
// stay.
func MonsterSpellCardSections(def *config.SpellDefinitionConfig, sd spells.SpellDefinition) []CardSection {
	casting := CardSection{Title: "CASTING"}
	casting.Add("Cast by monsters only - never learnable")
	if sd.IsProjectile && def.Physics != nil {
		if def.Physics.RangeTiles > 0 {
			casting.Add("Range: %.0f tiles", def.Physics.RangeTiles)
		}
		if def.Physics.SpeedTiles > 0 {
			casting.Add("Projectile Speed: %.0f tiles/s", def.Physics.SpeedTiles)
		}
		if hb := ProjectileHitboxLine(def.Physics); hb != "" {
			casting.Add("%s", hb)
		}
	}

	dmg := CardSection{Title: "DAMAGE"}
	if sd.IsProjectile && !sd.DealsNoDamage {
		dmg.Add("Deals the casting monster's attack damage")
		dmg.Add("No SP, Intellect, mastery or critical scaling")
	}

	effects := CardSection{Title: "EFFECTS"}
	effects.Add("%s", DamageTypeAoELine(def.School, sd.AoeRadiusTiles))
	for _, ln := range FilteredSpellEffectLines(sd) {
		effects.Add("%s", ln)
	}

	rules := CardSection{Title: "RULES"}
	rules.Add("Strikes your party, not other monsters")
	rules.Add("Monster only - never offered to the party")

	return []CardSection{casting, dmg, effects, rules}
}

// TrapCardSections renders a trap in template shape (character-independent).
func TrapCardSections(def *config.TrapDefinitionConfig, placeRangeTiles, maxPerOwner int) []CardSection {
	placement := CardSection{Title: "PLACEMENT"}
	placement.Add("Cost: %d SP", def.SPCost)
	placement.Add("%s", CooldownLine(def.CooldownSeconds))
	placement.Add("Range: %d tiles", placeRangeTiles)
	placement.Add("Armed Lifetime: %ds", def.LifetimeSeconds)

	dmg := CardSection{Title: "DAMAGE"}
	if def.DamageBase > 0 {
		dmg.Add("Base: %d", def.DamageBase)
		dmg.Add("(Intellect + Accuracy) / %d: scales", TrapStatScalingDivisor)
		dmg.Add("Trapper: +%d per tier", TrapperDamagePerTier)
	}

	gmTier := int(MasteryGrandMaster)
	effect := CardSection{Title: "EFFECT"}
	if def.StunTurns > 0 {
		effect.Add("Base Stun: %d TB turns / %ds", def.StunTurns, def.StunSeconds)
		effect.Add("Trapper: +%ds per tier, up to +%d TB turns at Grandmaster", TrapperSecondsPerTier, TrapperTurnBonus(gmTier))
	}
	if def.RootTurns > 0 {
		effect.Add("Base Root: %d TB turns / %ds", def.RootTurns, def.RootSeconds)
		effect.Add("Trapper: +%ds per tier, up to +%d TB turns at Grandmaster", TrapperSecondsPerTier, TrapperTurnBonus(gmTier))
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
		ArmorInteractionLines(&rules, def.Element, false, false)
	}
	rules.Add("Triggers once, then disappears")
	rules.Add("Maximum %d armed traps per character on the map", maxPerOwner)

	return []CardSection{placement, dmg, effect, effects, rules}
}

// ItemCardSections renders a wearable/consumable/quest item in template shape.
func ItemCardSections(def *config.ItemDefinitionConfig) []CardSection {
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
		effects.Add("Armor mitigates physical up to %d%%, elemental up to %d%% (diminishing)", ArmorPhysicalMitigationCap, ArmorElementalMitigationCap)
	}
	// Consumable / quest behavior shares the item formatter.
	for _, ln := range def.EffectLines() {
		if strings.HasPrefix(ln, "Armor class") || strings.HasPrefix(ln, "AC +Endurance") {
			continue // already decomposed in DEFENSE
		}
		if effects.containsText(ln) {
			continue
		}
		effects.Add("%s", ln)
	}

	usage := CardSection{Title: "USAGE"}
	for _, ln := range def.TooltipUsageLines() {
		usage.Add("%s", ln)
	}

	rules := CardSection{Title: "RULES"}
	if hasArmorSkill {
		rules.Add("Requires: %s Skill", strings.Title(def.ArmorType))
		rules.Add("Grandmaster: +%d%% Perfect Dodge while worn", ArmorGMDodgeBonus)
	}

	return []CardSection{defense, effects, usage, rules}
}

func (s *CardSection) containsText(t string) bool {
	for _, l := range s.lines {
		if l.text == t {
			return true
		}
	}
	return false
}
