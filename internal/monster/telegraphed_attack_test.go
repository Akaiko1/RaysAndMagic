package monster

import (
	"math"
	"testing"
)

func TestTelegraphedAttackValidation(t *testing.T) {
	good := TelegraphedAttack{Name: "Wave", Kind: "slow", School: "water", Shape: "cone", Range: 5, CooldownSeconds: 9, CooldownRounds: 3, WarningSeconds: 1.25, WarningRounds: 1, DamagePercent: 90, DurationSeconds: 9}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	// Positive controls: a root lasts any whole rounds; a strike needs no duration.
	for _, ok := range []func(*TelegraphedAttack){
		func(a *TelegraphedAttack) { a.Kind, a.DurationSeconds = "root", 6 },
		func(a *TelegraphedAttack) { a.Kind, a.DurationSeconds = "strike", 0 },
	} {
		a := good
		ok(&a)
		if err := a.validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*TelegraphedAttack)
	}{
		{"kind", func(a *TelegraphedAttack) { a.Kind = "unknown" }},
		{"shape", func(a *TelegraphedAttack) { a.Shape = "unknown" }},
		{"school", func(a *TelegraphedAttack) { a.School = "physical" }},
		{"name", func(a *TelegraphedAttack) { a.Name = "" }},
		{"range", func(a *TelegraphedAttack) { a.Range = 0 }},
		{"range-high", func(a *TelegraphedAttack) { a.Range = 9 }},
		{"cooldown-rounds", func(a *TelegraphedAttack) { a.CooldownRounds = 0 }},
		{"warning-rounds", func(a *TelegraphedAttack) { a.WarningRounds = 0 }},
		{"cooldown-nan", func(a *TelegraphedAttack) { a.CooldownSeconds = math.NaN() }},
		{"warning-inf", func(a *TelegraphedAttack) { a.WarningSeconds = math.Inf(1) }},
		{"zero-warning", func(a *TelegraphedAttack) { a.WarningSeconds = 0 }},
		{"negative-duration", func(a *TelegraphedAttack) { a.DurationSeconds = -1 }},
		{"part-of-a-round", func(a *TelegraphedAttack) { a.DurationSeconds = 4 }},
		{"slow-without-duration", func(a *TelegraphedAttack) { a.DurationSeconds = 0 }},
		{"slow-off-skip-step", func(a *TelegraphedAttack) { a.DurationSeconds = 6 }},
		{"root-without-duration", func(a *TelegraphedAttack) { a.Kind, a.DurationSeconds = "root", 0 }},
		{"damage", func(a *TelegraphedAttack) { a.DamagePercent = 201 }},
		{"threshold", func(a *TelegraphedAttack) { a.BelowHPPercent = 101 }},
		{"sustain-duration", func(a *TelegraphedAttack) { a.Kind, a.DurationSeconds, a.SustainDamage = "strike", 0, 8 }},
		{"shield-duration", func(a *TelegraphedAttack) { a.Kind, a.DurationSeconds = "shield", 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := good
			tc.mutate(&a)
			if a.validate() == nil {
				t.Fatal("invalid ability accepted")
			}
		})
	}
}
