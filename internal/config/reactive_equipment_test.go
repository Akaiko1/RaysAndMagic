package config

import "testing"

func TestReactiveEquipmentValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		def   WeaponDefinitionConfig
		valid bool
	}{
		{"none", WeaponDefinitionConfig{}, true},
		{"night", WeaponDefinitionConfig{NightBaseDamageMultiplier: 2}, true},
		{"meaningless_night", WeaponDefinitionConfig{NightBaseDamageMultiplier: 1}, false},
		{"negative_shell", WeaponDefinitionConfig{HitShellAbsorption: -1}, false},
		{"proc", WeaponDefinitionConfig{AttackZoneProc: &AttackZoneProcConfig{Spell: "zone", ChancePct: 3, Intellect: 50}}, true},
		{"missing_spell", WeaponDefinitionConfig{AttackZoneProc: &AttackZoneProcConfig{ChancePct: 3, Intellect: 50}}, false},
		{"chance_overflow", WeaponDefinitionConfig{AttackZoneProc: &AttackZoneProcConfig{Spell: "zone", ChancePct: 101, Intellect: 50}}, false},
		{"no_intellect", WeaponDefinitionConfig{AttackZoneProc: &AttackZoneProcConfig{Spell: "zone", ChancePct: 3}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.def.validateReactiveEffects() == nil) != tc.valid {
				t.Fatal("unexpected weapon validation")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		def   ItemDefinitionConfig
		valid bool
	}{
		{"none", ItemDefinitionConfig{}, true},
		{"dodge", ItemDefinitionConfig{Type: "card", CardDodgeChargePct: 25, CardDodgeChargeLimit: 4}, true},
		{"no_limit", ItemDefinitionConfig{Type: "card", CardDodgeChargePct: 25}, false},
		{"wrong_type", ItemDefinitionConfig{Type: "trinket", CardHealingCleanse: true}, false},
		{"reduction", ItemDefinitionConfig{Type: "card", CardRepeatedHitReductionPct: 5, CardRepeatedHitReductionCap: 30}, true},
		{"overflow", ItemDefinitionConfig{Type: "card", CardRepeatedHitReductionPct: 5, CardRepeatedHitReductionCap: 101}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.def.validateReactiveCard() == nil) != tc.valid {
				t.Fatal("unexpected card validation")
			}
		})
	}
}
