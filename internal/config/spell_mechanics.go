package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	uitext "ugataima/assets/text"
)

// SpellMechanicConfig is the shared authored contract for advanced spell effects.
// Mastery ladders run from Novice to Grandmaster; durations are gameplay seconds.
type SpellMechanicConfig struct {
	ReapplyImmunitySeconds int     `yaml:"reapply_immunity_seconds,omitempty"`
	Kind                   string  `yaml:"kind"`
	PowerByMastery         []int   `yaml:"power_by_mastery,omitempty"`
	ChargesByMastery       []int   `yaml:"charges_by_mastery,omitempty"`
	DurationByMastery      []int   `yaml:"duration_by_mastery,omitempty"`
	RadiusTiles            float64 `yaml:"radius_tiles,omitempty"`
	ConeDegrees            float64 `yaml:"cone_degrees,omitempty"`
	MaxTargets             int     `yaml:"max_targets,omitempty"`
	ProjectilesByMastery   []int   `yaml:"projectiles_by_mastery,omitempty"`
	MaxHitsPerTarget       int     `yaml:"max_hits_per_target,omitempty"` // Per cast, shared by all projectiles.
	InstantKillChance      float64 `yaml:"instant_kill_chance,omitempty"` // Probability in 0..1, before fallback damage.
	Autocast               string  `yaml:"autocast,omitempty"`
	Technique              string  `yaml:"technique,omitempty"`
	DamageSchool           string  `yaml:"damage_school,omitempty"`
	SpellDuration          int     `yaml:"-"`
	Summary                string  `yaml:"summary"`
}

type spellMechanicRule struct {
	offensive, personal, party                                                   bool
	power, charges, duration, radius, cone, damage, percent, targets, projectile bool
}

var spellMechanicRules = map[string]spellMechanicRule{
	"fire_aura":             {personal: true, power: true, charges: true, percent: true},
	"fate":                  {personal: true, power: true, charges: true, percent: true},
	"vampiric_weapon":       {personal: true, power: true, duration: true, percent: true},
	"haste":                 {party: true},
	"shield":                {party: true, power: true, duration: true, percent: true},
	"regeneration":          {party: true, power: true, duration: true, percent: true},
	"pain_reflection":       {party: true, power: true, duration: true, percent: true},
	"preservation":          {party: true, charges: true},
	"protection_from_magic": {party: true, charges: true},
	"celestial_armor":       {party: true, power: true, duration: true},
	"acid_burst":            {offensive: true, power: true, duration: true, damage: true, projectile: true},
	"flying_fist":           {offensive: true, power: true, damage: true, projectile: true},
	"frozen_ground":         {offensive: true, duration: true, radius: true, cone: true},
	"slow":                  {offensive: true, power: true, duration: true, radius: true, percent: true},
	"turn_undead":           {offensive: true, duration: true, radius: true},
	"mass_fear":             {offensive: true, duration: true, radius: true},
	"feeblemind":            {offensive: true, duration: true, radius: true},
	"berserk":               {offensive: true, duration: true, radius: true, cone: true},
	"destroy_undead":        {offensive: true, radius: true, damage: true, cone: true},
	"prismatic_light":       {offensive: true, radius: true, damage: true, targets: true},
	"shrapmetal":            {offensive: true, cone: true, damage: true, projectile: true},
	"souldrinker":           {offensive: true, power: true, radius: true, damage: true, percent: true},
	"vvoid_beacon":          {charges: true}, "identify_monster": {radius: true}, "dispel_magic": {radius: true}, "purify": {}, "shared_life": {},
}

func (m *SpellMechanicConfig) IsOffensive() bool {
	return m != nil && spellMechanicRules[m.Kind].offensive
}
func (m *SpellMechanicConfig) IsPartyBuff() bool { return m != nil && spellMechanicRules[m.Kind].party }

// TargetsAlly identifies personal enchantments that require choosing a hero.
func (m *SpellMechanicConfig) TargetsAlly() bool {
	return m != nil && spellMechanicRules[m.Kind].personal
}

// spellMasteryValue reads an optional validated four-entry ladder.
func spellMasteryValue(ladder []int, tier int) int {
	if len(ladder) == 0 {
		return 0
	}
	return TierValue([4]int(ladder), tier)
}

// Power returns this effect's authored strength for the mastery tier.
func (m *SpellMechanicConfig) Power(tier int) int { return spellMasteryValue(m.PowerByMastery, tier) }

// Charges returns this effect's authored uses for the mastery tier.
func (m *SpellMechanicConfig) Charges(tier int) int {
	return spellMasteryValue(m.ChargesByMastery, tier)
}

// Projectiles returns the authored number of physical shots in one cast.
func (m *SpellMechanicConfig) Projectiles(tier int) int {
	return spellMasteryValue(m.ProjectilesByMastery, tier)
}

