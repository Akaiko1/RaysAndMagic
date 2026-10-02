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

// TB keeps a living selected member (even exhausted or stunned, for the UI),
// snaps off a KO to the first one who can act, and stays put when nobody can;
// RT never auto-advances here.
func TestEnsureSelectedCharCanAct(t *testing.T) {
	knockOut := func(members ...*character.MMCharacter) {
		for _, m := range members {
			m.HitPoints = 0
			m.AddCondition(character.ConditionUnconscious)
		}
	}
	for _, tc := range []struct {
		name  string
		rt    bool
		setup func(g *MMGame)
		want  int
		// mustAct: the resulting selection must be a member who can act.
		mustAct bool
	}{
		{"alive_noop", false, func(*MMGame) {}, 0, true},
		{"exhausted_living_kept", false, func(g *MMGame) { g.party.Members[0].ActionsRemaining = 0 }, 0, false},
		{"stunned_living_kept", false, func(g *MMGame) { g.party.Members[0].ApplyCharStun(60, 1) }, 0, false},
		{"dead_advances", false, func(g *MMGame) { knockOut(g.party.Members[0]) }, 1, true},
		{"skips_dead_in_order", false, func(g *MMGame) { knockOut(g.party.Members[0], g.party.Members[1]) }, 2, true},
		{"everyone_dead_unchanged", false, func(g *MMGame) { knockOut(g.party.Members...) }, 0, false},
		{"rt_noop_on_dead", true, func(g *MMGame) { knockOut(g.party.Members[0]) }, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			g.turnBasedMode = !tc.rt
			tc.setup(g)
			g.selectedChar = 0
			g.ensureSelectedCharCanAct()
			if g.selectedChar != tc.want {
				t.Fatalf("selectedChar=%d, want %d", g.selectedChar, tc.want)
			}
			if tc.mustAct && !g.party.Members[g.selectedChar].CanAct() {
				t.Fatalf("snapped to non-actable index %d", g.selectedChar)
			}
		})
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

// One consumed TB action decrements the selected member, hands selection on
// once they are exhausted, and starts the monster phase when the whole party
// is spent; RT has no action slots to consume.
func TestConsumeSelectedCharAction(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rt          bool
		actions     [4]int // action slots before the consume
		wantActions int    // member 0 after the consume
		advance     bool   // selection must leave member 0 for a selectable member
		endTurn     bool   // the monster phase must begin
	}{
		{"decrements_and_stays", false, [4]int{3, 1, 1, 1}, 2, false, false},
		{"advances_on_exhaustion", false, [4]int{1, 1, 1, 1}, 0, true, false},
		{"ends_party_turn_when_all_exhausted", false, [4]int{1, 0, 0, 0}, 0, false, true},
		{"rt_noop", true, [4]int{1, 1, 1, 1}, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := selectionTestGame(t)
			g.turnBasedMode = !tc.rt
			g.currentTurn = 0
			for i, m := range g.party.Members {
				m.ActionsRemaining = tc.actions[i]
			}
			g.consumeSelectedCharAction()
			if got := g.party.Members[0].ActionsRemaining; got != tc.wantActions {
				t.Fatalf("ActionsRemaining=%d, want %d", got, tc.wantActions)
			}
			if tc.endTurn {
				if g.currentTurn != 1 || g.monsterTurnResolved {
					t.Fatalf("currentTurn=%d resolved=%v, want an unresolved monster turn", g.currentTurn, g.monsterTurnResolved)
				}
				return
			}
			if g.currentTurn != 0 {
				t.Fatalf("currentTurn=%d, want the party turn to continue", g.currentTurn)
			}
			if tc.advance {
				if g.selectedChar == 0 || !g.canSelectChar(g.selectedChar) {
					t.Fatalf("selection did not advance to a selectable member: %d", g.selectedChar)
				}
			} else if g.selectedChar != 0 {
				t.Fatalf("selectedChar moved to %d", g.selectedChar)
			}
		})
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
			if g.canSpendCombatAction(0) {
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
