package character

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/config"
)

// ConsumableRestore includes recipient attributes and Field Medicine exactly once.
func ConsumableRestore(c *MMCharacter, base, divisor int, mana bool, support ...PotionSupport) int {
	if base <= 0 {
		return 0
	}
	bonus := PotionSupport{}
	if len(support) > 0 {
		bonus = support[0]
	}
	pct := bonus.HPPct
	flat := bonus.HPFlat
	if mana {
		pct, flat = bonus.SPPct, 0
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
			pct += FieldMedicineRestorePct(c.SkillTier(SkillFieldMedicine))
		}
	}
	return amount*(100+pct)/100 + flat
}

// AddConsumableDetails explains only the restoration actually applied to this bearer.
func AddConsumableDetails(section *CardSection, def *config.ItemDefinitionConfig, bearer *MMCharacter, support ...PotionSupport) {
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
		if len(support) > 0 {
			bonus := support[0]
			if resource.mana && len(support) > 1 {
				bonus = support[1]
			}
			pct, flat := bonus.HPPct, bonus.HPFlat
			if resource.mana {
				pct, flat = bonus.SPPct, 0
			}
			if pct > 0 {
				section.AddDetail("Pharmacology (%s): +%d%% and +%d %s", bonus.Source, pct, flat, resource.name)
			}
		}
		selected := support
		if resource.mana && len(support) > 1 {
			selected = support[1:]
		}
		amount := ConsumableRestore(bearer, resource.base, resource.div, resource.mana, selected...)
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

// PotionSupport is one coherent Alchemist contribution, resolved for this potion
// and recipient. No tier or Intellect is borrowed from a different source.
type PotionSupport struct {
	Source               string
	HPPct, SPPct, HPFlat int
}

func (p *Party) PotionSupport(recipient *MMCharacter, base, div int, mana bool) PotionSupport {
	best := PotionSupport{}
	bestAmount := ConsumableRestore(recipient, base, div, mana)
	if p == nil {
		return best
	}
	for _, source := range p.Members {
		if source == nil || source.Class != ClassAlchemist || !source.CanUseCombatAction() || source.HasCondition(ConditionDead) || source.HasCondition(ConditionEradicated) || !source.HasSkill(SkillPharmacology) {
			continue
		}
		tier := source.SkillTier(SkillPharmacology)
		bonus := PotionSupport{Source: source.Name, HPPct: PharmacologyHP(tier), SPPct: PharmacologySP(tier), HPFlat: source.GetEffectiveIntellect() / 3}
		if amount := ConsumableRestore(recipient, base, div, mana, bonus); amount > bestAmount {
			best, bestAmount = bonus, amount
		}
	}
	return best
}
