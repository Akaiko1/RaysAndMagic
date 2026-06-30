package spells

import (
	"fmt"
	"strings"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SpellID represents dynamic spell identifiers loaded from YAML
type SpellID string

// String returns the string representation of a spell ID
func (s SpellID) String() string {
	return string(s)
}

// SpellDefinition represents the complete definition of a spell loaded from YAML
type SpellDefinition struct {
	ID                   SpellID
	Name                 string
	Description          string
	School               string
	Level                int // Spell level (1-9)
	SpellPointsCost      int
	CooldownSeconds      float64 // RT cast cooldown (seconds) at reference Speed; 0 = derive from Level
	Duration             int     // Duration in seconds (0 for instant spells)
	DisintegrateChance   float64
	AoeRadiusTiles       float64 // 0 = single-target; >0 = splash radius in tiles
	ProjectileSize       int
	IsProjectile         bool
	IsUtility            bool
	StatusIcon           string
	StatBonus            int            // Uniform stat bonus for buff spells like Bless
	StatBonusGrandmaster int            // optional GM-scaled uniform stat bonus cap
	StatBonuses          map[string]int // Per-stat alternative (lowercase stat keys)
	// Damage-formula modifiers (default behaviour when zero/false)
	DamageCostMultiplier  int  // base = cost × SpellDamagePerSP × this (default 1)
	ScalesWithPersonality bool // also add Personality/divisor to spell damage
	// AoE-stun effect (Darkness): >0 radius stuns all monsters in range, no damage
	StunRadiusTiles     float64
	StunDurationSeconds int
	StunDurationTurns   int
	DealsNoDamage       bool // zero direct damage (Disintegrate: only the instakill roll matters)
	// Party combat buffs (duration seconds)
	ResistBuffPct                      int // Day of the Gods: % incoming damage reduction
	ResistBuffPctGrandmaster           int // optional GM-scaled % reduction cap
	OutgoingDamageBonus                int // Hour of Power: flat outgoing damage bonus
	OutgoingDamageBonusGrandmaster     int // optional GM-scaled outgoing damage cap
	OutgoingDamageType                 string
	IncomingDamageReduction            int // base flat incoming damage reduction
	IncomingDamageReductionGrandmaster int // optional GM-scaled flat reduction cap
	// Bind Undead and Charm are two DISTINCT control spells (never mixed):
	BindUndead            bool    // Bind Undead: take control of an UNDEAD target — it hunts other monsters for you
	BindDurationSeconds   int     // Bind Undead duration (RT seconds)
	Pacify                bool    // Charm: pacify a LIVING target — it stops attacking; breaks on any hit it takes
	PacifyDurationSeconds int     // Charm duration (RT seconds)
	Revive                bool    // resurrect: restore a fallen ally (incl. eradicated)
	FullHeal              bool    // resurrect: restore to maximum HP
	ReviveHpPct           int     // Raise Dead: revive a fallen ally (not eradicated) to this % of max HP
	HealParty             bool    // Mass Heal: heal every party member
	StunChance            float64 // Psychic Shock: chance to stun the struck monster on hit
	// Party-centered instant nova (Inferno): damages all monsters AND the party in radius.
	PartyAoeRadiusTiles float64
	// Persistent damage zone (Hot Steam).
	StarburstFx     bool
	ZoneRadiusTiles float64
	ZoneTickDamage  int
	ZoneTickSeconds float64
	// Effect configuration
	HealAmount        int     // For healing spells
	VisionRadiusTiles float64 // vision spells: torch glow / wizard-eye radar radius (tiles)
	TargetSelf        bool    // Whether spell targets self or others
	Awaken            bool    // For awaken spell
	WaterWalk         bool    // For water walking spell
	WaterBreathing    bool    // For water breathing spell
	Message           string  // Effect message to display
}

// GetSpellDefinitionByID retrieves spell definition from YAML config
func GetSpellDefinitionByID(spellID SpellID) (SpellDefinition, error) {
	configDef, exists := config.GetSpellDefinition(string(spellID))
	if !exists {
		return SpellDefinition{}, fmt.Errorf("spell '%s' not found in spells.yaml", spellID)
	}

	return SpellDefinition{
		ID:                                 spellID,
		Name:                               configDef.Name,
		Description:                        configDef.Description,
		School:                             configDef.School,
		Level:                              configDef.Level,
		SpellPointsCost:                    configDef.SpellPointsCost,
		CooldownSeconds:                    configDef.CooldownSeconds,
		Duration:                           configDef.Duration,
		DisintegrateChance:                 configDef.DisintegrateChance,
		AoeRadiusTiles:                     configDef.AoeRadiusTiles,
		ProjectileSize:                     configDef.ProjectileSize,
		IsProjectile:                       configDef.IsProjectile,
		IsUtility:                          configDef.IsUtility,
		StatusIcon:                         configDef.StatusIcon,
		StatBonus:                          configDef.StatBonus,
		StatBonusGrandmaster:               configDef.StatBonusGrandmaster,
		StatBonuses:                        configDef.StatBonuses,
		DamageCostMultiplier:               configDef.DamageCostMultiplier,
		ScalesWithPersonality:              configDef.ScalesWithPersonality,
		StunRadiusTiles:                    configDef.StunRadiusTiles,
		StunDurationSeconds:                configDef.StunDurationSeconds,
		StunDurationTurns:                  configDef.StunDurationTurns,
		DealsNoDamage:                      configDef.DealsNoDamage,
		ResistBuffPct:                      configDef.ResistBuffPct,
		ResistBuffPctGrandmaster:           configDef.ResistBuffPctGrandmaster,
		OutgoingDamageBonus:                configDef.OutgoingDamageBonus,
		OutgoingDamageBonusGrandmaster:     configDef.OutgoingDamageBonusGrandmaster,
		OutgoingDamageType:                 strings.ToLower(strings.TrimSpace(configDef.OutgoingDamageType)),
		IncomingDamageReduction:            configDef.IncomingDamageReduction,
		IncomingDamageReductionGrandmaster: configDef.IncomingDamageReductionGrandmaster,
		BindUndead:                         configDef.BindUndead,
		BindDurationSeconds:                configDef.BindDurationSeconds,
		Pacify:                             configDef.Pacify,
		PacifyDurationSeconds:              configDef.PacifyDurationSeconds,
		Revive:                             configDef.Revive,
		FullHeal:                           configDef.FullHeal,
		ReviveHpPct:                        configDef.ReviveHpPct,
		HealParty:                          configDef.HealParty,
		StunChance:                         configDef.StunChance,
		PartyAoeRadiusTiles:                configDef.PartyAoeRadiusTiles,
		StarburstFx:                        configDef.StarburstFx,
		ZoneRadiusTiles:                    configDef.ZoneRadiusTiles,
		ZoneTickDamage:                     configDef.ZoneTickDamage,
		ZoneTickSeconds:                    configDef.ZoneTickSeconds,
		// Effect configuration from YAML
		HealAmount:        configDef.HealAmount,
		VisionRadiusTiles: configDef.VisionRadiusTiles,
		TargetSelf:        configDef.TargetSelf,
		Awaken:            configDef.Awaken,
		WaterWalk:         configDef.WaterWalk,
		WaterBreathing:    configDef.WaterBreathing,
		Message:           configDef.Message,
	}, nil
}

// IsOffensive reports whether this spell harms or disables enemies — i.e. it
// is a "combat" spell for the smart-attack autocast (Space). Decided purely by
// mechanical effect, NOT by the IsUtility flag: AoE-stun (Stun/Darkness) and
// damage zones (Hot Steam) are flagged utility yet are clearly offensive.
// Heals, revives, buffs and pure utility (vision/movement) all return false.
func (d SpellDefinition) IsOffensive() bool {
	return d.IsProjectile ||
		d.AoeRadiusTiles > 0 ||
		d.StunRadiusTiles > 0 ||
		d.ZoneRadiusTiles > 0 ||
		d.PartyAoeRadiusTiles > 0 ||
		d.BindUndead ||
		d.Pacify ||
		d.DisintegrateChance > 0 ||
		d.StunChance > 0
}

// EffectLines returns the character-INDEPENDENT mechanics of a spell as
// human-readable lines — the SINGLE SOURCE shared by the in-game tooltip and the
// map-editor spell card so the two can never drift. It excludes values that
// scale with the caster (projectile damage/heal totals, current buff magnitudes,
// buff duration); those are rendered per-consumer because the editor has no
// character context. Range lines read SpellDefinition fields — add a YAML field,
// add a line here, never name-switch.
func (d SpellDefinition) EffectLines() []string {
	var out []string
	if d.AoeRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("AoE radius: %.1f tiles (splashes nearby monsters)", d.AoeRadiusTiles))
	}
	if d.DisintegrateChance > 0 {
		out = append(out, fmt.Sprintf("Disintegrate: %.0f%% chance to instantly kill on hit (undead and dragons immune)", d.DisintegrateChance*100))
	}
	if d.StunChance > 0 {
		line := fmt.Sprintf("Stun chance: %.0f%% on hit", d.StunChance*100)
		if d.StunDurationSeconds > 0 {
			line += fmt.Sprintf(" (%ds / %d TB turns)", d.StunDurationSeconds, d.StunDurationTurns)
		}
		out = append(out, line)
	}
	if d.StunRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Stuns every monster within %.1f tiles for %ds / %d TB turns", d.StunRadiusTiles, d.StunDurationSeconds, d.StunDurationTurns))
	}
	if d.StunChance > 0 || d.StunRadiusTiles > 0 {
		out = append(out, "Repeated stuns wear off (diminishing returns), then the target is briefly immune")
	}
	if d.BindUndead {
		out = append(out, fmt.Sprintf("Binds an undead target for %ds (it fights other monsters for you)", d.BindDurationSeconds))
	}
	if d.Pacify {
		out = append(out, fmt.Sprintf("Pacifies a living target for %ds (stops attacking; breaks if hit)", d.PacifyDurationSeconds))
	}
	if d.PartyAoeRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Engulfs everything within %.1f tiles for %d damage - your party too", d.PartyAoeRadiusTiles, d.SpellPointsCost*SpellDamagePerSP))
	}
	if d.ZoneRadiusTiles > 0 {
		// Radius and tick cadence are rendered STRUCTURED in the unified card's ZONE
		// section (and filtered out of EFFECTS), so this summary line states only
		// who it hits — monsters, never the party.
		out = append(out, "Leaves a lingering zone that scalds any monster inside (your party is unharmed)")
	}
	switch {
	case d.HealParty:
		out = append(out, "Heals the entire party")
	case d.HealAmount > 0 && d.TargetSelf:
		out = append(out, "Self-target only")
	case d.HealAmount > 0:
		out = append(out, "Can target any party member")
	}
	if d.Revive {
		if d.FullHeal {
			out = append(out, "Revives a fallen ally to full HP (even if eradicated)")
		} else {
			out = append(out, "Revives a fallen ally")
		}
	}
	if d.ReviveHpPct > 0 {
		out = append(out, fmt.Sprintf("Revives a fallen ally to %d%% HP", d.ReviveHpPct))
	}
	if d.ResistBuffPct > 0 {
		if d.ResistBuffPctGrandmaster > d.ResistBuffPct {
			out = append(out, fmt.Sprintf("Party takes %d%% to %d%% less damage by mastery", d.ResistBuffPct, d.ResistBuffPctGrandmaster))
		} else {
			out = append(out, fmt.Sprintf("Party takes %d%% less damage", d.ResistBuffPct))
		}
	}
	if d.OutgoingDamageBonus > 0 {
		target := "attacks"
		if d.OutgoingDamageType == "physical" {
			target = "physical attacks"
		}
		if d.OutgoingDamageBonusGrandmaster > d.OutgoingDamageBonus {
			out = append(out, fmt.Sprintf("Party %s deal +%d to +%d damage by mastery", target, d.OutgoingDamageBonus, d.OutgoingDamageBonusGrandmaster))
		} else {
			out = append(out, fmt.Sprintf("Party %s deal +%d damage", target, d.OutgoingDamageBonus))
		}
	}
	if d.IncomingDamageReduction > 0 {
		if d.IncomingDamageReductionGrandmaster > d.IncomingDamageReduction {
			out = append(out, fmt.Sprintf("Party takes -%d to -%d damage per hit by mastery", d.IncomingDamageReduction, d.IncomingDamageReductionGrandmaster))
		} else {
			out = append(out, fmt.Sprintf("Party takes -%d damage per hit", d.IncomingDamageReduction))
		}
	}
	if d.VisionRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Sight/radar radius: %.0f tiles", d.VisionRadiusTiles))
	}
	if d.WaterWalk {
		out = append(out, "Allows the party to walk on water")
	}
	if d.WaterBreathing {
		out = append(out, "Allows underwater travel through deep water")
	}
	if d.Awaken {
		out = append(out, "Wakes all unconscious allies (back to 1 HP)")
	}

	// Scaling source — character-INDEPENDENT (which stat & mastery the effect
	// grows with), so the map-editor card and the in-game tooltip both surface
	// what a spell scales from. The numeric bonus itself is caster-dependent and
	// shown only by the in-game tooltip.
	switch {
	case d.IsProjectile && !d.DealsNoDamage:
		out = append(out, fmt.Sprintf("Damage scales with %s & %s mastery", d.DamageScalingStat(), d.School))
	case d.ZoneRadiusTiles > 0:
		out = append(out, fmt.Sprintf("Tick damage scales with Intellect & %s mastery", d.School))
	}
	if d.HealAmount > 0 {
		out = append(out, fmt.Sprintf("Healing scales with Personality & %s mastery", d.School))
	}
	if d.StatBonus > 0 {
		if d.StatBonusGrandmaster > d.StatBonus {
			out = append(out, fmt.Sprintf("+%d to +%d to all stats by mastery (whole party)", d.StatBonus, d.StatBonusGrandmaster))
		} else {
			out = append(out, fmt.Sprintf("+%d to all stats (whole party)", d.StatBonus))
		}
	}
	if len(d.StatBonuses) > 0 {
		// Per-stat buffs are authored absolute (no mastery scaling) — the exact
		// numbers are character-independent, so they belong in this shared SSoT.
		for _, key := range config.StatNames {
			if v, ok := d.StatBonuses[key]; ok && v != 0 {
				out = append(out, fmt.Sprintf("%+d %s (whole party)", v, strings.ToUpper(key[:1])+key[1:]))
			}
		}
	}
	return out
}

