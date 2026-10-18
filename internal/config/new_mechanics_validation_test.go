package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validMechanicTestWeapon() *WeaponDefinitionConfig {
	return &WeaponDefinitionConfig{
		Name:       "Test Blade",
		Category:   "sword",
		DamageType: "physical",
		Range:      1,
		Melee:      &MeleeAttackConfig{ArcType: 1},
		Graphics:   &WeaponGraphicsConfig{SlashWidth: 1, SlashLength: 1},
	}
}

func TestValidateWeaponConfigRejectsIncompleteNewMechanics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WeaponDefinitionConfig)
	}{
		{"ignite chance without duration", func(w *WeaponDefinitionConfig) { w.IgniteChance = 0.5 }},
		{"poison duration without chance", func(w *WeaponDefinitionConfig) { w.PoisonSeconds = 5 }},
		{"slow over one hundred", func(w *WeaponDefinitionConfig) { w.SlowPct, w.SlowSeconds = 101, 5 }},
		{"weaken without duration", func(w *WeaponDefinitionConfig) { w.WeakenPct = 25 }},
		{"stun chance without duration", func(w *WeaponDefinitionConfig) { w.StunChance = 0.1 }},
		{"root for part of a round", func(w *WeaponDefinitionConfig) { w.RootChance, w.RootSeconds = 0.1, 4 }},
		{"shred for part of a round", func(w *WeaponDefinitionConfig) { w.ArmorShredPct, w.ArmorShredSeconds = 20, 10 }},
		{"poison for part of a round", func(w *WeaponDefinitionConfig) { w.PoisonChance, w.PoisonSeconds = 0.2, 20 }},
		{"slow off the skip step", func(w *WeaponDefinitionConfig) { w.SlowPct, w.SlowSeconds = 30, 6 }},
		{"partial death burst", func(w *WeaponDefinitionConfig) { w.DeathBurstMaxHPPct = 10 }},
		{"death burst over one hundred", func(w *WeaponDefinitionConfig) { w.DeathBurstMaxHPPct, w.DeathBurstRadiusTiles = 101, 2 }},
		{"true share over one hundred", func(w *WeaponDefinitionConfig) { w.TrueDamagePct = 101 }},
		{"skirmish on a melee weapon", func(w *WeaponDefinitionConfig) {
			w.Signature = &WeaponSignatureConfig{Kind: SignatureSkirmish, ChargeLimit: 1, ChargeMoveTiles: 1}
		}},
		{"echo over one hundred", func(w *WeaponDefinitionConfig) { w.SpellEchoPct = 101 }},
		{"fresh-target multiplier alone", func(w *WeaponDefinitionConfig) { w.FreshTargetMultiplier = 0.5 }},
		{"hour strike that weakens", func(w *WeaponDefinitionConfig) { w.HourStrikeMarks, w.HourStrikeMultiplier = 12, 0.5 }},
		{"jam off the round", func(w *WeaponDefinitionConfig) { w.JamSeconds = 2 }},
		{"jam of a whole round", func(w *WeaponDefinitionConfig) { w.JamSeconds = CombatRoundSeconds }},
		{"ricochet without range", func(w *WeaponDefinitionConfig) { w.RicochetTargets = 1 }},
		{"ricochet on melee", func(w *WeaponDefinitionConfig) {
			w.RicochetTargets, w.RicochetRangeTiles = 1, 6
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weapon := validMechanicTestWeapon()
			tt.mutate(weapon)
			cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": weapon}}
			if err := validateWeaponConfig(cfg); err == nil {
				t.Fatal("invalid mechanic passed validation")
			}
		})
	}
}