// Duration returns this effect's authored duration in gameplay seconds.
func (m *SpellMechanicConfig) Duration(tier int) int {
	return spellMasteryValue(m.DurationByMastery, tier)
}

// SummaryText substitutes the same authored values that production mechanics use.
// A negative tier shows the full mastery ladder for character-independent views.
func (m *SpellMechanicConfig) SummaryText(tier int) string {
	format := func(values []int) string {
		if tier >= 0 {
			return strconv.Itoa(spellMasteryValue(values, tier))
		}
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = strconv.Itoa(v)
		}
		return strings.Join(parts, "/")
	}
	text := strings.NewReplacer(
		"{spell_duration}", strconv.Itoa(m.SpellDuration),
		"{power}", format(m.PowerByMastery), "{charges}", format(m.ChargesByMastery),
		"{duration}", format(m.DurationByMastery),
		"{radius}", strconv.FormatFloat(m.RadiusTiles, 'f', -1, 64),
		"{cone}", strconv.FormatFloat(m.ConeDegrees, 'f', -1, 64),
		"{max_targets}", strconv.Itoa(m.MaxTargets),
		"{projectiles}", format(m.ProjectilesByMastery),
		"{max_hits_per_target}", strconv.Itoa(m.MaxHitsPerTarget),
		"{instant_kill_chance}", strconv.FormatFloat(m.InstantKillChance*100, 'f', -1, 64),
	).Replace(m.Summary)
	if GlobalConfig != nil && GlobalConfig.StatusEffects.ControlSaves.Applies(m.Kind) {
		text += " " + GlobalConfig.StatusEffects.ControlSaves.Summary
		if GlobalConfig.StatusEffects.ControlSaves.Diminishes(m.Kind) {
			text += " " + GlobalConfig.StatusEffects.ControlSaves.DiminishingSummary
		}
	}
	if len(m.DurationByMastery) > 0 {
		turns := make([]int, len(m.DurationByMastery))
		for i, seconds := range m.DurationByMastery {
			turns[i] = (seconds + CombatRoundSeconds - 1) / CombatRoundSeconds
		}
		text += uitext.Text("advanced.summary_tb_duration", format(turns))
	}
	if m.Kind == "slow" && GlobalConfig != nil {
		text += uitext.Text("advanced.summary_slow", SlowSkipEveryTurns())
	}
	if m.Kind == "regeneration" {
		text += uitext.Text("advanced.summary_regeneration", RegenerationIntervalFrames/DefaultTPS, RegenerationRounds)
	}
	if m.ReapplyImmunitySeconds > 0 {
		text += uitext.Text("advanced.summary_immunity", m.ReapplyImmunitySeconds, (m.ReapplyImmunitySeconds+CombatRoundSeconds-1)/CombatRoundSeconds)
	}
	if t := Technique(m.Technique); t != nil {
		if m.Kind == "haste" {
			turns := make([]int, 4)
			for i, seconds := range t.Duration {
				turns[i] = (seconds + CombatRoundSeconds - 1) / CombatRoundSeconds
			}
			text += uitext.Text("advanced.summary_haste", format(t.Power[:]), format(t.TBPower[:]), format(t.Duration[:]), format(turns))
		}
	}
	return text
}

