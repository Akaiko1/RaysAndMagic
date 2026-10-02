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
