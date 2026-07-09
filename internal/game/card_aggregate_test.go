package game

import (
	"testing"

	"ugataima/internal/config"
)

// foldCardDefs must combine each card field the way the mechanics apply it:
// ints sum, stat/resist maps merge-sum, CardBonusVs multiplies per key, bools
// OR, strings take the first - so the Cards tab summary reads true totals.
func TestFoldCardDefsCombineRules(t *testing.T) {
	agg := foldCardDefs([]*config.ItemDefinitionConfig{
		{
			CardMoveSpeedPct:  10,
			CardBonusActions:  1,
			CardStatBonuses:   map[string]int{"might": 2, "speed": 5},
			CardBonusVs:       map[string]float64{"orc": 1.5},
			CardWalkOnWater:   false,
			CardSummonMonster: "wolf",
		},
		{
			CardMoveSpeedPct:  10,
			CardStatBonuses:   map[string]int{"might": 3},
			CardBonusVs:       map[string]float64{"orc": 2.0, "dragon": 3.0},
			CardWalkOnWater:   true,
			CardSummonMonster: "bear", // later card must NOT override the first
		},
		nil, // gaps are ignored
	})

	if agg.CardMoveSpeedPct != 20 {
		t.Errorf("move speed: sum = %d, want 20", agg.CardMoveSpeedPct)
	}
	if agg.CardBonusActions != 1 {
		t.Errorf("bonus actions: sum = %d, want 1", agg.CardBonusActions)
	}
	if agg.CardStatBonuses["might"] != 5 || agg.CardStatBonuses["speed"] != 5 {
		t.Errorf("stat bonuses merge-sum = %v, want might:5 speed:5", agg.CardStatBonuses)
	}
	if agg.CardBonusVs["orc"] != 3.0 { // 1.5 * 2.0
		t.Errorf("bonus_vs orc multiplies = %v, want 3.0", agg.CardBonusVs["orc"])
	}
	if agg.CardBonusVs["dragon"] != 3.0 {
		t.Errorf("bonus_vs dragon = %v, want 3.0", agg.CardBonusVs["dragon"])
	}
	if !agg.CardWalkOnWater {
		t.Error("walk-on-water must OR to true")
	}
	if agg.CardSummonMonster != "wolf" {
		t.Errorf("summon monster: first-set = %q, want wolf", agg.CardSummonMonster)
	}
}

// Poison fields are NOT plain sums: duration takes the best (max), resist tops
// out at 100% - the summary must match combat, not blindly add.
func TestFoldCardDefsPoisonNotSummed(t *testing.T) {
	agg := foldCardDefs([]*config.ItemDefinitionConfig{
		{CardPoisonProcPct: 20, CardPoisonDurationSec: 5, CardPoisonResistPct: 60},
		{CardPoisonProcPct: 20, CardPoisonDurationSec: 8, CardPoisonResistPct: 60},
	})
	if agg.CardPoisonProcPct != 40 {
		t.Errorf("poison proc chance sums = %d, want 40", agg.CardPoisonProcPct)
	}
	if agg.CardPoisonDurationSec != 8 {
		t.Errorf("poison duration is max = %d, want 8", agg.CardPoisonDurationSec)
	}
	if agg.CardPoisonResistPct != 100 {
		t.Errorf("poison resist caps = %d, want 100", agg.CardPoisonResistPct)
	}
}

// An empty collection folds to a definition with no effect lines (the summary
// then reads "No active card effects.").
func TestFoldCardDefsEmpty(t *testing.T) {
	if lines := foldCardDefs(nil).CardEffectLines(); len(lines) != 0 {
		t.Errorf("empty fold should have no effect lines, got %v", lines)
	}
}
