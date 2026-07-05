package game

// Arms Master (Dual Wielding) and Monk (Iron Body / Spiritual Training)
// mechanics tests.

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// dualWield equips member with a main-hand sword and an off-hand dagger,
// granting exactly the skills needed for both to be legal.
func makeDualWielder(t *testing.T, member *character.MMCharacter) {
	t.Helper()
	member.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillDagger] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	member.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("magic_dagger")
}

func TestAttackSlotFor_RTPicksWhicheverHandIsReady(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	member := g.party.Members[0]
	makeDualWielder(t, member)

	member.RTCooldown, member.OffHandRTCooldown = 0, 0
	if got := cs.attackSlotFor(member); got != items.SlotMainHand {
		t.Errorf("both hands ready: slot = %v, want SlotMainHand (main preferred)", got)
	}
	member.NextTBAttackOffHand = true
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("both hands ready, off-hand cursor: slot = %v, want SlotOffHand", got)
	}
	member.NextTBAttackOffHand = false

	member.RTCooldown = 30
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("main hand on cooldown: slot = %v, want SlotOffHand", got)
	}

	// attackSlotFor only ever gets consulted after AnyWeaponHandReady() has
	// confirmed at least one hand is free - "both busy" is a can't-happen
	// input in practice, but the function must still degrade predictably
	// (falls to the off-hand, same as "main hand not ready") rather than panic
	// or pick unpredictably.
	member.RTCooldown, member.OffHandRTCooldown = 30, 30
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("both hands busy (can't-happen input): slot = %v, want the same SlotOffHand fallback as main-hand-busy", got)
	}
}

// TestBowAndAxeDualWield_DispatchesRangedOrMeleeByResolvedHand covers a
// mixed loadout (melee main hand, ranged off hand): EquipmentMeleeAttack must
// dispatch melee-vs-ranged off whichever hand attackSlotFor actually resolved,
// not always the main hand - createArrowAttack must read that SAME hand too
// (it used to hardcode SlotMainHand internally, silently firing the wrong
// weapon's bow key/physics when the resolved hand was the off-hand).
func TestBowAndAxeDualWield_DispatchesRangedOrMeleeByResolvedHand(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	member := g.party.Members[0]
	member.Skills[character.SkillAxe] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillBow] = &character.Skill{Mastery: character.MasteryNovice}
	member.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("steel_axe")
	member.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("hunting_bow")
	g.selectedChar = 0

	// Main hand (axe, melee) resolved: swinging must NOT spawn an arrow.
	member.RTCooldown, member.OffHandRTCooldown = 0, 0
	arrowsBefore := len(g.arrows)
	if !cs.EquipmentMeleeAttack() {
		t.Fatal("attack with the axe ready should succeed")
	}
	if len(g.arrows) != arrowsBefore {
		t.Errorf("axe swing spawned an arrow (%d -> %d) - resolved the wrong hand", arrowsBefore, len(g.arrows))
	}

	// Main hand busy: attackSlotFor must resolve to the off-hand bow.
	member.RTCooldown, member.OffHandRTCooldown = 999, 0
	arrowsBefore = len(g.arrows)
	if !cs.EquipmentMeleeAttack() {
		t.Fatal("attack with the bow ready (off-hand) should succeed")
	}
	if len(g.arrows) != arrowsBefore+1 {
		t.Fatalf("bow swing (off-hand) should have spawned exactly one arrow, got %d -> %d", arrowsBefore, len(g.arrows))
	}
	if last := g.arrows[len(g.arrows)-1]; last.BowKey != "hunting_bow" {
		t.Errorf("arrow BowKey = %q, want hunting_bow - createArrowAttack read the wrong slot", last.BowKey)
	}
}

