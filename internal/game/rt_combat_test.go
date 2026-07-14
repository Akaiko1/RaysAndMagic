package game

import (
	"testing"

	"ugataima/internal/character"
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

	// Only the last member alive -> selection must land on them from a dead one.
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

// TestParkSelection_RTCyclingUnaffected: a manual park lets the player sit on a
// downed member (to use their potions), yet the held-Space RT loop still
// recovers - advanceRTActor clears the park and moves to a capable member, so
// cycling is never soft-locked.
func TestParkSelection_RTCyclingUnaffected(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 2 {
		t.Skip("need >=2 party members")
	}
	for _, m := range members { // make everyone weapon-capable + alive
		m.HitPoints = m.MaxHitPoints
		m.RTCooldown = 0
		m.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	}

	// Park on member 1, then KO them.
	g.selectedChar = 1
	g.parkSelection = true
	members[1].HitPoints = 0

	// The auto-snap must RESPECT the manual park (don't bounce off the downed
	// member) so the player can use their quick potions.
	g.ensureSelectedCanActRT()
	if g.selectedChar != 1 {
		t.Fatalf("manual park not respected: selection moved off downed member to %d", g.selectedChar)
	}

	// Acting (advanceRTActor, as commitRTAction does on each held-key fire) clears
	// the park and hands selection to a member who can act - cycling resumes.
	g.advanceRTActor(rtActWeapon)
	if g.parkSelection {
		t.Fatal("advanceRTActor must clear the manual park so RT cycling isn't soft-locked")
	}
	if g.selectedChar == 1 {
		t.Fatalf("advanceRTActor stayed on the downed member")
	}

	// Park cleared -> the auto-snap behaves normally again.
	g.ensureSelectedCanActRT()
	if !members[g.selectedChar].CanAct() {
		t.Fatalf("post-park selection landed on a member who can't act (%d)", g.selectedChar)
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
	// Isolate Speed: same weapon for all (sword mult 1.0 -> cooldown is the pure
	// Speed curve), equal HP, all ready, all weapon-capable.
	for i, m := range members {
		m.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
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
		if !g.rtActionReady(g.selectedChar, rtActWeapon) {
			g.advanceRTActor(rtActWeapon)
		}
		if g.rtActionReady(g.selectedChar, rtActWeapon) {
			sel := members[g.selectedChar]
			sel.RTCooldown = cs.WeaponCooldownFrames(sel)
			counts[g.selectedChar]++
			stagger = rtActionStagger
			g.advanceRTActor(rtActWeapon)
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

	// Now everyone is healthy -> smart-attack must NOT heal (weapon swing instead).
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints
	}
	spBefore := caster.SpellPoints
	_, spellID := cs.SmartAttack() // acted=true (weapon swing), but no SPELL may fire
	if spellID != "" {
		t.Errorf("smart-attack cast %q on a full-HP party; should have attacked with the weapon", spellID)
	}
	if caster.SpellPoints != spBefore {
		t.Errorf("smart-attack spent SP with no wounded ally (%d -> %d)", spBefore, caster.SpellPoints)
	}
}

// TestSmartAttack_BookHealWithoutQuickSlot: a healer with an EMPTY quick slot
// still auto-triages - Space finds the strongest heal in the spellbook when
// anyone in the party (not just the caster) is wounded.
func TestSmartAttack_BookHealWithoutQuickSlot(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 3 {
		t.Skip("need >=3 party members")
	}
	cleric := members[2] // Celestine knows heal_other from the class kit
	g.selectedChar = 2
	delete(cleric.Equipment, items.SlotSpell) // no quick-slotted spell at all
	cleric.SpellPoints, cleric.MaxSpellPoints = 50, 50
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints
	}
	hurt := members[0] // someone OTHER than the caster
	hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
	before := hurt.HitPoints

	cast, id := cs.SmartAttack()
	if !cast || id != spells.SpellID("heal_other") {
		t.Fatalf("expected book heal_other on the wounded ally, got cast=%v id=%q", cast, id)
	}
	if hurt.HitPoints <= before {
		t.Errorf("wounded ally not healed: %d -> %d", before, hurt.HitPoints)
	}
}

// TestSmartAttack_BookHealOverOffensiveQuickSlot: a healer keeping a COMBAT
// spell in the quick slot still heals first - the book heal outranks the
// slotted offensive spell while an ally is wounded.
func TestSmartAttack_BookHealOverOffensiveQuickSlot(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 3 {
		t.Skip("need >=3 party members")
	}
	cleric := members[2]
	g.selectedChar = 2
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("harm")); err == nil {
		cleric.Equipment[items.SlotSpell] = spellItem
	} else {
		t.Fatalf("create harm spell item: %v", err)
	}
	cleric.SpellPoints, cleric.MaxSpellPoints = 50, 50
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints
	}
	hurt := members[1]
	hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
	before := hurt.HitPoints

	cast, id := cs.SmartAttack()
	if !cast || id != spells.SpellID("heal_other") {
		t.Fatalf("expected the book heal to outrank the slotted combat spell, got cast=%v id=%q", cast, id)
	}
	if hurt.HitPoints <= before {
		t.Errorf("wounded ally not healed: %d -> %d", before, hurt.HitPoints)
	}
}

