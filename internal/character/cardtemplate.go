package character

import (
	"fmt"
	"math"
	"slices"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/spells"
)

// Card sections and mechanic descriptions shared by game and editor tooltips.

// cardLine is one rendered line plus whether it's full-view-only.
type cardLine struct {
	text   string
	detail bool
	runs   CardText
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
	s.lines = append(s.lines, cardLine{fmt.Sprintf(format, args...), false, nil})
}

// AddDetail appends a full-view-only line (hidden in compact).
func (s *CardSection) AddDetail(format string, args ...interface{}) {
	s.lines = append(s.lines, cardLine{fmt.Sprintf(format, args...), true, nil})
}

// AddText appends an always-visible line with tinted runs.
func (s *CardSection) AddText(t CardText) {
	s.lines = append(s.lines, cardLine{t.String(), false, t})
}

// AddDetailText appends a full-view-only line with tinted runs.
func (s *CardSection) AddDetailText(t CardText) {
	s.lines = append(s.lines, cardLine{t.String(), true, t})
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

// RenderCardRows flattens nonempty sections with results before details.
// Compact cards omit details; every row retains its mechanic and semantic role.
func RenderCardRows(sections []CardSection, full bool) CardRows {
	var out CardRows
	for _, sec := range sections {
		var rows CardRows
		for _, detail := range []bool{false, true} {
			if detail && !full {
				continue
			}
			for _, line := range sec.lines {
				if line.detail != detail {
					continue
				}
				kind := CardRowResult
				if detail {
					kind = CardRowDetail
				}
				start := len(rows)
				rows.Add(kind, line.text)
				for i := start; i < len(rows); i++ {
					rows[i].Section = sec.Title
				}
				if line.runs != nil && len(rows) == start+1 {
					rows[start].Runs = line.runs
				}
			}
		}
		if len(rows) == 0 {
			continue
		}
		if len(out) > 0 {
			out.Add(CardRowSpacer, "")
		}
		out.Add(CardRowSection, sec.Title)
		out = append(out, rows...)
	}
	return out
}

// RenderCardLines is the plain-text projection for non-rendering consumers.
func RenderCardLines(sections []CardSection, full bool) []string {
	return RenderCardRows(sections, full).Lines()
}

// DamageTypeAoE composes "Fire Damage - splash radius 2 tiles" (any element),
// the school named in its tint.
func DamageTypeAoE(damageType string, aoeTiles float64) CardText {
	return damageTypesAoE([]string{damageType}, aoeTiles)
}

// HitDamageTypeAoE is the type line of a hit split into normal shares:
// damageType's while a share of it remains, else every school the shares
// carry ("Fire Damage + Light Damage"), as when cards convert all of it.
func HitDamageTypeAoE(damageType string, shares []DamageShare, aoeTiles float64) CardText {
	shares = MergeDamageShares(shares...) // only shares that carry damage name a school
	schools := []string{damageType}
	if len(shares) > 0 && !slices.ContainsFunc(shares, func(share DamageShare) bool { return SameSchool(share.School, damageType) }) {
		schools = schools[:0]
		for _, share := range shares {
			schools = append(schools, share.School)
		}
	}
	return damageTypesAoE(schools, aoeTiles)
}

func damageTypesAoE(schools []string, aoeTiles float64) CardText {
	pieces := make([]CardRun, len(schools))
	for i, dt := range schools {
		if dt == "" {
			dt = damagecalc.Physical.String()
		}
		pieces[i] = SchoolRun(dt, uitext.Text("damage.school", config.TitleWords(dt)))
	}
	line := shareList(pieces)
	if aoeTiles > 0 {
		line = append(line, CardRun{Text: uitext.Text("damage.splash_radius", aoeTiles)})
	}
	return line
}

// Damage shares as cards name them: each school's normal share once, as the
// combat log names them, in that school's tint (physical damage is an
// untinted "Normal"); true damage keeps the word True in the hit's school's
// tint, where the log draws all true damage in its own color.

// physicalSchool: "" is a weapon's default, physical damage.
func physicalSchool(school string) bool {
	return SameSchool(school, damagecalc.Physical.String())
}

// SameSchool reports whether two school names are one damage school, read as
// a hit's packet reads them: case-blind, an empty or unknown name physical.
func SameSchool(a, b string) bool {
	ta, _ := damagecalc.ParseType(a)
	tb, _ := damagecalc.ParseType(b)
	return ta == tb
}

// SchoolRun is text in a damage school's tint; physical damage keeps the
// row's own color.
func SchoolRun(school, text string) CardRun {
	if physicalSchool(school) {
		return CardRun{Text: text}
	}
	return CardRun{Text: text, School: school}
}

// DamageShareName names a hit's non-true share: its school, or Normal.
func DamageShareName(school string) string {
	if physicalSchool(school) {
		return uitext.Text("damage.normal_share")
	}
	return config.TitleWords(school)
}

// DamageShare is one school's normal damage in a hit.
type DamageShare struct {
	School string
	Amount int
}

// MergeDamageShares is the shares a card names, one per school as in the
// combat log: empty ones dropped, each school's summed, in first-seen order.
func MergeDamageShares(shares ...DamageShare) []DamageShare {
	var out []DamageShare
	for _, share := range shares {
		if share.Amount <= 0 {
			continue
		}
		if physicalSchool(share.School) {
			share.School = damagecalc.Physical.String()
		}
		merged := false
		for i := range out {
			if SameSchool(out[i].School, share.School) {
				out[i].Amount += share.Amount
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, share)
		}
	}
	return out
}

// SumDamageShares is the shares' damage together.
func SumDamageShares(shares []DamageShare) int {
	total := 0
	for _, share := range shares {
		total += share.Amount
	}
	return total
}

// shareRun is a share's number and name, which a wrapped line keeps together.
func shareRun(school, text string) CardRun {
	run := SchoolRun(school, text)
	run.Whole = true
	return run
}

// shareList joins pieces, each kept whole, with the share separator; a
// wrapped list breaks only after a separator.
func shareList(pieces []CardRun) CardText {
	separator := uitext.Text("damage.share_separator")
	glue := strings.TrimRight(separator, " ")
	var list CardText
	for _, piece := range pieces {
		if len(list) > 0 {
			list = append(list, CardRun{Text: glue, Whole: true})
			if gap := separator[len(glue):]; gap != "" {
				list = append(list, CardRun{Text: gap})
			}
		}
		piece.Whole = true
		list = append(list, piece)
	}
	return list
}

// DamageSplit is "Total Damage: 33 (13 + 11 Fire + 9 True)": each normal
// share, then the true damage in trueSchool's tint. Beside an elemental share
// the physical one is a bare number, as the combat log writes it; alone it is
// "13 Normal".
func DamageSplit(label string, shares []DamageShare, trueSchool string, trueDamage int) CardText {
	elemental := slices.ContainsFunc(shares, func(share DamageShare) bool { return !physicalSchool(share.School) })
	pieces := make([]CardRun, 0, len(shares)+1)
	for _, share := range shares {
		text := uitext.Text("damage.share", share.Amount, DamageShareName(share.School))
		if elemental && physicalSchool(share.School) {
			text = uitext.Text("damage.unnamed_share", share.Amount)
		}
		pieces = append(pieces, SchoolRun(share.School, text))
	}
	if trueDamage > 0 {
		pieces = append(pieces, SchoolRun(trueSchool, uitext.Text("damage.true_share", trueDamage)))
	}
	total := trueDamage + SumDamageShares(shares)
	return StyledText(uitext.Text("damage.total_split", label, total, uitext.Slot), shareList(pieces))
}

// DamageShareTotal is one normal share on its own: "Fire Damage: 14".
func DamageShareTotal(school string, n int) CardText {
	return CardText{SchoolRun(school, uitext.Text("damage.share_total", DamageShareName(school), n))}
}

// SchoolBuff is what the party's buffs on one school add to a hit: "Fire buff: +24".
func SchoolBuff(school string, n int) CardText {
	return CardText{SchoolRun(school, uitext.Text("damage.school_buff", config.TitleWords(school), n))}
}

// TrueBonus is one source's part of the true share: "+9 True".
func TrueBonus(school string, n int) CardText {
	return CardText{shareRun(school, uitext.Text("damage.true_bonus", n))}
}

// TrueBonusNamed is a true part that names its school: "+15 Fire True Damage".
func TrueBonusNamed(school string, n int) CardText {
	return CardText{shareRun(school, uitext.Text("damage.true_bonus_school", n, config.TitleWords(school)))}
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
		shape = uitext.Text("weapon.arc_front")
	case 2:
		shape = uitext.Text("weapon.arc_flank")
	case 3:
		shape = uitext.Text("weapon.arc_diagonals")
	case 4:
		shape = uitext.Text("weapon.arc_sides")
	default:
		return ""
	}
	// Reach depth only: which directions get hit is the shape sentence's job,
	// and depth counts diagonals as one step for EVERY weapon, so a per-item
	// "diagonals included" note carried no information.
	reach := uitext.Text("weapon.reach_one")
	if def.Range >= 2 {
		reach = uitext.Text("weapon.reach_cone", def.Range)
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
		return uitext.Text("weapon.arc_short_front")
	case 2:
		return uitext.Text("weapon.arc_short_flank")
	case 3:
		return uitext.Text("weapon.arc_short_diagonals")
	case 4:
		return uitext.Text("weapon.arc_short_sides")
	default:
		return ""
	}
}

