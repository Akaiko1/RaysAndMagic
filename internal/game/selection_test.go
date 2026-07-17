package game

import (
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// selectionTestGame builds a 4-member party game ready for selection / action
// slot tests. Loads weapon + item configs because NewParty pulls starter
// equipment from YAML and would panic otherwise.
func selectionTestGame(t *testing.T) *MMGame {
	t.Helper()
	cfg := loadTestConfig(t)
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	for _, m := range g.party.Members {
		m.HitPoints = m.MaxHitPoints
		m.ActionsRemaining = 1
	}
	g.selectedChar = 0
	return g
}

func TestEnsureSelectedCharCanAct_NoopWhenSelectedIsAlive(t *testing.T) {
	g := selectionTestGame(t)
	g.ensureSelectedCharCanAct()
	if g.selectedChar != 0 {
		t.Errorf("selectedChar moved unexpectedly: %d, want 0", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_KeepsExhaustedLivingSelectedForUI(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].ActionsRemaining = 0
	g.ensureSelectedCharCanAct()
	if g.selectedChar != 0 {
		t.Errorf("selectedChar moved off exhausted living member: %d, want 0", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_KeepsStunnedLivingSelectedForUI(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].ApplyCharStun(60, 1)
	g.ensureSelectedCharCanAct()
	if g.selectedChar != 0 {
		t.Fatalf("selected character moved off stunned member to %d", g.selectedChar)
	}
}

func TestManualPartySelection_KeepsEradicatedMemberForInventory(t *testing.T) {
	g := selectionTestGame(t)
	eradicated := g.party.Members[1]
	eradicated.HitPoints = 0
	eradicated.AddCondition(character.ConditionEradicated)

	if !g.selectPartyMemberManually(1) {
		t.Fatal("manual selection rejected an existing eradicated member")
	}
	if !g.parkSelection {
		t.Fatal("manual selection must park the selected member")
	}
	g.ensureSelectedCharCanAct()
	if g.selectedChar != 1 {
		t.Fatalf("selection moved off eradicated member to %d", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_AdvancesWhenSelectedIsDead(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].HitPoints = 0
	g.party.Members[0].AddCondition(character.ConditionUnconscious)
	g.ensureSelectedCharCanAct()
	if g.selectedChar == 0 {
		t.Fatalf("selectedChar stayed on dead member 0")
	}
	if !g.party.Members[g.selectedChar].CanAct() {
		t.Errorf("snapped to non-actable index %d", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_SkipsDeadMembersInOrder(t *testing.T) {
	g := selectionTestGame(t)
	// Kill 0 and 1 - should snap to 2.
	for i := 0; i < 2; i++ {
		g.party.Members[i].HitPoints = 0
		g.party.Members[i].AddCondition(character.ConditionUnconscious)
	}
	g.ensureSelectedCharCanAct()
	if g.selectedChar != 2 {
		t.Errorf("selectedChar=%d, want 2 (first living after 0,1 are KO)", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_NoChangeWhenEveryoneDead(t *testing.T) {
	g := selectionTestGame(t)
	for _, m := range g.party.Members {
		m.HitPoints = 0
		m.AddCondition(character.ConditionUnconscious)
	}
	g.selectedChar = 0
	g.ensureSelectedCharCanAct()
	// firstEligiblePartyIndex returns -1; selectedChar should be unchanged.
	if g.selectedChar != 0 {
		t.Errorf("selectedChar moved to %d with full party wipe; expected unchanged", g.selectedChar)
	}
}

func TestEnsureSelectedCharCanAct_NoopInRealTime(t *testing.T) {
	g := selectionTestGame(t)
	g.turnBasedMode = false
	g.party.Members[0].HitPoints = 0
	g.party.Members[0].AddCondition(character.ConditionUnconscious)
	g.selectedChar = 0
	g.ensureSelectedCharCanAct()
	// Real-time mode: no auto-advance. Dead selected stays dead-selected.
	if g.selectedChar != 0 {
		t.Errorf("real-time mode shouldn't auto-advance; selectedChar=%d", g.selectedChar)
	}
}

func TestConsumeSelectedCharAction_DecrementsAndStays(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].ActionsRemaining = 3 // multiple actions
	g.consumeSelectedCharAction()
	if got := g.party.Members[0].ActionsRemaining; got != 2 {
		t.Errorf("ActionsRemaining=%d after one consume, want 2", got)
	}
	if g.selectedChar != 0 {
		t.Errorf("selectedChar moved to %d but member 0 still had actions", g.selectedChar)
	}
}

func TestConsumeSelectedCharAction_AdvancesOnExhaustion(t *testing.T) {
	g := selectionTestGame(t)
	g.party.Members[0].ActionsRemaining = 1
	// Members 1..3 all have 1 action remaining (default from selectionTestGame).
	g.consumeSelectedCharAction()
	if g.party.Members[0].ActionsRemaining != 0 {
		t.Errorf("member 0 ActionsRemaining=%d, want 0", g.party.Members[0].ActionsRemaining)
	}
	if g.selectedChar == 0 {
		t.Errorf("selectedChar should have advanced past 0")
	}
	if !g.canSelectChar(g.selectedChar) {
		t.Errorf("auto-advanced to non-actable index %d", g.selectedChar)
	}
}

func TestConsumeSelectedCharAction_EndsPartyTurnWhenAllExhausted(t *testing.T) {
	g := selectionTestGame(t)
	g.currentTurn = 0
	for _, m := range g.party.Members {
		m.ActionsRemaining = 0
	}
	g.party.Members[0].ActionsRemaining = 1 // only 0 has an action left
	g.consumeSelectedCharAction()
	if g.currentTurn != 1 {
		t.Errorf("currentTurn=%d, want 1 (monster turn) after exhausting last action", g.currentTurn)
	}
	if g.monsterTurnResolved {
		t.Errorf("monsterTurnResolved should be false to let monsters act")
	}
}

func TestConsumeSelectedCharAction_NoopInRealTime(t *testing.T) {
	g := selectionTestGame(t)
	g.turnBasedMode = false
	before := g.party.Members[0].ActionsRemaining
	g.consumeSelectedCharAction()
	if got := g.party.Members[0].ActionsRemaining; got != before {
		t.Errorf("ActionsRemaining changed in real-time mode: %d -> %d", before, got)
	}
}

func TestTurnBasedActorGateRejectsStunnedAndExhaustedMember(t *testing.T) {
	def, err := spells.GetSpellDefinitionByID("firebolt")
	if err != nil {
		t.Fatalf("load firebolt: %v", err)
	}

	for _, tc := range []struct {
		name              string
		setup             func(*character.MMCharacter)
		wantDirectBlocked bool
	}{
		{
			name: "stunned",
			setup: func(member *character.MMCharacter) {
				member.ApplyCharStun(60, 1)
			},
			wantDirectBlocked: true,
		},
		{
			name: "no_action_slot",
			setup: func(member *character.MMCharacter) {
				member.ActionsRemaining = 0
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			g.combat = NewCombatSystem(g)
			member := g.party.Members[0]
			member.SpellPoints = member.MaxSpellPoints
			tc.setup(member)

			beforeSP := member.SpellPoints
			if g.canSelectChar(0) {
				t.Fatal("ineligible member remained a turn-based actor")
			}
			if g.canSpendTurnBasedAction(0) {
				t.Fatal("UI action gate bypassed an ineligible turn-based actor")
			}
			if tc.wantDirectBlocked {
				if g.combat.castResolvedSpell("firebolt", def, member, def.SpellPointsCost, true, true) {
					t.Fatal("direct spell path bypassed stun")
				}
				if member.SpellPoints != beforeSP {
					t.Fatalf("blocked cast spent SP: %d -> %d", beforeSP, member.SpellPoints)
				}
			}
		})
	}
}

func TestSkipTurnBasedPartyTurnWithoutActor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(*MMGame)
		wantSkip bool
	}{
		{
			name: "all_stunned",
			setup: func(g *MMGame) {
				for _, member := range g.party.Members {
					member.ApplyCharStun(60, 1)
				}
			},
			wantSkip: true,
		},
		{
			name: "all_action_slots_spent",
			setup: func(g *MMGame) {
				for _, member := range g.party.Members {
					member.ActionsRemaining = 0
				}
			},
			wantSkip: true,
		},
		{
			name: "ko_and_exhausted_mix",
			setup: func(g *MMGame) {
				for i, member := range g.party.Members {
					if i < 2 {
						member.HitPoints = 0
						member.AddCondition(character.ConditionUnconscious)
					} else {
						member.ActionsRemaining = 0
					}
				}
			},
			wantSkip: true,
		},
		{
			name: "full_party_wipe_uses_game_over",
			setup: func(g *MMGame) {
				for _, member := range g.party.Members {
					member.HitPoints = 0
					member.AddCondition(character.ConditionUnconscious)
				}
			},
			wantSkip: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			g.currentTurn = 0
			tc.setup(g)

			if !g.partyAllExhausted() {
				t.Fatal("setup must leave no legal party actor")
			}
			if got := g.skipTurnBasedPartyTurnWithoutActor(); got != tc.wantSkip {
				t.Fatalf("skip=%v, want %v", got, tc.wantSkip)
			}
			if tc.wantSkip {
				if g.currentTurn != 1 || g.monsterTurnResolved {
					t.Fatalf("empty party turn did not hand control to monsters: turn=%d resolved=%v", g.currentTurn, g.monsterTurnResolved)
				}
			} else if g.currentTurn != 0 {
				t.Fatalf("full party wipe advanced to turn %d instead of remaining for game-over", g.currentTurn)
			}
		})
	}
}

func TestTurnBasedStunConsumesPartyTurnBeforeAutoPass(t *testing.T) {
	g := selectionTestGame(t)
	g.currentTurn = 0
	for _, member := range g.party.Members {
		member.ApplyCharStun(60, 1)
	}

	g.startPartyTurn()
	for i, member := range g.party.Members {
		if member.ActionsRemaining != 0 {
			t.Fatalf("stunned member %d received %d action slots", i, member.ActionsRemaining)
		}
	}
	if !g.skipTurnBasedPartyTurnWithoutActor() {
		t.Fatal("stunned party turn was not handed to monsters")
	}
	if g.currentTurn != 1 {
		t.Fatalf("currentTurn=%d, want monster turn after stunned party pass", g.currentTurn)
	}
}

func TestStartPartyTurn_AssignsSpeedBonusActionsByFastestMember(t *testing.T) {
	g := selectionTestGame(t)
	speeds := []int{10, 26, 20, 18}
	for i, speed := range speeds {
		g.party.Members[i].Speed = speed
		g.party.Members[i].ActionsRemaining = 0
	}

	g.startPartyTurn()

	got := []int{
		g.party.Members[0].ActionsRemaining,
		g.party.Members[1].ActionsRemaining,
		g.party.Members[2].ActionsRemaining,
		g.party.Members[3].ActionsRemaining,
	}
	want := []int{1, 2, 1, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slot %d actions=%d, want %d (all actions=%v)", i, got[i], want[i], got)
		}
	}
}

func TestStartPartyTurn_TwoSpeedBonusesGoToTwoFastestTieBySlot(t *testing.T) {
	g := selectionTestGame(t)
	speeds := []int{51, 30, 51, 10}
	for i, speed := range speeds {
		g.party.Members[i].Speed = speed
		g.party.Members[i].ActionsRemaining = 0
	}

	g.startPartyTurn()

	got := []int{
		g.party.Members[0].ActionsRemaining,
		g.party.Members[1].ActionsRemaining,
		g.party.Members[2].ActionsRemaining,
		g.party.Members[3].ActionsRemaining,
	}
	want := []int{2, 1, 2, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slot %d actions=%d, want %d (all actions=%v)", i, got[i], want[i], got)
		}
	}
}
