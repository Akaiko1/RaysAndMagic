package spells

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/stats"
)

func TestResolvedSpellFormulaCells(t *testing.T) {
	old := config.GlobalSpells
	t.Cleanup(func() { config.GlobalSpells = old })
	config.GlobalSpells = &config.SpellSystemConfig{Spells: map[string]*config.SpellDefinitionConfig{
		"damage": {Name: "Damage fixture", School: "fire", IsProjectile: true, SpellPointsCost: 4, Graphics: &config.ProjectileRenderConfig{Color: [3]int{12, 34, 56}}},
		"heal":   {Name: "Healing fixture", School: "body", IsUtility: true, HealAmount: 20},
	}}
	for _, tc := range []struct {
		name                        string
		id                          SpellID
		tier                        int
		values                      stats.StatBonuses
		base, bonus, mastery, total int
	}{
		{"damage_base", "damage", 0, stats.StatBonuses{}, 12, 0, 0, 12},
		{"damage_scaled", "damage", 2, stats.StatBonuses{Intellect: 12, Personality: 90}, 12, 4, 10, 26},
		{"healing_base", "heal", 0, stats.StatBonuses{}, 20, 0, 0, 20},
		{"healing_scaled", "heal", 3, stats.StatBonuses{Personality: 12, Intellect: 90}, 20, 6, 15, 41},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def, err := GetSpellDefinitionByID(tc.id)
			if err != nil {
				t.Fatal(err)
			}
			formula := def.DamageFormula().Formula
			if tc.id == "heal" {
				formula = def.HealingFormula()
			}
			got := formula.Evaluate(tc.values, tc.tier)
			if got.Base != tc.base || got.StatBonus != tc.bonus || got.Mastery != tc.mastery || got.Total != tc.total {
				t.Fatalf("resolved %s formula=%+v; want base=%d stat=%d mastery=%d total=%d", tc.id, got, tc.base, tc.bonus, tc.mastery, tc.total)
			}
		})
	}
	cfg := &config.Config{}
	for _, id := range []string{"damage", "missing"} {
		t.Run("graphics/"+id, func(t *testing.T) {
			got, err := cfg.GetSpellGraphicsConfig(id)
			if id == "missing" {
				if err == nil {
					t.Fatal("missing graphics accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Color != [3]int{12, 34, 56} {
				t.Fatalf("graphics color=%v", got.Color)
			}
		})
	}
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "unavailable"}[unavailable], func(t *testing.T) {
			id := SpellID("missing")
			if unavailable {
				config.GlobalSpells = nil
				id = "damage"
			}
			if _, err := GetSpellDefinitionByID(id); err == nil {
				t.Fatal("unresolved spell accepted")
			}
			if _, err := cfg.GetSpellGraphicsConfig(string(id)); err == nil {
				t.Fatal("unresolved graphics accepted")
			}
		})
	}
}