// SplashCritRule describes the shared source roll. Target-specific bonuses
// such as Designate Target are resolved separately for each victim.
func SplashCritRule() string { return uitext.Text("spell.splash_critical") }

func WeaponSplashCritRule() string {
	return SplashCritRule() + " " + uitext.Text("weapon.splash_designation")
}

// CardSectionSet titles the equipment-set block of a card.
const CardSectionSet = "SET"

// CooldownLine formats real-time recovery and the turn-based action cost.
func CooldownLine(seconds float64) string {
	return CooldownLineTB(seconds, uitext.Text("casting.one_action"))
}

// CooldownLineTB is CooldownLine with its own turn-based cost.
func CooldownLineTB(seconds float64, tb string) string {
	return uitext.Text("casting.cooldown", CardSeconds(seconds), tb)
}

// SpellTBCost is what one cast costs in turn-based combat: a jump is party
// movement, so it ends the turn like a step.
func SpellTBCost(def spells.SpellDefinition) string {
	if def.JumpTiles > 0 {
		return uitext.Text("casting.ends_turn")
	}
	return uitext.Text("casting.one_action")
}

// CardSeconds rounds a duration to the hundredths every card line shows, half
// up, so a breakdown's stages add up to its total.
func CardSeconds(seconds float64) float64 {
	return math.Round(seconds*100) / 100
}

