package character

import (
	"fmt"
	uitext "ugataima/assets/text"
	"ugataima/internal/config"
)

func (c *MMCharacter) TacticalSkillValue(skill SkillType, values [4]int) int {
	if c == nil || !c.HasSkill(skill) {
		return 0
	}
	return values[min(3, max(0, c.SkillTier(skill)))]
}

func tacticalSkillDescription(skill SkillType) (string, bool) {
	c := config.TacticalSkills()
	var key string
	var args []any
	add := func(values [4]int) {
		for _, v := range values {
			args = append(args, v)
		}
	}
	switch skill {
	case SkillOverwatch:
		key = "overwatch"
		add(c.OverwatchChance)
		args = append(args, c.OverwatchReadySeconds)
	case SkillBallistics:
		key = "ballistics"
		add(c.BallisticsSpeedPct)
		add(c.BallisticsRangeTiles)
	case SkillFieldMedicine:
		key = "field_medicine"
		add(c.MedicineRestorePct)
		add(c.MedicinePoisonReductionPct)
	case SkillDesignateTarget:
		key = "designate_target"
		add(c.DesignationSeconds)
		add(c.DesignationCritPct)
	default:
		return "", false
	}
	return fmt.Sprintf(c.Descriptions[key], args...), true
}

func BallisticsWeapon(def *config.WeaponDefinitionConfig) bool {
	return def != nil && def.Range > 3 && (def.Category == "bow" || def.Category == "blaster")
}

// EffectiveWeaponFlight is shared by launch, target eligibility and tooltips.
func EffectiveWeaponFlight(def *config.WeaponDefinitionConfig, c *MMCharacter) (rangeTiles, speedTiles float64) {
	if def == nil {
		return 0, 0
	}
	rangeTiles = float64(def.Range)
	if def.Physics != nil {
		speedTiles = def.Physics.SpeedTiles
	}
	if BallisticsWeapon(def) && c != nil {
		rules := config.TacticalSkills()
		rangeTiles += float64(c.TacticalSkillValue(SkillBallistics, rules.BallisticsRangeTiles))
		speedTiles *= 1 + float64(c.TacticalSkillValue(SkillBallistics, rules.BallisticsSpeedPct))/100
	}
	return
}

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
		amount = amount * (100 + c.TacticalSkillValue(SkillFieldMedicine, config.TacticalSkills().MedicineRestorePct)) / 100
	}
	return amount
}

func ConsumableRuleLines(def *config.ItemDefinitionConfig, bearer *MMCharacter) []string {
	if def == nil || def.Revive || (def.HealBase <= 0 && def.ManaBase <= 0) {
		return nil
	}
	c := config.TacticalSkills()
	v := c.MedicineRestorePct
	out := []string{uitext.Text("item.field_medicine_bonus", v[0], v[1], v[2], v[3])}
	if bearer != nil {
		out = append(out, uitext.Text("item.current_recovery", ConsumableRestore(bearer, def.HealBase, def.HealEnduranceDivisor, false), ConsumableRestore(bearer, def.ManaBase, def.ManaPersonalityDivisor, true)))
	}
	if c.AutoDrinkThresholdPct > 0 {
		line := uitext.Text("item.auto_drink_rule", c.AutoDrinkThresholdPct, c.AutoDrinkSeconds)
		if def.CurePoison {
			line = uitext.Text("item.auto_drink_when_poisoned", line)
		}
		out = append(out, line)
	}
	return out
}
