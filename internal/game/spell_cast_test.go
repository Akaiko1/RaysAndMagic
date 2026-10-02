package game

// Spell-cast integration tests - exercise the REAL CombatSystem.CastEquippedSpell
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
	caster.LearnSpell(spells.SpellID(spellKey))
	caster.Equipment[items.SlotSpell] = spellItem
	caster.SpellPoints = sp
	caster.Intellect = intellect
	cs.game.selectedChar = 0
}

// CastEquippedSpell pays the definition's effective cost when the caster can
// afford it, and refuses without spending when SP falls short or no spell is
// slotted.
func TestCastEquippedSpell(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spare   int // SP beyond the effective cost (negative: short)
		equip   bool
		wantPay bool
	}{
		{"pays_effective_cost", 6, true, true},
		{"refuses_when_sp_short", -1, true, false},
		{"refuses_without_equipped_spell", 100, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			caster := cs.game.party.Members[0]
			def, err := spells.GetSpellDefinitionByID("fireball")
			if err != nil {
				t.Fatalf("fireball def: %v", err)
			}
			cost := cs.effectiveSpellCost(caster, def.SpellPointsCost)
			if cost <= 0 {
				t.Fatalf("fireball must cost SP, got %d", cost)
			}
			equipSpellAndPrepareCaster(t, cs, "fireball", cost+tc.spare, 0)
			if !tc.equip {
				delete(caster.Equipment, items.SlotSpell)
			}
			before := caster.SpellPoints
			if got := cs.CastEquippedSpell(); got != tc.wantPay {
				t.Fatalf("CastEquippedSpell=%v, want %v", got, tc.wantPay)
			}
			want := before
			if tc.wantPay {
				want -= cost
			}
			if caster.SpellPoints != want {
				t.Errorf("SP after cast: got %d, want %d", caster.SpellPoints, want)
			}
		})
	}
}

// CalculateSpellDamage follows the canonical formula: base = cost x
// SpellDamagePerSP x damage_cost_multiplier (default 1); the stat term is the
// school's stat / SpellIntellectDivisor, plus Personality when the spell
// scales_with_personality; mastery adds nothing at Novice.
func TestCalculateSpellDamage_FollowsCanonicalFormula(t *testing.T) {
	for _, tc := range []struct {
		name                string
		spell               string
		intellect, personal int
		// multiplied/personality: the authored flags this row exercises.
		multiplied, personality bool
	}{
		{"fireball_intellect_only", "fireball", 14, 0, false, false},
		{"fireball_ignores_personality", "fireball", 90, 60, false, false},
		{"ray_of_light_cost_multiplier_and_both_stats", "ray_of_light", 90, 60, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			equipSpellAndPrepareCaster(t, cs, tc.spell, 100, tc.intellect)
			caster := cs.game.party.Members[0]
			caster.Personality = tc.personal
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.spell))
			if err != nil {
				t.Fatalf("%s def: %v", tc.spell, err)
			}
			if (def.DamageCostMultiplier > 1) != tc.multiplied || def.ScalesWithPersonality != tc.personality {
				t.Fatalf("%s no longer exercises this row: damage_cost_multiplier=%d scales_with_personality=%v",
					tc.spell, def.DamageCostMultiplier, def.ScalesWithPersonality)
			}

			base, statBonus, total := cs.CalculateSpellDamage(def.ID, caster)

			wantBase := def.SpellPointsCost * spells.SpellDamagePerSP * max(1, def.DamageCostMultiplier)
			wantStat := caster.GetEffectiveIntellect() / spells.SpellIntellectDivisor
			if def.ScalesWithPersonality {
				wantStat += caster.GetEffectivePersonality() / spells.SpellIntellectDivisor
			}
			if base != wantBase {
				t.Errorf("base: got %d, want %d (cost %d x %d x %d)", base, wantBase, def.SpellPointsCost, spells.SpellDamagePerSP, max(1, def.DamageCostMultiplier))
			}
			if statBonus != wantStat {
				t.Errorf("stat bonus: got %d, want %d", statBonus, wantStat)
			}
			if total != wantBase+wantStat {
				t.Errorf("total: got %d, want %d", total, wantBase+wantStat)
			}
		})
	}
}