func TestAttackSlotFor_NonDualWielderAlwaysMainHand(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	delete(member.Equipment, items.SlotOffHand)
	member.RTCooldown = 999 // even on cooldown, a single-weapon fighter has nowhere else to go

	if got := cs.attackSlotFor(member); got != items.SlotMainHand {
		t.Errorf("non-dual-wielder slot = %v, want SlotMainHand always", got)
	}
}

func TestAttackSlotFor_TBFollowsCursorAndWraps(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	member := g.party.Members[0]
	makeDualWielder(t, member)

	member.NextTBAttackOffHand = false
	if got := cs.attackSlotFor(member); got != items.SlotMainHand {
		t.Errorf("cursor=false: slot = %v, want SlotMainHand", got)
	}
	member.NextTBAttackOffHand = true
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("cursor=true: slot = %v, want SlotOffHand", got)
	}
}

func TestAnyWeaponHandReady_DualWielderReadyOnEitherHand(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	makeDualWielder(t, member)

	member.RTCooldown, member.OffHandRTCooldown = 30, 0
	if !member.AnyWeaponHandReady() {
		t.Error("off-hand ready should count as ready even with main hand on cooldown")
	}
	member.RTCooldown, member.OffHandRTCooldown = 30, 30
	if member.AnyWeaponHandReady() {
		t.Error("both hands on cooldown should not be ready")
	}
}

// TestCommitRTWeaponAttack_SetsOnlyTheHandThatSwung verifies the off-hand's
// cooldown is set independently - the main hand's timer must be untouched.
func TestCommitRTWeaponAttack_SetsOnlyTheHandThatSwung(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	ih := &InputHandler{game: g}
	g.selectedChar = 0
	member := g.party.Members[0]
	makeDualWielder(t, member)

	member.RTCooldown = 30 // main hand busy -> attackSlotFor picks the off-hand
	member.OffHandRTCooldown = 0

	ih.commitRTWeaponAttack(rtActWeapon, member)

	if member.RTCooldown != 30 {
		t.Errorf("main hand cooldown should be untouched by an off-hand swing, got %d", member.RTCooldown)
	}
	if member.OffHandRTCooldown <= 0 {
		t.Error("off-hand cooldown should be set after it swung")
	}
	if member.NextTBAttackOffHand {
		t.Error("after an off-hand RT swing, cursor should wrap back to main hand")
	}
}

func TestDualWield_MixedBowAndMuramasaAlternateInRTAndTB(t *testing.T) {
	tests := []struct {
		name        string
		mainWeapon  string
		offWeapon   string
		firstArrow  bool
		secondArrow bool
	}{
		{
			name:        "main bow offhand muramasa",
			mainWeapon:  "elven_bow",
			offWeapon:   "muramasa",
			firstArrow:  true,
			secondArrow: false,
		},
		{
			name:        "main muramasa offhand bow",
			mainWeapon:  "muramasa",
			offWeapon:   "elven_bow",
			firstArrow:  false,
			secondArrow: true,
		},
	}

	for _, tt := range tests {
		for _, turnBased := range []bool{false, true} {
			mode := "RT"
			if turnBased {
				mode = "TB"
			}
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = turnBased
				ih := &InputHandler{game: g}
				g.selectedChar = 0
				member := g.party.Members[0]
				member.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryNovice}
				member.Skills[character.SkillBow] = &character.Skill{Mastery: character.MasteryNovice}
				member.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
				member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(tt.mainWeapon)
				member.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML(tt.offWeapon)
				member.RTCooldown, member.OffHandRTCooldown = 0, 0
				member.NextTBAttackOffHand = false
				member.ActionsRemaining = 2

				assertAttackArrow := func(wantArrow bool) {
					t.Helper()
					before := len(g.arrows)
					if !cs.EquipmentMeleeAttack() {
						t.Fatal("weapon attack should act")
					}
					gotArrow := len(g.arrows) == before+1
					if gotArrow != wantArrow {
						t.Fatalf("arrow spawned = %v, want %v (arrows %d -> %d)", gotArrow, wantArrow, before, len(g.arrows))
					}
					if gotArrow {
						if last := g.arrows[len(g.arrows)-1]; last.BowKey != "elven_bow" {
							t.Fatalf("arrow BowKey = %q, want elven_bow", last.BowKey)
						}
					}
				}

				assertAttackArrow(tt.firstArrow)
				if turnBased {
					g.consumeSelectedCharWeaponAction()
				} else {
					ih.commitRTWeaponAttack(rtActWeapon, member)
					if !g.rtActionReady(0, rtActSmart) {
						t.Fatal("RT SmartAttack should be ready for the off-hand weapon fallback while main hand cools down")
					}
					g.selectedChar = 0
					// Also cover the other RT failure mode: by the time selection
					// cycles back, both hands can be ready again. The cursor must
					// still give the other hand its turn instead of preferring
					// slot 1 forever.
					member.RTCooldown, member.OffHandRTCooldown = 0, 0
				}
				assertAttackArrow(tt.secondArrow)
			})
		}
	}
}

