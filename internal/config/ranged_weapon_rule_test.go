package config

import (
	"strings"
	"testing"
)

// One rule decides "ranged": reach >= RangedWeaponMinRangeTiles. Every shipped
// ranged weapon carries projectile physics and every melee one carries melee
// config, and a projectile category authored with melee reach fails the load
// instead of swinging and doing nothing.
func TestRangedWeaponRuleIsOnePredicate(t *testing.T) {
	cfg, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for key, def := range cfg.Weapons {
		if def.IsRanged() != (def.Physics != nil) || !def.IsRanged() != (def.Melee != nil) {
			t.Errorf("%s: range %d ranged=%v but physics=%v melee=%v", key, def.Range, def.IsRanged(), def.Physics != nil, def.Melee != nil)
		}
	}
	for _, tc := range []struct {
		rangeTiles int
		ranged     bool
	}{{1, false}, {3, false}, {RangedWeaponMinRangeTiles, true}, {12, true}} {
		if got := (&WeaponDefinitionConfig{Range: tc.rangeTiles}).IsRanged(); got != tc.ranged {
			t.Errorf("range %d ranged=%v, want %v", tc.rangeTiles, got, tc.ranged)
		}
	}
	for _, category := range []string{"bow", "crossbow", "throwing", "blaster"} {
		bad := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"short": {Category: category, Range: 3}}}
		if err := validateWeaponConfig(bad); err == nil || !strings.Contains(err.Error(), "needs range >=") {
			t.Errorf("%s with melee reach passed validation: %v", category, err)
		}
	}
}

// The loader rejects a weapon below its class floor and a floor naming no
// authored category. At-floor weapons pass by loading the shipped catalog above.
func TestWeaponClassMinimums(t *testing.T) {
	melee := func(category string, rangeTiles, arc int) *WeaponDefinitionConfig {
		return &WeaponDefinitionConfig{Category: category, Range: rangeTiles, Melee: &MeleeAttackConfig{ArcType: arc},
			Graphics: &WeaponGraphicsConfig{SlashWidth: 1, SlashLength: 1}}
	}
	for _, tc := range []struct {
		def    *WeaponDefinitionConfig
		floors map[string]WeaponClassMinimum
		want   string
	}{
		{melee("spear", 1, 2), map[string]WeaponClassMinimum{"spear": {Range: 2}}, "range 1; the class minimum is 2"},
		{melee("mace", 1, 1), map[string]WeaponClassMinimum{"mace": {ArcType: 2}}, "arc_type 1; the class minimum is 2"},
		{melee("spear", 2, 1), map[string]WeaponClassMinimum{"spaer": {Range: 2}}, `"spaer"`},
	} {
		sys := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"w": tc.def}, WeaponClassMinimums: tc.floors}
		if err := validateWeaponConfig(sys); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
	}
}
