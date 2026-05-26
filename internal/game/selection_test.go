package game

import (
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
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
	// Kill 0 and 1 — should snap to 2.
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