func TestValidateWeaponConfigAcceptsCompleteNewMechanics(t *testing.T) {
	weapon := validMechanicTestWeapon()
	weapon.IgniteChance, weapon.IgniteSeconds = 0.25, 6
	weapon.PoisonChance, weapon.PoisonSeconds = 0.2, 12
	weapon.StunChance, weapon.StunSeconds = 0.1, 9
	weapon.RootChance, weapon.RootSeconds = 0.1, 6
	weapon.ArmorShredPct, weapon.ArmorShredSeconds = 20, 15
	weapon.ExecuteBelowPct = 15
	weapon.DeathBurstMaxHPPct, weapon.DeathBurstRadiusTiles = 20, 2
	weapon.TrueDamagePct = 5
	weapon.SlowPct, weapon.SlowSeconds = 20, 9
	weapon.WeakenPct, weapon.WeakenSeconds = 25, 6
	weapon.SpellEchoPct = 10
	weapon.TBActionsPerRound = 3
	weapon.FreshTargetMinHPPct, weapon.FreshTargetMultiplier = 90, 5
	weapon.HourStrikeMarks, weapon.HourStrikeMultiplier = 12, 3
	weapon.JamSeconds = 1

	cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"test": weapon}}
	if err := validateWeaponConfig(cfg); err != nil {
		t.Fatalf("complete mechanics rejected: %v", err)
	}
}

// A weapon's status lasts its seconds in RT and the same span in TB turns,
// written the one shared way.
func TestWeaponStatusTooltipUsesSharedDuration(t *testing.T) {
	weapon := &WeaponDefinitionConfig{SlowPct: 30, SlowSeconds: 9, StunChance: 0.1, StunSeconds: 3}
	lines := strings.Join(weapon.CoreEffectLines(), "\n")
	if want := fmt.Sprintf("by 30%% for 9s / 3 TB turns (TB: skips one turn in %d)", SlowSkipEveryTurns()); !strings.Contains(lines, want) {
		t.Fatalf("slow tooltip does not state the shared duration and TB skip cadence %q:\n%s", want, lines)
	}
	if !strings.Contains(lines, "Stun Chance: 10% (3s / 1 TB turn)") {
		t.Fatalf("stun tooltip does not use the shared duration:\n%s", lines)
	}
}

func TestValidateItemConfigRejectsIncompleteNewMechanics(t *testing.T) {
	// A complete reusable flask device: valid only with no recharge of its own.
	flaskDevice := func(recharge int) *ItemDefinitionConfig {
		return &ItemDefinitionConfig{Type: "device", UseAction: "throw_flask", UseCooldownSeconds: recharge, BrewColor: [3]int{1, 2, 3},
			Flask: &FlaskDefinition{Reusable: true, ProjectileFx: "fx", Element: "water", Damage: [4]int{1, 2, 3, 4}, RangeTiles: 6, RadiusTiles: 2, CooldownSeconds: 3}}
	}
	// Controls: each device action validates when complete, so the rows below
	// fail on the field they break.
	for name, control := range map[string]*ItemDefinitionConfig{
		"flask device without a recharge": flaskDevice(0),
		"jump device with a recharge":     {Type: "device", UseAction: "jump", UseSpell: "jump", UseJumpExtraTiles: 1, UseCooldownSeconds: 10},
		"summon device with a recharge":   {Type: "device", UseAction: "summon", SummonDistanceTiles: 3, UseCooldownSeconds: 10},
	} {
		if err := validateItemConfig(&ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": control}}); err != nil {
			t.Fatalf("control: a complete %s must validate: %v", name, err)
		}
	}
	tests := []struct {
		name string
		item *ItemDefinitionConfig
	}{
		{"device without action", &ItemDefinitionConfig{Type: "device", UseSpell: "jump", UseCooldownSeconds: 10}},
		{"device without reference", &ItemDefinitionConfig{Type: "device", UseAction: "jump", UseCooldownSeconds: 10}},
		{"device unsupported action", &ItemDefinitionConfig{Type: "device", UseAction: "missing", UseSpell: "jump", UseCooldownSeconds: 10}},
		{"device without recharge", &ItemDefinitionConfig{Type: "device", UseAction: "jump", UseSpell: "jump", UseJumpExtraTiles: 1}},
		{"summon device without recharge", &ItemDefinitionConfig{Type: "device", UseAction: "summon", SummonDistanceTiles: 3}},
		{"thrown flask with a recharge", flaskDevice(6)},
		{"thrown flask with a negative recharge", flaskDevice(-1)},
		{"jump device with a negative recharge", &ItemDefinitionConfig{Type: "device", UseAction: "jump", UseSpell: "jump", UseJumpExtraTiles: 1, UseCooldownSeconds: -1}},
		{"summon device with a negative recharge", &ItemDefinitionConfig{Type: "device", UseAction: "summon", SummonDistanceTiles: 3, UseCooldownSeconds: -1}},
		{"device with negative range", &ItemDefinitionConfig{Type: "device", UseAction: "jump", UseSpell: "jump", UseJumpExtraTiles: -1, UseCooldownSeconds: 10}},
		{"device behavior on potion", &ItemDefinitionConfig{Type: "consumable", UseAction: "jump", UseSpell: "jump", UseJumpExtraTiles: 1, UseCooldownSeconds: 10}},
		{"base damage without equipment or collection", &ItemDefinitionConfig{Type: "trinket", WeaponBaseDamageBonus: 15}},
		{"negative card base damage", &ItemDefinitionConfig{Type: "card", WeaponBaseDamageBonus: -1}},
		{"reflect over one hundred", &ItemDefinitionConfig{ProjectileReflectPct: 101}},
		{"status below floor", &ItemDefinitionConfig{StatusDurationPct: MinHostileStatusDurationPct - 1}},
		{"scale stack without cap", &ItemDefinitionConfig{ScaleStackAC: 1}},
		{"spell ward key on item", &ItemDefinitionConfig{Type: "consumable", DeprecatedResistBuffPct: 50}},
		{"ward without school", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60, StatusIcon: "ward"}},
		{"ward without duration", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "fire", ResistBuffSchoolPct: 50, StatusIcon: "ward"}},
		{"ward without icon", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "fire", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60}},
		{"physical ward", &ItemDefinitionConfig{Type: "consumable", ResistBuffSchool: "physical", ResistBuffSchoolPct: 50, BuffDurationSeconds: 60, StatusIcon: "ward"}},
		{"buff on armor", &ItemDefinitionConfig{Type: "armor", BuffArmorClass: 10, BuffDurationSeconds: 60, StatusIcon: "stone"}},
		{"negative HP regen", &ItemDefinitionConfig{BuffHPRegenPct: -1}},
		{"mana regen over one hundred", &ItemDefinitionConfig{BuffManaRegenPct: 101}},
		{"regen without duration", &ItemDefinitionConfig{Type: "consumable", BuffHPRegenPct: 3, StatusIcon: "regen"}},
		{"regen without icon", &ItemDefinitionConfig{Type: "consumable", BuffManaRegenPct: 5, BuffDurationSeconds: 60}},
		{"regen on armor", &ItemDefinitionConfig{Type: "armor", BuffHPRegenPct: 3, BuffDurationSeconds: 60, StatusIcon: "regen"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"test": tt.item}}
			if err := validateItemConfig(cfg); err == nil {
				t.Fatal("invalid mechanic passed validation")
			}
		})
	}
}

