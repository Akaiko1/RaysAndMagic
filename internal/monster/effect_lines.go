package monster

import (
	"fmt"
	"strings"

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
	return !d.WarlordIdol && d.Disposition != "caravan" && d.Disposition != DispositionFish
}

// CombatEffectLines is the single formatter for monster attack/special ability
// rows shown by editor previews. Keep new YAML combat knobs here so consumers do
// not hand-pick fields and drift.
func (d MonsterDefinition) CombatEffectLines(contexts ...CombatEffectContext) []EffectLine {
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
	if d.hasTrapVolley() {
		addSchool(damagecalc.Fire.String(), fmt.Sprintf(
			"Trap field: sows %d fire traps (%.0f dmg) within %.0f tiles every %.0fs / %d turns",
			d.TrapVolleyCount, float64(d.TrapVolleyDamage), d.TrapVolleyRadiusTiles,
			d.TrapVolleyIntervalSeconds, d.TrapVolleyIntervalTurns))
	}
	if d.PounceRangeTiles > 0 {
		add(fmt.Sprintf("Pounce: %.1f tiles every %.0fs", d.PounceRangeTiles, d.PounceCooldownSeconds))
	}
	if d.PoisonChance > 0 {
		add(fmt.Sprintf("Poison: %.0f%% for %ds", d.PoisonChance*100, d.PoisonDurationSec))
	}
	if d.IgniteChance > 0 {
		addSchool(damagecalc.Fire.String(), fmt.Sprintf("Ignite: %.0f%% for %ds", d.IgniteChance*100, d.IgniteDurationSec))
	}
	if d.StunCharChance > 0 {
		add(fmt.Sprintf("Stun: %.0f%% (%ds / %d turns)", d.StunCharChance*100, d.StunCharSeconds, d.StunCharTurns))
	}
	if d.DispelChance > 0 {
		add(fmt.Sprintf("Dispel buff: %.0f%%", d.DispelChance*100))
	}
	if d.FireburstChance > 0 {
		addSchool(damagecalc.Fire.String(), fmt.Sprintf("Fireburst: %.0f%% for %d-%d", d.FireburstChance*100, d.FireburstDamageMin, d.FireburstDamageMax))
	}
	if d.DragonBreathChance > 0 {
		school := normalizeEffectSchool(d.DragonBreathType)
		addSchool(school, fmt.Sprintf("Dragon Breath: %.0f%% %s attack to whole party", d.DragonBreathChance*100, school))
	}
	if d.PiercingShotChance > 0 {
		add(fmt.Sprintf("Piercing shot: %.0f%% (%d targets)", d.PiercingShotChance*100, d.PiercingShotTargets))
	}
	if d.AllyHealChance > 0 {
		add(fmt.Sprintf("Heals allies: %.0f%% for %d (%.0f tiles)", d.AllyHealChance*100, d.AllyHealAmount, d.AllyHealRadius))
	}
	addf := func(format string, args ...any) { add(fmt.Sprintf(format, args...)) }
	// Boss kit.
	if d.PassiveUntilQuest != "" {
		addf("Sealed until quest: %s", d.PassiveUntilQuest)
	}
	if d.EvadeRadiusTiles > 0 {
		addf("Evades within %.1f tiles (cd %.0fs)", d.EvadeRadiusTiles, d.BossCooldownSecs)
	}
	if d.InfernoChance > 0 {
		addf("Inferno nova: %.0f%% for %d fire damage (%.1f tiles)", d.InfernoChance*100, d.InfernoDamage, d.InfernoRangeTiles)
	}
	if d.TeleportAtHP > 0 {
		addf("Blinks at/below %d HP (%.0f%%)", d.TeleportAtHP, d.TeleportChance*100)
	}
	if len(d.SummonMonsters) > 0 {
		n := d.SummonCount
		if n == 0 {
			n = 1
		}
		capText := "no live summon cap"
		if d.SummonMax > 0 {
			capText = fmt.Sprintf("max %d alive", d.SummonMax)
		}
		if d.SummonFirstGuaranteed {
			addf("Summons %dx {%s}: first guaranteed, then %.0f%% (%s)", n, strings.Join(d.SummonMonsters, ", "), d.SummonChance*100, capText)
		} else {
			addf("Summons %dx {%s}: %.0f%% (%s)", n, strings.Join(d.SummonMonsters, ", "), d.SummonChance*100, capText)
		}
	}
	if d.EnrageAtHP > 0 {
		addf("Enrages at/below %d HP: dmg x%.1f, cd x%.1f", d.EnrageAtHP, effectiveMultiplier(d.EnrageDamageMult), effectiveMultiplier(d.EnrageCooldownMult))
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
