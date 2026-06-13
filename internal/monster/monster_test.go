package monster

import (
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
		{"evasive without radius", MonsterDefinition{PassiveUntilQuest: "q", BossCooldownSecs: 1}, true},
		{"evasive without cooldown", MonsterDefinition{PassiveUntilQuest: "q", EvadeRadiusTiles: 3}, true},
		{"fully configured boss", MonsterDefinition{
			InfernoChance: 0.1, InfernoDamage: 28,
			PassiveUntilQuest: "q", EvadeRadiusTiles: 3, BossCooldownSecs: 1,
		}, false},
	}
	for _, tc := range cases {
		cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{"boss": tc.def}}
		err := validateMonsterConfiguration(cfg)
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

func TestValidateMonsterConfiguration_AttackCadenceMatchesModes(t *testing.T) {
	tests := []struct {
		name    string
		def     MonsterDefinition
		wantErr bool
	}{
		{name: "default single attack", def: MonsterDefinition{}},
		{name: "two attacks with half cooldown", def: MonsterDefinition{AttacksPerRound: 2, AttackCooldownMult: 0.5}},
		{name: "four attacks with quarter cooldown", def: MonsterDefinition{AttacksPerRound: 4, AttackCooldownMult: 0.25}},
		{name: "two attacks missing multiplier", def: MonsterDefinition{AttacksPerRound: 2}, wantErr: true},
		{name: "four attacks with half cooldown", def: MonsterDefinition{AttacksPerRound: 4, AttackCooldownMult: 0.5}, wantErr: true},
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
