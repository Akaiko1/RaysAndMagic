package monster

import (
	"testing"

	damagecalc "ugataima/internal/damage"
)

func TestTakeDamageParts_ResistanceThenNormalOnlySoak(t *testing.T) {
	m := &Monster3D{
		HitPoints:    100,
		MaxHitPoints: 100,
		Resistances:  map[DamageType]int{DamagePhysical: 50},
		SoakDamage:   5,
		SoakFrames:   1,
	}

	got := m.TakeDamageParts(damagecalc.Parts{Normal: 50, True: 20}, DamagePhysical, 0)
	if got != 30 {
		t.Fatalf("dealt = %d, want 30 (normal 50%% -> 25 -> soak 20; true 50%% -> 10)", got)
	}
	if m.HitPoints != 70 {
		t.Fatalf("HP = %d, want 70", m.HitPoints)
	}
}

func TestTakeDamageParts_ScriptedInvulnerabilityAbsorbsBoth(t *testing.T) {
	m := &Monster3D{
		HitPoints:    100,
		MaxHitPoints: 100,
		BossDormant:  true,
	}

	if got := m.TakeDamageParts(damagecalc.Parts{Normal: 100, True: 100}, DamagePhysical, 0); got != 0 {
		t.Fatalf("invulnerable target took %d, want 0", got)
	}
	if m.HitPoints != 100 || m.WasAttacked {
		t.Fatalf("invulnerable target changed: hp=%d attacked=%v", m.HitPoints, m.WasAttacked)
	}
}
