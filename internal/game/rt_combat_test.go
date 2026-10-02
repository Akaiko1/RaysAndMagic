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

// TestRTHoldSpace_HighSpeedAttacksMore: holding the attack key drives the real
// RT dispatcher every frame; a high-Speed member (shorter cooldown) acts more
// often than slow ones, and round-robin never starves anyone.
func TestRTHoldSpace_HighSpeedAttacksMore(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	members := g.party.Members
	n := len(members)
	if n < 2 {
		t.Skip("need >=2 party members")
	}
	// Isolate Speed: same weapon for all, equal HP, all ready, all weapon-capable.
	for i, m := range members {
		m.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
		delete(m.Equipment, items.SlotOffHand)
		m.HitPoints, m.MaxHitPoints = 50, 50
		m.RTCooldown, m.OffHandRTCooldown = 0, 0
		m.Speed = 5
		if i == 0 {
			m.Speed = 60 // the fast one
		}
	}

	ih := NewInputHandler(g)
	counts := make([]int, n)
	g.selectedChar = 0
	for f := 0; f < 20*g.config.GetTPS(); f++ {
		// The per-frame RT clocks the game loop advances.
		if g.spellInputCooldown > 0 {
			g.spellInputCooldown--
		}
		before := make([]int, n)
		for i, m := range members {
			if m.RTCooldown > 0 {
				m.RTCooldown--
			}
			before[i] = m.RTCooldown
		}
		ih.performRTCombatAction(rtActWeapon, false)
		for i, m := range members {
			if m.RTCooldown > before[i] {
				counts[i]++
			}
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

// TestSmartAttack_HealPriority: Space heals the most-wounded ally first. A
// slotted heal wins over the book pick, a book heal outranks a slotted combat
// spell or an empty slot, and a healthy party gets no spell at all.
func TestSmartAttack_HealPriority(t *testing.T) {
	slot := func(t *testing.T, c *character.MMCharacter, id spells.SpellID) {
		t.Helper()
		spellItem, err := spells.CreateSpellItem(id)
		if err != nil {
			t.Fatalf("create %s spell item: %v", id, err)
		}
		c.LearnSpell(id)
		c.Equipment[items.SlotSpell] = spellItem
	}
	slottedHeal := func(t *testing.T, c *character.MMCharacter) {
		c.LearnSpell("heal_other")
		c.Equipment[items.SlotSpell] = items.Item{
			Name: "Heal", Type: items.ItemUtilitySpell,
			SpellEffect: items.SpellEffectHealOther, SpellCost: 4,
		}
	}
	for _, tc := range []struct {
		name   string
		caster int // Celestine (2) knows heal_other from the class kit
		hurt   int // -1: the whole party is healthy
		setup  func(t *testing.T, c *character.MMCharacter)
		sp     int
		want   spells.SpellID
	}{
		{"slotted_heal_heals_most_wounded", 0, 1, slottedHeal, 50, "heal_other"},
		{"healthy_party_casts_nothing", 0, -1, slottedHeal, 50, ""},
		{"book_heal_without_quick_slot", 2, 0, func(_ *testing.T, c *character.MMCharacter) { delete(c.Equipment, items.SlotSpell) }, 50, "heal_other"},
		{"book_heal_over_offensive_slot", 2, 1, func(t *testing.T, c *character.MMCharacter) { slot(t, c, "harm") }, 50, "heal_other"},
		{"slotted_heal_over_book_heal", 2, 1, func(t *testing.T, c *character.MMCharacter) { slot(t, c, "mass_heal") }, 80, "mass_heal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = false
			members := g.party.Members
			if len(members) <= max(tc.caster, tc.hurt) {
				t.Skipf("need >%d party members", max(tc.caster, tc.hurt))
			}
			caster := members[tc.caster]
			g.selectedChar = tc.caster
			tc.setup(t, caster)
			caster.SpellPoints, caster.MaxSpellPoints = tc.sp, tc.sp
			for _, m := range members {
				m.HitPoints = m.MaxHitPoints
			}
			before := 0
			if tc.hurt >= 0 {
				hurt := members[tc.hurt]
				hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
				before = hurt.HitPoints
			}
			spBefore := caster.SpellPoints

			cast, id := cs.SmartAttack()
			if id != tc.want || (tc.want != "" && !cast) {
				t.Fatalf("SmartAttack cast=%v id=%q, want %q", cast, id, tc.want)
			}
			if tc.hurt < 0 {
				if caster.SpellPoints != spBefore {
					t.Errorf("smart-attack spent SP with no wounded ally (%d -> %d)", spBefore, caster.SpellPoints)
				}
				return
			}
			if got := members[tc.hurt].HitPoints; got <= before {
				t.Errorf("wounded ally not healed: %d -> %d", before, got)
			}
		})
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
