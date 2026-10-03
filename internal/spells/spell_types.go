package spells

import (
	"fmt"
	"strings"
	uitext "ugataima/assets/text"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/stats"
)

// SpellID represents dynamic spell identifiers loaded from YAML
type SpellID string

const (
	// SpellCategoryBuff marks a beneficial timed spell that has no personal
	// cooldown in real-time mode. Turn-based casts still spend an action.
	SpellCategoryBuff = "buff"
)

// String returns the string representation of a spell ID
func (s SpellID) String() string {
	return string(s)
}

// SpellDefinition represents the complete definition of a spell loaded from YAML
type SpellDefinition struct {
	ID                    SpellID
	Name                  string
	Description           string
	School                string
	SpellPointsCost       int
	Category              string
	CooldownSeconds       float64 // RT cast cooldown (seconds) at reference Speed; authored per spell (required)
	DamageByMastery       []int   // exact damage per mastery tier (Novice..GM); wins over the cost formula
	SummonMonster         string  // monster key summoned as a party ally
	SummonMax             int     // live cap for this spell's summons
	SummonHPByMastery     []int   // spawned HP per mastery tier (Novice..GM)
	SummonDamageByMastery []int   // spawned damage per mastery tier (Novice..GM)
	JumpTiles             float64 // >0: self teleport this many tiles straight ahead
	ZoneAheadTiles        float64 // zone line placed this far in front of the party
	ZoneWidthTiles        int     // zone line width in tiles (across the facing)
	ZoneEdgeTiles         int     // band beside a wall zone that takes partial damage
	ZoneEdgeDamagePercent int     // damage share in that band
	ZoneBurnSeconds       float64 // zone hits ignite the victim for this long
	StandeeDestroyChance  float64 // chance to topple a crossed-standee tile it hits
	SparesParty           bool    // party-centred nova that does not hurt the party
	Duration              int     // Duration in seconds (0 for instant spells)
	DisintegrateChance    float64
	AoeRadiusTiles        float64 // 0 = single-target; >0 = splash radius in tiles
	ProjectileSize        int
	IsProjectile          bool
	IsUtility             bool
	StatusIcon            string
	StatBonus             int            // Uniform stat bonus for buff spells like Bless
	StatBonusGrandmaster  int            // optional GM-scaled uniform stat bonus cap
	StatBonuses           map[string]int // Per-stat alternative (lowercase stat keys)
	// Damage-formula modifiers (default behaviour when zero/false)
	DamageCostMultiplier  int  // base = cost x SpellDamagePerSP x this (default 1)
	ScalesWithPersonality bool // also add Personality/divisor to spell damage
	MasteryDamagePerTier  int  // explicit special-spell scaling (Inferno)
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
	BindUndead            bool    // Bind Undead: take control of an UNDEAD target - it hunts other monsters for you
	BindDurationSeconds   int     // Bind Undead duration (RT seconds)
	Pacify                bool    // Charm: pacify a LIVING target - it stops attacking; breaks on any hit it takes
	PacifyDurationSeconds int     // Charm duration (RT seconds)
	Revive                bool    // resurrect: restore a fallen ally (incl. eradicated)
	FullHeal              bool    // resurrect: restore to maximum HP
	ReviveHpPct           int     // Raise Dead: revive a fallen ally (not eradicated) to this % of max HP
	HealParty             bool    // Mass Heal: heal every party member
	StunChance            float64 // Psychic Shock: chance to stun the struck monster on hit
	// Party-centered instant nova (Inferno): damages all monsters AND the party in radius.
	PartyAoeRadiusTiles float64
	// MapWide (Inferno rework): the nova burns EVERY monster on the map AND the party.
	MapWide bool
	// Persistent damage zone (Hot Steam).
	StarburstFx     bool
	ZoneRadiusTiles float64
	ZoneTickDamage  int
	ZoneTickSeconds float64
	// Per-school party resist buff (Fire Shield).
	ResistBuffSchool    string
	ResistBuffSchoolPct int
	// Mortar (Stone Blossom): no collisions in flight, detonates at a fixed distance.
	MortarRangeTiles float64
	// Effect configuration
	Schools          []string // every school the spell belongs to (dual-school); empty = just School
	HealAmount       int      // For healing spells
	LightRadiusTiles float64  // light around the party (tiles)
	RadarRadiusTiles float64  // compass radar of monsters through walls (tiles)
	TargetSelf       bool     // Whether spell targets self or others
	Awaken           bool     // For awaken spell
	WaterWalk        bool     // For water walking spell
	WaterBreathing   bool     // For water breathing spell
	Fly              bool     // Fly: walk through non-border tiles
	TerrainPassage   bool     // Shared traversal capability granted by a timed buff
	OutdoorOnly      bool     // castable only under a day/night sky
	TownPortal       bool     // opens the visited-destination picker
	Message          string   // Effect message to display
}

