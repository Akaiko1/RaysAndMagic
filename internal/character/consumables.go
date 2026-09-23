package character

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/config"
)

// ConsumableRestore includes recipient attributes and Field Medicine exactly once.
func ConsumableRestore(c *MMCharacter, base, divisor int, mana bool) int {
	if base <= 0 {
		return 0
	}
	amount := base
	if c != nil {
		if divisor > 0 {
			if mana {
				amount += c.GetEffectivePersonality() / divisor
			} else {
				amount += c.GetEffectiveEndurance() / divisor
			}
		}
		if c.HasSkill(SkillFieldMedicine) {
			amount = amount * (100 + FieldMedicineRestorePct(c.SkillTier(SkillFieldMedicine))) / 100
		}
	}
	return amount
}

func ConsumableRuleLines(def *config.ItemDefinitionConfig, bearer *MMCharacter) []string {
	if def == nil || def.Revive || (def.HealBase <= 0 && def.ManaBase <= 0) {
		return nil
	}
	c := config.AutoDrinkSettings()
	v := fieldMedicineRestorePct
	out := []string{uitext.Text("item.field_medicine_bonus", v[0], v[1], v[2], v[3])}
	if bearer != nil {
		out = append(out, uitext.Text("item.current_recovery", ConsumableRestore(bearer, def.HealBase, def.HealEnduranceDivisor, false), ConsumableRestore(bearer, def.ManaBase, def.ManaPersonalityDivisor, true)))
	}
	if c.ThresholdPct > 0 {
		line := uitext.Text("item.auto_drink_rule", c.ThresholdPct, c.IntervalSeconds)
		if def.CurePoison {
			line = uitext.Text("item.auto_drink_when_poisoned", line)
		}
		out = append(out, line)
	}
	return out
}
