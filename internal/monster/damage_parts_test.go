package monster

import (
	"testing"

	damagecalc "ugataima/internal/damage"
)

// takeDamageParts is the monster half of a single-school hit: normal and true
// damage share the element and resistance; Stone Skin soaks only the normal part.
func takeDamageParts(m *Monster3D, parts damagecalc.Parts, damageType DamageType, resistPiercePct int) int {
	return m.TakeDamagePacket([]DamageComponent{{Parts: parts, DamageType: damageType, ResistPiercePct: resistPiercePct}}).Total()
}

func TestTakeDamageParts_ResistanceThenNormalOnlySoak(t *testing.T) {
	m := &Monster3D{
		HitPoints:    100,
		MaxHitPoints: 100,
		Resistances:  map[DamageType]int{DamagePhysical: 50},
		SoakDamage:   5,
		SoakFrames:   1,
	}

	got := takeDamageParts(m, damagecalc.Parts{Normal: 50, True: 20}, DamagePhysical, 0)
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

	if got := takeDamageParts(m, damagecalc.Parts{Normal: 100, True: 100}, DamagePhysical, 0); got != 0 {
		t.Fatalf("invulnerable target took %d, want 0", got)
	}
	if m.HitPoints != 100 || m.WasAttacked {
		t.Fatalf("invulnerable target changed: hp=%d attacked=%v", m.HitPoints, m.WasAttacked)
	}
}

func TestTakeDamagePacket_MultiSchoolHitPaysSoakOnce(t *testing.T) {
	m := &Monster3D{
		HitPoints:    1000,
		MaxHitPoints: 1000,
		Resistances:  map[DamageType]int{},
		SoakDamage:   10,
		SoakFrames:   1,
	}
	dealt := m.TakeDamagePacket([]DamageComponent{
		{Parts: damagecalc.Parts{Normal: 80}, DamageType: DamagePhysical},
		{Parts: damagecalc.Parts{Normal: 20}, DamageType: DamageFire},
	})
	if got := dealt.Total(); got != 90 {
		t.Fatalf("80 physical + 20 fire with soak 10 dealt %d, want 90 from one hit", got)
	}
	if m.HitPoints != 910 {
		t.Fatalf("HP = %d, want 910", m.HitPoints)
	}
}