// ArmorInteractionLines spells out how a normal hit meets the target's defenses
// under the percentage armor model: armor mitigates physical up to its cap and
// non-physical damage up to a lower cap (diminishing returns), and it also meets
// Resistance; ranged physical shots can pierce armor. Universal/educational
// RULES -> DETAIL tier (full view only); the map editor renders full.
// Each school among the hit's normal shares that carry damage gets its line,
// damageType's when none do; the true damage line keeps damageType.
func ArmorInteractionLines(sec *CardSection, damageType string, shares []DamageShare, isRanged, hasTrueDmg bool) {
	shares = MergeDamageShares(shares...)
	dt := strings.ToLower(strings.TrimSpace(damageType))
	schools := []string{dt}
	if len(shares) > 0 {
		schools = schools[:0]
		for _, share := range shares {
			schools = append(schools, strings.ToLower(share.School))
		}
	}
	for _, name := range schools {
		school, err := damagecalc.ParseType(name)
		if name == "" || (err == nil && school == damagecalc.Physical) {
			sec.AddDetail("%s", uitext.Text("armor.physical_reduction", config.ArmorPhysicalMitigationCap))
			if isRanged {
				sec.AddDetail("%s", ArmorPierceShotsLine())
			}
			continue
		}
		sec.AddDetail("%s", uitext.Text("armor.elemental_reduction", config.ArmorElementalMitigationCap, config.TitleWords(name)))
	}
	if hasTrueDmg {
		school := config.TitleWords(dt)
		if dt == "" {
			school = "Physical"
		}
		sec.AddDetail("%s", uitext.Text("armor.true_resistance", school))
	}
}