// SchoolScalesWithPersonality reports whether a school's spells scale with
// Personality instead of Intellect — the self-magic schools (body/mind/spirit).
func SchoolScalesWithPersonality(school string) bool {
	switch school {
	case "body", "mind", "spirit":
		return true
	}
	return false
}

// DamageStatLabel names the stat(s) that scale a spell's damage: Personality for
// self schools, Intellect otherwise, plus a second Personality term when the
// spell is flagged scales_with_personality. Character-independent SSoT shared by
// EffectLines and the in-game damage label.
func DamageStatLabel(school string, scalesWithPersonality bool) string {
	if SchoolScalesWithPersonality(school) {
		return "Personality"
	}
	if scalesWithPersonality {
		return "Intellect + Personality"
	}
	return "Intellect"
}

// DamageScalingStat is DamageStatLabel for this definition.
func (d SpellDefinition) DamageScalingStat() string {
	return DamageStatLabel(d.School, d.ScalesWithPersonality)
}

// IsHeal reports whether this spell restores HP to a living ally (single-target
// or whole party). Revives (which target the fallen) are intentionally excluded
// so the heal hotkey never wastes a resurrect on a conscious member.
func (d SpellDefinition) IsHeal() bool {
	return d.HealAmount > 0 || d.HealParty
}