// TestSmartAttack_QuickSlottedHealPreferred: when the quick slot holds a heal,
// it wins over the book pick (here Mass Heal from the slot vs heal_other in
// the book).
func TestSmartAttack_QuickSlottedHealPreferred(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	if len(members) < 3 {
		t.Skip("need >=3 party members")
	}
	cleric := members[2]
	g.selectedChar = 2
	if spellItem, err := spells.CreateSpellItem(spells.SpellID("mass_heal")); err == nil {
		cleric.Equipment[items.SlotSpell] = spellItem
	} else {
		t.Fatalf("create mass_heal spell item: %v", err)
	}
	cleric.SpellPoints, cleric.MaxSpellPoints = 80, 80
	for _, m := range members {
		m.HitPoints = m.MaxHitPoints
	}
	hurt := members[1]
	hurt.HitPoints = hurt.MaxHitPoints * 30 / 100

	cast, id := cs.SmartAttack()
	if !cast || id != spells.SpellID("mass_heal") {
		t.Fatalf("expected the quick-slotted heal to be preferred over the book heal, got cast=%v id=%q", cast, id)
	}
}

// TestRTCycle_CapabilityAware: holding F cycles only casters, C only healers,
// R only the armed - incapable members are skipped, and a capable member on
// cooldown is WAITED on (selection lands there) rather than jumping to an
// incapable one.
func TestRTCycle_CapabilityAware(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	m := g.party.Members
	if len(m) < 4 {
		t.Skip("need >=4 party members")
	}
	for _, c := range m {
		c.Equipment = map[items.EquipSlot]items.Item{}
		c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
		c.SpellPoints, c.MaxSpellPoints = 100, 100
		c.HitPoints, c.MaxHitPoints = 50, 50
		c.RTCooldown = 0
	}
	m[0].Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")                                                       // only armed
	m[1].Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: items.SpellEffect("fireball"), SpellCost: 4} // only caster
	m[2].MagicSchools[character.MagicSchoolBody] = &character.MagicSkill{KnownSpells: []spells.SpellID{"heal"}}                         // only healer

	check := func(kind rtActionKind, from, want int, msg string) {
		g.selectedChar = from
		g.advanceRTActor(kind)
		if g.selectedChar != want {
			t.Errorf("%s: from %d want %d, got %d", msg, from, want, g.selectedChar)
		}
	}
	check(rtActCast, 0, 1, "F lands on the only caster")
	check(rtActCast, 1, 1, "F stays on the sole caster")
	check(rtActHeal, 0, 2, "C lands on the only healer")
	check(rtActHeal, 3, 2, "C skips non-healers to the healer")
	check(rtActWeapon, 3, 0, "R lands on the only armed")

	// Caster on cooldown: F waits on them (capable), not jump to an incapable member.
	m[1].RTCooldown = 30
	check(rtActCast, 0, 1, "F waits on the capable caster even on cooldown")
	if g.rtActionReady(1, rtActCast) {
		t.Errorf("caster on cooldown must not be ready")
	}
	// A broke caster (no SP) is incapable -> not selected.
	m[1].RTCooldown = 0
	m[1].SpellPoints = 0
	g.selectedChar = 0
	g.advanceRTActor(rtActCast)
	if g.selectedChar == 1 {
		t.Errorf("F should skip a caster with no SP")
	}
}

// TestRTCycle_WaitsQuietlyWhenAllOnCooldown: holding F while every caster is on
// cooldown must NOT churn the selection frame each tick - it parks on a capable
// caster once and holds there until one is ready. Mirrors the pre-fire selection
// logic in handleCombatInput.
func TestRTCycle_WaitsQuietlyWhenAllOnCooldown(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	m := g.party.Members
	if len(m) < 4 {
		t.Skip("need >=4 party members")
	}
	for _, c := range m {
		c.Equipment = map[items.EquipSlot]items.Item{}
		c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
		c.SpellPoints, c.MaxSpellPoints = 100, 100
		c.HitPoints, c.MaxHitPoints = 50, 50
		c.RTCooldown = 0
	}
	// Two casters (1 & 3), both on cooldown; non-casters elsewhere.
	m[1].Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: items.SpellEffect("fireball"), SpellCost: 4}
	m[3].Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: items.SpellEffect("fireball"), SpellCost: 4}
	m[1].RTCooldown, m[3].RTCooldown = 40, 40

	// Replays the pre-fire selection steps without firing.
	step := func(kind rtActionKind) {
		g.ensureSelectedCanActRT()
		if !g.rtActionCapable(g.selectedChar, kind) {
			g.advanceRTActor(kind)
		}
		if !g.rtActionReady(g.selectedChar, kind) {
			if i := g.nextReadyRTActor(kind); i >= 0 {
				g.selectedChar = i
			}
		}
	}

	g.selectedChar = 0 // start on a non-caster
	step(rtActCast)    // one-time park onto a capable caster
	parked := g.selectedChar
	if parked != 1 && parked != 3 {
		t.Fatalf("F should park on a caster, got %d", parked)
	}
	for f := 0; f < 30; f++ { // tick cooldowns down (still >0) - must not move
		m[1].RTCooldown--
		m[3].RTCooldown--
		step(rtActCast)
		if g.selectedChar != parked {
			t.Fatalf("frame %d: selection jittered off the parked caster %d -> %d", f, parked, g.selectedChar)
		}
	}
	// When a caster comes off cooldown, selection may move to fire it.
	m[1].RTCooldown, m[3].RTCooldown = 0, 0
	step(rtActCast)
	if !g.rtActionReady(g.selectedChar, rtActCast) {
		t.Errorf("once ready, selection should rest on a fireable caster, got %d", g.selectedChar)
	}
}