// DamageForMastery evaluates the authored damage without character stats.
// Effect summaries and editor ranges use the same formula as combat.
func (d SpellDefinition) DamageForMastery(tier int) int {
	return d.DamageFormula().Evaluate(stats.StatBonuses{}, tier).Total
}

// SchoolList returns every school the spell belongs to: Schools when authored,
// else the single School. The ONE place dual-school membership is resolved.
func (d SpellDefinition) SchoolList() []string {
	if len(d.Schools) > 0 {
		return d.Schools
	}
	return []string{d.School}
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
		SpellPointsCost:                    configDef.SpellPointsCost,
		Category:                           strings.ToLower(strings.TrimSpace(configDef.Category)),
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
		MasteryDamagePerTier:               configDef.MasteryDamagePerTier,
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
		MapWide:                            configDef.MapWide,
		DamageByMastery:                    configDef.DamageByMastery,
		SummonMonster:                      configDef.SummonMonster,
		SummonMax:                          configDef.SummonMax,
		SummonHPByMastery:                  configDef.SummonHPByMastery,
		SummonDamageByMastery:              configDef.SummonDamageByMastery,
		JumpTiles:                          configDef.JumpTiles,
		ZoneAheadTiles:                     configDef.ZoneAheadTiles,
		ZoneWidthTiles:                     configDef.ZoneWidthTiles,
		ZoneEdgeTiles:                      configDef.ZoneEdgeTiles,
		ZoneEdgeDamagePercent:              configDef.ZoneEdgeDamagePercent,
		ZoneBurnSeconds:                    configDef.ZoneBurnSeconds,
		StandeeDestroyChance:               configDef.StandeeDestroyChance,
		SparesParty:                        configDef.SparesParty,
		StarburstFx:                        configDef.StarburstFx,
		ZoneRadiusTiles:                    configDef.ZoneRadiusTiles,
		ZoneTickDamage:                     configDef.ZoneTickDamage,
		ZoneTickSeconds:                    configDef.ZoneTickSeconds,
		ResistBuffSchool:                   configDef.ResistBuffSchool,
		ResistBuffSchoolPct:                configDef.ResistBuffSchoolPct,
		MortarRangeTiles:                   configDef.MortarRangeTiles,
		// Effect configuration from YAML
		Schools:          configDef.Schools,
		HealAmount:       configDef.HealAmount,
		LightRadiusTiles: configDef.LightRadiusTiles,
		RadarRadiusTiles: configDef.RadarRadiusTiles,
		TargetSelf:       configDef.TargetSelf,
		Awaken:           configDef.Awaken,
		WaterWalk:        configDef.WaterWalk,
		WaterBreathing:   configDef.WaterBreathing,
		Fly:              configDef.Fly,
		TerrainPassage:   configDef.TerrainPassage,
		OutdoorOnly:      configDef.OutdoorOnly,
		TownPortal:       configDef.TownPortal,
		Message:          configDef.Message,
	}, nil
}

