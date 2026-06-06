package game

import (
	"testing"

	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// TestEnsureSelectedCanActRT_SkipsDead reproduces the freeze bug: when the
// selected member is killed, real-time selection must hand off to a living one
// instead of sticking on the corpse.
func TestEnsureSelectedCanActRT_SkipsDead(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 2 {
		t.Skip("need >=2 party members")
	}

	g.selectedChar = 1
	members[1].HitPoints = 0 // KO the selected member
	g.ensureSelectedCanActRT()
	if g.selectedChar == 1 || !members[g.selectedChar].CanAct() {
		t.Fatalf("selection stuck on dead member (selectedChar=%d, CanAct=%v)",
			g.selectedChar, members[g.selectedChar].CanAct())
	}

	// Only the last member alive → selection must land on them from a dead one.
	last := len(members) - 1
	for i, m := range members {
		if i != last {
			m.HitPoints = 0
		} else if m.HitPoints <= 0 {
			m.HitPoints = 1
		}
	}
	g.selectedChar = 0
	g.ensureSelectedCanActRT()
	if g.selectedChar != last {
		t.Fatalf("expected selection on sole survivor %d, got %d", last, g.selectedChar)
	}
}

// TestRTHoldSpace_HighSpeedAttacksMore verifies the per-character cooldown +
// auto-advance model: holding the attack key, a high-Speed member (shorter
// cooldown) acts more often than slow ones. Mirrors the real-time act loop in
// handleCombatInput using the shared helpers.
func TestRTHoldSpace_HighSpeedAttacksMore(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	n := len(members)
	if n < 2 {
		t.Skip("need >=2 party members")
	}
	// Isolate Speed: strip weapons (unarmed → cooldown is the pure Speed curve),
	// equal HP, all ready.
	for i, m := range members {
		delete(m.Equipment, items.SlotMainHand)
		m.HitPoints, m.MaxHitPoints = 50, 50
		m.RTCooldown = 0
		m.Speed = 5
		if i == 0 {
			m.Speed = 60 // the fast one
		}
	}

	counts := make([]int, n)
	g.selectedChar = 0
	stagger := 0
	for f := 0; f < 2400; f++ { // 20s at 120 TPS
		for _, m := range members {
			if m.RTCooldown > 0 {
				m.RTCooldown--
			}
		}
		if stagger > 0 {
			stagger--
			continue
		}
		g.ensureSelectedCanActRT()
		if !g.rtCharReady(g.selectedChar) {
			g.advanceToNextReadyCharRT()
		}
		if g.rtCharReady(g.selectedChar) {
			sel := members[g.selectedChar]
			sel.RTCooldown = cs.WeaponCooldownFrames(sel)
			counts[g.selectedChar]++
			stagger = rtActionStagger
			g.advanceToNextReadyCharRT()
		}
	}
	t.Logf("attack counts per member: %v", counts)
	if counts[0] <= counts[1] {
		t.Errorf("high-Speed member should attack more: counts=%v", counts)
	}
	for i := 1; i < n; i++ {
		if counts[i] == 0 {
			t.Errorf("slow member %d never got to act (round-robin starved)", i)
		}
	}
}

// TestSmartAttack_HealsMostWoundedThenAttacks: with a heal slotted, Space heals
// the most-wounded ally when someone is hurt, and reverts to a weapon swing when
// the party is healthy.
func TestSmartAttack_HealsMostWoundedThenAttacks(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 2 {
		t.Skip("need >=2 party members")
	}
	caster := members[0]
	g.selectedChar = 0
	caster.Equipment[items.SlotSpell] = items.Item{
		Name: "Heal", Type: items.ItemUtilitySpell,
		SpellEffect: items.SpellEffectHealOther, SpellCost: 4,
	}
	caster.SpellPoints, caster.MaxSpellPoints = 50, 50
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints // everyone full
	}
	// Wound member 1 to ~30%.
	hurt := members[1]
	hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
	before := hurt.HitPoints

	cast, id := cs.SmartAttack()
	if !cast || id != spells.SpellID("heal_other") {
		t.Fatalf("expected smart-attack to cast heal_other on the wounded ally, got cast=%v id=%q", cast, id)
	}
	if hurt.HitPoints <= before {
		t.Errorf("wounded ally not healed: %d -> %d", before, hurt.HitPoints)
	}

	// Now everyone is healthy → smart-attack must NOT heal (weapon swing instead).
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints
	}
	spBefore := caster.SpellPoints
	cast, _ = cs.SmartAttack()
	if cast {
		t.Errorf("smart-attack healed a full-HP party; should have attacked")
	}
	if caster.SpellPoints != spBefore {
		t.Errorf("smart-attack spent SP with no wounded ally (%d -> %d)", spBefore, caster.SpellPoints)
	}
}