func TestValidateItemConfigCanonicalizesTimedWard(t *testing.T) {
	item := &ItemDefinitionConfig{
		Type:                "consumable",
		ResistBuffSchool:    " FIRE ",
		ResistBuffSchoolPct: 50,
		BuffDurationSeconds: 60,
		StatusIcon:          " fire_shield ",
	}
	cfg := &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{"ward": item}}
	if err := validateItemConfig(cfg); err != nil {
		t.Fatalf("valid ward rejected: %v", err)
	}
	if item.ResistBuffSchool != "fire" || item.StatusIcon != "fire_shield" {
		t.Fatalf("ward metadata not canonicalized: %+v", item)
	}
}

func TestValidateCrateRollSourceRejectsAmbiguousFields(t *testing.T) {
	tables := map[string]*WeightedLootTable{"gear": {Rolls: 1}}
	for _, tt := range []struct {
		name   string
		source CrateRollSource
	}{
		{"weight above one hundred", CrateRollSource{Pool: "nothing", Weight: 101}},
		{"weight below a tenth", CrateRollSource{Pool: "nothing", Weight: 0.05}},
		{"weight finer than a tenth", CrateRollSource{Pool: "nothing", Weight: 2.45}},
		{"removed rare pool", CrateRollSource{Pool: "rare", Weight: 50}},
		{"item type on a map pool", CrateRollSource{Pool: "map", ItemType: "armor", Weight: 50}},
		{"rarity on an empty slot", CrateRollSource{Pool: "nothing", Rarity: "rare", Weight: 50}},
		{"rarity on gold", CrateRollSource{Pool: "gold", Amount: 10, Rarity: "rare", Weight: 50}},
		{"gold without an amount", CrateRollSource{Pool: "gold", Weight: 50}},
		{"amount on a catalog pool", CrateRollSource{Pool: "catalog", ItemType: "armor", Amount: 5, Weight: 50}},
		{"unknown catalog type", CrateRollSource{Pool: "catalog", ItemType: "potion", Weight: 50}},
		{"unknown rarity", CrateRollSource{Pool: "map", Rarity: "ultra", Weight: 50}},
		{"backwards rarity span", CrateRollSource{Pool: "map", Rarity: "rare-common", Weight: 50}},
		{"unknown loot table", CrateRollSource{Pool: "loot_table", LootTable: "missing", Weight: 50}},
		{"loot table pool without a table", CrateRollSource{Pool: "loot_table", Weight: 50}},
		{"rarity on a loot table", CrateRollSource{Pool: "loot_table", LootTable: "gear", Rarity: "rare", Weight: 50}},
		{"amount on a loot table", CrateRollSource{Pool: "loot_table", LootTable: "gear", Amount: 5, Weight: 50}},
		{"loot table on a map pool", CrateRollSource{Pool: "map", LootTable: "gear", Weight: 50}},
		{"loot table on gold", CrateRollSource{Pool: "gold", Amount: 10, LootTable: "gear", Weight: 50}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCrateRollSource(tables, "test", 0, tt.source); err == nil {
				t.Fatal("ambiguous crate source passed validation")
			}
		})
	}
	for _, ok := range []CrateRollSource{
		{Pool: "nothing", Weight: 50},
		{Pool: "map", Rarity: "uncommon+", Weight: 2.4},
		{Pool: "map", Rarity: "common-uncommon", Weight: 66.4},
		{Pool: "arena_points", Amount: 5000, Weight: 1.7},
		{Pool: "loot_table", LootTable: "gear", Weight: 3.5},
	} {
		if err := validateCrateRollSource(tables, "test", 0, ok); err != nil {
			t.Fatalf("valid source %+v rejected: %v", ok, err)
		}
	}
}

