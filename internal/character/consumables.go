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

// AddConsumableDetails explains only the restoration actually applied to this bearer.
func AddConsumableDetails(section *CardSection, def *config.ItemDefinitionConfig, bearer *MMCharacter) {
	if def == nil {
		return
	}
	if bearer == nil || def.Revive {
		for _, line := range def.RecoveryLines() {
			section.Add("%s", line)
		}
		return
	}
	for _, resource := range []struct {
		name, stat       string
		base, div, value int
		mana             bool
	}{
		{"HP", "Endurance", def.HealBase, def.HealEnduranceDivisor, bearer.GetEffectiveEndurance(), false},
		{"SP", "Personality", def.ManaBase, def.ManaPersonalityDivisor, bearer.GetEffectivePersonality(), true},
	} {
		if resource.base <= 0 {
			continue
		}
		section.AddDetail("Base recovery: %d %s", resource.base, resource.name)
		if resource.div > 0 {
			section.AddDetail("%s (%d / %d): +%d %s", resource.stat, resource.value, resource.div, resource.value/resource.div, resource.name)
		}
		if bearer.HasSkill(SkillFieldMedicine) {
			section.AddDetail("Field Medicine: +%d%% %s recovery", FieldMedicineRestorePct(bearer.SkillTier(SkillFieldMedicine)), resource.name)
		}
		amount := ConsumableRestore(bearer, resource.base, resource.div, resource.mana)
		if resource.mana {
			section.Add("%s", uitext.Text("item.current_sp_recovery", amount))
		} else {
			section.Add("%s", uitext.Text("item.current_hp_recovery", amount))
		}
	}
}

// AddConsumableUsage keeps automatic and manual use conditions together.
func AddConsumableUsage(section *CardSection, def *config.ItemDefinitionConfig) {
	if def == nil || def.Revive || (def.HealBase <= 0 && def.ManaBase <= 0) {
		return
	}
	c := config.AutoDrinkSettings()
	if c.ThresholdPct > 0 && def.SummonDistanceTiles <= 0 {
		resource := "HP"
		if def.HealBase <= 0 {
			resource = "SP"
		} else if def.ManaBase > 0 {
			resource = "HP or SP"
		}
		line := uitext.Text("item.auto_drink_condition", resource, c.ThresholdPct)
		if def.CurePoison {
			line = uitext.Text("item.auto_drink_when_poisoned", line)
		}
		section.Add("%s", line)
		section.AddDetail("%s", uitext.Text("item.auto_drink_details", c.IntervalSeconds))
	}
}
