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
		{"root", MonsterDefinition{Abilities: Abilities{RootParty: TimedStatusAbility{Chance: .07, Seconds: 3}}}, true},
		{"root missing duration", MonsterDefinition{Abilities: Abilities{RootParty: TimedStatusAbility{Chance: .07}}}, false},
		{"root part of a round", MonsterDefinition{Abilities: Abilities{RootParty: TimedStatusAbility{Chance: .07, Seconds: 2}}}, false},
		{"root missing chance", MonsterDefinition{Abilities: Abilities{RootParty: TimedStatusAbility{Seconds: 3}}}, false},
		{"root bad chance", MonsterDefinition{Abilities: Abilities{RootParty: TimedStatusAbility{Chance: 1.1, Seconds: 3}}}, false},
		{"melt armor", MonsterDefinition{Abilities: Abilities{MeltArmor: ChanceAbility{Chance: 1}}}, true},
		{"melt armor bad chance", MonsterDefinition{Abilities: Abilities{MeltArmor: ChanceAbility{Chance: 1.5}}}, false},
		{"blink", MonsterDefinition{ProjectileSpell: "lightning", RangedAttackRange: 4, Abilities: Abilities{RearBlink: RearBlinkAbility{Chance: .07, RangeTiles: 4}}}, true},
		{"blink beyond reach", MonsterDefinition{ProjectileSpell: "lightning", RangedAttackRange: 4, Abilities: Abilities{RearBlink: RearBlinkAbility{Chance: .07, RangeTiles: 5}}}, false},
		{"blink missing attack", MonsterDefinition{RangedAttackRange: 4, Abilities: Abilities{RearBlink: RearBlinkAbility{Chance: .07, RangeTiles: 4}}}, false},
		{"blink missing chance", MonsterDefinition{ProjectileSpell: "lightning", RangedAttackRange: 4, Abilities: Abilities{RearBlink: RearBlinkAbility{RangeTiles: 4}}}, false},
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
