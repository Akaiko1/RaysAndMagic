package game

// Spell-cast integration tests — exercise the REAL CombatSystem.CastEquippedSpell
// path with spells loaded from spells.yaml. Replaces the placebo
// internal/character/combat_test.go which used to test a fake CombatSystem
// against a fake formula.

import (
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// equipSpellAndPrepareCaster equips the named spell on the first party
// member, sets their SP/Intellect, and returns the caster. The party's
// selected char is set to index 0.
func equipSpellAndPrepareCaster(t *testing.T, cs *CombatSystem, spellKey string, sp, intellect int) {
	t.Helper()
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellKey))
	if err != nil {
		t.Fatalf("spell %q missing from spells.yaml: %v", spellKey, err)
	}
	itemType := items.ItemBattleSpell
	if def.IsUtility {
		itemType = items.ItemUtilitySpell
	}
	spellItem := items.Item{
		Name:        def.Name,
		Type:        itemType,
		SpellSchool: def.School,
		SpellCost:   def.SpellPointsCost,
		SpellEffect: items.SpellEffect(spellKey),
		Attributes:  map[string]int{},
	}
	caster := cs.game.party.Members[0]
	if caster.Equipment == nil {
		caster.Equipment = make(map[items.EquipSlot]items.Item)
	}
	caster.Equipment[items.SlotSpell] = spellItem
	caster.SpellPoints = sp
	caster.Intellect = intellect
	cs.game.selectedChar = 0
}

func TestCastEquippedSpell_ConsumesSpellPoints(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// Fireball: spell_points_cost = 4 in YAML.
	equipSpellAndPrepareCaster(t, cs, "fireball", 10, 0)

	if !cs.CastEquippedSpell() {
		t.Fatalf("CastEquippedSpell returned false despite sufficient SP")
	}
	caster := cs.game.party.Members[0]
	want := 10 - 4
	if caster.SpellPoints != want {
		t.Errorf("SP after cast: got %d, want %d", caster.SpellPoints, want)
	}
}

func TestCastEquippedSpell_FailsWhenSPInsufficient(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// Fireball costs 4; give caster 3.
	equipSpellAndPrepareCaster(t, cs, "fireball", 3, 0)
	before := cs.game.party.Members[0].SpellPoints

	if cs.CastEquippedSpell() {
		t.Errorf("cast should fail when SP < cost")
	}
	if got := cs.game.party.Members[0].SpellPoints; got != before {
		t.Errorf("SP should be unchanged on failed cast: got %d, want %d", got, before)
	}
}

func TestCastEquippedSpell_FailsWithoutEquippedSpell(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := cs.game.party.Members[0]
	caster.SpellPoints = 100
	if caster.Equipment != nil {
		delete(caster.Equipment, items.SlotSpell)
	}
	cs.game.selectedChar = 0

	if cs.CastEquippedSpell() {
		t.Errorf("cast should fail when no spell is equipped")
	}
}

func TestCalculateSpellDamage_FollowsCanonicalFormula(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// Reuse the helper to load a real spell; assertion below derives expected
	// damage purely from balance constants + YAML cost — no magic literal.
	equipSpellAndPrepareCaster(t, cs, "fireball", 100, 14)
	caster := cs.game.party.Members[0]
	def, _ := spells.GetSpellDefinitionByID("fireball")

	base, intBonus, total := cs.CalculateSpellDamage(def.ID, caster)

	// Canonical formula: base = cost × SpellDamagePerSP, intBonus =
	// intellect / SpellIntellectDivisor, total = base + intBonus (+ mastery
	// bonus, which is 0 at Novice — the default for a freshly created char).
	wantBase := def.SpellPointsCost * spells.SpellDamagePerSP
	wantInt := caster.Intellect / spells.SpellIntellectDivisor
	wantTotal := wantBase + wantInt

	if base != wantBase {
		t.Errorf("base damage: got %d, want %d (cost=%d × %d)", base, wantBase, def.SpellPointsCost, spells.SpellDamagePerSP)
	}
	if intBonus != wantInt {
		t.Errorf("intellect bonus: got %d, want %d (int=%d / %d)", intBonus, wantInt, caster.Intellect, spells.SpellIntellectDivisor)
	}
	if total != wantTotal {
		t.Errorf("total damage: got %d, want %d", total, wantTotal)
	}
}

// Ray of Light: base damage = cost × SpellDamagePerSP × 2 (damage_cost_multiplier),
// and the stat term scales with BOTH Intellect AND Personality (scales_with_personality).
func TestRayOfLight_DamageScalesWithCostAndBothStats(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	equipSpellAndPrepareCaster(t, cs, "ray_of_light", 100, 90) // sp=100, intellect=90
	caster := cs.game.party.Members[0]
	caster.Personality = 60
	def, _ := spells.GetSpellDefinitionByID("ray_of_light")

	if def.DamageCostMultiplier != 2 {
		t.Fatalf("ray_of_light should have damage_cost_multiplier 2 in YAML, got %d", def.DamageCostMultiplier)
	}
	if !def.ScalesWithPersonality {
		t.Fatalf("ray_of_light should have scales_with_personality true in YAML")
	}

	base, statBonus, total := cs.CalculateSpellDamage(def.ID, caster)

	wantBase := def.SpellPointsCost * spells.SpellDamagePerSP * 2
	wantStat := caster.GetEffectiveIntellect()/spells.SpellIntellectDivisor +
		caster.GetEffectivePersonality()/spells.SpellIntellectDivisor
	wantTotal := wantBase + wantStat

	if base != wantBase {
		t.Errorf("base: got %d, want %d (cost %d × %d × 2)", base, wantBase, def.SpellPointsCost, spells.SpellDamagePerSP)
	}
	if statBonus != wantStat {
		t.Errorf("stat bonus: got %d, want %d (Int/%d + Per/%d)", statBonus, wantStat, spells.SpellIntellectDivisor, spells.SpellIntellectDivisor)
	}
	if total != wantTotal {
		t.Errorf("total: got %d, want %d", total, wantTotal)
	}
}

// Regression: a normal spell (no multiplier / personality flag) is unchanged by
// the new data-driven fields — base stays cost × SpellDamagePerSP, stat term is
// Intellect-only.
func TestNormalSpell_UnaffectedByRayOfLightFields(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	equipSpellAndPrepareCaster(t, cs, "fireball", 100, 90)
	caster := cs.game.party.Members[0]
	caster.Personality = 60
	def, _ := spells.GetSpellDefinitionByID("fireball")

	base, statBonus, _ := cs.CalculateSpellDamage(def.ID, caster)

	if want := def.SpellPointsCost * spells.SpellDamagePerSP; base != want {
		t.Errorf("fireball base changed: got %d, want %d (cost × perSP, multiplier defaults to 1)", base, want)
	}
	if want := caster.GetEffectiveIntellect() / spells.SpellIntellectDivisor; statBonus != want {
		t.Errorf("fireball stat bonus should be Intellect-only: got %d, want %d (Personality must NOT contribute)", statBonus, want)
	}
}