// IsBuff reports whether this is a YAML-authored beneficial timed effect. Buff
// spells deliberately have no personal real-time cooldown so a party can be
// prepared without waiting between each cast.
func (d SpellDefinition) IsBuff() bool {
	return d.Category == SpellCategoryBuff
}

// IsOffensive reports whether this spell harms or disables enemies - i.e. it
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
		d.MapWide ||
		d.BindUndead ||
		d.Pacify ||
		d.DisintegrateChance > 0 ||
		d.StunChance > 0
}

// CoreEffectLines omits summaries for values the live tooltip already renders
// with the current caster; all views come from effectLines, so wording cannot
// drift.
func (d SpellDefinition) CoreEffectLines() []string {
	return d.effectLines(effectViewCore)
}

// CardEffectLines keeps base values but omits rows the editor renders separately.
func (d SpellDefinition) CardEffectLines() []string {
	return d.effectLines(effectViewCard)
}

// BuffMechanicLines omits magnitudes supplied by running stat and combat buffs.
// Their cast or saved values may differ from the current spell definition.
func (d SpellDefinition) BuffMechanicLines() []string {
	return d.effectLines(effectViewBuff)
}

type effectView uint8

const (
	effectViewCore effectView = iota
	effectViewCard
	effectViewBuff
)