// One rarity field reads every gate the crates need.
func TestParseRarityRange(t *testing.T) {
	for _, tc := range []struct {
		in       string
		min, max int
	}{
		{"", 0, 4}, {"rare", 2, 2}, {"common-uncommon", 0, 1}, {"uncommon+", 1, 4}, {"common-rare", 0, 2}, {" Legendary ", 3, 3},
	} {
		got, err := ParseRarityRange(tc.in)
		if err != nil || got.Min != tc.min || got.Max != tc.max {
			t.Fatalf("ParseRarityRange(%q) = %+v, %v; want %d..%d", tc.in, got, err, tc.min, tc.max)
		}
	}
}

// Loot data decodes strictly: a field the crates no longer read fails the load.
func TestLootTablesRejectRemovedCrateFields(t *testing.T) {
	for _, field := range []string{"special_rolls: []", "min_rarity: rare", "legendary_pct: 5"} {
		data := "crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: nothing\n        weight: 100\n    " + field + "\n"
		if field != "special_rolls: []" {
			data = "crates:\n  test:\n    rolls: 1\n    roll_sources:\n      - pool: nothing\n        weight: 100\n        " + field + "\n"
		}
		path := filepath.Join(t.TempDir(), "loot.yaml")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLootTables(path); err == nil {
			t.Fatalf("removed field %q loaded silently", field)
		}
	}
}

func TestValidateCratesRequiresReadablePercentageWeights(t *testing.T) {
	cfg := &LootTablesConfig{Crates: map[string]*CrateConfig{
		"bad_total": {
			Rolls: 1,
			RollSources: []CrateRollSource{
				{Pool: "nothing", Weight: 60},
				{Pool: "nothing", Weight: 30},
			},
		},
	}}
	if err := validateCrates(cfg); err == nil || !strings.Contains(err.Error(), "total 100") {
		t.Fatalf("non-percentage crate weights validation = %v", err)
	}

	cfg.Crates["bad_total"].RollSources[1].Weight = 40
	if err := validateCrates(cfg); err != nil {
		t.Fatalf("percentage crate weights rejected: %v", err)
	}
}