// CreateSpellItem creates an item from a spell definition
func CreateSpellItem(spellID SpellID) (items.Item, error) {
	def, err := GetSpellDefinitionByID(spellID)
	if err != nil {
		return items.Item{}, err
	}

	itemType := items.ItemBattleSpell
	if def.IsUtility {
		itemType = items.ItemUtilitySpell
	}

	return items.Item{
		Name:        def.Name,
		Type:        itemType,
		Description: def.Description,
		SpellSchool: def.School,
		SpellCost:   def.SpellPointsCost,
		SpellEffect: items.SpellEffect(spellID),
		Attributes:  make(map[string]int),
	}, nil
}

// GetSpellIDByName returns dynamic SpellID for a given spell name
func GetSpellIDByName(name string) (SpellID, error) {
	if _, spellKey, exists := config.GetSpellDefinitionByName(name); exists {
		return SpellID(spellKey), nil
	}
	return "", fmt.Errorf("spell '%s' not found in spells.yaml", name)
}

// GetSpellIDsBySchool returns all spell IDs for a given magic school
func GetSpellIDsBySchool(school string) ([]SpellID, error) {
	spellKeys := config.GetSpellsBySchool(school)
	spellIDs := make([]SpellID, 0, len(spellKeys))

	for _, spellKey := range spellKeys {
		spellIDs = append(spellIDs, SpellID(spellKey))
	}

	return spellIDs, nil
}
