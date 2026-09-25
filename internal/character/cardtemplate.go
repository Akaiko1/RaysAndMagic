package character

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
)

// Card sections and mechanic descriptions shared by game and editor tooltips.

// cardLine is one rendered line plus whether it's full-view-only.
type cardLine struct {
	text   string
	detail bool
}

// CardSection is one mechanic's results and their explanation. Add() records
// always-visible results and conditions; AddDetail() records the full-view
// calculation and exceptions. Rendering keeps source order within each tier,
// with results first, so expanding a card never buries the answer.
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
// Results precede details; compact (full=false) skips detail lines.
// The map editor always passes true - it's a reference panel, not a tooltip.
func RenderCardLines(sections []CardSection, full bool) []string {
	var out []string
	for _, sec := range sections {
		var lines []string
		for _, detail := range []bool{false, true} {
			if detail && !full {
				continue
			}
			for _, l := range sec.lines {
				if l.detail == detail {
					lines = append(lines, l.text)
				}
			}
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
		dt = damagecalc.Physical.String()
	}
	line := config.TitleWords(dt) + " Damage"
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

// SplashCritRule describes the shared source roll. Target-specific bonuses
// such as Designate Target are resolved separately for each victim.
const SplashCritRule = "One base critical roll applies to the primary hit and its splash"

const WeaponSplashCritRule = SplashCritRule + "; Designate Target's critical bonus applies separately to each marked victim"

// CooldownLine formats a real-time cooldown, noting that turn-based combat
// ignores the seconds and spends the actor's single action for the turn instead.
func CooldownLine(seconds float64) string {
	return fmt.Sprintf("RT Cooldown: %.2fs - TB: 1 action", seconds)
}

// ArmorInteractionLines spells out how a normal hit meets the target's defenses
// under the percentage armor model: armor mitigates physical up to its cap and
// non-physical damage up to a lower cap (diminishing returns), and it also meets
// Resistance; ranged physical shots can pierce armor. Universal/educational
// RULES -> DETAIL tier (full view only); the map editor renders full.
func ArmorInteractionLines(sec *CardSection, damageType string, isRanged, hasTrueDmg bool) {
	dt := strings.ToLower(strings.TrimSpace(damageType))
	school, err := damagecalc.ParseType(dt)
	if dt == "" || (err == nil && school == damagecalc.Physical) {
		sec.AddDetail("Reduced by target Armor (up to %d%%, diminishing)", ArmorPhysicalMitigationCap)
		if isRanged {
			sec.AddDetail("%d%% of shots pierce armor entirely", ArmorPierceRangedChancePct)
		}
	} else {
		sec.AddDetail("Reduced by target Armor (up to %d%%) and %s Resistance", ArmorElementalMitigationCap, config.TitleWords(dt))
	}
	if hasTrueDmg {
		school := config.TitleWords(dt)
		if dt == "" {
			school = "Physical"
		}
		sec.AddDetail("True Damage also bypasses flat reduction; %s Resistance still applies", school)
	}
}

// FilteredSpellEffectLines drops the EffectLines entries the unified template
// renders STRUCTURED elsewhere (the composed "X Damage - AoE" line and the
// decomposed DAMAGE/HEALING sections), so they don't appear twice.
func FilteredSpellEffectLines(sd spells.SpellDefinition) []string {
	return sd.CardEffectLines()
}

// FilteredItemEffectLines omits values rendered in the DEFENSE section.
func FilteredItemEffectLines(def *config.ItemDefinitionConfig) []string {
	if def == nil {
		return nil
	}
	return def.CoreEffectLines()
}

// WeaponStrikeFormulaLine describes the split after all Normal formula terms
// have been summed. True damage, crits and outgoing buffs are separate stages.
func WeaponStrikeFormulaLine(def *config.WeaponDefinitionConfig) string {
	if strikes := WeaponStrikeCount(def); strikes > 1 {
		return fmt.Sprintf("Per strike: divide Normal formula total by %d, round up", strikes)
	}
	return ""
}

type SpellRuleKind uint8

const (
	SpellRuleGeneral SpellRuleKind = iota
	// SpellRuleMasteryPolicy identifies skill rules; item tooltips show only
	// active contributions beside DAMAGE.
	SpellRuleMasteryPolicy
	// SpellRuleDodge lets the live tooltip describe the current packet: before
	// elemental GM it is fully dodgeable; once typed true damage exists, only
	// the normal component is avoided.
	SpellRuleDodge
	SpellRuleDamage
	SpellRuleCritical
	SpellRuleZone
	SpellRuleDuration
)

type SpellRule struct {
	Kind SpellRuleKind
	Text string
}

// SpellRules is the character-independent rules contract shared by the
// in-game spell tooltip and editor catalog. Rule kinds let the live card
// replace reference formulas with current values without duplicating the
// underlying applicability logic.
func SpellRules(def spells.SpellDefinition) []SpellRule {
	var out []SpellRule
	add := func(kind SpellRuleKind, format string, args ...any) {
		out = append(out, SpellRule{Kind: kind, Text: fmt.Sprintf(format, args...)})
	}

	school := config.TitleWords(def.School)
	switch {
	case def.PartyAoeRadiusTiles > 0 || def.MapWide:
		add(SpellRuleDamage, "All damage remains normal %s damage", strings.ToLower(school))
		add(SpellRuleDamage, "Enemy %s Resistance reduces damage", school)
		if !def.SparesParty {
			add(SpellRuleDamage, "Party %s Resistance reduces self-damage", school)
		}
		if MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleMasteryPolicy, "Elemental Mastery: ignores %d-%d%% of enemy %s Resistance",
				ElementalMasteryPiercePct(0), ElementalMasteryPiercePct(3), school)
		}
		add(SpellRuleCritical, "Cannot critically hit")
	case def.DealsNoDamage:
		// Control projectiles need to distinguish their effect from a damage hit.
		// Movement and summons have no direct attack whose crit needs explaining.
		if def.JumpTiles <= 0 && def.SummonMonster == "" {
			add(SpellRuleGeneral, "Deals no damage")
			add(SpellRuleCritical, "Cannot critically hit")
		}
	case def.IsProjectile || def.ZoneRadiusTiles > 0:
		add(SpellRuleDamage, "%s Resistance reduces damage", school)
		if MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleMasteryPolicy, "Elemental Mastery: ignores %d-%d%% of enemy %s Resistance",
				ElementalMasteryPiercePct(0), ElementalMasteryPiercePct(3), school)
			// A spell whose damage is an authored per-tier ladder gets no mastery
			// add-ons at all - the ladder IS the payload (same exclusion Inferno
			// earns through mastery_damage_per_tier), so no GM true-damage split.
			if len(def.DamageByMastery) != 4 {
				add(SpellRuleMasteryPolicy, "Grandmaster %s Magic: +%d %s true damage",
					school, int(MasteryGrandMaster)*MasterySpellEffectPerLevel, school)
			}
		} else {
			add(SpellRuleMasteryPolicy, "Grandmaster %s Magic: ignores %d%% of enemy %s Resistance",
				school, SelfMagicGMResistPiercePct, school)
		}
	}
	if def.AoeRadiusTiles > 0 {
		if def.MortarRangeTiles > 0 {
			add(SpellRuleCritical, "One critical roll boosts the entire bloom")
		} else {
			add(SpellRuleCritical, "%s", SplashCritRule)
		}
	}
	if def.IsProjectile && def.MortarRangeTiles <= 0 {
		if !def.DealsNoDamage && MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleDodge, "At Grandmaster, Perfect Dodge avoids normal damage; typed true damage still lands")
		} else {
			add(SpellRuleDodge, "Can be evaded by Perfect Dodge")
		}
	}
	if def.MortarRangeTiles > 0 {
		add(SpellRuleDamage, "The bloom cannot be evaded by Perfect Dodge")
	}
	if def.Pacify {
		add(SpellRuleGeneral, "Any received hit breaks the charm")
		add(SpellRuleGeneral, "No effect on undead")
	}
	if def.StatBonus > 0 || len(def.StatBonuses) > 0 {
		if def.StatBonusGrandmaster > def.StatBonus {
			add(SpellRuleDuration, "Mastery increases duration and the bonus")
		} else {
			add(SpellRuleDuration, "Mastery increases duration, not the bonus")
		}
		add(SpellRuleDuration, "Recasting refreshes the effect")
	}
	if def.ZoneRadiusTiles > 0 {
		add(SpellRuleZone, "Overlapping zones of the same spell do not stack")
	}
	return out
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
	}

	dmg := CardSection{Title: "DAMAGE"}
	if sd.IsProjectile && !sd.DealsNoDamage {
		dmg.Add("Deals the casting monster's attack damage")
		dmg.Add("Cannot critically hit")
	}

	effects := CardSection{Title: "EFFECTS"}
	dmg.Add("%s", DamageTypeAoELine(def.School, sd.AoeRadiusTiles))
	for _, ln := range FilteredSpellEffectLines(sd) {
		effects.Add("%s", ln)
	}

	casting.Add("Strikes your party, not other monsters")

	return []CardSection{dmg, effects, casting}
}