func (d SpellDefinition) effectLines(view effectView) []string {
	includeStructured := view == effectViewCard
	var out []string
	// Every authored field states itself here, so the game tooltip, the editor
	// card and the shop line can never disagree about a new spell.
	if d.SummonMonster != "" {
		line := uitext.Text("spell.summons_an_ally_up_to_at_a", d.SummonMax)
		if includeStructured && len(d.SummonHPByMastery) == 4 && len(d.SummonDamageByMastery) == 4 {
			line += uitext.Text("spell.by_mastery_hp_and_damage",
				d.SummonHPByMastery[0], d.SummonHPByMastery[3],
				d.SummonDamageByMastery[0], d.SummonDamageByMastery[3])
		}
		out = append(out, line, uitext.Text("spell.summon_lasts_until_killed"))
	}
	if d.JumpTiles > 0 {
		out = append(out, uitext.Text("spell.teleports_the_party_tiles_straight_ahead_refused", d.JumpTiles))
	}
	if d.DisintegrateChance > 0 {
		out = append(out, uitext.Text("spell.disintegrate_chance_to_instantly_kill_on_hit", d.DisintegrateChance*100))
	}
	if d.StunChance > 0 {
		line := uitext.Text("spell.stun_chance_on_hit", d.StunChance*100)
		if d.StunDurationSeconds > 0 {
			line += uitext.Text("spell.stun_duration", d.StunDurationSeconds, tbTurns(d.StunDurationTurns))
		}
		out = append(out, line)
	}
	if d.StunRadiusTiles > 0 {
		out = append(out, uitext.Text("spell.stuns_every_monster_within_tiles_for_s", d.StunRadiusTiles, d.StunDurationSeconds, tbTurns(d.StunDurationTurns)))
	}
	if d.StunChance > 0 || d.StunRadiusTiles > 0 {
		out = append(out, uitext.Text("spell.repeated_stuns_wear_off_diminishing_returns_then"))
	}
	if d.BindUndead {
		out = append(out, uitext.Text("spell.binds_an_undead_target_for_s_it", d.BindDurationSeconds), uitext.Text("spell.bound_undead_still_takes_party_hits"))
	}
	if d.Pacify {
		out = append(out, uitext.Text("spell.pacifies_a_living_target_for_s_stops", d.PacifyDurationSeconds))
	}
	if includeStructured && d.PartyAoeRadiusTiles > 0 {
		minDamage := d.DamageForMastery(0)
		maxDamage := d.DamageForMastery(3)
		caught := uitext.Text("spell.your_party_too")
		if d.SparesParty {
			caught = uitext.Text("spell.the_party_is_spared")
		}
		if maxDamage > minDamage {
			out = append(out, uitext.Text("spell.engulfs_everything_within_tiles_for_damage_by", d.PartyAoeRadiusTiles, minDamage, maxDamage, caught))
		} else {
			out = append(out, uitext.Text("spell.engulfs_everything_within_tiles_for_damage", d.PartyAoeRadiusTiles, minDamage, caught))
		}
	}
	if includeStructured && d.MapWide {
		minDamage := d.DamageForMastery(0)
		maxDamage := d.DamageForMastery(3)
		caught := uitext.Text("spell.your_party_too")
		if d.SparesParty {
			caught = uitext.Text("spell.the_party_is_spared")
		}
		if maxDamage > minDamage {
			out = append(out, uitext.Text("spell.burns_every_monster_on_the_map_for", minDamage, maxDamage, caught))
		} else {
			out = append(out, uitext.Text("spell.map_damage_fixed", minDamage, caught))
		}
	}
	if d.MortarRangeTiles > 0 {
		out = append(out, uitext.Text("spell.arcs_over_everything_and_blooms_exactly_tiles", d.MortarRangeTiles))
	}
	if d.TerrainPassage {
		out = append(out, uitext.Text("spell.the_party_crosses_terrain_and_walls_but"), uitext.Text("spell.no_fighting_inside_solid_terrain"))
		if d.OutdoorOnly {
			out = append(out, uitext.Text("spell.ends_without_open_sky"))
		}
	}
	if d.OutdoorOnly {
		out = append(out, uitext.Text("spell.only_under_an_open_sky_never_in"))
	}
	if d.TownPortal {
		out = append(out, uitext.Text("spell.opens_a_portal_to_visited_taverns_towns"))
	}
	if view != effectViewBuff && d.ResistBuffSchoolPct > 0 && d.ResistBuffSchool != "" {
		out = append(out, uitext.Text("spell.party_resists_for_the_duration",
			strings.ToUpper(d.ResistBuffSchool[:1])+d.ResistBuffSchool[1:], d.ResistBuffSchoolPct))
	}
	if d.ZoneRadiusTiles > 0 {
		// Radius and tick cadence are rendered STRUCTURED in the unified card's ZONE
		// section (and filtered out of EFFECTS), so this summary line states only
		// who it hits - monsters, never the party.
		out = append(out, uitext.Text("spell.leaves_a_lingering_zone_that_harms_any"))
	}
	switch {
	case d.HealParty:
		out = append(out, uitext.Text("spell.heal_skips_allies_at_0_hp"))
	case d.HealAmount > 0 && d.TargetSelf:
		// Self heals have no ally to point at.
	case d.HealAmount > 0:
		out = append(out, uitext.Text("spell.heals_the_ally_you_point_at"))
	}
	if d.Revive {
		if d.FullHeal {
			out = append(out, uitext.Text("spell.revives_a_fallen_ally_to_full_hp"))
		} else {
			out = append(out, uitext.Text("item.revives_a_fallen_ally"))
		}
	}
	if d.ReviveHpPct > 0 {
		out = append(out, uitext.Text("spell.revives_the_first_fallen_ally_to_hp", d.ReviveHpPct))
	}
	if includeStructured && d.ResistBuffPct > 0 {
		if d.ResistBuffPctGrandmaster > d.ResistBuffPct {
			out = append(out, uitext.Text("spell.party_takes_to_less_damage_by_mastery", d.ResistBuffPct, d.ResistBuffPctGrandmaster))
		} else {
			out = append(out, uitext.Text("spell.party_takes_less_damage", d.ResistBuffPct))
		}
	}
	if includeStructured && d.OutgoingDamageBonus > 0 {
		target := OutgoingDamageTarget(d.OutgoingDamageType)
		if d.OutgoingDamageBonusGrandmaster > d.OutgoingDamageBonus {
			out = append(out, uitext.Text("spell.party_deal_to_damage_by_mastery", target, d.OutgoingDamageBonus, d.OutgoingDamageBonusGrandmaster))
		} else {
			out = append(out, uitext.Text("spell.party_deal_damage", target, d.OutgoingDamageBonus))
		}
	}
	if includeStructured && d.IncomingDamageReduction > 0 {
		if d.IncomingDamageReductionGrandmaster > d.IncomingDamageReduction {
			out = append(out, uitext.Text("spell.party_takes_to_damage_per_hit_by", d.IncomingDamageReduction, d.IncomingDamageReductionGrandmaster))
		} else {
			out = append(out, uitext.Text("spell.party_takes_damage_per_hit", d.IncomingDamageReduction))
		}
	}
	if d.LightRadiusTiles > 0 {
		out = append(out, uitext.Text("spell.light_radius_tiles", d.LightRadiusTiles))
	}
	if d.RadarRadiusTiles > 0 {
		out = append(out, uitext.Text("spell.radar_radius_tiles", d.RadarRadiusTiles))
	}
	if d.WaterWalk {
		out = append(out, uitext.Text("spell.allows_the_party_to_walk_on_water"))
	}
	if d.WaterBreathing {
		out = append(out, uitext.Text("spell.deep_water_leads_to_the_depths"))
	}
	if d.Awaken {
		out = append(out, uitext.Text("spell.wakes_all_unconscious_allies_back_to_hp"))
	}

	if includeStructured && d.StatBonus > 0 {
		if d.StatBonusGrandmaster > d.StatBonus {
			out = append(out, uitext.Text("spell.to_to_all_stats_by_mastery_whole", d.StatBonus, d.StatBonusGrandmaster))
		} else {
			out = append(out, uitext.Text("spell.to_all_stats_whole_party", d.StatBonus))
		}
	}
	// Per-stat buffs are authored absolute (no mastery scaling) - the exact
	// numbers are character-independent, so they belong in this shared SSoT.
	if view != effectViewBuff {
		out = append(out, PerStatBonusLines(d.StatBonuses)...)
	}
	return out
}

