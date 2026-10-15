package config

import (
	"fmt"
	uitext "ugataima/assets/text"
)

func validateDeviceDefinition(key string, d *ItemDefinitionConfig) error {
	if d.Type != "device" && d.UseAction == "" && d.UseSpell == "" && d.UseCooldownSeconds == 0 && d.UseJumpExtraTiles == 0 {
		return nil
	}
	if d.Type != "device" || d.UseCooldownSeconds <= 0 {
		return fmt.Errorf("item %q: device requires positive use_cooldown_seconds", key)
	}
	switch d.UseAction {
	case "throw_flask":
		if d.Flask == nil || d.UseSpell != "" || d.UseJumpExtraTiles != 0 || d.SummonDistanceTiles != 0 {
			return fmt.Errorf("item %q: throw_flask device requires a flask and no jump or summon fields", key)
		}
	case "jump":
		if d.UseSpell == "" || d.UseJumpExtraTiles < 0 || d.SummonDistanceTiles != 0 {
			return fmt.Errorf("item %q: jump device requires use_spell, nonnegative use_jump_extra_tiles and no summon distance", key)
		}
	case "summon":
		if d.SummonDistanceTiles <= 0 || d.UseSpell != "" || d.UseJumpExtraTiles != 0 {
			return fmt.Errorf("item %q: summon device requires positive summon_distance_tiles and no jump fields", key)
		}
	default:
		return fmt.Errorf("item %q: unsupported device use_action %q", key, d.UseAction)
	}
	return nil
}

// Both binaries validate cross-catalog links after their shared startup load.
// A device's action is explicit; the item category alone grants no capability.
func ValidateDeviceSpellReferences(catalog *ItemSystemConfig, spells *SpellSystemConfig) error {
	if catalog == nil {
		return nil
	}
	for key, d := range catalog.Items {
		if d == nil || d.Type != "device" {
			continue
		}
		if err := validateDeviceDefinition(key, d); err != nil {
			return err
		}
		if d.UseAction != "jump" {
			continue
		}
		if spells == nil || spells.Spells[d.UseSpell] == nil {
			return fmt.Errorf("item %q: use_spell %q does not exist", key, d.UseSpell)
		}
		if d.UseAction == "jump" && spells.Spells[d.UseSpell].JumpTiles <= 0 {
			return fmt.Errorf("item %q: use_spell %q has no jump capability", key, d.UseSpell)
		}
	}
	return nil
}

// DeviceRechargeLine is every device's recharge line.
func (d *ItemDefinitionConfig) DeviceRechargeLine() string {
	return uitext.Text("item.device_recharge", d.UseCooldownSeconds)
}

func (d *ItemDefinitionConfig) DeviceEffectLines() []string {
	if d.Type != "device" {
		return nil
	}
	if d.UseAction == "summon" {
		return []string{uitext.Text("item.device_summon", d.SummonDistanceTiles), d.DeviceRechargeLine()}
	}
	if d.UseAction != "jump" || GlobalSpells == nil {
		return nil
	}
	spell := GlobalSpells.Spells[d.UseSpell]
	if spell == nil {
		return nil
	}
	distance := spell.JumpTiles + float64(d.UseJumpExtraTiles)
	return []string{
		uitext.Text("item.device_jump", fmt.Sprintf("%g", distance)),
		d.DeviceRechargeLine(),
	}
}
