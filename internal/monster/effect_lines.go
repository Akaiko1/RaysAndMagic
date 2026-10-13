package monster

import (
	"fmt"

	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
)

// EffectLine is a character-independent monster mechanic line. School tints the
// row in consumers that support damage-school colors.
type EffectLine struct {
	Text   string
	School string
}

// HasAttackStats distinguishes authored combatants from fish, travelers and
// inert encounter props. Temporary control or boss phases do not change a
// catalog's description of the actor's available attacks.
func (d MonsterDefinition) HasAttackStats() bool {
	return !d.WarlordIdol && d.Disposition != DispositionCaravan && d.Disposition != DispositionFish
}

// CombatEffectLines is the single formatter for a monster's combat rows
// (Identify, map tooltips): AttackLines, AbilityLines, then BehaviorLines.
// The editor shows the three under their own headings. Consumers never
// hand-pick fields.
func (d MonsterDefinition) CombatEffectLines(contexts ...CombatEffectContext) []EffectLine {
	if !d.HasAttackStats() {
		return nil
	}
	out := d.AttackLines(contexts...)
	out = append(out, d.AbilityLines()...)
	return append(out, d.BehaviorLines()...)
}

// AttackLines describes the monster's own attack: ranged spell or weapon (with
// the spell's riders) and the melee profile.
func (d MonsterDefinition) AttackLines(contexts ...CombatEffectContext) []EffectLine {
	if !d.HasAttackStats() {
		return nil
	}
	var ctx CombatEffectContext
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	var out []EffectLine
	add := func(text string) {
		out = append(out, EffectLine{Text: text})
	}
	addSchool := func(school, text string) {
		out = append(out, EffectLine{Text: text, School: normalizeEffectSchool(school)})
	}

	if d.ProjectileSpell != "" {
		if sp, ok := config.GetSpellDefinition(d.ProjectileSpell); ok && sp != nil {
			school := normalizeEffectSchool(sp.School)
			addSchool(school, fmt.Sprintf("Ranged spell: %s (%s)", sp.Name, school))
			if sp.AoeRadiusTiles > 0 {
				addSchool(school, fmt.Sprintf("Projectile AoE: whole party on hit (spell radius %.1f)", sp.AoeRadiusTiles))
			}
			if rider := d.ProjectileStun(d.ProjectileSpell); rider.Chance > 0 {
				add(fmt.Sprintf("Projectile stun: %.0f%% on hit, %s", rider.Chance*100, config.DurationText(rider.Seconds)))
			}
			if sp.DisintegrateChance > 0 {
				add(fmt.Sprintf("Disintegrate on hit: %.0f%%", sp.DisintegrateChance*100))
			}
		} else {
			add(fmt.Sprintf("Ranged spell: %s", d.ProjectileSpell))
		}
	}
	if d.ProjectileWeapon != "" {
		if w, ok := config.GetWeaponDefinition(d.ProjectileWeapon); ok && w != nil {
			school := normalizeEffectSchool(w.DamageType)
			if d.Champion != "" {
				add(fmt.Sprintf("Ranged weapon: %s", w.Name))
			} else {
				addSchool(school, fmt.Sprintf("Ranged weapon: %s (%s)", w.Name, school))
			}
		} else {
			add(fmt.Sprintf("Ranged weapon: %s", d.ProjectileWeapon))
		}
	}
	profile := d.MeleeProfile(ctx.ElementalAttack, ctx.ElementalSchool)
	if profile.School != "" {
		addSchool(profile.School, "Melee: Physical")
		if profile.ElementalAttack.Chance > 0 && d.Disposition == "" {
			element := profile.ElementalSchool
			if element == "" {
				element = "biome-dependent"
			}
			line := fmt.Sprintf("Elemental Attack: %g%%, x%g raw melee damage (%s)", profile.ElementalAttack.Chance*100, profile.ElementalAttack.DamageMultiplier, element)
			if profile.ElementalSchool == "" {
				add(line)
			} else {
				addSchool(profile.ElementalSchool, line)
			}
		}
	}
	return out
}

// AbilityLines describes every authored ability (abilityRows), in display order.
func (d MonsterDefinition) AbilityLines() []EffectLine {
	if !d.HasAttackStats() {
		return nil
	}
	return d.abilityLines()
}

// BehaviorLines describes the boss and rally behaviour that frames the fight.
func (d MonsterDefinition) BehaviorLines() []EffectLine {
	if !d.HasAttackStats() {
		return nil
	}
	var out []EffectLine
	addf := func(format string, args ...any) { out = append(out, EffectLine{Text: fmt.Sprintf(format, args...)}) }
	if d.PassiveUntilQuest != "" {
		addf("Sealed until quest: %s", d.PassiveUntilQuest)
	}
	if d.EvadeRadiusTiles > 0 {
		addf("Evades within %.1f tiles (cd %.0fs)", d.EvadeRadiusTiles, d.BossCooldownSecs)
	}
	if d.DeathRalliesType != "" {
		addf("Death rallies: %s", d.DeathRalliesType)
	}
	if d.RallyOnAggroTiles > 0 {
		if d.RallyMaxTargets > 0 {
			addf("Aggro rally: up to %d mobs within %.0f tiles", d.RallyMaxTargets, d.RallyOnAggroTiles)
		} else {
			addf("Aggro rally: every mob within %.0f tiles", d.RallyOnAggroTiles)
		}
	}

	return out
}

func normalizeEffectSchool(school string) string {
	damageType, err := damagecalc.ParseType(school)
	if err != nil {
		return damagecalc.Physical.String()
	}
	return damageType.String()
}

// Zero means no authored modifier; combat only applies positive multipliers.
func effectiveMultiplier(value float64) float64 {
	if value > 0 {
		return value
	}
	return 1
}
