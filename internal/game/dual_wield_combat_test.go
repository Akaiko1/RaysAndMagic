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

// attackSlotFor case table. RT follows readiness (main preferred, the cursor
// breaks a both-ready tie), TB follows the cursor; a single weapon always swings
// the main hand; an empty main hand always redirects to the off hand. "Both
// busy" cannot reach attackSlotFor (AnyWeaponHandReady gates it) but must still
// degrade like "main busy" instead of panicking.
func TestAttackSlotFor_CaseTable(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tb              bool
		gear            string // dual, single, dual_no_main
		mainCD, offCD   int
		offCursor, want bool // want: true = off hand
	}{
		{name: "rt/dual/both_ready", gear: "dual"},
		{name: "rt/dual/both_ready_off_cursor", gear: "dual", offCursor: true, want: true},
		{name: "rt/dual/main_busy", gear: "dual", mainCD: 30, want: true},
		{name: "rt/dual/both_busy", gear: "dual", mainCD: 30, offCD: 30, want: true},
		{name: "rt/single/main_busy", gear: "single", mainCD: 999},
		{name: "rt/dual_no_main/both_ready", gear: "dual_no_main", want: true},
		{name: "tb/dual/main_cursor", tb: true, gear: "dual"},
		{name: "tb/dual/off_cursor", tb: true, gear: "dual", offCursor: true, want: true},
		{name: "tb/single/off_cursor", tb: true, gear: "single", offCursor: true},
		{name: "tb/dual_no_main/main_cursor", tb: true, gear: "dual_no_main", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			cs.game.turnBasedMode = tc.tb
			member := cs.game.party.Members[0]
			switch tc.gear {
			case "single":
				member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
				delete(member.Equipment, items.SlotOffHand)
			case "dual", "dual_no_main":
				makeDualWielder(t, member)
				if tc.gear == "dual_no_main" {
					delete(member.Equipment, items.SlotMainHand)
				}
			}
			member.RTCooldown, member.OffHandRTCooldown = tc.mainCD, tc.offCD
			member.NextTBAttackOffHand = tc.offCursor
			want := items.SlotMainHand
			if tc.want {
				want = items.SlotOffHand
			}
			if got := cs.attackSlotFor(member); got != want {
				t.Errorf("slot = %v, want %v", got, want)
			}
		})
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

// TestSpiritualTrainingChanceFormula checks the tier->chance formula.
func TestSpiritualTrainingChanceFormula(t *testing.T) {
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
}

// The Spiritual Training proc is a FREE offensive cast and nothing else: it
// never spends SP (fired or not), never free-procs a party buff, and never
// rolls its own Orc Warlord summon (that roll belongs to the swing that
// triggered it; the card is forced to 100% so a leak spawns at once). Only a
// trained monk with an offensive quick spell launches anything.
func TestSpiritualTrainingFreeOffensiveProcOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		skill    bool
		spell    spells.SpellID
		wantCast bool
	}{
		{name: "gm/offensive", skill: true, spell: "mind_blast", wantCast: true},
		{name: "untrained/offensive", spell: "mind_blast"},
		{name: "gm/party_buff", skill: true, spell: "bless"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			setupSummonableWorld(t, cs)
			forceOrcWarlordSummonAlways(t, g)
			member := g.party.Members[0]
			delete(member.Skills, character.SkillSpiritualTraining)
			if tc.skill {
				member.Skills[character.SkillSpiritualTraining] = &character.Skill{Mastery: character.MasteryGrandMaster}
			}
			spellItem, err := spells.CreateSpellItem(tc.spell)
			if err != nil {
				t.Fatalf("setup: CreateSpellItem(%s): %v", tc.spell, err)
			}
			member.Equipment[items.SlotSpell] = spellItem
			member.SpellPoints, member.MaxSpellPoints = 20, 20
			g.selectedChar = 0

			for i := 0; i < 500; i++ {
				cs.trySpiritualTraining(member)
				if member.SpellPoints != 20 {
					t.Fatalf("proc %d spent SP (20 -> %d); it must always be free", i, member.SpellPoints)
				}
			}
			if cast := len(g.magicProjectiles) > 0; cast != tc.wantCast {
				t.Errorf("projectiles launched = %v, want %v", cast, tc.wantCast)
			}
			if len(g.statBuffs) != 0 {
				t.Errorf("proc applied %d party buff(s); only offensive spells may proc", len(g.statBuffs))
			}
			if got := cs.countCardSummons(); got != 0 {
				t.Errorf("proc rolled its own Orc Warlord summon: %d allies", got)
			}
		})
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
	oldWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = nil
	t.Cleanup(func() { world.GlobalWorldManager = oldWorldManager })
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
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

// forceOrcWarlordSummonAlways slots the Orc Warlord Card and pins its authored
// chance to 100 for the test (restored via t.Cleanup), so tryCardSummonOnAction's
// percentage roll always passes - a probabilistic proc becomes a deterministic
// "did this even attempt to roll at all".
func forceOrcWarlordSummonAlways(t *testing.T, g *MMGame) {
	t.Helper()
	for i := range g.cardSlots {
		g.cardSlots[i].key = "orc_warlord_card"
	}
	def := cardDef("orc_warlord_card")
	if def == nil {
		t.Fatal("orc_warlord_card definition missing")
	}
	old := def.CardSummonChance
	def.CardSummonChance = 100
	t.Cleanup(func() { def.CardSummonChance = old })
	sources := g.cardSummonSources()
	if len(sources) != len(g.cardSlots) {
		t.Fatalf("forced summon sources = %d, want %d", len(sources), len(g.cardSlots))
	}
	for _, source := range sources {
		if source.Chance != 100 {
			t.Fatalf("forced %s chance = %d, want 100", source.CardKey, source.Chance)
		}
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
	forceOrcWarlordSummonAlways(t, g)

	member := g.party.Members[0]
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	delete(member.Skills, character.SkillSpiritualTraining)
	g.selectedChar = 0
	g.world.Monsters = nil
	tile := float64(g.config.GetTileSize())
	if _, _, ok := cs.findNearestSummonTile(g.camera.X+2*tile, g.camera.Y, 10); !ok {
		t.Fatal("summonable-world fixture has no free spawn tile")
	}

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
	member.LearnSpell("firebolt")
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