// TestStartPartyTurn_DualWielderGetsTwoPersonalActions verifies the personal
// Dual Wielding bonus is independent of the party-wide Speed bonus pool.
func TestStartPartyTurn_DualWielderGetsTwoPersonalActions(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	member := g.party.Members[0]
	makeDualWielder(t, member)
	member.Speed = 10 // well under the Speed-bonus-action thresholds

	g.startPartyTurn()

	if member.ActionsRemaining != 2 {
		t.Errorf("dual-wielder ActionsRemaining = %d, want 2 (personal bonus, no Speed bonus involved)", member.ActionsRemaining)
	}
	if member.NextTBAttackOffHand {
		t.Error("a fresh round must start on the main hand")
	}
}

// TestConsumeSelectedCharWeaponAction_FlipsCursorEachSwing verifies the cursor
// alternates main->off->main, so a 3rd action (e.g. from a Speed bonus) wraps
// back to the main hand automatically.
func TestConsumeSelectedCharWeaponAction_FlipsCursorEachSwing(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	member := g.party.Members[0]
	makeDualWielder(t, member)
	member.ActionsRemaining = 3
	g.selectedChar = 0

	if member.NextTBAttackOffHand {
		t.Fatal("setup: cursor should start false")
	}
	g.consumeSelectedCharWeaponAction() // swing 1: main hand -> flips to true
	if !member.NextTBAttackOffHand {
		t.Error("after 1st swing, cursor should point to the off-hand")
	}
	g.consumeSelectedCharWeaponAction() // swing 2: off hand -> flips to false
	if member.NextTBAttackOffHand {
		t.Error("after 2nd swing, cursor should wrap back to the main hand")
	}
	g.consumeSelectedCharWeaponAction() // swing 3 (e.g. Speed bonus): main hand again
	if !member.NextTBAttackOffHand {
		t.Error("after 3rd swing, cursor should point to the off-hand again")
	}
}

// TestIronBodyAddsFlatACPerTierIncludingNovice verifies (tier+1)*AC - the
// Novice-included idiom (unlike Bodybuilding/ArmsMaster, which are zero at
// Novice) - since a Monk has no armor slots to fall back on.
func TestIronBodyAddsFlatACPerTierIncludingNovice(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	member.Equipment = map[items.EquipSlot]items.Item{} // no armor at all, like a Monk

	base := cs.CalculateTotalArmorClass(member)

	member.Skills[character.SkillIronBody] = &character.Skill{Mastery: character.MasteryNovice}
	if got, want := cs.CalculateTotalArmorClass(member), base+character.IronBodyACPerTier; got != want {
		t.Errorf("Iron Body Novice AC = %d, want %d (base %d + %d)", got, want, base, character.IronBodyACPerTier)
	}

	member.Skills[character.SkillIronBody].Mastery = character.MasteryGrandMaster
	if got, want := cs.CalculateTotalArmorClass(member), base+4*character.IronBodyACPerTier; got != want {
		t.Errorf("Iron Body Grandmaster AC = %d, want %d (base %d + %d)", got, want, base, 4*character.IronBodyACPerTier)
	}
}

