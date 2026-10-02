package monster

import "testing"

// Boss flags travel in pairs (chance + magnitude, evasive phase + tuning);
// load-time validation must reject a half-configured boss instead of letting
// the missing half silently zero out in code.
func TestValidateMonsterConfiguration_BossFlagPairs(t *testing.T) {
	cases := []struct {
		name    string
		def     MonsterDefinition
		wantErr bool
	}{
		{"inferno chance without damage", MonsterDefinition{Boss: true, InfernoChance: 0.1}, true},
		{"inferno chance without range", MonsterDefinition{Boss: true, InfernoChance: 0.1, InfernoDamage: 28}, true},
		{"inferno fully configured", MonsterDefinition{Boss: true, InfernoChance: 0.1, InfernoDamage: 28, InfernoRangeTiles: 8}, false},
		{"poison chance without duration", MonsterDefinition{PoisonChance: 0.2}, true},
		{"poison fully configured", MonsterDefinition{PoisonChance: 0.2, PoisonDurationSec: 15}, false},
		{"boss-only behavior needs boss flag", MonsterDefinition{SummonChance: 0.2, SummonMonsters: []string{"rat"}}, true},
		{"bare boss is valid", MonsterDefinition{Boss: true}, false},
		{"dormant boss (passive, no evade) is valid", MonsterDefinition{Boss: true, PassiveUntilQuest: "q"}, false},
		{"evasive without cooldown", MonsterDefinition{Boss: true, PassiveUntilQuest: "q", EvadeRadiusTiles: 3}, true},
		{"evasive fully configured", MonsterDefinition{Boss: true, PassiveUntilQuest: "q", EvadeRadiusTiles: 3, BossCooldownSecs: 1}, false},
		{"summon chance without monsters", MonsterDefinition{Boss: true, SummonChance: 0.2}, true},
		{"summon configured", MonsterDefinition{Boss: true, SummonChance: 0.2, SummonMonsters: []string{"rat"}}, false},
		{"dragon breath chance without damage type", MonsterDefinition{DragonBreathChance: 0.33}, true},
		{"dragon breath configured", MonsterDefinition{DragonBreathChance: 0.33, DragonBreathType: "fire"}, false},
		{"enrage without effect", MonsterDefinition{Boss: true, EnrageAtHP: 100}, true},
		{"enrage with damage mult", MonsterDefinition{Boss: true, EnrageAtHP: 100, EnrageDamageMult: 1.5}, false},
		{"fully configured boss", MonsterDefinition{
			Boss:          true,
			InfernoChance: 0.1, InfernoDamage: 28, InfernoRangeTiles: 8,
			PassiveUntilQuest: "q", EvadeRadiusTiles: 3, BossCooldownSecs: 1,
			SummonChance: 0.1, SummonMonsters: []string{"rat"},
			EnrageAtHP: 100, EnrageCooldownMult: 0.6,
		}, false},
	}
	for _, tc := range cases {
		if tc.def.SizeClass == "" {
			tc.def.SizeClass = "person" // these cases exercise other rules, not size
		}
		cfg := &MonsterYAMLConfig{
			Monsters: map[string]MonsterDefinition{"boss": tc.def},
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

func TestSetupMonsterFromConfig_CopiesBossClassification(t *testing.T) {
	m := &Monster3D{Resistances: make(map[DamageType]int)}
	m.SetupMonsterFromConfig(&MonsterDefinition{Name: "Boss", Boss: true})
	if !m.IsBoss() {
		t.Fatal("boss: true must be copied to the runtime monster")
	}
}

func TestSetupMonsterFromConfig_CachesSprite(t *testing.T) {
	m := &Monster3D{Resistances: make(map[DamageType]int)}
	m.SetupMonsterFromConfig(&MonsterDefinition{Name: "Dragon", Sprite: "dragon_gold"})
	if got := m.GetSpriteType(); got != "dragon_gold" {
		t.Fatalf("sprite = %q, want dragon_gold", got)
	}
}

func TestValidateMonsterConfiguration_AlarmRally(t *testing.T) {
	cases := []struct {
		name    string
		def     MonsterDefinition
		wantErr bool
	}{
		{"uncapped rally is valid", MonsterDefinition{RallyOnAggroTiles: 12}, false},
		{"capped rally is valid", MonsterDefinition{RallyOnAggroTiles: 12, RallyMaxTargets: 4}, false},
		{"negative cap is invalid", MonsterDefinition{RallyOnAggroTiles: 12, RallyMaxTargets: -1}, true},
		{"cap without rally is invalid", MonsterDefinition{RallyMaxTargets: 4}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
				"alarm": {SizeClass: "person", RallyOnAggroTiles: tc.def.RallyOnAggroTiles, RallyMaxTargets: tc.def.RallyMaxTargets},
			}}
			err := validateMonsterConfiguration(cfg)
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestParseDamageTypeNormalizesExternalKeys(t *testing.T) {
	if got, err := ParseDamageType(" FIRE "); err != nil || got != DamageFire {
		t.Fatalf("ParseDamageType( FIRE ) = (%v, %v), want (%v, nil)", got, err, DamageFire)
	}
}

func TestValidateMonsterConfigurationRejectsUnknownResistanceSchool(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"bad_resist": {
			SizeClass:   "person",
			Resistances: map[string]int{"flame-ish": 50},
		},
	}}
	if err := validateMonsterConfiguration(cfg); err == nil {
		t.Fatal("unknown resistance school passed validation")
	}
}

func TestValidateMonsterConfigurationCanonicalizesDamageSchools(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"typed": {
			SizeClass:        "person",
			DragonBreathType: " FIRE ",
			Resistances:      map[string]int{" DARK ": 50},
		},
	}}
	if err := validateMonsterConfiguration(cfg); err != nil {
		t.Fatalf("validate monster: %v", err)
	}
	def := cfg.Monsters["typed"]
	if def.DragonBreathType != "fire" || def.Resistances["dark"] != 50 {
		t.Fatalf("damage schools were not canonicalized: breath=%q resistances=%v", def.DragonBreathType, def.Resistances)
	}

	cfg.Monsters["typed"] = MonsterDefinition{
		SizeClass:   "person",
		Resistances: map[string]int{"dark": 10, " DARK ": 20},
	}
	if err := validateMonsterConfiguration(cfg); err == nil {
		t.Fatal("duplicate resistance aliases passed validation")
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
			if tt.def.SizeClass == "" {
				tt.def.SizeClass = "person" // these cases exercise cadence, not size
			}
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

// Dragon breath travels from the definition to the runtime monster unchanged;
// the synthetic row keeps the rule covered whatever the catalog authors.
func TestDragonBreathLoadedFromConfig(t *testing.T) {
	defs := map[string]MonsterDefinition{"synthetic": {Name: "Breather", DragonBreathChance: 0.5, DragonBreathType: "earth"}}
	for key, def := range MonsterConfig.Monsters {
		if def.DragonBreathChance > 0 {
			defs[key] = def
		}
	}
	for key, def := range defs {
		t.Run(key, func(t *testing.T) {
			var m *Monster3D
			if key == "synthetic" {
				m = &Monster3D{Resistances: make(map[DamageType]int)}
				m.SetupMonsterFromConfig(&def)
			} else {
				m = NewMonster3DFromConfig(0, 0, key, nil)
			}
			if m.DragonBreathChance != def.DragonBreathChance || m.DragonBreathDamageType != def.DragonBreathType {
				t.Fatalf("breath = %v/%q, want %v/%q", m.DragonBreathChance, m.DragonBreathDamageType, def.DragonBreathChance, def.DragonBreathType)
			}
		})
	}
}

// A biome-specific definition wins over a universal one sharing its letter;
// the universal one serves every other biome.
func TestMonsterLetterResolutionPrefersBiomeSpecificDefinition(t *testing.T) {
	cfg := &MonsterYAMLConfig{Monsters: map[string]MonsterDefinition{
		"sea_o":    {Letter: "o", Biomes: []string{"water"}},
		"forest_o": {Letter: "o", Biomes: []string{"forest"}},
		"plain_o":  {Letter: "o"},
	}}
	for _, tc := range []struct{ biome, want string }{
		{"water", "sea_o"},
		{"forest", "forest_o"},
		{"desert", "plain_o"},
		{"", "plain_o"},
	} {
		if _, key, err := cfg.GetMonsterByLetterForBiome("o", tc.biome); err != nil || key != tc.want {
			t.Errorf("biome %q: o resolved to %q (err %v), want %q", tc.biome, key, err, tc.want)
		}
	}
	if _, _, err := cfg.GetMonsterByLetterForBiome("q", "water"); err == nil {
		t.Error("an unknown letter resolved")
	}
}

// passive_until_attacked and the attack cadence travel from every definition to
// the runtime monster; the synthetic row keeps the rule covered.
func TestSetupMonsterFromConfig_CopiesPassiveAndCadence(t *testing.T) {
	check := func(t *testing.T, m *Monster3D, def MonsterDefinition) {
		t.Helper()
		if m.PassiveUntilAttacked != def.PassiveUntilHit || m.AttackCooldownMultiplier != def.AttackCooldownMult {
			t.Fatalf("passive=%v cooldown=%v, want %v/%v", m.PassiveUntilAttacked, m.AttackCooldownMultiplier, def.PassiveUntilHit, def.AttackCooldownMult)
		}
		if def.AttacksPerRound > 0 && m.GetTurnBasedAttackCount() != def.AttacksPerRound {
			t.Fatalf("TB attacks = %d, want authored %d", m.GetTurnBasedAttackCount(), def.AttacksPerRound)
		}
	}
	t.Run("synthetic", func(t *testing.T) {
		def := MonsterDefinition{Name: "Probe", PassiveUntilHit: true, AttacksPerRound: 4, AttackCooldownMult: 0.25}
		m := &Monster3D{Resistances: make(map[DamageType]int)}
		m.SetupMonsterFromConfig(&def)
		check(t, m, def)
	})
	for key, def := range MonsterConfig.Monsters {
		t.Run(key, func(t *testing.T) { check(t, NewMonster3DFromConfig(0, 0, key, nil), def) })
	}
}