// OutgoingDamageTarget names what a party damage buff boosts, by the same
// damage-type parse combat applies.
func OutgoingDamageTarget(damageType string) string {
	if t, err := damagecalc.ParseType(damageType); err == nil && t == damagecalc.Physical {
		return uitext.Text("spell.physical_attacks")
	}
	return uitext.Text("spell.attacks")
}

// PerStatBonusLines states a per-stat party bonus map in canonical stat order.
func PerStatBonusLines(bonuses map[string]int) []string {
	var out []string
	for _, key := range config.StatNames {
		if v := bonuses[key]; v != 0 {
			out = append(out, uitext.Text("spell.whole_party", v, strings.ToUpper(key[:1])+key[1:]))
		}
	}
	return out
}

// tbTurns renders a turn-based span: "1 TB turn", "3 TB turns".
func tbTurns(n int) string {
	if n == 1 {
		return "1 TB turn"
	}
	return fmt.Sprintf("%d TB turns", n)
}

// SchoolScalesWithPersonality reports whether a school's spells scale with
// Personality instead of Intellect - the self-magic schools (body/mind/spirit).
func SchoolScalesWithPersonality(school string) bool {
	damageType, err := damagecalc.ParseType(school)
	if err != nil {
		return false
	}
	switch damageType {
	case damagecalc.Body, damagecalc.Mind, damagecalc.Spirit:
		return true
	}
	return false
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

// GetSpellIDsBySchool returns all spell IDs for a given magic school
func GetSpellIDsBySchool(school string) ([]SpellID, error) {
	spellKeys := config.GetSpellsBySchool(school)
	spellIDs := make([]SpellID, 0, len(spellKeys))

	for _, spellKey := range spellKeys {
		spellIDs = append(spellIDs, SpellID(spellKey))
	}

	return spellIDs, nil
}