func validateSpellMechanic(def *SpellDefinitionConfig) error {
	m := def.Mechanic
	if m == nil {
		return nil
	}
	m.SpellDuration = def.Duration
	rule, ok := spellMechanicRules[m.Kind]
	if !ok {
		return fmt.Errorf("unknown mechanic kind %q", m.Kind)
	}
	for name, values := range map[string][]int{
		"power_by_mastery": m.PowerByMastery, "charges_by_mastery": m.ChargesByMastery,
		"duration_by_mastery":    m.DurationByMastery,
		"projectiles_by_mastery": m.ProjectilesByMastery,
	} {
		if len(values) != 0 && len(values) != 4 {
			return fmt.Errorf("mechanic.%s requires four mastery entries", name)
		}
		for _, value := range values {
			if value <= 0 {
				return fmt.Errorf("mechanic.%s entries must be positive", name)
			}
		}
	}
	for name, value := range map[string]float64{"radius_tiles": m.RadiusTiles, "cone_degrees": m.ConeDegrees} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("mechanic.%s must be finite and nonnegative", name)
		}
	}
	if m.ConeDegrees > 360 {
		return fmt.Errorf("mechanic cone angle must be no greater than 360")
	}
	if math.IsNaN(m.InstantKillChance) || math.IsInf(m.InstantKillChance, 0) || m.InstantKillChance < 0 || m.InstantKillChance > 1 {
		return fmt.Errorf("mechanic.instant_kill_chance must be finite and within 0..1")
	}
	if m.InstantKillChance != 0 && m.Kind != "destroy_undead" {
		return fmt.Errorf("mechanic %s does not support instant_kill_chance", m.Kind)
	}
	if m.ReapplyImmunitySeconds < 0 || (m.Kind == "feeblemind") != (m.ReapplyImmunitySeconds > 0) {
		return fmt.Errorf("mechanic.reapply_immunity_seconds is required only for feeblemind")
	}
	if m.Summary == "" {
		return fmt.Errorf("mechanic.summary is required")
	}
	if m.DamageSchool != "" {
		if _, err := canonicalDamageSchool(m.DamageSchool); err != nil && m.DamageSchool != "physical" {
			return fmt.Errorf("mechanic.damage_school is invalid")
		}
	}
	switch m.Autocast {
	case "", "quickening", "purify", "dispel_magic", "celestial_armor":
	default:
		return fmt.Errorf("unknown mechanic.autocast %q", m.Autocast)
	}
	switch m.Technique {
	case "", "quickening", "purify":
	default:
		return fmt.Errorf("unsupported mechanic.technique %q", m.Technique)
	}
	if m.Autocast != "" && m.Autocast != m.Kind && !(m.Kind == "haste" && m.Autocast == "quickening") {
		return fmt.Errorf("mechanic.autocast does not match its behavior")
	}
	if (m.Kind == "haste" && m.Technique != "quickening") || ((m.Kind == "purify" || m.Kind == "dispel_magic") && m.Technique != "purify") {
		return fmt.Errorf("mechanic requires its shared technique")
	}
	power, charges, duration, radius, cone, damage, percent, targets := rule.power, rule.charges, rule.duration, rule.radius, rule.cone, rule.damage, rule.percent, rule.targets
	if rule.projectile && !def.IsProjectile {
		return fmt.Errorf("%s requires projectile physics", m.Kind)
	}
	if m.Kind == "destroy_undead" && m.InstantKillChance <= 0 {
		return fmt.Errorf("destroy_undead requires instant_kill_chance")
	}
	if m.Kind == "frozen_ground" && def.Duration <= 0 {
		return fmt.Errorf("frozen_ground requires a positive water-freeze duration")
	}
	if m.Kind == "shared_life" && (def.HealAmount <= 0 || !def.HealParty || def.TargetSelf) {
		return fmt.Errorf("shared_life requires heal_amount for its distributed healing pool")
	}
	for _, field := range []struct {
		name     string
		required bool
		present  bool
	}{
		{"power_by_mastery", power, len(m.PowerByMastery) == 4},
		{"charges_by_mastery", charges, len(m.ChargesByMastery) == 4},
		{"duration_by_mastery", duration, len(m.DurationByMastery) == 4},
		{"radius_tiles", radius, m.RadiusTiles > 0},
		{"cone_degrees", cone, m.ConeDegrees > 0},
		{"max_targets", targets, m.MaxTargets > 0},
		{"projectiles_by_mastery", m.Kind == "shrapmetal", len(m.ProjectilesByMastery) == 4},
		{"max_hits_per_target", m.Kind == "shrapmetal", m.MaxHitsPerTarget > 0},
		{"damage_school", damage || m.Kind == "fire_aura", m.DamageSchool != ""},
	} {
		if field.required != field.present {
			return fmt.Errorf("mechanic %s requires exactly its supported %s settings", m.Kind, field.name)
		}
	}
	if m.MaxTargets < 0 {
		return fmt.Errorf("mechanic.max_targets must not be negative")
	}
	if m.MaxHitsPerTarget < 0 {
		return fmt.Errorf("mechanic.max_hits_per_target must not be negative")
	}
	if damage && len(def.DamageByMastery) != 4 && def.DamageBase == 0 && def.DamageSource == "" && !def.IsProjectile {
		return fmt.Errorf("damaging mechanic requires a damage formula")
	}
	if percent {
		for _, value := range m.PowerByMastery {
			if value > 100 || m.Kind == "slow" && value == 100 {
				return fmt.Errorf("mechanic percentage exceeds its usable range")
			}
		}
	}
	if m.Technique != "" && m.Kind != "haste" && m.Kind != "purify" && m.Kind != "dispel_magic" {
		return fmt.Errorf("mechanic %s does not use a technique", m.Kind)
	}
	for _, field := range []struct {
		token     string
		available bool
	}{
		{"power", power}, {"charges", charges}, {"duration", duration}, {"radius", radius}, {"cone", cone}, {"max_targets", targets}, {"spell_duration", def.Duration > 0},
		{"instant_kill_chance", m.Kind == "destroy_undead"},
		{"projectiles", m.Kind == "shrapmetal"}, {"max_hits_per_target", m.Kind == "shrapmetal"},
	} {
		if strings.Contains(m.Summary, "{"+field.token+"}") && !field.available {
			return fmt.Errorf("mechanic.summary references absent %s", field.token)
		}
	}
	if strings.ContainsAny(m.SummaryText(-1), "{}") {
		return fmt.Errorf("mechanic.summary contains unknown placeholders")
	}
	return nil
}
