package monster

import "testing"

func TestPartyAbilityConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		def   MonsterDefinition
		valid bool
	}{
		{"none", MonsterDefinition{}, true},
		{"group", MonsterDefinition{Banding: true, BandGroup: "trial"}, true},
		{"group without banding", MonsterDefinition{BandGroup: "trial"}, false},
		{"root", MonsterDefinition{RootPartyChance: .07, RootPartySeconds: 2, RootPartyTurns: 1}, true},
		{"root missing duration", MonsterDefinition{RootPartyChance: .07, RootPartySeconds: 2}, false},
		{"root missing chance", MonsterDefinition{RootPartySeconds: 2, RootPartyTurns: 1}, false},
		{"root bad chance", MonsterDefinition{RootPartyChance: 1.1, RootPartySeconds: 2, RootPartyTurns: 1}, false},
		{"blink", MonsterDefinition{RearBlinkChance: .07, RearBlinkRangeTiles: 4, ProjectileSpell: "lightning", RangedAttackRange: 4}, true},
		{"blink beyond reach", MonsterDefinition{RearBlinkChance: .07, RearBlinkRangeTiles: 5, ProjectileSpell: "lightning", RangedAttackRange: 4}, false},
		{"blink missing attack", MonsterDefinition{RearBlinkChance: .07, RearBlinkRangeTiles: 4, RangedAttackRange: 4}, false},
		{"blink missing chance", MonsterDefinition{RearBlinkRangeTiles: 4, ProjectileSpell: "lightning", RangedAttackRange: 4}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Name = "Trial"
			tc.def.SizeClass = "person"
			err := validateMonsterConfiguration(&MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{"trial": tc.def}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
