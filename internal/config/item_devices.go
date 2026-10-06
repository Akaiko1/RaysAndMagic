package config

import (
	"fmt"
	uitext "ugataima/assets/text"
)

func validateDeviceDefinition(key string, d *ItemDefinitionConfig) error {
	if d.Type != "device" && d.UseAction == "" && d.UseSpell == "" && d.UseCooldownSeconds == 0 && d.UseJumpExtraTiles == 0 {
		return nil
	}
	if d.Type != "device" || d.UseAction != "jump" || d.UseSpell == "" || d.UseCooldownSeconds <= 0 || d.UseJumpExtraTiles < 0 {
		return fmt.Errorf("item %q: device requires a supported use_action (jump), use_spell, positive use_cooldown_seconds and nonnegative use_jump_extra_tiles", key)
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
		if spells == nil || spells.Spells[d.UseSpell] == nil {
			return fmt.Errorf("item %q: use_spell %q does not exist", key, d.UseSpell)
		}
		if d.UseAction == "jump" && spells.Spells[d.UseSpell].JumpTiles <= 0 {
			return fmt.Errorf("item %q: use_spell %q has no jump capability", key, d.UseSpell)
		}
	}
	return nil
}

func (d *ItemDefinitionConfig) DeviceEffectLines() []string {
	if d.Type != "device" || d.UseAction != "jump" || GlobalSpells == nil {
		return nil
	}
	spell := GlobalSpells.Spells[d.UseSpell]
	if spell == nil {
		return nil
	}
	distance := spell.JumpTiles + float64(d.UseJumpExtraTiles)
	return []string{
		uitext.Text("item.device_jump", fmt.Sprintf("%g", distance)),
		uitext.Text("item.device_recharge", d.UseCooldownSeconds),
	}
}
