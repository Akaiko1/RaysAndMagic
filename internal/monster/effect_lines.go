package monster

import (
	"fmt"
	"strings"

	"ugataima/internal/config"
)

// EffectLine is a character-independent monster mechanic line. School tints the
// row in consumers that support damage-school colors.
type EffectLine struct {
	Text   string
	School string
}

// CombatEffectLines is the single formatter for monster attack/special ability
// rows shown by editor previews. Keep new YAML combat knobs here so consumers do
// not hand-pick fields and drift.
func (d MonsterDefinition) CombatEffectLines() []EffectLine {
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
			add(fmt.Sprintf("Ranged weapon: %s", w.Name))
		} else {
			add(fmt.Sprintf("Ranged weapon: %s", d.ProjectileWeapon))
		}
	}
	if d.PounceRangeTiles > 0 {
		add(fmt.Sprintf("Pounce: %.1f tiles every %.0fs", d.PounceRangeTiles, d.PounceCooldownSeconds))
	}
	if d.PoisonChance > 0 {
		add(fmt.Sprintf("Poison: %.0f%% for %ds", d.PoisonChance*100, d.PoisonDurationSec))
	}
	if d.IgniteChance > 0 {
		addSchool("fire", fmt.Sprintf("Ignite: %.0f%% for %ds", d.IgniteChance*100, d.IgniteDurationSec))
	}
	if d.StunCharChance > 0 {
		add(fmt.Sprintf("Stun: %.0f%% (%ds / %d turns)", d.StunCharChance*100, d.StunCharSeconds, d.StunCharTurns))
	}
	if d.DispelChance > 0 {
		add(fmt.Sprintf("Dispel buff: %.0f%%", d.DispelChance*100))
	}
	if d.FireburstChance > 0 {
		addSchool("fire", fmt.Sprintf("Fireburst: %.0f%% for %d-%d", d.FireburstChance*100, d.FireburstDamageMin, d.FireburstDamageMax))
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
	return out
}

func normalizeEffectSchool(school string) string {
	school = strings.ToLower(strings.TrimSpace(school))
	if school == "" {
		return "physical"
	}
	return school
}