// Pursuit is required authored gameplay data, not a silent code fallback.
func TestMonsterPursuitValidation(t *testing.T) {
	valid := MonsterPursuitConfig{MaxRadiusTiles: 16, RetaliationSeconds: 10, RetaliationTurns: 4, RetaliationMarginTiles: 2}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*MonsterPursuitConfig)
	}{
		{"missing leash", func(c *MonsterPursuitConfig) { c.MaxRadiusTiles = 0 }},
		{"infinite leash", func(c *MonsterPursuitConfig) { c.MaxRadiusTiles = math.Inf(1) }},
		{"invalid time", func(c *MonsterPursuitConfig) { c.RetaliationSeconds = math.NaN() }},
		{"missing turns", func(c *MonsterPursuitConfig) { c.RetaliationTurns = 0 }},
		{"negative margin", func(c *MonsterPursuitConfig) { c.RetaliationMarginTiles = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.change(&c)
			if c.validate() == nil {
				t.Fatal("invalid pursuit configuration accepted")
			}
		})
	}
}

func TestTurnBasedMouseHoldValidation(t *testing.T) {
	valid := MouseHoldTimingConfig{DelaySeconds: .6, RepeatSeconds: .35}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{"missing", 0}, {"negative", -1}, {"infinite", math.Inf(1)}, {"NaN", math.NaN()},
	} {
		for _, delay := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/delay=%v", tc.name, delay), func(t *testing.T) {
				c := valid
				if delay {
					c.DelaySeconds = tc.value
				} else {
					c.RepeatSeconds = tc.value
				}
				if c.validate() == nil {
					t.Fatal("invalid mouse hold timing accepted")
				}
			})
		}
	}
}

// Slot pools share the existing rarity grammar, preserve explicit boss rewards,
// and reject authored gates with no candidates before gameplay can roll them.
func TestEquipmentSlotRarityValidation(t *testing.T) {
	old := GlobalItems
	t.Cleanup(func() { GlobalItems = old })
	GlobalItems = &ItemSystemConfig{Items: map[string]*ItemDefinitionConfig{
		"common":     {Type: "accessory", EquipSlot: "ring", Rarity: "common"},
		"rare":       {Type: "accessory", EquipSlot: "ring", Rarity: "rare"},
		"boss":       {Type: "accessory", EquipSlot: "ring", Rarity: "legendary", NoLoot: true},
		"crafted":    {Type: "accessory", EquipSlot: "ring", Rarity: "unique", CraftedOnly: true},
		"other_slot": {Type: "accessory", EquipSlot: "amulet", Rarity: "unique"},
	}}
	for _, tc := range []struct {
		rarity, want string
		valid        bool
	}{
		{"", "boss,common,rare", true}, {"rare", "rare", true},
		{"common-rare", "common,rare", true}, {"rare+", "boss,rare", true},
		{"unique", "", false}, {"ultra", "", false}, {"rare-common", "", false},
	} {
		src := CrateRollSource{Pool: "equipment_slot", EquipSlot: "ring", Weight: 100, Rarity: tc.rarity}
		err := validateCrateRollSource(nil, "test_cache", 0, src)
		if (err == nil) != tc.valid {
			t.Fatalf("rarity=%q: %v", tc.rarity, err)
		}
		if !tc.valid {
			if !strings.Contains(err.Error(), "test_cache") {
				t.Fatal("error lost its content location", err)
			}
			continue
		}
		if got := strings.Join(CrateEquipmentSlotKeys(src.EquipSlot, src.RarityTiers()), ","); got != tc.want {
			t.Errorf("rarity=%q: keys=%q want %q", tc.rarity, got, tc.want)
		}
	}
}

// A returning weapon's return leg carries no weapon effects, so a true share
// authored on one would silently vanish on the way back.
func TestValidateWeaponConfigRejectsTrueShareOnReturningWeapon(t *testing.T) {
	weapons, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, pct := range []int{0, 10} {
		weapon := *weapons.Weapons["solstice_returning_gale"]
		weapon.TrueDamagePct = pct
		cfg := &WeaponSystemConfig{Weapons: map[string]*WeaponDefinitionConfig{"solstice_returning_gale": &weapon}}
		if err := validateWeaponConfig(cfg); (err != nil) != (pct > 0) || err != nil && !strings.Contains(err.Error(), "true_damage_pct") {
			t.Fatalf("true_damage_pct %d on a returning weapon: validation error = %v", pct, err)
		}
	}
}
