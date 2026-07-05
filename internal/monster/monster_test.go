package monster

import (
	"math"
	"testing"
)

// Boss flags travel in pairs (chance + magnitude, evasive phase + tuning);
// load-time validation must reject a half-configured boss instead of letting
// the missing half silently zero out in code.
func TestValidateMonsterConfiguration_BossFlagPairs(t *testing.T) {
	cases := []struct {
		name    string
		def     MonsterDefinition
		wantErr bool
	}{
		{"inferno chance without damage", MonsterDefinition{InfernoChance: 0.1}, true},
		{"poison chance without duration", MonsterDefinition{PoisonChance: 0.2}, true},
		{"poison fully configured", MonsterDefinition{PoisonChance: 0.2, PoisonDurationSec: 15}, false},
		{"dormant boss (passive, no evade) is valid", MonsterDefinition{PassiveUntilQuest: "q"}, false},
		{"evasive without cooldown", MonsterDefinition{PassiveUntilQuest: "q", EvadeRadiusTiles: 3}, true},
		{"evasive fully configured", MonsterDefinition{PassiveUntilQuest: "q", EvadeRadiusTiles: 3, BossCooldownSecs: 1}, false},
		{"summon chance without monsters", MonsterDefinition{SummonChance: 0.2}, true},
		{"summon configured", MonsterDefinition{SummonChance: 0.2, SummonMonsters: []string{"rat"}}, false},
		{"dragon breath chance without damage type", MonsterDefinition{DragonBreathChance: 0.33}, true},
		{"dragon breath configured", MonsterDefinition{DragonBreathChance: 0.33, DragonBreathType: "fire"}, false},
		{"enrage without effect", MonsterDefinition{EnrageAtHP: 100}, true},
		{"enrage with damage mult", MonsterDefinition{EnrageAtHP: 100, EnrageDamageMult: 1.5}, false},
		{"fully configured boss", MonsterDefinition{
			InfernoChance: 0.1, InfernoDamage: 28,
			PassiveUntilQuest: "q", EvadeRadiusTiles: 3, BossCooldownSecs: 1,
			SummonChance: 0.1, SummonMonsters: []string{"rat"},
			EnrageAtHP: 100, EnrageCooldownMult: 0.6,
		}, false},
	}
	for _, tc := range cases {
		cfg := &MonsterYAMLConfig{
			Monsters:    map[string]MonsterDefinition{"boss": tc.def},
			DamageTypes: map[string]int{"physical": 0, "fire": 1},
		}
		err := validateMonsterConfiguration(cfg)
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

func TestValidateMonsterConfiguration_AttackCadenceAllowsExplicitTurnBasedOverride(t *testing.T) {
	tests := []struct {
		name    string
		def     MonsterDefinition
		wantErr bool
	}{
		{name: "default single attack", def: MonsterDefinition{}},
		{name: "cooldown-only derives turn cadence", def: MonsterDefinition{AttackCooldownMult: 0.6}},
		{name: "two attacks with half cooldown", def: MonsterDefinition{AttacksPerRound: 2, AttackCooldownMult: 0.5}},
		{name: "four attacks with quarter cooldown", def: MonsterDefinition{AttacksPerRound: 4, AttackCooldownMult: 0.25}},
		{name: "explicit TB-only multiattack", def: MonsterDefinition{AttacksPerRound: 2}},
		{name: "explicit RT/TB desync", def: MonsterDefinition{AttacksPerRound: 1, AttackCooldownMult: 0.3}},
		{name: "explicit multiattack desync", def: MonsterDefinition{AttacksPerRound: 4, AttackCooldownMult: 0.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{"monster": tt.def}}
			err := validateMonsterConfiguration(cfg)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestSetupMonsterFromConfig_CooldownOnlyRangedDerivesTurnBasedAttacks(t *testing.T) {
	m := &Monster3D{Resistances: make(map[DamageType]int)}
	m.SetupMonsterFromConfig(&MonsterDefinition{
		Name:               "Fast Archer",
		ProjectileWeapon:   "short_bow",
		AttackCooldownMult: 0.6,
	})

	if !m.HasRangedAttack() {
		t.Fatal("test monster should be ranged")
	}
	if got := m.GetTurnBasedAttackCount(); got != 2 {
		t.Fatalf("cooldown-only ranged TB attacks = %d, want 2", got)
	}
}

func TestSetupMonsterFromConfig_ExplicitAttacksPerRoundOverridesCooldown(t *testing.T) {
	m := &Monster3D{Resistances: make(map[DamageType]int)}
	m.SetupMonsterFromConfig(&MonsterDefinition{
		Name:               "TB Balanced Archer",
		ProjectileWeapon:   "short_bow",
		AttacksPerRound:    1,
		AttackCooldownMult: 0.3,
	})

	if got := m.GetTurnBasedAttackCount(); got != 1 {
		t.Fatalf("explicit attacks_per_round override = %d, want 1", got)
	}
}

func TestNewMonster3DFromConfig_Valid(t *testing.T) {
	// This assumes TestMain loads the config and 'goblin' exists in monsters.yaml
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Did not expect panic with valid config: %v", r)
		}
	}()
	m := NewMonster3DFromConfig(1, 2, "goblin", nil)
	if m == nil || m.Name == "" {
		t.Error("Expected valid Monster3D instance with name")
	}
}

func TestDragonBreathLoadedFromConfig(t *testing.T) {
	tests := []struct {
		key        string
		damageType string
	}{
		{"dragon", "fire"},
		{"dragon_red", "dark"},
		{"dragon_green", "earth"},
		{"dragon_gold", "air"},
		{"elder_dragon", "fire"},
		{"elder_dragon_red", "dark"},
		{"elder_dragon_green", "earth"},
		{"elder_dragon_gold", "air"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := NewMonster3DFromConfig(0, 0, tt.key, nil)
			if math.Abs(m.DragonBreathChance-0.33) > 0.0001 {
				t.Fatalf("DragonBreathChance = %v, want 0.33", m.DragonBreathChance)
			}
			if m.DragonBreathDamageType != tt.damageType {
				t.Fatalf("DragonBreathDamageType = %q, want %q", m.DragonBreathDamageType, tt.damageType)
			}
			if (tt.key == "dragon" || tt.key == "elder_dragon") && m.FireburstChance != 0 {
				t.Fatalf("%s still has FireburstChance %v", tt.key, m.FireburstChance)
			}
		})
	}
}

func TestMonsterLetterResolutionPrefersBiomeSpecificDefinition(t *testing.T) {
	_, key, err := MonsterConfig.GetMonsterByLetterForBiome("o", "water")
	if err != nil {
		t.Fatalf("resolve water o: %v", err)
	}
	if key != "octopus" {
		t.Fatalf("expected water o to resolve to octopus, got %s", key)
	}

	_, key, err = MonsterConfig.GetMonsterByLetterForBiome("o", "forest")
	if err != nil {
		t.Fatalf("resolve forest o: %v", err)
	}
	if key != "orc" {
		t.Fatalf("expected forest o to resolve to orc, got %s", key)
	}
}

func TestWaterMonsterBehaviorConfig(t *testing.T) {
	medusa := NewMonster3DFromConfig(0, 0, "medusa", nil)
	if !medusa.PassiveUntilAttacked {
		t.Fatalf("medusa should be passive until attacked")
	}

	octopus := NewMonster3DFromConfig(0, 0, "octopus", nil)
	if got := octopus.GetTurnBasedAttackCount(); got != 4 {
		t.Fatalf("expected octopus to attack 4 times in turn-based mode, got %d", got)
	}
	if octopus.AttackCooldownMultiplier != 0.25 {
		t.Fatalf("expected octopus real-time cooldown multiplier 0.25, got %v", octopus.AttackCooldownMultiplier)
	}
}
