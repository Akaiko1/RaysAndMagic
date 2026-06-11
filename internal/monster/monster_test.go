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
	if octopus.AttackCooldownMultiplier != 0.5 {
		t.Fatalf("expected octopus real-time cooldown multiplier 0.5, got %v", octopus.AttackCooldownMultiplier)
	}
}
