package config

import (
	"fmt"
	"reflect"

	uitext "ugataima/assets/text"
)

// CardRule is a card effect whose duplicates never stack: among the active
// cards the copy with the highest Strength applies its whole rule.
type CardRule struct {
	Fields   string // its YAML fields, for load errors
	Strength func(*ItemDefinitionConfig) int
	Copy     func(dst, src *ItemDefinitionConfig) // the rule's fields only
	Validate func(*ItemDefinitionConfig) bool
	Lines    func(*ItemDefinitionConfig) []string
}

var (
	CardWildfire = CardRule{
		Fields:   "card_wildfire_hp_pct, card_wildfire_radius_tiles, card_wildfire_burn_seconds, card_wildfire_school",
		Strength: func(d *ItemDefinitionConfig) int { return d.CardWildfireHPPct },
		Copy: func(dst, src *ItemDefinitionConfig) {
			dst.CardWildfireHPPct, dst.CardWildfireRadiusTiles, dst.CardWildfireBurnSeconds, dst.CardWildfireSchool = src.CardWildfireHPPct, src.CardWildfireRadiusTiles, src.CardWildfireBurnSeconds, src.CardWildfireSchool
		},
		Validate: func(d *ItemDefinitionConfig) bool {
			return validPercent(d.CardWildfireHPPct) && positive(d.CardWildfireRadiusTiles) && validDamageSchool(d.CardWildfireSchool) &&
				ValidateEffectSeconds("card_wildfire_burn_seconds", d.CardWildfireBurnSeconds) == nil
		},
		Lines: func(d *ItemDefinitionConfig) []string {
			return []string{uitext.Text("item.card_wildfire", d.CardWildfireHPPct, d.CardWildfireSchool, NumberText(d.CardWildfireRadiusTiles), DurationText(d.CardWildfireBurnSeconds))}
		},
	}
	CardOverflow = CardRule{
		Fields:   "card_overflow_pct, card_overflow_cap_pct",
		Strength: func(d *ItemDefinitionConfig) int { return d.CardOverflowPct },
		Copy: func(dst, src *ItemDefinitionConfig) {
			dst.CardOverflowPct, dst.CardOverflowCapPct = src.CardOverflowPct, src.CardOverflowCapPct
		},
		Validate: func(d *ItemDefinitionConfig) bool {
			return validPercent(d.CardOverflowPct) && validPercent(d.CardOverflowCapPct)
		},
		Lines: func(d *ItemDefinitionConfig) []string {
			return []string{uitext.Text("item.card_overflow", d.CardOverflowPct, d.CardOverflowCapPct)}
		},
	}
	CardCrumbling = CardRule{
		Fields:   "card_crumble_armor, card_crumble_cap_pct",
		Strength: func(d *ItemDefinitionConfig) int { return d.CardCrumbleArmor },
		Copy: func(dst, src *ItemDefinitionConfig) {
			dst.CardCrumbleArmor, dst.CardCrumbleCapPct = src.CardCrumbleArmor, src.CardCrumbleCapPct
		},
		Validate: func(d *ItemDefinitionConfig) bool { return d.CardCrumbleArmor > 0 && validPercent(d.CardCrumbleCapPct) },
		Lines: func(d *ItemDefinitionConfig) []string {
			return []string{uitext.Text("item.card_crumbling", d.CardCrumbleArmor, d.CardCrumbleCapPct)}
		},
	}
	CardStormReversal = CardRule{
		Fields:   "card_storm_reversal_pct, card_storm_reversal_school",
		Strength: func(d *ItemDefinitionConfig) int { return d.CardStormReversalPct },
		Copy: func(dst, src *ItemDefinitionConfig) {
			dst.CardStormReversalPct, dst.CardStormReversalSchool = src.CardStormReversalPct, src.CardStormReversalSchool
		},
		Validate: func(d *ItemDefinitionConfig) bool {
			return validPercent(d.CardStormReversalPct) && validDamageSchool(d.CardStormReversalSchool)
		},
		Lines: func(d *ItemDefinitionConfig) []string {
			return []string{uitext.Text("item.card_storm_reversal", d.CardStormReversalPct, d.CardStormReversalSchool)}
		},
	}
	// CardRules lists every non-stacking card rule in text order.
	CardRules = []CardRule{CardWildfire, CardOverflow, CardCrumbling, CardStormReversal}
)

// authors reports whether d sets any field of the rule.
func (r CardRule) authors(d *ItemDefinitionConfig) bool {
	var probe ItemDefinitionConfig
	r.Copy(&probe, d)
	return !reflect.ValueOf(probe).IsZero()
}

// CardRuleLines are the non-stacking rules d authors, each with its
// no-stacking note.
func (d *ItemDefinitionConfig) CardRuleLines() []string {
	var lines []string
	for _, r := range CardRules {
		if r.Strength(d) > 0 {
			lines = append(append(lines, r.Lines(d)...), uitext.Text("item.boss_card_duplicates"))
		}
	}
	return lines
}

func (d *ItemDefinitionConfig) validateCardRules() error {
	for _, r := range CardRules {
		if !r.authors(d) {
			continue
		}
		if d.Type != "card" || !r.Validate(d) {
			return fmt.Errorf("%s need type card and every field in range", r.Fields)
		}
	}
	return nil
}

// CrumblingArmorCapPercent is the highest erosion cap any card authors, 0
// when none does: the bound saved erosion is repaired against.
func CrumblingArmorCapPercent() int {
	capPercent := 0
	if GlobalItems != nil {
		for _, d := range GlobalItems.Items {
			capPercent = max(capPercent, d.CardCrumbleCapPct)
		}
	}
	return capPercent
}

func validPercent(value int) bool { return value >= 1 && value <= 100 }

// validDamageSchool reports a canonical damage school key, as YAML authors it.
func validDamageSchool(school string) bool {
	canonical, err := canonicalDamageSchool(school)
	return err == nil && canonical == school
}
