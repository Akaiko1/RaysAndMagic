package config

import (
	"fmt"
	uitext "ugataima/assets/text"
)

// AttackZoneProcConfig casts a persistent zone on a committed weapon action.
// Its damage uses authored Intellect without the wielder's stats or mastery.
type AttackZoneProcConfig struct {
	Spell     string `yaml:"spell"`
	ChancePct int    `yaml:"chance_pct"`
	Intellect int    `yaml:"intellect"`
}

func (w *WeaponDefinitionConfig) validateReactiveEffects() error {
	if w.NightBaseDamageMultiplier < 0 || w.NightBaseDamageMultiplier == 1 || w.HitShellAbsorption < 0 {
		return fmt.Errorf("night multiplier must be zero or >=2; shell absorption must be nonnegative")
	}
	if p := w.AttackZoneProc; p != nil {
		if p.Spell == "" || p.ChancePct < 1 || p.ChancePct > 100 || p.Intellect < 1 {
			return fmt.Errorf("attack zone proc requires spell, chance_pct 1..100, positive intellect")
		}
	}
	return nil
}

// ValidateWeaponSpellReferences runs after both catalogs load in either binary.
func ValidateWeaponSpellReferences(weapons *WeaponSystemConfig, spells *SpellSystemConfig) error {
	if weapons == nil {
		return nil
	}
	for key, w := range weapons.Weapons {
		if w == nil || w.AttackZoneProc == nil {
			continue
		}
		p := w.AttackZoneProc
		if spells == nil || spells.Spells[p.Spell] == nil || spells.Spells[p.Spell].ZoneRadiusTiles <= 0 {
			return fmt.Errorf("weapon %q: proc spell %q must be a persistent zone", key, p.Spell)
		}
	}
	return nil
}

func (d *ItemDefinitionConfig) validateReactiveCard() error {
	if d.CardDodgeChargePct == 0 && d.CardDodgeChargeLimit == 0 && d.CardRepeatedHitReductionPct == 0 && d.CardRepeatedHitReductionCap == 0 && !d.CardHealingCleanse {
		return nil
	}
	if d.Type != "card" || d.CardDodgeChargePct < 0 || d.CardDodgeChargeLimit < 0 ||
		(d.CardDodgeChargePct == 0) != (d.CardDodgeChargeLimit == 0) ||
		d.CardRepeatedHitReductionPct < 0 || d.CardRepeatedHitReductionCap < 0 || d.CardRepeatedHitReductionCap > 100 ||
		(d.CardRepeatedHitReductionPct == 0) != (d.CardRepeatedHitReductionCap == 0) || d.CardRepeatedHitReductionPct > d.CardRepeatedHitReductionCap {
		return fmt.Errorf("invalid reactive card effect or stack limit")
	}
	return nil
}

func (w *WeaponDefinitionConfig) reactiveEffectLines() []string {
	var out []string
	if w.NightBaseDamageMultiplier > 0 {
		out = append(out, uitext.Text("weapon.night_base_multiplier", w.NightBaseDamageMultiplier))
	}
	if w.HitShellAbsorption > 0 {
		out = append(out, uitext.Text("weapon.hit_shell", w.HitShellAbsorption))
	}
	if p := w.AttackZoneProc; p != nil {
		name := p.Spell
		if GlobalSpells != nil {
			if def, ok := GlobalSpells.Spells[p.Spell]; ok {
				name = def.Name
			}
		}
		out = append(out, uitext.Text("weapon.attack_zone_proc", p.ChancePct, name, p.Intellect))
	}
	return out
}
