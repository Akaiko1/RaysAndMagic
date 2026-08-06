package config

import "testing"

func TestDamageSchoolValidationUsesSharedCatalog(t *testing.T) {
	t.Run("spell", func(t *testing.T) {
		cfg := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{
			"bad": {School: "arcane"},
		}}
		if err := validateSpellAuthoring(cfg); err == nil {
			t.Fatal("unknown spell school passed validation")
		}
	})

	t.Run("weapon", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {DamageType: "arcane"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("unknown weapon damage_type passed validation")
		}
	})

	t.Run("weapon projectile", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {ProjectileSchool: "arcane"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("unknown weapon projectile_school passed validation")
		}
	})

	t.Run("ranged staff requires projectile school", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {Category: "staff", Range: 6, DamageType: "air"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("ranged staff without projectile_school passed validation")
		}
	})

	t.Run("ranged staff cannot deal physical damage", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {Category: "staff", Range: 6, DamageType: "physical", ProjectileSchool: "air"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("physical ranged staff passed validation")
		}
	})

	t.Run("ranged staff damage and projectile schools must match", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {Category: "staff", Range: 6, DamageType: "fire", ProjectileSchool: "air"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("ranged staff with mismatched schools passed validation")
		}
	})

	t.Run("ranged book follows magic weapon validation", func(t *testing.T) {
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{
			"bad": {Category: "book", Range: 6, DamageType: "dark", ProjectileSchool: "air"},
		}}
		if err := validateWeaponConfig(cfg); err == nil {
			t.Fatal("ranged book with mismatched schools passed validation")
		}
	})

	t.Run("item resistance", func(t *testing.T) {
		cfg := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{
			"bad": {Resistances: map[string]int{"arcane": 10}},
		}}
		if err := validateItemConfig(cfg); err == nil {
			t.Fatal("unknown item resistance school passed validation")
		}
	})

	t.Run("trap", func(t *testing.T) {
		cfg := &TrapSystemConfig{Traps: map[string]*TrapDefinitionConfig{
			"bad": {
				Name: "Bad", Icon: "bad", Element: "arcane",
				Level: 1, SPCost: 1, CooldownSeconds: 1, LifetimeSeconds: 1,
				DamageBase: 1,
			},
		}}
		if err := validateTrapConfig(cfg); err == nil {
			t.Fatal("unknown trap element passed validation")
		}
	})

	t.Run("crate", func(t *testing.T) {
		cfg := &LootTablesConfig{Crates: map[string]*CrateConfig{
			"bad": {TrapDamage: 1, TrapDamageTypes: []string{"arcane"}},
		}}
		if err := validateCrates(cfg); err == nil {
			t.Fatal("unknown crate trap damage type passed validation")
		}
	})

	t.Run("champion magic school", func(t *testing.T) {
		cfg := &ChampionSystemConfig{
			Tiers: map[string]*ChampionTier{
				ChampionDefaultTier: {
					Level: 1, Mastery: "novice", HP: 1, Experience: 1, ArenaPoints: 1,
				},
			},
			Champions: map[string]*ChampionDefinition{
				"bad": {Name: "Bad", Class: "knight", Skills: []string{"sword"}, SpellSchools: []string{"arcane"}},
			},
		}
		if err := validateChampionConfig(cfg); err == nil {
			t.Fatal("unknown champion magic school passed validation")
		}
	})

	t.Run("level-up magic school", func(t *testing.T) {
		cfg := &LevelUpConfig{LevelUps: map[string]LevelUpClassConfig{
			"knight": {Levels: []LevelUpLevel{{
				Level: 3, Choices: []LevelUpChoice{{Type: "magic_mastery", School: "arcane"}},
			}}},
		}}
		if err := validateLevelUpConfig(cfg); err == nil {
			t.Fatal("unknown level-up magic school passed validation")
		}
	})
}

func TestDamageSchoolValidationCanonicalizesRuntimeValues(t *testing.T) {
	spell := &SpellDefinitionConfig{
		School:           " FIRE ",
		Schools:          []string{" DARK "},
		ResistBuffSchool: " WATER ",
		CooldownSeconds:  1,
	}
	spells := &SpellSystemConfig{Spells: map[string]*SpellDefinitionConfig{"test": spell}}
	if err := validateSpellAuthoring(spells); err != nil {
		t.Fatalf("validate spell: %v", err)
	}
	if spell.School != "fire" || spell.Schools[0] != "dark" || spell.ResistBuffSchool != "water" {
		t.Fatalf("spell schools were not canonicalized: %+v", spell)
	}

	item := &ItemDefinitionConfig{
		Resistances:     map[string]int{" FIRE ": 25},
		CardResistBonus: map[string]int{" DARK ": 50},
	}
	items := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": item}}
	if err := validateItemConfig(items); err != nil {
		t.Fatalf("validate item: %v", err)
	}
	if item.Resistances["fire"] != 25 || item.CardResistBonus["dark"] != 50 {
		t.Fatalf("item resistance keys were not canonicalized: %+v %+v", item.Resistances, item.CardResistBonus)
	}

	duplicate := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{
		"test": {Resistances: map[string]int{"fire": 10, " FIRE ": 20}},
	}}
	if err := validateItemConfig(duplicate); err == nil {
		t.Fatal("duplicate resistance aliases passed validation")
	}

	levelUps := &LevelUpConfig{LevelUps: map[string]LevelUpClassConfig{
		"knight": {Levels: []LevelUpLevel{{
			Level: 3, Choices: []LevelUpChoice{{Type: "magic_mastery", School: " DARK "}},
		}}},
	}}
	if err := validateLevelUpConfig(levelUps); err != nil {
		t.Fatalf("validate level-up: %v", err)
	}
	if got := levelUps.LevelUps["knight"].Levels[0].Choices[0].School; got != "dark" {
		t.Fatalf("level-up school = %q, want dark", got)
	}
}