// FilteredSpellEffectLines omits the rows the unified template renders
// STRUCTURED elsewhere (the composed "X Damage - AoE" line and the decomposed
// DAMAGE/HEALING sections), so they don't appear twice.
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
		return uitext.Text("weapon.split_damage", strikes)
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
	add := func(kind SpellRuleKind, text string) {
		out = append(out, SpellRule{Kind: kind, Text: text})
	}

	school := config.TitleWords(def.DamageType())
	magicSchool := config.TitleWords(def.School)
	if def.Mechanic != nil && config.GlobalConfig != nil && config.GlobalConfig.StatusEffects.ControlSaves.Applies(def.Mechanic.Kind) {
		add(SpellRuleGeneral, config.GlobalConfig.StatusEffects.ControlSaves.Details)
	}
	switch {
	case def.PartyAoeRadiusTiles > 0 || def.MapWide:
		add(SpellRuleDamage, uitext.Text("spell.area_mitigation", config.ArmorElementalMitigationCap, school))
		if !def.SparesParty {
			add(SpellRuleDamage, uitext.Text("spell.self_mitigation", school))
		}
		if MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleMasteryPolicy, uitext.Text("spell.elemental_mastery_pierce", ElementalMasteryPiercePct(0), ElementalMasteryPiercePct(3), school))
		}
		add(SpellRuleCritical, uitext.Text("spell.no_critical"))
	case def.DealsNoDamage:
		// Control projectiles need to distinguish their effect from a damage hit.
		// Movement and summons need no separate damage disclaimer.
		if def.JumpTiles <= 0 && def.SummonMonster == "" {
			add(SpellRuleGeneral, uitext.Text("spell.no_damage"))
		}
	case def.IsProjectile || def.ZoneRadiusTiles > 0:
		cap := config.ArmorElementalMitigationCap
		physical := def.DamageType() == "physical"
		if physical {
			cap = config.ArmorPhysicalMitigationCap
		}
		add(SpellRuleDamage, uitext.Text("armor.elemental_reduction", cap, school))
		if physical && def.IsProjectile {
			add(SpellRuleDamage, ArmorPierceShotsLine()) // every physical projectile pierces alike
		}
		if MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleMasteryPolicy, uitext.Text("spell.elemental_mastery_pierce", ElementalMasteryPiercePct(0), ElementalMasteryPiercePct(3), school))
			// A spell whose damage is an authored per-tier ladder gets no mastery
			// add-ons at all - the ladder IS the payload (same exclusion Inferno
			// earns through mastery_damage_per_tier), so no GM true-damage split.
			if len(def.DamageByMastery) != 4 {
				add(SpellRuleMasteryPolicy, uitext.Text("spell.elemental_grandmaster_damage", magicSchool, int(MasteryGrandMaster)*MasterySpellEffectPerLevel, school))
			}
		} else {
			add(SpellRuleMasteryPolicy, uitext.Text("spell.self_grandmaster_pierce", magicSchool, SelfMagicGMResistPiercePct, school))
		}
	}
	if def.AoeRadiusTiles > 0 {
		if def.MortarRangeTiles > 0 {
			add(SpellRuleCritical, uitext.Text("spell.bloom_critical"))
		} else {
			add(SpellRuleCritical, SplashCritRule())
			add(SpellRuleDamage, uitext.Text("spell.splash_obstacles"))
			add(SpellRuleDamage, uitext.Text("spell.splash_dodge"))
		}
	}
	if def.IsProjectile && def.MortarRangeTiles <= 0 {
		if !def.DealsNoDamage && MagicSchoolID(def.School).IsElemental() {
			add(SpellRuleDodge, uitext.Text("spell.gm_dodge"))
		} else {
			add(SpellRuleDodge, uitext.Text("spell.normal_dodge"))
		}
		if def.StunChance > 0 {
			add(SpellRuleGeneral, uitext.Text("spell.dodged_stun"))
		}
	}
	if def.MortarRangeTiles > 0 {
		add(SpellRuleDamage, uitext.Text("spell.bloom_dodge"))
	}
	if def.GrantsStatBuff() {
		durationGrows := len(def.DurationByMastery) == 4 && def.DurationByMastery[3] > def.DurationByMastery[0]
		if durationGrows && def.StatBonusGrandmaster > def.StatBonus {
			add(SpellRuleDuration, uitext.Text("spell.mastery_duration_and_bonus"))
		} else if durationGrows {
			add(SpellRuleDuration, uitext.Text("spell.mastery_duration_only"))
		}
	}
	// A handled no-op keeps the SP but spends the turn and cooldown.
	switch {
	case def.Awaken:
		add(SpellRuleGeneral, uitext.Text("spell.awaken_no_target"))
	case def.Revive || def.ReviveHpPct > 0:
		add(SpellRuleGeneral, uitext.Text("spell.revive_no_target"))
	case def.JumpTiles > 0:
		add(SpellRuleGeneral, uitext.Text("spell.jump_blocked"))
	}
	if def.ZoneRadiusTiles > 0 {
		add(SpellRuleZone, uitext.Text("spell.zone_no_stacking"))
	}
	return out
}

// MonsterSpellCardSections renders a MONSTER-ONLY spell. Monsters cast these
// with their OWN attack damage (combat.go spawnMonsterSpellProjectile) - no SP
// cost, no Intellect/mastery scaling, no crit - so the player-formula card would
// be a fiction. Disintegrate / AoE / stun riders still fire, so the effect lines
// stay.
func MonsterSpellCardSections(def *config.SpellDefinitionConfig, sd spells.SpellDefinition) []CardSection {
	casting := CardSection{Title: "CASTING"}
	casting.Add("%s", uitext.Text("spell.monster_only"))
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
		dmg.Add("%s", uitext.Text("spell.monster_damage"))
		dmg.Add("%s", uitext.Text("spell.no_critical"))
	}

	effects := CardSection{Title: "EFFECTS"}
	dmg.AddText(DamageTypeAoE(sd.DamageType(), sd.AoeRadiusTiles))
	// Against the party a disintegrate roll eradicates the hero it hits; the
	// monster-target immunities do not apply.
	if sd.DisintegrateChance > 0 {
		effects.Add("%s", uitext.Text("spell.monster_disintegrate", sd.DisintegrateChance*100))
		sd.DisintegrateChance = 0
	}
	for _, ln := range FilteredSpellEffectLines(sd) {
		effects.Add("%s", ln)
	}

	casting.Add("%s", uitext.Text("spell.monster_targets"))

	return []CardSection{dmg, effects, casting}
}