func TestIronBodyGrandmasterAddsPerfectDodge(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	member.Luck = 0
	member.BuffBonuses = character.StatBonuses{}
	member.Equipment = map[items.EquipSlot]items.Item{}

	member.Skills[character.SkillIronBody] = &character.Skill{Mastery: character.MasteryNovice}
	if _, got := cs.RollPerfectDodge(member); got != 0 {
		t.Fatalf("Iron Body Novice dodge = %d, want 0", got)
	}

	member.Skills[character.SkillIronBody].Mastery = character.MasteryGrandMaster
	if _, got := cs.RollPerfectDodge(member); got != character.IronBodyGMDodgeBonus {
		t.Fatalf("Iron Body Grandmaster dodge = %d, want %d", got, character.IronBodyGMDodgeBonus)
	}
}

func TestSmartAttack_MonkSkipsOffensiveQuickSpellForWeaponAttack(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.Class = character.ClassMonk
	member.Skills = map[character.SkillType]*character.Skill{
		character.SkillMartialArts: {Mastery: character.MasteryNovice},
	}
	member.Equipment = map[items.EquipSlot]items.Item{
		items.SlotMainHand: items.CreateWeaponFromYAML("monk_fists"),
	}
	spellItem, err := spells.CreateSpellItem(spells.SpellID("mind_blast"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(mind_blast): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 50, 50
	g.selectedChar = 0
	g.world.Monsters = nil

	acted, spellID := cs.SmartAttack()
	if !acted {
		t.Fatal("monk smart attack should swing fists when no heal is needed")
	}
	if spellID != "" {
		t.Fatalf("monk smart attack cast quick spell %q; want weapon attack", spellID)
	}
	if len(g.magicProjectiles) != 0 {
		t.Fatalf("monk smart attack should not cast the quick spell directly, spawned %d projectiles", len(g.magicProjectiles))
	}
}

// TestSpiritualTrainingChanceFormulaAndFreeCast checks the tier->chance
// formula (deterministic) and that the proc never spends spell points across
// many trials at the highest chance tier - it must be free (0 SP) whether or
// not it actually fires, mirroring the Pixie Card's free Fire Bolt proc.
func TestSpiritualTrainingChanceFormulaAndFreeCast(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]

	if got := spiritualTrainingChancePct(member); got != 0 {
		t.Fatalf("no skill: chance = %d, want 0", got)
	}

	member.Skills[character.SkillSpiritualTraining] = &character.Skill{Mastery: character.MasteryNovice}
	if got, want := spiritualTrainingChancePct(member), character.SpiritualTrainingProcPctPerTier; got != want {
		t.Errorf("Novice chance = %d, want %d", got, want)
	}
	member.Skills[character.SkillSpiritualTraining].Mastery = character.MasteryGrandMaster
	if got, want := spiritualTrainingChancePct(member), 4*character.SpiritualTrainingProcPctPerTier; got != want {
		t.Errorf("Grandmaster chance = %d, want %d", got, want)
	}

	spellItem, err := spells.CreateSpellItem(spells.SpellID("mind_blast"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(mind_blast): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 20, 20

	for i := 0; i < 300; i++ {
		before := member.SpellPoints
		cs.trySpiritualTraining(member)
		if member.SpellPoints != before {
			t.Fatalf("Spiritual Training proc spent SP (%d -> %d); it must always be free", before, member.SpellPoints)
		}
	}
}

func TestSpiritualTrainingRollsOnAttackActionWithoutMonsterHit(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.Class = character.ClassMonk
	member.Skills = map[character.SkillType]*character.Skill{
		character.SkillMartialArts:       {Mastery: character.MasteryNovice},
		character.SkillSpiritualTraining: {Mastery: character.MasteryGrandMaster},
	}
	member.Equipment = map[items.EquipSlot]items.Item{
		items.SlotMainHand: items.CreateWeaponFromYAML("monk_fists"),
	}
	spellItem, err := spells.CreateSpellItem(spells.SpellID("mind_blast"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(mind_blast): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 20, 20
	g.selectedChar = 0
	g.world.Monsters = nil

	for i := 0; i < 300 && len(g.magicProjectiles) == 0; i++ {
		if !cs.EquipmentMeleeAttack() {
			t.Fatal("monk weapon attack should act even with no monster in arc")
		}
	}
	if len(g.magicProjectiles) == 0 {
		t.Fatal("Spiritual Training never proc'd from attack actions without monster hits")
	}
	if member.SpellPoints != 20 {
		t.Fatalf("Spiritual Training spent SP: got %d, want 20", member.SpellPoints)
	}
}

// TestSpiritualTrainingNeverFiresWithoutTheSkill is a no-op safety net: with
// chance forced to 0 (no skill), 200 calls must never touch SP or panic.
func TestSpiritualTrainingNeverFiresWithoutTheSkill(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	spellItem, err := spells.CreateSpellItem(spells.SpellID("mind_blast"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(mind_blast): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 20, 20

	for i := 0; i < 200; i++ {
		cs.trySpiritualTraining(member)
	}
	if member.SpellPoints != 20 {
		t.Errorf("SP changed to %d without Spiritual Training skill at all", member.SpellPoints)
	}
}

// TestRtActionReady_SmartAllowsOffhandWeaponFallback verifies Space
// (rtActSmart) can wake up for an off-hand-only-ready dual-wielder. input.go
// handles that state as weapon fallback only, so it does not let heals/spells
// sneak past a busy main-hand cooldown.
func TestRtActionReady_SmartAllowsOffhandWeaponFallback(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	makeDualWielder(t, member)

	member.RTCooldown = 999 // main hand / spell cooldown busy
	member.OffHandRTCooldown = 0

	if !g.rtActionReady(0, rtActWeapon) {
		t.Error("rtActWeapon should still be ready off the free off-hand")
	}
	if !g.rtActionReady(0, rtActSmart) {
		t.Error("rtActSmart should be ready for off-hand weapon fallback")
	}
}

// TestAttackSlotFor_RedirectsToOffHandWhenMainHandUnequipped covers unequipping
// just the main hand of a dual-wielder (legal for anyone but a zero-other-
// weapon-skill character - see HasAnyWeaponSkill). attackSlotFor must not
// blindly follow cooldown/cursor into a slot that's actually empty.
func TestAttackSlotFor_RedirectsToOffHandWhenMainHandUnequipped(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	makeDualWielder(t, member)
	delete(member.Equipment, items.SlotMainHand)

	g.turnBasedMode = false
	member.RTCooldown, member.OffHandRTCooldown = 0, 0 // main hand would normally win
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("RT, main hand empty: slot = %v, want SlotOffHand", got)
	}

	g.turnBasedMode = true
	member.NextTBAttackOffHand = false // cursor would normally point at the main hand
	if got := cs.attackSlotFor(member); got != items.SlotOffHand {
		t.Errorf("TB, main hand empty: slot = %v, want SlotOffHand", got)
	}
}

// TestAnyWeaponHandReady_IgnoresStaleMainHandCooldownWhenUnequipped verifies
// readiness follows the off-hand's OWN cooldown once the main hand is
// unequipped - the empty main hand's cooldown clearing must not make an
// actually-busy off-hand look ready.
func TestAnyWeaponHandReady_IgnoresStaleMainHandCooldownWhenUnequipped(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]
	makeDualWielder(t, member)
	delete(member.Equipment, items.SlotMainHand)

	member.RTCooldown, member.OffHandRTCooldown = 0, 30
	if member.AnyWeaponHandReady() {
		t.Error("off-hand still on cooldown should NOT be ready just because the empty main hand's cooldown cleared")
	}
	member.OffHandRTCooldown = 0
	if !member.AnyWeaponHandReady() {
		t.Error("off-hand off cooldown should be ready")
	}
}

// TestEquipmentMeleeAttack_SwingsOffHandWhenMainHandUnequipped is the
// end-to-end reproduction: an Arms Master who unequips just the main hand
// (nothing stops that - the unequip guard only protects a zero-other-
// weapon-skill character) must still be able to swing the remaining
// off-hand weapon via the normal attack action, not soft-lock.
func TestEquipmentMeleeAttack_SwingsOffHandWhenMainHandUnequipped(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	member := g.party.Members[0]
	makeDualWielder(t, member)
	delete(member.Equipment, items.SlotMainHand)
	member.RTCooldown, member.OffHandRTCooldown = 0, 0
	g.selectedChar = 0

	if !cs.EquipmentMeleeAttack() {
		t.Fatal("attack should succeed by swinging the remaining off-hand weapon")
	}
}

// setupSummonableWorld gives cs a small real walkable map + tile manager so
// tryCardSummonOnAction's spawn search (findNearestSummonTile) can actually
// place an ally - without this, an empty World3D{} silently fails every
// spawn attempt regardless of whether the roll fired, making "did it summon"
// unobservable.
func setupSummonableWorld(t *testing.T, cs *CombatSystem) {
	t.Helper()
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tile config: %v", err)
	}
	tw := world.NewWorld3D(cs.game.config)
	tw.Width, tw.Height = 30, 30
	tw.Tiles = make([][]world.TileType3D, tw.Height)
	for y := range tw.Tiles {
		tw.Tiles[y] = make([]world.TileType3D, tw.Width)
		for x := range tw.Tiles[y] {
			tw.Tiles[y][x] = world.TileEmpty
		}
	}
	cs.game.world = tw
	tile := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = 15*tile, 15*tile
}

// forceOrcWarlordSummonAlways stacks the Orc Warlord Card in every slot so
// tryCardSummonOnAction's percentage roll always passes (chance >> 100),
// turning a probabilistic proc into a deterministic "did this even attempt
// to roll at all".
func forceOrcWarlordSummonAlways(g *MMGame) {
	for i := range g.cardSlots {
		g.cardSlots[i].key = "orc_warlord_card"
	}
}

// TestTrySpiritualTraining_NeverRollsItsOwnOrcWarlordSummon is the regression
// guard for the double-roll bug: trySpiritualTraining's free cast used to go
// through the normal castResolvedSpell path, which unconditionally rolled
// tryCardSummonOnAction - a second, independent roll on top of the one
// EquipmentMeleeAttack already made for the same attack action. With the
// summon chance forced far past 100%, any leak here spawns allies
// immediately instead of hiding behind low odds.
func TestTrySpiritualTraining_NeverRollsItsOwnOrcWarlordSummon(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	setupSummonableWorld(t, cs)
	forceOrcWarlordSummonAlways(g)

	member := g.party.Members[0]
	member.Skills[character.SkillSpiritualTraining] = &character.Skill{Mastery: character.MasteryGrandMaster}
	spellItem, err := spells.CreateSpellItem(spells.SpellID("mind_blast"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(mind_blast): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 20, 20

	for i := 0; i < 300; i++ {
		cs.trySpiritualTraining(member)
	}

	if got := cs.countCardSummons(); got != 0 {
		t.Errorf("countCardSummons() = %d, want 0 - trySpiritualTraining's free cast must never roll "+
			"the Orc Warlord summon itself (that roll belongs to the swing that triggered it)", got)
	}
}

// TestEquipmentMeleeAttack_StillRollsOrcWarlordSummonWithoutSpiritualTraining
// is the contrast case: a character with no Spiritual Training must still
// trigger the normal once-per-action summon roll, confirming the fix didn't
// also silence the legitimate call site.
func TestEquipmentMeleeAttack_StillRollsOrcWarlordSummonWithoutSpiritualTraining(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	setupSummonableWorld(t, cs)
	forceOrcWarlordSummonAlways(g)

	member := g.party.Members[0]
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	delete(member.Skills, character.SkillSpiritualTraining)
	g.selectedChar = 0
	g.world.Monsters = nil

	if !cs.EquipmentMeleeAttack() {
		t.Fatal("attack should succeed")
	}
	if got := cs.countCardSummons(); got == 0 {
		t.Error("countCardSummons() = 0, want > 0 - the normal attack-action summon roll should still fire")
	}
}

// TestSmartAttack_OffhandFallbackDoesNotCastPastMainCooldown pins the P2 fix:
// rtActionReady lets an off-hand-ready dual-wielder use Space even while the
// main hand / cast cooldown is up (so Space can swing the off-hand), but
// SmartAttack must NOT spend that opening on a free heal/offensive spell -
// those are gated by the main RTCooldown. It must fall through to the weapon.
func TestSmartAttack_OffhandFallbackDoesNotCastPastMainCooldown(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = false
	member := g.party.Members[0]
	member.Class = character.ClassKnight // non-Monk: the quick-spell branch is live
	makeDualWielder(t, member)
	spellItem, err := spells.CreateSpellItem(spells.SpellID("firebolt"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(firebolt): %v", err)
	}
	member.Equipment[items.SlotSpell] = spellItem
	member.SpellPoints, member.MaxSpellPoints = 99, 99
	g.selectedChar = 0

	// Main hand busy, off-hand free.
	member.RTCooldown, member.OffHandRTCooldown = 999, 0
	if !g.rtActionReady(0, rtActSmart) {
		t.Fatal("an off-hand-ready dual-wielder should still qualify for Space")
	}
	projBefore := len(g.magicProjectiles)
	acted, spellID := cs.SmartAttack()
	if !acted {
		t.Fatal("Space should swing the off-hand weapon")
	}
	if spellID != "" {
		t.Errorf("Space cast %q past the main cooldown; want an off-hand weapon swing", spellID)
	}
	if len(g.magicProjectiles) != projBefore {
		t.Errorf("a spell projectile spawned (%d -> %d) despite the main cooldown being up", projBefore, len(g.magicProjectiles))
	}

	// Sanity: once the main hand is ready, Space DOES cast the equipped spell.
	member.RTCooldown = 0
	if _, spellID := cs.SmartAttack(); spellID == "" {
		t.Error("with the main hand ready, Space should cast the equipped offensive spell")
	}
}

// TestSpiritualTraining_DoesNotFreeProcPartyBuffs pins the offensive-only gate:
// a non-offensive utility spell (Bless) slotted as the quick-spell must NOT be
// free-cast by melee swings. Without the IsOffensive() filter a Monk could keep
// Bless/Heroism/Stone Skin/Hour of Power permanently up for 0 SP off autoattacks.
func TestSpiritualTraining_DoesNotFreeProcPartyBuffs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	member := g.party.Members[0]
	member.Skills[character.SkillSpiritualTraining] = &character.Skill{Mastery: character.MasteryGrandMaster}
	blessItem, err := spells.CreateSpellItem(spells.SpellID("bless"))
	if err != nil {
		t.Fatalf("setup: CreateSpellItem(bless): %v", err)
	}
	member.Equipment[items.SlotSpell] = blessItem
	member.SpellPoints, member.MaxSpellPoints = 99, 99
	g.selectedChar = 0

	for i := 0; i < 500; i++ {
		cs.trySpiritualTraining(member)
	}
	if len(g.statBuffs) != 0 {
		t.Errorf("Spiritual Training free-proc'd a party buff (Bless): statBuffs=%d, want 0 - only offensive spells should proc", len(g.statBuffs))
	}
}
