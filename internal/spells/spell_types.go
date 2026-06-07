package spells

import (
	"fmt"
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
	ID                 SpellID
	Name               string
	Description        string
	School             string
	Level              int // Spell level (1-9)
	SpellPointsCost    int
	CooldownSeconds    float64 // RT cast cooldown (seconds) at reference Speed; 0 = derive from Level
	Duration           int     // Duration in seconds (0 for instant spells)
	DisintegrateChance float64
	AoeRadiusTiles     float64 // 0 = single-target; >0 = splash radius in tiles
	ProjectileSize     int
	IsProjectile       bool
	IsUtility          bool
	StatusIcon         string
	StatBonus          int // Stat bonus for buff spells like Bless
	// Damage-formula modifiers (default behaviour when zero/false)
	DamageCostMultiplier  int  // base = cost × SpellDamagePerSP × this (default 1)
	ScalesWithPersonality bool // also add Personality/divisor to spell damage
	// AoE-stun effect (Darkness): >0 radius stuns all monsters in range, no damage
	StunRadiusTiles     float64
	StunDurationSeconds int
	StunDurationTurns   int
	DealsNoDamage       bool // zero direct damage (Disintegrate: only the instakill roll matters)
	// Party combat buffs (duration seconds)
	ResistBuffPct           int // Day of the Gods: % incoming damage reduction
	OutgoingDamageBonus     int // Hour of Power: flat outgoing damage bonus
	IncomingDamageReduction int // Hour of Power: flat incoming damage reduction
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
	HealAmount     int     // For healing spells
	VisionBonus    float64 // For vision enhancement spells
	TargetSelf     bool    // Whether spell targets self or others
	Awaken         bool    // For awaken spell
	WaterWalk      bool    // For water walking spell
	WaterBreathing bool    // For water breathing spell
	Message        string  // Effect message to display
}

