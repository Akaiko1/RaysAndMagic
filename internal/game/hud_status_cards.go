package game

import (
	"cmp"
	"image/color"
	"strconv"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/stats"
)

var (
	hudCardReadyColor   = color.RGBA{150, 220, 140, 255}
	hudCardBlockedColor = color.RGBA{236, 111, 95, 255}
)

// buffStatusCard is the status-rail hover card: the running effect with its
// live magnitudes, the time left on both clocks, and how to end it.
func (g *MMGame) buffStatusCard(s *UtilitySpellStatus) ([]string, color.Color) {
	rows, plate := g.buffStatusCardRows(s)
	return rows.Lines(), plate
}

func (g *MMGame) buffStatusCardRows(s *UtilitySpellStatus) (rows character.CardRows, plate color.Color) {
	id := string(s.SpellID)
	name, subtitle := s.Label, ""
	plate = schoolPlateColor("")
	var mechanics []string
	if def, err := spells.GetSpellDefinitionByID(s.SpellID); err == nil {
		name = cmp.Or(name, def.Name)
		subtitle = spellSchoolsLabel(def) + " Magic"
		plate = schoolPlateColor(def.School)
		mechanics = def.BuffMechanicLines()
	} else if d := config.Technique(id); d != nil {
		name = cmp.Or(name, d.Name)
		subtitle = uitext.Text("buff.source_technique")
	} else if def, ok := config.GetItemDefinition(id); ok && def.HasTimedBuff() {
		name = cmp.Or(name, def.Name)
		subtitle = uitext.Text("buff.source_draught")
	}
	name = cmp.Or(name, spellDisplayName(s.SpellID))
	providence := g.phaseOwnsBuff(id)
	if providence {
		subtitle = uitext.Text("buff.source_providence")
	}

	effects := ttSection{Title: "EFFECTS"}
	for _, line := range append(g.activeBuffEffectLines(id), mechanics...) {
		effects.Add("%s", line)
	}
	duration := ttSection{Title: "DURATION"}
	if providence {
		boundary := "dusk"
		if g.dayNightIsNight {
			boundary = "dawn"
		}
		duration.Add("%s", uitext.Text("buff.until_phase", boundary))
	} else {
		duration.Add("%s", uitext.Text("buff.remaining", g.buffRemainingText(s.Duration)))
	}
	sections := []ttSection{effects, duration}
	if !providence {
		usage := ttSection{Title: "USAGE"}
		usage.Add("%s", uitext.Text("buff.double_click_dispel"))
		sections = append(sections, usage)
	}
	return renderTooltipRows(name, subtitle, sections, true), plate
}

// activeBuffEffectLines states what the running buff grants with the values it
// was cast with (mastery included), in the wording of the card that grants it.
func (g *MMGame) activeBuffEffectLines(id string) []string {
	if def, ok := config.GetItemDefinition(id); ok && def.HasTimedBuff() {
		return def.TimedBuffEffectLines() // the HUD owns the remaining duration
	}
	var out []string
	if b, ok := g.statBuffByID(id); ok {
		out = append(out, statBonusLines(b.Bonuses)...)
	}
	b, ok := g.combatBuffByID(id)
	if !ok {
		return out
	}
	if b.OutBonus > 0 {
		out = append(out, uitext.Text("spell.party_deal_damage", spells.OutgoingDamageTarget(b.OutDamageType), b.OutBonus))
	}
	if b.ResistPct > 0 {
		out = append(out, uitext.Text("spell.party_takes_less_damage", b.ResistPct))
	}
	if b.InReduce > 0 {
		out = append(out, uitext.Text("spell.party_takes_damage_per_hit", b.InReduce))
	}
	if b.ResistSchoolPct > 0 && b.ResistSchool != "" {
		out = append(out, uitext.Text("spell.party_resists_for_the_duration", config.TitleWords(b.ResistSchool), b.ResistSchoolPct))
	}
	if b.DodgePct > 0 {
		out = append(out, uitext.Text("buff.dodge", strconv.Itoa(b.DodgePct)))
	}
	if b.RecoveryPct > 0 {
		out = append(out, uitext.Text("buff.rt_recovery", strconv.Itoa(b.RecoveryPct)))
	}
	if b.ExtraActions > 0 {
		out = append(out, uitext.Text("buff.tb_pool", strconv.Itoa(b.ExtraActions)))
	}
	return out
}

// statBonusLines folds a uniform bonus into one line, otherwise one per stat.
func statBonusLines(b stats.StatBonuses) []string {
	if !b.IsZero() && b == stats.Uniform(b.Might) {
		return []string{uitext.Text("spell.to_all_stats_whole_party", b.Might)}
	}
	return spells.PerStatBonusLines(b.ToMap())
}

func (g *MMGame) buffRemainingText(frames int) string {
	tps := max(1, g.config.GetTPS())
	seconds := (max(0, frames) + tps - 1) / tps
	return cardDurationText(strconv.Itoa(seconds), pluralizeCount(tbRoundsForSeconds(seconds), "round", "rounds"))
}

// campStatusCard is the camp button's hover card. Its last line says whether
// the party can camp now, from the same rule TryCamp enforces.
func (g *MMGame) campStatusCard() ([]string, []color.Color) {
	rows, colors := g.campStatusCardRows()
	return rows.Lines(), colors
}

func (g *MMGame) campStatusCardRows() (character.CardRows, []color.Color) {
	effects := ttSection{Title: "EFFECTS"}
	effects.Add("%s", uitext.Text("ui.camp_restores"))
	effects.Add("%s", uitext.Text("ui.camp_cures"))
	effects.Add("%s", uitext.Text("ui.camp_dead_stay"))
	usage := ttSection{Title: "USAGE"}
	usage.Add("%s", uitext.Text("ui.camp_cost", CampFoodCost, max(0, g.party.Food)))
	usage.Add("%s", uitext.Text("ui.camp_rule", CampEnemyRadiusTiles))
	lines := renderTooltipRows(uitext.Text("ui.camp"), "", []ttSection{effects, usage}, true)
	status, ink := uitext.Text("ui.camp_click"), hudCardReadyColor
	if reason := g.campBlocked(); reason != "" {
		status, ink = reason, hudCardBlockedColor
	}
	lines.Add(character.CardRowSpacer, "")
	lines.Add(character.CardRowResult, status)
	colors := make([]color.Color, len(lines))
	colors[len(colors)-1] = ink
	return lines, colors
}