// GetSpellDefinitionByID retrieves spell definition from YAML config
func GetSpellDefinitionByID(spellID SpellID) (SpellDefinition, error) {
	configDef, exists := config.GetSpellDefinition(string(spellID))
	if !exists {
		return SpellDefinition{}, fmt.Errorf("spell '%s' not found in spells.yaml", spellID)
	}

	return SpellDefinition{
		ID:                      spellID,
		Name:                    configDef.Name,
		Description:             configDef.Description,
		School:                  configDef.School,
		Level:                   configDef.Level,
		SpellPointsCost:         configDef.SpellPointsCost,
		CooldownSeconds:         configDef.CooldownSeconds,
		Duration:                configDef.Duration,
		DisintegrateChance:      configDef.DisintegrateChance,
		AoeRadiusTiles:          configDef.AoeRadiusTiles,
		ProjectileSize:          configDef.ProjectileSize,
		IsProjectile:            configDef.IsProjectile,
		IsUtility:               configDef.IsUtility,
		StatusIcon:              configDef.StatusIcon,
		StatBonus:               configDef.StatBonus,
		DamageCostMultiplier:    configDef.DamageCostMultiplier,
		ScalesWithPersonality:   configDef.ScalesWithPersonality,
		StunRadiusTiles:         configDef.StunRadiusTiles,
		StunDurationSeconds:     configDef.StunDurationSeconds,
		StunDurationTurns:       configDef.StunDurationTurns,
		DealsNoDamage:           configDef.DealsNoDamage,
		ResistBuffPct:           configDef.ResistBuffPct,
		OutgoingDamageBonus:     configDef.OutgoingDamageBonus,
		IncomingDamageReduction: configDef.IncomingDamageReduction,
		BindUndead:              configDef.BindUndead,
		BindDurationSeconds:     configDef.BindDurationSeconds,
		Pacify:                  configDef.Pacify,
		PacifyDurationSeconds:   configDef.PacifyDurationSeconds,
		Revive:                  configDef.Revive,
		FullHeal:                configDef.FullHeal,
		ReviveHpPct:             configDef.ReviveHpPct,
		HealParty:               configDef.HealParty,
		StunChance:              configDef.StunChance,
		PartyAoeRadiusTiles:     configDef.PartyAoeRadiusTiles,
		StarburstFx:             configDef.StarburstFx,
		ZoneRadiusTiles:         configDef.ZoneRadiusTiles,
		ZoneTickDamage:          configDef.ZoneTickDamage,
		ZoneTickSeconds:         configDef.ZoneTickSeconds,
		// Effect configuration from YAML
		HealAmount:     configDef.HealAmount,
		VisionBonus:    configDef.VisionBonus,
		TargetSelf:     configDef.TargetSelf,
		Awaken:         configDef.Awaken,
		WaterWalk:      configDef.WaterWalk,
		WaterBreathing: configDef.WaterBreathing,
		Message:        configDef.Message,
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
// scale with the caster (projectile damage/heal totals, the Bless stat bonus,
// buff duration); those are rendered per-consumer because the editor has no
// character context. Every line reads a flat SpellDefinition field — add a YAML
// field, add a line here, never name-switch.
func (d SpellDefinition) EffectLines() []string {
	var out []string
	if d.AoeRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("AoE radius: %.1f tiles (splashes nearby monsters)", d.AoeRadiusTiles))
	}
	if d.DisintegrateChance > 0 {
		out = append(out, fmt.Sprintf("Disintegrate: %.0f%% chance to instantly kill on hit", d.DisintegrateChance*100))
	}
	if d.StunChance > 0 {
		line := fmt.Sprintf("Stun chance: %.0f%% on hit", d.StunChance*100)
		if d.StunDurationSeconds > 0 {
			line += fmt.Sprintf(" (%ds)", d.StunDurationSeconds)
		}
		out = append(out, line)
	}
	if d.StunRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Stuns every monster within %.1f tiles for %ds", d.StunRadiusTiles, d.StunDurationSeconds))
	}
	if d.BindUndead {
		out = append(out, fmt.Sprintf("Binds an undead target for %ds (it fights other monsters for you)", d.BindDurationSeconds))
	}
	if d.Pacify {
		out = append(out, fmt.Sprintf("Pacifies a living target for %ds (stops attacking; breaks if hit)", d.PacifyDurationSeconds))
	}
	if d.PartyAoeRadiusTiles > 0 {
		out = append(out, fmt.Sprintf("Engulfs everything within %.1f tiles for %d damage — your party too", d.PartyAoeRadiusTiles, d.SpellPointsCost*SpellDamagePerSP))
	}
	if d.ZoneRadiusTiles > 0 {
		// Tick damage is caster-scaled (Intellect + mastery) — shown by the in-game
		// tooltip; here (character-independent) we list only radius and cadence.
		out = append(out, fmt.Sprintf("Leaves a %.1f-tile zone, searing everything inside every %.0fs", d.ZoneRadiusTiles, d.ZoneTickSeconds))
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
	// Party buffs: the numeric value is caster-dependent (scales with mastery, like
	// Bless) and printed by the in-game tooltip; here (character-independent) we
	// state only the effect + its scaling source so the map-editor card shows it.
	if d.ResistBuffPct > 0 {
		out = append(out, fmt.Sprintf("Party %% damage resistance (scales with %s mastery)", d.School))
	}
	if d.OutgoingDamageBonus > 0 {
		out = append(out, fmt.Sprintf("Party flat attack bonus (scales with %s mastery)", d.School))
	}
	if d.IncomingDamageReduction > 0 {
		out = append(out, fmt.Sprintf("Party flat damage reduction (scales with %s mastery)", d.School))
	}
	if d.VisionBonus > 0 {
		out = append(out, "Extends the party's sight radius")
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
		out = append(out, fmt.Sprintf("Bonus scales with %s mastery", d.School))
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
